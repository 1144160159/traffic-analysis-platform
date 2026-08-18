package kafka

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type GenerationConsumerConfig struct {
	Brokers          []string
	Topic            string
	GroupID          string
	StartOffset      int64
	RebalanceTimeout time.Duration
	Security         SecurityConfig
}

type GroupLifecycleState string

const (
	GroupLifecycleAssigned GroupLifecycleState = "ASSIGNED"
	GroupLifecycleReady    GroupLifecycleState = "READY"
	GroupLifecycleRevoked  GroupLifecycleState = "REVOKED"
	GroupLifecycleStopped  GroupLifecycleState = "STOPPED"
)

type GroupPartitionAssignment struct {
	Topic     string
	Partition int
	Offset    int64
}

type GroupLifecycleReceipt struct {
	Topic        string
	GroupID      string
	MemberID     string
	GenerationID int32
	OwnerID      string
	OwnerEpoch   int64
	State        GroupLifecycleState
	Assignments  []GroupPartitionAssignment
	ObservedAt   time.Time
}

type GroupLifecycleObserver func(context.Context, GroupLifecycleReceipt) error

type GenerationMessageHandler func(
	context.Context,
	*segmentkafka.Generation,
	string,
	segmentkafka.PartitionAssignment,
) error

type consumerGroupRuntime interface {
	Next(context.Context) (*segmentkafka.Generation, error)
	Close() error
}

type GenerationConsumer struct {
	config     GenerationConsumerConfig
	group      consumerGroupRuntime
	logger     *zap.Logger
	ownerID    string
	ownerEpoch int64
	start      func(*segmentkafka.Generation, func(context.Context))

	mu       sync.Mutex
	observer GroupLifecycleObserver
	running  bool
	closed   bool
	last     GroupLifecycleReceipt
	revoked  bool
}

