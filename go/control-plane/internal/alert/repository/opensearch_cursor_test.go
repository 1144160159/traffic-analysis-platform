package repository

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	commonErrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"go.uber.org/zap"
)

func newCursorTestRepository(t *testing.T, handler http.HandlerFunc, enabled bool) *OpenSearchRepository {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	repository, err := NewOpenSearchRepository(OpenSearchConfig{
		Addresses:          []string{server.URL},
		ReadTarget:         "alerts-v2-read",
		WriteTarget:        "alerts-v2-write",
		ExactTarget:        true,
		CursorEnabled:      enabled,
		CursorSigningKey:   "unit-test-root-signing-key-with-sufficient-entropy",
		ShallowResultLimit: 1000,
		MaxPageSize:        200,
		QueryTimeout:       2 * time.Second,
		CursorTTL:          2 * time.Minute,
		TrackTotalHitsUpTo: 10000,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return repository
}

func writeOpenSearchJSON(t *testing.T, writer http.ResponseWriter, payload string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if _, err := writer.Write([]byte(payload)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func decodeRequestJSON(t *testing.T, request *http.Request) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return body
}

func TestOpenSearchCursorLiveTraversalUsesBoundedStableSearchAfter(t *testing.T) {
	var mutex sync.Mutex
	searchCalls := 0
	repository := newCursorTestRepository(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writeOpenSearchJSON(t, writer, `{}`)
			return
		}
		if request.Method != http.MethodPost || request.URL.Path != "/alerts-v2-read/_search" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
		if request.URL.Query().Get("allow_partial_search_results") != "false" ||
			request.URL.Query().Get("timeout") != "2000ms" ||
			request.URL.Query().Get("track_total_hits") != "10000" {
			t.Fatalf("missing search guards: %s", request.URL.RawQuery)
		}
		body := decodeRequestJSON(t, request)
		if _, exists := body["from"]; exists {
			t.Fatal("cursor search must not send from")
		}
		if _, exists := body["aggs"]; exists {
			t.Fatal("cursor search must not run dashboard aggregations")
		}
		if body["size"] != float64(3) {
			t.Fatalf("size = %#v, want requested size + 1", body["size"])
		}
		sorts, ok := body["sort"].([]any)
		if !ok || len(sorts) != 2 {
			t.Fatalf("stable two-field sort missing: %#v", body["sort"])
		}
		boolQuery := body["query"].(map[string]any)["bool"].(map[string]any)
		filters := boolQuery["filter"].([]any)
		firstFilter := filters[0].(map[string]any)["term"].(map[string]any)
		if firstFilter["tenant_id"] != "tenant-a" {
			t.Fatalf("tenant filter missing from filter context: %#v", boolQuery)
		}
		if _, exists := boolQuery["must"].([]any)[0].(map[string]any)["term"]; exists {
			t.Fatal("tenant filter must not be placed in relevance must context")
		}
		if len(body["_source"].([]any)) < 20 {
			t.Fatalf("source allowlist missing: %#v", body["_source"])
		}

		mutex.Lock()
		defer mutex.Unlock()
		searchCalls++
		if searchCalls == 1 {
			if _, exists := body["search_after"]; exists {
				t.Fatal("first cursor page must not send search_after")
			}
			writeOpenSearchJSON(t, writer, `{
			  "took":4,"timed_out":false,"_shards":{"failed":0},
			  "hits":{"total":{"value":3,"relation":"eq"},"hits":[
			    {"_source":{"tenant_id":"tenant-a","alert_id":"a3"},"sort":["2026-08-04T01:00:03Z","a3"]},
			    {"_source":{"tenant_id":"tenant-a","alert_id":"a2"},"sort":["2026-08-04T01:00:02Z","a2"]},
			    {"_source":{"tenant_id":"tenant-a","alert_id":"a1"},"sort":["2026-08-04T01:00:01Z","a1"]}
			  ]}}
			}`)
			return
		}
		searchAfter := body["search_after"].([]any)
		if len(searchAfter) != 2 || searchAfter[1] != "a2" {
			t.Fatalf("search_after = %#v, want last returned hit", searchAfter)
		}
		writeOpenSearchJSON(t, writer, `{
		  "took":2,"timed_out":false,"_shards":{"failed":0},
		  "hits":{"total":{"value":3,"relation":"eq"},"hits":[
		    {"_source":{"tenant_id":"tenant-a","alert_id":"a1"},"sort":["2026-08-04T01:00:01Z","a1"]}
		  ]}}
	}`)
	}, true)

	query := &SearchQuery{
		TenantID: "tenant-a", Query: "malware", Severity: []string{"high"},
		Size: 2, SortField: "last_seen", SortOrder: "desc", CursorMode: SearchCursorModeLive,
	}
	first, err := repository.Search(context.Background(), query)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if !first.HasMore || first.NextCursor == "" || len(first.Alerts) != 2 || first.Partial {
		t.Fatalf("unexpected first page: %+v", first)
	}
	secondQuery := *query
	secondQuery.Cursor = first.NextCursor
	second, err := repository.Search(context.Background(), &secondQuery)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Alerts) != 1 || second.Alerts[0].AlertID != "a1" {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestOpenSearchCursorRejectsTenantTamperAndQueryDriftBeforeSearch(t *testing.T) {
	searches := 0
	repository := newCursorTestRepository(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writeOpenSearchJSON(t, writer, `{}`)
			return
		}
		searches++
		writeOpenSearchJSON(t, writer, `{
		  "took":1,"timed_out":false,"_shards":{"failed":0},
		  "hits":{"total":{"value":2,"relation":"eq"},"hits":[
		    {"_source":{"tenant_id":"tenant-a","alert_id":"a2"},"sort":[2,"a2"]},
		    {"_source":{"tenant_id":"tenant-a","alert_id":"a1"},"sort":[1,"a1"]}
		  ]}}
	}`)
	}, true)
	base := &SearchQuery{TenantID: "tenant-a", Query: "fixed", Size: 1, CursorMode: SearchCursorModeLive}
	first, err := repository.Search(context.Background(), base)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("create cursor: result=%+v err=%v", first, err)
	}
	if searches != 1 {
		t.Fatalf("searches = %d, want 1", searches)
	}

	crossTenant := *base
	crossTenant.TenantID = "tenant-b"
	crossTenant.Cursor = first.NextCursor
	if _, err := repository.Search(context.Background(), &crossTenant); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) {
		t.Fatalf("cross-tenant cursor error = %v", err)
	}
	drifted := *base
	drifted.Query = "changed"
	drifted.Cursor = first.NextCursor
	if _, err := repository.Search(context.Background(), &drifted); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) {
		t.Fatalf("query-drift cursor error = %v", err)
	}
	tampered := *base
	parts := strings.Split(first.NextCursor, ".")
	replacement := byte('A')
	if parts[1][10] == replacement {
		replacement = 'B'
	}
	parts[1] = parts[1][:10] + string(replacement) + parts[1][11:]
	tampered.Cursor = strings.Join(parts, ".")
	if _, err := repository.Search(context.Background(), &tampered); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) {
		t.Fatalf("tampered cursor error = %v", err)
	}
	if searches != 1 {
		t.Fatalf("invalid cursors reached OpenSearch: searches=%d", searches)
	}
}

