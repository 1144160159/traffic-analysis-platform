package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func notificationEscalationCommandRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/escalation-policies", nil)
	request.Header.Set("Idempotency-Key", "notification-escalation-key-0001")
	return request
}

func notificationEscalationCommandPayload() notificationEscalationRequest {
	enabled := true
	expected := int64(0)
	return notificationEscalationRequest{
		Name: "critical escalation", Stages: []map[string]interface{}{{"after_minutes": float64(5), "target_role": "soc"}},
		Enabled: &enabled, ActionID: "notification-escalation-create-0001", Reason: "escalate critical alerts", ExpectedRevision: &expected,
	}
}

func expectNotificationEscalationCreate(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-escalation-key-0001").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-escalation-key-0001").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("INSERT INTO notification_escalation_policies").
		WithArgs("tenant-a", "critical escalation", `[{"after_minutes":5,"target_role":"soc"}]`, true, "operator-a").
		WillReturnRows(sqlmock.NewRows([]string{"policy_id", "tenant_id", "name", "stages", "enabled", "created_by", "revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-0000-0000-000000000131", "tenant-a", "critical escalation", `[{"after_minutes":5,"target_role":"soc"}]`, true, "operator-a", int64(1), now, now))
	mock.ExpectExec("INSERT INTO notification_governance_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "00000000-0000-0000-0000-000000000131", int64(1), "created", sqlmock.AnyArg(), "operator-a", "escalate critical alerts", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_outbox").
		WithArgs(sqlmock.AnyArg(), "00000000-0000-0000-0000-000000000131", int64(1), "tenant-a", "traffic.notification.escalation.v1.PolicyCreated", "tenant-a:00000000-0000-0000-0000-000000000131", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_requests").
		WithArgs("tenant-a", "notification-escalation-key-0001", sqlmock.AnyArg(), "notification-escalation-create-0001", "00000000-0000-0000-0000-000000000131", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestNotificationEscalationCreateCommitsEveryFactTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationEscalationCreate(mock, nil)
	record, err := NewAdvancedRepository(db, zap.NewNop()).CreateNotificationEscalationPolicy(
		context.Background(), notificationEscalationCommandRequest(), "tenant-a", "operator-a", notificationEscalationCommandPayload(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 1 || record.OutboxStatus != "pending" || record.EventID == "" {
		t.Fatalf("unexpected escalation record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationEscalationAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationEscalationCreate(mock, errors.New("audit unavailable"))
	_, err = NewAdvancedRepository(db, zap.NewNop()).CreateNotificationEscalationPolicy(
		context.Background(), notificationEscalationCommandRequest(), "tenant-a", "operator-a", notificationEscalationCommandPayload(),
	)
	if err == nil {
		t.Fatal("expected audit failure to roll back escalation transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationEscalationPatchRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expected := int64(2)
	req := notificationEscalationRequest{Enabled: notificationBoolPointer(false), ActionID: "escalation-patch", Reason: "disable", ExpectedRevision: &expected}
	request := notificationEscalationCommandRequest()
	request.Header.Set("Idempotency-Key", "notification-escalation-key-0002")
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-escalation-key-0002").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-escalation-key-0002").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("UPDATE notification_escalation_policies").WithArgs("tenant-a", "00000000-0000-0000-0000-000000000131", "", nil, req.Enabled, expected).
		WillReturnRows(sqlmock.NewRows([]string{"policy_id", "tenant_id", "name", "stages", "enabled", "created_by", "revision", "created_at", "updated_at"}))
	mock.ExpectQuery("SELECT revision FROM notification_escalation_policies").WithArgs("tenant-a", "00000000-0000-0000-0000-000000000131").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectRollback()
	_, _, err = NewAdvancedRepository(db, zap.NewNop()).PatchNotificationEscalationPolicy(
		context.Background(), request, "tenant-a", "00000000-0000-0000-0000-000000000131", "operator-a", req,
	)
	if !errors.Is(err, errNotificationRuleRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
