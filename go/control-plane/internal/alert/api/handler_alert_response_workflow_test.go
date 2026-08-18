package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const (
	responseJobID   = "alert-action-22222222-2222-4222-8222-222222222222"
	responseEventID = "11111111-1111-4111-8111-111111111111"
)

func TestCreateAlertResponseActionExactIdempotentReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT job_id,event_id::text").
		WithArgs("tenant-a", "alert-response-key-replay").
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "event_id", "alert_id", "action_id", "action", "target",
			"reason", "dry_run", "expected_revision", "revision", "status", "approval_status", "requested_by",
		}).AddRow(
			responseJobID, responseEventID, "AL-1", "alert-response-block-ip", "block_ip",
			"185.22.14.9", "confirmed response", false, int64(0), int64(1),
			"pending_approval", "pending", "operator-a",
		))
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions",
		`{"action_id":"alert-response-block-ip","action":"block_ip","target":"185.22.14.9","reason":"confirmed response","dry_run":false,"expected_revision":0}`,
		[]string{authmodel.ScopeAlertWrite},
		"operator-a",
		"alert-response-key-replay",
	)
	recorder := httptest.NewRecorder()
	handler.CreateAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusAccepted ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"idempotent_reuse":true`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(responseJobID)) {
		t.Fatalf("unexpected idempotent replay response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateAlertResponseActionRejectsIdempotencyConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT job_id,event_id::text").
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "event_id", "alert_id", "action_id", "action", "target",
			"reason", "dry_run", "expected_revision", "revision", "status", "approval_status", "requested_by",
		}).AddRow(
			responseJobID, responseEventID, "AL-OTHER", "alert-response-block-ip", "block_ip",
			"185.22.14.9", "confirmed response", false, int64(0), int64(1),
			"pending_approval", "pending", "operator-a",
		))
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions",
		`{"action_id":"alert-response-block-ip","action":"block_ip","target":"185.22.14.9","reason":"confirmed response","dry_run":false,"expected_revision":0}`,
		[]string{authmodel.ScopeAlertWrite},
		"operator-a",
		"alert-response-key-conflict",
	)
	recorder := httptest.NewRecorder()
	handler.CreateAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("IDEMPOTENCY_KEY_CONFLICT")) {
		t.Fatalf("unexpected conflict response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertResponseApprovalRequiresPlaybookApprove(t *testing.T) {
	handler := NewHandler(nil, nil, zap.NewNop())
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"independent approval"}`,
		[]string{authmodel.ScopeAlertWrite},
		"approver-b",
		"alert-approval-key-denied",
	)
	recorder := httptest.NewRecorder()
	handler.DecideAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=403 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAlertResponseApprovalRejectsSelfApproval(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoApproval(mock)
	expectLockedResponseAction(mock, "operator-a", "pending_approval", "pending", false, 1)
	expectNoApproval(mock)
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"independent approval"}`,
		[]string{authmodel.ScopePlaybookApprove},
		"operator-a",
		"alert-approval-key-self",
	)
	recorder := httptest.NewRecorder()
	handler.DecideAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusForbidden ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("INDEPENDENT_APPROVER_REQUIRED")) {
		t.Fatalf("unexpected self-approval response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentAlertResponseApprovalCommitsOutboxAndAudit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoApproval(mock)
	expectLockedResponseAction(mock, "operator-a", "pending_approval", "pending", false, 1)
	expectNoApproval(mock)
	mock.ExpectExec("INSERT INTO alert_response_approvals").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO alert_response_outbox").
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectResponseAudit(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"independent approval"}`,
		[]string{authmodel.ScopePlaybookApprove},
		"approver-b",
		"alert-approval-key-approve",
	)
	recorder := httptest.NewRecorder()
	handler.DecideAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusAccepted ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"approved_awaiting_executor"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"revision":2`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"outbox_status":"pending_retry"`)) {
		t.Fatalf("unexpected approval response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAlertResponseApprovalRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoApproval(mock)
	expectLockedResponseAction(mock, "operator-a", "pending_approval", "pending", false, 2)
	expectNoApproval(mock)
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/approval",
		`{"decision":"approve","expected_revision":1,"reason":"independent approval"}`,
		[]string{authmodel.ScopePlaybookApprove},
		"approver-b",
		"alert-approval-key-stale",
	)
	recorder := httptest.NewRecorder()
	handler.DecideAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("REVISION_CONFLICT")) {
		t.Fatalf("unexpected stale revision response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIndependentAlertResponseRejectionCancelsWithoutOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoApproval(mock)
	expectLockedResponseAction(mock, "operator-a", "pending_approval", "pending", false, 1)
	expectNoApproval(mock)
	mock.ExpectExec("INSERT INTO alert_response_approvals").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectResponseAudit(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/approval",
		`{"decision":"reject","expected_revision":1,"reason":"insufficient evidence"}`,
		[]string{authmodel.ScopePlaybookApprove},
		"approver-b",
		"alert-approval-key-reject",
	)
	recorder := httptest.NewRecorder()
	handler.DecideAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusAccepted ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"cancelled"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"approval_status":"rejected"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"outbox_status":"not_required"`)) {
		t.Fatalf("unexpected rejection response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelPendingAlertResponseActionIsAtomic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoControlRequest(mock)
	expectLockedResponseAction(mock, "operator-a", "pending_approval", "pending", false, 1)
	expectNoControlRequest(mock)
	mock.ExpectExec("INSERT INTO alert_response_control_requests").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectResponseAudit(mock)
	mock.ExpectCommit()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/cancel",
		`{"expected_revision":1,"reason":"operator cancelled request"}`,
		[]string{authmodel.ScopeAlertWrite},
		"operator-a",
		"alert-cancel-key-pending",
	)
	recorder := httptest.NewRecorder()
	handler.CancelAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusAccepted ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"status":"cancelled"`)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"revision":2`)) {
		t.Fatalf("unexpected cancellation response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCancelAlertResponseActionCannotOvertakePublishedEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoControlRequest(mock)
	expectLockedResponseAction(mock, "operator-a", "approved_awaiting_executor", "approved", false, 2)
	expectNoControlRequest(mock)
	mock.ExpectExec("UPDATE alert_response_outbox").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/cancel",
		`{"expected_revision":2,"reason":"cancel before delivery"}`,
		[]string{authmodel.ScopeAlertWrite},
		"operator-a",
		"alert-cancel-key-too-late",
	)
	recorder := httptest.NewRecorder()
	handler.CancelAlertResponseAction(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("TOO_LATE_TO_CANCEL")) {
		t.Fatalf("unexpected late cancellation response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompensationRejectsReceiptWithoutExternalEffect(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectNoControlRequest(mock)
	expectLockedResponseAction(mock, "operator-a", "completed", "approved", false, 3)
	expectNoControlRequest(mock)
	mock.ExpectQuery("SELECT state,external_effect,provider,provider_receipt_id,effect_ids::text,trace_id").
		WillReturnRows(sqlmock.NewRows([]string{"state", "external_effect", "provider", "provider_receipt_id", "effect_ids", "trace_id"}).
			AddRow("completed", false, "ephemeral-firewall", "receipt-1", `["rule-1"]`, "trace-1"))
	mock.ExpectRollback()

	handler := NewHandler(nil, nil, zap.NewNop())
	handler.SetActionAuditWriter(NewAlertActionAuditWriter(db, zap.NewNop()))
	request := responseWorkflowRequest(
		http.MethodPost,
		"/api/v1/alerts/AL-1/response-actions/"+responseJobID+"/compensations",
		`{"expected_revision":3,"reason":"restore prior network access"}`,
		[]string{authmodel.ScopePlaybookApprove},
		"approver-b",
		"alert-compensate-key-no-effect",
	)
	recorder := httptest.NewRecorder()
	handler.RequestAlertResponseCompensation(recorder, request)
	if recorder.Code != http.StatusConflict ||
		!bytes.Contains(recorder.Body.Bytes(), []byte("NO_EXTERNAL_EFFECT")) {
		t.Fatalf("unexpected compensation response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func responseWorkflowRequest(
	method, path, body string,
	permissions []string,
	userID, idempotencyKey string,
) *http.Request {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request = withTenant(request, "tenant-a")
	request = withUser(request, userID)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	request = request.WithContext(context.WithValue(request.Context(), httpx.ContextKeyPermissions, permissions))
	request = mux.SetURLVars(request, map[string]string{
		"id": "AL-1", "job_id": responseJobID,
	})
	return request
}

func expectNoApproval(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT job_id,alert_id,decision").
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "alert_id", "decision", "expected_revision", "reason",
			"decided_by", "resulting_revision", "resulting_status", "approval_status",
		}))
}

func expectNoControlRequest(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT job_id,alert_id,operation").
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "alert_id", "operation", "expected_revision", "reason",
			"requested_by", "state", "resulting_revision", "resulting_status",
		}))
}

func expectLockedResponseAction(
	mock sqlmock.Sqlmock,
	requestedBy, status, approvalStatus string,
	dryRun bool,
	revision int64,
) {
	mock.ExpectQuery("SELECT job_id,event_id::text,tenant_id,alert_id").
		WithArgs("tenant-a", "AL-1", responseJobID).
		WillReturnRows(sqlmock.NewRows([]string{
			"job_id", "event_id", "tenant_id", "alert_id", "action_id", "action",
			"target", "reason", "requested_by", "trace_id", "status", "approval_status", "dry_run", "revision",
		}).AddRow(
			responseJobID, responseEventID, "tenant-a", "AL-1",
			"alert-response-block-ip", "block_ip", "185.22.14.9", "confirmed response",
			requestedBy, "trace-alert-response", status, approvalStatus, dryRun, revision,
		))
}

func expectResponseAudit(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WithArgs("audit_logs", "user_id").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("audit_logs", "event_id").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(1, 1))
}
