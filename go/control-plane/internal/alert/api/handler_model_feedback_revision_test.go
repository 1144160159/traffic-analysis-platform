package api

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func modelFeedbackRevisionFixture() (ModelFeedbackAdjudicatedV1, FeedbackRequest) {
	event := ModelFeedbackAdjudicatedV1{
		EventID:   "11111111-1111-4111-8111-111111111111",
		EventType: modelFeedbackRevisionEventType, SchemaVersion: 1,
		TenantID: "tenant-a", PredictionID: "prediction-a", AlertID: "alert-a",
		Label: "FP", AdjudicationState: "ADJUDICATED", ReasonCode: "FALSE_ALARM",
		ModelVersion: "model-v1", RuleVersion: "rule-v1", AdjudicatedBy: "operator-a",
		OccurredAtMS: time.Now().UnixMilli(), TraceID: "00112233445566778899aabbccddeeff",
	}
	event.FeedbackID = modelFeedbackAggregateIdentity(event.TenantID, event.PredictionID)
	return event, FeedbackRequest{
		Label: "FP", ReasonCode: "FALSE_ALARM", Comment: "reviewed",
		AdjudicationState: "ADJUDICATED", ExpectedLabelRevision: 0,
	}
}

func newModelFeedbackRevisionTestHandler(t *testing.T) (*FeedbackHandler, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFeedbackHandler(nil, nil, nil, nil, nil, zap.NewNop())
	handler.actionAudit = NewAlertActionAuditWriter(db, zap.NewNop())
	return handler, mock, func() { db.Close() }
}

func expectNewModelFeedbackRevision(mock sqlmock.Sqlmock, headPayload *string) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload::text,created_at").
		WillReturnError(sql.ErrNoRows)
	head := mock.ExpectQuery("SELECT payload::text FROM alert_feedback")
	if headPayload == nil {
		head.WillReturnError(sql.ErrNoRows)
	} else {
		head.WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(*headPayload))
	}
}

