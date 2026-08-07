package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type probeAckClaims struct {
	probeID string
}

func (c probeAckClaims) GetUserID() string        { return "00000000-0000-0000-0000-000000000001" }
func (c probeAckClaims) GetTenantID() string      { return "tenant-a" }
func (c probeAckClaims) GetUsername() string      { return "probe-agent" }
func (c probeAckClaims) GetRoles() []string       { return []string{"probe"} }
func (c probeAckClaims) GetPermissions() []string { return []string{authmodel.ScopeProbeIngest} }
func (c probeAckClaims) GetProbeID() string       { return c.probeID }

func probeOperationRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"operation_id", "tenant_id", "probe_id", "operation_type", "status",
		"command_revision", "state_revision", "desired_version", "command_hash",
		"reported_version", "reported_hash", "agent_version", "ack_error", "requested_by",
		"request", "result", "trace_id", "expires_at", "acknowledged_at",
		"created_at", "updated_at", "outbox_published",
		"control_event_id", "control_published_at", "lifecycle_event_id", "lifecycle_published_at",
		"agent_ack_event_id",
	})
}

func probeAckRequest(body, probeID, operationID string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/probes/"+probeID+"/operations/"+operationID+"/ack", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	claims := probeAckClaims{probeID: probeID}
	ctx := context.WithValue(request.Context(), httpx.ContextKeyClaims, claims)
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, claims.GetTenantID())
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, claims.GetUserID())
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, claims.GetPermissions())
	ctx = context.WithValue(ctx, httpx.ContextKeyTraceID, "trace-probe-ack-test")
	request = request.WithContext(ctx)
	return mux.SetURLVars(request, map[string]string{"id": probeID, "operation_id": operationID})
}

