package kafka

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const generationConsumerKafkaSentinel = "ephemeral-only"

// TestGenerationConsumerRealKafkaRebalanceAndOffset proves that the production
// runner obtains real member/generation identities, commits offset+1, revokes
// before a successor generation, and rejoins after a second member arrives.
// It deliberately requires an explicitly owned loopback broker so normal unit
// runs cannot mutate a shared or production Kafka cluster.
func TestGenerationConsumerRealKafkaRebalanceAndOffset(t *testing.T) {
	broker := strings.TrimSpace(os.Getenv("GENERATION_CONSUMER_EPHEMERAL_KAFKA_BROKER"))
	if broker == "" {
		t.Skip("GENERATION_CONSUMER_EPHEMERAL_KAFKA_BROKER is not configured")
	}
	if os.Getenv("GENERATION_CONSUMER_EPHEMERAL_KAFKA_SENTINEL") != generationConsumerKafkaSentinel {
		t.Fatal("explicit ephemeral Kafka sentinel is required")
	}
	host, _, err := net.SplitHostPort(broker)
	if err != nil || (host != "127.0.0.1" && host != "localhost") {
		t.Fatalf("ephemeral Kafka must use a loopback broker: %q", broker)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	topic := "generation-consumer-integration-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	groupID := "generation-consumer-group-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	createGenerationIntegrationTopic(t, ctx, broker, topic, 2)
	writeGenerationIntegrationMessage(t, ctx, broker, topic, 0, []byte("probe-a"), []byte("first"))

	first, firstEvents, firstCommits := startRealGenerationConsumer(t, ctx, broker, topic, groupID)
	firstReady := awaitGenerationLifecycle(t, firstEvents, GroupLifecycleReady, 0)
	if firstReady.MemberID == "" || firstReady.GenerationID <= 0 || len(firstReady.Assignments) != 2 {
		t.Fatalf("first broker assignment is incomplete: %#v", firstReady)
	}
	firstCommit := awaitGenerationCommit(t, firstCommits)
	if firstCommit.partition != 0 || firstCommit.offset != 1 {
		t.Fatalf("first commit=%#v want partition=0 offset=1", firstCommit)
	}
	awaitCommittedGenerationOffset(t, ctx, broker, groupID, topic, 0, 1)

	second, secondEvents, _ := startRealGenerationConsumer(t, ctx, broker, topic, groupID)
	secondReady := awaitGenerationLifecycle(t, secondEvents, GroupLifecycleReady, 0)
	if secondReady.MemberID == "" || secondReady.GenerationID <= firstReady.GenerationID || len(secondReady.Assignments) != 1 {
		t.Fatalf("second broker assignment=%#v first=%#v", secondReady, firstReady)
	}
	firstRevoked := awaitGenerationLifecycle(t, firstEvents, GroupLifecycleRevoked, firstReady.GenerationID)
	firstSuccessor := awaitGenerationLifecycle(t, firstEvents, GroupLifecycleReady, firstReady.GenerationID+1)
	if firstSuccessor.GenerationID != secondReady.GenerationID || firstSuccessor.OwnerEpoch <= firstReady.OwnerEpoch ||
		len(firstSuccessor.Assignments) != 1 {
		t.Fatalf("first successor=%#v second ready=%#v revoked=%#v", firstSuccessor, secondReady, firstRevoked)
	}

	cancel()
	awaitGenerationRunExit(t, first)
	awaitGenerationRunExit(t, second)
	awaitGenerationLifecycle(t, firstEvents, GroupLifecycleStopped, firstSuccessor.GenerationID)
	awaitGenerationLifecycle(t, secondEvents, GroupLifecycleStopped, secondReady.GenerationID)
}

type realGenerationCommit struct {
	partition int
	offset    int64
}

type realGenerationRuntime struct {
	done chan error
}

type realGenerationNoopDLQ struct{}

func (realGenerationNoopDLQ) Send(context.Context, *ReceivedMessage, error) error { return nil }

func startRealGenerationConsumer(
	t *testing.T,
	ctx context.Context,
	broker, topic, groupID string,
) (*realGenerationRuntime, <-chan GroupLifecycleReceipt, <-chan realGenerationCommit) {
	t.Helper()
	runner, err := NewGenerationConsumer(GenerationConsumerConfig{
		Brokers: []string{broker}, Topic: topic, GroupID: groupID,
		StartOffset: segmentkafka.FirstOffset, RebalanceTimeout: 10 * time.Second,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan GroupLifecycleReceipt, 32)
	if err := runner.SetGroupLifecycleObserver(func(_ context.Context, receipt GroupLifecycleReceipt) error {
		events <- receipt
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fetcher, err := NewAssignedPartitionFetcher([]string{broker}, SecurityConfig{})
	if err != nil {
		t.Fatal(err)
	}
	commits := make(chan realGenerationCommit, 16)
	processor, err := NewGenerationMessageProcessor(GenerationMessageProcessorConfig{
		AssignedPartitionFetcher:  fetcher,
		ErrorClassifier:           IsPermanent,
		DLQProducer:               realGenerationNoopDLQ{},
		DLQAcknowledgementBarrier: func(context.Context, *ReceivedMessage, error) error { return nil },
		CommitOffsets: func(generation *segmentkafka.Generation, offsets map[string]map[int]int64) error {
			return generation.CommitOffsets(offsets)
		},
		CommitObserver: func(message *ReceivedMessage, offset int64) {
			commits <- realGenerationCommit{partition: message.Partition, offset: offset}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &realGenerationRuntime{done: make(chan error, 1)}
	go func() {
		runtime.done <- runner.Run(ctx, func(
			generationContext context.Context,
			generation *segmentkafka.Generation,
			topic string,
			assignment segmentkafka.PartitionAssignment,
		) error {
			return processor.ProcessPartition(
				generationContext, generation, topic, assignment,
				func(context.Context, *ReceivedMessage) error { return nil },
			)
		})
	}()
	return runtime, events, commits
}

func awaitGenerationLifecycle(
	t *testing.T,
	events <-chan GroupLifecycleReceipt,
	state GroupLifecycleState,
	minimumGeneration int32,
) GroupLifecycleReceipt {
	t.Helper()
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case receipt := <-events:
			if receipt.State == state && receipt.GenerationID >= minimumGeneration {
				return receipt
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for lifecycle state=%s at or after generation=%d", state, minimumGeneration)
		}
	}
}

func awaitGenerationCommit(t *testing.T, commits <-chan realGenerationCommit) realGenerationCommit {
	t.Helper()
	select {
	case commit := <-commits:
		return commit
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for a real generation commit")
		return realGenerationCommit{}
	}
}

func awaitGenerationRunExit(t *testing.T, runtime *realGenerationRuntime) {
	t.Helper()
	select {
	case err := <-runtime.done:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, segmentkafka.ErrGroupClosed) {
			t.Fatalf("generation runtime exit error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("generation runtime did not stop")
	}
}

func createGenerationIntegrationTopic(
	t *testing.T,
	ctx context.Context,
	broker, topic string,
	partitions int,
) {
	t.Helper()
	client := &segmentkafka.Client{Addr: segmentkafka.TCP(broker)}
	response, err := client.CreateTopics(ctx, &segmentkafka.CreateTopicsRequest{Topics: []segmentkafka.TopicConfig{{
		Topic: topic, NumPartitions: partitions, ReplicationFactor: 1,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if topicErr := response.Errors[topic]; topicErr != nil {
		t.Fatalf("create integration topic: %v", topicErr)
	}
}

func writeGenerationIntegrationMessage(
	t *testing.T,
	ctx context.Context,
	broker, topic string,
	partition int,
	key, value []byte,
) {
	t.Helper()
	connection, err := segmentkafka.DialLeader(ctx, "tcp", broker, topic, partition)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.WriteMessages(segmentkafka.Message{Key: key, Value: value}); err != nil {
		t.Fatal(err)
	}
}

func awaitCommittedGenerationOffset(
	t *testing.T,
	ctx context.Context,
	broker, groupID, topic string,
	partition int,
	want int64,
) {
	t.Helper()
	client := &segmentkafka.Client{Addr: segmentkafka.TCP(broker)}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.OffsetFetch(ctx, &segmentkafka.OffsetFetchRequest{
			GroupID: groupID, Topics: map[string][]int{topic: {partition}},
		})
		if err == nil && response.Error == nil {
			for _, offset := range response.Topics[topic] {
				if offset.Partition == partition && offset.Error == nil && offset.CommittedOffset == want {
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("committed offset for %s/%d did not become %d", topic, partition, want)
}

func (commit realGenerationCommit) String() string {
	return fmt.Sprintf("partition=%d offset=%d", commit.partition, commit.offset)
}
