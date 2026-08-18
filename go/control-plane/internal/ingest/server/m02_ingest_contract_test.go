package server

import (
	"context"
	"errors"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/queue"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestM02IngestAndControlMatrix(t *testing.T) {
	logger := zap.NewNop()
	handler := getHandler(t)
	handler.logger = logger
	handler.handlerConfig.EnableDedup = false
	handler.handlerConfig.EnableDLQ = true
	handler.deduper = nil

	t.Run("authenticated identity precedes producer", func(t *testing.T) {
		producerCalled := false
		handler.writeFlowEvents = func(context.Context, []*pb.FlowEvent) (queue.BatchWriteResult, error) {
			producerCalled = true
			return queue.BatchWriteResult{}, nil
		}
		_, err := handler.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
			AcceptedResponseRevision: 1,
			Events: []*pb.FlowEvent{{
				Header: &pb.EventHeader{TenantId: "tenant-b", ProbeId: "probe-a", EventId: "m02-identity", EventTs: 1},
				Tuple:  &pb.FiveTuple{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
			}},
		})
		if status.Code(err) != codes.PermissionDenied || producerCalled {
			t.Fatalf("code=%s producer_called=%v err=%v", status.Code(err), producerCalled, err)
		}
	})

	t.Run("partial Kafka result is an exact item set", func(t *testing.T) {
		handler.writeFlowEvents = func(_ context.Context, events []*pb.FlowEvent) (queue.BatchWriteResult, error) {
			return queue.BatchWriteResult{Items: []queue.FlowWriteItemResult{
				{InputIndex: 0, EventID: events[0].Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED, ReasonCode: "KAFKA_REQUIRED_ACKS_ALL", AckScope: "KAFKA_RECORD"},
				{InputIndex: 1, EventID: events[1].Header.EventId, Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE, ReasonCode: "KAFKA_RETRYABLE", AckScope: "KAFKA_RECORD"},
			}}, errors.New("injected partial Kafka write")
		}
		response, err := handler.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
			AcceptedResponseRevision: 1,
			Events:                   []*pb.FlowEvent{validFlow("m02-acked"), validFlow("m02-retry")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.Accepted != 1 || response.Rejected != 0 || len(response.ItemResults) != 2 ||
			response.ItemResults[0].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED ||
			response.ItemResults[1].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE {
			t.Fatalf("response=%+v", response)
		}
	})

	t.Run("PCAP response is impossible before Kafka durability", func(t *testing.T) {
		handler.writePcapIndex = func(context.Context, *pb.PcapIndexMeta) error {
			return errors.New("injected Kafka outage")
		}
		_, err := handler.UploadPcapIndex(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadPcapIndexRequest{
			Index: &pb.PcapIndexMeta{FileKey: "tenant-a/probe-a/capture.pcap.zst"},
		})
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("code=%s err=%v", status.Code(err), err)
		}
	})

	t.Run("control bridge failure cannot acknowledge operation", func(t *testing.T) {
		bridge := &fakeProbeControlBridge{exchangeErr: errors.New("injected durable authority outage")}
		handler.SetProbeControlBridge(bridge)
		defer handler.SetProbeControlBridge(nil)
		_, err := handler.Heartbeat(
			probeIdentityContext("tenant-a", "probe-a"),
			&pb.HeartbeatRequest{TenantId: "tenant-a", ProbeId: "probe-a"},
		)
		if status.Code(err) != codes.Unavailable || bridge.calls != 1 {
			t.Fatalf("code=%s calls=%d err=%v", status.Code(err), bridge.calls, err)
		}
	})

	t.Run("missing producer fails closed", func(t *testing.T) {
		handler.writePcapIndex = nil
		_, err := handler.UploadPcapIndex(auth.WithTestProbe(
			auth.WithTestTenant(context.Background(), "tenant-a"), "probe-a"),
			&pb.UploadPcapIndexRequest{Index: &pb.PcapIndexMeta{FileKey: "capture.pcap.zst"}},
		)
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("code=%s err=%v", status.Code(err), err)
		}
	})

	t.Run("writer scope rejects a different authenticated probe", func(t *testing.T) {
		handler.handlerConfig.CanaryTenantID = "tenant-a"
		handler.handlerConfig.CanaryProbeIDs = []string{"probe-canary"}
		handler.writeFlowEvents = func(_ context.Context, events []*pb.FlowEvent) (queue.BatchWriteResult, error) {
			t.Fatalf("writer called outside canary scope: %s", events[0].Header.ProbeId)
			return queue.BatchWriteResult{}, nil
		}
		response, err := handler.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
			AcceptedResponseRevision: 1,
			Events:                   []*pb.FlowEvent{validFlow("m02-outside-scope")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(response.ItemResults) != 1 || response.ItemResults[0].ReasonCode != "CANARY_SCOPE_NOT_ACTIVE" ||
			response.ItemResults[0].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE {
			t.Fatalf("response=%+v", response)
		}
		handler.handlerConfig.CanaryTenantID = ""
		handler.handlerConfig.CanaryProbeIDs = nil
	})
}
