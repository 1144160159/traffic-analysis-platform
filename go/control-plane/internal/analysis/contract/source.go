// Package contract 采集阶段协议(SourceStageCommand/回执/执行器端口):
// analysis-service 派发层与执行器适配层共享的跨边界类型,避免 service↔adapters 依赖环。
package contract

import (
	"context"
	"encoding/json"
)

// SourceStageCommand 采集阶段命令(SourceKind 归一)。
type SourceStageCommand struct {
	TenantID            string          `json:"tenant_id"`
	TaskID              string          `json:"task_id"`
	RunID               string          `json:"run_id"`
	ExecutionSpecSHA256 string          `json:"execution_spec_sha256"`
	SourceKind          string          `json:"source_kind"` // PROBE_CAPTURE_WINDOW|PCAP_REPLAY
	ProbeID             string          `json:"probe_id"`    // 人工执行链选定的探针
	ObjectRef           string          `json:"object_ref"`  // PCAP_REPLAY:s3://bucket/key
	ObjectSHA256        string          `json:"object_sha256"`
	WindowStartMs       int64           `json:"window_start_ms"`
	WindowEndMs         int64           `json:"window_end_ms"`
	PacketLimit         int64           `json:"packet_limit"`
	ByteLimit           int64           `json:"byte_limit"`
	// PROBE_CAPTURE_WINDOW 实时采集(有界窗口;探针常驻采集覆盖,流量经共享
	// 分支按订阅窗口归属 run)
	Interface       string `json:"interface,omitempty"`
	BPFFilter       string `json:"bpf_filter,omitempty"`
	SpoolQuotaBytes int64  `json:"spool_quota_bytes,omitempty"`
	LeaseEpoch      int64  `json:"lease_epoch,omitempty"`
	FencingToken    string `json:"fencing_token"`
}

// ProviderOperationReceipt 采集执行回执(provider 级;SOURCE_FENCE 由 source receipt 提供)。
type ProviderOperationReceipt struct {
	OperationID string          `json:"operation_id"`
	State       string          `json:"state"` // ACCEPTED|RUNNING|COMPLETED|FAILED
	InputCount  int64           `json:"input_count"`
	OutputCount int64           `json:"output_count"`
	ErrorCount  int64           `json:"error_count"`
	RejectCount int64           `json:"reject_count"`
	WatermarkMs int64           `json:"watermark_ms"`
	Fence       json.RawMessage `json:"fence"`
	PayloadHash string          `json:"payload_hash"`
}

// ProviderAuthoritySnapshot 执行状态快照。
type ProviderAuthoritySnapshot struct {
	OperationID string `json:"operation_id"`
	State       string `json:"state"`
	Detail      string `json:"detail"`
}

// ReplayFeeder 回放数据面端口(生产:读对象→校验 hash→喂入共享分支;测试:桩)。
type ReplayFeeder interface {
	Feed(ctx context.Context, cmd SourceStageCommand) (ProviderOperationReceipt, error)
}

// SourceExecutor 采集执行器统一端口(ISP 收窄:调度中心只依赖 Dispatch;
// Cancel/Resolve 为执行器可选能力,不进入端口契约)。
type SourceExecutor interface {
	Dispatch(ctx context.Context, cmd SourceStageCommand) (*ProviderOperationReceipt, error)
}