func TestOpenSearchPITCursorCreatesRotatesAndClosesTenantContext(t *testing.T) {
	created := false
	closedPIT := ""
	repository := newCursorTestRepository(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/":
			writeOpenSearchJSON(t, writer, `{}`)
		case request.Method == http.MethodPost && request.URL.Path == "/alerts-v2-read/_search/point_in_time":
			created = true
			if request.URL.Query().Get("keep_alive") != "120000ms" {
				t.Fatalf("PIT keep_alive = %q", request.URL.Query().Get("keep_alive"))
			}
			writeOpenSearchJSON(t, writer, `{"pit_id":"pit-1","_shards":{"total":1,"successful":1,"failed":0}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/_search":
			body := decodeRequestJSON(t, request)
			pit := body["pit"].(map[string]any)
			if pit["id"] != "pit-1" || pit["keep_alive"] != "2m" {
				t.Fatalf("unexpected PIT search body: %#v", body)
			}
			writeOpenSearchJSON(t, writer, `{
			  "pit_id":"pit-2","took":3,"timed_out":false,"_shards":{"failed":0},
			  "hits":{"total":{"value":2,"relation":"eq"},"hits":[
			    {"_source":{"tenant_id":"tenant-a","alert_id":"a2"},"sort":[2,"a2"]},
			    {"_source":{"tenant_id":"tenant-a","alert_id":"a1"},"sort":[1,"a1"]}
			  ]}}
			}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/_search/point_in_time":
			body := decodeRequestJSON(t, request)
			ids := body["pit_id"].([]any)
			closedPIT = ids[0].(string)
			writeOpenSearchJSON(t, writer, `{"pits":[{"pit_id":"pit-2","successful":true}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	}, true)
	result, err := repository.Search(context.Background(), &SearchQuery{
		TenantID: "tenant-a", Size: 1, CursorMode: SearchCursorModePIT,
	})
	if err != nil {
		t.Fatalf("PIT search: %v", err)
	}
	if !created || result.NextCursor == "" || result.SnapshotID == "" || !result.HasMore {
		t.Fatalf("unexpected PIT result: %+v", result)
	}
	if err := repository.CloseSearchCursor(context.Background(), "tenant-a", result.NextCursor); err != nil {
		t.Fatalf("close PIT cursor: %v", err)
	}
	if closedPIT != "pit-2" {
		t.Fatalf("closed PIT = %q, want rotated pit-2", closedPIT)
	}
}

func TestOpenSearchCursorFailsClosedOnTimeoutOrShardFailure(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		response string
	}{
		{"timeout", `{"took":2000,"timed_out":true,"_shards":{"failed":0},"hits":{"total":{"value":0,"relation":"eq"},"hits":[]}}`},
		{"shard failure", `{"took":3,"timed_out":false,"_shards":{"failed":1},"hits":{"total":{"value":1,"relation":"gte"},"hits":[]}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := newCursorTestRepository(t, func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/" {
					writeOpenSearchJSON(t, writer, `{}`)
					return
				}
				writeOpenSearchJSON(t, writer, testCase.response)
			}, true)
			_, err := repository.Search(context.Background(), &SearchQuery{
				TenantID: "tenant-a", Size: 20, CursorMode: SearchCursorModeLive,
			})
			if !commonErrors.IsCode(err, commonErrors.ErrCodeOpenSearchError) ||
				!strings.Contains(err.Error(), "did not complete") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestOpenSearchLegacyCompatibilityAndEnabledShallowBound(t *testing.T) {
	requestBodies := make(chan map[string]any, 1)
	handler := func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writeOpenSearchJSON(t, writer, `{}`)
			return
		}
		requestBodies <- decodeRequestJSON(t, request)
		writeOpenSearchJSON(t, writer, `{
		  "took":1,"timed_out":false,"_shards":{"failed":0},
		  "hits":{"total":{"value":10001,"relation":"eq"},"hits":[]},"aggregations":{}
	}`)
	}
	legacy := newCursorTestRepository(t, handler, false)
	if _, err := legacy.Search(context.Background(), &SearchQuery{TenantID: "tenant-a", From: 9999, Size: 2}); err != nil {
		t.Fatalf("legacy compatibility search: %v", err)
	}
	body := <-requestBodies
	if body["from"] != float64(9999) || body["size"] != float64(2) || body["aggs"] == nil {
		t.Fatalf("legacy request changed: %#v", body)
	}
	if _, err := legacy.Search(context.Background(), &SearchQuery{TenantID: "tenant-a", CursorMode: SearchCursorModeLive}); !commonErrors.IsCode(err, commonErrors.ErrCodeServiceUnavailable) {
		t.Fatalf("disabled cursor error = %v", err)
	}

	enabled := newCursorTestRepository(t, handler, true)
	if _, err := enabled.Search(context.Background(), &SearchQuery{TenantID: "tenant-a", From: 999, Size: 2}); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) {
		t.Fatalf("unbounded shallow search error = %v", err)
	}
}

