package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	commonErrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"go.uber.org/zap"
)

var m09N015ResourceName = regexp.MustCompile(`^m09-n015-[a-f0-9]{10}-(?:read|a|b)$`)

type m09N015OpenSearch struct {
	baseURL string
	user    string
	pass    string
	client  *http.Client
}

func newM09N015OpenSearch(t *testing.T) *m09N015OpenSearch {
	t.Helper()
	baseURL := strings.TrimRight(os.Getenv("M09_N015_OPENSEARCH_URL"), "/")
	if os.Getenv("RUN_M09_N015_OPENSEARCH_INTEGRATION") != "1" || baseURL == "" {
		t.Skip("M09-N015 Kubernetes OpenSearch integration is not enabled")
	}
	if baseURL != "http://opensearch.middleware.svc:9200" {
		t.Fatalf("refusing undeclared OpenSearch service %q", baseURL)
	}
	return &m09N015OpenSearch{
		baseURL: baseURL, user: os.Getenv("OPENSEARCH_USERNAME"), pass: os.Getenv("OPENSEARCH_PASSWORD"),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *m09N015OpenSearch) do(t *testing.T, method, path, payload string, statuses ...int) []byte {
	t.Helper()
	var body io.Reader
	if payload != "" {
		body = bytes.NewBufferString(payload)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, o.baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth(o.user, o.pass)
	if payload != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := o.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if response.StatusCode == status {
			return responseBody
		}
	}
	t.Fatalf("%s %s returned %s: %s", method, path, response.Status, responseBody)
	return nil
}

func m09N015Names(t *testing.T) (string, string, string) {
	t.Helper()
	suffix := strings.TrimSpace(os.Getenv("M09_N015_RESOURCE_SUFFIX"))
	if !regexp.MustCompile(`^[a-f0-9]{10}$`).MatchString(suffix) {
		t.Fatalf("invalid M09_N015_RESOURCE_SUFFIX %q", suffix)
	}
	alias, indexA, indexB := "m09-n015-"+suffix+"-read", "m09-n015-"+suffix+"-a", "m09-n015-"+suffix+"-b"
	for _, name := range []string{alias, indexA, indexB} {
		if !m09N015ResourceName.MatchString(name) {
			t.Fatalf("refusing unsafe OpenSearch resource name %q", name)
		}
	}
	return alias, indexA, indexB
}

func m09N015DeleteIndices(t *testing.T, client *m09N015OpenSearch, indices ...string) {
	t.Helper()
	for _, index := range indices {
		if !m09N015ResourceName.MatchString(index) || strings.HasSuffix(index, "-read") {
			t.Fatalf("refusing unsafe OpenSearch cleanup target %q", index)
		}
		client.do(t, http.MethodDelete, "/"+index, "", http.StatusOK, http.StatusNotFound)
	}
}

func m09N015CreateIndex(t *testing.T, client *m09N015OpenSearch, index string) {
	t.Helper()
	payload := `{"settings":{"number_of_shards":1,"number_of_replicas":0},"mappings":{"dynamic":false,"properties":{"tenant_id":{"type":"keyword"},"alert_id":{"type":"keyword"},"search_text":{"type":"text"},"severity":{"type":"keyword"},"status":{"type":"keyword"},"last_seen":{"type":"date"},"first_seen":{"type":"date"},"score":{"type":"float"},"rule_version":{"type":"keyword"},"model_version":{"type":"keyword"},"attack_phase":{"type":"keyword"}}}}`
	client.do(t, http.MethodPut, "/"+index, payload, http.StatusOK)
}

func m09N015PutAlert(t *testing.T, client *m09N015OpenSearch, index, tenant, id, observed string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tenant_id": tenant, "alert_id": id, "search_text": "bounded cursor integration",
		"severity": "high", "status": "new", "last_seen": observed, "first_seen": observed,
		"score": 0.9, "rule_version": "rule-v1", "model_version": "model-v1", "attack_phase": "execution",
	})
	if err != nil {
		t.Fatal(err)
	}
	client.do(t, http.MethodPut, "/"+index+"/_doc/"+id+"?refresh=true", string(payload), http.StatusCreated, http.StatusOK)
}

func m09N015SwitchAlias(t *testing.T, client *m09N015OpenSearch, alias, from, to string) {
	t.Helper()
	actions := make([]map[string]any, 0, 2)
	if from != "" {
		actions = append(actions, map[string]any{"remove": map[string]any{"index": from, "alias": alias}})
	}
	actions = append(actions, map[string]any{"add": map[string]any{"index": to, "alias": alias}})
	payload, err := json.Marshal(map[string]any{"actions": actions})
	if err != nil {
		t.Fatal(err)
	}
	client.do(t, http.MethodPost, "/_aliases", string(payload), http.StatusOK)
}

