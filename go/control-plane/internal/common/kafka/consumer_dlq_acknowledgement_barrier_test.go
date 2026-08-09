package kafka

import (
	"context"
	"errors"
	"strings"
	"testing"

	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestDLQAcknowledgementBarrierReceivesSourceIdentity(t *testing.T) {
	consumer := &Consumer{logger: zap.NewNop()}
	message := &ReceivedMessage{Message: segmentKafka.Message{
		Topic: "dashboard.task.events.v1", Partition: 2, Offset: 41,
	}}
	processingErr := Permanent(errors.New("invalid dashboard event"))
	consumer.SetDLQAcknowledgementBarrier(func(_ context.Context, got *ReceivedMessage, gotErr error) error {
		if got != message || got.Topic != "dashboard.task.events.v1" || got.Partition != 2 || got.Offset != 41 {
			t.Fatalf("unexpected DLQ source identity: %+v", got)
		}
		if !errors.Is(gotErr, processingErr) {
			t.Fatalf("processing error=%v want %v", gotErr, processingErr)
		}
		return nil
	})
	if err := consumer.runDLQAcknowledgementBarrier(context.Background(), message, processingErr); err != nil {
		t.Fatal(err)
	}
}

func TestDLQAcknowledgementBarrierFailureRetainsCommitAuthority(t *testing.T) {
	consumer := &Consumer{logger: zap.NewNop()}
	consumer.SetDLQAcknowledgementBarrier(func(context.Context, *ReceivedMessage, error) error {
		return errors.New("postgres unavailable")
	})
	err := consumer.runDLQAcknowledgementBarrier(context.Background(), &ReceivedMessage{}, errors.New("poison"))
	if err == nil || !strings.Contains(err.Error(), "persistence barrier failed") || !strings.Contains(err.Error(), "postgres unavailable") {
		t.Fatalf("barrier error=%v", err)
	}
}

func TestDLQAcknowledgementBarrierPanicFailsClosed(t *testing.T) {
	consumer := &Consumer{logger: zap.NewNop()}
	consumer.SetDLQAcknowledgementBarrier(func(context.Context, *ReceivedMessage, error) error {
		panic("receipt writer bug")
	})
	err := consumer.runDLQAcknowledgementBarrier(context.Background(), &ReceivedMessage{}, errors.New("poison"))
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("barrier panic error=%v", err)
	}
}
