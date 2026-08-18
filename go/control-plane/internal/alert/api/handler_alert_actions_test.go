package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

func TestAlertResponseActionRequiresWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/response-actions", bytes.NewBufferString(`{"action":"阻断 IP","target":"AL-1","reason":"confirmed response"}`))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, "tenant-a")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertRead}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertResponseActionPersistsAuditRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_actions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_response_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WithArgs("audit_logs", "user_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("audit_logs", "event_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/response-actions", bytes.NewBufferString(`{"action_id":"alert-response-block-ip","action":"阻断 IP","target":"185.22.14.9","reason":"confirmed response","dry_run":true,"expected_revision":0}`))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, "tenant-a")
	request = withUser(request, "operator-a")
	request.Header.Set("Idempotency-Key", "alert-response-key-000001")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertWrite}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202 body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"action_id":"alert-response-block-ip"`)) {
		t.Fatalf("response is missing stable action_id: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRealAlertResponseActionWaitsForApprovalWithoutOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_actions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WithArgs("audit_logs", "user_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("audit_logs", "event_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/response-actions", bytes.NewBufferString(`{"action_id":"alert-response-block-ip","action":"block_ip","target":"185.22.14.9","reason":"confirmed response","dry_run":false,"expected_revision":0}`))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, "tenant-a")
	request = withUser(request, "operator-a")
	request.Header.Set("Idempotency-Key", "alert-response-key-000002")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertWrite}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202 body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"pending_approval"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"outbox_status":"awaiting_approval"`)) {
		t.Fatalf("real action was not held for approval: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAlertResponseActionReturnsProviderReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT a.action_id,a.action,a.target,a.status").
		WithArgs("tenant-a", "AL-1", "job-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"action_id", "action", "target", "status", "approval_status", "reason", "dry_run", "revision",
			"approved_by", "approved_at", "result", "error", "created_at", "updated_at", "published", "cancelled", "attempts", "last_error",
		}).AddRow("alert-response-block-ip", "block_ip", "185.22.14.9", "completed", "approved", "confirmed response", false, 3,
			"approver-a", now, `{"provider":"edge-fw"}`, "", now, now, true, false, 1, ""))
	mock.ExpectQuery("SELECT event_id::text,state,simulated,external_effect").
		WithArgs("tenant-a", "AL-1", "job-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"event_id", "state", "simulated", "external_effect", "aggregate_version", "result", "error", "kafka_partition", "kafka_offset",
			"provider", "provider_receipt_id", "effect_state", "effect_ids", "trace_id", "receipt_sha256", "authority_lookup", "executed_at",
		}).AddRow("11111111-1111-4111-8111-111111111111", "completed", false, true, 3, `{"provider":"edge-fw"}`, "", 2, 19,
			"edge-fw", "receipt-9001", "confirmed", `["rule-42"]`, "trace-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `{"approval_status":"approved"}`, now))

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/AL-1/response-actions/job-1", nil)
	request = withTenant(request, "tenant-a")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertRead}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1", "job_id": "job-1"})
	recorder := httptest.NewRecorder()

	handler.GetAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if available, _ := envelope.Data["receipt_available"].(bool); !available {
		t.Fatalf("receipt_available=%v body=%s", envelope.Data["receipt_available"], recorder.Body.String())
	}
	receipt, _ := envelope.Data["execution_receipt"].(map[string]interface{})
	if receipt["provider"] != "edge-fw" || receipt["provider_receipt_id"] != "receipt-9001" || receipt["effect_state"] != "confirmed" {
		t.Fatalf("unexpected provider receipt: %#v", receipt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAlertResponseActionMakesMissingReceiptExplicit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT a.action_id,a.action,a.target,a.status").
		WithArgs("tenant-a", "AL-1", "job-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"action_id", "action", "target", "status", "approval_status", "reason", "dry_run", "revision",
			"approved_by", "approved_at", "result", "error", "created_at", "updated_at", "published", "cancelled", "attempts", "last_error",
		}).AddRow("alert-response-block-ip", "block_ip", "185.22.14.9", "pending_approval", "pending", "confirmed response", false, 1,
			"", nil, `{}`, "", now, now, false, false, 0, ""))
	mock.ExpectQuery("SELECT event_id::text,state,simulated,external_effect").
		WithArgs("tenant-a", "AL-1", "job-2").
		WillReturnError(sql.ErrNoRows)

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/AL-1/response-actions/job-2", nil)
	request = withTenant(request, "tenant-a")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertRead}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1", "job_id": "job-2"})
	recorder := httptest.NewRecorder()

	handler.GetAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"receipt_available":false`)) {
		t.Fatalf("missing receipt was not explicit: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
