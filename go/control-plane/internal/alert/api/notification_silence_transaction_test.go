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

func notificationSilenceCommandRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/silence-rules", nil)
	request.Header.Set("Idempotency-Key", "notification-silence-key-0001")
	return request
}

func notificationSilenceCommandPayload() (NotificationSilenceRule, notificationSilenceRuleRequest) {
	start := time.Date(2026, 8, 4, 1, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	enabled := true
	zero := int64(0)
	req := notificationSilenceRuleRequest{
		Name: "maintenance", Scope: "all", StartsAt: start, EndsAt: end,
		AffectedTargets: []string{"campus-a"}, Policy: "all", Reason: "planned maintenance", Enabled: &enabled,
		ActionID: "notification-silence-action-0001", ActionReason: "schedule maintenance window", ExpectedRevision: &zero,
	}
	rule, _ := req.toRule("tenant-a", "operator-a")
	return rule, req
}

func expectNotificationSilenceCreate(mock sqlmock.Sqlmock, auditErr error) {
	rule, _ := notificationSilenceCommandPayload()
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-silence-key-0001").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-silence-key-0001").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("INSERT INTO notification_silence_rules").
		WithArgs(sqlmock.AnyArg(), "tenant-a", rule.Name, rule.Scope, rule.StartsAt, rule.EndsAt, `["campus-a"]`, rule.Policy, rule.Reason, rule.Enabled, rule.CreatedBy).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "tenant_id", "name", "scope", "starts_at", "ends_at", "affected_targets", "policy", "reason", "enabled", "created_by", "revision", "created_at", "updated_at"}).
			AddRow("00000000-0000-0000-0000-000000000141", "tenant-a", rule.Name, rule.Scope, rule.StartsAt, rule.EndsAt, `["campus-a"]`, rule.Policy, rule.Reason, true, "operator-a", int64(1), now, now))
	mock.ExpectExec("INSERT INTO notification_governance_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_requests").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestNotificationSilenceCreateCommitsEveryFactTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationSilenceCreate(mock, nil)
	rule, req := notificationSilenceCommandPayload()
	record, err := NewAdvancedRepository(db, zap.NewNop()).CreateNotificationSilenceRule(context.Background(), notificationSilenceCommandRequest(), rule, req)
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 1 || record.OutboxStatus != "pending" || record.EventID == "" {
		t.Fatalf("unexpected silence record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationSilenceAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationSilenceCreate(mock, errors.New("audit unavailable"))
	rule, req := notificationSilenceCommandPayload()
	_, err = NewAdvancedRepository(db, zap.NewNop()).CreateNotificationSilenceRule(context.Background(), notificationSilenceCommandRequest(), rule, req)
	if err == nil {
		t.Fatal("expected audit failure to roll back silence transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationSilencePatchRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expected := int64(2)
	enabled := false
	req := notificationSilencePatchRequest{Enabled: &enabled, ActionID: "silence-patch", ActionReason: "disable", ExpectedRevision: &expected}
	request := notificationSilenceCommandRequest()
	request.Header.Set("Idempotency-Key", "notification-silence-key-0002")
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-silence-key-0002").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-silence-key-0002").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("UPDATE notification_silence_rules").WillReturnRows(sqlmock.NewRows([]string{"rule_id", "tenant_id", "name", "scope", "starts_at", "ends_at", "affected_targets", "policy", "reason", "enabled", "created_by", "revision", "created_at", "updated_at"}))
	mock.ExpectQuery("SELECT revision FROM notification_silence_rules").WithArgs("tenant-a", "00000000-0000-0000-0000-000000000141").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectRollback()
	_, _, err = NewAdvancedRepository(db, zap.NewNop()).PatchNotificationSilenceRule(
		context.Background(), request, "tenant-a", "00000000-0000-0000-0000-000000000141", "operator-a", req,
	)
	if !errors.Is(err, errNotificationRuleRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
