// Package contract 有界对象回放命令(Rust probe-agent ReplayWindowCommand 的 Go 镜像)。
// 人工执行链:调度中心选定探针后,经 probe.control.v2 投递;回放发生在探针位置。
package contract

// ReplayWindowCommand 有界对象回放命令(typed;校验在 Rust 侧 validate)。
type ReplayWindowCommand struct {
	TenantID            string `json:"tenant_id"`
	TaskID              string `json:"task_id"`
	RunID               string `json:"run_id"`
	ExecutionSpecSHA256 string `json:"execution_spec_sha256"`
	ProbeID             string `json:"probe_id"`
	ObjectRef           string `json:"object_ref"`    // s3://bucket/key
	ObjectSHA256        string `json:"object_sha256"` // 64 hex
	// Interface 测试阶段 wire 回放注入目标(虚拟网卡输入端);空 = 进程内共享分支喂入。
	// 非空时探针经 AF_PACKET 向该接口注入真实流量(须在探针 allowlist 内,fail-closed),
	// 供输出端探针实时采集;omitempty 保证两侧 canonical hash 一致(空字段不出现在 JSON)。
	Interface     string `json:"interface,omitempty"`
	WindowStartMs int64  `json:"window_start_ms"`
	WindowEndMs   int64  `json:"window_end_ms"`
	PacketLimit   uint64 `json:"packet_limit"`
	ByteLimit     uint64 `json:"byte_limit"`
	FencingToken  string `json:"fencing_token"`
}

// ProbeCommandEnvelope probe.control.v2 命令信封(与 gateway bridge 契约一致:
// event_type=traffic.probe.v2.OperationRequested,schema_version=2)。
type ProbeCommandEnvelope struct {
	EventID         string                 `json:"event_id"`
	EventType       string                 `json:"event_type"`
	SchemaVersion   int                    `json:"schema_version"`
	TenantID        string                 `json:"tenant_id"`
	ProbeID         string                 `json:"probe_id"`
	OperationID     string                 `json:"operation_id"`
	OperationType   string                 `json:"operation_type"`
	CommandRevision int64                  `json:"command_revision"`
	DesiredVersion  string                 `json:"desired_version"`
	CommandHash     string                 `json:"command_hash"`
	ExpiresAt       string                 `json:"expires_at"` // RFC3339Nano
	TraceID         string                 `json:"trace_id"`
	Command         interface{}            `json:"command"` // ReplayWindowCommand | CaptureWindowCommand(按 operation_type)
}

// 桥接器契约常量(与 gateway control/bridge.go 对齐)。
const (
	ProbeControlTopic            = "probe.control.v2"
	ProbeEventTypeOpRequested    = "traffic.probe.v2.OperationRequested"
	ProbeCommandSchemaVersion    = 2
	ProbeOperationTypePcapReplay = "pcap_replay"
)
