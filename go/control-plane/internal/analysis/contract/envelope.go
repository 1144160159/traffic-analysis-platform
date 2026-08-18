package contract

import (
	"encoding/json"
	"time"
)

// Envelope 信封契约(卷B §3.2)。JSON 直通 Kafka;字段加法式演进。

// RunSubscription 运行订阅:唯一承载 run 上下文,随 revision 单调广播。
// 状态 PREPARE(物化时,revision 1)→ ACTIVE(S1 领取时,更高 revision)→ CANCELLED。
// 字段加法式演进(旧消费者 ignoreUnknown);scope/subscription sha 为规范冻结值。
type RunSubscription struct {
	SchemaVersion       string          `json:"schema_version"`
	TenantID            string          `json:"tenant_id"`
	RunID               string          `json:"run_id"`
	TaskID              string          `json:"task_id,omitempty"`
	Revision            int64           `json:"revision"`
	State               string          `json:"state"` // PREPARE|ACTIVE|CANCELLED
	ExecutionSpecSHA256 string          `json:"execution_spec_sha256"`
	PlanRevision        int64           `json:"plan_revision,omitempty"`
	SourceKind          string          `json:"source_kind,omitempty"`
	WindowStartMs       int64           `json:"window_start_ms"`
	WindowEndMs         int64           `json:"window_end_ms"`
	PrepareAtMs         int64           `json:"prepare_at_ms,omitempty"`
	AllowedLatenessMs   int64           `json:"allowed_lateness_ms,omitempty"`
	LeaseEpoch          int64           `json:"lease_epoch,omitempty"`
	EffectivePolicySHA256 string        `json:"effective_policy_sha256,omitempty"`
	ExpiresAtMs         int64           `json:"expires_at_ms,omitempty"`
	ScopeSHA256         string          `json:"scope_sha256,omitempty"`
	SubscriptionSHA256  string          `json:"subscription_sha256,omitempty"`
	Fence               json.RawMessage `json:"fence"`
}

// AnalysisFlowEnvelope run-scoped 分析信封:base flow 保持无归属,
// RunScopeRouter 按订阅派生 0..N 个本信封(唯一携带执行上下文)。
type AnalysisFlowEnvelope struct {
	SchemaVersion       string          `json:"schema_version"`
	TenantID            string          `json:"tenant_id"`
	TaskID              string          `json:"task_id"`
	RunID               string          `json:"run_id"`
	ExecutionSpecSHA256 string          `json:"execution_spec_sha256"`
	WindowID            string          `json:"window_id"`
	StageID             string          `json:"stage_id"`
	Attempt             int32           `json:"attempt"`
	FencingToken        string          `json:"fencing_token"`
	Event               json.RawMessage `json:"event"`
}

// StageReceipt 执行器回执(PG inbox 消费后转为权威事实)。
type StageReceipt struct {
	SchemaVersion   string `json:"schema_version"`
	TenantID        string `json:"tenant_id"`
	RunID           string `json:"run_id"`
	ExecutionNodeID string `json:"execution_node_id"`
	Attempt         int32  `json:"attempt"`
	FencingToken    string `json:"fencing_token"`
	Provider        string `json:"provider"`
	InputCount      int64  `json:"input_count"`
	OutputCount     int64  `json:"output_count"`
	ErrorCount      int64  `json:"error_count"`
	RejectCount     int64  `json:"reject_count"`
	WatermarkMs     int64  `json:"watermark_ms"`
	Fence           json.RawMessage `json:"fence"`
	PayloadHash     string `json:"payload_hash"`
}

// PlanReadyReceipt required consumer 的 PlanReady ACK(引用执行哈希与订阅 revision)。
type PlanReadyReceipt struct {
	SchemaVersion       string `json:"schema_version"`
	TenantID            string `json:"tenant_id"`
	RunID               string `json:"run_id"`
	ConsumerID          string `json:"consumer_id"`
	ExecutionSpecSHA256 string `json:"execution_spec_sha256"`
	SubscriptionRevision int64 `json:"subscription_revision"`
	ReadyAtMs           int64  `json:"ready_at_ms"`
}

// NewEnvelopeTS 信封时间戳(毫秒)。
func NewEnvelopeTS() int64 { return time.Now().UnixMilli() }
