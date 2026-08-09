package consumer

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type playbookProjectionCapture struct {
	input *api.PlaybookExecutionEventProjectionInput
}

func (capture *playbookProjectionCapture) ApplyPlaybookExecutionEventProjection(_ context.Context, input api.PlaybookExecutionEventProjectionInput) error {
	capture.input = &input
	return nil
}

func TestPlaybookExecutionEventConsumerValidatesHeadersAndApplies(t *testing.T) {
	capture := &playbookProjectionCapture{}
	consumer := &PlaybookExecutionEventConsumer{applier: capture, expectedTopic: api.PlaybookExecutionEventTopic}
	eventID := "55555555-5555-4555-8555-555555555555"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.playbook.v2.ExecutionCompleted","tenant_id":"tenant-a","aggregate_type":"playbook_execution","aggregate_id":"execution-a","aggregate_version":3,"partition_key":"tenant-a:execution-a","schema_version":2,"execution_id":"execution-a","playbook_name":"isolate-host","playbook_version":3,"alert_id":"alert-a","status":"completed","approval_status":"approved","executor_status":"succeeded","trace_id":"trace-consumer"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: api.PlaybookExecutionEventTopic, Key: []byte("tenant-a:execution-a"), Value: payload,
		Partition: 2, Offset: 19,
	}}
	for key, value := range map[string]string{
		"event_id": eventID, "event_type": "traffic.playbook.v2.ExecutionCompleted", "tenant_id": "tenant-a",
		"aggregate_type": "playbook_execution", "aggregate_id": "execution-a", "aggregate_version": "3",
		"schema_version": "2", "trace_id": "trace-consumer", "target_topic": api.PlaybookExecutionEventTopic,
	} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatalf("handle() error = %v", err)
	}
	if capture.input == nil || capture.input.ExecutionID != "execution-a" ||
		capture.input.AggregateVersion != 3 || capture.input.KafkaPartition != 2 ||
		capture.input.KafkaOffset != 19 || capture.input.ExecutorStatus != "succeeded" {
		t.Fatalf("unexpected projection input: %#v", capture.input)
	}
}

func TestPlaybookExecutionEventConsumerRejectsHeaderBodyMismatch(t *testing.T) {
	capture := &playbookProjectionCapture{}
	consumer := &PlaybookExecutionEventConsumer{applier: capture, expectedTopic: api.PlaybookExecutionEventTopic}
	eventID := "66666666-6666-4666-8666-666666666666"
	payload := []byte(`{"event_id":"` + eventID + `","event_type":"traffic.playbook.v2.ExecutionApproved","tenant_id":"tenant-a","aggregate_type":"playbook_execution","aggregate_id":"execution-a","aggregate_version":2,"partition_key":"tenant-a:execution-a","schema_version":2,"execution_id":"execution-a","playbook_name":"isolate-host","status":"approved_awaiting_executor","trace_id":"trace-mismatch"}`)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: api.PlaybookExecutionEventTopic, Key: []byte("tenant-a:execution-a"), Value: payload,
		Partition: 0, Offset: 2,
	}}
	for key, value := range map[string]string{
		"event_id": eventID, "event_type": "traffic.playbook.v2.ExecutionApproved", "tenant_id": "wrong-tenant",
		"aggregate_type": "playbook_execution", "aggregate_id": "execution-a", "aggregate_version": "2",
		"schema_version": "2", "trace_id": "trace-mismatch", "target_topic": api.PlaybookExecutionEventTopic,
	} {
		message.Headers = append(message.Headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("header/body mismatch must fail closed")
	}
	if capture.input != nil {
		t.Fatal("mismatched event must not reach the projection applier")
	}
}
