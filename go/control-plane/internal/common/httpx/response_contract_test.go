package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONContractSuccessFillsStableSnapshotMetadata(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyTraceID, "trace-contract-1")
	ctx = context.WithValue(ctx, ContextKeyTenantID, "tenant-a")
	recorder := httptest.NewRecorder()

	JSONContractSuccess(recorder, ctx, map[string]interface{}{"count": 0}, ContractMeta{
		SnapshotID:       "snapshot-1",
		AsOf:             "2026-08-04T00:00:00Z",
		Partial:          false,
		MissingSections:  nil,
		SourceWatermarks: nil,
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusOK)
	}
	var response ContractResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Meta.ContractVersion != 1 || response.Meta.SchemaVersion != 1 {
		t.Fatalf("versions=%+v", response.Meta)
	}
	if response.Meta.SnapshotID != "snapshot-1" || response.Meta.AsOf != "2026-08-04T00:00:00Z" {
		t.Fatalf("snapshot metadata=%+v", response.Meta)
	}
	if response.Meta.GeneratedAt == "" || response.Meta.TraceID != "trace-contract-1" {
		t.Fatalf("generated/trace metadata=%+v", response.Meta)
	}
	if response.Meta.ResultCode != "SUCCESS" || response.Meta.TenantID != "tenant-a" {
		t.Fatalf("result/tenant metadata=%+v", response.Meta)
	}
	if response.Meta.MissingSections == nil || response.Meta.SourceWatermarks == nil {
		t.Fatalf("empty collections must be explicit: %+v", response.Meta)
	}
}

func TestJSONContractErrorKeepsHTTPAndBusinessSemanticsDistinct(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyTraceID, "trace-conflict-1")
	recorder := httptest.NewRecorder()
	revision := int64(7)

	JSONContractError(recorder, ctx, http.StatusConflict, "REVISION_CONFLICT", "resource revision changed", ErrorOptions{
		OperationID:      "updateAsset",
		CurrentRevision:  &revision,
		ProjectionStatus: "current",
		FieldErrors: []FieldError{{
			Field: "expected_revision", Code: "STALE_REVISION", Message: "expected revision is stale",
		}},
	})

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d", recorder.Code, http.StatusConflict)
	}
	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success || response.Error == nil {
		t.Fatalf("expected structured failure: %+v", response)
	}
	if response.Error.Code != "REVISION_CONFLICT" || response.Error.TraceID != "trace-conflict-1" {
		t.Fatalf("error identity=%+v", response.Error)
	}
	if response.Error.Retryable || response.Error.CurrentRevision == nil || *response.Error.CurrentRevision != 7 {
		t.Fatalf("conflict semantics=%+v", response.Error)
	}
	if response.Error.OperationID != "updateAsset" || len(response.Error.FieldErrors) != 1 {
		t.Fatalf("operation/fields=%+v", response.Error)
	}
}

func TestLegacyJSONErrorAddsTraceAndRetryabilityWithoutReturningSuccess(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyTraceID, "trace-dependency-1")
	recorder := httptest.NewRecorder()

	JSONError(recorder, ctx, http.StatusServiceUnavailable, "AUTHORITY_UNAVAILABLE", "authoritative store unavailable")

	var response Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if recorder.Code != http.StatusServiceUnavailable || response.Success || response.Error == nil {
		t.Fatalf("transport/business mismatch: status=%d response=%+v", recorder.Code, response)
	}
	if response.Error.TraceID != "trace-dependency-1" || !response.Error.Retryable {
		t.Fatalf("trace/retryable=%+v", response.Error)
	}
}
