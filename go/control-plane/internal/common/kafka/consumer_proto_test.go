package kafka

import (
	"testing"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	segmentkafka "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

func TestReceivedMessageUnmarshalProtoSupportsGeneratedV2Messages(t *testing.T) {
	source := &pb.DetectionBatch{TenantId: "tenant-a", BatchId: "batch-1"}
	payload, err := proto.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	message := &ReceivedMessage{Message: segmentkafka.Message{Value: payload}}
	var decoded pb.DetectionBatch

	if err := message.UnmarshalProto(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetTenantId() != "tenant-a" || decoded.GetBatchId() != "batch-1" {
		t.Fatalf("unexpected decoded message: %#v", decoded)
	}
}