func m09N015Repository(t *testing.T, client *m09N015OpenSearch, alias string) *OpenSearchRepository {
	t.Helper()
	repository, err := NewOpenSearchRepository(OpenSearchConfig{
		Addresses: []string{client.baseURL}, Username: client.user, Password: client.pass,
		ReadTarget: alias, WriteTarget: alias, ExactTarget: true, CursorEnabled: true,
		CursorSigningKey:   "m09-n015-k8s-cursor-signing-key-with-domain-separation",
		ShallowResultLimit: 1000, MaxPageSize: 20, QueryTimeout: 3 * time.Second,
		CursorTTL: 2 * time.Minute, TrackTotalHitsUpTo: 10000,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func TestOpenSearchCursorK8sIntegration(t *testing.T) {
	client := newM09N015OpenSearch(t)
	alias, indexA, indexB := m09N015Names(t)
	m09N015DeleteIndices(t, client, indexA, indexB)
	defer m09N015DeleteIndices(t, client, indexA, indexB)
	m09N015CreateIndex(t, client, indexA)
	m09N015CreateIndex(t, client, indexB)
	for sequence, id := range []string{"a1", "a2", "a3"} {
		m09N015PutAlert(t, client, indexA, "tenant-a", id, fmt.Sprintf("2026-08-16T01:00:0%dZ", sequence+1))
	}
	m09N015PutAlert(t, client, indexA, "tenant-b", "tenant-b-only", "2026-08-16T01:00:09Z")
	m09N015PutAlert(t, client, indexB, "tenant-a", "b1", "2026-08-16T02:00:00Z")
	m09N015SwitchAlias(t, client, alias, "", indexA)
	repository := m09N015Repository(t, client, alias)
	defer repository.Close()

	liveQuery := &SearchQuery{TenantID: "tenant-a", Size: 1, SortField: "last_seen", SortOrder: "desc", CursorMode: SearchCursorModeLive}
	liveFirst, err := repository.Search(context.Background(), liveQuery)
	if err != nil || len(liveFirst.Alerts) != 1 || liveFirst.Alerts[0].AlertID != "a3" || liveFirst.NextCursor == "" {
		t.Fatalf("live first page = %+v err=%v", liveFirst, err)
	}
	m09N015SwitchAlias(t, client, alias, indexA, indexB)
	liveNext := *liveQuery
	liveNext.Cursor = liveFirst.NextCursor
	if _, err := repository.Search(context.Background(), &liveNext); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("live cursor did not fail closed after alias switch: %v", err)
	}

	m09N015SwitchAlias(t, client, alias, indexB, indexA)
	pitQuery := &SearchQuery{TenantID: "tenant-a", Size: 1, SortField: "last_seen", SortOrder: "desc", CursorMode: SearchCursorModePIT}
	pitPage, err := repository.Search(context.Background(), pitQuery)
	if err != nil || pitPage.NextCursor == "" || pitPage.SnapshotID == "" {
		t.Fatalf("PIT first page = %+v err=%v", pitPage, err)
	}
	ids := []string{pitPage.Alerts[0].AlertID}
	firstPITCursor := pitPage.NextCursor
	m09N015SwitchAlias(t, client, alias, indexA, indexB)
	m09N015PutAlert(t, client, indexA, "tenant-a", "a4-after-pit", "2026-08-16T03:00:00Z")
	for pitPage.HasMore {
		next := *pitQuery
		next.Cursor = pitPage.NextCursor
		pitPage, err = repository.Search(context.Background(), &next)
		if err != nil {
			t.Fatalf("PIT continuation after alias switch: %v", err)
		}
		for _, alert := range pitPage.Alerts {
			ids = append(ids, alert.AlertID)
		}
	}
	if strings.Join(ids, ",") != "a3,a2,a1" {
		t.Fatalf("PIT snapshot IDs = %v, want frozen tenant-a set", ids)
	}
	crossTenant := *pitQuery
	crossTenant.TenantID = "tenant-b"
	crossTenant.Cursor = firstPITCursor
	if _, err := repository.Search(context.Background(), &crossTenant); !commonErrors.IsCode(err, commonErrors.ErrCodeInvalidParameter) {
		t.Fatalf("cross-tenant PIT cursor error = %v", err)
	}
	if len(pitPage.SourceWatermarks["opensearch.alerts.target_sha256"]) != 64 {
		t.Fatalf("PIT source watermarks = %#v", pitPage.SourceWatermarks)
	}

	unavailable, err := NewOpenSearchRepository(OpenSearchConfig{
		Addresses: []string{"http://127.0.0.1:1"}, ReadTarget: alias, WriteTarget: alias, ExactTarget: true,
		CursorEnabled: true, CursorSigningKey: "m09-n015-k8s-unavailable-signing-key-with-enough-bytes",
		QueryTimeout: 500 * time.Millisecond, CursorTTL: time.Minute,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer unavailable.Close()
	if _, err := unavailable.Search(context.Background(), liveQuery); !commonErrors.IsCode(err, commonErrors.ErrCodeOpenSearchError) {
		t.Fatalf("unavailable OpenSearch did not fail closed: %v", err)
	}
}

func TestOpenSearchCursorK8sCleanupOracle(t *testing.T) {
	client := newM09N015OpenSearch(t)
	alias, indexA, indexB := m09N015Names(t)
	m09N015DeleteIndices(t, client, indexA, indexB)
	for _, path := range []string{"/" + alias, "/" + indexA, "/" + indexB} {
		client.do(t, http.MethodHead, path, "", http.StatusNotFound)
	}
}
