package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/gorilla/mux"
)

const encryptedSnapshotTestSigningKey = "encrypted-snapshot-test-signing-key-32-bytes"

type encryptedSnapshotStubReader struct {
	result encryptedTrafficSnapshotReadResult
	query  encryptedTrafficSnapshotQuery
	calls  int
}

func (r *encryptedSnapshotStubReader) ReadEncryptedTrafficSnapshot(_ context.Context, query encryptedTrafficSnapshotQuery) encryptedTrafficSnapshotReadResult {
	r.calls++
	r.query = query
	return r.result
}

type encryptedSnapshotTestResponse struct {
	Success bool                         `json:"success"`
	Data    EncryptedTrafficSnapshotData `json:"data"`
	Meta    httpx.ContractMeta           `json:"meta"`
	Error   *httpx.ErrorInfo             `json:"error"`
}

func TestEncryptedTrafficSnapshotUsesAuthenticatedTenantAndStableMeta(t *testing.T) {
	reader := &encryptedSnapshotStubReader{result: encryptedTrafficSnapshotReadResult{
		FlowMetadata:         encryptedSnapshotTestSection("available"),
		PlaintextVisible:     encryptedSnapshotTestSection("available"),
		SideChannel:          encryptedSnapshotTestSection("available"),
		RawReference:         encryptedSnapshotTestSection("available"),
		RandomnessStatistics: encryptedSnapshotTestSection("not_computable"),
		SourceWatermarks: map[string]string{
			"clickhouse.sessions": "max_event_time_ms=1;row_count=1;closed_window_sha256=abc",
		},
		MissingSections: []string{"randomness_statistics.sample_bytes", "randomness_statistics.sample_bytes"},
	}}
	handler := newEncryptedTrafficSnapshotHandlerForTest(reader, true, encryptedSnapshotTestSigningKey)
	handler.now = func() time.Time { return time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC) }
	target := "/api/v1/encrypted-traffic/snapshot?start_time=2026-08-15T08:00:00Z&end_time=2026-08-15T09:00:00Z"
	first := performEncryptedSnapshotRequest(t, handler, target, "tenant-auth", []string{"alert:read", "pcap:read"})
	second := performEncryptedSnapshotRequest(t, handler, target, "tenant-auth", []string{"alert:read", "pcap:read"})
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Fatalf("status = %d/%d", first.Code, second.Code)
	}
	var one, two encryptedSnapshotTestResponse
	if err := json.Unmarshal(first.Body.Bytes(), &one); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(second.Body.Bytes(), &two); err != nil {
		t.Fatal(err)
	}
	if reader.query.TenantID != "tenant-auth" || reader.calls != 2 || !reader.query.PcapReadAllowed || reader.query.PcapDownloadAllowed {
		t.Fatalf("captured query/calls = %+v/%d", reader.query, reader.calls)
	}
	if one.Data.TenantID != "tenant-auth" || one.Data.SnapshotID == "" || one.Data.SnapshotID != one.Meta.SnapshotID {
		t.Fatalf("snapshot identity = %+v meta=%+v", one.Data, one.Meta)
	}
	if one.Meta.SnapshotID != two.Meta.SnapshotID || !one.Meta.Partial || len(one.Meta.MissingSections) != 1 {
		t.Fatalf("meta one=%+v two=%+v", one.Meta, two.Meta)
	}
}

func TestEncryptedTrafficSnapshotRejectsTenantOverrideScopeAndMissingRange(t *testing.T) {
	reader := &encryptedSnapshotStubReader{}
	handler := newEncryptedTrafficSnapshotHandlerForTest(reader, true, encryptedSnapshotTestSigningKey)
	base := "/api/v1/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000"
	override := performEncryptedSnapshotRequest(t, handler, base+"&tenant_id=other", "tenant-auth", []string{"alert:read"})
	forbidden := performEncryptedSnapshotRequest(t, handler, base, "tenant-auth", []string{"pcap:read"})
	missingRange := performEncryptedSnapshotRequest(t, handler, "/api/v1/encrypted-traffic/snapshot", "tenant-auth", []string{"alert:read"})
	if override.Code != http.StatusBadRequest || forbidden.Code != http.StatusForbidden || missingRange.Code != http.StatusBadRequest || reader.calls != 0 {
		t.Fatalf("statuses=%d/%d/%d calls=%d", override.Code, forbidden.Code, missingRange.Code, reader.calls)
	}
}

