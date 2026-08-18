package control

import (
	"context"
	"errors"
	"testing"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/protobuf/proto"
)

type fakeReadinessKeyedProducer struct {
	topic    string
	keys     []string
	payloads [][]byte
	headers  [][]commonkafka.MessageHeader
	err      error
}

func (producer *fakeReadinessKeyedProducer) Topic() string { return producer.topic }

func (producer *fakeReadinessKeyedProducer) Send(
	_ context.Context,
	key string,
	payload []byte,
	headers ...commonkafka.MessageHeader,
) (commonkafka.BrokerReceipt, error) {
	producer.keys = append(producer.keys, key)
	producer.payloads = append(producer.payloads, append([]byte(nil), payload...))
	producer.headers = append(producer.headers, append([]commonkafka.MessageHeader(nil), headers...))
	return commonkafka.BrokerReceipt{Topic: producer.topic, Key: key, Partition: 1, Offset: 8}, producer.err
}

func TestProbeControlReadinessPublisherEnvelope(t *testing.T) {
	producer := &fakeReadinessKeyedProducer{topic: ProbeGroupReadinessTopic}
	publisher, err := NewProbeControlReadinessPublisher(
		producer, "11111111-1111-4111-8111-111111111111",
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 13, 0, 0, 0, time.UTC)
	publisher.now = func() time.Time { return now }
	generation := commonkafka.GroupLifecycleReceipt{
		Topic: "probe.control.v2", GroupID: "ingest-gateway-probe-control-v2",
		MemberID: "member-a", GenerationID: 7, OwnerID: "owner-a", OwnerEpoch: 42,
		State: commonkafka.GroupLifecycleReady,
	}
	receipt, err := publisher.Publish(context.Background(), generation, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Topic != ProbeGroupReadinessTopic || receipt.Key != generation.GroupID ||
		len(producer.payloads) != 1 || producer.keys[0] != generation.GroupID {
		t.Fatalf("unexpected broker publication: receipt=%#v keys=%v", receipt, producer.keys)
	}
	var wire pb.ProbeGroupReadinessReceiptV1
	if err := proto.Unmarshal(producer.payloads[0], &wire); err != nil {
		t.Fatal(err)
	}
	if wire.ConsumerGroup != generation.GroupID || wire.ObservedTopic != generation.Topic ||
		wire.GenerationId != 7 || wire.OwnerEpoch != 42 ||
		wire.State != pb.ProbeGroupReadinessStateV1_PROBE_GROUP_READINESS_STATE_V1_READY ||
		wire.ObservedAtMs != now.UnixMilli() || wire.ExpiresAtMs != now.Add(time.Minute).UnixMilli() {
		t.Fatalf("unexpected readiness wire receipt: %#v", &wire)
	}
	headers := make(map[string]string)
	for _, header := range producer.headers[0] {
		headers[header.Key] = header.Value
	}
	if headers["event_id"] != wire.ReceiptId || headers["consumer_group"] != wire.ConsumerGroup ||
		headers["source_service"] != ProbeReadinessSourceService ||
		headers["proto_message_type"] != "traffic.v1.ProbeGroupReadinessReceiptV1" {
		t.Fatalf("unexpected readiness headers: %#v", headers)
	}
}

func TestProbeControlReadinessRenewalLifecycle(t *testing.T) {
	producer := &fakeReadinessKeyedProducer{topic: ProbeGroupReadinessTopic}
	publisher, err := NewProbeControlReadinessPublisher(
		producer, "11111111-1111-4111-8111-111111111111",
	)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 13, 0, 0, 0, time.UTC)
	publisher.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	ticks := make(chan time.Time, 2)
	ticks <- now
	ticks <- now
	ctx, cancel := context.WithCancel(context.Background())
	publisher.renewalTicker = func(time.Duration) (<-chan time.Time, func()) {
		return ticks, func() {}
	}
	generation := commonkafka.GroupLifecycleReceipt{
		Topic: "probe.control.v2", GroupID: "ingest-gateway-probe-control-v2",
		MemberID: "member-a", GenerationID: 7, OwnerID: "owner-a", OwnerEpoch: 42,
		State: commonkafka.GroupLifecycleReady,
	}
	done := make(chan error, 1)
	go func() { done <- publisher.RunRenewal(ctx, generation, 3*time.Second) }()
	deadline := time.After(time.Second)
	for len(producer.payloads) < 2 {
		select {
		case <-deadline:
			t.Fatal("renewal did not publish twice")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("renewal err=%v, want cancellation", err)
	}
}

func TestProbeControlReadinessPublishRejectsInventedIdentity(t *testing.T) {
	producer := &fakeReadinessKeyedProducer{topic: ProbeGroupReadinessTopic}
	publisher, err := NewProbeControlReadinessPublisher(
		producer, "11111111-1111-4111-8111-111111111111",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(context.Background(), commonkafka.GroupLifecycleReceipt{
		Topic: "probe.control.v2", GroupID: "group", State: commonkafka.GroupLifecycleReady,
	}, time.Minute); err == nil {
		t.Fatal("incomplete broker generation identity was accepted")
	}
	if len(producer.payloads) != 0 {
		t.Fatal("invalid readiness receipt reached Kafka")
	}
}
