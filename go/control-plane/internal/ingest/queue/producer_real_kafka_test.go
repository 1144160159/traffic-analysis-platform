package queue

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestFlowPartialAckProducerRealKafka(t *testing.T) {
	broker := os.Getenv("M02_PARTIAL_ACK_EPHEMERAL_KAFKA_BROKER")
	if broker == "" {
		t.Skip("requires owned ephemeral Kafka")
	}
	topic := os.Getenv("M02_PARTIAL_ACK_EPHEMERAL_FLOW_TOPIC")
	pcapTopic := os.Getenv("M02_PARTIAL_ACK_EPHEMERAL_PCAP_TOPIC")
	sessionTopic := os.Getenv("M02_PARTIAL_ACK_EPHEMERAL_SESSION_TOPIC")
	if topic == "" || pcapTopic == "" || sessionTopic == "" {
		t.Fatal("ephemeral topic contract is incomplete")
	}

	producer, err := NewProducer(ProducerConfig{
		Brokers:        []string{broker},
		FlowTopic:      topic,
		PcapIndexTopic: pcapTopic,
		SessionTopic:   sessionTopic,
		BatchSize:      3,
		BatchTimeout:   10 * time.Millisecond,
		Compression:    "none",
		RequiredAcks:   "all",
		MaxRetries:     3,
	}, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()

	events := []*pb.FlowEvent{
		realKafkaFlow("real-kafka-event-0"),
		realKafkaFlow("real-kafka-event-1"),
		realKafkaFlow("real-kafka-event-2"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := producer.WriteFlowEvents(ctx, events)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := result.ValidateExactSet(len(events)); err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Items {
		if item.Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED || item.AckScope != "KAFKA_RECORD" {
			t.Fatalf("item=%+v", item)
		}
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{broker},
		Topic:    topic,
		GroupID:  "m02-partial-ack-real-kafka-reader",
		MinBytes: 1,
		MaxBytes: 1 << 20,
	})
	defer reader.Close()
	consumed := make([]string, 0, len(events))
	for len(consumed) < len(events) {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		var event pb.FlowEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			t.Fatalf("decode: %v", err)
		}
		consumed = append(consumed, event.Header.EventId)
	}
	sort.Strings(consumed)
	for index, eventID := range consumed {
		if eventID != events[index].Header.EventId {
			t.Fatalf("consumed=%v", consumed)
		}
	}
}

func realKafkaFlow(eventID string) *pb.FlowEvent {
	return &pb.FlowEvent{
		Header:      &pb.EventHeader{TenantId: "tenant-real", ProbeId: "probe-real", EventId: eventID, EventTs: 1, IngestTs: 2},
		Tuple:       &pb.FiveTuple{SrcIp: "10.10.0.1", DstIp: "10.10.0.2", SrcPort: 12345, DstPort: 443, Protocol: 6},
		CommunityId: "1:real-kafka",
	}
}