func TestEncryptedTrafficSnapshotPcapPermissionIsFieldScoped(t *testing.T) {
	reader := &encryptedSnapshotStubReader{result: encryptedTrafficSnapshotReadResult{
		FlowMetadata: encryptedSnapshotTestSection("available"), PlaintextVisible: encryptedSnapshotTestSection("available"),
		SideChannel: encryptedSnapshotTestSection("available"), RawReference: encryptedSnapshotForbiddenSection("pcap:read_required"),
		RandomnessStatistics: encryptedSnapshotTestSection("no_sample"),
		MissingSections:      []string{"raw_reference.permission"}, SourceWatermarks: map[string]string{},
	}}
	handler := newEncryptedTrafficSnapshotHandlerForTest(reader, true, encryptedSnapshotTestSigningKey)
	response := performEncryptedSnapshotRequest(t, handler,
		"/api/v1/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000",
		"tenant-auth", []string{"alert:read"})
	var payload encryptedSnapshotTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !payload.Meta.Partial || payload.Data.RawReference.Availability != "forbidden" || reader.query.PcapReadAllowed {
		t.Fatalf("response=%s query=%+v", response.Body.String(), reader.query)
	}
}

func TestEncryptedTrafficSnapshotContinuationIsSignedAndQueryBound(t *testing.T) {
	reader := &encryptedSnapshotStubReader{result: encryptedTrafficSnapshotReadResult{
		FlowMetadata: encryptedSnapshotTestSection("available"), PlaintextVisible: encryptedSnapshotTestSection("available"),
		SideChannel: encryptedSnapshotTestSection("available"), RawReference: encryptedSnapshotTestSection("available"),
		RandomnessStatistics: encryptedSnapshotTestSection("no_sample"), SourceWatermarks: map[string]string{}, NextOffset: 500,
	}}
	handler := newEncryptedTrafficSnapshotHandlerForTest(reader, true, encryptedSnapshotTestSigningKey)
	base := "/api/v1/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000&protocol=TLS"
	first := performEncryptedSnapshotRequest(t, handler, base, "tenant-auth", []string{"alert:read", "pcap:*"})
	var payload encryptedSnapshotTestResponse
	if err := json.Unmarshal(first.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.NextContinuation == "" {
		t.Fatalf("next continuation missing: %s", first.Body.String())
	}
	reader.result.NextOffset = 0
	second := performEncryptedSnapshotRequest(t, handler, base+"&continuation="+payload.Data.NextContinuation,
		"tenant-auth", []string{"alert:read", "pcap:*"})
	if second.Code != http.StatusOK || reader.query.Offset != 500 || !reader.query.PcapDownloadAllowed {
		t.Fatalf("second status/query=%d/%+v body=%s", second.Code, reader.query, second.Body.String())
	}
	tampered := payload.Data.NextContinuation[:len(payload.Data.NextContinuation)-1] + "x"
	bad := performEncryptedSnapshotRequest(t, handler, base+"&continuation="+tampered, "tenant-auth", []string{"alert:read"})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("tampered continuation status=%d body=%s", bad.Code, bad.Body.String())
	}
}

func TestEncryptedTrafficRandomnessDistinguishesNoSampleFromNotComputable(t *testing.T) {
	empty := buildEncryptedRandomnessSection(nil, "empty")
	if empty.Availability != "no_sample" || empty.Partial {
		t.Fatalf("empty randomness section = %+v", empty)
	}
	section := buildEncryptedRandomnessSection([]encryptedSnapshotFingerprintRow{{
		SessionID: "session-a", EntropyPayload: 0, AlgorithmVersion: "entropy-v2",
	}}, "watermark")
	facts := section.Facts.([]EncryptedTrafficRandomnessFact)
	if section.Availability != "not_computable" || !section.Partial || len(facts) != 1 ||
		facts[0].EntropyValue != nil || facts[0].SampleBytes != nil || facts[0].LegacyObservedValue == nil || *facts[0].LegacyObservedValue != 0 {
		t.Fatalf("randomness section = %+v facts=%+v", section, facts)
	}
}

