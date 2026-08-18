package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

type fakeCapturePublisher struct {
	env      contract.ProbeCommandEnvelope
	revision int64
	err      error
}

func (p *fakeCapturePublisher) Publish(_ context.Context, env contract.ProbeCommandEnvelope) error {
	p.env = env
	return p.err
}

func (p *fakeCapturePublisher) NextProbeCommandRevision(_ context.Context, _, _ string) (int64, error) {
	p.revision++
	return p.revision, nil
}

func captureCommand() contract.SourceStageCommand {
	return contract.SourceStageCommand{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "PROBE_CAPTURE_WINDOW", ProbeID: "probe-8-2tb", Interface: "ens9f0",
		WindowStartMs: 1000, WindowEndMs: 2000, PacketLimit: 100000, ByteLimit: 0,
		SpoolQuotaBytes: 64 * 1024 * 1024, FencingToken: "fence-1",
	}
}

func TestCaptureWindowAdapterPublishesCommand(t *testing.T) {
	pub := &fakeCapturePublisher{}
	a := NewCaptureWindowAdapter(pub, pub)
	receipt, err := a.Dispatch(context.Background(), captureCommand())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if receipt.State != "ACCEPTED" {
		t.Fatalf("expected ACCEPTED, got %s", receipt.State)
	}
	env := pub.env
	if env.OperationType != "capture_window" || env.ProbeID != "probe-8-2tb" {
		t.Fatalf("command routing mismatch: %+v", env)
	}
	cmd, ok := env.Command.(*CaptureWindowCommand)
	if !ok || cmd == nil || cmd.Interface != "ens9f0" || cmd.RunID != "run-1" || cmd.FencingToken != "fence-1" {
		t.Fatalf("command payload mismatch: %+v", env.Command)
	}
	if len(env.CommandHash) != 64 {
		t.Fatalf("command hash must be 64 hex")
	}
	if env.CommandRevision != 1 {
		t.Fatalf("revision should come from source, got %d", env.CommandRevision)
	}
}

func TestCaptureWindowAdapterRejectsMissingInterface(t *testing.T) {
	pub := &fakeCapturePublisher{}
	a := NewCaptureWindowAdapter(pub, pub)
	cmd := captureCommand()
	cmd.Interface = ""
	if _, err := a.Dispatch(context.Background(), cmd); err == nil || !strings.Contains(err.Error(), "interface") {
		t.Fatalf("missing interface must be rejected, got %v", err)
	}
}

func TestCaptureWindowAdapterRejectsUnbounded(t *testing.T) {
	pub := &fakeCapturePublisher{}
	a := NewCaptureWindowAdapter(pub, pub)
	cmd := captureCommand()
	cmd.PacketLimit = 0
	cmd.ByteLimit = 0
	if _, err := a.Dispatch(context.Background(), cmd); err == nil {
		t.Fatalf("unbounded capture must be rejected")
	}
}

func TestCaptureWindowAdapterRejectsBadWindow(t *testing.T) {
	pub := &fakeCapturePublisher{}
	a := NewCaptureWindowAdapter(pub, pub)
	cmd := captureCommand()
	cmd.WindowStartMs = 2000
	cmd.WindowEndMs = 1000
	if _, err := a.Dispatch(context.Background(), cmd); err == nil {
		t.Fatalf("inverted window must be rejected")
	}
}
