// Package adapters 采集执行器适配器(SourceExecutor 统一接口)。
// Adapter 只做协议转换:冻结 manifest 校验 → 构造 ReplayWindowCommand →
// 经 probe.control.v2 投递给选定探针执行(回放发生在探针位置);
// 最终回执由探针→gateway→analysis.receipts.v1 异步回流,adapter 不执行回放。
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

// PcapReplayAdapter PCAP 回放适配器(协议转换)。
// Dispatch 先校验冻结 manifest(object_ref+sha256+窗口+限额),再发布命令;
// 校验失败拒绝(ATC-SRC-ERR-002),不回退到"跳过校验"。
type PcapReplayAdapter struct {
	publisher ProbeCommandPublisher
	revisions CommandRevisionSource
	now       func() time.Time
}

// CommandRevisionSource 命令 revision 权威(按 tenant+probe 单调递增)。
type CommandRevisionSource interface {
	NextProbeCommandRevision(ctx context.Context, tenantID, probeID string) (int64, error)
}

func NewPcapReplayAdapter(publisher ProbeCommandPublisher, revisions CommandRevisionSource) *PcapReplayAdapter {
	if revisions == nil {
		revisions = staticRevisionSource(1)
	}
	return &PcapReplayAdapter{publisher: publisher, revisions: revisions, now: time.Now}
}

// staticRevisionSource 测试/降级用固定 revision 源。
type staticRevisionSource int64

func (s staticRevisionSource) NextProbeCommandRevision(context.Context, string, string) (int64, error) {
	return int64(s), nil
}

func (a *PcapReplayAdapter) Dispatch(ctx context.Context, cmd contract.SourceStageCommand) (*contract.ProviderOperationReceipt, error) {
	if cmd.SourceKind != "PCAP_REPLAY" {
		return nil, fmt.Errorf("pcap replay adapter only accepts PCAP_REPLAY, got %s", cmd.SourceKind)
	}
	if err := validateReplayCommand(cmd); err != nil {
		return nil, err
	}
	if a.publisher == nil {
		return nil, fmt.Errorf("probe command publisher is not configured")
	}

	command := contract.ReplayWindowCommand{
		TenantID:            cmd.TenantID,
		TaskID:              cmd.TaskID,
		RunID:               cmd.RunID,
		ExecutionSpecSHA256: cmd.ExecutionSpecSHA256,
		ProbeID:             cmd.ProbeID,
		ObjectRef:           cmd.ObjectRef,
		ObjectSHA256:        cmd.ObjectSHA256,
		Interface:           cmd.Interface,
		WindowStartMs:       cmd.WindowStartMs,
		WindowEndMs:         cmd.WindowEndMs,
		PacketLimit:         uint64(cmd.PacketLimit),
		ByteLimit:           uint64(cmd.ByteLimit),
		FencingToken:        cmd.FencingToken,
	}
	commandJSON, err := json.Marshal(command)
	if err != nil {
		return nil, fmt.Errorf("marshal replay command: %w", err)
	}
	// 命令哈希必须与探针侧 canonical_json(键排序)一致:
	// 经 map 重排后计算 SHA-256(与 Rust deterministic_command_hash 对齐)。
	var canonical map[string]interface{}
	if err := json.Unmarshal(commandJSON, &canonical); err != nil {
		return nil, fmt.Errorf("canonicalize replay command: %w", err)
	}
	canonicalJSON, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical replay command: %w", err)
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
		OperationType:   contract.ProbeOperationTypePcapReplay,
		CommandRevision: revision,
		DesiredVersion:  cmd.ExecutionSpecSHA256,
		CommandHash:     hex.EncodeToString(hash[:]),
		// 心跳投递周期 60s;命令在 Redis 投递缓存中的存活窗口必须容忍探针心跳
		// 间歇性失败(连接重建/网关滚动)。5 分钟窗口在实测中导致命令在排队
		// 期间过期(探针收到后以 expired 拒绝);30 分钟是安全下界。
		ExpiresAt: a.now().Add(30 * time.Minute).Format(time.RFC3339Nano),
		TraceID:         uuid.NewString(),
		Command:         &command,
	}
	if err := a.publisher.Publish(ctx, env); err != nil {
		return nil, err
	}
	return &contract.ProviderOperationReceipt{
		OperationID: operationID,
		State:       "ACCEPTED",
		Fence:       json.RawMessage(fmt.Sprintf(`{"kind":"dispatch_fence","operation_id":%q,"probe_id":%q}`, operationID, cmd.ProbeID)),
	}, nil
}

func (a *PcapReplayAdapter) Cancel(_ context.Context, _ string) (*contract.ProviderOperationReceipt, error) {
	return &contract.ProviderOperationReceipt{State: "CANCELLED", OperationID: "cancel-not-tracked"}, nil
}

func (a *PcapReplayAdapter) Resolve(_ context.Context, operationID string) (*contract.ProviderAuthoritySnapshot, error) {
	return &contract.ProviderAuthoritySnapshot{OperationID: operationID, State: "UNKNOWN"}, nil
}

// validateReplayCommand 冻结 manifest 校验(调度侧语义:身份/对象引用/hash/窗口/限额/fencing)。
func validateReplayCommand(cmd contract.SourceStageCommand) error {
	for field, v := range map[string]string{
		"tenant_id": cmd.TenantID, "task_id": cmd.TaskID, "run_id": cmd.RunID,
		"execution_spec_sha256": cmd.ExecutionSpecSHA256, "object_ref": cmd.ObjectRef,
		"object_sha256": cmd.ObjectSHA256, "fencing_token": cmd.FencingToken,
		"probe_id": cmd.ProbeID,
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
	if !strings.HasPrefix(cmd.ObjectRef, "s3://") {
		return newAdapterError(string(contract.ErrCodeInvalidTransition), "object_ref must be s3://")
	}
	if !isHex64(cmd.ObjectSHA256) {
		return newAdapterError(string(contract.ErrCodeInvalidTransition), "object_sha256 must be 64 hex chars")
	}
	// wire 回放注入目标(测试阶段虚拟网卡;可选):Linux 接口名 1..=15 字节字母数字/._-
	if cmd.Interface != "" {
		if !ifNamePattern.MatchString(cmd.Interface) {
			return newAdapterError(string(contract.ErrCodeInvalidTransition), "interface must match [A-Za-z0-9._-]{1,15}")
		}
	}
	return nil
}

// ifNamePattern Linux 接口名约束(IFNAMSIZ=16 含 NUL)。
var ifNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// adapterError 稳定错误。
type adapterError struct {
	Code    string
	Message string
}

func newAdapterError(code, message string) *adapterError { return &adapterError{Code: code, Message: message} }
func (e *adapterError) Error() string                    { return e.Code + ": " + e.Message }
