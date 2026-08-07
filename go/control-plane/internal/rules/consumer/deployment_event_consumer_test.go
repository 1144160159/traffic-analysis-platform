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

type fakeDeploymentProjectionApplier struct {
	inputs []DeploymentEventProjectionInput
	err    error
}

func (applier *fakeDeploymentProjectionApplier) ApplyDeploymentEventProjection(
	_ context.Context,
	input DeploymentEventProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func deploymentEventMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":       "11111111-1111-4111-8111-111111111111",
		"schema_version": 1, "event_type": "deployment_event",
		"action": "gray_started", "deployment_id": "deployment-1",
		"tenant_id": "tenant-a", "rule_version": "rule-v1",
		"model_version": "", "feature_set_id": "",
		"scope":  map[string]interface{}{"percentage": float64(20)},
		"status": "gray", "operator_id": "operator-a",
		"timestamp": int64(1720000000123),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "schema_version": "1",
		"event_type": "deployment_event", "action": "gray_started",
		"tenant_id": "tenant-a", "deployment_id": "deployment-1",
		"event_ts": "1720000000123",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "deployment.events.v1", Key: []byte("deployment-1"),
		Partition: 2, Offset: 17, Value: payload, Headers: headers,
	}}
}

func TestDeploymentEventConsumerAppliesValidatedEvent(t *testing.T) {
	applier := &fakeDeploymentProjectionApplier{}
	consumer := &DeploymentEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), deploymentEventMessage(t)); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want 1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.DeploymentID != "deployment-1" || input.Status != "gray" ||
		input.KafkaPartition != 2 || input.KafkaOffset != 17 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestDeploymentEventConsumerRejectsIdentityMismatch(t *testing.T) {
	applier := &fakeDeploymentProjectionApplier{}
	consumer := &DeploymentEventConsumer{applier: applier}
	message := deploymentEventMessage(t)
	message.Key = []byte("deployment-2")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected partition key mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestDeploymentEventConsumerRejectsUnknownField(t *testing.T) {
	applier := &fakeDeploymentProjectionApplier{}
	consumer := &DeploymentEventConsumer{applier: applier}
	message := deploymentEventMessage(t)
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

func TestDeploymentEventConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeDeploymentProjectionApplier{err: errors.New("database unavailable")}
	consumer := &DeploymentEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), deploymentEventMessage(t)); err == nil {
		t.Fatal("expected projection failure")
	}
}

func deploymentProjectionFixture() DeploymentEventProjectionInput {
	return DeploymentEventProjectionInput{
		EventID:      "11111111-1111-4111-8111-111111111111",
		DeploymentID: "deployment-1", TenantID: "tenant-a",
		Action: "gray_started", Status: "gray", OperatorID: "operator-a",
		TimestampMS: 1720000000123, Payload: map[string]interface{}{"status": "gray"},
		KafkaPartition: 2, KafkaOffset: 17,
	}
}

func TestPostgresDeploymentProjectionCommitsEventAndState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, err := NewPostgresDeploymentEventProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO deployment_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_state_projection").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := projection.ApplyDeploymentEventProjection(
		context.Background(), deploymentProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDeploymentProjectionSchemaFailsClosed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresDeploymentEventProjection(db)
	mock.ExpectQuery("SELECT count").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	if err := projection.VerifySchema(context.Background()); err == nil {
		t.Fatal("expected incomplete projection schema error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDeploymentProjectionAcceptsExactDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresDeploymentEventProjection(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO deployment_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectCommit()
	if err := projection.ApplyDeploymentEventProjection(
		context.Background(), deploymentProjectionFixture(),
	); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresDeploymentProjectionRejectsCollision(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	projection, _ := NewPostgresDeploymentEventProjection(db)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO deployment_event_projection").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()
	if err := projection.ApplyDeploymentEventProjection(
		context.Background(), deploymentProjectionFixture(),
	); err == nil {
		t.Fatal("expected collision")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
