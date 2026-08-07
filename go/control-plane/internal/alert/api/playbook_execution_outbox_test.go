package api

import (
	"context"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/DATA-DOG/go-sqlmock"
	"go.uber.org/zap"
)

func TestPlaybookExecutionOutboxMarksPublishedOnlyAfterKafkaAck(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	handler.playbookExecutionTopic = PlaybookExecutionEventTopic
	eventID := "11111111-1111-4111-8111-111111111111"
	handler.playbookExecutionPublish = func(_ context.Context, key string, _ []byte, headers ...commonkafka.MessageHeader) error {
		if key != "tenant-a:execution-a" {
			t.Fatalf("key=%q", key)
		}
		assertProbeHeader(t, headers, "event_id", eventID)
		assertProbeHeader(t, headers, "aggregate_type", "playbook_execution")
		assertProbeHeader(t, headers, "aggregate_version", "3")
		assertProbeHeader(t, headers, "target_topic", PlaybookExecutionEventTopic)
		assertProbeHeader(t, headers, "trace_id", "trace-playbook-1")
		return nil
	}
	item := playbookExecutionOutboxItem{
		OutboxID: 9, EventID: eventID, ExecutionID: "execution-a", TenantID: "tenant-a",
		PlaybookName: "isolate-host", EventType: "traffic.playbook.v2.ExecutionCompleted",
		SchemaVersion: 2, AggregateVersion: 3, PartitionKey: "tenant-a:execution-a", Attempts: 1,
		Payload: []byte(`{"event_id":"` + eventID + `","event_type":"traffic.playbook.v2.ExecutionCompleted","tenant_id":"tenant-a","aggregate_type":"playbook_execution","aggregate_id":"execution-a","aggregate_version":3,"partition_key":"tenant-a:execution-a","schema_version":2,"execution_id":"execution-a","playbook_name":"isolate-host","status":"completed","trace_id":"trace-playbook-1"}`),
	}
	mock.ExpectExec("UPDATE alert_playbook_execution_outbox").WithArgs(int64(9), "worker-a").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := handler.publishPlaybookExecutionOutboxItem(context.Background(), "worker-a", &item); err != nil {
		t.Fatalf("publishPlaybookExecutionOutboxItem() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPlaybookExecutionOutboxBrokerFailureTransitionsToDead(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(nil, nil, nil, nil, NewAdvancedRepository(db, zap.NewNop()))
	handler.playbookExecutionTopic = PlaybookExecutionEventTopic
	handler.playbookExecutionPublish = func(context.Context, string, []byte, ...commonkafka.MessageHeader) error {
		return errors.New("broker unavailable")
	}
	eventID := "22222222-2222-4222-8222-222222222222"
	item := playbookExecutionOutboxItem{
		OutboxID: 10, EventID: eventID, ExecutionID: "execution-b", TenantID: "tenant-a",
		PlaybookName: "isolate-host", EventType: "traffic.playbook.v2.ExecutionFailed",
		SchemaVersion: 2, AggregateVersion: 4, PartitionKey: "tenant-a:execution-b", Attempts: playbookOutboxMaxAttempts,
		Payload: []byte(`{"event_id":"` + eventID + `","event_type":"traffic.playbook.v2.ExecutionFailed","tenant_id":"tenant-a","aggregate_type":"playbook_execution","aggregate_id":"execution-b","aggregate_version":4,"partition_key":"tenant-a:execution-b","schema_version":2,"execution_id":"execution-b","playbook_name":"isolate-host","status":"failed","trace_id":"trace-playbook-2"}`),
	}
	mock.ExpectExec("UPDATE alert_playbook_execution_outbox").
		WithArgs(int64(10), "dead", "broker unavailable", "worker-b").WillReturnResult(sqlmock.NewResult(0, 1))
	err = handler.publishPlaybookExecutionOutboxItem(context.Background(), "worker-b", &item)
	if err == nil || err.Error() != "broker unavailable" {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePlaybookExecutionOutboxRejectsAggregateCollision(t *testing.T) {
	item := playbookExecutionOutboxItem{
		EventID: "33333333-3333-4333-8333-333333333333", ExecutionID: "execution-c", TenantID: "tenant-a",
		PlaybookName: "isolate-host", EventType: "traffic.playbook.v2.ExecutionApproved",
		SchemaVersion: 2, AggregateVersion: 2, PartitionKey: "tenant-a:execution-c",
		Payload: []byte(`{"event_id":"33333333-3333-4333-8333-333333333333","event_type":"traffic.playbook.v2.ExecutionApproved","tenant_id":"tenant-a","aggregate_type":"playbook_execution","aggregate_id":"wrong-execution","aggregate_version":2,"partition_key":"tenant-a:execution-c","schema_version":2,"execution_id":"execution-c","playbook_name":"isolate-host","status":"approved_awaiting_executor","trace_id":"trace-playbook-3"}`),
	}
	if err := validatePlaybookExecutionOutboxItem(&item); err == nil {
		t.Fatal("aggregate identity collision must fail closed")
	}
}
