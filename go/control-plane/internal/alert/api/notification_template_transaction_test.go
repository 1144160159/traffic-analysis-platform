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

func notificationTemplateCommandRequest() *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/templates", nil)
	request.Header.Set("Idempotency-Key", "notification-template-key-0001")
	return request
}

func notificationTemplateCommandPayload() notificationTemplateRequest {
	enabled := true
	expected := 0
	return notificationTemplateRequest{
		TemplateType: "email", Name: "critical-template", Subject: "Critical {{.alert_id}}",
		Body: "Investigate", VariableSchema: map[string]interface{}{"alert_id": "string"}, Enabled: &enabled,
		ActionID: "notification-template-create-0001", Reason: "create critical template", ExpectedVersion: &expected,
	}
}

func expectNotificationTemplateCreate(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:notification-template-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").
		WithArgs("tenant-a", "notification-template-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("INSERT INTO notification_templates").
		WithArgs("tenant-a", "email", "critical-template", "Critical {{.alert_id}}", "Investigate", `{"alert_id":"string"}`, true, "operator-a").
		WillReturnRows(sqlmock.NewRows([]string{
			"template_id", "tenant_id", "template_type", "name", "version", "subject", "body",
			"variable_schema", "validation_status", "enabled", "created_by", "created_at", "updated_at",
		}).AddRow("00000000-0000-0000-0000-000000000121", "tenant-a", "email", "critical-template", 1,
			"Critical {{.alert_id}}", "Investigate", `{"alert_id":"string"}`, "passed", true, "operator-a", now, now))
	mock.ExpectExec("INSERT INTO notification_governance_history").
		WithArgs(sqlmock.AnyArg(), "tenant-a", "00000000-0000-0000-0000-000000000121", 1, "created", sqlmock.AnyArg(), "operator-a", "create critical template", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_outbox").
		WithArgs(sqlmock.AnyArg(), "00000000-0000-0000-0000-000000000121", 1, "tenant-a", "traffic.notification.template.v1.TemplateCreated", "tenant-a:00000000-0000-0000-0000-000000000121", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO notification_governance_requests").
		WithArgs("tenant-a", "notification-template-key-0001", sqlmock.AnyArg(), "notification-template-create-0001", "00000000-0000-0000-0000-000000000121", 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestNotificationTemplateCreateCommitsEveryFactTogether(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationTemplateCreate(mock, nil)
	record, err := NewAdvancedRepository(db, zap.NewNop()).CreateNotificationTemplate(
		context.Background(), notificationTemplateCommandRequest(), "tenant-a", "operator-a", notificationTemplateCommandPayload(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != 1 || record.OutboxStatus != "pending" || record.EventID == "" {
		t.Fatalf("unexpected template record: %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationTemplateAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectNotificationTemplateCreate(mock, errors.New("audit unavailable"))
	_, err = NewAdvancedRepository(db, zap.NewNop()).CreateNotificationTemplate(
		context.Background(), notificationTemplateCommandRequest(), "tenant-a", "operator-a", notificationTemplateCommandPayload(),
	)
	if err == nil {
		t.Fatal("expected audit failure to roll back template transaction")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationTemplatePatchRejectsStaleVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expected := 2
	req := notificationTemplateRequest{Enabled: notificationBoolPointer(false), ActionID: "template-patch", Reason: "disable", ExpectedVersion: &expected}
	request := notificationTemplateCommandRequest()
	request.Header.Set("Idempotency-Key", "notification-template-key-0002")
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("tenant-a:notification-template-key-0002").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM notification_governance_requests r").WithArgs("tenant-a", "notification-template-key-0002").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("UPDATE notification_templates").WithArgs("tenant-a", "00000000-0000-0000-0000-000000000121", "", "", "", "", nil, req.Enabled, expected).
		WillReturnRows(sqlmock.NewRows([]string{"template_id", "tenant_id", "template_type", "name", "version", "subject", "body", "variable_schema", "validation_status", "enabled", "created_by", "created_at", "updated_at"}))
	mock.ExpectQuery("SELECT version FROM notification_templates").WithArgs("tenant-a", "00000000-0000-0000-0000-000000000121").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	mock.ExpectRollback()
	_, _, err = NewAdvancedRepository(db, zap.NewNop()).PatchNotificationTemplate(
		context.Background(), request, "tenant-a", "00000000-0000-0000-0000-000000000121", "operator-a", req,
	)
	if !errors.Is(err, errNotificationRuleRevisionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
