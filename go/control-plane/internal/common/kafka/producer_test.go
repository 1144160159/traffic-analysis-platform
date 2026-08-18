package kafka

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func keyedProducerTestConfig() ProducerConfig {
	return ProducerConfig{
		Brokers:      []string{"127.0.0.1:9092"},
		Topic:        "probe.control.v2",
		BatchSize:    1,
		MaxAttempts:  1,
		RequiredAcks: "all",
		Compression:  "none",
		Async:        false,
	}
}

func TestKeyedProducerRealBrokerReceipt(t *testing.T) {
	broker := strings.TrimSpace(os.Getenv("KEYED_PRODUCER_TEST_KAFKA_BROKER"))
	topic := strings.TrimSpace(os.Getenv("KEYED_PRODUCER_TEST_KAFKA_TOPIC"))
	if broker == "" || topic == "" {
		t.Skip("real Kafka broker/topic are not configured")
	}
	cfg := keyedProducerTestConfig()
	cfg.Brokers = []string{broker}
	cfg.Topic = topic
	producer, err := NewKeyedProducer(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	attemptID := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	key := "tenant-real:probe-real"
	payload := []byte(`{"receipt":"real"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	receipt, err := producer.Send(ctx, key, payload,
		MessageHeader{Key: PublishAttemptHeader, Value: attemptID},
		MessageHeader{Key: "event_id", Value: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.AttemptID != attemptID || receipt.Topic != topic || receipt.Key != key ||
		receipt.Partition < 0 || receipt.Offset < 0 || receipt.AcknowledgedAt.IsZero() {
		t.Fatalf("incomplete broker receipt: %#v", receipt)
	}
	connection, err := segmentkafka.DialLeader(ctx, "tcp", broker, topic, receipt.Partition)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Seek(receipt.Offset, 0); err != nil {
		t.Fatal(err)
	}
	message, err := connection.ReadMessage(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	if message.Offset != receipt.Offset || string(message.Key) != key || string(message.Value) != string(payload) ||
		kafkaMessageHeader(message.Headers, PublishAttemptHeader) != attemptID {
		t.Fatalf("broker record does not match receipt: receipt=%#v message=%#v", receipt, message)
	}
}

func TestKeyedProducerBrokerReceiptMatrix(t *testing.T) {
	producer := &Producer{config: ProducerConfig{Topic: "probe.control.v2"}, logger: zap.NewNop()}
	keyed := &KeyedProducer{producer: producer, pending: make(map[string]chan brokerCompletion)}
	keyed.send = func(_ context.Context, key string, _ []byte, headers ...MessageHeader) error {
		attemptID, err := publishAttemptID(headers)
		if err != nil {
			return err
		}
		var offset int64
		if _, err := fmt.Sscanf(key, "tenant:probe-%d", &offset); err != nil {
			return err
		}
		time.Sleep(time.Duration(20-offset%20) * time.Microsecond)
		keyed.complete([]segmentkafka.Message{{
			Topic: "probe.control.v2", Partition: int(offset % 7), Offset: 1000 + offset,
			Key: []byte(key), Headers: []segmentkafka.Header{{Key: PublishAttemptHeader, Value: []byte(attemptID)}},
		}}, nil)
		return nil
	}

	const count = 40
	var wg sync.WaitGroup
	errorsChannel := make(chan error, count)
	for index := 0; index < count; index++ {
		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("tenant:probe-%d", index)
			attemptID := fmt.Sprintf("00000000-0000-4000-8000-%012d", index)
			receipt, err := keyed.Send(context.Background(), key, []byte("payload"),
				MessageHeader{Key: PublishAttemptHeader, Value: attemptID})
			if err != nil {
				errorsChannel <- err
				return
			}
			if receipt.AttemptID != attemptID || receipt.Key != key ||
				receipt.Topic != "probe.control.v2" || receipt.Partition != index%7 ||
				receipt.Offset != int64(1000+index) || receipt.AcknowledgedAt.IsZero() {
				errorsChannel <- fmt.Errorf("cross-wired receipt for index %d: %#v", index, receipt)
			}
		}()
	}
	wg.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Error(err)
	}
	if len(keyed.pending) != 0 {
		t.Fatalf("pending completions leaked: %d", len(keyed.pending))
	}

	keyed.send = func(context.Context, string, []byte, ...MessageHeader) error { return nil }
	receipt, err := keyed.Send(context.Background(), "tenant:probe-timeout", []byte("payload"))
	var unknown *PublishOutcomeUnknownError
	if !errors.As(err, &unknown) || receipt.AttemptID == "" || receipt.Partition != -1 || receipt.Offset != -1 {
		t.Fatalf("timeout receipt=%#v err=%v, want typed unknown", receipt, err)
	}
}

func TestNewKeyedProducerStablePartitionMatrix(t *testing.T) {
	partitions := []int{0, 1, 2, 3, 4, 5, 6}
	fixtures := map[string]int{
		"tenant-a:probe-a": 3,
		"tenant-a:probe-b": 2,
		"tenant-b:probe-a": 6,
		"tenant-z:probe-z": 0,
	}

	var baseline map[string]int
	for _, unrelatedConfig := range []string{"", "event_id", "tenant+probe", "not-a-balancer"} {
		cfg := keyedProducerTestConfig()
		cfg.IdempotentKey = unrelatedConfig
		producer, err := NewKeyedProducer(cfg, zap.NewNop())
		if err != nil {
			t.Fatalf("NewKeyedProducer(%q) error = %v", unrelatedConfig, err)
		}
		hash, ok := producer.producer.writer.Balancer.(*segmentkafka.Hash)
		if !ok {
			t.Fatalf("NewKeyedProducer(%q) balancer = %T, want *kafka.Hash", unrelatedConfig, producer.producer.writer.Balancer)
		}

		observed := make(map[string]int, len(fixtures))
		for key, want := range fixtures {
			partition := hash.Balance(segmentkafka.Message{Key: []byte(key)}, partitions...)
			if partition != want {
				t.Fatalf("key %q partition = %d, want %d", key, partition, want)
			}
			observed[key] = partition
		}
		if baseline == nil {
			baseline = observed
		} else {
			for key, want := range baseline {
				if observed[key] != want {
					t.Fatalf("IdempotentKey %q changed key %q partition from %d to %d", unrelatedConfig, key, want, observed[key])
				}
			}
		}
		if err := producer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}

func TestNewKeyedProducerRejectsWeakConfigAndEmptyKey(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProducerConfig)
	}{
		{name: "acks empty", mutate: func(cfg *ProducerConfig) { cfg.RequiredAcks = "" }},
		{name: "acks one", mutate: func(cfg *ProducerConfig) { cfg.RequiredAcks = "one" }},
		{name: "acks none", mutate: func(cfg *ProducerConfig) { cfg.RequiredAcks = "none" }},
		{name: "async", mutate: func(cfg *ProducerConfig) { cfg.Async = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := keyedProducerTestConfig()
			test.mutate(&cfg)
			producer, err := NewKeyedProducer(cfg, zap.NewNop())
			if !errors.Is(err, ErrWeakKeyedProducer) {
				t.Fatalf("NewKeyedProducer() error = %v, want ErrWeakKeyedProducer", err)
			}
			if producer != nil {
				t.Fatal("NewKeyedProducer() returned producer for weak config")
			}
		})
	}

	producer, err := NewKeyedProducer(keyedProducerTestConfig(), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	for _, key := range []string{"", " ", "\t\n"} {
		if _, err := producer.Send(context.Background(), key, []byte("payload")); !errors.Is(err, ErrEmptyKafkaMessageKey) {
			t.Fatalf("Send(%q) error = %v, want ErrEmptyKafkaMessageKey", key, err)
		}
	}
}
