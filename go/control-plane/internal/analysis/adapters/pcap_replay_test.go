package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

type fakeProbePublisher struct {
	envelopes []contract.ProbeCommandEnvelope
	err       error
}

func (f *fakeProbePublisher) Publish(_ context.Context, env contract.ProbeCommandEnvelope) error {
	f.envelopes = append(f.envelopes, env)
	return f.err
}

func strings64(s string) string {
	base := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	_ = s
	return base
}

func validReplayCommand() contract.SourceStageCommand {
	return contract.SourceStageCommand{
		TenantID: "default", TaskID: "task-1", RunID: "run-1",
		ExecutionSpecSHA256: "spec-1",
		SourceKind:          "PCAP_REPLAY",
		ProbeID:             "probe-agent",
		ObjectRef:           "s3://analysis-bench/pcap/x.pcap",
		ObjectSHA256:        strings64("obj"),
		WindowStartMs:       1, WindowEndMs: 60_000,
		PacketLimit: 10_000, FencingToken: "fence-1",
	}
}

func TestPcapReplayAdapterPublishesCommand(t *testing.T) {
	pub := &fakeProbePublisher{}
	adapter := NewPcapReplayAdapter(pub, nil)
	receipt, err := adapter.Dispatch(context.Background(), validReplayCommand())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if receipt.State != "ACCEPTED" || receipt.OperationID == "" {
		t.Fatalf("expected ACCEPTED receipt, got %+v", receipt)
	}
	if len(pub.envelopes) != 1 {
		t.Fatalf("expected 1 published command, got %d", len(pub.envelopes))
	}
	env := pub.envelopes[0]
	if env.EventType != contract.ProbeEventTypeOpRequested || env.SchemaVersion != 2 {
		t.Fatalf("bridge contract mismatch: %+v", env)
	}
	if env.OperationType != "pcap_replay" || env.ProbeID != "probe-agent" {
		t.Fatalf("command routing mismatch: %+v", env)
	}
	cmd, ok := env.Command.(*contract.ReplayWindowCommand)
	if !ok || cmd == nil || cmd.RunID != "run-1" || cmd.PacketLimit != 10_000 {
		t.Fatalf("command payload mismatch: %+v", env.Command)
	}
	if len(env.CommandHash) != 64 {
		t.Fatalf("command hash must be 64 hex")
	}
}

func TestPcapReplayAdapterRejectsMissingProbe(t *testing.T) {
	adapter := NewPcapReplayAdapter(&fakeProbePublisher{}, nil)
	cmd := validReplayCommand()
	cmd.ProbeID = ""
	if _, err := adapter.Dispatch(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "probe_id is required") {
		t.Fatalf("expected probe_id rejection, got %v", err)
	}
}

func TestPcapReplayAdapterRejectsBadHash(t *testing.T) {
	adapter := NewPcapReplayAdapter(&fakeProbePublisher{}, nil)
	cmd := validReplayCommand()
	cmd.ObjectSHA256 = "not-hex"
	if _, err := adapter.Dispatch(context.Background(), cmd); err == nil {
		t.Fatalf("expected hash rejection")
	}
}

func TestPcapReplayAdapterRejectsUnbounded(t *testing.T) {
	adapter := NewPcapReplayAdapter(&fakeProbePublisher{}, nil)
	cmd := validReplayCommand()
	cmd.PacketLimit = 0
	cmd.ByteLimit = 0
	if _, err := adapter.Dispatch(context.Background(), cmd); err == nil {
		t.Fatalf("expected bounded replay rejection")
	}
}

func TestPcapReplayAdapterRejectsWrongSourceKind(t *testing.T) {
	adapter := NewPcapReplayAdapter(&fakeProbePublisher{}, nil)
	cmd := validReplayCommand()
	cmd.SourceKind = "PROBE_CAPTURE_WINDOW"
	if _, err := adapter.Dispatch(context.Background(), cmd); err == nil {
		t.Fatalf("expected source kind rejection")
	}
}

func TestPcapReplayAdapterPublisherErrorPropagates(t *testing.T) {
	pub := &fakeProbePublisher{err: errors.New("broker down")}
	adapter := NewPcapReplayAdapter(pub, nil)
	if _, err := adapter.Dispatch(context.Background(), validReplayCommand()); err == nil {
		t.Fatalf("expected publisher error to propagate")
	}
}

func TestPcapReplayAdapterPassesWireInterfaceThrough(t *testing.T) {
	pub := &fakeProbePublisher{}
	adapter := NewPcapReplayAdapter(pub, nil)
	cmd := validReplayCommand()
	cmd.Interface = "ta-veth-in"
	if _, err := adapter.Dispatch(context.Background(), cmd); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	env := pub.envelopes[0]
	rc, ok := env.Command.(*contract.ReplayWindowCommand)
	if !ok || rc.Interface != "ta-veth-in" {
		t.Fatalf("wire interface must flow through to the probe command, got %+v", env.Command)
	}
}

func TestPcapReplayAdapterRejectsInvalidWireInterface(t *testing.T) {
	adapter := NewPcapReplayAdapter(&fakeProbePublisher{}, nil)
	for _, iface := range []string{"bad/iface", "toolonginterfacename123", " spaced "} {
		cmd := validReplayCommand()
		cmd.Interface = iface
		if _, err := adapter.Dispatch(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "interface must match") {
			t.Fatalf("expected interface %q rejection, got %v", iface, err)
		}
	}
}
