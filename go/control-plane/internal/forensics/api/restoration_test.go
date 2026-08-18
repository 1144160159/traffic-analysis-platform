package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/forensics/restoration"
)

type fakeRestorationProcessor struct {
	calls   int
	request restoration.ProcessRequest
	receipt *restoration.CommitReceipt
	err     error
}

func (processor *fakeRestorationProcessor) Process(_ context.Context, request restoration.ProcessRequest) (*restoration.CommitReceipt, error) {
	processor.calls++
	processor.request = request
	return processor.receipt, processor.err
}

func restorationHTTPBody(extra string) string {
	body := `{
		"idempotency_key":"restore-request-0001",
		"session_id":"session-1",
		"community_id":"1:community",
		"flow_ids":["flow-1"],
		"flow_id":"flow-1",
		"five_tuple":{"source_ip":"192.0.2.1","destination_ip":"198.51.100.2","source_port":51000,"destination_port":80,"protocol":6},
		"direction":"server_to_client",
		"capture_time_start":"2026-08-14T12:00:00Z",
		"capture_time_end":"2026-08-14T12:01:00Z",
		"protocol_profile_id":"http1-response-body-v1",
		"ftp_tls_enabled":false,
		"reason":"approved forensic restoration"`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

func restorationAuthorityContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "analyst-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-a")
	return ctx
}

func TestCreateRestorationReturnsExplicitDisabledResult(t *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forensics/restorations", strings.NewReader(restorationHTTPBody(""))).WithContext(restorationAuthorityContext())
	response := httptest.NewRecorder()
	handler.CreateRestoration(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "RESTORATION_DISABLED") {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestCreateRestorationRejectsUnknownFieldsBeforeProcessor(t *testing.T) {
	processor := &fakeRestorationProcessor{}
	handler := &Handler{restorationProcessor: processor}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forensics/restorations", strings.NewReader(restorationHTTPBody(`"execute_payload":true`))).WithContext(restorationAuthorityContext())
	response := httptest.NewRecorder()
	handler.CreateRestoration(response, request)
	if response.Code != http.StatusBadRequest || processor.calls != 0 {
		t.Fatalf("status/calls/body = %d/%d/%s", response.Code, processor.calls, response.Body.String())
	}
}

func TestCreateRestorationBindsTenantActorAndTraceFromContext(t *testing.T) {
	processor := &fakeRestorationProcessor{receipt: &restoration.CommitReceipt{
		TenantID: "tenant-a", RestorationID: "11111111-1111-4111-8111-111111111111", Revision: 1, Status: "complete",
	}}
	handler := &Handler{restorationProcessor: processor}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/forensics/restorations", strings.NewReader(restorationHTTPBody(""))).WithContext(restorationAuthorityContext())
	response := httptest.NewRecorder()
	handler.CreateRestoration(response, request)
	if response.Code != http.StatusCreated || processor.calls != 1 {
		t.Fatalf("status/calls/body = %d/%d/%s", response.Code, processor.calls, response.Body.String())
	}
	if processor.request.TenantID != "tenant-a" || processor.request.ActorID != "analyst-a" || processor.request.TraceID != "trace-a" {
		t.Fatalf("request authority = %+v", processor.request)
	}
	var envelope struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.Success {
		t.Fatalf("invalid success envelope: %s err=%v", response.Body.String(), err)
	}
}
