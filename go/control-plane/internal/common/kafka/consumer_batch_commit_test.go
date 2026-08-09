package kafka

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	segmentKafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func TestHandleAndCommitBatchDoesNotReportProcessedBeforeBrokerAck(t *testing.T) {
	commitErr := errors.New("coordinator unavailable")
	consumer := &Consumer{
		logger: zap.NewNop(),
		commitFunc: func(context.Context, ...segmentKafka.Message) error {
			return commitErr
		},
	}
	batch := []*ReceivedMessage{{Message: segmentKafka.Message{Topic: "events.v1", Partition: 0, Offset: 9}}}
	observerCalled := false
	consumer.SetCommitObserver(func([]segmentKafka.Message) { observerCalled = true })

	err := consumer.handleAndCommitBatch(context.Background(), batch, func(context.Context, []*ReceivedMessage) error {
		return nil
	})
	if !errors.Is(err, commitErr) {
		t.Fatalf("handleAndCommitBatch error=%v, want wrapped commit error", err)
	}
	if got := atomic.LoadInt64(&consumer.metrics.MessagesProcessed); got != 0 {
		t.Fatalf("MessagesProcessed=%d, want 0 before broker ack", got)
	}
	if got := atomic.LoadInt64(&consumer.metrics.CommitsFailed); got != 1 {
		t.Fatalf("CommitsFailed=%d, want 1", got)
	}
	if observerCalled {
		t.Fatal("commit observer called without broker acknowledgement")
	}
}

func TestHandleAndCommitBatchReportsProcessedAfterBrokerAck(t *testing.T) {
	consumer := &Consumer{
		logger: zap.NewNop(),
		commitFunc: func(_ context.Context, messages ...segmentKafka.Message) error {
			if len(messages) != 2 {
				t.Fatalf("committed messages=%d, want 2", len(messages))
			}
			return nil
		},
	}
	batch := []*ReceivedMessage{
		{Message: segmentKafka.Message{Topic: "events.v1", Partition: 0, Offset: 9}},
		{Message: segmentKafka.Message{Topic: "events.v1", Partition: 0, Offset: 10}},
	}

	if err := consumer.handleAndCommitBatch(context.Background(), batch, func(context.Context, []*ReceivedMessage) error {
		return nil
	}); err != nil {
		t.Fatalf("handleAndCommitBatch returned error: %v", err)
	}
	if got := atomic.LoadInt64(&consumer.metrics.MessagesProcessed); got != 2 {
		t.Fatalf("MessagesProcessed=%d, want 2", got)
	}
	if got := atomic.LoadInt64(&consumer.metrics.CommitsSucceeded); got != 1 {
		t.Fatalf("CommitsSucceeded=%d, want 1", got)
	}
}

func TestHandleAndCommitBatchRetainsOffsetsWhenDLQBarrierFails(t *testing.T) {
	commitCalls := 0
	consumer := &Consumer{
		logger: zap.NewNop(),
		config: ConsumerConfig{
			CommitOnDLQSuccess: true,
		},
		dlqProducer: &DLQProducer{
			initErr: errors.New("invalid DLQ TLS configuration"),
			logger:  zap.NewNop(),
		},
		commitFunc: func(context.Context, ...segmentKafka.Message) error {
			commitCalls++
			return nil
		},
	}
	batch := []*ReceivedMessage{{Message: segmentKafka.Message{Topic: "events.v1", Partition: 0, Offset: 9}}}

	err := consumer.handleAndCommitBatch(context.Background(), batch, func(context.Context, []*ReceivedMessage) error {
		return Permanent(errors.New("invalid event"))
	})
	if err == nil || !strings.Contains(err.Error(), "DLQ durability barrier failed") {
		t.Fatalf("handleAndCommitBatch error=%v, want DLQ durability error", err)
	}
	if commitCalls != 0 {
		t.Fatalf("commit calls=%d, want 0 after failed DLQ barrier", commitCalls)
	}
	if got := atomic.LoadInt64(&consumer.metrics.MessagesProcessed); got != 0 {
		t.Fatalf("MessagesProcessed=%d, want 0", got)
	}
}

func TestBackgroundCommitterRetainsOffsetsUntilBrokerAck(t *testing.T) {
	var attempts int64
	acknowledged := make(chan struct{})
	badPayload := make(chan []segmentKafka.Message, 1)
	consumer := &Consumer{
		logger:        zap.NewNop(),
		config:        ConsumerConfig{CommitInterval: time.Millisecond},
		commitChan:    make(chan commitRequest, 1),
		stopCommitter: make(chan struct{}),
		commitFunc: func(_ context.Context, messages ...segmentKafka.Message) error {
			if len(messages) != 1 || messages[0].Offset != 12 {
				badPayload <- append([]segmentKafka.Message(nil), messages...)
				return errors.New("unexpected commit payload")
			}
			if atomic.AddInt64(&attempts, 1) == 1 {
				return errors.New("temporary coordinator failure")
			}
			select {
			case <-acknowledged:
			default:
				close(acknowledged)
			}
			return nil
		},
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumer.backgroundCommitter()
	}()
	consumer.commitChan <- commitRequest{messages: []segmentKafka.Message{{Topic: "events.v1", Offset: 12}}}

	select {
	case <-acknowledged:
	case <-time.After(time.Second):
		close(consumer.stopCommitter)
		<-done
		t.Fatal("background committer did not retry the retained offset")
	}
	close(consumer.stopCommitter)
	<-done
	select {
	case payload := <-badPayload:
		t.Fatalf("commit payload=%v, want retained offset 12", payload)
	default:
	}
	if got := atomic.LoadInt64(&attempts); got < 2 {
		t.Fatalf("commit attempts=%d, want at least 2", got)
	}
}