func TestEncryptedTrafficIndicatorsNeverTurnEncryptionIntoMaliciousVerdict(t *testing.T) {
	section := buildEncryptedSideChannelSection([]encryptedSnapshotSessionRow{{
		SessionID: "session-a", DstPort: 853, Protocol: 17, BytesFwd: 20 * 1024 * 1024,
		BytesBwd: 1024, EvidenceIDs: []string{"evidence-a"},
	}}, 1, "watermark")
	facts := section.Facts.([]EncryptedTrafficSideChannelFact)
	if len(facts) != 1 || facts[0].TunnelIndicators[0].Verdict != "indicator_only" || facts[0].ExfiltrationIndicators[0].Verdict != "indicator_only" {
		t.Fatalf("side-channel facts = %+v", facts)
	}
	payload, _ := json.Marshal(facts)
	if strings.Contains(strings.ToLower(string(payload)), `"verdict":"malicious"`) {
		t.Fatalf("encrypted transport became a malicious verdict: %s", payload)
	}
}

func TestEncryptedTrafficSnapshotFeatureFlagAndSigningKeyFailClosed(t *testing.T) {
	reader := &encryptedSnapshotStubReader{}
	disabled := newEncryptedTrafficSnapshotHandlerForTest(reader, false, encryptedSnapshotTestSigningKey)
	target := "/api/v1/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000"
	if response := performEncryptedSnapshotRequest(t, disabled, target, "tenant-auth", []string{"alert:read"}); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled status=%d", response.Code)
	}
	unsigned := newEncryptedTrafficSnapshotHandlerForTest(reader, true, "short")
	if response := performEncryptedSnapshotRequest(t, unsigned, target, "tenant-auth", []string{"alert:read"}); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unsigned status=%d", response.Code)
	}
	if reader.calls != 0 {
		t.Fatalf("reader calls=%d", reader.calls)
	}
}