func TestProbeAckRequiresTokenBoundProbeIdentity(t *testing.T) {
	handler := NewSystemHandler(nil, nil, zap.NewNop())
	operationID := "11111111-1111-1111-1111-111111111111"
	request := probeAckRequest(`{"command_revision":1,"reported_hash":"sha256:test","agent_version":"1.0","applied":true}`, "probe-a", operationID)
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyClaims, probeAckClaims{probeID: "probe-b"}))
	recorder := httptest.NewRecorder()

	handler.AcknowledgeProbeOperation(recorder, request)

	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "PROBE_IDENTITY_MISMATCH") {
		t.Fatalf("expected bound identity rejection, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestProbeAckRejectsTrailingJSONValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewSystemHandler(nil, db, zap.NewNop())
	operationID := "11111111-1111-4111-8111-111111111111"
	request := probeAckRequest(
		`{"command_revision":1,"reported_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","agent_version":"1.0","applied":true}{"unexpected":true}`,
		"probe-a",
		operationID,
	)
	recorder := httptest.NewRecorder()

	handler.AcknowledgeProbeOperation(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "exactly one JSON object") {
		t.Fatalf("expected trailing JSON rejection, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetProbeOperationReportsAcceptedAsPartialUntilAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	operationID := "22222222-2222-2222-2222-222222222222"
	mock.ExpectQuery(regexp.QuoteMeta("FROM probe_operations o")).
		WithArgs("tenant-a", operationID).
		WillReturnRows(probeOperationRows().AddRow(
			operationID, "tenant-a", "probe-a", "config_push", "accepted",
			int64(7), int64(1), "cfg-7", "command-sha", "", "", "", "", "operator-a",
			[]byte(`{"config_version":"cfg-7"}`), []byte(`{"status":"accepted"}`), "trace-probe",
			now.Add(10*time.Minute), nil, now, now, false,
			"77777777-7777-4777-8777-777777777777", nil, "", nil, "",
		))
	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := httptest.NewRequest(http.MethodGet, "/api/v1/probes/operations/"+operationID, nil)
	ctx := context.WithValue(request.Context(), httpx.ContextKeyTenantID, "tenant-a")
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, []string{authmodel.ScopeProbeRead})
	request = mux.SetURLVars(request.WithContext(ctx), map[string]string{"operation_id": operationID})
	recorder := httptest.NewRecorder()

	handler.GetProbeOperation(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data map[string]interface{} `json:"data"`
		Meta httpx.ContractMeta     `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data["status"] != "accepted" || !envelope.Meta.Partial || len(envelope.Meta.MissingSections) != 1 || envelope.Meta.MissingSections[0] != "agent_ack" {
		t.Fatalf("accepted operation must remain visibly incomplete: data=%v meta=%+v", envelope.Data, envelope.Meta)
	}
	if got := envelope.Meta.SourceWatermarks["kafka.probe.control.event_id"]; got != "77777777-7777-4777-8777-777777777777@outbox_pending" {
		t.Fatalf("control watermark=%q, want pending stable event id", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOutOfOrderProbeAckIsRetainedWithoutRegressingProbeState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	operationID := "33333333-3333-3333-3333-333333333333"
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM probe_operations o")).
		WithArgs("tenant-a", "probe-a", operationID).
		WillReturnRows(probeOperationRows().AddRow(
			operationID, "tenant-a", "probe-a", "config_push", "accepted",
			int64(3), int64(1), "cfg-3", "command-sha", "", "", "", "", "operator-a",
			[]byte(`{"config_version":"cfg-3"}`), []byte(`{"status":"accepted"}`), "trace-probe",
			now.Add(10*time.Minute), nil, now, now, false,
			"88888888-8888-4888-8888-888888888888", nil, "", nil, "",
		))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(operationID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT COALESCE\\(max\\(command_revision\\),0\\)").
		WithArgs("tenant-a", "probe-a").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(4)))
	mock.ExpectExec("INSERT INTO probe_operation_ack_receipts").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("UPDATE probe_operations").
		WillReturnRows(sqlmock.NewRows([]string{"state_revision"}).AddRow(int64(2)))
	mock.ExpectExec("INSERT INTO probe_operation_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO probe_operation_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta("FROM probe_operations o")).
		WithArgs("tenant-a", operationID).
		WillReturnRows(probeOperationRows().AddRow(
			operationID, "tenant-a", "probe-a", "config_push", "stale",
			int64(3), int64(2), "cfg-3", "command-sha", "cfg-3", "sha256:cfg-3", "1.2.0",
			"newer command revision is already applied", "operator-a",
			[]byte(`{"config_version":"cfg-3"}`), []byte(`{"status":"stale"}`), "trace-probe",
			now.Add(10*time.Minute), now, now, now, false,
			"88888888-8888-4888-8888-888888888888", now,
			"99999999-9999-4999-8999-999999999999", nil, "",
		))

	handler := NewSystemHandler(nil, db, zap.NewNop())
	request := probeAckRequest(`{"command_revision":3,"reported_version":"cfg-3","reported_hash":"sha256:cfg-3","agent_version":"1.2.0","applied":true}`, "probe-a", operationID)
	recorder := httptest.NewRecorder()
	handler.AcknowledgeProbeOperation(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"stale"`) {
		t.Fatalf("expected retained stale ACK, got %d %s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestKafkaProbeAckUsesSourceEventIDAndCommitsAllEffectsTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	operationID := "44444444-4444-4444-8444-444444444444"
	sourceEventID := "55555555-5555-4555-8555-555555555555"

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM probe_operations o")).
		WithArgs("tenant-a", "probe-a", operationID).
		WillReturnRows(probeOperationRows().AddRow(
			operationID, "tenant-a", "probe-a", "connectivity_test", "delivered",
			int64(9), int64(2), "cfg-9", "command-sha", "", "", "", "", "operator-a",
			[]byte(`{"targets":["ingest-gateway"]}`), []byte(`{"status":"delivered"}`), "trace-kafka-ack",
			now.Add(10*time.Minute), nil, now, now, true,
			"66666666-6666-4666-8666-666666666666", now, "", nil, "",
		))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(operationID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery("SELECT COALESCE\\(max\\(command_revision\\),0\\)").
		WithArgs("tenant-a", "probe-a").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO probe_operation_ack_receipts").
		WithArgs(
			sourceEventID, operationID, "tenant-a", "probe-a", int64(9), "cfg-9",
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"0.1.0", true, "", sqlmock.AnyArg(), true, "", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("UPDATE probe_operations").
		WillReturnRows(sqlmock.NewRows([]string{"state_revision"}).AddRow(int64(3)))
	mock.ExpectExec("INSERT INTO probe_operation_history").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE probes SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO probe_operation_outbox").
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectAuditInsert(mock)
	mock.ExpectCommit()

	handler := NewSystemHandler(nil, db, zap.NewNop())
	err = handler.ApplyProbeOperationAck(
		context.Background(), "tenant-a", "probe-a", operationID, sourceEventID,
		ProbeOperationAckInput{
			CommandRevision: 9,
			ReportedVersion: "cfg-9",
			ReportedHash:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			AgentVersion:    "0.1.0",
			Applied:         true,
			AcknowledgedAt:  now,
			Detail:          map[string]interface{}{"executor": "builtin"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
