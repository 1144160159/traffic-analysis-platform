package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type fakeConsumerGroupRuntime struct {
	mu          sync.Mutex
	generations []*segmentkafka.Generation
	nextCalls   int
	closed      bool
}

func (group *fakeConsumerGroupRuntime) Next(ctx context.Context) (*segmentkafka.Generation, error) {
	group.mu.Lock()
	if group.closed {
		group.mu.Unlock()
		return nil, segmentkafka.ErrGroupClosed
	}
	if group.nextCalls < len(group.generations) {
		generation := group.generations[group.nextCalls]
		group.nextCalls++
		group.mu.Unlock()
		return generation, nil
	}
	group.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestGenerationConsumerRejoinsAfterGenerationEnded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group := &fakeConsumerGroupRuntime{generations: []*segmentkafka.Generation{
		{ID: 7, GroupID: "probe-group", MemberID: "member-a", Assignments: map[string][]segmentkafka.PartitionAssignment{"probe.topic": {{ID: 0, Offset: 1}}}},
		{ID: 8, GroupID: "probe-group", MemberID: "member-b", Assignments: map[string][]segmentkafka.PartitionAssignment{"probe.topic": {{ID: 0, Offset: 2}}}},
	}}
	consumer := &GenerationConsumer{
		config: GenerationConsumerConfig{Topic: "probe.topic", GroupID: "probe-group"},
		group:  group, logger: zap.NewNop(), ownerID: "owner-a",
		start: func(_ *segmentkafka.Generation, handler func(context.Context)) { go handler(ctx) },
	}
	secondReady := make(chan struct{})
	if err := consumer.SetGroupLifecycleObserver(func(_ context.Context, receipt GroupLifecycleReceipt) error {
		if receipt.State == GroupLifecycleReady && receipt.GenerationID == 8 {
			close(secondReady)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(ctx, func(
			generationContext context.Context,
			generation *segmentkafka.Generation,
			_ string,
			_ segmentkafka.PartitionAssignment,
		) error {
			if generation.ID == 7 {
				return segmentkafka.ErrGenerationEnded
			}
			<-generationContext.Done()
			return generationContext.Err()
		})
	}()
	select {
	case <-secondReady:
	case <-time.After(time.Second):
		t.Fatal("consumer did not rejoin the successor generation")
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() err=%v", err)
	}
}

func TestGenerationConsumerSuppressesStoppedAfterRevokeFailure(t *testing.T) {
	group := &fakeConsumerGroupRuntime{generations: []*segmentkafka.Generation{{
		ID: 7, GroupID: "probe-group", MemberID: "member-a",
		Assignments: map[string][]segmentkafka.PartitionAssignment{"probe.topic": {{ID: 0, Offset: 1}}},
	}}}
	consumer := &GenerationConsumer{
		config: GenerationConsumerConfig{Topic: "probe.topic", GroupID: "probe-group"},
		group:  group, logger: zap.NewNop(), ownerID: "owner-a",
		start: func(_ *segmentkafka.Generation, handler func(context.Context)) {
			go handler(context.Background())
		},
	}
	states := []GroupLifecycleState{}
	if err := consumer.SetGroupLifecycleObserver(func(_ context.Context, receipt GroupLifecycleReceipt) error {
		states = append(states, receipt.State)
		if receipt.State == GroupLifecycleRevoked {
			return errors.New("readiness transport unavailable")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := consumer.Run(context.Background(), func(
		context.Context, *segmentkafka.Generation, string, segmentkafka.PartitionAssignment,
	) error {
		return errors.New("handler failed")
	})
	if err == nil {
		t.Fatal("revoke failure was hidden")
	}
	for _, state := range states {
		if state == GroupLifecycleStopped {
			t.Fatalf("false STOPPED emitted after revoke failure: %v", states)
		}
	}
}

func (group *fakeConsumerGroupRuntime) Close() error {
	group.mu.Lock()
	group.closed = true
	group.mu.Unlock()
	return nil
}

func TestGenerationConsumerLifecycleMatrix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	group := &fakeConsumerGroupRuntime{generations: []*segmentkafka.Generation{{
		ID: 7, GroupID: "probe-group", MemberID: "member-a",
		Assignments: map[string][]segmentkafka.PartitionAssignment{
			"probe.topic": {{ID: 2, Offset: 11}, {ID: 0, Offset: 4}},
		},
	}}}
	consumer := &GenerationConsumer{
		config: GenerationConsumerConfig{Topic: "probe.topic", GroupID: "probe-group"},
		group:  group, logger: zap.NewNop(), ownerID: "owner-a",
		start: func(_ *segmentkafka.Generation, handler func(context.Context)) { go handler(ctx) },
	}
	var mu sync.Mutex
	var receipts []GroupLifecycleReceipt
	readySeen := make(chan struct{})
	revokedSeen := make(chan struct{})
	handlersExited := 0
	if err := consumer.SetGroupLifecycleObserver(func(_ context.Context, receipt GroupLifecycleReceipt) error {
		mu.Lock()
		receipts = append(receipts, receipt)
		mu.Unlock()
		if receipt.State == GroupLifecycleReady {
			select {
			case <-readySeen:
			default:
				close(readySeen)
			}
		}
		if receipt.State == GroupLifecycleRevoked {
			mu.Lock()
			exitedBeforeRevoke := handlersExited
			mu.Unlock()
			if exitedBeforeRevoke != 0 {
				t.Errorf("revoke emitted after %d handlers exited", exitedBeforeRevoke)
			}
			select {
			case <-revokedSeen:
			default:
				close(revokedSeen)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(ctx, func(
			generationContext context.Context,
			_ *segmentkafka.Generation,
			_ string,
			_ segmentkafka.PartitionAssignment,
		) error {
			<-generationContext.Done()
			<-revokedSeen
			mu.Lock()
			handlersExited++
			mu.Unlock()
			return generationContext.Err()
		})
	}()
	select {
	case <-readySeen:
	case <-time.After(time.Second):
		t.Fatal("generation never became ready")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() err=%v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("generation Run did not stop")
	}
	mu.Lock()
	defer mu.Unlock()
	states := make([]GroupLifecycleState, len(receipts))
	for index, receipt := range receipts {
		states[index] = receipt.State
		if receipt.OwnerID != "owner-a" || receipt.OwnerEpoch != 1 {
			t.Fatalf("unexpected owner receipt: %#v", receipt)
		}
	}
	want := []GroupLifecycleState{
		GroupLifecycleAssigned, GroupLifecycleReady,
		GroupLifecycleRevoked, GroupLifecycleStopped,
	}
	if fmt.Sprint(states) != fmt.Sprint(want) {
		t.Fatalf("states=%v want=%v", states, want)
	}
	if len(receipts[0].Assignments) != 2 || receipts[0].Assignments[0].Partition != 0 ||
		receipts[0].Assignments[1].Partition != 2 {
		t.Fatalf("assignments not normalized: %#v", receipts[0].Assignments)
	}
	if !group.closed {
		t.Fatal("consumer group was not closed")
	}
}

type fakeGenerationDLQ struct {
	count int
	err   error
}

func (dlq *fakeGenerationDLQ) Send(context.Context, *ReceivedMessage, error) error {
	dlq.count++
	return dlq.err
}

func TestGenerationMessageProcessorDurabilityMatrix(t *testing.T) {
	permanentErr := Permanent(errors.New("invalid envelope"))
	retryableErr := errors.New("database unavailable")
	tests := []struct {
		name         string
		handlerErr   error
		dlqErr       error
		barrierErr   error
		commitErr    error
		wantDLQ      int
		wantBarrier  int
		wantCommit   int
		wantObserver int
		wantUnknown  bool
		wantErr      bool
	}{
		{name: "handler success commits", wantCommit: 1, wantObserver: 1},
		{name: "retryable has no external durability", handlerErr: retryableErr, wantErr: true},
		{name: "permanent durable quarantine commits", handlerErr: permanentErr, wantDLQ: 1, wantBarrier: 1, wantCommit: 1, wantObserver: 1},
		{name: "DLQ failure blocks commit", handlerErr: permanentErr, dlqErr: errors.New("DLQ down"), wantDLQ: 1, wantErr: true},
		{name: "barrier failure blocks commit", handlerErr: permanentErr, barrierErr: errors.New("receipt down"), wantDLQ: 1, wantBarrier: 1, wantErr: true},
		{name: "commit response loss is unknown", commitErr: errors.New("connection reset"), wantCommit: 1, wantUnknown: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dlq := &fakeGenerationDLQ{err: test.dlqErr}
			barrierCalls := 0
			commitCalls := 0
			observerCalls := 0
			processor, err := NewGenerationMessageProcessor(GenerationMessageProcessorConfig{
				AssignedPartitionFetcher: func(
					_ context.Context,
					topic string,
					assignment segmentkafka.PartitionAssignment,
					handle func(segmentkafka.Message) error,
				) error {
					return handle(segmentkafka.Message{
						Topic: topic, Partition: assignment.ID, Offset: 12,
						Key: []byte("key"), Value: []byte("value"),
					})
				},
				ErrorClassifier: IsPermanent,
				DLQProducer:     dlq,
				DLQAcknowledgementBarrier: func(context.Context, *ReceivedMessage, error) error {
					barrierCalls++
					return test.barrierErr
				},
				CommitOffsets: func(_ *segmentkafka.Generation, offsets map[string]map[int]int64) error {
					commitCalls++
					if offsets["probe.topic"][2] != 13 {
						t.Fatalf("commit offsets=%v", offsets)
					}
					return test.commitErr
				},
				CommitObserver: func(message *ReceivedMessage, offset int64) {
					observerCalls++
					if message.Offset != 12 || offset != 13 {
						t.Fatalf("observer message/offset=%d/%d", message.Offset, offset)
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = processor.ProcessPartition(
				context.Background(),
				&segmentkafka.Generation{ID: 1, GroupID: "group", MemberID: "member"},
				"probe.topic", segmentkafka.PartitionAssignment{ID: 2, Offset: 10},
				func(context.Context, *ReceivedMessage) error { return test.handlerErr },
			)
			if (err != nil) != test.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, test.wantErr)
			}
			var unknown *CommitOutcomeUnknownError
			if errors.As(err, &unknown) != test.wantUnknown {
				t.Fatalf("unknown=%v err=%v", test.wantUnknown, err)
			}
			if dlq.count != test.wantDLQ || barrierCalls != test.wantBarrier ||
				commitCalls != test.wantCommit || observerCalls != test.wantObserver {
				t.Fatalf(
					"dlq/barrier/commit/observer=%d/%d/%d/%d want %d/%d/%d/%d",
					dlq.count, barrierCalls, commitCalls, observerCalls,
					test.wantDLQ, test.wantBarrier, test.wantCommit, test.wantObserver,
				)
			}
		})
	}
}

func TestNewGenerationMessageProcessorRejectsMissingDurabilityDependencies(t *testing.T) {
	if _, err := NewGenerationMessageProcessor(GenerationMessageProcessorConfig{}); err == nil {
		t.Fatal("empty generation processor config was accepted")
	}
}
