package api

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func feedbackTransactionFixture() (*FeedbackRecord, *AlertFeedbackExtended) {
	createdAt := time.UnixMilli(1720000000123).UTC()
	record := &FeedbackRecord{
		FeedbackID: "11111111-1111-4111-8111-111111111111",
		AlertID:    "alert-1", TenantID: "tenant-a", UserID: "",
		Label: "FP", ReasonCode: "FALSE_ALARM", Comment: "known scanner",
		AlertType: "scan", Severity: "medium",
		ModelVersion: "model-v1", RuleVersion: "rule-v1",
		CreatedAt: createdAt,
	}
	event := &AlertFeedbackExtended{
		EventID: record.FeedbackID, EventType: "alert.feedback.v1",
		SchemaVersion: 1, AggregateVersion: 1,
		FeedbackID: record.FeedbackID, AlertID: record.AlertID,
		TenantID: record.TenantID, UserID: record.UserID,
		Label: record.Label, ReasonCode: record.ReasonCode, Comment: record.Comment,
		AlertType: record.AlertType, Severity: record.Severity,
		ModelVersion: record.ModelVersion, RuleVersion: record.RuleVersion,
		Timestamp: createdAt.UnixMilli(),
	}
	return record, event
}

func expectFeedbackAudit(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT data_type FROM information_schema.columns").
		WillReturnRows(sqlmock.NewRows([]string{"data_type"}).AddRow("text"))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO audit_logs").
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestFeedbackTransactionCommitsBusinessAuditAndOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	handler.actionAudit = NewAlertActionAuditWriter(db, zap.NewNop())
	record, event := feedbackTransactionFixture()
	request := httptest.NewRequest("POST", "/api/v1/alerts/alert-1/feedback", nil)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectFeedbackAudit(mock)
	mock.ExpectExec("INSERT INTO alert_feedback_outbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := handler.commitFeedbackTransaction(
		context.Background(), request, record, event, "request-1", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.IdempotentReplay || !result.CreatedAt.Equal(record.CreatedAt) {
		t.Fatalf("unexpected commit result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackTransactionRollsBackWhenOutboxInsertFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	handler.actionAudit = NewAlertActionAuditWriter(db, zap.NewNop())
	record, event := feedbackTransactionFixture()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectFeedbackAudit(mock)
	mock.ExpectExec("INSERT INTO alert_feedback_outbox").
		WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()

	_, err = handler.commitFeedbackTransaction(
		context.Background(),
		httptest.NewRequest("POST", "/api/v1/alerts/alert-1/feedback", nil),
		record, event, "request-1", nil,
	)
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackTransactionReturnsIdempotentReplayWithoutNewAuditOrOutbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	handler.actionAudit = NewAlertActionAuditWriter(db, zap.NewNop())
	record, event := feedbackTransactionFixture()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT tenant_id,alert_id,label").
		WillReturnRows(sqlmock.NewRows([]string{
			"tenant_id", "alert_id", "label", "reason_code", "comment",
			"add_to_whitelist", "created_at",
		}).AddRow(
			record.TenantID, record.AlertID, record.Label, record.ReasonCode,
			record.Comment, record.AddToWhitelist, record.CreatedAt,
		))
	mock.ExpectCommit()

	result, err := handler.commitFeedbackTransaction(
		context.Background(),
		httptest.NewRequest("POST", "/api/v1/alerts/alert-1/feedback", nil),
		record, event, "request-1", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IdempotentReplay || !result.CreatedAt.Equal(record.CreatedAt) {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackIdentityIsStableOnlyWhenIdempotencyKeyIsPresent(t *testing.T) {
	first := feedbackIdentity("tenant-a", "alert-1", "request-1")
	second := feedbackIdentity("tenant-a", "alert-1", "request-1")
	if first != second {
		t.Fatalf("stable feedback identity changed: %s != %s", first, second)
	}
	if first == feedbackIdentity("tenant-a", "alert-1", "request-2") {
		t.Fatal("different idempotency keys produced the same feedback identity")
	}
	if feedbackIdentity("tenant-a", "alert-1", "") == feedbackIdentity("tenant-a", "alert-1", "") {
		t.Fatal("legacy requests without idempotency keys must receive independent identities")
	}
}
