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
	for range 3 {
		mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	for range 3 {
		mock.ExpectExec("ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_retry").WillReturnResult(sqlmock.NewResult(0, 0))
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
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/AL-1/response-actions", bytes.NewBufferString(`{"action":"阻断 IP","target":"185.22.14.9","reason":"confirmed response","dry_run":true}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertWrite}))
	request = mux.SetURLVars(request, map[string]string{"id": "AL-1"})
	recorder := httptest.NewRecorder()

	handler.CreateAlertResponseAction(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d want 201 body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
