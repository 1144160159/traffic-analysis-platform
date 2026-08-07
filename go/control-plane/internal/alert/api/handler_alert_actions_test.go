package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	request.Header.Set("X-Tenant-ID", "tenant-a")
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
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("X-User-ID", "operator-a")
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
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("X-User-ID", "operator-a")
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
