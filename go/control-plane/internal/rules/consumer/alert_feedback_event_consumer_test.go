package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeAlertFeedbackProjectionApplier struct {
	inputs []AlertFeedbackProjectionInput
	err    error
}

func (applier *fakeAlertFeedbackProjectionApplier) ApplyAlertFeedbackProjection(
	_ context.Context,
	input AlertFeedbackProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func alertFeedbackEventMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":   "11111111-1111-4111-8111-111111111111",
		"event_type": "alert.feedback.v1", "schema_version": 1,
		"aggregate_version": 1, "alert_id": "alert-1", "tenant_id": "tenant-a",
		"label": "FP", "reason_code": "FALSE_ALARM", "comment": "known scanner",
		"user_id": "user-a", "timestamp": int64(1720000000123),
		"add_to_whitelist": false,
		"feedback_id":      "11111111-1111-4111-8111-111111111111",
		"alert_type":       "scan", "severity": "medium",
		"labels": []string{"network"}, "model_version": "model-v1", "rule_version": "rule-v1",
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "event_type": "alert.feedback.v1",
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": "tenant-a", "alert_id": "alert-1",
		"feedback_id": event["feedback_id"].(string),
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "alert.feedback.v1", Key: []byte("tenant-a:alert-1"),
		Partition: 1, Offset: 23, Value: payload, Headers: headers,
	}}
}

func TestAlertFeedbackEventConsumerAppliesValidatedEvent(t *testing.T) {
	applier := &fakeAlertFeedbackProjectionApplier{}
	consumer := &AlertFeedbackEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), alertFeedbackEventMessage(t)); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want 1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.FeedbackID != input.EventID || input.Label != "FP" ||
		input.KafkaPartition != 1 || input.KafkaOffset != 23 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestAlertFeedbackEventConsumerRejectsIdentityMismatch(t *testing.T) {
	applier := &fakeAlertFeedbackProjectionApplier{}
	consumer := &AlertFeedbackEventConsumer{applier: applier}
	message := alertFeedbackEventMessage(t)
	message.Key = []byte("tenant-a:alert-2")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected partition key mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestAlertFeedbackEventConsumerRejectsUnknownField(t *testing.T) {
	applier := &fakeAlertFeedbackProjectionApplier{}
	consumer := &AlertFeedbackEventConsumer{applier: applier}
	message := alertFeedbackEventMessage(t)
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		t.Fatal(err)
	}
	payload["unexpected"] = true
	message.Value, _ = json.Marshal(payload)
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}

func TestAlertFeedbackEventConsumerRequiresFPReason(t *testing.T) {
	applier := &fakeAlertFeedbackProjectionApplier{}
	consumer := &AlertFeedbackEventConsumer{applier: applier}
	message := alertFeedbackEventMessage(t)
	var payload map[string]interface{}
	_ = json.Unmarshal(message.Value, &payload)
	payload["reason_code"] = ""
	message.Value, _ = json.Marshal(payload)
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected missing FP reason rejection")
	}
}

func TestAlertFeedbackEventConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeAlertFeedbackProjectionApplier{err: errors.New("database unavailable")}
	consumer := &AlertFeedbackEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), alertFeedbackEventMessage(t)); err == nil {
		t.Fatal("expected projection failure")
	}
}

func alertFeedbackProjectionFixture() AlertFeedbackProjectionInput {
	return AlertFeedbackProjectionInput{
		EventID:    "11111111-1111-4111-8111-111111111111",
		FeedbackID: "11111111-1111-4111-8111-111111111111",
		TenantID:   "tenant-a", AlertID: "alert-1", UserID: "user-a",
		Label: "FP", ReasonCode: "FALSE_ALARM",
		ModelVersion: "model-v1", RuleVersion: "rule-v1",
		EventTimestampMS: 1720000000123,
		Payload:          map[string]interface{}{"label": "FP"},
		KafkaPartition:   1, KafkaOffset: 23,
	}
}

func TestPostgresAlertFeedbackProjectionCommitsEventAndInbox(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresAlertFeedbackProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO model_feedback_inbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := projection.ApplyAlertFeedbackProjection(
		context.Background(), alertFeedbackProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertFeedbackProjectionAcceptsExactDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresAlertFeedbackProjection(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()
	if err := projection.ApplyAlertFeedbackProjection(
		context.Background(), alertFeedbackProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertFeedbackProjectionRejectsCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresAlertFeedbackProjection(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_feedback_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()
	if err := projection.ApplyAlertFeedbackProjection(
		context.Background(), alertFeedbackProjectionFixture(),
	); err == nil {
		t.Fatal("expected collision")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertFeedbackProjectionSchemaFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresAlertFeedbackProjection(db)
	mock.ExpectQuery("SELECT count").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(28))
	if err := projection.VerifySchema(context.Background()); err == nil {
		t.Fatal("expected incomplete projection schema error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
