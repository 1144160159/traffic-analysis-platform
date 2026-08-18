package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fakeProbeOperationProjectionApplier struct {
	inputs []api.ProbeOperationProjectionInput
	err    error
}

func (applier *fakeProbeOperationProjectionApplier) ApplyProbeOperationProjection(
	_ context.Context,
	input api.ProbeOperationProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func probeOperationEventMessage(t *testing.T, revision int64) *commonkafka.ReceivedMessage {
	return probeOperationEventMessageWithType(t, probeOperationAcknowledgedLifecycleEvent, revision)
}

func probeOperationEventMessageWithType(t *testing.T, eventType string, revision int64) *commonkafka.ReceivedMessage {
	t.Helper()
	status := "completed"
	if eventType == probeOperationExpiredLifecycleEvent {
		status = "expired"
	}
	event := map[string]interface{}{
		"event_id":   "11111111-1111-4111-8111-111111111111",
		"event_type": eventType,
		"tenant_id":  "tenant-a", "probe_id": "probe-a",
		"operation_id": "22222222-2222-4222-8222-222222222222",
		"revision":     revision, "status": status, "trace_id": "trace-a",
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "event_type": event["event_type"].(string),
		"tenant_id": event["tenant_id"].(string), "probe_id": event["probe_id"].(string),
		"operation_id":      event["operation_id"].(string),
		"aggregate_version": "3", "schema_version": "2", "target_topic": "probe.events.v2",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.events.v2", Key: []byte("tenant-a:probe-a"),
		Partition: 1, Offset: 9, Value: payload, Headers: headers,
	}}
}

func TestProbeOperationExpiredCompatibility(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	message := probeOperationEventMessageWithType(t, probeOperationExpiredLifecycleEvent, 3)
	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 || applier.inputs[0].EventType != probeOperationExpiredLifecycleEvent ||
		applier.inputs[0].Status != "expired" {
		t.Fatalf("unexpected expiry projection: %#v", applier.inputs)
	}
}

func TestProbeOperationExpiredProjectionMatrix(t *testing.T) {
	for _, eventType := range []string{
		probeOperationAcknowledgedLifecycleEvent,
		probeOperationExpiredLifecycleEvent,
	} {
		t.Run(eventType, func(t *testing.T) {
			applier := &fakeProbeOperationProjectionApplier{}
			consumer := &ProbeOperationEventConsumer{applier: applier}
			if err := consumer.handle(context.Background(), probeOperationEventMessageWithType(t, eventType, 3)); err != nil {
				t.Fatal(err)
			}
			if len(applier.inputs) != 1 || applier.inputs[0].EventType != eventType {
				t.Fatalf("projection inputs=%#v", applier.inputs)
			}
		})
	}
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	message := probeOperationEventMessageWithType(t, "traffic.probe.v2.OperationDeleted", 3)
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("unknown lifecycle event reached projection")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("unknown lifecycle event was applied")
	}
}

func TestProbeOperationEventConsumerAppliesValidatedEvent(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), probeOperationEventMessage(t, 3)); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want 1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.ProbeID != "probe-a" || input.Revision != 3 ||
		input.KafkaPartition != 1 || input.KafkaOffset != 9 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestProbeOperationEventConsumerRejectsVersionMismatch(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), probeOperationEventMessage(t, 4)); err == nil {
		t.Fatal("expected aggregate version mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestProbeOperationEventConsumerRejectsPartitionKeyMismatch(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	message := probeOperationEventMessage(t, 3)
	message.Key = []byte("tenant-a:probe-b")
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected partition key mismatch")
	}
}

func TestProbeOperationEventConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{err: errors.New("database unavailable")}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	if err := consumer.handle(context.Background(), probeOperationEventMessage(t, 3)); err == nil {
		t.Fatal("expected projection failure")
	}
}

func TestProbeOperationEventConsumerRejectsTrailingJSON(t *testing.T) {
	applier := &fakeProbeOperationProjectionApplier{}
	consumer := &ProbeOperationEventConsumer{applier: applier}
	message := probeOperationEventMessage(t, 3)
	message.Value = append(message.Value, []byte(`{"unexpected":true}`)...)
	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}
