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

type fakeTopicActionProjectionApplier struct {
	inputs []api.TopicActionProjectionInput
	err    error
}

func (applier *fakeTopicActionProjectionApplier) ApplyTopicActionProjection(
	_ context.Context,
	input api.TopicActionProjectionInput,
) error {
	applier.inputs = append(applier.inputs, input)
	return applier.err
}

func topicActionKafkaMessage(t *testing.T, eventType string, revision int64) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":   "11111111-1111-4111-8111-111111111111",
		"event_type": eventType,
		"tenant_id":  "tenant-a",
		"topic":      "apt",
		"job_id":     "22222222-2222-4222-8222-222222222222",
		"action_id":  "export_snapshot",
		"revision":   revision,
		"trace_id":   "trace-a",
		"status":     "completed",
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "event_type": event["event_type"].(string),
		"tenant_id": event["tenant_id"].(string), "job_id": event["job_id"].(string),
		"aggregate_version": "3", "schema_version": "2",
		"target_topic": "traffic.topic.action.v2",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "traffic.topic.action.v2", Partition: 2, Offset: 19,
		Value: payload, Headers: headers,
	}}
}

func TestTopicActionEventConsumerAppliesValidatedResult(t *testing.T) {
	applier := &fakeTopicActionProjectionApplier{}
	consumer := &TopicActionEventConsumer{applier: applier}
	message := topicActionKafkaMessage(t, topicActionResultEvent, 3)

	if err := consumer.handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	if len(applier.inputs) != 1 {
		t.Fatalf("projection calls=%d want 1", len(applier.inputs))
	}
	input := applier.inputs[0]
	if input.TenantID != "tenant-a" || input.Topic != "apt" || input.Revision != 3 ||
		input.KafkaPartition != 2 || input.KafkaOffset != 19 {
		t.Fatalf("unexpected projection input: %#v", input)
	}
}

func TestTopicActionEventConsumerRejectsAggregateVersionMismatch(t *testing.T) {
	applier := &fakeTopicActionProjectionApplier{}
	consumer := &TopicActionEventConsumer{applier: applier}
	message := topicActionKafkaMessage(t, topicActionResultEvent, 4)

	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected aggregate version mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}

func TestTopicActionEventConsumerRejectsHeaderBodyIdentityMismatch(t *testing.T) {
	applier := &fakeTopicActionProjectionApplier{}
	consumer := &TopicActionEventConsumer{applier: applier}
	message := topicActionKafkaMessage(t, topicActionResultEvent, 3)
	for index := range message.Headers {
		if message.Headers[index].Key == "tenant_id" {
			message.Headers[index].Value = []byte("tenant-b")
		}
	}

	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected tenant identity mismatch")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("mismatched event reached projection applier")
	}
}

func TestTopicActionEventConsumerPropagatesProjectionFailure(t *testing.T) {
	applier := &fakeTopicActionProjectionApplier{err: errors.New("database unavailable")}
	consumer := &TopicActionEventConsumer{applier: applier}

	if err := consumer.handle(
		context.Background(),
		topicActionKafkaMessage(t, topicActionResultEvent, 3),
	); err == nil {
		t.Fatal("expected projection failure")
	}
}

func TestTopicActionEventConsumerRejectsTrailingJSON(t *testing.T) {
	applier := &fakeTopicActionProjectionApplier{}
	consumer := &TopicActionEventConsumer{applier: applier}
	message := topicActionKafkaMessage(t, topicActionResultEvent, 3)
	message.Value = append(message.Value, []byte(`{"unexpected":true}`)...)

	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
	if len(applier.inputs) != 0 {
		t.Fatal("invalid event reached projection applier")
	}
}
