package consumer

import (
	"context"
	"errors"
	"testing"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type stubAlertDLQProducer struct {
	err   error
	sends int
}

func (s *stubAlertDLQProducer) Send(context.Context, *commonkafka.ReceivedMessage, error) error {
	s.sends++
	return s.err
}

func (s *stubAlertDLQProducer) Close() error { return nil }

func malformedDetectionMessage() *commonkafka.ReceivedMessage {
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "detections", Partition: 2, Offset: 41, Value: []byte("not-a-detection"),
	}}
}

func TestAlertBatchDoesNotCrossCommitBarrierWhenDLQFails(t *testing.T) {
	dlq := &stubAlertDLQProducer{err: errors.New("DLQ unavailable")}
	consumer := &Consumer{dlqProducer: dlq, logger: zap.NewNop()}

	err := consumer.processBatch(context.Background(), []*commonkafka.ReceivedMessage{malformedDetectionMessage()})

	if err == nil {
		t.Fatal("processing error plus DLQ failure must stop offset commit")
	}
	if dlq.sends != 1 {
		t.Fatalf("DLQ sends=%d want 1", dlq.sends)
	}
}

func TestAlertBatchMayCommitAfterPoisonRecordIsDurablyDLQed(t *testing.T) {
	dlq := &stubAlertDLQProducer{}
	consumer := &Consumer{dlqProducer: dlq, logger: zap.NewNop()}

	err := consumer.processBatch(context.Background(), []*commonkafka.ReceivedMessage{malformedDetectionMessage()})

	if err != nil {
		t.Fatalf("durably DLQed poison record should satisfy commit barrier: %v", err)
	}
	if dlq.sends != 1 {
		t.Fatalf("DLQ sends=%d want 1", dlq.sends)
	}
}
