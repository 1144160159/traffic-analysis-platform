package queue

import (
	"context"
	"errors"
	"testing"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

func flowForWrite(eventID string) *pb.FlowEvent {
	return &pb.FlowEvent{
		Header: &pb.EventHeader{TenantId: "tenant-a", ProbeId: "probe-a", EventId: eventID, EventTs: 1},
		Tuple:  &pb.FiveTuple{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
	}
}

func TestWriteFlowEventsPreservesKafkaPerMessageResults(t *testing.T) {
	producer := &Producer{
		logger: zap.NewNop(),
		config: ProducerConfig{FlowTopic: "flows"},
		writeFlowBatch: func(_ context.Context, _ string, messages []kafkaCommon.Message) error {
			if len(messages) != 3 {
				t.Fatalf("messages=%d want 3", len(messages))
			}
			return kafka.WriteErrors{nil, errors.New("retry me"), nil}
		},
	}

	result, err := producer.WriteFlowEvents(context.Background(), []*pb.FlowEvent{
		flowForWrite("event-0"), flowForWrite("event-1"), flowForWrite("event-2"),
	})
	if err == nil {
		t.Fatal("expected aggregate write error")
	}
	if exactErr := result.ValidateExactSet(3); exactErr != nil {
		t.Fatalf("exact-set: %v", exactErr)
	}
	want := []pb.FlowItemDisposition{
		pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED,
		pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE,
		pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED,
	}
	for i, item := range result.Items {
		if item.InputIndex != i || item.EventID != "event-"+string(rune('0'+i)) || item.Disposition != want[i] {
			t.Fatalf("item[%d]=%+v want disposition %s", i, item, want[i])
		}
	}
}

func TestWriteFlowEventsClassifiesCanceledOutcomeUnknown(t *testing.T) {
	producer := &Producer{
		logger: zap.NewNop(),
		config: ProducerConfig{FlowTopic: "flows"},
		writeFlowBatch: func(_ context.Context, _ string, _ []kafkaCommon.Message) error {
			return context.DeadlineExceeded
		},
	}
	result, err := producer.WriteFlowEvents(context.Background(), []*pb.FlowEvent{flowForWrite("event-0")})
	if err == nil {
		t.Fatal("expected write error")
	}
	if got := result.Items[0].Disposition; got != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN {
		t.Fatalf("disposition=%s want OUTCOME_UNKNOWN", got)
	}
}
