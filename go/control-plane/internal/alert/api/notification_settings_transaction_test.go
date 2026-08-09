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

func notificationSettingsCommandRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/settings", nil)
	request.Header.Set("Idempotency-Key", "notification-settings-key-0001")
	return request
}

func notificationSettingsCommandPayload() map[string]interface{} {
	return map[string]interface{}{
		"enabled": true, "min_severity": "high", "rate_limit_per_min": 10,
		"channels": map[string]interface{}{"email": true}, "secret_ref": "notification-secret-ref",
	}
}

func expectNotificationSettingsCreate(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-settings-key-0001").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-settings-key-0001").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("SELECT revision FROM alert_notification_settings").WithArgs("tenant-a").WillReturnRows(sqlmock.NewRows([]string{"revision"}))
	mock.ExpectQuery("INSERT INTO alert_notification_settings").
		WithArgs("tenant-a", `{"channels":{"email":true},"enabled":true,"min_severity":"high","rate_limit_per_min":10,"secret_ref":"notification-secret-ref"}`).
		WillReturnRows(sqlmock.NewRows([]string{"revision", "updated_at"}).AddRow(int64(1), now))
	mock.ExpectExec("INSERT INTO notification_governance_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", sqlmock.AnyArg(), int64(1), sqlmock.AnyArg(), "operator-a", "enable email notifications", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_outbox").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(1), "tenant-a", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_requests").
		WithArgs("tenant-a", "notification-settings-key-0001", sqlmock.AnyArg(), "notification-settings-action-0001", sqlmock.AnyArg(), int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestNotificationSettingsCreateCommitsEveryFactTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationSettingsCreate(mock, nil)
	zero := int64(0)
	record, err := NewAdvancedRepository(db, zap.NewNop()).SaveNotificationSettingsCommand(
		context.Background(), notificationSettingsCommandRequest(), "tenant-a", "operator-a",
		notificationSettingsCommandPayload(), "notification-settings-action-0001", "enable email notifications", &zero,
	)
	if err != nil {
		t.Fatal(err)
	}
	if record["revision"] != int64(1) || record["outbox_status"] != "pending" || record["event_id"] == "" {
		t.Fatalf("unexpected settings record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationSettingsAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationSettingsCreate(mock, errors.New("audit unavailable"))
	zero := int64(0)
	_, err = NewAdvancedRepository(db, zap.NewNop()).SaveNotificationSettingsCommand(
		context.Background(), notificationSettingsCommandRequest(), "tenant-a", "operator-a",
		notificationSettingsCommandPayload(), "notification-settings-action-0001", "enable email notifications", &zero,
	)
	if err == nil {
		t.Fatal("expected audit failure to roll back settings transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationSettingsRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expected := int64(2)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-settings-key-0001").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-settings-key-0001").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("SELECT revision FROM alert_notification_settings").WithArgs("tenant-a").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectRollback()
	_, err = NewAdvancedRepository(db, zap.NewNop()).SaveNotificationSettingsCommand(
		context.Background(), notificationSettingsCommandRequest(), "tenant-a", "operator-a",
		notificationSettingsCommandPayload(), "notification-settings-action-0001", "enable email notifications", &expected,
	)
	if !errors.Is(err, errNotificationRuleRevisionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
