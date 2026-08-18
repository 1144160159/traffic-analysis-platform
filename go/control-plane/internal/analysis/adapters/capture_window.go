// Package adapters 采集执行器适配器(SourceExecutor 统一接口)。
// CaptureWindowAdapter 只做协议转换:冻结 manifest 校验 → 构造 CaptureWindowCommand
// → 经 probe.control.v2 投递给选定探针(实时采集窗口由探针常驻采集覆盖);
// 最终回执由探针→gateway→analysis.receipts.v1 异步回流,adapter 不执行采集。
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// CaptureWindowAdapter PROBE_CAPTURE_WINDOW/LIVE_STREAM_WINDOW 适配器(协议转换)。
type CaptureWindowAdapter struct {
	publisher ProbeCommandPublisher
	revisions CommandRevisionSource
	now       func() time.Time
}

// NewCaptureWindowAdapter 构造(与回放适配器共用命令 revision 权威)。
func NewCaptureWindowAdapter(publisher ProbeCommandPublisher, revisions CommandRevisionSource) *CaptureWindowAdapter {
	if revisions == nil {
		revisions = staticRevisionSource(1)
	}
	return &CaptureWindowAdapter{publisher: publisher, revisions: revisions, now: time.Now}
}

// CaptureWindowCommand 有界实时采集窗口命令(与 Rust CaptureWindowCommand 镜像)。
type CaptureWindowCommand struct {
	TenantID            string `json:"tenant_id"`
	TaskID              string `json:"task_id"`
	RunID               string `json:"run_id"`
	ExecutionSpecSHA256 string `json:"execution_spec_sha256"`
	ProbeID             string `json:"probe_id"`
	Interface           string `json:"interface"`
	BPFFilter           string `json:"bpf_filter,omitempty"`
	WindowStartMs       int64  `json:"window_start_ms"`
	WindowEndMs         int64  `json:"window_end_ms"`
	PacketLimit         uint64 `json:"packet_limit"`
	ByteLimit           uint64 `json:"byte_limit"`
	SpoolQuotaBytes     uint64 `json:"spool_quota_bytes"`
	LeaseEpoch          uint64 `json:"lease_epoch"`
	FencingToken        string `json:"fencing_token"`
}

// Dispatch 协议转换:校验冻结 manifest → 发布 capture_window 命令 → ACCEPTED。
func (a *CaptureWindowAdapter) Dispatch(ctx context.Context, cmd contract.SourceStageCommand) (*contract.ProviderOperationReceipt, error) {
	if cmd.SourceKind != "PROBE_CAPTURE_WINDOW" && cmd.SourceKind != "LIVE_STREAM_WINDOW" {
		return nil, fmt.Errorf("capture window adapter only accepts PROBE_CAPTURE_WINDOW/LIVE_STREAM_WINDOW, got %s", cmd.SourceKind)
	}
	if err := validateCaptureWindowCommand(cmd); err != nil {
		return nil, err
	}
	if a.publisher == nil {
		return nil, fmt.Errorf("probe command publisher is not configured")
	}

	command := CaptureWindowCommand{
		TenantID:            cmd.TenantID,
		TaskID:              cmd.TaskID,
		RunID:               cmd.RunID,
		ExecutionSpecSHA256: cmd.ExecutionSpecSHA256,
		ProbeID:             cmd.ProbeID,
		Interface:           cmd.Interface,
		BPFFilter:           cmd.BPFFilter,
		WindowStartMs:       cmd.WindowStartMs,
		WindowEndMs:         cmd.WindowEndMs,
		PacketLimit:         uint64(cmd.PacketLimit),
		ByteLimit:           uint64(cmd.ByteLimit),
		SpoolQuotaBytes:     uint64(cmd.SpoolQuotaBytes),
		LeaseEpoch:          uint64(cmd.LeaseEpoch),
		FencingToken:        cmd.FencingToken,
	}
	commandJSON, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("marshal capture window command: %w", err)
	}
	// 命令哈希与探针侧 canonical_json(键排序)一致:map 重排后 SHA-256。
	var canonical map[string]interface{}
	if err := json.Unmarshal(commandJSON, &canonical); err != nil {
		return nil, fmt.Errorf("canonicalize capture window command: %w", err)
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical capture window command: %w", err)
	}
	hash := sha256.Sum256(canonicalJSON)
	operationID := uuid.NewString()
	revision, err := a.revisions.NextProbeCommandRevision(ctx, cmd.TenantID, cmd.ProbeID)
	if err != nil {
		return nil, fmt.Errorf("allocate probe command revision: %w", err)
	}
	env := contract.ProbeCommandEnvelope{
		EventID:         uuid.NewString(),
		EventType:       contract.ProbeEventTypeOpRequested,
		SchemaVersion:   contract.ProbeCommandSchemaVersion,
		TenantID:        cmd.TenantID,
		ProbeID:         cmd.ProbeID,
		OperationID:     operationID,
		OperationType:   "capture_window",
		CommandRevision: revision,
		DesiredVersion:  cmd.ExecutionSpecSHA256,
		CommandHash:     hex.EncodeToString(hash[:]),
		// 心跳投递周期 60s;窗口命令与回放同容忍度(30 分钟安全下界)。
		ExpiresAt: a.now().Add(30 * time.Minute).Format(time.RFC3339Nano),
		TraceID:   uuid.NewString(),
		Command:   &command,
	}
	if err := a.publisher.Publish(ctx, env); err != nil {
		return nil, err
	}
	return &contract.ProviderOperationReceipt{
		OperationID: operationID,
		State:       "ACCEPTED",
		Fence:       json.RawMessage(fmt.Sprintf(`{"kind":"capture_window_fence","operation_id":%q,"probe_id":%q,"interface":%q}`, operationID, cmd.ProbeID, cmd.Interface)),
	}, nil
}

// validateCaptureWindowCommand 冻结 manifest 校验(身份/interface/窗口/限额/fencing)。
func validateCaptureWindowCommand(cmd contract.SourceStageCommand) error {
	for field, v := range map[string]string{
		"tenant_id": cmd.TenantID, "task_id": cmd.TaskID, "run_id": cmd.RunID,
		"execution_spec_sha256": cmd.ExecutionSpecSHA256, "probe_id": cmd.ProbeID,
		"interface": cmd.Interface, "fencing_token": cmd.FencingToken,
	} {
		if strings.TrimSpace(v) == "" {
			return newAdapterError(string(contract.ErrCodeInvalidTransition), field+" is required")
		}
	}
	if cmd.WindowEndMs <= cmd.WindowStartMs {
		return newAdapterError(string(contract.ErrCodeWindowMisfired), "window_end must be after window_start")
	}
	if cmd.PacketLimit <= 0 && cmd.ByteLimit <= 0 {
		return newAdapterError(string(contract.ErrCodeInvalidTransition), "packet_limit or byte_limit is required (bounded capture)")
	}
	if len(cmd.BPFFilter) > 512 {
		return newAdapterError(string(contract.ErrCodeInvalidTransition), "bpf_filter exceeds max length 512")
	}
	if cmd.SpoolQuotaBytes > 64*1024*1024*1024 {
		return newAdapterError(string(contract.ErrCodeInvalidTransition), "spool_quota_bytes exceeds 64GiB hard limit")
	}
	return nil
}
