package consumer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeModelActionExecutionApplier struct {
	inputs []ModelActionExecutionInput
}

func (applier *fakeModelActionExecutionApplier) ApplyModelActionExecution(
	_ context.Context,
	input ModelActionExecutionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return nil
}

func modelActionMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":       "11111111-1111-4111-8111-111111111111",
		"event_type":     "model.action.requested.v1",
		"schema_version": 1, "aggregate_version": 1,
		"job_id":    "22222222-2222-4222-8222-222222222222",
		"action_id": "33333333-3333-4333-8333-333333333333",
		"tenant_id": "tenant-a",
		"model_id":  "44444444-4444-4444-8444-444444444444",
		"version":   "", "action": "request-retraining",
		"target": "mlops", "payload": map[string]interface{}{
			"dataset_id": "dataset-1", "strategy": "full", "reason": "drift",
		},
		"status": "queued", "requested_by": "operator-a",
		"trace_id":   "trace-1",
		"created_at": "2026-07-31T08:30:00Z",
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]segmentkafka.Header, 0)
	for key, value := range map[string]string{
		"event_id":       event["event_id"].(string),
		"event_type":     "model.action.requested.v1",
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": "tenant-a", "job_id": event["job_id"].(string),
		"action_id":    event["action_id"].(string),
		"trace_id":     "trace-1",
		"content_type": "application/json",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "model-actions.v1", Partition: 1, Offset: 12,
		Key: []byte(event["model_id"].(string)), Value: payload, Headers: headers,
	}}
}

func TestModelActionEventConsumerValidatesAndApplies(t *testing.T) {
	applier := &fakeModelActionExecutionApplier{}
	consumer := &ModelActionEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), modelActionMessage(t)); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want=1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.Action != "request-retraining" ||
		input.KafkaPartition != 1 || input.KafkaOffset != 12 {
		t.Fatalf("unexpected model action input: %#v", input)
	}
}

func TestModelActionEventConsumerRejectsPartitionIdentityMismatch(t *testing.T) {
	applier := &fakeModelActionExecutionApplier{}
	consumer := &ModelActionEventConsumer{applier: applier}
	message := modelActionMessage(t)
	message.Key = []byte("55555555-5555-4555-8555-555555555555")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected model partition key mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid model action reached execution inbox")
	}
}

func modelActionExecutionFixture() ModelActionExecutionInput {
	return ModelActionExecutionInput{
		EventID:          "11111111-1111-4111-8111-111111111111",
		JobID:            "22222222-2222-4222-8222-222222222222",
		ActionID:         "33333333-3333-4333-8333-333333333333",
		AggregateVersion: 1, TenantID: "tenant-a",
		ModelID: "44444444-4444-4444-8444-444444444444",
		Action:  "request-retraining", Target: "mlops",
		Payload: map[string]interface{}{
			"dataset_id": "dataset-1", "strategy": "full", "reason": "drift",
		},
		Status: "queued", RequestedBy: "operator-a", TraceID: "trace-1",
		CreatedAt: time.Date(2026, 7, 31, 8, 30, 0, 0, time.UTC),
		RawPayload: map[string]interface{}{
			"event_id": "11111111-1111-4111-8111-111111111111",
		},
		KafkaPartition: 1, KafkaOffset: 12,
	}
}

func TestPostgresModelActionExecutionCommitsInboxAndNonTerminalState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewPostgresModelActionExecutionInbox(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_action_execution_inbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := inbox.ApplyModelActionExecution(
		context.Background(), modelActionExecutionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresModelActionExecutionExactReplayReconcilesState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewPostgresModelActionExecutionInbox(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_action_execution_inbox").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := inbox.ApplyModelActionExecution(
		context.Background(), modelActionExecutionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresModelActionExecutionRejectsAuthoritativeMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inbox, _ := NewPostgresModelActionExecutionInbox(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO model_action_execution_inbox").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE model_action_jobs").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if err := inbox.ApplyModelActionExecution(
		context.Background(), modelActionExecutionFixture(),
	); err == nil {
		t.Fatal("expected authoritative mismatch to fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
