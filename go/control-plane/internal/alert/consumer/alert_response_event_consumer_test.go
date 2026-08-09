package consumer

import (
	"context"
	"encoding/json"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeAlertResponseProjection struct {
	inputs []AlertResponseProjectionInput
}

func (projection *fakeAlertResponseProjection) ApplyAlertResponseProjection(
	_ context.Context,
	input AlertResponseProjectionInput,
) error {
	projection.inputs = append(projection.inputs, input)
	return nil
}

func alertResponseMessage(t *testing.T, dryRun bool) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":       "11111111-1111-4111-8111-111111111111",
		"event_type":     "alert.response.requested.v1",
		"schema_version": 1, "aggregate_version": 1,
		"job_id":    "alert-action-22222222-2222-4222-8222-222222222222",
		"tenant_id": "tenant-a", "alert_id": "alert-1",
		"action_id": "alert-response-block-ip", "action": "block_ip",
		"target": "198.51.100.10", "reason": "approved test",
		"requested_by": "operator-a", "trace_id": "trace-alert-response-test", "dry_run": dryRun,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := make([]segmentkafka.Header, 0)
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "event_type": event["event_type"].(string),
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": "tenant-a", "alert_id": "alert-1",
		"job_id": event["job_id"].(string), "action_id": "alert-response-block-ip",
		"trace_id": "trace-alert-response-test",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "alert.response.requested.v1", Partition: 1, Offset: 9,
		Key:   []byte("tenant-a:" + event["job_id"].(string)),
		Value: payload, Headers: headers,
	}}
}

func TestAlertResponseEventConsumerValidatesDryRun(t *testing.T) {
	projection := &fakeAlertResponseProjection{}
	consumer := &AlertResponseEventConsumer{applier: projection}
	if err := consumer.handle(context.Background(), alertResponseMessage(t, true)); err != nil {
		t.Fatal(err)
	}
	if len(projection.inputs) != 1 || !projection.inputs[0].DryRun ||
		projection.inputs[0].KafkaPartition != 1 || projection.inputs[0].KafkaOffset != 9 {
		t.Fatalf("unexpected projection input: %#v", projection.inputs)
	}
}

func TestAlertResponseEventConsumerAcceptsApprovedAggregateVersion(t *testing.T) {
	projection := &fakeAlertResponseProjection{}
	consumer := &AlertResponseEventConsumer{applier: projection}
	message := alertResponseMessage(t, false)
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		t.Fatal(err)
	}
	payload["aggregate_version"] = 2
	payload["approved_by"] = "approver-b"
	payload["approval_reason"] = "independent approval"
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	message.Value = encoded
	for index := range message.Headers {
		if message.Headers[index].Key == "aggregate_version" {
			message.Headers[index].Value = []byte("2")
		}
	}
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(projection.inputs) != 1 || projection.inputs[0].AggregateVersion != 2 ||
		projection.inputs[0].DryRun {
		t.Fatalf("unexpected approved projection input: %#v", projection.inputs)
	}
}

func TestAlertResponseEventConsumerRejectsIdentityMismatch(t *testing.T) {
	projection := &fakeAlertResponseProjection{}
	consumer := &AlertResponseEventConsumer{applier: projection}
	message := alertResponseMessage(t, true)
	message.Key = []byte("tenant-b:wrong")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected partition identity mismatch")
	} else if !commonkafka.IsPermanent(err) {
		t.Fatalf("identity mismatch must be permanently quarantinable: %v", err)
	}
	if len(projection.inputs) != 0 {
		t.Fatal("invalid event reached projection")
	}
}

func TestPostgresAlertResponseProjectionCompletesSimulation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := AlertResponseProjectionInput{
		EventID:  "11111111-1111-4111-8111-111111111111",
		JobID:    "alert-action-22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", AlertID: "alert-1", ActionID: "block-ip",
		Action: "block_ip", Target: "198.51.100.10", Reason: "test", TraceID: "trace-test",
		DryRun: true, AggregateVersion: 1, KafkaPartition: 1, KafkaOffset: 9,
	}
	expectNoCommittedAlertResponseReceipt(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_execution_receipts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertResponseProjectionBlocksLegacyRealEvent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := AlertResponseProjectionInput{
		EventID:  "11111111-1111-4111-8111-111111111111",
		JobID:    "alert-action-22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", AlertID: "alert-1", ActionID: "block-ip",
		Action: "block_ip", Target: "198.51.100.10", Reason: "test", TraceID: "trace-test",
		DryRun: false, AggregateVersion: 1, KafkaPartition: 1, KafkaOffset: 10,
	}
	expectNoCommittedAlertResponseReceipt(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_execution_receipts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertResponseProjectionRejectsAuthoritativePayloadMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := AlertResponseProjectionInput{
		EventID:  "11111111-1111-4111-8111-111111111111",
		JobID:    "alert-action-22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", AlertID: "alert-1", ActionID: "block-ip",
		Action: "block_ip", Target: "198.51.100.10", Reason: "test",
		RequestedBy: "operator-a", TraceID: "trace-test", DryRun: true, AggregateVersion: 1, KafkaPartition: 1, KafkaOffset: 9,
	}
	expectNoCommittedAlertResponseReceipt(mock)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO alert_response_execution_receipts").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE alert_response_actions").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err == nil {
		t.Fatal("expected authoritative payload mismatch to fail closed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresAlertResponseProjectionExactReplayAtNewOffsetIsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	input := AlertResponseProjectionInput{
		EventID:  "11111111-1111-4111-8111-111111111111",
		JobID:    "alert-action-22222222-2222-4222-8222-222222222222",
		TenantID: "tenant-a", AlertID: "alert-1", ActionID: "block-ip",
		Action: "block_ip", Target: "198.51.100.10", Reason: "test", TraceID: "trace-test",
		DryRun: true, AggregateVersion: 1, KafkaPartition: 2, KafkaOffset: 99,
	}
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"receipt_exists", "exact"}).AddRow(true, true),
	)
	projection, err := NewPostgresAlertResponseProjection(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := projection.ApplyAlertResponseProjection(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectNoCommittedAlertResponseReceipt(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"receipt_exists", "exact"}).AddRow(false, false),
	)
}
