package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/repository"
)

func TestStageDispatcherBuildCommandPCAPReplay(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "PCAP_REPLAY", WindowStartMs: 1000, WindowEndMs: 2000,
		SourceSpec: json.RawMessage(`{"pcap_object":"s3://b/p.pcap","pcap_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","packet_limit":1000,"byte_limit":0,"probe_id":"probe-agent"}`),
	}
	cmd, err := d.buildCommand(att, "fence-1")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if cmd.ObjectRef != "s3://b/p.pcap" || cmd.PacketLimit != 1000 || cmd.FencingToken != "fence-1" {
		t.Fatalf("command mismatch: %+v", cmd)
	}
	if cmd.ProbeID != "probe-agent" {
		t.Fatalf("probe id not propagated: %+v", cmd)
	}
	if cmd.RunID != "run-1" || cmd.TaskID != "task-1" || cmd.TenantID != "t1" {
		t.Fatalf("identity fields missing: %+v", cmd)
	}
}

func TestStageDispatcherBuildCommandPCAPReplayWireInterface(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "PCAP_REPLAY", WindowStartMs: 1000, WindowEndMs: 2000,
		SourceSpec: json.RawMessage(`{"pcap_object":"s3://b/p.pcap","pcap_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","packet_limit":1000,"byte_limit":0,"probe_id":"probe-agent","interface":"ta-veth-in"}`),
	}
	cmd, err := d.buildCommand(att, "fence-1")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if cmd.Interface != "ta-veth-in" {
		t.Fatalf("wire interface must flow from source_spec to command, got %+v", cmd)
	}
	// 空接口(生产回放)不携带字段
	att2 := &repository.PendingSourceAttempt{
		TenantID: "t1", TaskID: "task-1", RunID: "run-1", ExecutionSpecSHA256: "spec-1",
		SourceKind: "PCAP_REPLAY", WindowStartMs: 1000, WindowEndMs: 2000,
		SourceSpec: json.RawMessage(`{"pcap_object":"s3://b/p.pcap","pcap_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","packet_limit":1000,"byte_limit":0,"probe_id":"probe-agent"}`),
	}
	cmd2, err := d.buildCommand(att2, "fence-1")
	if err != nil {
		t.Fatalf("build command: %v", err)
	}
	if cmd2.Interface != "" {
		t.Fatalf("production replay must not carry a wire interface, got %+v", cmd2)
	}
}

func TestStageDispatcherBuildCommandRejectsEmptySpec(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{TenantID: "t1", SourceKind: "PCAP_REPLAY"}
	if _, err := d.buildCommand(att, "f"); err == nil {
		t.Fatalf("empty source_spec must be rejected")
	}
}

func TestStageDispatcherBuildCommandRejectsMissingProbe(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{
		TenantID: "t1", SourceKind: "PCAP_REPLAY",
		SourceSpec: json.RawMessage(`{"pcap_object":"s3://b/p.pcap","pcap_sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","packet_limit":1000}`),
	}
	if _, err := d.buildCommand(att, "f"); err == nil || !strings.Contains(err.Error(), "probe_id is required") {
		t.Fatalf("missing probe_id must be rejected, got %v", err)
	}
}

func TestStageDispatcherBuildCommandRejectsUnknownKind(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	att := &repository.PendingSourceAttempt{TenantID: "t1", SourceKind: "SATELLITE_LASER"}
	if _, err := d.buildCommand(att, "f"); err == nil {
		t.Fatalf("unknown source_kind must be rejected")
	}
}

func TestStageDispatcherNilExecutorFailsClosed(t *testing.T) {
	d := NewStageDispatcher(nil, nil, nil)
	if _, err := d.DispatchOnce(context.Background(), 1); err == nil {
		t.Fatalf("nil executor must fail closed")
	}
}
