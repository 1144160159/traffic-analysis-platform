package consumer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	graphNebula "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/graph/nebula"
)

type recordingNebulaAssetWriter struct {
	calls      int
	projection graphNebula.AssetEntityProjection
	err        error
}

func (w *recordingNebulaAssetWriter) UpsertAssetEntity(
	_ context.Context,
	projection graphNebula.AssetEntityProjection,
) error {
	w.calls++
	w.projection = projection
	return w.err
}

func TestOpenSearchAssetProjectionUsesDeterministicIDAndExternalVersion(t *testing.T) {
	event := validAssetProjectionEvent()
	var requestBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/assets-v2-write/_doc/"+event.AssetID {
			t.Fatalf("path=%s", request.URL.Path)
		}
		if got := request.URL.Query().Get("version"); got != "2" {
			t.Fatalf("version=%q", got)
		}
		if got := request.URL.Query().Get("version_type"); got != "external_gte" {
			t.Fatalf("version_type=%q", got)
		}
		requestBody, _ = io.ReadAll(request.Body)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"result":"created"}`))
	}))
	defer server.Close()

	target, err := NewOpenSearchAssetProjection([]string{server.URL}, "", "", "assets-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	first, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("OpenSearch projection is not deterministic")
	}
	if err := target.Apply(context.Background(), event, first); err != nil {
		t.Fatal(err)
	}
	if string(requestBody) != string(first) {
		t.Fatal("OpenSearch body differs from deterministic projection")
	}
}

func TestOpenSearchAssetProjectionRejectsTargetFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":"version conflict"}`))
	}))
	defer server.Close()
	target, err := NewOpenSearchAssetProjection([]string{server.URL}, "", "", "assets-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	event := validAssetProjectionEvent()
	projection, _ := target.Projection(event)
	if err := target.Apply(context.Background(), event, projection); err == nil ||
		!strings.Contains(err.Error(), "409") {
		t.Fatalf("expected target status error, got %v", err)
	}
}

func TestOpenSearchAssetProjectionReadinessRequiresWriteAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			_, _ = writer.Write([]byte(`{"version":{"number":"2.14.0"}}`))
		case request.Method == http.MethodHead && request.URL.Path == "/_alias/assets-v2-write":
			writer.WriteHeader(http.StatusNotFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	target, err := NewOpenSearchAssetProjection([]string{server.URL}, "", "", "assets-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "write alias") {
		t.Fatalf("expected missing write alias error, got %v", err)
	}
}

// This test is deliberately guarded by both a loopback endpoint and a sentinel
// document. The alignment runner creates and removes the owned OpenSearch
// container; this test must never accept a shared or production cluster.
func TestOpenSearchAssetProjectionEphemeralIntegration(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("ASSET_PROJECTION_EPHEMERAL_OS_URL"), "/")
	if baseURL == "" {
		t.Skip("ASSET_PROJECTION_EPHEMERAL_OS_URL is not set")
	}
	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Fatalf("refusing non-loopback OpenSearch endpoint: %q", baseURL)
	}
	if os.Getenv("ASSET_PROJECTION_EPHEMERAL_OS_SENTINEL") != "ephemeral-only" {
		t.Fatal("refusing OpenSearch endpoint without explicit ephemeral sentinel")
	}
	response, err := http.Get(baseURL + "/codex-ephemeral-asset-projection-sentinel/_doc/ephemeral-only")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var sentinel struct {
		Found  bool `json:"found"`
		Source struct {
			Marker string `json:"marker"`
		} `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&sentinel); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !sentinel.Found || sentinel.Source.Marker != "ephemeral-only" {
		t.Fatalf("refusing non-sentinel OpenSearch cluster: status=%s sentinel=%+v", response.Status, sentinel)
	}

	target, err := NewOpenSearchAssetProjection([]string{baseURL}, "", "", "assets-v2-write")
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	event := validAssetProjectionEvent()
	projection, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), event, projection); err != nil {
		t.Fatalf("same-version replay must be idempotent: %v", err)
	}

	older := event
	older.EventID = "18c92190-a6c3-5ed9-84a8-4aaec263984f"
	older.AggregateVersion = 1
	older.Revision = 1
	older.Asset.Revision = 1
	older.TraceID = "trace-asset-1"
	olderProjection, err := target.Projection(older)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Apply(context.Background(), older, olderProjection); err == nil ||
		!strings.Contains(err.Error(), "409") {
		t.Fatalf("older external version must be rejected, got %v", err)
	}

	response, err = http.Get(baseURL + "/assets-v2-read/_doc/" + event.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var stored struct {
		Found   bool                `json:"found"`
		Version int                 `json:"_version"`
		Source  assetSearchDocument `json:"_source"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stored); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !stored.Found || stored.Version != 2 ||
		stored.Source.EventID != event.EventID || stored.Source.Revision != 2 ||
		stored.Source.Asset.AssetID != event.AssetID {
		t.Fatalf("unexpected durable OpenSearch projection: status=%s document=%+v", response.Status, stored)
	}
}

func TestNebulaAssetProjectionIsDeterministicAndCarriesWatermark(t *testing.T) {
	writer := &recordingNebulaAssetWriter{}
	target, err := NewNebulaAssetProjection(writer)
	if err != nil {
		t.Fatal(err)
	}
	event := validAssetProjectionEvent()
	first, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := target.Projection(event)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("NebulaGraph projection is not deterministic")
	}
	if err := target.Apply(context.Background(), event, first); err != nil {
		t.Fatal(err)
	}
	if writer.calls != 1 ||
		writer.projection.TenantID != event.TenantID ||
		writer.projection.AssetID != event.AssetID ||
		writer.projection.Revision != event.AggregateVersion {
		t.Fatalf("unexpected graph projection: %+v", writer.projection)
	}
	if writer.projection.Metadata["event_id"] != event.EventID ||
		writer.projection.Metadata["trace_id"] != event.TraceID ||
		writer.projection.Metadata["revision"].(float64) != float64(event.AggregateVersion) {
		t.Fatalf("missing graph reconciliation metadata: %+v", writer.projection.Metadata)
	}
}

func validAssetProjectionEvent() AssetUpsertedV2 {
	tenantID := "tenant-a"
	assetID := uuid.MustParse("dd5804b8-36f4-5cbd-822e-46b7652afd85").String()
	eventID := uuid.MustParse("8ab2d2ea-6b78-5fa0-8cb4-e681fbaaa4f3").String()
	lastSeen := time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC)
	return AssetUpsertedV2{
		EventID:          eventID,
		EventType:        "traffic.asset.v2.AssetUpserted",
		SchemaVersion:    2,
		AggregateVersion: 2,
		PartitionKey:     tenantID + ":" + assetID,
		TenantID:         tenantID,
		AssetID:          assetID,
		Revision:         2,
		TraceID:          "trace-asset-2",
		Asset: config.AssetRecord{
			AssetID:     assetID,
			Revision:    2,
			DisplayCode: "ASSET-000002",
			TenantID:    tenantID,
			AssetType:   "server",
			Status:      "active",
			IPAddress:   "10.0.0.2",
			MACAddress:  "00:11:22:33:44:55",
			Hostname:    "server-2",
			Source:      "api",
			Department:  "security",
			Criticality: 4,
			Metadata:    map[string]any{"zone": "prod"},
			FirstSeen:   lastSeen.Add(-time.Hour),
			LastSeen:    lastSeen,
		},
	}
}

func marshalValidAssetProjectionEvent(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(validAssetProjectionEvent())
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