func expectModelFeedbackRevisionCommit(mock sqlmock.Sqlmock) {
	mock.ExpectExec("INSERT INTO alert_feedback").WillReturnResult(sqlmock.NewResult(0, 1))
	expectFeedbackAudit(mock)
	mock.ExpectExec("INSERT INTO alert_feedback_outbox").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestModelFeedbackFirstRevisionCommitsAuthorityAuditAndOutbox(t *testing.T) {
	handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
	defer closeDB()
	event, command := modelFeedbackRevisionFixture()
	expectNewModelFeedbackRevision(mock, nil)
	expectModelFeedbackRevisionCommit(mock)

	result, err := handler.commitModelFeedbackRevision(context.Background(),
		httptest.NewRequest("POST", "/alerts/alert-a/feedback", nil),
		&event, command, "command-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Event.LabelRevision != 1 || result.Event.AggregateVersion != 1 || result.Event.PreviousEventID != "" {
		t.Fatalf("unexpected first revision: %#v", result.Event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFeedbackNextRevisionLinksPreviousEvent(t *testing.T) {
	handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
	defer closeDB()
	event, command := modelFeedbackRevisionFixture()
	head := event
	head.EventID = "33333333-3333-4333-8333-333333333333"
	head.LabelRevision, head.AggregateVersion = 1, 1
	headPayload, _ := json.Marshal(head)
	headJSON := string(headPayload)
	command.ExpectedLabelRevision = 1
	expectNewModelFeedbackRevision(mock, &headJSON)
	expectModelFeedbackRevisionCommit(mock)

	result, err := handler.commitModelFeedbackRevision(context.Background(),
		httptest.NewRequest("POST", "/alerts/alert-a/feedback", nil),
		&event, command, "command-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Event.LabelRevision != 2 || result.Event.AggregateVersion != 2 || result.Event.PreviousEventID != head.EventID {
		t.Fatalf("unexpected second revision: %#v", result.Event)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFeedbackRejectsStaleRevisionAndRetractedHead(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		headState string
		headModel string
		expected  int64
		want      error
	}{
		{name: "stale", headState: "ADJUDICATED", expected: 0, want: errModelFeedbackRevisionConflict},
		{name: "terminal", headState: "RETRACTED", expected: 1, want: errModelFeedbackAlreadyRetracted},
		{name: "immutable-model", headState: "ADJUDICATED", headModel: "model-v0", expected: 1, want: errModelFeedbackRevisionConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
			defer closeDB()
			event, command := modelFeedbackRevisionFixture()
			head := event
			head.EventID = "33333333-3333-4333-8333-333333333333"
			head.LabelRevision, head.AggregateVersion = 1, 1
			head.AdjudicationState = testCase.headState
			if testCase.headModel != "" {
				head.ModelVersion = testCase.headModel
			}
			headPayload, _ := json.Marshal(head)
			headJSON := string(headPayload)
			command.ExpectedLabelRevision = testCase.expected
			expectNewModelFeedbackRevision(mock, &headJSON)
			mock.ExpectRollback()
			_, err := handler.commitModelFeedbackRevision(context.Background(),
				httptest.NewRequest("POST", "/alerts/alert-a/feedback", nil),
				&event, command, "command", nil)
			if !stderrors.Is(err, testCase.want) {
				t.Fatalf("got %v, want %v", err, testCase.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelFeedbackIdempotentReplayDoesNotWriteAgain(t *testing.T) {
	handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
	defer closeDB()
	event, command := modelFeedbackRevisionFixture()
	event.LabelRevision, event.AggregateVersion = 1, 1
	payload, _ := json.Marshal(event)
	createdAt := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload::text,created_at").
		WillReturnRows(sqlmock.NewRows([]string{"payload", "created_at", "comment", "add_to_whitelist"}).
			AddRow(string(payload), createdAt, command.Comment, command.AddToWhitelist))
	mock.ExpectCommit()

	result, err := handler.commitModelFeedbackRevision(context.Background(),
		httptest.NewRequest("POST", "/alerts/alert-a/feedback", nil),
		&event, command, "command-1", nil)
	if err != nil || !result.IdempotentReplay || result.Event.EventID != event.EventID {
		t.Fatalf("unexpected replay result=%#v err=%v", result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFeedbackIdempotencyRejectsChangedCommand(t *testing.T) {
	handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
	defer closeDB()
	event, command := modelFeedbackRevisionFixture()
	event.LabelRevision, event.AggregateVersion = 1, 1
	payload, _ := json.Marshal(event)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT payload::text,created_at").
		WillReturnRows(sqlmock.NewRows([]string{"payload", "created_at", "comment", "add_to_whitelist"}).
			AddRow(string(payload), time.Now(), command.Comment, command.AddToWhitelist))
	mock.ExpectRollback()
	command.Comment = "changed command"
	_, err := handler.commitModelFeedbackRevision(context.Background(),
		httptest.NewRequest("POST", "/alerts/alert-a/feedback", nil), &event, command, "command-1", nil)
	if !stderrors.Is(err, errModelFeedbackRevisionConflict) {
		t.Fatalf("changed idempotent command err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFeedbackOutboxClaimIsEventTypeScoped(t *testing.T) {
	handler, mock, closeDB := newModelFeedbackRevisionTestHandler(t)
	defer closeDB()
	mock.ExpectQuery(regexp.QuoteMeta("AND payload->>'event_type'=$3")).
		WithArgs(50, "worker-a", modelFeedbackRevisionEventType).
		WillReturnRows(sqlmock.NewRows([]string{
			"outbox_id", "event_id", "feedback_id", "tenant_id", "alert_id",
			"partition_key", "schema_version", "aggregate_version", "payload",
		}))
	processed, err := handler.drainTypedOutbox(
		context.Background(), "worker-a", modelFeedbackRevisionEventType, new(kafka.Producer), 50,
	)
	if err != nil || processed != 0 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFeedbackIdentitiesAreTenantBound(t *testing.T) {
	if modelFeedbackAggregateIdentity("tenant-a", "prediction") == modelFeedbackAggregateIdentity("tenant-b", "prediction") {
		t.Fatal("aggregate identity crossed tenant boundary")
	}
	if modelFeedbackRevisionEventIdentity("tenant-a", "prediction", "command") ==
		modelFeedbackRevisionEventIdentity("tenant-b", "prediction", "command") {
		t.Fatal("event identity crossed tenant boundary")
	}
}

func TestVerifyModelFeedbackProducerReadinessRequiresBrokerReceipt(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	options := ModelFeedbackProducerReadiness{
		Topic: modelFeedbackRevisionEventType, ConsumerGroup: "consumer-a",
		CandidateSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ContractSHA256:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	mock.ExpectQuery("SELECT r.state").WillReturnError(sql.ErrNoRows)
	if err := VerifyModelFeedbackProducerReadiness(context.Background(), db, options); err == nil {
		t.Fatal("missing broker receipt authorized the producer")
	}
	mock.ExpectQuery("SELECT r.state").WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("READY"))
	if err := VerifyModelFeedbackProducerReadiness(context.Background(), db, options); err != nil {
		t.Fatal(err)
	}
	options.CandidateSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := VerifyModelFeedbackProducerReadiness(context.Background(), db, options); err == nil {
		t.Fatal("zero candidate hash authorized the producer")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