func NewGenerationConsumer(
	config GenerationConsumerConfig,
	logger *zap.Logger,
) (*GenerationConsumer, error) {
	if len(config.Brokers) == 0 || strings.TrimSpace(config.Topic) == "" ||
		strings.TrimSpace(config.GroupID) == "" {
		return nil, fmt.Errorf("generation consumer brokers topic and group are required")
	}
	if config.StartOffset == 0 {
		config.StartOffset = segmentkafka.FirstOffset
	}
	if config.StartOffset != segmentkafka.FirstOffset && config.StartOffset != segmentkafka.LastOffset {
		return nil, fmt.Errorf("generation consumer start offset is invalid")
	}
	if config.RebalanceTimeout <= 0 {
		config.RebalanceTimeout = 30 * time.Second
	}
	dialer, err := config.Security.Dialer("traffic-generation-consumer")
	if err != nil {
		return nil, err
	}
	group, err := segmentkafka.NewConsumerGroup(segmentkafka.ConsumerGroupConfig{
		ID: config.GroupID, Brokers: config.Brokers, Topics: []string{config.Topic},
		Dialer: dialer, GroupBalancers: []segmentkafka.GroupBalancer{segmentkafka.RoundRobinGroupBalancer{}},
		RebalanceTimeout: config.RebalanceTimeout, StartOffset: config.StartOffset,
	})
	if err != nil {
		return nil, fmt.Errorf("create Kafka generation consumer group: %w", err)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GenerationConsumer{
		config: config, group: group, logger: logger, ownerID: uuid.NewString(),
		ownerEpoch: time.Now().UTC().UnixMicro(),
	}, nil
}

// SetGroupLifecycleObserver installs the sole lifecycle authority callback.
// It must be registered before Run so constructor/startup cannot invent ready.
func (consumer *GenerationConsumer) SetGroupLifecycleObserver(observer GroupLifecycleObserver) error {
	if consumer == nil || observer == nil {
		return fmt.Errorf("generation group lifecycle observer is required")
	}
	consumer.mu.Lock()
	defer consumer.mu.Unlock()
	if consumer.running || consumer.closed {
		return fmt.Errorf("generation lifecycle observer must be installed before Run")
	}
	if consumer.observer != nil {
		return fmt.Errorf("generation lifecycle observer is already installed")
	}
	consumer.observer = observer
	return nil
}

func (consumer *GenerationConsumer) Run(ctx context.Context, handler GenerationMessageHandler) (runErr error) {
	if consumer == nil || consumer.group == nil || handler == nil {
		return fmt.Errorf("generation consumer runtime and handler are required")
	}
	consumer.mu.Lock()
	if consumer.running || consumer.closed {
		consumer.mu.Unlock()
		return fmt.Errorf("generation consumer can only Run once")
	}
	consumer.running = true
	consumer.mu.Unlock()

	defer func() {
		_ = consumer.group.Close()
		consumer.mu.Lock()
		consumer.running = false
		consumer.closed = true
		stopped := consumer.last
		revoked := consumer.revoked
		consumer.mu.Unlock()
		if stopped.Topic == "" || !revoked {
			return
		}
		stopped.State = GroupLifecycleStopped
		stopped.ObservedAt = time.Now().UTC()
		stopCtx, stopCancel := consumer.lifecycleContext()
		defer stopCancel()
		if stopErr := consumer.emitGroupLifecycle(stopCtx, stopped); stopErr != nil && runErr == nil {
			runErr = fmt.Errorf("emit stopped generation lifecycle: %w", stopErr)
		}
	}()

	for {
		generation, err := consumer.group.Next(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, segmentkafka.ErrGroupClosed) {
				return ctx.Err()
			}
			return fmt.Errorf("join Kafka consumer generation: %w", err)
		}
		assignments := normalizedGroupAssignments(generation)
		if len(assignments) == 0 {
			_ = consumer.group.Close()
			return fmt.Errorf("Kafka generation %d has no assignment for topic %s", generation.ID, consumer.config.Topic)
		}
		epoch := atomic.AddInt64(&consumer.ownerEpoch, 1)
		base := GroupLifecycleReceipt{
			Topic: consumer.config.Topic, GroupID: generation.GroupID,
			MemberID: generation.MemberID, GenerationID: generation.ID,
			OwnerID: consumer.ownerID, OwnerEpoch: epoch,
			Assignments: assignments, ObservedAt: time.Now().UTC(),
		}
		consumer.mu.Lock()
		consumer.last = base
		consumer.revoked = false
		consumer.mu.Unlock()
		assigned := base
		assigned.State = GroupLifecycleAssigned
		if err := consumer.emitGroupLifecycle(ctx, assigned); err != nil {
			_ = consumer.group.Close()
			return fmt.Errorf("emit assigned generation lifecycle: %w", err)
		}

		results := make(chan error, len(assignments))
		for _, assignedPartition := range generation.Assignments[consumer.config.Topic] {
			assignment := assignedPartition
			consumer.startGeneration(generation, func(generationContext context.Context) {
				results <- handler(generationContext, generation, consumer.config.Topic, assignment)
			})
		}
		ready := base
		ready.State = GroupLifecycleReady
		ready.ObservedAt = time.Now().UTC()
		if err := consumer.emitGroupLifecycle(ctx, ready); err != nil {
			_ = consumer.group.Close()
			return fmt.Errorf("emit ready generation lifecycle: %w", err)
		}

		var firstErr error
		select {
		case firstErr = <-results:
		case <-ctx.Done():
			firstErr = ctx.Err()
		}
		revoked := base
		revoked.State = GroupLifecycleRevoked
		revoked.ObservedAt = time.Now().UTC()
		revokeCtx, revokeCancel := consumer.lifecycleContext()
		revokeErr := consumer.emitGroupLifecycle(revokeCtx, revoked)
		revokeCancel()
		if revokeErr != nil {
			firstErr = errors.Join(
				firstErr,
				fmt.Errorf("emit revoked generation lifecycle: %w", revokeErr),
			)
		}
		if revokeErr == nil {
			consumer.mu.Lock()
			consumer.revoked = true
			consumer.mu.Unlock()
		}
		// Revoke is externally visible before Close asks every partition handler
		// to drain. This closes admission during the bounded drain window.
		if ctx.Err() != nil || revokeErr != nil ||
			(firstErr != nil && !errors.Is(firstErr, segmentkafka.ErrGenerationEnded)) {
			_ = consumer.group.Close()
		}
		for remaining := 1; remaining < len(assignments); remaining++ {
			select {
			case handlerErr := <-results:
				if firstErr == nil && handlerErr != nil {
					firstErr = handlerErr
				}
			case <-ctx.Done():
				_ = consumer.group.Close()
			}
		}
		if firstErr != nil && !errors.Is(firstErr, segmentkafka.ErrGenerationEnded) &&
			!errors.Is(firstErr, context.Canceled) {
			return firstErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (consumer *GenerationConsumer) startGeneration(
	generation *segmentkafka.Generation,
	handler func(context.Context),
) {
	if consumer.start != nil {
		consumer.start(generation, handler)
		return
	}
	generation.Start(handler)
}

func (consumer *GenerationConsumer) lifecycleContext() (context.Context, context.CancelFunc) {
	timeout := consumer.config.RebalanceTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
}

func (consumer *GenerationConsumer) Close() error {
	if consumer == nil || consumer.group == nil {
		return nil
	}
	return consumer.group.Close()
}

func (consumer *GenerationConsumer) emitGroupLifecycle(
	ctx context.Context,
	receipt GroupLifecycleReceipt,
) error {
	consumer.mu.Lock()
	observer := consumer.observer
	consumer.mu.Unlock()
	if observer == nil {
		return nil
	}
	return observer(ctx, receipt)
}

func normalizedGroupAssignments(generation *segmentkafka.Generation) []GroupPartitionAssignment {
	if generation == nil {
		return nil
	}
	result := make([]GroupPartitionAssignment, 0)
	for topic, assignments := range generation.Assignments {
		for _, assignment := range assignments {
			result = append(result, GroupPartitionAssignment{
				Topic: topic, Partition: assignment.ID, Offset: assignment.Offset,
			})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Topic != result[right].Topic {
			return result[left].Topic < result[right].Topic
		}
		return result[left].Partition < result[right].Partition
	})
	return result
}

type AssignedPartitionFetcher func(
	context.Context,
	string,
	segmentkafka.PartitionAssignment,
	func(segmentkafka.Message) error,
) error

type GenerationErrorClassifier func(error) bool

type GenerationDLQProducer interface {
	Send(context.Context, *ReceivedMessage, error) error
}

type GenerationCommitOffsets func(
	*segmentkafka.Generation,
	map[string]map[int]int64,
) error

type GenerationCommitObserver func(*ReceivedMessage, int64)

type GenerationMessageProcessorConfig struct {
	AssignedPartitionFetcher  AssignedPartitionFetcher
	ErrorClassifier           GenerationErrorClassifier
	DLQProducer               GenerationDLQProducer
	DLQAcknowledgementBarrier DLQAcknowledgementBarrier
	CommitOffsets             GenerationCommitOffsets
	CommitObserver            GenerationCommitObserver
}

type GenerationMessageProcessor struct {
	config GenerationMessageProcessorConfig
}

type CommitOutcomeUnknownError struct {
	Topic     string
	Partition int
	Offset    int64
	Cause     error
}

func (err *CommitOutcomeUnknownError) Error() string {
	if err == nil {
		return "Kafka offset commit outcome is unknown"
	}
	return fmt.Sprintf(
		"Kafka offset commit outcome is unknown for %s/%d offset %d: %v",
		err.Topic, err.Partition, err.Offset, err.Cause,
	)
}

func (err *CommitOutcomeUnknownError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func NewGenerationMessageProcessor(
	config GenerationMessageProcessorConfig,
) (*GenerationMessageProcessor, error) {
	if config.AssignedPartitionFetcher == nil || config.ErrorClassifier == nil ||
		config.DLQProducer == nil || config.DLQAcknowledgementBarrier == nil ||
		config.CommitOffsets == nil || config.CommitObserver == nil {
		return nil, fmt.Errorf("generation message processor requires fetch classifier DLQ barrier commit and observer dependencies")
	}
	return &GenerationMessageProcessor{config: config}, nil
}

func (processor *GenerationMessageProcessor) ProcessPartition(
	ctx context.Context,
	generation *segmentkafka.Generation,
	topic string,
	assignment segmentkafka.PartitionAssignment,
	handler MessageHandler,
) error {
	if processor == nil || generation == nil || handler == nil || strings.TrimSpace(topic) == "" || assignment.ID < 0 {
		return fmt.Errorf("generation partition processor inputs are invalid")
	}
	return processor.config.AssignedPartitionFetcher(ctx, topic, assignment, func(message segmentkafka.Message) error {
		if message.Topic == "" {
			message.Topic = topic
		}
		if message.Topic != topic || message.Partition != assignment.ID || message.Offset < assignment.Offset {
			return fmt.Errorf("assigned partition fetch returned a foreign Kafka record")
		}
		received := &ReceivedMessage{Message: message}
		messageContext := received.Context(ctx)
		if err := handler(messageContext, received); err != nil {
			if !processor.config.ErrorClassifier(err) {
				return err
			}
			if dlqErr := processor.config.DLQProducer.Send(messageContext, received, err); dlqErr != nil {
				return fmt.Errorf("publish generation record to DLQ: %w", dlqErr)
			}
			if barrierErr := processor.config.DLQAcknowledgementBarrier(messageContext, received, err); barrierErr != nil {
				return fmt.Errorf("persist generation DLQ acknowledgement: %w", barrierErr)
			}
		}
		nextOffset := message.Offset + 1
		if err := processor.config.CommitOffsets(generation, map[string]map[int]int64{
			topic: {assignment.ID: nextOffset},
		}); err != nil {
			return &CommitOutcomeUnknownError{
				Topic: topic, Partition: assignment.ID, Offset: nextOffset, Cause: err,
			}
		}
		processor.config.CommitObserver(received, nextOffset)
		return nil
	})
}

func NewAssignedPartitionFetcher(
	brokers []string,
	security SecurityConfig,
) (AssignedPartitionFetcher, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("assigned partition fetcher brokers are required")
	}
	dialer, err := security.Dialer("traffic-generation-partition-fetcher")
	if err != nil {
		return nil, err
	}
	return func(
		ctx context.Context,
		topic string,
		assignment segmentkafka.PartitionAssignment,
		handle func(segmentkafka.Message) error,
	) error {
		reader := segmentkafka.NewReader(segmentkafka.ReaderConfig{
			Brokers: brokers, Topic: topic, Partition: assignment.ID, Dialer: dialer,
			MinBytes: 1, MaxBytes: 10 * 1024 * 1024, MaxWait: 500 * time.Millisecond,
		})
		defer reader.Close()
		if err := reader.SetOffset(assignment.Offset); err != nil {
			return fmt.Errorf("seek assigned Kafka partition: %w", err)
		}
		for {
			message, err := reader.FetchMessage(ctx)
			if err != nil {
				return err
			}
			if err := handle(message); err != nil {
				return err
			}
		}
	}, nil
}
