package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func savedViewRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/views", bytes.NewBufferString(
		`{"action_id":"alert-view-save","action":"save_view","target":"critical-alerts","reason":"operator workspace","detail":{"filters":{"severity":"critical"}}}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Tenant-ID", "tenant-a")
	request.Header.Set("X-User-ID", "operator-a")
	request.Header.Set("Idempotency-Key", "alert-saved-view-key-0001")
	return request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, []string{model.ScopeAlertWrite}))
}

func expectSavedViewMutation(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:alert-saved-view-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM alert_saved_view_requests r").
		WithArgs("tenant-a", "alert-saved-view-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{
			"payload_sha256", "view_id", "resulting_revision", "event_id", "name", "filters", "created_at", "updated_at", "status",
		}))
	mock.ExpectQuery("INSERT INTO alert_saved_views").
		WithArgs("tenant-a", "critical-alerts", `{"severity":"critical"}`, "operator-a", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"view_id", "name", "revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-0000-0000-000000000101", "critical-alerts", int64(1), now, now))
	mock.ExpectExec("INSERT INTO alert_saved_view_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "00000000-0000-0000-0000-000000000101", int64(1), "critical-alerts", `{"severity":"critical"}`, "operator-a", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO alert_saved_view_outbox").
		WithArgs(sqlmock.AnyArg(), "00000000-0000-0000-0000-000000000101", int64(1), "tenant-a", "tenant-a:00000000-0000-0000-0000-000000000101", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WithArgs("audit_logs", "user_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("audit_logs", "event_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr == nil {
		audit.WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec("INSERT INTO alert_saved_view_requests").
			WithArgs("tenant-a", "alert-saved-view-key-0001", sqlmock.AnyArg(), "00000000-0000-0000-0000-000000000101", int64(1), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
	} else {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
	}
}

func TestSaveAlertViewCommitsBusinessHistoryOutboxAuditAndRequestTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectSavedViewMutation(mock, nil)
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	recorder := httptest.NewRecorder()
	handler.SaveAlertView(recorder, savedViewRequest())
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, token := range []string{`"revision":1`, `"outbox_status":"pending"`, `"idempotent_reuse":false`} {
		if !bytes.Contains(recorder.Body.Bytes(), []byte(token)) {
			t.Fatalf("response missing %s: %s", token, recorder.Body.String())
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAlertViewAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectSavedViewMutation(mock, errors.New("audit unavailable"))
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	recorder := httptest.NewRecorder()
	handler.SaveAlertView(recorder, savedViewRequest())
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAlertViewRequiresIdempotencyKeyBeforeDatabaseMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := savedViewRequest()
	request.Header.Del("Idempotency-Key")
	recorder := httptest.NewRecorder()
	handler.SaveAlertView(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAlertViewIdempotentReplayReturnsExistingCommittedResult(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:alert-saved-view-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	payloadHash := opaqueKeyDigest(strings.Join([]string{
		"tenant-a", "operator-a", "alert-view-save", "critical-alerts", "operator workspace", `{"severity":"critical"}`,
	}, "\x00"))
	mock.ExpectQuery("FROM alert_saved_view_requests r").
		WithArgs("tenant-a", "alert-saved-view-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{
			"payload_sha256", "view_id", "resulting_revision", "event_id", "name", "filters", "created_at", "updated_at", "status",
		}).AddRow(payloadHash, "00000000-0000-0000-0000-000000000101", int64(2), "00000000-0000-0000-0000-000000000202", "critical-alerts", `{"severity":"critical"}`, now, now, "published"))
	mock.ExpectCommit()
	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	recorder := httptest.NewRecorder()
	handler.SaveAlertView(recorder, savedViewRequest())
	if recorder.Code != http.StatusCreated || !bytes.Contains(recorder.Body.Bytes(), []byte(`"idempotent_reuse":true`)) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"outbox_status":"published"`)) {
		t.Fatalf("replay lost final outbox status: %s", recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
