package server

import (
	"context"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/queue"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestM06AssetBindingGatewayContract(t *testing.T) {
	handler := getHandler(t)
	handler.handlerConfig.CanaryTenantID = ""
	handler.handlerConfig.CanaryProbeIDs = nil

	t.Run("authenticated identity precedes Kafka", func(t *testing.T) {
		called := false
		handler.writeAssetBindings = func(context.Context, []*pb.MacIpBinding) (queue.AssetBindingWriteResult, error) {
			called = true
			return queue.AssetBindingWriteResult{}, nil
		}
		_, err := handler.UploadAssetBindings(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadAssetBindingsRequest{
			TenantId: "tenant-b", ProbeId: "probe-a", AcceptedResponseRevision: 1,
			Bindings: []*pb.MacIpBinding{validGatewayAssetBinding("obs-forged")},
		})
		if status.Code(err) != codes.PermissionDenied || called {
			t.Fatalf("code=%s called=%v err=%v", status.Code(err), called, err)
		}
	})

	t.Run("exact result includes accepted and deterministic rejection", func(t *testing.T) {
		handler.writeAssetBindings = func(_ context.Context, bindings []*pb.MacIpBinding) (queue.AssetBindingWriteResult, error) {
			if len(bindings) != 1 || bindings[0].TenantId != "tenant-a" || bindings[0].ProbeId != "probe-a" {
				t.Fatalf("bindings=%+v", bindings)
			}
			return queue.AssetBindingWriteResult{Items: []queue.AssetBindingWriteItemResult{{
				InputIndex: 0, ObservationID: bindings[0].ObservationId,
				Disposition: pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED,
				ReasonCode:  "KAFKA_REQUIRED_ACKS_ALL", AckScope: "KAFKA_RECORD",
			}}}, nil
		}
		valid := validGatewayAssetBinding("obs-ok")
		invalid := validGatewayAssetBinding("obs-invalid")
		invalid.MacAddress = "00-11-22-33-44-55"
		response, err := handler.UploadAssetBindings(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadAssetBindingsRequest{
			AcceptedResponseRevision: 1, Bindings: []*pb.MacIpBinding{valid, invalid},
		})
		if err != nil {
			t.Fatal(err)
		}
		if response.ResponseRevision != 1 || response.Accepted != 1 || response.Rejected != 1 || len(response.ItemResults) != 2 ||
			response.ItemResults[0].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED ||
			response.ItemResults[1].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID {
			t.Fatalf("response=%+v", response)
		}
	})

	t.Run("default-off writer is retryable and legacy clients fail the RPC", func(t *testing.T) {
		handler.writeAssetBindings = nil
		response, err := handler.UploadAssetBindings(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadAssetBindingsRequest{
			AcceptedResponseRevision: 1, Bindings: []*pb.MacIpBinding{validGatewayAssetBinding("obs-retry")},
		})
		if err != nil || len(response.ItemResults) != 1 ||
			response.ItemResults[0].Disposition != pb.AssetBindingItemDisposition_ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE {
			t.Fatalf("response=%+v err=%v", response, err)
		}
		_, err = handler.UploadAssetBindings(authenticatedFlowContext("tenant-a", "probe-a"), &pb.UploadAssetBindingsRequest{
			Bindings: []*pb.MacIpBinding{validGatewayAssetBinding("obs-legacy")},
		})
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("code=%s err=%v", status.Code(err), err)
		}
	})
}

func validGatewayAssetBinding(observationID string) *pb.MacIpBinding {
	return &pb.MacIpBinding{
		MacAddress: "00:11:22:33:44:55", IpAddress: "10.0.0.8", ObservationId: observationID,
		ObservedAt: time.Now().Add(-time.Second).UnixMilli(), Source: "arp", SchemaVersion: 1,
	}
}
