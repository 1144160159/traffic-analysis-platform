package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/gorilla/mux"
	"google.golang.org/protobuf/proto"
)

type fakeAuditBatchPublisher struct {
	err     error
	key     string
	batch   *pb.AuditLogBatch
	headers []commonkafka.MessageHeader
	calls   int
}

func (f *fakeAuditBatchPublisher) SendProto(_ context.Context, key string, message proto.Message, headers ...commonkafka.MessageHeader) error {
	f.calls++
	f.key = key
	f.headers = append([]commonkafka.MessageHeader(nil), headers...)
	if batch, ok := message.(*pb.AuditLogBatch); ok {
		f.batch = proto.Clone(batch).(*pb.AuditLogBatch)
	}
	return f.err
}

func TestAuditBatchInternalRouteIsRegistered(t *testing.T) {
	router := mux.NewRouter()
	internal := router.PathPrefix("/internal/v1").Subrouter()
	NewSystemHandler(nil, nil, nil).RegisterInternalRoutes(internal)
	match := &mux.RouteMatch{}
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/audit/batches", nil)
	if !router.Match(request, match) || match.MatchErr != nil {
		t.Fatalf("internal audit batch route is not registered: %v", match.MatchErr)
	}
}

func TestAuditBatchFailsClosedForPermissionTenantAndDisabledPublisher(t *testing.T) {
	validBody := `{"action_id":"audit-batch-ingest","reason":"service audit flush","events":[{"event_id":"audit-1","action":"EXPORT","created_at":1786000000000}]}`
	handler := NewSystemHandler(nil, nil, nil)

	missingPermission := auditBatchRequestForTest(validBody, "tenant-a", []string{authmodel.ScopeAuditRead})
	response := httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, missingPermission)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), auditBatchOperationID) {
		t.Fatalf("missing permission response=%d %s", response.Code, response.Body.String())
	}

	missingTenant := auditBatchRequestForTest(validBody, "", []string{authmodel.ScopeAuditWrite})
	response = httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, missingTenant)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing tenant response=%d %s", response.Code, response.Body.String())
	}

	disabled := auditBatchRequestForTest(validBody, "tenant-a", []string{authmodel.ScopeAuditWrite})
	response = httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, disabled)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "AUDIT_BATCH_INGRESS_DISABLED") {
		t.Fatalf("disabled publisher response=%d %s", response.Code, response.Body.String())
	}
}

func TestAuditBatchRejectsTenantOverrideAndAnyInvalidEventBeforePublish(t *testing.T) {
	publisher := &fakeAuditBatchPublisher{}
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAuditBatchProducer(publisher)
	body := `{"action_id":"audit-batch-ingest","reason":"service audit flush","events":[` +
		`{"event_id":"audit-1","tenant_id":"tenant-other","action":"EXPORT","created_at":1786000000000},` +
		`{"event_id":"","action":""}]}`
	request := auditBatchRequestForTest(body, "tenant-a", []string{authmodel.ScopeAuditWrite})
	response := httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, request)
	if response.Code != http.StatusBadRequest || publisher.calls != 0 {
		t.Fatalf("invalid batch response=%d calls=%d body=%s", response.Code, publisher.calls, response.Body.String())
	}
	for _, code := range []string{"TENANT_OVERRIDE_FORBIDDEN", "INVALID_EVENT_ID", "INVALID_ACTION", "INVALID_CREATED_AT"} {
		if !strings.Contains(response.Body.String(), code) {
			t.Fatalf("invalid batch response missing %s: %s", code, response.Body.String())
		}
	}
}

func TestAuditBatchKafkaAcknowledgementReturnsNonFinalContractEnvelope(t *testing.T) {
	publisher := &fakeAuditBatchPublisher{}
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAuditBatchProducer(publisher)
	body := `{"action_id":"audit-batch-ingest","reason":"service audit flush","events":[` +
		`{"event_id":"audit-2","tenant_id":"tenant-a","action":"SECOND","detail":{"zero":0},"created_at":1786000000000},` +
		`{"event_id":"audit-1","action":"FIRST","created_at":1786000000000}]}`
	request := auditBatchRequestForTest(body, "tenant-a", []string{authmodel.ScopeAuditWrite})
	response := httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("accepted response=%d %s", response.Code, response.Body.String())
	}
	if publisher.calls != 1 || !strings.HasPrefix(publisher.key, "audit-batch-") || publisher.batch == nil || len(publisher.batch.Events) != 2 {
		t.Fatalf("publisher calls=%d key=%q batch=%v", publisher.calls, publisher.key, publisher.batch)
	}
	if publisher.batch.Events[0].EventId != "audit-1" || publisher.batch.Events[1].EventId != "audit-2" {
		t.Fatalf("batch must be deterministically sorted: %+v", publisher.batch.Events)
	}
	if publisher.batch.Events[0].TenantId != "tenant-a" || publisher.batch.Events[1].TenantId != "tenant-a" {
		t.Fatalf("authenticated tenant was not authoritative: %+v", publisher.batch.Events)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]interface{})
	meta := envelope["meta"].(map[string]interface{})
	if data["final"] != false || data["status"] != "accepted" || data["job_id"] == "" {
		t.Fatalf("accepted response must remain non-final: %v", data)
	}
	if meta["operation_id"] != auditBatchOperationID || meta["projection_status"] != "pending" || meta["partial"] != false {
		t.Fatalf("contract metadata mismatch: %v", meta)
	}

	request = auditBatchRequestForTest(body, "tenant-a", []string{authmodel.ScopeAuditWrite})
	replay := httptest.NewRecorder()
	handler.IngestAuditLogBatch(replay, request)
	var replayEnvelope map[string]interface{}
	_ = json.Unmarshal(replay.Body.Bytes(), &replayEnvelope)
	if replayEnvelope["data"].(map[string]interface{})["job_id"] != data["job_id"] {
		t.Fatalf("exact replay job identity drifted: first=%v replay=%v", data, replayEnvelope["data"])
	}
}

func TestAuditBatchDoesNotReturn202WithoutKafkaAcknowledgement(t *testing.T) {
	publisher := &fakeAuditBatchPublisher{err: errors.New("broker unavailable")}
	handler := NewSystemHandler(nil, nil, nil)
	handler.SetAuditBatchProducer(publisher)
	body := `{"action_id":"audit-batch-ingest","reason":"service audit flush","events":[{"event_id":"audit-1","action":"EXPORT","created_at":1786000000000}]}`
	request := auditBatchRequestForTest(body, "tenant-a", []string{authmodel.ScopeAuditWrite})
	response := httptest.NewRecorder()
	handler.IngestAuditLogBatch(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "AUDIT_BATCH_NOT_ACCEPTED") {
		t.Fatalf("publisher failure response=%d %s", response.Code, response.Body.String())
	}
}

func auditBatchRequestForTest(body, tenantID string, permissions []string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/audit/batches", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	ctx := request.Context()
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, "service:test")
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, []string{"service"})
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, permissions)
	ctx = context.WithValue(ctx, httpx.ContextKeyRequestID, "request-audit-batch")
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-audit-batch")
	return request.WithContext(ctx)
}
