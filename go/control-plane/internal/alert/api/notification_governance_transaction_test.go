package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func notificationRuleCommandRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/subscriptions", nil)
	request.Header.Set("Idempotency-Key", "notification-rule-key-0001")
	request.Header.Set("User-Agent", "notification-governance-test")
	return request
}

func notificationRuleCommandPayload() notificationRuleRequest {
	enabled := true
	expectedRevision := int64(0)
	return notificationRuleRequest{
		Name: "critical pager", Conditions: map[string]interface{}{"severity": "critical"},
		Channels: []string{"email"}, Enabled: &enabled, ActionID: "notification-rule-create-0001",
		Reason: "route critical alerts", ExpectedRevision: &expectedRevision,
	}
}

func expectNotificationRuleCreate(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:notification-rule-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").
		WithArgs("tenant-a", "notification-rule-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("INSERT INTO notification_rules").
		WithArgs("tenant-a", "critical pager", `{"severity":"critical"}`, `["email"]`, true, "", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"rule_id", "tenant_id", "name", "conditions", "channels", "enabled", "created_by", "revision", "created_at", "updated_at",
		}).AddRow("00000000-0000-0000-0000-000000000111", "tenant-a", "critical pager", `{"severity":"critical"}`, `["email"]`, true, "", int64(1), now, now))
	mock.ExpectExec("INSERT INTO notification_governance_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "00000000-0000-0000-0000-000000000111", int64(1), "created", sqlmock.AnyArg(), "operator-a", "route critical alerts", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_outbox").
		WithArgs(sqlmock.AnyArg(), "00000000-0000-0000-0000-000000000111", int64(1), "tenant-a", "traffic.notification.rule.v1.RuleCreated", "tenant-a:00000000-0000-0000-0000-000000000111", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_requests").
		WithArgs("tenant-a", "notification-rule-key-0001", sqlmock.AnyArg(), "notification-rule-create-0001", "00000000-0000-0000-0000-000000000111", int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestNotificationRuleCreateCommitsBusinessHistoryOutboxAuditAndRequestTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationRuleCreate(mock, nil)
	repository := NewAdvancedRepository(db, zap.NewNop())
	record, err := repository.CreateNotificationRule(context.Background(), notificationRuleCommandRequest(), "tenant-a", "operator-a", notificationRuleCommandPayload())
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 1 || record.OutboxStatus != "pending" || record.EventID == "" || record.IdempotentReuse {
		t.Fatalf("unexpected committed record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationRuleCreateAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationRuleCreate(mock, errors.New("audit unavailable"))
	repository := NewAdvancedRepository(db, zap.NewNop())
	if _, err := repository.CreateNotificationRule(context.Background(), notificationRuleCommandRequest(), "tenant-a", "operator-a", notificationRuleCommandPayload()); err == nil {
		t.Fatal("expected atomic command to fail when audit insert fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationRuleCreateIdempotentReplayReturnsCommittedResponse(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	req := notificationRuleCommandPayload()
	payload, _ := json.Marshal(map[string]interface{}{"action": "created", "aggregate_id": "", "request": req})
	digest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(digest[:])
	now := time.Now().UTC()
	response, _ := json.Marshal(NotificationRuleRecord{
		RuleID: "00000000-0000-0000-0000-000000000111", TenantID: "tenant-a", Name: "critical pager",
		Conditions: map[string]interface{}{"severity": "critical"}, Channels: []string{"email"}, Enabled: true,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	})
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:notification-rule-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").
		WithArgs("tenant-a", "notification-rule-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}).
			AddRow(payloadHash, string(response), "00000000-0000-0000-0000-000000000211", "published"))
	mock.ExpectCommit()
	repository := NewAdvancedRepository(db, zap.NewNop())
	record, err := repository.CreateNotificationRule(context.Background(), notificationRuleCommandRequest(), "tenant-a", "operator-a", req)
	if err != nil {
		t.Fatal(err)
	}
	if !record.IdempotentReuse || record.OutboxStatus != "published" || record.EventID != "00000000-0000-0000-0000-000000000211" {
		t.Fatalf("unexpected replay record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationRulePatchRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectedRevision := int64(2)
	req := notificationRuleRequest{Enabled: notificationBoolPointer(false), ActionID: "notification-rule-patch-0001", Reason: "disable route", ExpectedRevision: &expectedRevision}
	request := notificationRuleCommandRequest()
	request.Header.Set("Idempotency-Key", "notification-rule-key-0002")
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:notification-rule-key-0002").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").
		WithArgs("tenant-a", "notification-rule-key-0002").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("UPDATE notification_rules").
		WithArgs("tenant-a", "00000000-0000-0000-0000-000000000111", "", nil, nil, req.Enabled, sqlmock.AnyArg(), expectedRevision).
		WillReturnRows(sqlmock.NewRows([]string{"rule_id", "tenant_id", "name", "conditions", "channels", "enabled", "created_by", "revision", "created_at", "updated_at"}))
	mock.ExpectQuery("SELECT revision FROM notification_rules").
		WithArgs("tenant-a", "00000000-0000-0000-0000-000000000111").
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectRollback()
	repository := NewAdvancedRepository(db, zap.NewNop())
	_, _, err = repository.PatchNotificationRule(context.Background(), request, "tenant-a", "00000000-0000-0000-0000-000000000111", "operator-a", req)
	if !errors.Is(err, errNotificationRuleRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func notificationBoolPointer(value bool) *bool { return &value }
