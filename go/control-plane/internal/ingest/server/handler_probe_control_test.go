package server

import (
	"context"
	"errors"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeProbeControlBridge struct {
	calls       int
	tenantID    string
	probeID     string
	acks        []*pb.ProbeOperationAck
	commands    []*pb.ProbeOperationCommand
	accepted    []string
	exchangeErr error
}

func (bridge *fakeProbeControlBridge) Exchange(
	_ context.Context,
	tenantID string,
	probeID string,
	acks []*pb.ProbeOperationAck,
) ([]*pb.ProbeOperationCommand, []string, error) {
	bridge.calls++
	bridge.tenantID = tenantID
	bridge.probeID = probeID
	bridge.acks = acks
	return bridge.commands, bridge.accepted, bridge.exchangeErr
}

func newProbeControlTestHandler(t *testing.T, bridge ProbeControlBridge) *IngestHandler {
	t.Helper()
	handler := getHandler(t)
	handler.SetProbeControlBridge(bridge)
	t.Cleanup(func() { handler.SetProbeControlBridge(nil) })
	return handler
}

func probeIdentityContext(tenantID, probeID string) context.Context {
	ctx := auth.WithTestTenant(context.Background(), tenantID)
	return context.WithValue(ctx, auth.ProbeIDKey, probeID)
}

func TestHeartbeatExchangesControlOnlyForAuthenticatedIdentity(t *testing.T) {
	operationID := "22222222-2222-4222-8222-222222222222"
	bridge := &fakeProbeControlBridge{
		commands: []*pb.ProbeOperationCommand{{
			OperationId: operationID,
			TenantId:    "tenant-a",
			ProbeId:     "probe-a",
		}},
		accepted: []string{operationID},
	}
	handler := newProbeControlTestHandler(t, bridge)

	response, err := handler.Heartbeat(
		probeIdentityContext("tenant-a", "probe-a"),
		&pb.HeartbeatRequest{
			TenantId: "tenant-a",
			ProbeId:  "probe-a",
			OperationAcks: []*pb.ProbeOperationAck{{
				OperationId: operationID,
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bridge.calls != 1 || bridge.tenantID != "tenant-a" || bridge.probeID != "probe-a" {
		t.Fatalf("unexpected bridge call: %#v", bridge)
	}
	if len(response.OperationCommands) != 1 ||
		len(response.AcceptedAckOperationIds) != 1 ||
		response.AcceptedAckOperationIds[0] != operationID {
		t.Fatalf("unexpected heartbeat control response: %#v", response)
	}
}

func TestHeartbeatRejectsBodyIdentityMismatchBeforeBridge(t *testing.T) {
	bridge := &fakeProbeControlBridge{}
	handler := newProbeControlTestHandler(t, bridge)

	_, err := handler.Heartbeat(
		probeIdentityContext("tenant-a", "probe-a"),
		&pb.HeartbeatRequest{TenantId: "tenant-a", ProbeId: "probe-b"},
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v want %v: %v", status.Code(err), codes.PermissionDenied, err)
	}
	if bridge.calls != 0 {
		t.Fatalf("bridge called %d times for mismatched identity", bridge.calls)
	}
}

func TestHeartbeatDoesNotAcknowledgeWhenControlBridgeFails(t *testing.T) {
	bridge := &fakeProbeControlBridge{exchangeErr: errors.New("durability unavailable")}
	handler := newProbeControlTestHandler(t, bridge)

	_, err := handler.Heartbeat(
		probeIdentityContext("tenant-a", "probe-a"),
		&pb.HeartbeatRequest{TenantId: "tenant-a", ProbeId: "probe-a"},
	)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code=%v want %v: %v", status.Code(err), codes.Unavailable, err)
	}
}
