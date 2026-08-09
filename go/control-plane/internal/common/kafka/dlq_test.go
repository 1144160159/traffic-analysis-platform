package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestResolveDLQTopicUsesRegisteredTarget(t *testing.T) {
	config := DLQConfig{TopicPrefix: "dlq.", TargetTopic: "dlq.v1"}
	if got := resolveDLQTopic(config, "traffic.topic.action.v2"); got != "dlq.v1" {
		t.Fatalf("resolved DLQ topic=%q want dlq.v1", got)
	}
}

func TestResolveDLQTopicPreservesLegacyPrefixMode(t *testing.T) {
	config := DLQConfig{TopicPrefix: "dlq."}
	if got := resolveDLQTopic(config, "probe.acks.v2"); got != "dlq.probe.acks.v2" {
		t.Fatalf("resolved DLQ topic=%q want dlq.probe.acks.v2", got)
	}
}

func TestDLQProducerSecurityInitializationFailsClosed(t *testing.T) {
	producer := NewDLQProducer(DLQConfig{
		Brokers:  []string{"kafka.invalid:9092"},
		Security: SecurityConfig{SecurityProtocol: "unsupported"},
	}, "test-service", zap.NewNop())
	t.Cleanup(func() { _ = producer.Close() })

	err := producer.Send(context.Background(), &ReceivedMessage{Message: segmentKafka.Message{Topic: "events.v1"}}, errors.New("handler failed"))
	if err == nil || !strings.Contains(err.Error(), "initialize Kafka DLQ security") {
		t.Fatalf("Send error=%v, want fail-closed security initialization error", err)
	}
}

func TestDLQBatchMarshalFailureRejectsWholeBatch(t *testing.T) {
	marshalCalls := 0
	producer := &DLQProducer{
		writer:      &segmentKafka.Writer{},
		config:      DLQConfig{TopicPrefix: "dlq.", MaxRetries: 1},
		serviceName: "test-service",
		hostname:    "test-host",
		logger:      zap.NewNop(),
		marshalFunc: func(value interface{}) ([]byte, error) {
			marshalCalls++
			if marshalCalls == 2 {
				return nil, errors.New("synthetic marshal failure")
			}
			return json.Marshal(value)
		},
	}
	valid := &ReceivedMessage{Message: segmentKafka.Message{Topic: "events.v1", Offset: 1}}
	invalid := &ReceivedMessage{Message: segmentKafka.Message{Topic: "events.v1", Offset: 2}}

	// A serialization failure must reject the whole batch before any Kafka write
	// can be attempted; silently dropping one item would allow a source commit.
	items := []struct {
		Msg *ReceivedMessage
		Err error
	}{
		{Msg: valid, Err: errors.New("first")},
		{Msg: invalid, Err: errors.New("second")},
	}
	err := producer.SendBatch(context.Background(), items)
	if err == nil || !strings.Contains(err.Error(), "marshal DLQ batch message") {
		t.Fatalf("SendBatch error=%v, want whole-batch marshal rejection", err)
	}
}