func TestOpenSearchLegacyBoundedSummaryOmitsUnsafeAggregations(t *testing.T) {
	repository := newCursorTestRepository(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writeOpenSearchJSON(t, writer, `{}`)
			return
		}
		if got := request.URL.Query().Get("track_total_hits"); got != "10000" {
			t.Fatalf("track_total_hits=%q want 10000", got)
		}
		body := decodeRequestJSON(t, request)
		if _, exists := body["aggs"]; exists {
			t.Fatalf("bounded summary sent unsafe aggregations: %#v", body)
		}
		writeOpenSearchJSON(t, writer, `{
		  "took":12,"timed_out":false,"_shards":{"failed":0},
		  "hits":{"total":{"value":10000,"relation":"gte"},"hits":[
		    {"_source":{"tenant_id":"tenant-a","alert_id":"a1","last_seen":"2026-08-04T01:00:00Z"}}
		  ]}}
		`)
	}, false)
	result, err := repository.Search(context.Background(), &SearchQuery{
		TenantID: "tenant-a", Size: 1, SortField: "last_seen", SortOrder: "desc",
		OmitAggregations: true, BoundedTotalHits: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AggregationsOmitted || result.Total != 10000 || result.TotalRelation != "gte" || len(result.Alerts) != 1 {
		t.Fatalf("unexpected bounded summary: %+v", result)
	}
}

func TestSearchCursorCodecExpiresAndRejectsUnknownClaims(t *testing.T) {
	codec, err := newSearchCursorCodec("unit-test-secret", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC)
	codec.now = func() time.Time { return now }
	token, err := codec.encode("tenant-a", strings.Repeat("a", 64), SearchCursorModeLive, 50,
		[]json.RawMessage{json.RawMessage(`1`), json.RawMessage(`"a1"`)}, "", now)
	if err != nil {
		t.Fatal(err)
	}
	codec.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := codec.decode(token, "tenant-a"); err != errSearchCursorExpired {
		t.Fatalf("expired cursor error = %v", err)
	}
	codec.now = func() time.Time { return now }
	payload := []byte(`{"v":1,"tenant_id":"tenant-a","query_sha256":"` + strings.Repeat("a", 64) +
		`","mode":"live","size":50,"sort_values":[1,"a1"],"expires_at":` +
		strconv.FormatInt(now.Add(time.Minute).Unix(), 10) + `,"unknown":true}`)
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, codec.signingKey)
	_, _ = mac.Write([]byte(payloadPart))
	unknownToken := payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, err := codec.decode(unknownToken, "tenant-a"); err != errSearchCursorInvalid {
		t.Fatalf("unknown claims error = %v", err)
	}
}