func TestEncryptedTrafficSnapshotRouteIsRegisteredAndDefaultOff(t *testing.T) {
	router := mux.NewRouter()
	handler := newEncryptedTrafficSnapshotHandlerForTest(&encryptedSnapshotStubReader{}, false, encryptedSnapshotTestSigningKey)
	handler.RegisterRoutes(router)
	request := httptest.NewRequest(http.MethodGet,
		"/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("registered default-off route status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEncryptedTrafficSnapshotEnforcesResponseBudget(t *testing.T) {
	reader := &encryptedSnapshotStubReader{result: encryptedTrafficSnapshotReadResult{
		FlowMetadata: EncryptedTrafficSnapshotSection{
			Availability: "available", Source: "test", SourceWatermark: "test-watermark",
			RuleVersions: []string{}, ModelVersions: []string{}, MissingReasons: []string{},
			Facts: []map[string]string{{"oversized": strings.Repeat("x", encryptedTrafficSnapshotMaxResponseBytes)}},
		},
		PlaintextVisible:     encryptedSnapshotTestSection("no_sample"),
		SideChannel:          encryptedSnapshotTestSection("no_sample"),
		RawReference:         encryptedSnapshotTestSection("no_sample"),
		RandomnessStatistics: encryptedSnapshotTestSection("no_sample"),
		SourceWatermarks:     map[string]string{"test": "bounded"},
	}}
	handler := newEncryptedTrafficSnapshotHandlerForTest(reader, true, encryptedSnapshotTestSigningKey)
	response := performEncryptedSnapshotRequest(t, handler,
		"/api/v1/encrypted-traffic/snapshot?start_time=1786780800000&end_time=1786784400000",
		"tenant-auth", []string{"alert:read"})
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "RESPONSE_BUDGET_EXCEEDED") {
		t.Fatalf("response budget status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEncryptedTrafficSnapshotEvidenceReferencesAreBoundedAndVisible(t *testing.T) {
	references := make([]string, encryptedTrafficSnapshotMaxEvidenceRefs+1)
	for index := range references {
		references[index] = "evidence-" + strconv.Itoa(index)
	}
	result := encryptedTrafficSnapshotReadResult{
		FlowMetadata: EncryptedTrafficSnapshotSection{Facts: []EncryptedTrafficFlowMetadataFact{{EvidenceRefs: references}}},
		RawReference: EncryptedTrafficSnapshotSection{Facts: []EncryptedTrafficRawReferenceFact{{EvidenceRefs: []string{"overflow"}}}},
	}
	if !applyEncryptedTrafficEvidenceBudget(&result, encryptedTrafficSnapshotMaxEvidenceRefs) {
		t.Fatal("expected evidence budget truncation")
	}
	flow := result.FlowMetadata.Facts.([]EncryptedTrafficFlowMetadataFact)
	raw := result.RawReference.Facts.([]EncryptedTrafficRawReferenceFact)
	if len(flow[0].EvidenceRefs) != encryptedTrafficSnapshotMaxEvidenceRefs || len(raw[0].EvidenceRefs) != 0 ||
		!result.FlowMetadata.Partial || !result.RawReference.Partial {
		t.Fatalf("flow=%d raw=%d flowSection=%+v rawSection=%+v", len(flow[0].EvidenceRefs), len(raw[0].EvidenceRefs), result.FlowMetadata, result.RawReference)
	}
}

func TestEncryptedTrafficFlowMetadataBindsPersistedTLSVersionAndReferences(t *testing.T) {
	section := buildEncryptedFlowMetadataSection([]encryptedSnapshotSessionRow{{
		SessionID: "session-a", EventID: "event-a", EvidenceIDs: []string{"evidence-a"},
		Protocol: 6, DstPort: 443,
	}}, 1, "sessions-watermark")
	applyEncryptedHandshakeMetadata(&section, []encryptedSnapshotFingerprintRow{{SessionID: "session-a", TLSVersion: "TLSv1.3"}})
	facts := section.Facts.([]EncryptedTrafficFlowMetadataFact)
	if len(facts) != 1 || facts[0].TLSVersion == nil || *facts[0].TLSVersion != "TLSv1.3" ||
		len(facts[0].SourceEventIDs) != 1 || facts[0].SourceEventIDs[0] != "event-a" || len(facts[0].EvidenceRefs) != 1 {
		t.Fatalf("flow metadata facts=%+v", facts)
	}
}

func TestEncryptedTrafficSnapshotLegacyColumnsRemainExplicitPartial(t *testing.T) {
	legacy := map[string]bool{"session_id": true, "ts_end": true}
	if expression := encryptedSnapshotSessionPartialExpression(legacy); expression != "toUInt8(1)" {
		t.Fatalf("legacy partial expression=%s", expression)
	}
	missing := encryptedSnapshotMissingFieldsExpression(legacy, []string{"source_event_ids", "evidence_ids"})
	for _, field := range []string{"source_event_ids_not_persisted", "evidence_ids_not_persisted", "missing_fields_not_persisted"} {
		if !strings.Contains(missing, field) {
			t.Fatalf("legacy missing expression lacks %s: %s", field, missing)
		}
	}
	if watermark := encryptedSnapshotWatermarkExpression(legacy); watermark != "ts_end" {
		t.Fatalf("legacy watermark expression=%s", watermark)
	}
	modern := map[string]bool{
		"source_event_ids": true, "evidence_ids": true, "is_partial": true,
		"missing_fields": true, "source_watermark_ms": true, "event_time_end_ms": true,
	}
	if expression := encryptedSnapshotSessionPartialExpression(modern); expression != "is_partial" {
		t.Fatalf("modern partial expression=%s", expression)
	}
}

func encryptedSnapshotTestSection(availability string) EncryptedTrafficSnapshotSection {
	return EncryptedTrafficSnapshotSection{
		Availability: availability, Source: "test", SourceWatermark: "test-watermark",
		RuleVersions: []string{}, ModelVersions: []string{}, MissingReasons: []string{}, Facts: []interface{}{},
	}
}

func performEncryptedSnapshotRequest(t *testing.T, handler *EncryptedTrafficSnapshotHandler, target, tenant string, permissions []string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, tenant)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "encrypted-snapshot-reader")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-encrypted-snapshot")
	response := httptest.NewRecorder()
	handler.GetSnapshot(response, request.WithContext(ctx))
	return response
}
