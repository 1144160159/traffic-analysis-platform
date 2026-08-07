package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"go.uber.org/zap"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

var userSettingsTestUserID = uuid.MustParse("00000000-0000-0000-0000-000000000123")

func userSettingsTestCommand() UserSettingsCommand {
	zero := int64(0)
	return UserSettingsCommand{
		TenantID: "tenant-a", UserID: userSettingsTestUserID, Username: "analyst-a", Category: "display",
		Settings: map[string]interface{}{"page_size": 50}, ActionID: "settings-action-0001",
		IdempotencyKey: "settings-key-0001", Reason: "increase table page size", ExpectedRevision: &zero,
		TraceID: "trace-settings-0001", SourceIP: "10.0.0.10", UserAgent: "test-agent",
	}
}

func expectUserSettingsCreate(mock sqlmock.Sqlmock, auditErr error) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("tenant-a:" + userSettingsTestUserID.String() + ":display:settings-key-0001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM user_settings_requests r").
		WithArgs("tenant-a", userSettingsTestUserID, "display", "settings-key-0001").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("SELECT revision FROM user_settings").
		WithArgs("tenant-a", userSettingsTestUserID, "display").
		WillReturnRows(sqlmock.NewRows([]string{"revision"}))
	mock.ExpectQuery("INSERT INTO user_settings").
		WithArgs("tenant-a", userSettingsTestUserID, "display", `{"page_size":50}`).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "user_id", "category", "settings", "revision", "created_at", "updated_at"}).
			AddRow("tenant-a", userSettingsTestUserID, "display", []byte(`{"page_size":50}`), int64(1), now, now))
	mock.ExpectExec("INSERT INTO user_settings_history").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO user_settings_outbox").WillReturnResult(sqlmock.NewResult(1, 1))
	audit := mock.ExpectExec("INSERT INTO audit_logs")
	if auditErr != nil {
		audit.WillReturnError(auditErr)
		mock.ExpectRollback()
		return
	}
	audit.WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO user_settings_requests").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
}

func TestUserSettingsCommandCommitsStateHistoryOutboxAuditAndRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectUserSettingsCreate(mock, nil)

	result, err := NewUserSettingsRepository(db, zap.NewNop()).SaveCommand(context.Background(), userSettingsTestCommand())
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 1 || result.EventID == "" || result.OutboxStatus != "pending" || result.IdempotentReuse {
		t.Fatalf("unexpected committed result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserSettingsCommandAuditFailureRollsBackEveryFact(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectUserSettingsCreate(mock, errors.New("audit unavailable"))

	_, err = NewUserSettingsRepository(db, zap.NewNop()).SaveCommand(context.Background(), userSettingsTestCommand())
	if err == nil {
		t.Fatal("expected audit failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserSettingsCommandRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	command := userSettingsTestCommand()
	expected := int64(2)
	command.ExpectedRevision = &expected
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM user_settings_requests r").WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}))
	mock.ExpectQuery("SELECT revision FROM user_settings").WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(3)))
	mock.ExpectRollback()

	_, err = NewUserSettingsRepository(db, zap.NewNop()).SaveCommand(context.Background(), command)
	if !commonerrors.IsCode(err, commonerrors.ErrCodeVersionConflict) {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUserSettingsCommandReplaysCommittedResponse(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	command := userSettingsTestCommand()
	now := time.Now().UTC()
	response := `{"tenant_id":"tenant-a","user_id":"00000000-0000-0000-0000-000000000123","category":"display","settings":{"page_size":50},"revision":1,"created_at":"` + now.Format(time.RFC3339Nano) + `","updated_at":"` + now.Format(time.RFC3339Nano) + `"}`
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	payload, _ := jsonPayloadHash(command)
	mock.ExpectQuery("FROM user_settings_requests r").
		WillReturnRows(sqlmock.NewRows([]string{"payload_sha256", "response_payload", "event_id", "status"}).
			AddRow(payload, response, "00000000-0000-0000-0000-000000000999", "published"))
	mock.ExpectCommit()

	result, err := NewUserSettingsRepository(db, zap.NewNop()).SaveCommand(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IdempotentReuse || result.EventID != "00000000-0000-0000-0000-000000000999" || result.OutboxStatus != "published" {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func jsonPayloadHash(command UserSettingsCommand) (string, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"action_id": command.ActionID, "reason": command.Reason,
		"expected_revision": command.ExpectedRevision, "settings": command.Settings,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
