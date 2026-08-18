package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

func TestStageDispatcherBuildCommandCaptureWindow(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "PROBE_CAPTURE_WINDOW", WindowStartMs: 1000, WindowEndMs: 2000,
		SourceSpec: json.RawMessage(`{"probe_id":"probe-8-2tb","interface":"ens9f0","packet_limit":100000,"byte_limit":0,"spool_quota_bytes":67108864}`),
	}
	cmd, err := d.buildCommand(att, "fence-1")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if cmd.Interface != "ens9f0" || cmd.SourceKind != "PROBE_CAPTURE_WINDOW" ||
		cmd.FencingToken != "fence-1" || cmd.PacketLimit != 100000 {
		t.Fatalf("command mismatch: %+v", cmd)
	}
}

func TestStageDispatcherBuildCommandLiveStreamAlias(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "LIVE_STREAM_WINDOW", WindowStartMs: 1000, WindowEndMs: 2000,
		SourceSpec: json.RawMessage(`{"probe_id":"probe-8-2tb","interface":"ens9f0","packet_limit":100000}`),
	}
	cmd, err := d.buildCommand(att, "fence-1")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if cmd.SourceKind != "PROBE_CAPTURE_WINDOW" {
		t.Fatalf("LIVE_STREAM_WINDOW must normalize to PROBE_CAPTURE_WINDOW, got %s", cmd.SourceKind)
	}
}

func TestStageDispatcherBuildCommandCaptureRequiresInterface(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", SourceKind: "PROBE_CAPTURE_WINDOW",
		SourceSpec: json.RawMessage(`{"probe_id":"probe-8-2tb","packet_limit":100000}`),
	}
	if _, err := d.buildCommand(att, "f"); err == nil || !strings.Contains(err.Error(), "interface") {
		t.Fatalf("missing interface must be rejected, got %v", err)
	}
}
