package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestOpenSearchWritesUseExternalGTEVersioning(t *testing.T) {
	var mu sync.Mutex
	var singleQuery string
	var bulkBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/":
			_, _ = io.WriteString(writer, `{"version":{"number":"2.11.0"}}`)
		case request.URL.Path == "/alerts-v2-write/_doc/alert-a":
			mu.Lock()
			singleQuery = request.URL.RawQuery
			mu.Unlock()
			_, _ = io.WriteString(writer, `{"result":"created"}`)
		case request.URL.Path == "/_bulk":
			body, _ := io.ReadAll(request.Body)
			mu.Lock()
			bulkBody = string(body)
			mu.Unlock()
			_, _ = io.WriteString(writer, `{"errors":false,"items":[{"index":{"_id":"alert-a","status":201}}]}`)
		default:
			http.Error(writer, fmt.Sprintf("unexpected path %s", request.URL.Path), http.StatusNotFound)
		}
	}))
	defer server.Close()

	writer, err := NewOpenSearchWriter([]string{server.URL}, "", "", "alerts-v2-write", true, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	now := time.Unix(1_800_000_000, 123_000_000).UTC()
	alert := &Alert{TenantID: "tenant-a", AlertID: "alert-a", FirstSeen: now, LastSeen: now, UpdatedTs: now}
	version := AlertSourceVersion(alert)

	if err := writer.WriteAlert(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBatch(context.Background(), []*Alert{alert}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(singleQuery, "version="+fmt.Sprint(version)) || !strings.Contains(singleQuery, "version_type=external_gte") {
		t.Fatalf("single write omitted external_gte source version: %s", singleQuery)
	}
	line := strings.Split(strings.TrimSpace(bulkBody), "\n")[0]
	var metadata map[string]map[string]any
	if err := json.Unmarshal([]byte(line), &metadata); err != nil {
		t.Fatal(err)
	}
	index := metadata["index"]
	if index["version_type"] != "external_gte" || int64(index["version"].(float64)) != version {
		t.Fatalf("bulk write omitted external_gte source version: %s", line)
	}
}

func TestOpenSearchReconcileReadsFrozenLegacyExactTargetWithKeywordFields(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/":
			_, _ = io.WriteString(writer, `{"version":{"number":"2.11.0"}}`)
		case "/alerts/_search":
			body, _ := io.ReadAll(request.Body)
			requestBody = string(body)
			_, _ = io.WriteString(writer, `{"timed_out":false,"_shards":{"failed":0},"hits":{"hits":[{"_source":{"tenant_id":"tenant-a","alert_id":"alert-a","dedup_fingerprint":"legacy-fingerprint"},"sort":["alert-a"]}]}}`)
		default:
			http.Error(writer, fmt.Sprintf("unexpected path %s", request.URL.Path), http.StatusNotFound)
		}
	}))
	defer server.Close()

	writer, err := NewOpenSearchReconcileTarget(
		[]string{server.URL}, "", "", "alerts", "traffic-alerts", false, true, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	alerts, truncated, err := writer.ListProjectionAlerts(context.Background(), ProjectionScope{
		TenantID: "tenant-a", BusinessIDs: []string{"alert-a"},
		TargetIndexVersion: defaultAlertProjectionTargetVersion, MaxDocuments: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(alerts) != 1 || alerts[0].AlertID != "alert-a" || alerts[0].Fingerprint != "legacy-fingerprint" {
		t.Fatalf("unexpected legacy projection result: truncated=%t alerts=%+v", truncated, alerts)
	}
	for _, required := range []string{`"tenant_id.keyword":"tenant-a"`, `"alert_id.keyword":["alert-a"]`, `"alert_id.keyword":{"order":"asc"}`} {
		if !strings.Contains(requestBody, required) {
			t.Fatalf("legacy exact-target request omitted %s: %s", required, requestBody)
		}
	}
	if writer.readTarget != "alerts" || writer.writeTarget != "traffic-alerts" {
		t.Fatalf("read and write targets were not kept independent: read=%s write=%s", writer.readTarget, writer.writeTarget)
	}
}

func TestOpenSearchWriterPreservesLegacyWildcardReadCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"version":{"number":"2.11.0"}}`)
	}))
	defer server.Close()
	writer, err := NewOpenSearchWriter([]string{server.URL}, "", "", "traffic-alerts", false, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if writer.readTarget != "traffic-alerts-*" || !writer.legacyReadKeywordFields {
		t.Fatalf("legacy compatibility target drifted: read=%s keyword_fields=%t", writer.readTarget, writer.legacyReadKeywordFields)
	}
}

func TestProjectionMetadataBindsClusterAndSingleWriteIndexReadOnly(t *testing.T) {
	var aliasMethod string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/":
			_, _ = io.WriteString(writer, `{"cluster_uuid":"cluster-a","version":{"number":"2.11.0"}}`)
		case "/_alias/alerts-v2-write":
			aliasMethod = request.Method
			_, _ = io.WriteString(writer, `{"alerts-v2-000002":{"aliases":{"alerts-v2-write":{"is_write_index":false}}},"alerts-v2-000001":{"aliases":{"alerts-v2-write":{"is_write_index":true}}}}`)
		default:
			http.Error(writer, fmt.Sprintf("unexpected path %s", request.URL.Path), http.StatusNotFound)
		}
	}))
	defer server.Close()

	projection, err := NewOpenSearchReconcileTarget(
		[]string{server.URL}, "", "", "alerts", "alerts-v2-write", true, true, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	metadata, err := projection.ProjectionMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ClusterUUID != "cluster-a" || metadata.ReadTarget != "alerts" || metadata.WriteAlias != "alerts-v2-write" || len(metadata.WriteIndices) != 2 {
		t.Fatalf("unexpected projection metadata: %+v", metadata)
	}
	if aliasMethod != http.MethodGet || metadata.WriteIndices[0].Index != "alerts-v2-000001" || !metadata.WriteIndices[0].IsWriteIndex || metadata.WriteIndices[1].IsWriteIndex {
		t.Fatalf("alias membership was not read deterministically: method=%s metadata=%+v", aliasMethod, metadata)
	}
}

func TestProjectionMetadataReturnsMissingAliasAsBlockedEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/" {
			_, _ = io.WriteString(writer, `{"cluster_uuid":"cluster-a","version":{"number":"2.11.0"}}`)
			return
		}
		http.Error(writer, `{"error":"alias missing"}`, http.StatusNotFound)
	}))
	defer server.Close()

	projection, err := NewOpenSearchReconcileTarget(
		[]string{server.URL}, "", "", "alerts", "alerts-v2-write", true, true, zap.NewNop(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer projection.Close()
	metadata, err := projection.ProjectionMetadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ClusterUUID != "cluster-a" || len(metadata.WriteIndices) != 0 {
		t.Fatalf("missing alias did not produce blocked evidence metadata: %+v", metadata)
	}
}
