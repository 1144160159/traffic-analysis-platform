package server

import (
	"context"
	"errors"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/queue"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func authenticatedFlowContext(tenantID, probeID string) context.Context {
	ctx := auth.WithTestTenant(context.Background(), tenantID)
	return auth.WithTestProbe(ctx, probeID)
}

func TestUploadFlowsRejectsIdentityMismatchBeforeProducer(t *testing.T) {
	h := getHandler(t)
	_, err := h.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{Events: []*pb.FlowEvent{{
		Header: &pb.EventHeader{TenantId: "tenant-b", ProbeId: "probe-a", EventId: "event-1", EventTs: 1},
		Tuple:  &pb.FiveTuple{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
	}}})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%s err=%v", status.Code(err), err)
	}
}

func TestUploadFlowsReturnsExactRejectedItemWithoutProducer(t *testing.T) {
	h := getHandler(t)
	response, err := h.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{Events: []*pb.FlowEvent{{
		Header: &pb.EventHeader{EventId: "invalid-event", EventTs: 1},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.ResponseRevision != 1 || len(response.ItemResults) != 1 {
		t.Fatalf("response=%+v", response)
	}
	item := response.ItemResults[0]
	if item.InputIndex != 0 || item.EventId != "invalid-event" || item.Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_REJECTED_INVALID {
		t.Fatalf("item=%+v", item)
	}
}

func TestUploadFlowsNewClientReceivesMixedExactSetAndOnlyAckedDedupCommits(t *testing.T) {
	h := getHandler(t)
	originalWriter := h.writeFlowEvents
	defer func() { h.writeFlowEvents = originalWriter }()
	callCount := 0
	h.writeFlowEvents = func(_ context.Context, events []*pb.FlowEvent) (queue.BatchWriteResult, error) {
		callCount++
		items := make([]queue.FlowWriteItemResult, len(events))
		for i, event := range events {
			disposition := pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED
			reason := "KAFKA_REQUIRED_ACKS_ALL"
			if event.Header.EventId == "retry-event" {
				disposition = pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE
				reason = "KAFKA_RETRYABLE"
			}
			items[i] = queue.FlowWriteItemResult{InputIndex: i, EventID: event.Header.EventId, Disposition: disposition, ReasonCode: reason, AckScope: "KAFKA_RECORD"}
		}
		return queue.BatchWriteResult{Items: items}, errors.New("partial Kafka failure")
	}
	events := []*pb.FlowEvent{
		validFlow("acked-event"),
		validFlow("retry-event"),
	}
	response, err := h.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
		Events:                   events,
		AcceptedResponseRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Accepted != 1 || len(response.ItemResults) != 2 {
		t.Fatalf("response=%+v", response)
	}
	if response.ItemResults[0].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_KAFKA_ACKED ||
		response.ItemResults[1].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE {
		t.Fatalf("items=%+v", response.ItemResults)
	}

	response, err = h.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
		Events:                   events,
		AcceptedResponseRevision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.ItemResults[0].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED ||
		response.ItemResults[1].Disposition != pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_RETRYABLE {
		t.Fatalf("second items=%+v", response.ItemResults)
	}
	if callCount != 2 {
		t.Fatalf("writer calls=%d want 2 (retry item is republished, ACKed item is not)", callCount)
	}
}

func TestUploadFlowsLegacyClientFailsClosedOnNonterminalOutcome(t *testing.T) {
	h := getHandler(t)
	originalWriter := h.writeFlowEvents
	defer func() { h.writeFlowEvents = originalWriter }()
	h.writeFlowEvents = func(_ context.Context, events []*pb.FlowEvent) (queue.BatchWriteResult, error) {
		return queue.BatchWriteResult{Items: []queue.FlowWriteItemResult{{
			InputIndex: 0, EventID: events[0].Header.EventId,
			Disposition: pb.FlowItemDisposition_FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN,
			ReasonCode:  "KAFKA_OUTCOME_UNKNOWN", AckScope: "KAFKA_RECORD",
		}}}, context.DeadlineExceeded
	}
	_, err := h.UploadFlows(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadFlowsRequest{
		Events: []*pb.FlowEvent{validFlow("legacy-unknown")},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code=%s err=%v", status.Code(err), err)
	}
}

func validFlow(eventID string) *pb.FlowEvent {
	return &pb.FlowEvent{
		Header: &pb.EventHeader{EventId: eventID, EventTs: 1},
		Tuple:  &pb.FiveTuple{SrcIp: "10.0.0.1", DstIp: "10.0.0.2"},
	}
}
