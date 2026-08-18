// @generated
/// EventHeader 事件头（所有事件的公共字段）
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct EventHeader {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub event_ts: i64,
    #[prost(int64, tag="5")]
    pub ingest_ts: i64,
    #[prost(string, tag="6")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub feature_set_id: ::prost::alloc::string::String,
    /// kafka_ts is the millisecond timestamp assigned immediately before the event is published to Kafka.
    #[prost(int64, tag="8")]
    pub kafka_ts: i64,
    /// flink_out_ts is the millisecond timestamp assigned when a Flink job emits or persists the derived event.
    #[prost(int64, tag="9")]
    pub flink_out_ts: i64,
    /// The fields below are an additive v1 event envelope. Existing fields remain
    /// wire-compatible while producers migrate to the complete contract.
    #[prost(string, tag="10")]
    pub event_type: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub schema_version: ::prost::alloc::string::String,
    #[prost(string, tag="12")]
    pub aggregate_type: ::prost::alloc::string::String,
    #[prost(string, tag="13")]
    pub aggregate_id: ::prost::alloc::string::String,
    #[prost(uint64, tag="14")]
    pub aggregate_version: u64,
    #[prost(int64, tag="15")]
    pub occurred_at: i64,
    #[prost(int64, tag="16")]
    pub produced_at: i64,
    #[prost(string, tag="17")]
    pub trace_id: ::prost::alloc::string::String,
    #[prost(string, tag="18")]
    pub causation_id: ::prost::alloc::string::String,
    #[prost(string, tag="19")]
    pub correlation_id: ::prost::alloc::string::String,
    #[prost(string, tag="20")]
    pub idempotency_key: ::prost::alloc::string::String,
    #[prost(string, tag="21")]
    pub producer: ::prost::alloc::string::String,
}
/// FiveTuple 五元组
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FiveTuple {
    #[prost(string, tag="1")]
    pub src_ip: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub dst_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="3")]
    pub src_port: u32,
    #[prost(uint32, tag="4")]
    pub dst_port: u32,
    #[prost(uint32, tag="5")]
    pub protocol: u32,
}
/// PacketLengthStats 包长度统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PacketLengthStats {
    #[prost(uint32, tag="1")]
    pub min: u32,
    #[prost(uint32, tag="2")]
    pub max: u32,
    #[prost(float, tag="3")]
    pub mean: f32,
    #[prost(float, tag="4")]
    pub std: f32,
}
/// InterArrivalStats 到达间隔统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct InterArrivalStats {
    #[prost(float, tag="1")]
    pub min_ms: f32,
    #[prost(float, tag="2")]
    pub max_ms: f32,
    #[prost(float, tag="3")]
    pub mean_ms: f32,
    #[prost(float, tag="4")]
    pub std_ms: f32,
}
/// ActiveIdleStats Active/Idle 时间统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ActiveIdleStats {
    #[prost(float, tag="1")]
    pub min_ms: f32,
    #[prost(float, tag="2")]
    pub mean_ms: f32,
    #[prost(float, tag="3")]
    pub max_ms: f32,
    #[prost(float, tag="4")]
    pub std_ms: f32,
}
/// TrafficFeatureObservation is a bounded, additive carrier from packet capture
/// through sessionization to feature projection. It contains no raw payload:
/// byte histograms and hashes are sufficient for deterministic statistics while
/// raw_traffic_ref, when present, points to separately governed PCAP evidence.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct TrafficFeatureObservation {
    #[prost(string, tag="1")]
    pub schema_version: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub algorithm_version: ::prost::alloc::string::String,
    #[prost(sint32, repeated, tag="3")]
    pub signed_packet_lengths: ::prost::alloc::vec::Vec<i32>,
    #[prost(int64, repeated, tag="4")]
    pub packet_event_time_us: ::prost::alloc::vec::Vec<i64>,
    /// Sixteen nibble buckets (0x0..0xf), bounded independently of flow size.
    #[prost(uint64, repeated, tag="5")]
    pub payload_nibble_counts: ::prost::alloc::vec::Vec<u64>,
    #[prost(uint64, tag="6")]
    pub payload_observed_bytes: u64,
    #[prost(bool, tag="7")]
    pub sequence_truncated: bool,
    #[prost(enumeration="TransportSecurityProtocol", tag="8")]
    pub transport_security: i32,
    #[prost(string, tag="9")]
    pub tls_version: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub ja3: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub ja4: ::prost::alloc::string::String,
    #[prost(string, tag="12")]
    pub sni: ::prost::alloc::string::String,
    #[prost(string, tag="13")]
    pub cert_sha256: ::prost::alloc::string::String,
    #[prost(bool, tag="14")]
    pub cert_is_self_signed: bool,
    #[prost(bool, tag="15")]
    pub cert_is_self_signed_known: bool,
    #[prost(uint32, tag="16")]
    pub pubkey_len: u32,
    #[prost(bool, tag="17")]
    pub pubkey_len_known: bool,
    #[prost(string, tag="18")]
    pub quic_version: ::prost::alloc::string::String,
    #[prost(string, tag="19")]
    pub raw_traffic_ref: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="20")]
    pub missing_fields: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// TransportSecurityProtocol names only a positively observed wire protocol.
/// Port numbers alone MUST NOT set this value or imply malicious traffic.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TransportSecurityProtocol {
    Unspecified = 0,
    Tls = 1,
    Quic = 2,
}
impl TransportSecurityProtocol {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TransportSecurityProtocol::Unspecified => "TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED",
            TransportSecurityProtocol::Tls => "TRANSPORT_SECURITY_PROTOCOL_TLS",
            TransportSecurityProtocol::Quic => "TRANSPORT_SECURITY_PROTOCOL_QUIC",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED" => Some(Self::Unspecified),
            "TRANSPORT_SECURITY_PROTOCOL_TLS" => Some(Self::Tls),
            "TRANSPORT_SECURITY_PROTOCOL_QUIC" => Some(Self::Quic),
            _ => None,
        }
    }
}
/// FlowDirection 流方向
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum FlowDirection {
    Unspecified = 0,
    Forward = 1,
    Backward = 2,
    Bidirectional = 3,
}
impl FlowDirection {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            FlowDirection::Unspecified => "FLOW_DIRECTION_UNSPECIFIED",
            FlowDirection::Forward => "FLOW_DIRECTION_FORWARD",
            FlowDirection::Backward => "FLOW_DIRECTION_BACKWARD",
            FlowDirection::Bidirectional => "FLOW_DIRECTION_BIDIRECTIONAL",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "FLOW_DIRECTION_UNSPECIFIED" => Some(Self::Unspecified),
            "FLOW_DIRECTION_FORWARD" => Some(Self::Forward),
            "FLOW_DIRECTION_BACKWARD" => Some(Self::Backward),
            "FLOW_DIRECTION_BIDIRECTIONAL" => Some(Self::Bidirectional),
            _ => None,
        }
    }
}
/// Severity 严重程度
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum Severity {
    Unspecified = 0,
    Info = 1,
    Low = 2,
    Medium = 3,
    High = 4,
    Critical = 5,
}
impl Severity {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            Severity::Unspecified => "SEVERITY_UNSPECIFIED",
            Severity::Info => "SEVERITY_INFO",
            Severity::Low => "SEVERITY_LOW",
            Severity::Medium => "SEVERITY_MEDIUM",
            Severity::High => "SEVERITY_HIGH",
            Severity::Critical => "SEVERITY_CRITICAL",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "SEVERITY_UNSPECIFIED" => Some(Self::Unspecified),
            "SEVERITY_INFO" => Some(Self::Info),
            "SEVERITY_LOW" => Some(Self::Low),
            "SEVERITY_MEDIUM" => Some(Self::Medium),
            "SEVERITY_HIGH" => Some(Self::High),
            "SEVERITY_CRITICAL" => Some(Self::Critical),
            _ => None,
        }
    }
}
/// AlertStatus 告警状态
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum AlertStatus {
    Unspecified = 0,
    New = 1,
    Triage = 2,
    Assigned = 3,
    InProgress = 4,
    Resolved = 5,
    Closed = 6,
    FalsePositive = 7,
}
impl AlertStatus {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            AlertStatus::Unspecified => "ALERT_STATUS_UNSPECIFIED",
            AlertStatus::New => "ALERT_STATUS_NEW",
            AlertStatus::Triage => "ALERT_STATUS_TRIAGE",
            AlertStatus::Assigned => "ALERT_STATUS_ASSIGNED",
            AlertStatus::InProgress => "ALERT_STATUS_IN_PROGRESS",
            AlertStatus::Resolved => "ALERT_STATUS_RESOLVED",
            AlertStatus::Closed => "ALERT_STATUS_CLOSED",
            AlertStatus::FalsePositive => "ALERT_STATUS_FALSE_POSITIVE",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "ALERT_STATUS_UNSPECIFIED" => Some(Self::Unspecified),
            "ALERT_STATUS_NEW" => Some(Self::New),
            "ALERT_STATUS_TRIAGE" => Some(Self::Triage),
            "ALERT_STATUS_ASSIGNED" => Some(Self::Assigned),
            "ALERT_STATUS_IN_PROGRESS" => Some(Self::InProgress),
            "ALERT_STATUS_RESOLVED" => Some(Self::Resolved),
            "ALERT_STATUS_CLOSED" => Some(Self::Closed),
            "ALERT_STATUS_FALSE_POSITIVE" => Some(Self::FalsePositive),
            _ => None,
        }
    }
}
/// DeploymentStatus 部署状态
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum DeploymentStatus {
    Unspecified = 0,
    Planned = 1,
    Gray = 2,
    Active = 3,
    Paused = 4,
    RolledBack = 5,
}
impl DeploymentStatus {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            DeploymentStatus::Unspecified => "DEPLOYMENT_STATUS_UNSPECIFIED",
            DeploymentStatus::Planned => "DEPLOYMENT_STATUS_PLANNED",
            DeploymentStatus::Gray => "DEPLOYMENT_STATUS_GRAY",
            DeploymentStatus::Active => "DEPLOYMENT_STATUS_ACTIVE",
            DeploymentStatus::Paused => "DEPLOYMENT_STATUS_PAUSED",
            DeploymentStatus::RolledBack => "DEPLOYMENT_STATUS_ROLLED_BACK",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "DEPLOYMENT_STATUS_UNSPECIFIED" => Some(Self::Unspecified),
            "DEPLOYMENT_STATUS_PLANNED" => Some(Self::Planned),
            "DEPLOYMENT_STATUS_GRAY" => Some(Self::Gray),
            "DEPLOYMENT_STATUS_ACTIVE" => Some(Self::Active),
            "DEPLOYMENT_STATUS_PAUSED" => Some(Self::Paused),
            "DEPLOYMENT_STATUS_ROLLED_BACK" => Some(Self::RolledBack),
            _ => None,
        }
    }
}
/// TaskType 任务类型
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TaskType {
    Unspecified = 0,
    Replay = 1,
    Train = 2,
    Eval = 3,
    PcapCut = 4,
}
impl TaskType {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TaskType::Unspecified => "TASK_TYPE_UNSPECIFIED",
            TaskType::Replay => "TASK_TYPE_REPLAY",
            TaskType::Train => "TASK_TYPE_TRAIN",
            TaskType::Eval => "TASK_TYPE_EVAL",
            TaskType::PcapCut => "TASK_TYPE_PCAP_CUT",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TASK_TYPE_UNSPECIFIED" => Some(Self::Unspecified),
            "TASK_TYPE_REPLAY" => Some(Self::Replay),
            "TASK_TYPE_TRAIN" => Some(Self::Train),
            "TASK_TYPE_EVAL" => Some(Self::Eval),
            "TASK_TYPE_PCAP_CUT" => Some(Self::PcapCut),
            _ => None,
        }
    }
}
/// TaskStatus 任务状态
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TaskStatus {
    Unspecified = 0,
    Queued = 1,
    Running = 2,
    Succeeded = 3,
    Failed = 4,
    Canceled = 5,
}
impl TaskStatus {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TaskStatus::Unspecified => "TASK_STATUS_UNSPECIFIED",
            TaskStatus::Queued => "TASK_STATUS_QUEUED",
            TaskStatus::Running => "TASK_STATUS_RUNNING",
            TaskStatus::Succeeded => "TASK_STATUS_SUCCEEDED",
            TaskStatus::Failed => "TASK_STATUS_FAILED",
            TaskStatus::Canceled => "TASK_STATUS_CANCELED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TASK_STATUS_UNSPECIFIED" => Some(Self::Unspecified),
            "TASK_STATUS_QUEUED" => Some(Self::Queued),
            "TASK_STATUS_RUNNING" => Some(Self::Running),
            "TASK_STATUS_SUCCEEDED" => Some(Self::Succeeded),
            "TASK_STATUS_FAILED" => Some(Self::Failed),
            "TASK_STATUS_CANCELED" => Some(Self::Canceled),
            _ => None,
        }
    }
}
/// Alert 告警事件
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Alert {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub first_seen: i64,
    #[prost(int64, tag="4")]
    pub last_seen: i64,
    #[prost(enumeration="Severity", tag="5")]
    pub severity: i32,
    #[prost(string, tag="6")]
    pub alert_type: ::prost::alloc::string::String,
    #[prost(float, tag="7")]
    pub score: f32,
    #[prost(string, repeated, tag="8")]
    pub labels: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="9")]
    pub src_ip: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub dst_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="11")]
    pub src_port: u32,
    #[prost(uint32, tag="12")]
    pub dst_port: u32,
    #[prost(uint32, tag="13")]
    pub protocol: u32,
    #[prost(string, tag="14")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="15")]
    pub session_id: ::prost::alloc::string::String,
    #[prost(string, tag="16")]
    pub campaign_id: ::prost::alloc::string::String,
    #[prost(string, tag="17")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="18")]
    pub rule_version: ::prost::alloc::string::String,
    #[prost(string, tag="19")]
    pub feature_set_id: ::prost::alloc::string::String,
    #[prost(enumeration="AlertStatus", tag="20")]
    pub status: i32,
    #[prost(string, tag="21")]
    pub assignee: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="22")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="23")]
    pub dedup_fingerprint: ::prost::alloc::string::String,
    #[prost(int64, tag="24")]
    pub updated_ts: i64,
    #[prost(string, tag="25")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(int64, tag="26")]
    pub ingest_ts: i64,
    #[prost(string, tag="27")]
    pub protocol_name: ::prost::alloc::string::String,
    #[prost(int32, tag="28")]
    pub count: i32,
    #[prost(string, tag="29")]
    pub arkime_session_link: ::prost::alloc::string::String,
    #[prost(string, tag="30")]
    pub feedback_label: ::prost::alloc::string::String,
    #[prost(uint32, tag="31")]
    pub feedback_count: u32,
    #[prost(uint64, tag="32")]
    pub state_version: u64,
    /// trace_id is the 32-character lowercase W3C trace identifier propagated
    /// from the originating request/event through every alert projection.
    #[prost(string, tag="33")]
    pub trace_id: ::prost::alloc::string::String,
}
/// Evidence 证据
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Evidence {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub evidence_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub ts: i64,
    #[prost(string, tag="5")]
    pub r#type: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub summary: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub metrics_json: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub snippet_ref_json: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub arkime_link: ::prost::alloc::string::String,
    #[prost(float, tag="10")]
    pub confidence: f32,
    #[prost(string, tag="11")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(int64, tag="12")]
    pub ingest_ts: i64,
    #[prost(string, tag="13")]
    pub visualization_url: ::prost::alloc::string::String,
}
/// Campaign 战役
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Campaign {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub campaign_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub ts_start: i64,
    #[prost(int64, tag="4")]
    pub ts_end: i64,
    #[prost(string, repeated, tag="5")]
    pub entities: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="6")]
    pub alerts: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(float, tag="7")]
    pub score: f32,
    #[prost(string, tag="8")]
    pub summary: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(int64, tag="10")]
    pub ingest_ts: i64,
    #[prost(message, optional, tag="11")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="12")]
    pub campaign_type: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="13")]
    pub attack_phases: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="14")]
    pub rule_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="15")]
    pub model_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// AlertBatch 告警批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertBatch {
    #[prost(message, repeated, tag="1")]
    pub alerts: ::prost::alloc::vec::Vec<Alert>,
    #[prost(message, repeated, tag="2")]
    pub evidences: ::prost::alloc::vec::Vec<Evidence>,
    #[prost(message, repeated, tag="3")]
    pub campaigns: ::prost::alloc::vec::Vec<Campaign>,
    #[prost(string, tag="4")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub created_at: i64,
}
/// AlertUpdate 告警更新
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertUpdate {
    #[prost(string, tag="1")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(enumeration="AlertStatus", tag="2")]
    pub status: i32,
    #[prost(string, tag="3")]
    pub assignee: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub comment: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub updated_by: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub updated_at: i64,
}
/// AlertFeedback 用户反馈
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertFeedback {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub feedback_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub user_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub label: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub reason_code: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub comment: ::prost::alloc::string::String,
    /// Boolean semantics expressed as uint32 (0/1) for wire compatibility;
    /// a future breaking-change migration should use `bool`.
    #[prost(uint32, tag="8")]
    pub add_to_whitelist: u32,
    #[prost(string, tag="9")]
    pub alert_type: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub severity: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="12")]
    pub rule_version: ::prost::alloc::string::String,
    #[prost(int64, tag="13")]
    pub ts: i64,
    #[prost(int64, tag="14")]
    pub ingest_ts: i64,
}
/// WhitelistRule 白名单规则
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct WhitelistRule {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub rule_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub rule_type: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub src_ip: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub dst_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="6")]
    pub src_port: u32,
    #[prost(uint32, tag="7")]
    pub dst_port: u32,
    #[prost(uint32, tag="8")]
    pub protocol: u32,
    #[prost(string, tag="9")]
    pub alert_type: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub reason_code: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub comment: ::prost::alloc::string::String,
    #[prost(string, tag="12")]
    pub status: ::prost::alloc::string::String,
    #[prost(string, tag="13")]
    pub created_by: ::prost::alloc::string::String,
    #[prost(int64, tag="14")]
    pub created_ts: i64,
    #[prost(int64, tag="15")]
    pub updated_ts: i64,
    #[prost(int64, tag="16")]
    pub expires_at: i64,
    #[prost(int64, tag="17")]
    pub ingest_ts: i64,
}
/// AlertStateTransition 告警状态转换
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertStateTransition {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub transition_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub old_status: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub new_status: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub old_assignee: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub new_assignee: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub changed_by: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub change_reason: ::prost::alloc::string::String,
    #[prost(uint64, tag="10")]
    pub state_version: u64,
    #[prost(int64, tag="11")]
    pub ts: i64,
    #[prost(int64, tag="12")]
    pub ingest_ts: i64,
}
/// DedupStats 去重统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DedupStats {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub fingerprint: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub alert_type: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub severity: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub src_ip: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub dst_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="7")]
    pub dst_port: u32,
    #[prost(int64, tag="8")]
    pub first_seen: i64,
    #[prost(int64, tag="9")]
    pub last_seen: i64,
    #[prost(uint64, tag="10")]
    pub occurrence_count: u64,
    #[prost(string, repeated, tag="11")]
    pub sample_alert_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(int64, tag="12")]
    pub ingest_ts: i64,
}
/// StorageHealthEvent 存储健康状态事件
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StorageHealthEvent {
    #[prost(string, tag="1")]
    pub storage_type: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub storage_name: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub status: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub error_message: ::prost::alloc::string::String,
    #[prost(uint32, tag="5")]
    pub consecutive_failures: u32,
    #[prost(int64, tag="6")]
    pub ts: i64,
    #[prost(int64, tag="7")]
    pub ingest_ts: i64,
}
/// ModelFeedbackMetrics 模型反馈指标聚合
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ModelFeedbackMetrics {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub alert_type: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub hour: i64,
    #[prost(uint64, tag="5")]
    pub total_alerts: u64,
    #[prost(uint64, tag="6")]
    pub tp_count: u64,
    #[prost(uint64, tag="7")]
    pub fp_count: u64,
    #[prost(uint64, tag="8")]
    pub unlabeled_count: u64,
    #[prost(float, tag="9")]
    pub precision: f32,
    #[prost(float, tag="10")]
    pub recall: f32,
    #[prost(float, tag="11")]
    pub f1_score: f32,
    #[prost(int64, tag="12")]
    pub ingest_ts: i64,
}
/// AlertCorrelationEdge 告警关联边
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertCorrelationEdge {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub edge_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub source_alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub target_alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub correlation_type: ::prost::alloc::string::String,
    #[prost(float, tag="6")]
    pub correlation_score: f32,
    #[prost(string, repeated, tag="7")]
    pub shared_entities: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(int64, tag="8")]
    pub time_delta_ms: i64,
    #[prost(int64, tag="9")]
    pub ts: i64,
    #[prost(int64, tag="10")]
    pub ingest_ts: i64,
}
/// NotificationEvent 通知事件
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct NotificationEvent {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub notification_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub channel: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub status: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub error_message: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub rule_id: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub recipient: ::prost::alloc::string::String,
    #[prost(int64, tag="9")]
    pub sent_at: i64,
    #[prost(int64, tag="10")]
    pub ingest_ts: i64,
}
/// AlertExtendedBatch 扩展批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertExtendedBatch {
    #[prost(string, tag="1")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub created_at: i64,
    #[prost(message, repeated, tag="10")]
    pub alerts: ::prost::alloc::vec::Vec<Alert>,
    #[prost(message, repeated, tag="11")]
    pub evidences: ::prost::alloc::vec::Vec<Evidence>,
    #[prost(message, repeated, tag="12")]
    pub campaigns: ::prost::alloc::vec::Vec<Campaign>,
    #[prost(message, repeated, tag="13")]
    pub feedbacks: ::prost::alloc::vec::Vec<AlertFeedback>,
    #[prost(message, repeated, tag="14")]
    pub whitelist_rules: ::prost::alloc::vec::Vec<WhitelistRule>,
    #[prost(message, repeated, tag="15")]
    pub state_transitions: ::prost::alloc::vec::Vec<AlertStateTransition>,
    #[prost(message, repeated, tag="16")]
    pub dedup_stats: ::prost::alloc::vec::Vec<DedupStats>,
    #[prost(message, repeated, tag="17")]
    pub storage_health_events: ::prost::alloc::vec::Vec<StorageHealthEvent>,
    #[prost(message, repeated, tag="18")]
    pub model_feedback_metrics: ::prost::alloc::vec::Vec<ModelFeedbackMetrics>,
    #[prost(message, repeated, tag="19")]
    pub correlation_edges: ::prost::alloc::vec::Vec<AlertCorrelationEdge>,
    #[prost(message, repeated, tag="20")]
    pub notification_events: ::prost::alloc::vec::Vec<NotificationEvent>,
}
// ---------------------------------------------------------------------------
// 业务对象
// ---------------------------------------------------------------------------

/// 不可变计划修订。execution_spec_sha256 只覆盖规范化后的运行字段,
/// 不含 plan_source/selection_origins/created_by/created_at。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisPlanRevision {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub plan_revision: i64,
    #[prost(enumeration="PlanSource", tag="4")]
    pub plan_source: i32,
    #[prost(enumeration="SourceKind", tag="5")]
    pub source_kind: i32,
    /// 采集源/窗口/限额(site 相关字段)
    #[prost(string, tag="6")]
    pub source_spec_json: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="7")]
    pub selected_feature_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="8")]
    pub feature_set_id: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub encrypted_recognition_model_ref: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="10")]
    pub threat_detector_refs: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="11")]
    pub rule_refs: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="12")]
    pub machine_summary_schema_ref: ::prost::alloc::string::String,
    /// 五阶段 DAG(ExecutionNode exact-set)
    #[prost(string, tag="13")]
    pub stage_dag_json: ::prost::alloc::string::String,
    #[prost(string, tag="14")]
    pub completion_policy_json: ::prost::alloc::string::String,
    #[prost(string, tag="15")]
    pub resource_budget_json: ::prost::alloc::string::String,
    #[prost(int64, tag="16")]
    pub catalog_revision: i64,
    #[prost(string, repeated, tag="17")]
    pub selection_origins: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="18")]
    pub canonicalization_version: ::prost::alloc::string::String,
    #[prost(string, tag="19")]
    pub execution_spec_sha256: ::prost::alloc::string::String,
    #[prost(string, tag="20")]
    pub plan_revision_sha256: ::prost::alloc::string::String,
    #[prost(string, tag="21")]
    pub created_by: ::prost::alloc::string::String,
    #[prost(int64, tag="22")]
    pub created_at_ms: i64,
}
/// 不可变调度修订:精确绑定一个已批准 plan revision,不解析"当前 active plan"。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisScheduleRevision {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub revision: i64,
    #[prost(int64, tag="4")]
    pub approved_plan_revision: i64,
    #[prost(string, tag="5")]
    pub execution_spec_sha256: ::prost::alloc::string::String,
    #[prost(enumeration="TriggerKind", tag="6")]
    pub trigger_kind: i32,
    #[prost(string, tag="7")]
    pub timezone: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub window_or_cron_json: ::prost::alloc::string::String,
    #[prost(int64, tag="9")]
    pub prepare_lead_time_ms: i64,
    /// MISFIRE_FAIL|MISFIRE_DELAY|MISFIRE_BOUNDED_REPLAY
    #[prost(string, tag="10")]
    pub misfire_policy: ::prost::alloc::string::String,
    /// FORBID_OVERLAP|ALLOW_OVERLAP
    #[prost(string, tag="11")]
    pub concurrency_policy: ::prost::alloc::string::String,
    #[prost(enumeration="SchedulingClass", tag="12")]
    pub scheduling_class: i32,
    #[prost(string, tag="13")]
    pub resource_restrictions_json: ::prost::alloc::string::String,
    #[prost(string, tag="14")]
    pub schedule_sha256: ::prost::alloc::string::String,
    #[prost(enumeration="ScheduleActivationState", tag="15")]
    pub activation_state: i32,
}
/// 一次确定性触发事实(物化后与 Task 一一对应)。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct TriggerInstance {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub trigger_instance_id: ::prost::alloc::string::String,
    /// actor|schedule|event
    #[prost(string, tag="3")]
    pub identity_kind: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub canonical_identity_hash: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub request_sha256: ::prost::alloc::string::String,
    #[prost(enumeration="TriggerInstanceState", tag="6")]
    pub state: i32,
    #[prost(string, tag="7")]
    pub materialized_task_id: ::prost::alloc::string::String,
    #[prost(enumeration="TriggerKind", tag="8")]
    pub trigger_kind: i32,
    #[prost(string, tag="9")]
    pub window_id: ::prost::alloc::string::String,
    #[prost(int64, tag="10")]
    pub created_at_ms: i64,
}
/// 业务请求:绑定 definition/plan/trigger 与当前 Run。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisTask {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub plan_revision: i64,
    #[prost(string, tag="5")]
    pub execution_spec_sha256: ::prost::alloc::string::String,
    /// on-demand 时为 0
    #[prost(int64, tag="6")]
    pub schedule_revision: i64,
    #[prost(string, tag="7")]
    pub trigger_instance_id: ::prost::alloc::string::String,
    #[prost(enumeration="SchedulingClass", tag="8")]
    pub effective_class: i32,
    #[prost(string, tag="9")]
    pub effective_policy_sha256: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub current_run_id: ::prost::alloc::string::String,
    #[prost(int64, tag="11")]
    pub created_at_ms: i64,
}
/// 一次有界执行尝试;整任务重试创建新 run,不覆写旧终态。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisRun {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub task_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub execution_spec_sha256: ::prost::alloc::string::String,
    #[prost(enumeration="RunState", tag="5")]
    pub state: i32,
    #[prost(enumeration="Completeness", tag="6")]
    pub completeness: i32,
    #[prost(enumeration="IntegrityState", tag="7")]
    pub integrity_state: i32,
    #[prost(enumeration="FindingConclusion", tag="8")]
    pub finding_conclusion: i32,
    #[prost(enumeration="RiskSeverity", tag="9")]
    pub risk_severity: i32,
    #[prost(int64, tag="10")]
    pub window_start_ms: i64,
    #[prost(int64, tag="11")]
    pub window_end_ms: i64,
    #[prost(int64, tag="12")]
    pub revision: i64,
    #[prost(int64, tag="13")]
    pub started_at_ms: i64,
    #[prost(int64, tag="14")]
    pub finalized_at_ms: i64,
    #[prost(int64, tag="15")]
    pub created_at_ms: i64,
    #[prost(string, tag="16")]
    pub cancel_manifest_sha256: ::prost::alloc::string::String,
}
/// 执行节点 attempt(以 business_phase_id + execution_node_id 标识真实节点)。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisStageAttempt {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub stage_attempt_id: ::prost::alloc::string::String,
    /// S1..S5
    #[prost(string, tag="4")]
    pub business_phase_id: ::prost::alloc::string::String,
    /// SESSIONIZATION|FEATURE_EXTRACTION|ENCRYPTED_RECOGNIZER|RULE_DETECTION|BEHAVIOR_DETECTION|DETECTION_AGGREGATE|RECONCILE|MACHINE_FINALIZATION
    #[prost(string, tag="5")]
    pub execution_node_id: ::prost::alloc::string::String,
    #[prost(int32, tag="6")]
    pub attempt: i32,
    #[prost(enumeration="StageAttemptState", tag="7")]
    pub state: i32,
    #[prost(enumeration="ProviderMode", tag="8")]
    pub provider_mode: i32,
    #[prost(enumeration="ActivationMode", tag="9")]
    pub activation_mode: i32,
    #[prost(string, tag="10")]
    pub fencing_token: ::prost::alloc::string::String,
    #[prost(int64, tag="11")]
    pub lease_expires_at_ms: i64,
    #[prost(int64, tag="12")]
    pub started_at_ms: i64,
    #[prost(int64, tag="13")]
    pub finished_at_ms: i64,
    /// OPTIONAL_PREDICATE_FALSE|BLOCKED_BY_UPSTREAM_FAILURE|CANCELLED_BEFORE_DISPATCH|NOT_APPLICABLE_BY_PLAN
    #[prost(string, tag="14")]
    pub skip_reason: ::prost::alloc::string::String,
}
/// 执行器提交的不可变事实。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisStageReceipt {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub execution_node_id: ::prost::alloc::string::String,
    #[prost(int32, tag="4")]
    pub attempt: i32,
    #[prost(string, tag="5")]
    pub fencing_token: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub provider: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub input_count: i64,
    #[prost(int64, tag="8")]
    pub output_count: i64,
    #[prost(int64, tag="9")]
    pub error_count: i64,
    #[prost(int64, tag="10")]
    pub reject_count: i64,
    #[prost(int64, tag="11")]
    pub watermark_ms: i64,
    #[prost(string, tag="12")]
    pub fence_json: ::prost::alloc::string::String,
    #[prost(string, tag="13")]
    pub payload_hash: ::prost::alloc::string::String,
    #[prost(int64, tag="14")]
    pub received_at_ms: i64,
}
/// 每个输入×detector 的 typed outcome。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisResult {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub input_identity: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub detector_id: ::prost::alloc::string::String,
    #[prost(enumeration="DetectorDisposition", tag="5")]
    pub disposition: i32,
    #[prost(double, tag="6")]
    pub score: f64,
    #[prost(string, repeated, tag="7")]
    pub labels: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="8")]
    pub evidence_refs: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(int64, tag="9")]
    pub created_at_ms: i64,
}
/// 机器总体摘要(与终态同事务冻结)。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct MachineAnalysisSummary {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(enumeration="FindingConclusion", tag="3")]
    pub finding_conclusion: i32,
    #[prost(enumeration="RiskSeverity", tag="4")]
    pub risk_severity: i32,
    #[prost(enumeration="Completeness", tag="5")]
    pub completeness: i32,
    #[prost(enumeration="IntegrityState", tag="6")]
    pub integrity_state: i32,
    #[prost(string, tag="7")]
    pub scope_json: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub key_findings_json: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub limitations_json: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub evidence_manifest_hash: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub closure_manifest_hash: ::prost::alloc::string::String,
    #[prost(string, tag="12")]
    pub canonical_sha256: ::prost::alloc::string::String,
    #[prost(int64, tag="13")]
    pub created_at_ms: i64,
}
/// 人读报告(独立 ReportState,失败不回退 Run)。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HumanReadableReport {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub report_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub summary_sha256: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub template_revision: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub locale: ::prost::alloc::string::String,
    #[prost(enumeration="ReportState", tag="7")]
    pub state: i32,
    #[prost(string, tag="8")]
    pub object_key: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub object_sha256: ::prost::alloc::string::String,
    #[prost(int64, tag="10")]
    pub object_size: i64,
    #[prost(int64, tag="11")]
    pub created_at_ms: i64,
    #[prost(int64, tag="12")]
    pub updated_at_ms: i64,
}
// ---------------------------------------------------------------------------
// API 请求/响应(核心端点,卷B §1.2)
// ---------------------------------------------------------------------------

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CreateTaskDefinitionRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub name: ::prost::alloc::string::String,
    #[prost(enumeration="SchedulingClass", tag="3")]
    pub default_scheduling_class: i32,
    #[prost(string, tag="4")]
    pub owner: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CreatePlanRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(enumeration="PlanSource", tag="3")]
    pub plan_source: i32,
    #[prost(enumeration="SourceKind", tag="4")]
    pub source_kind: i32,
    #[prost(string, tag="5")]
    pub source_spec_json: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="6")]
    pub selected_feature_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="7")]
    pub encrypted_recognition_model_ref: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="8")]
    pub threat_detector_refs: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="9")]
    pub rule_refs: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="10")]
    pub completion_policy_json: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub resource_budget_json: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CreateScheduleRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub approved_plan_revision: i64,
    #[prost(enumeration="TriggerKind", tag="4")]
    pub trigger_kind: i32,
    #[prost(string, tag="5")]
    pub timezone: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub window_or_cron_json: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub prepare_lead_time_ms: i64,
    #[prost(string, tag="8")]
    pub misfire_policy: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub concurrency_policy: ::prost::alloc::string::String,
    #[prost(enumeration="SchedulingClass", tag="10")]
    pub scheduling_class: i32,
    #[prost(string, tag="11")]
    pub resource_restrictions_json: ::prost::alloc::string::String,
}
/// 即时分析触发(三步向导最终提交;default/custom 均为主业务链执行环节)。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct SubmitTriggerRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(enumeration="PlanSource", tag="3")]
    pub plan_source: i32,
    /// custom 时才携带;覆盖项=探针/采集源/特征/识别模型/检测模型/规则/阈值
    #[prost(string, tag="4")]
    pub custom_overrides_json: ::prost::alloc::string::String,
    #[prost(enumeration="SourceKind", tag="5")]
    pub source_kind: i32,
    /// PCAP_REPLAY 必填 object_ref+window
    #[prost(string, tag="6")]
    pub source_spec_json: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub client_idempotency_key: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct SubmitTriggerResponse {
    #[prost(string, tag="1")]
    pub task_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub status_url: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub execution_spec_sha256: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListRunsRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub task_definition_id: ::prost::alloc::string::String,
    #[prost(enumeration="RunState", tag="3")]
    pub state: i32,
    #[prost(enumeration="SchedulingClass", tag="4")]
    pub scheduling_class: i32,
    #[prost(int64, tag="5")]
    pub window_start_ms: i64,
    #[prost(int64, tag="6")]
    pub window_end_ms: i64,
    #[prost(int32, tag="7")]
    pub limit: i32,
    #[prost(string, tag="8")]
    pub cursor: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListRunsResponse {
    #[prost(message, repeated, tag="1")]
    pub runs: ::prost::alloc::vec::Vec<AnalysisRun>,
    #[prost(string, tag="2")]
    pub next_cursor: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub total: i64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GetRunResponse {
    #[prost(message, optional, tag="1")]
    pub run: ::core::option::Option<AnalysisRun>,
    #[prost(message, repeated, tag="2")]
    pub stages: ::prost::alloc::vec::Vec<AnalysisStageAttempt>,
    #[prost(message, optional, tag="3")]
    pub summary: ::core::option::Option<MachineAnalysisSummary>,
    #[prost(message, optional, tag="4")]
    pub report: ::core::option::Option<HumanReadableReport>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListRunResultsRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(enumeration="DetectorDisposition", tag="3")]
    pub disposition: i32,
    #[prost(int32, tag="4")]
    pub limit: i32,
    #[prost(string, tag="5")]
    pub cursor: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListRunResultsResponse {
    #[prost(message, repeated, tag="1")]
    pub results: ::prost::alloc::vec::Vec<AnalysisResult>,
    #[prost(string, tag="2")]
    pub next_cursor: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub total: i64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CancelRunRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub client_idempotency_key: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RetryRunRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub client_idempotency_key: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RequestReportRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub template_revision: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub locale: ::prost::alloc::string::String,
}
/// 通用回执。
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AnalysisOperationReceipt {
    #[prost(string, tag="1")]
    pub operation_id: ::prost::alloc::string::String,
    /// accepted|running|succeeded|failed|cancelled
    #[prost(string, tag="2")]
    pub state: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub revision: i64,
    #[prost(string, tag="4")]
    pub status_url: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub error_code: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub error_message: ::prost::alloc::string::String,
}
// ---------------------------------------------------------------------------
// 枚举(0=UNSPECIFIED,unknown fail closed)
// ---------------------------------------------------------------------------

/// 计划参数来源:只回答"计划参数由批准默认值还是授权人工覆盖准备",
/// 不决定触发方式、容量、拓扑(与 TriggerKind 正交)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum PlanSource {
    Unspecified = 0,
    AutoDefault = 1,
    ManualCustom = 2,
}
impl PlanSource {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            PlanSource::Unspecified => "PLAN_SOURCE_UNSPECIFIED",
            PlanSource::AutoDefault => "PLAN_SOURCE_AUTO_DEFAULT",
            PlanSource::ManualCustom => "PLAN_SOURCE_MANUAL_CUSTOM",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "PLAN_SOURCE_UNSPECIFIED" => Some(Self::Unspecified),
            "PLAN_SOURCE_AUTO_DEFAULT" => Some(Self::AutoDefault),
            "PLAN_SOURCE_MANUAL_CUSTOM" => Some(Self::ManualCustom),
            _ => None,
        }
    }
}
/// 触发方式(与 PlanSource 正交,任一来源可绑定任一触发)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TriggerKind {
    Unspecified = 0,
    ContinuousWindow = 1,
    CronWindow = 2,
    EventDriven = 3,
    OnDemand = 4,
}
impl TriggerKind {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TriggerKind::Unspecified => "TRIGGER_KIND_UNSPECIFIED",
            TriggerKind::ContinuousWindow => "TRIGGER_KIND_CONTINUOUS_WINDOW",
            TriggerKind::CronWindow => "TRIGGER_KIND_CRON_WINDOW",
            TriggerKind::EventDriven => "TRIGGER_KIND_EVENT_DRIVEN",
            TriggerKind::OnDemand => "TRIGGER_KIND_ON_DEMAND",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TRIGGER_KIND_UNSPECIFIED" => Some(Self::Unspecified),
            "TRIGGER_KIND_CONTINUOUS_WINDOW" => Some(Self::ContinuousWindow),
            "TRIGGER_KIND_CRON_WINDOW" => Some(Self::CronWindow),
            "TRIGGER_KIND_EVENT_DRIVEN" => Some(Self::EventDriven),
            "TRIGGER_KIND_ON_DEMAND" => Some(Self::OnDemand),
            _ => None,
        }
    }
}
/// 数据源类型。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum SourceKind {
    Unspecified = 0,
    LiveStreamWindow = 1,
    ProbeCaptureWindow = 2,
    PcapReplay = 3,
}
impl SourceKind {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            SourceKind::Unspecified => "SOURCE_KIND_UNSPECIFIED",
            SourceKind::LiveStreamWindow => "SOURCE_KIND_LIVE_STREAM_WINDOW",
            SourceKind::ProbeCaptureWindow => "SOURCE_KIND_PROBE_CAPTURE_WINDOW",
            SourceKind::PcapReplay => "SOURCE_KIND_PCAP_REPLAY",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "SOURCE_KIND_UNSPECIFIED" => Some(Self::Unspecified),
            "SOURCE_KIND_LIVE_STREAM_WINDOW" => Some(Self::LiveStreamWindow),
            "SOURCE_KIND_PROBE_CAPTURE_WINDOW" => Some(Self::ProbeCaptureWindow),
            "SOURCE_KIND_PCAP_REPLAY" => Some(Self::PcapReplay),
            _ => None,
        }
    }
}
/// 调度类别。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum SchedulingClass {
    Unspecified = 0,
    Baseline = 1,
    Interactive = 2,
    Acceptance = 3,
}
impl SchedulingClass {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            SchedulingClass::Unspecified => "SCHEDULING_CLASS_UNSPECIFIED",
            SchedulingClass::Baseline => "SCHEDULING_CLASS_BASELINE",
            SchedulingClass::Interactive => "SCHEDULING_CLASS_INTERACTIVE",
            SchedulingClass::Acceptance => "SCHEDULING_CLASS_ACCEPTANCE",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "SCHEDULING_CLASS_UNSPECIFIED" => Some(Self::Unspecified),
            "SCHEDULING_CLASS_BASELINE" => Some(Self::Baseline),
            "SCHEDULING_CLASS_INTERACTIVE" => Some(Self::Interactive),
            "SCHEDULING_CLASS_ACCEPTANCE" => Some(Self::Acceptance),
            _ => None,
        }
    }
}
/// 任务定义状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TaskDefinitionState {
    Unspecified = 0,
    Draft = 1,
    Validated = 2,
    Active = 3,
    Suspended = 4,
    Retired = 5,
}
impl TaskDefinitionState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TaskDefinitionState::Unspecified => "TASK_DEFINITION_STATE_UNSPECIFIED",
            TaskDefinitionState::Draft => "TASK_DEFINITION_STATE_DRAFT",
            TaskDefinitionState::Validated => "TASK_DEFINITION_STATE_VALIDATED",
            TaskDefinitionState::Active => "TASK_DEFINITION_STATE_ACTIVE",
            TaskDefinitionState::Suspended => "TASK_DEFINITION_STATE_SUSPENDED",
            TaskDefinitionState::Retired => "TASK_DEFINITION_STATE_RETIRED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TASK_DEFINITION_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "TASK_DEFINITION_STATE_DRAFT" => Some(Self::Draft),
            "TASK_DEFINITION_STATE_VALIDATED" => Some(Self::Validated),
            "TASK_DEFINITION_STATE_ACTIVE" => Some(Self::Active),
            "TASK_DEFINITION_STATE_SUSPENDED" => Some(Self::Suspended),
            "TASK_DEFINITION_STATE_RETIRED" => Some(Self::Retired),
            _ => None,
        }
    }
}
/// 计划治理头状态(CAS 推进)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum PlanGovernanceState {
    Unspecified = 0,
    Draft = 1,
    Validated = 2,
    Approved = 3,
    Active = 4,
    Retired = 5,
}
impl PlanGovernanceState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            PlanGovernanceState::Unspecified => "PLAN_GOVERNANCE_STATE_UNSPECIFIED",
            PlanGovernanceState::Draft => "PLAN_GOVERNANCE_STATE_DRAFT",
            PlanGovernanceState::Validated => "PLAN_GOVERNANCE_STATE_VALIDATED",
            PlanGovernanceState::Approved => "PLAN_GOVERNANCE_STATE_APPROVED",
            PlanGovernanceState::Active => "PLAN_GOVERNANCE_STATE_ACTIVE",
            PlanGovernanceState::Retired => "PLAN_GOVERNANCE_STATE_RETIRED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "PLAN_GOVERNANCE_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "PLAN_GOVERNANCE_STATE_DRAFT" => Some(Self::Draft),
            "PLAN_GOVERNANCE_STATE_VALIDATED" => Some(Self::Validated),
            "PLAN_GOVERNANCE_STATE_APPROVED" => Some(Self::Approved),
            "PLAN_GOVERNANCE_STATE_ACTIVE" => Some(Self::Active),
            "PLAN_GOVERNANCE_STATE_RETIRED" => Some(Self::Retired),
            _ => None,
        }
    }
}
/// 调度激活头状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum ScheduleActivationState {
    Unspecified = 0,
    Draft = 1,
    Active = 2,
    Paused = 3,
    Retired = 4,
}
impl ScheduleActivationState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            ScheduleActivationState::Unspecified => "SCHEDULE_ACTIVATION_STATE_UNSPECIFIED",
            ScheduleActivationState::Draft => "SCHEDULE_ACTIVATION_STATE_DRAFT",
            ScheduleActivationState::Active => "SCHEDULE_ACTIVATION_STATE_ACTIVE",
            ScheduleActivationState::Paused => "SCHEDULE_ACTIVATION_STATE_PAUSED",
            ScheduleActivationState::Retired => "SCHEDULE_ACTIVATION_STATE_RETIRED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "SCHEDULE_ACTIVATION_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "SCHEDULE_ACTIVATION_STATE_DRAFT" => Some(Self::Draft),
            "SCHEDULE_ACTIVATION_STATE_ACTIVE" => Some(Self::Active),
            "SCHEDULE_ACTIVATION_STATE_PAUSED" => Some(Self::Paused),
            "SCHEDULE_ACTIVATION_STATE_RETIRED" => Some(Self::Retired),
            _ => None,
        }
    }
}
/// 触发实例状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum TriggerInstanceState {
    Unspecified = 0,
    PendingMaterialization = 1,
    Materialized = 2,
    Suppressed = 3,
    Quarantined = 4,
}
impl TriggerInstanceState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            TriggerInstanceState::Unspecified => "TRIGGER_INSTANCE_STATE_UNSPECIFIED",
            TriggerInstanceState::PendingMaterialization => "TRIGGER_INSTANCE_STATE_PENDING_MATERIALIZATION",
            TriggerInstanceState::Materialized => "TRIGGER_INSTANCE_STATE_MATERIALIZED",
            TriggerInstanceState::Suppressed => "TRIGGER_INSTANCE_STATE_SUPPRESSED",
            TriggerInstanceState::Quarantined => "TRIGGER_INSTANCE_STATE_QUARANTINED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "TRIGGER_INSTANCE_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "TRIGGER_INSTANCE_STATE_PENDING_MATERIALIZATION" => Some(Self::PendingMaterialization),
            "TRIGGER_INSTANCE_STATE_MATERIALIZED" => Some(Self::Materialized),
            "TRIGGER_INSTANCE_STATE_SUPPRESSED" => Some(Self::Suppressed),
            "TRIGGER_INSTANCE_STATE_QUARANTINED" => Some(Self::Quarantined),
            _ => None,
        }
    }
}
/// Run 状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum RunState {
    Unspecified = 0,
    Accepted = 1,
    Preparing = 2,
    Queued = 3,
    Running = 4,
    Finalizing = 5,
    Succeeded = 6,
    PartiallySucceeded = 7,
    Failed = 8,
    CancelRequested = 9,
    Cancelled = 10,
}
impl RunState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            RunState::Unspecified => "RUN_STATE_UNSPECIFIED",
            RunState::Accepted => "RUN_STATE_ACCEPTED",
            RunState::Preparing => "RUN_STATE_PREPARING",
            RunState::Queued => "RUN_STATE_QUEUED",
            RunState::Running => "RUN_STATE_RUNNING",
            RunState::Finalizing => "RUN_STATE_FINALIZING",
            RunState::Succeeded => "RUN_STATE_SUCCEEDED",
            RunState::PartiallySucceeded => "RUN_STATE_PARTIALLY_SUCCEEDED",
            RunState::Failed => "RUN_STATE_FAILED",
            RunState::CancelRequested => "RUN_STATE_CANCEL_REQUESTED",
            RunState::Cancelled => "RUN_STATE_CANCELLED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "RUN_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "RUN_STATE_ACCEPTED" => Some(Self::Accepted),
            "RUN_STATE_PREPARING" => Some(Self::Preparing),
            "RUN_STATE_QUEUED" => Some(Self::Queued),
            "RUN_STATE_RUNNING" => Some(Self::Running),
            "RUN_STATE_FINALIZING" => Some(Self::Finalizing),
            "RUN_STATE_SUCCEEDED" => Some(Self::Succeeded),
            "RUN_STATE_PARTIALLY_SUCCEEDED" => Some(Self::PartiallySucceeded),
            "RUN_STATE_FAILED" => Some(Self::Failed),
            "RUN_STATE_CANCEL_REQUESTED" => Some(Self::CancelRequested),
            "RUN_STATE_CANCELLED" => Some(Self::Cancelled),
            _ => None,
        }
    }
}
/// 执行节点 attempt 状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum StageAttemptState {
    Unspecified = 0,
    Pending = 1,
    Dispatched = 2,
    Running = 3,
    Succeeded = 4,
    Partial = 5,
    Failed = 6,
    CancelRequested = 7,
    Cancelled = 8,
    Skipped = 9,
}
impl StageAttemptState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            StageAttemptState::Unspecified => "STAGE_ATTEMPT_STATE_UNSPECIFIED",
            StageAttemptState::Pending => "STAGE_ATTEMPT_STATE_PENDING",
            StageAttemptState::Dispatched => "STAGE_ATTEMPT_STATE_DISPATCHED",
            StageAttemptState::Running => "STAGE_ATTEMPT_STATE_RUNNING",
            StageAttemptState::Succeeded => "STAGE_ATTEMPT_STATE_SUCCEEDED",
            StageAttemptState::Partial => "STAGE_ATTEMPT_STATE_PARTIAL",
            StageAttemptState::Failed => "STAGE_ATTEMPT_STATE_FAILED",
            StageAttemptState::CancelRequested => "STAGE_ATTEMPT_STATE_CANCEL_REQUESTED",
            StageAttemptState::Cancelled => "STAGE_ATTEMPT_STATE_CANCELLED",
            StageAttemptState::Skipped => "STAGE_ATTEMPT_STATE_SKIPPED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "STAGE_ATTEMPT_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "STAGE_ATTEMPT_STATE_PENDING" => Some(Self::Pending),
            "STAGE_ATTEMPT_STATE_DISPATCHED" => Some(Self::Dispatched),
            "STAGE_ATTEMPT_STATE_RUNNING" => Some(Self::Running),
            "STAGE_ATTEMPT_STATE_SUCCEEDED" => Some(Self::Succeeded),
            "STAGE_ATTEMPT_STATE_PARTIAL" => Some(Self::Partial),
            "STAGE_ATTEMPT_STATE_FAILED" => Some(Self::Failed),
            "STAGE_ATTEMPT_STATE_CANCEL_REQUESTED" => Some(Self::CancelRequested),
            "STAGE_ATTEMPT_STATE_CANCELLED" => Some(Self::Cancelled),
            "STAGE_ATTEMPT_STATE_SKIPPED" => Some(Self::Skipped),
            _ => None,
        }
    }
}
/// 检测器对单个输入×required detector 的 typed outcome(无消息≠阴性)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum DetectorDisposition {
    Unspecified = 0,
    Positive = 1,
    Negative = 2,
    Inconclusive = 3,
    Incompatible = 4,
    Error = 5,
    NotRun = 6,
}
impl DetectorDisposition {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            DetectorDisposition::Unspecified => "DETECTOR_DISPOSITION_UNSPECIFIED",
            DetectorDisposition::Positive => "DETECTOR_DISPOSITION_POSITIVE",
            DetectorDisposition::Negative => "DETECTOR_DISPOSITION_NEGATIVE",
            DetectorDisposition::Inconclusive => "DETECTOR_DISPOSITION_INCONCLUSIVE",
            DetectorDisposition::Incompatible => "DETECTOR_DISPOSITION_INCOMPATIBLE",
            DetectorDisposition::Error => "DETECTOR_DISPOSITION_ERROR",
            DetectorDisposition::NotRun => "DETECTOR_DISPOSITION_NOT_RUN",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "DETECTOR_DISPOSITION_UNSPECIFIED" => Some(Self::Unspecified),
            "DETECTOR_DISPOSITION_POSITIVE" => Some(Self::Positive),
            "DETECTOR_DISPOSITION_NEGATIVE" => Some(Self::Negative),
            "DETECTOR_DISPOSITION_INCONCLUSIVE" => Some(Self::Inconclusive),
            "DETECTOR_DISPOSITION_INCOMPATIBLE" => Some(Self::Incompatible),
            "DETECTOR_DISPOSITION_ERROR" => Some(Self::Error),
            "DETECTOR_DISPOSITION_NOT_RUN" => Some(Self::NotRun),
            _ => None,
        }
    }
}
/// Run 总体机器结论(与 DetectorDisposition 分层)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum FindingConclusion {
    Unspecified = 0,
    ThreatFound = 1,
    NoThreatObserved = 2,
    Inconclusive = 3,
    NoData = 4,
    NotEvaluated = 5,
}
impl FindingConclusion {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            FindingConclusion::Unspecified => "FINDING_CONCLUSION_UNSPECIFIED",
            FindingConclusion::ThreatFound => "FINDING_CONCLUSION_THREAT_FOUND",
            FindingConclusion::NoThreatObserved => "FINDING_CONCLUSION_NO_THREAT_OBSERVED",
            FindingConclusion::Inconclusive => "FINDING_CONCLUSION_INCONCLUSIVE",
            FindingConclusion::NoData => "FINDING_CONCLUSION_NO_DATA",
            FindingConclusion::NotEvaluated => "FINDING_CONCLUSION_NOT_EVALUATED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "FINDING_CONCLUSION_UNSPECIFIED" => Some(Self::Unspecified),
            "FINDING_CONCLUSION_THREAT_FOUND" => Some(Self::ThreatFound),
            "FINDING_CONCLUSION_NO_THREAT_OBSERVED" => Some(Self::NoThreatObserved),
            "FINDING_CONCLUSION_INCONCLUSIVE" => Some(Self::Inconclusive),
            "FINDING_CONCLUSION_NO_DATA" => Some(Self::NoData),
            "FINDING_CONCLUSION_NOT_EVALUATED" => Some(Self::NotEvaluated),
            _ => None,
        }
    }
}
/// 风险严重度(只描述已观察发现,不参与 RunState/FindingConclusion 判定)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum RiskSeverity {
    Unspecified = 0,
    Critical = 1,
    High = 2,
    Medium = 3,
    Low = 4,
    None = 5,
    Unknown = 6,
}
impl RiskSeverity {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            RiskSeverity::Unspecified => "RISK_SEVERITY_UNSPECIFIED",
            RiskSeverity::Critical => "RISK_SEVERITY_CRITICAL",
            RiskSeverity::High => "RISK_SEVERITY_HIGH",
            RiskSeverity::Medium => "RISK_SEVERITY_MEDIUM",
            RiskSeverity::Low => "RISK_SEVERITY_LOW",
            RiskSeverity::None => "RISK_SEVERITY_NONE",
            RiskSeverity::Unknown => "RISK_SEVERITY_UNKNOWN",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "RISK_SEVERITY_UNSPECIFIED" => Some(Self::Unspecified),
            "RISK_SEVERITY_CRITICAL" => Some(Self::Critical),
            "RISK_SEVERITY_HIGH" => Some(Self::High),
            "RISK_SEVERITY_MEDIUM" => Some(Self::Medium),
            "RISK_SEVERITY_LOW" => Some(Self::Low),
            "RISK_SEVERITY_NONE" => Some(Self::None),
            "RISK_SEVERITY_UNKNOWN" => Some(Self::Unknown),
            _ => None,
        }
    }
}
/// 完整性(与结论确定性正交)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum Completeness {
    Unspecified = 0,
    Complete = 1,
    Partial = 2,
    Incomplete = 3,
    Unknown = 4,
}
impl Completeness {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            Completeness::Unspecified => "COMPLETENESS_UNSPECIFIED",
            Completeness::Complete => "COMPLETENESS_COMPLETE",
            Completeness::Partial => "COMPLETENESS_PARTIAL",
            Completeness::Incomplete => "COMPLETENESS_INCOMPLETE",
            Completeness::Unknown => "COMPLETENESS_UNKNOWN",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "COMPLETENESS_UNSPECIFIED" => Some(Self::Unspecified),
            "COMPLETENESS_COMPLETE" => Some(Self::Complete),
            "COMPLETENESS_PARTIAL" => Some(Self::Partial),
            "COMPLETENESS_INCOMPLETE" => Some(Self::Incomplete),
            "COMPLETENESS_UNKNOWN" => Some(Self::Unknown),
            _ => None,
        }
    }
}
/// 证据完整性状态。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum IntegrityState {
    Unspecified = 0,
    Verified = 1,
    Unverified = 2,
    Failed = 3,
}
impl IntegrityState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            IntegrityState::Unspecified => "INTEGRITY_STATE_UNSPECIFIED",
            IntegrityState::Verified => "INTEGRITY_STATE_VERIFIED",
            IntegrityState::Unverified => "INTEGRITY_STATE_UNVERIFIED",
            IntegrityState::Failed => "INTEGRITY_STATE_FAILED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "INTEGRITY_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "INTEGRITY_STATE_VERIFIED" => Some(Self::Verified),
            "INTEGRITY_STATE_UNVERIFIED" => Some(Self::Unverified),
            "INTEGRITY_STATE_FAILED" => Some(Self::Failed),
            _ => None,
        }
    }
}
/// 人读报告状态(独立于 RunState)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum ReportState {
    Unspecified = 0,
    NotRequested = 1,
    Queued = 2,
    Generating = 3,
    Verifying = 4,
    Available = 5,
    Failed = 6,
    Cancelled = 7,
}
impl ReportState {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            ReportState::Unspecified => "REPORT_STATE_UNSPECIFIED",
            ReportState::NotRequested => "REPORT_STATE_NOT_REQUESTED",
            ReportState::Queued => "REPORT_STATE_QUEUED",
            ReportState::Generating => "REPORT_STATE_GENERATING",
            ReportState::Verifying => "REPORT_STATE_VERIFYING",
            ReportState::Available => "REPORT_STATE_AVAILABLE",
            ReportState::Failed => "REPORT_STATE_FAILED",
            ReportState::Cancelled => "REPORT_STATE_CANCELLED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "REPORT_STATE_UNSPECIFIED" => Some(Self::Unspecified),
            "REPORT_STATE_NOT_REQUESTED" => Some(Self::NotRequested),
            "REPORT_STATE_QUEUED" => Some(Self::Queued),
            "REPORT_STATE_GENERATING" => Some(Self::Generating),
            "REPORT_STATE_VERIFYING" => Some(Self::Verifying),
            "REPORT_STATE_AVAILABLE" => Some(Self::Available),
            "REPORT_STATE_FAILED" => Some(Self::Failed),
            "REPORT_STATE_CANCELLED" => Some(Self::Cancelled),
            _ => None,
        }
    }
}
/// provider/activation 模式(执行节点契约)。
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum ProviderMode {
    Unspecified = 0,
    SharedStream = 1,
    DedicatedOperation = 2,
    AuthorityLocal = 3,
}
impl ProviderMode {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            ProviderMode::Unspecified => "PROVIDER_MODE_UNSPECIFIED",
            ProviderMode::SharedStream => "PROVIDER_MODE_SHARED_STREAM",
            ProviderMode::DedicatedOperation => "PROVIDER_MODE_DEDICATED_OPERATION",
            ProviderMode::AuthorityLocal => "PROVIDER_MODE_AUTHORITY_LOCAL",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "PROVIDER_MODE_UNSPECIFIED" => Some(Self::Unspecified),
            "PROVIDER_MODE_SHARED_STREAM" => Some(Self::SharedStream),
            "PROVIDER_MODE_DEDICATED_OPERATION" => Some(Self::DedicatedOperation),
            "PROVIDER_MODE_AUTHORITY_LOCAL" => Some(Self::AuthorityLocal),
            _ => None,
        }
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum ActivationMode {
    Unspecified = 0,
    PipelinedStream = 1,
    AfterUpstreamClose = 2,
    AuthorityLocal = 3,
}
impl ActivationMode {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            ActivationMode::Unspecified => "ACTIVATION_MODE_UNSPECIFIED",
            ActivationMode::PipelinedStream => "ACTIVATION_MODE_PIPELINED_STREAM",
            ActivationMode::AfterUpstreamClose => "ACTIVATION_MODE_AFTER_UPSTREAM_CLOSE",
            ActivationMode::AuthorityLocal => "ACTIVATION_MODE_AUTHORITY_LOCAL",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "ACTIVATION_MODE_UNSPECIFIED" => Some(Self::Unspecified),
            "ACTIVATION_MODE_PIPELINED_STREAM" => Some(Self::PipelinedStream),
            "ACTIVATION_MODE_AFTER_UPSTREAM_CLOSE" => Some(Self::AfterUpstreamClose),
            "ACTIVATION_MODE_AUTHORITY_LOCAL" => Some(Self::AuthorityLocal),
            _ => None,
        }
    }
}
/// Asset represents a network device discovered through passive or active means.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Asset {
    /// UUID
    #[prost(string, tag="1")]
    pub asset_id: ::prost::alloc::string::String,
    /// 租户 ID
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    /// IPv4/IPv6
    #[prost(string, tag="3")]
    pub ip_address: ::prost::alloc::string::String,
    /// MAC (canonical form xx:xx:xx:xx:xx:xx)
    #[prost(string, tag="4")]
    pub mac_address: ::prost::alloc::string::String,
    /// hostname (DHCP/DNS/LLMNR)
    #[prost(string, tag="5")]
    pub hostname: ::prost::alloc::string::String,
    /// OUI vendor name
    #[prost(string, tag="6")]
    pub vendor: ::prost::alloc::string::String,
    /// OS fingerprint (DHCP option 60, HTTP UA)
    #[prost(string, tag="7")]
    pub os_type: ::prost::alloc::string::String,
    /// discovery source: arp/dhcp/dns/lldp/snmp/manual
    #[prost(string, tag="8")]
    pub source: ::prost::alloc::string::String,
    /// unix ms
    #[prost(int64, tag="9")]
    pub first_seen: i64,
    /// unix ms
    #[prost(int64, tag="10")]
    pub last_seen: i64,
    /// VLAN tag
    #[prost(string, tag="11")]
    pub vlan_id: ::prost::alloc::string::String,
    /// switch port from LLDP/SNMP
    #[prost(string, tag="12")]
    pub switch_port: ::prost::alloc::string::String,
}
/// AssetEvent records changes to an asset for audit trail.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AssetEvent {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub asset_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    /// first_seen, ip_changed, mac_changed, inactive, reactivated
    #[prost(string, tag="4")]
    pub event_type: ::prost::alloc::string::String,
    /// JSON of previous state
    #[prost(string, tag="5")]
    pub old_value: ::prost::alloc::string::String,
    /// JSON of new state
    #[prost(string, tag="6")]
    pub new_value: ::prost::alloc::string::String,
    /// unix ms
    #[prost(int64, tag="7")]
    pub created_at: i64,
}
/// MAC→IP binding learned from ARP/DHCP traffic.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct MacIpBinding {
    #[prost(string, tag="1")]
    pub mac_address: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub ip_address: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    /// unix ms
    #[prost(int64, tag="4")]
    pub observed_at: i64,
    /// arp or dhcp
    #[prost(string, tag="5")]
    pub source: ::prost::alloc::string::String,
    /// Stable probe-local identity. Retries must retain this value and payload.
    #[prost(string, tag="6")]
    pub observation_id: ::prost::alloc::string::String,
    /// Authenticated gateway identity; a non-empty mismatching value is rejected.
    #[prost(string, tag="7")]
    pub probe_id: ::prost::alloc::string::String,
    /// Observation scope. Empty remains compatible with untagged legacy traffic.
    #[prost(string, tag="8")]
    pub vlan_id: ::prost::alloc::string::String,
    /// Packet/parser identity used for audit correlation, not as Kafka authority.
    #[prost(string, tag="9")]
    pub source_event_id: ::prost::alloc::string::String,
    /// Additive payload contract revision. New probe uploads use value 1.
    #[prost(uint32, tag="10")]
    pub schema_version: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UpsertAssetRequest {
    #[prost(message, optional, tag="1")]
    pub asset: ::core::option::Option<Asset>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UpsertAssetResponse {
    #[prost(string, tag="1")]
    pub asset_id: ::prost::alloc::string::String,
    /// true if new, false if updated
    #[prost(bool, tag="2")]
    pub created: bool,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GetAssetRequest {
    #[prost(string, tag="1")]
    pub asset_id: ::prost::alloc::string::String,
    /// alternative lookup key
    #[prost(string, tag="2")]
    pub mac_address: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GetAssetResponse {
    #[prost(message, optional, tag="1")]
    pub asset: ::core::option::Option<Asset>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListAssetsRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int32, tag="2")]
    pub page_size: i32,
    #[prost(string, tag="3")]
    pub page_token: ::prost::alloc::string::String,
    /// optional filter
    #[prost(string, tag="4")]
    pub ip_prefix: ::prost::alloc::string::String,
    /// optional filter
    #[prost(string, tag="5")]
    pub vendor_filter: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ListAssetsResponse {
    #[prost(message, repeated, tag="1")]
    pub assets: ::prost::alloc::vec::Vec<Asset>,
    #[prost(string, tag="2")]
    pub next_page_token: ::prost::alloc::string::String,
    #[prost(int32, tag="3")]
    pub total_count: i32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RecordMacIpBindingRequest {
    #[prost(message, repeated, tag="1")]
    pub bindings: ::prost::alloc::vec::Vec<MacIpBinding>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RecordMacIpBindingResponse {
    #[prost(int32, tag="1")]
    pub accepted: i32,
    #[prost(int32, tag="2")]
    pub rejected: i32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GetAssetHistoryRequest {
    #[prost(string, tag="1")]
    pub asset_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int32, tag="3")]
    pub page_size: i32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GetAssetHistoryResponse {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<AssetEvent>,
}
/// AuditLog — 审计事件 (Kafka: audit.logs)
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AuditLog {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub user_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub action: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub object_type: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub object_id: ::prost::alloc::string::String,
    /// JSON
    #[prost(string, tag="7")]
    pub detail: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub ip_addr: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub user_agent: ::prost::alloc::string::String,
    /// unix ms
    #[prost(int64, tag="10")]
    pub created_at: i64,
}
/// UserEvent — 用户行为 (Kafka: user.events.v1)
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UserEvent {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub user_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub username: ::prost::alloc::string::String,
    /// login/logout/token_refresh/api_access
    #[prost(string, tag="5")]
    pub event_type: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub source_ip: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub user_agent: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub resource: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub action: ::prost::alloc::string::String,
    /// success/denied/error
    #[prost(string, tag="10")]
    pub result: ::prost::alloc::string::String,
    /// unix ms
    #[prost(int64, tag="11")]
    pub timestamp: i64,
}
/// DeviceLog — 设备日志 (Kafka: device.logs.v1)
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DeviceLog {
    #[prost(string, tag="1")]
    pub log_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub device_ip: ::prost::alloc::string::String,
    /// switch/router/firewall/server
    #[prost(string, tag="4")]
    pub device_type: ::prost::alloc::string::String,
    #[prost(uint32, tag="5")]
    pub facility: u32,
    #[prost(uint32, tag="6")]
    pub severity: u32,
    /// unix ms
    #[prost(int64, tag="7")]
    pub timestamp: i64,
    #[prost(string, tag="8")]
    pub message: ::prost::alloc::string::String,
    /// JSON, structured parse result
    #[prost(string, tag="9")]
    pub parsed: ::prost::alloc::string::String,
    /// syslog/snmp_trap/netflow
    #[prost(string, tag="10")]
    pub source: ::prost::alloc::string::String,
}
/// DeadLetter — 死信队列 (Kafka: dlq.v1)
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DeadLetter {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub source_topic: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub source_key: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub error_msg: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub raw_payload: ::prost::alloc::string::String,
    #[prost(uint32, tag="7")]
    pub retry_count: u32,
    /// unix ms
    #[prost(int64, tag="8")]
    pub created_at: i64,
}
/// AuditLogBatch / UserEventBatch / DeviceLogBatch / DeadLetterBatch
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AuditLogBatch {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<AuditLog>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UserEventBatch {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<UserEvent>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DeviceLogBatch {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<DeviceLog>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DeadLetterBatch {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<DeadLetter>,
}
/// CampaignBatch 战役批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CampaignBatch {
    #[prost(message, repeated, tag="1")]
    pub campaigns: ::prost::alloc::vec::Vec<Campaign>,
    #[prost(string, tag="2")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub created_at: i64,
}
/// CampaignQuery 战役查询请求
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CampaignQuery {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub campaign_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub start_time: i64,
    #[prost(int64, tag="4")]
    pub end_time: i64,
    #[prost(string, repeated, tag="5")]
    pub campaign_types: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// CampaignQueryResponse 战役查询响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CampaignQueryResponse {
    #[prost(message, repeated, tag="1")]
    pub campaigns: ::prost::alloc::vec::Vec<Campaign>,
    #[prost(int32, tag="2")]
    pub total_count: i32,
    #[prost(bool, tag="3")]
    pub has_more: bool,
}
/// DetectionBehavior 行为检测结果
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DetectionBehavior {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub object_type: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub object_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub ts: i64,
    #[prost(string, repeated, tag="7")]
    pub labels: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(float, repeated, tag="8")]
    pub scores: ::prost::alloc::vec::Vec<f32>,
    #[prost(string, tag="9")]
    pub top_label: ::prost::alloc::string::String,
    #[prost(float, tag="10")]
    pub top_score: f32,
    /// Additive source context: consumers must not manufacture an empty tuple or
    /// evidence list when producing an alert projection.
    #[prost(message, optional, tag="11")]
    pub tuple: ::core::option::Option<FiveTuple>,
    #[prost(string, repeated, tag="12")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// DetectionBusiness 业务检测结果
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DetectionBusiness {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub rule_version: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub ts: i64,
    #[prost(string, tag="5")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub session_id: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub campaign_id: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub detection_type: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub label: ::prost::alloc::string::String,
    #[prost(float, tag="10")]
    pub score: f32,
}
/// DetectionBatch 检测结果批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DetectionBatch {
    #[prost(message, repeated, tag="1")]
    pub behaviors: ::prost::alloc::vec::Vec<DetectionBehavior>,
    #[prost(message, repeated, tag="2")]
    pub businesses: ::prost::alloc::vec::Vec<DetectionBusiness>,
    #[prost(string, tag="3")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub created_at: i64,
}
/// FeatureStat L1 统计特征
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureStat {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub schema_version: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub object_type: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub object_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub ts: i64,
    #[prost(uint32, tag="7")]
    pub protocol: u32,
    #[prost(uint32, tag="8")]
    pub duration_ms: u32,
    #[prost(float, tag="9")]
    pub pps: f32,
    #[prost(float, tag="10")]
    pub bps: f32,
    #[prost(float, tag="11")]
    pub up_down_ratio: f32,
    #[prost(float, tag="12")]
    pub pktlen_mean: f32,
    #[prost(float, tag="13")]
    pub pktlen_std: f32,
    #[prost(float, tag="14")]
    pub iat_mean_ms: f32,
    #[prost(float, tag="15")]
    pub iat_std_ms: f32,
    #[prost(float, tag="16")]
    pub active_mean_ms: f32,
    #[prost(float, tag="17")]
    pub idle_mean_ms: f32,
    #[prost(uint32, tag="18")]
    pub tcp_flag_syn_cnt: u32,
    #[prost(uint32, tag="19")]
    pub tcp_flag_ack_cnt: u32,
    #[prost(uint32, tag="20")]
    pub tcp_init_win_bytes_fwd: u32,
    #[prost(uint32, tag="21")]
    pub tcp_init_win_bytes_bwd: u32,
    #[prost(float, repeated, tag="22")]
    pub extra: ::prost::alloc::vec::Vec<f32>,
    /// Original session context required by downstream detections and evidence.
    #[prost(message, optional, tag="23")]
    pub tuple: ::core::option::Option<FiveTuple>,
    #[prost(string, repeated, tag="24")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(enumeration="FeatureCategory", tag="25")]
    pub feature_category: i32,
    #[prost(enumeration="FeatureAvailability", tag="26")]
    pub availability: i32,
    #[prost(string, tag="27")]
    pub algorithm_version: ::prost::alloc::string::String,
    #[prost(string, tag="28")]
    pub window_id: ::prost::alloc::string::String,
    #[prost(int64, tag="29")]
    pub event_time_start_ms: i64,
    #[prost(int64, tag="30")]
    pub event_time_end_ms: i64,
    #[prost(string, tag="31")]
    pub value_unit: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="32")]
    pub source_event_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="33")]
    pub missing_fields: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="34")]
    pub missing_reason: ::prost::alloc::string::String,
}
/// FeatureSeq L2 序列特征
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureSeq {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub object_type: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub object_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub window_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub ts_start: i64,
    #[prost(int64, tag="7")]
    pub ts_end: i64,
    #[prost(string, tag="8")]
    pub pktlen_seq_hash: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub iat_seq_hash: ::prost::alloc::string::String,
    #[prost(float, tag="10")]
    pub wavelet_releng_fwd: f32,
    #[prost(float, tag="11")]
    pub wavelet_releng_bwd: f32,
    #[prost(float, tag="12")]
    pub wavelet_entropy_fwd: f32,
    #[prost(float, tag="13")]
    pub wavelet_entropy_bwd: f32,
    #[prost(float, tag="14")]
    pub wavelet_detail_mean_fwd: f32,
    #[prost(float, tag="15")]
    pub wavelet_detail_mean_bwd: f32,
    #[prost(float, tag="16")]
    pub wavelet_detail_std_fwd: f32,
    #[prost(float, tag="17")]
    pub wavelet_detail_std_bwd: f32,
    #[prost(string, tag="18")]
    pub seq_blob_ref: ::prost::alloc::string::String,
    #[prost(enumeration="FeatureCategory", tag="19")]
    pub feature_category: i32,
    #[prost(enumeration="FeatureAvailability", tag="20")]
    pub availability: i32,
    #[prost(string, tag="21")]
    pub schema_version: ::prost::alloc::string::String,
    #[prost(string, tag="22")]
    pub algorithm_version: ::prost::alloc::string::String,
    #[prost(string, tag="23")]
    pub value_unit: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="24")]
    pub source_event_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="25")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="26")]
    pub missing_fields: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="27")]
    pub missing_reason: ::prost::alloc::string::String,
}
/// FeatureFingerprint L3 指纹特征
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureFingerprint {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub session_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub ts: i64,
    /// Boolean semantics expressed as uint32 (0/1) for wire compatibility;
    /// a future breaking-change migration should use `bool`.
    #[prost(uint32, tag="5")]
    pub is_encrypted: u32,
    #[prost(string, tag="6")]
    pub tls_version: ::prost::alloc::string::String,
    #[prost(string, tag="7")]
    pub ja3: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub sni_hash: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub cert_sha256: ::prost::alloc::string::String,
    #[prost(uint32, tag="10")]
    pub cert_is_self_signed: u32,
    #[prost(uint32, tag="11")]
    pub pubkey_len: u32,
    #[prost(float, repeated, tag="12")]
    pub hex_freq: ::prost::alloc::vec::Vec<f32>,
    #[prost(float, repeated, tag="13")]
    pub hex_ratio: ::prost::alloc::vec::Vec<f32>,
    #[prost(float, tag="14")]
    pub entropy_payload: f32,
    #[prost(float, tag="15")]
    pub chi_square_bfd: f32,
    #[prost(enumeration="FeatureCategory", tag="16")]
    pub feature_category: i32,
    #[prost(enumeration="FeatureAvailability", tag="17")]
    pub availability: i32,
    #[prost(string, tag="18")]
    pub schema_version: ::prost::alloc::string::String,
    #[prost(string, tag="19")]
    pub algorithm_version: ::prost::alloc::string::String,
    #[prost(string, tag="20")]
    pub window_id: ::prost::alloc::string::String,
    #[prost(int64, tag="21")]
    pub event_time_start_ms: i64,
    #[prost(int64, tag="22")]
    pub event_time_end_ms: i64,
    #[prost(string, tag="23")]
    pub value_unit: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="24")]
    pub source_event_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="25")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="26")]
    pub missing_fields: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="27")]
    pub missing_reason: ::prost::alloc::string::String,
    #[prost(string, tag="28")]
    pub ja4: ::prost::alloc::string::String,
    #[prost(string, tag="29")]
    pub sni: ::prost::alloc::string::String,
    #[prost(string, tag="30")]
    pub quic_version: ::prost::alloc::string::String,
    #[prost(enumeration="TransportSecurityProtocol", tag="31")]
    pub transport_security: i32,
    #[prost(string, tag="32")]
    pub raw_traffic_ref: ::prost::alloc::string::String,
}
/// FeatureBatch 特征批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureBatch {
    #[prost(message, repeated, tag="1")]
    pub stats: ::prost::alloc::vec::Vec<FeatureStat>,
    #[prost(message, repeated, tag="2")]
    pub sequences: ::prost::alloc::vec::Vec<FeatureSeq>,
    #[prost(message, repeated, tag="3")]
    pub fingerprints: ::prost::alloc::vec::Vec<FeatureFingerprint>,
    #[prost(string, tag="4")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub created_at: i64,
}
/// FeatureCategory is an explicit semantic category, not a detection label.
/// Encrypted traffic and high entropy are never interpreted as malicious here.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum FeatureCategory {
    Unspecified = 0,
    FlowMetadata = 1,
    PlaintextVisible = 2,
    SideChannel = 3,
    RawReference = 4,
    RandomnessStatistics = 5,
}
impl FeatureCategory {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            FeatureCategory::Unspecified => "FEATURE_CATEGORY_UNSPECIFIED",
            FeatureCategory::FlowMetadata => "FEATURE_CATEGORY_FLOW_METADATA",
            FeatureCategory::PlaintextVisible => "FEATURE_CATEGORY_PLAINTEXT_VISIBLE",
            FeatureCategory::SideChannel => "FEATURE_CATEGORY_SIDE_CHANNEL",
            FeatureCategory::RawReference => "FEATURE_CATEGORY_RAW_REFERENCE",
            FeatureCategory::RandomnessStatistics => "FEATURE_CATEGORY_RANDOMNESS_STATISTICS",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "FEATURE_CATEGORY_UNSPECIFIED" => Some(Self::Unspecified),
            "FEATURE_CATEGORY_FLOW_METADATA" => Some(Self::FlowMetadata),
            "FEATURE_CATEGORY_PLAINTEXT_VISIBLE" => Some(Self::PlaintextVisible),
            "FEATURE_CATEGORY_SIDE_CHANNEL" => Some(Self::SideChannel),
            "FEATURE_CATEGORY_RAW_REFERENCE" => Some(Self::RawReference),
            "FEATURE_CATEGORY_RANDOMNESS_STATISTICS" => Some(Self::RandomnessStatistics),
            _ => None,
        }
    }
}
/// FeatureAvailability separates absent, unsupported and invalid calculations;
/// numeric proto defaults must not be interpreted as successfully measured.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum FeatureAvailability {
    Unspecified = 0,
    Available = 1,
    MissingInput = 2,
    NotApplicable = 3,
    Unsupported = 4,
    Invalid = 5,
    PartiallyAvailable = 6,
}
impl FeatureAvailability {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            FeatureAvailability::Unspecified => "FEATURE_AVAILABILITY_UNSPECIFIED",
            FeatureAvailability::Available => "FEATURE_AVAILABILITY_AVAILABLE",
            FeatureAvailability::MissingInput => "FEATURE_AVAILABILITY_MISSING_INPUT",
            FeatureAvailability::NotApplicable => "FEATURE_AVAILABILITY_NOT_APPLICABLE",
            FeatureAvailability::Unsupported => "FEATURE_AVAILABILITY_UNSUPPORTED",
            FeatureAvailability::Invalid => "FEATURE_AVAILABILITY_INVALID",
            FeatureAvailability::PartiallyAvailable => "FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "FEATURE_AVAILABILITY_UNSPECIFIED" => Some(Self::Unspecified),
            "FEATURE_AVAILABILITY_AVAILABLE" => Some(Self::Available),
            "FEATURE_AVAILABILITY_MISSING_INPUT" => Some(Self::MissingInput),
            "FEATURE_AVAILABILITY_NOT_APPLICABLE" => Some(Self::NotApplicable),
            "FEATURE_AVAILABILITY_UNSUPPORTED" => Some(Self::Unsupported),
            "FEATURE_AVAILABILITY_INVALID" => Some(Self::Invalid),
            "FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE" => Some(Self::PartiallyAvailable),
            _ => None,
        }
    }
}
/// FlowEvent 流事件（对应 ClickHouse flows_raw 表）
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FlowEvent {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub flow_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(message, optional, tag="4")]
    pub tuple: ::core::option::Option<FiveTuple>,
    #[prost(string, tag="5")]
    pub direction: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub ts_start: i64,
    #[prost(int64, tag="7")]
    pub ts_end: i64,
    #[prost(uint32, tag="8")]
    pub duration_ms: u32,
    #[prost(uint32, tag="9")]
    pub packets_fwd: u32,
    #[prost(uint32, tag="10")]
    pub packets_bwd: u32,
    #[prost(uint64, tag="11")]
    pub bytes_fwd: u64,
    #[prost(uint64, tag="12")]
    pub bytes_bwd: u64,
    #[prost(float, tag="13")]
    pub pps: f32,
    #[prost(float, tag="14")]
    pub bps: f32,
    #[prost(message, optional, tag="15")]
    pub pktlen_stats: ::core::option::Option<PacketLengthStats>,
    #[prost(message, optional, tag="16")]
    pub iat_stats: ::core::option::Option<InterArrivalStats>,
    #[prost(uint32, tag="17")]
    pub tcp_flags_fwd: u32,
    #[prost(uint32, tag="18")]
    pub tcp_flags_bwd: u32,
    #[prost(uint32, tag="19")]
    pub tos: u32,
    #[prost(message, optional, tag="20")]
    pub active_stats: ::core::option::Option<ActiveIdleStats>,
    #[prost(message, optional, tag="21")]
    pub idle_stats: ::core::option::Option<ActiveIdleStats>,
    #[prost(uint32, tag="22")]
    pub subflow_count: u32,
    /// Additive identity algorithm revision. Zero means the legacy algorithm;
    /// revision 2 is the canonical capture-event-time identity contract.
    #[prost(uint32, tag="23")]
    pub identity_revision: u32,
    #[prost(message, optional, tag="24")]
    pub feature_observation: ::core::option::Option<TrafficFeatureObservation>,
}
/// FlowBatch 流事件批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FlowBatch {
    #[prost(message, repeated, tag="1")]
    pub flows: ::prost::alloc::vec::Vec<FlowEvent>,
    #[prost(message, optional, tag="2")]
    pub metadata: ::core::option::Option<BatchMetadata>,
}
/// BatchMetadata 批次元数据
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct BatchMetadata {
    #[prost(string, tag="1")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(uint32, tag="5")]
    pub batch_size: u32,
    #[prost(string, tag="6")]
    pub compression: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub created_at: i64,
}
/// GraphQueryLog 图查询日志
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphQueryLog {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub query_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub user_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub query_type: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub center_ip: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="6")]
    pub center_ips: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(uint32, tag="7")]
    pub depth: u32,
    #[prost(string, tag="8")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(int64, tag="9")]
    pub query_start_time: i64,
    #[prost(int64, tag="10")]
    pub query_end_time: i64,
    #[prost(uint32, tag="11")]
    pub node_count: u32,
    #[prost(uint32, tag="12")]
    pub edge_count: u32,
    #[prost(uint32, tag="13")]
    pub path_count: u32,
    #[prost(uint64, tag="14")]
    pub result_size_bytes: u64,
    #[prost(uint32, tag="15")]
    pub duration_ms: u32,
    /// Boolean semantics expressed as uint32 (0/1) for wire compatibility;
    /// a future breaking-change migration should use `bool`.
    #[prost(uint32, tag="16")]
    pub cache_hit: u32,
    #[prost(uint32, tag="17")]
    pub ch_query_count: u32,
    #[prost(uint32, tag="18")]
    pub ch_total_duration_ms: u32,
    #[prost(uint64, tag="19")]
    pub ch_rows_read: u64,
    #[prost(uint64, tag="20")]
    pub ch_bytes_read: u64,
    #[prost(string, tag="21")]
    pub status: ::prost::alloc::string::String,
    #[prost(string, tag="22")]
    pub error_code: ::prost::alloc::string::String,
    #[prost(string, tag="23")]
    pub error_message: ::prost::alloc::string::String,
    #[prost(string, tag="24")]
    pub trace_id: ::prost::alloc::string::String,
    #[prost(string, tag="25")]
    pub client_ip: ::prost::alloc::string::String,
    #[prost(string, tag="26")]
    pub user_agent: ::prost::alloc::string::String,
    #[prost(int64, tag="27")]
    pub created_at: i64,
}
/// GraphCacheStats 缓存命中率统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphCacheStats {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="2")]
    pub hour: i64,
    #[prost(string, tag="3")]
    pub query_type: ::prost::alloc::string::String,
    #[prost(uint64, tag="4")]
    pub total_queries: u64,
    #[prost(uint64, tag="5")]
    pub cache_hits: u64,
    #[prost(uint64, tag="6")]
    pub cache_misses: u64,
    #[prost(float, tag="7")]
    pub avg_duration_ms: f32,
    #[prost(float, tag="8")]
    pub p95_duration_ms: f32,
    #[prost(float, tag="9")]
    pub p99_duration_ms: f32,
    #[prost(uint64, tag="10")]
    pub total_nodes: u64,
    #[prost(uint64, tag="11")]
    pub total_edges: u64,
    #[prost(uint64, tag="12")]
    pub error_count: u64,
    #[prost(uint64, tag="13")]
    pub timeout_count: u64,
}
/// GraphHotIP 热点 IP 统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphHotIp {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="2")]
    pub date: i64,
    #[prost(string, tag="3")]
    pub ip: ::prost::alloc::string::String,
    #[prost(uint64, tag="4")]
    pub query_count: u64,
    #[prost(uint64, tag="5")]
    pub total_neighbors: u64,
    #[prost(float, tag="6")]
    pub avg_session_count: f32,
    #[prost(int64, tag="7")]
    pub last_query_time: i64,
}
/// GraphSlowQuery 慢查询
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphSlowQuery {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub query_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub query_type: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub center_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="5")]
    pub depth: u32,
    #[prost(string, tag="6")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(uint32, tag="7")]
    pub duration_ms: u32,
    #[prost(uint32, tag="8")]
    pub node_count: u32,
    #[prost(uint32, tag="9")]
    pub edge_count: u32,
    #[prost(uint64, tag="10")]
    pub ch_rows_read: u64,
    #[prost(uint64, tag="11")]
    pub ch_bytes_read: u64,
    #[prost(string, tag="12")]
    pub error_message: ::prost::alloc::string::String,
    #[prost(int64, tag="13")]
    pub created_at: i64,
}
/// GraphIPAffinity IP 关系强度
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphIpAffinity {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="2")]
    pub date: i64,
    #[prost(string, tag="3")]
    pub ip_a: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub ip_b: ::prost::alloc::string::String,
    #[prost(uint32, tag="5")]
    pub session_count: u32,
    #[prost(uint64, tag="6")]
    pub total_bytes: u64,
    #[prost(float, tag="7")]
    pub avg_duration_ms: f32,
    #[prost(uint32, tag="8")]
    pub a_to_b_count: u32,
    #[prost(uint32, tag="9")]
    pub b_to_a_count: u32,
    #[prost(int64, tag="10")]
    pub first_seen: i64,
    #[prost(int64, tag="11")]
    pub last_seen: i64,
}
/// GraphQueryLogBatch 批量上报图查询日志
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphQueryLogBatch {
    #[prost(message, repeated, tag="1")]
    pub logs: ::prost::alloc::vec::Vec<GraphQueryLog>,
    #[prost(string, tag="2")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub created_at: i64,
}
/// GraphStatsBatch 批量上报图统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphStatsBatch {
    #[prost(message, repeated, tag="1")]
    pub cache_stats: ::prost::alloc::vec::Vec<GraphCacheStats>,
    #[prost(message, repeated, tag="2")]
    pub hot_ips: ::prost::alloc::vec::Vec<GraphHotIp>,
    #[prost(message, repeated, tag="3")]
    pub slow_queries: ::prost::alloc::vec::Vec<GraphSlowQuery>,
    #[prost(message, repeated, tag="4")]
    pub ip_affinities: ::prost::alloc::vec::Vec<GraphIpAffinity>,
    #[prost(string, tag="5")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub created_at: i64,
}
/// GraphEntityIdentity is the tenant-scoped canonical identity used to derive a
/// stable NebulaGraph VID. vertex_id is the lowercase SHA-256 prefix defined by
/// the M07 graph projection contract; producers may not choose an arbitrary VID.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphEntityIdentity {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub entity_type: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub canonical_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub vertex_id: ::prost::alloc::string::String,
}
/// GraphEvidenceAnchor binds a relation to immutable evidence. At least one
/// anchor is required for every projected relation and attack-chain edge.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphEvidenceAnchor {
    #[prost(string, tag="1")]
    pub evidence_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub evidence_kind: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub immutable_uri: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub sha256: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub source_event_id: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub occurred_at: i64,
}
/// GraphProjectionSource carries the source ordering and integrity identity.
/// aggregate_version is compared before a NebulaGraph mutation is accepted.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphProjectionSource {
    #[prost(string, tag="1")]
    pub source_system: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub source_event_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub aggregate_type: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub aggregate_id: ::prost::alloc::string::String,
    #[prost(uint64, tag="5")]
    pub aggregate_version: u64,
    #[prost(string, tag="6")]
    pub source_sha256: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub occurred_at: i64,
}
/// GraphProjectedEntity is an idempotent entity upsert/revocation payload.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphProjectedEntity {
    #[prost(message, optional, tag="1")]
    pub identity: ::core::option::Option<GraphEntityIdentity>,
    #[prost(map="string, string", tag="2")]
    pub attributes: ::std::collections::HashMap<::prost::alloc::string::String, ::prost::alloc::string::String>,
    #[prost(int64, tag="3")]
    pub valid_from: i64,
    #[prost(int64, tag="4")]
    pub valid_to: i64,
    #[prost(message, optional, tag="5")]
    pub source: ::core::option::Option<GraphProjectionSource>,
    #[prost(string, tag="6")]
    pub projection_sha256: ::prost::alloc::string::String,
    #[prost(bool, tag="7")]
    pub revoked: bool,
}
/// GraphProjectedRelation is an idempotent relationship upsert/revocation
/// payload. edge_id is derived from tenant, relation type and endpoint VIDs.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphProjectedRelation {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub edge_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub relation_type: ::prost::alloc::string::String,
    #[prost(message, optional, tag="4")]
    pub source_identity: ::core::option::Option<GraphEntityIdentity>,
    #[prost(message, optional, tag="5")]
    pub target_identity: ::core::option::Option<GraphEntityIdentity>,
    #[prost(enumeration="GraphProvenanceKind", tag="6")]
    pub provenance_kind: i32,
    #[prost(double, tag="7")]
    pub confidence: f64,
    #[prost(string, tag="8")]
    pub uncertainty: ::prost::alloc::string::String,
    #[prost(message, repeated, tag="9")]
    pub evidence: ::prost::alloc::vec::Vec<GraphEvidenceAnchor>,
    #[prost(int64, tag="10")]
    pub valid_from: i64,
    #[prost(int64, tag="11")]
    pub valid_to: i64,
    #[prost(message, optional, tag="12")]
    pub source: ::core::option::Option<GraphProjectionSource>,
    #[prost(string, tag="13")]
    pub projection_sha256: ::prost::alloc::string::String,
    #[prost(bool, tag="14")]
    pub revoked: bool,
}
/// GraphProjectionEvent is the graph projector's canonical Kafka payload.
/// partition_key must equal tenant_id plus the projected entity or relation
/// identity, so all versions for one aggregate are observed in order.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphProjectionEvent {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub partition_key: ::prost::alloc::string::String,
    #[prost(oneof="graph_projection_event::Projection", tags="3, 4")]
    pub projection: ::core::option::Option<graph_projection_event::Projection>,
}
/// Nested message and enum types in `GraphProjectionEvent`.
pub mod graph_projection_event {
    #[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Oneof)]
    pub enum Projection {
        #[prost(message, tag="3")]
        Entity(super::GraphProjectedEntity),
        #[prost(message, tag="4")]
        Relation(super::GraphProjectedRelation),
    }
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct GraphProjectionEventBatch {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<GraphProjectionEvent>,
    #[prost(string, tag="2")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub created_at: i64,
}
/// GraphProvenanceKind prevents an inferred or analyst-authored relationship
/// from being serialized as a directly observed fact.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum GraphProvenanceKind {
    Unspecified = 0,
    Observed = 1,
    Derived = 2,
    Analyst = 3,
}
impl GraphProvenanceKind {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            GraphProvenanceKind::Unspecified => "GRAPH_PROVENANCE_KIND_UNSPECIFIED",
            GraphProvenanceKind::Observed => "GRAPH_PROVENANCE_KIND_OBSERVED",
            GraphProvenanceKind::Derived => "GRAPH_PROVENANCE_KIND_DERIVED",
            GraphProvenanceKind::Analyst => "GRAPH_PROVENANCE_KIND_ANALYST",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "GRAPH_PROVENANCE_KIND_UNSPECIFIED" => Some(Self::Unspecified),
            "GRAPH_PROVENANCE_KIND_OBSERVED" => Some(Self::Observed),
            "GRAPH_PROVENANCE_KIND_DERIVED" => Some(Self::Derived),
            "GRAPH_PROVENANCE_KIND_ANALYST" => Some(Self::Analyst),
            _ => None,
        }
    }
}
/// SessionEvent 双向会话事件
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct SessionEvent {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub session_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(message, optional, tag="4")]
    pub tuple: ::core::option::Option<FiveTuple>,
    #[prost(int64, tag="5")]
    pub ts_start: i64,
    #[prost(int64, tag="6")]
    pub ts_end: i64,
    #[prost(uint32, tag="7")]
    pub duration_ms: u32,
    #[prost(uint32, tag="8")]
    pub protocol: u32,
    #[prost(string, tag="9")]
    pub client_ip: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub server_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="11")]
    pub client_port: u32,
    #[prost(uint32, tag="12")]
    pub server_port: u32,
    #[prost(uint64, tag="13")]
    pub packets_total: u64,
    #[prost(uint64, tag="14")]
    pub bytes_total: u64,
    #[prost(uint64, tag="15")]
    pub bytes_fwd: u64,
    #[prost(uint64, tag="16")]
    pub bytes_bwd: u64,
    #[prost(float, tag="17")]
    pub up_down_ratio: f32,
    #[prost(uint32, tag="18")]
    pub num_pkts: u32,
    #[prost(float, tag="19")]
    pub avg_payload: f32,
    #[prost(uint32, tag="20")]
    pub min_payload: u32,
    #[prost(uint32, tag="21")]
    pub max_payload: u32,
    #[prost(float, tag="22")]
    pub std_payload: f32,
    #[prost(float, tag="23")]
    pub mean_iat_ms: f32,
    #[prost(float, tag="24")]
    pub min_iat_ms: f32,
    #[prost(float, tag="25")]
    pub max_iat_ms: f32,
    #[prost(float, tag="26")]
    pub std_iat_ms: f32,
    #[prost(uint32, tag="27")]
    pub flags_syn: u32,
    #[prost(uint32, tag="28")]
    pub flags_ack: u32,
    #[prost(uint32, tag="29")]
    pub flags_fin: u32,
    #[prost(uint32, tag="30")]
    pub flags_psh: u32,
    #[prost(uint32, tag="31")]
    pub flags_rst: u32,
    #[prost(uint32, tag="32")]
    pub dns_pkt_cnt: u32,
    #[prost(uint32, tag="33")]
    pub tcp_pkt_cnt: u32,
    #[prost(uint32, tag="34")]
    pub udp_pkt_cnt: u32,
    #[prost(uint32, tag="35")]
    pub icmp_pkt_cnt: u32,
    #[prost(bool, tag="36")]
    pub has_syn: bool,
    #[prost(bool, tag="37")]
    pub has_fin: bool,
    #[prost(bool, tag="38")]
    pub has_rst: bool,
    #[prost(bool, tag="39")]
    pub is_established: bool,
    #[prost(uint32, tag="40")]
    pub evidence_count: u32,
    #[prost(string, repeated, tag="41")]
    pub flow_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="42")]
    pub end_reason: ::prost::alloc::string::String,
    /// Additive M03 identity and event-time contract. All timestamps are Unix
    /// epoch milliseconds; identity_version names the deterministic ID recipe.
    #[prost(string, tag="43")]
    pub identity_version: ::prost::alloc::string::String,
    #[prost(uint64, tag="44")]
    pub session_version: u64,
    #[prost(int64, tag="45")]
    pub event_time_start_ms: i64,
    #[prost(int64, tag="46")]
    pub event_time_end_ms: i64,
    #[prost(string, repeated, tag="47")]
    pub source_event_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="48")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(enumeration="SessionCompleteness", tag="49")]
    pub completeness: i32,
    #[prost(string, repeated, tag="50")]
    pub missing_fields: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(message, optional, tag="51")]
    pub feature_observation: ::core::option::Option<TrafficFeatureObservation>,
    /// Directional packet counts: forward = packets emitted in the initiator
    /// (client) direction, backward = responder (server) direction. Zero when the
    /// producer cannot attribute direction; both fields are additive (legacy
    /// producers stay wire-compatible with proto3 zero defaults).
    #[prost(uint64, tag="52")]
    pub packets_fwd: u64,
    #[prost(uint64, tag="53")]
    pub packets_bwd: u64,
}
/// SessionBatch 会话事件批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct SessionBatch {
    #[prost(message, repeated, tag="1")]
    pub sessions: ::prost::alloc::vec::Vec<SessionEvent>,
    #[prost(string, tag="2")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub run_id: ::prost::alloc::string::String,
    #[prost(uint32, tag="6")]
    pub batch_size: u32,
    #[prost(string, tag="7")]
    pub compression: ::prost::alloc::string::String,
    #[prost(int64, tag="8")]
    pub created_at: i64,
}
/// SessionCompleteness distinguishes a usable complete session from an
/// explicitly partial or invalid observation. Zero remains the legacy/unknown
/// value so existing producers stay wire-compatible without claiming success.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum SessionCompleteness {
    Unspecified = 0,
    Complete = 1,
    Partial = 2,
    Truncated = 3,
    Invalid = 4,
}
impl SessionCompleteness {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            SessionCompleteness::Unspecified => "SESSION_COMPLETENESS_UNSPECIFIED",
            SessionCompleteness::Complete => "SESSION_COMPLETENESS_COMPLETE",
            SessionCompleteness::Partial => "SESSION_COMPLETENESS_PARTIAL",
            SessionCompleteness::Truncated => "SESSION_COMPLETENESS_TRUNCATED",
            SessionCompleteness::Invalid => "SESSION_COMPLETENESS_INVALID",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "SESSION_COMPLETENESS_UNSPECIFIED" => Some(Self::Unspecified),
            "SESSION_COMPLETENESS_COMPLETE" => Some(Self::Complete),
            "SESSION_COMPLETENESS_PARTIAL" => Some(Self::Partial),
            "SESSION_COMPLETENESS_TRUNCATED" => Some(Self::Truncated),
            "SESSION_COMPLETENESS_INVALID" => Some(Self::Invalid),
            _ => None,
        }
    }
}
/// PcapIndexMeta PCAP 索引元数据
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapIndexMeta {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub file_key: ::prost::alloc::string::String,
    #[prost(int64, tag="4")]
    pub ts_start: i64,
    #[prost(int64, tag="5")]
    pub ts_end: i64,
    #[prost(uint64, tag="6")]
    pub byte_size: u64,
    #[prost(uint32, tag="7")]
    pub zstd_level: u32,
    #[prost(string, tag="8")]
    pub sha256: ::prost::alloc::string::String,
    #[prost(string, tag="9")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub flow_id: ::prost::alloc::string::String,
    #[prost(uint64, tag="11")]
    pub offset_start: u64,
    #[prost(uint64, tag="12")]
    pub offset_end: u64,
    #[prost(string, tag="13")]
    pub bloom_filter_b64: ::prost::alloc::string::String,
    #[prost(string, repeated, tag="14")]
    pub community_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(int64, tag="15")]
    pub created_ts: i64,
    /// Object-store manifest fields are additive. Kafka source coordinates remain
    /// transport authority and MUST NOT be copied into this wire message.
    #[prost(string, tag="16")]
    pub bucket: ::prost::alloc::string::String,
    #[prost(string, tag="17")]
    pub object_version: ::prost::alloc::string::String,
    #[prost(string, tag="18")]
    pub etag: ::prost::alloc::string::String,
    #[prost(uint64, tag="19")]
    pub original_size: u64,
    #[prost(uint64, tag="20")]
    pub stored_size: u64,
    #[prost(string, tag="21")]
    pub compression: ::prost::alloc::string::String,
    #[prost(uint32, tag="22")]
    pub manifest_version: u32,
    /// Additive: total packet count observed in the archived capture window.
    /// Zero means the producer did not record a count (legacy wire-compatible).
    #[prost(uint64, tag="23")]
    pub packet_count: u64,
}
/// PcapIndexBatch PCAP 索引批次
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapIndexBatch {
    #[prost(message, repeated, tag="1")]
    pub indexes: ::prost::alloc::vec::Vec<PcapIndexMeta>,
    #[prost(string, tag="2")]
    pub batch_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(int64, tag="5")]
    pub created_at: i64,
}
/// PcapCutRequest PCAP 裁剪请求
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapCutRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub src_ip: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub dst_ip: ::prost::alloc::string::String,
    #[prost(uint32, tag="4")]
    pub src_port: u32,
    #[prost(uint32, tag="5")]
    pub dst_port: u32,
    #[prost(uint32, tag="6")]
    pub protocol: u32,
    #[prost(int64, tag="7")]
    pub start_time: i64,
    #[prost(int64, tag="8")]
    pub end_time: i64,
    #[prost(string, tag="9")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="10")]
    pub flow_id: ::prost::alloc::string::String,
    #[prost(uint32, tag="11")]
    pub max_packets: u32,
    #[prost(uint64, tag="12")]
    pub max_bytes: u64,
    #[prost(string, tag="13")]
    pub output_format: ::prost::alloc::string::String,
    #[prost(bool, tag="14")]
    pub compress: bool,
}
/// PcapCutResponse PCAP 裁剪响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapCutResponse {
    #[prost(string, tag="1")]
    pub job_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub status: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub download_url: ::prost::alloc::string::String,
    #[prost(int32, tag="4")]
    pub progress_percent: i32,
    #[prost(string, tag="5")]
    pub error_message: ::prost::alloc::string::String,
    #[prost(uint64, tag="6")]
    pub total_packets: u64,
    #[prost(uint64, tag="7")]
    pub total_bytes: u64,
    #[prost(int32, tag="8")]
    pub files_scanned: i32,
    #[prost(int32, tag="9")]
    pub files_matched: i32,
    #[prost(int64, tag="10")]
    pub created_at: i64,
    #[prost(int64, tag="11")]
    pub started_at: i64,
    #[prost(int64, tag="12")]
    pub completed_at: i64,
    #[prost(int64, tag="13")]
    pub expires_at: i64,
}
/// PcapCutJobStatus 裁剪任务状态查询
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapCutJobStatus {
    #[prost(string, tag="1")]
    pub job_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
}
// ==================== Flow 上报 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadFlowsRequest {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<FlowEvent>,
    #[prost(string, tag="2")]
    pub compression: ::prost::alloc::string::String,
    /// Highest exact-set response revision the client can durably apply.
    #[prost(uint32, tag="3")]
    pub accepted_response_revision: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadFlowsResponse {
    #[prost(int32, tag="1")]
    pub accepted: i32,
    #[prost(int32, tag="2")]
    pub rejected: i32,
    #[prost(string, repeated, tag="3")]
    pub rejected_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="4")]
    pub message: ::prost::alloc::string::String,
    /// Additive exact-set acknowledgement. When present, there is exactly one
    /// result for every request input_index; old aggregate fields remain for
    /// wire compatibility but cannot override these dispositions.
    #[prost(message, repeated, tag="5")]
    pub item_results: ::prost::alloc::vec::Vec<FlowItemResult>,
    #[prost(uint32, tag="6")]
    pub response_revision: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FlowItemResult {
    #[prost(uint32, tag="1")]
    pub input_index: u32,
    #[prost(string, tag="2")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(enumeration="FlowItemDisposition", tag="3")]
    pub disposition: i32,
    #[prost(string, tag="4")]
    pub reason_code: ::prost::alloc::string::String,
    /// The only currently approved durable acceptance barrier for FlowEvent.
    #[prost(string, tag="5")]
    pub ack_scope: ::prost::alloc::string::String,
}
// ==================== Flow 流式上报 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StreamFlowsRequest {
    #[prost(message, optional, tag="1")]
    pub event: ::core::option::Option<FlowEvent>,
    #[prost(uint32, tag="2")]
    pub accepted_response_revision: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StreamFlowsResponse {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(bool, tag="2")]
    pub accepted: bool,
    #[prost(string, tag="3")]
    pub error: ::prost::alloc::string::String,
    #[prost(enumeration="FlowItemDisposition", tag="4")]
    pub disposition: i32,
    #[prost(string, tag="5")]
    pub reason_code: ::prost::alloc::string::String,
    #[prost(string, tag="6")]
    pub ack_scope: ::prost::alloc::string::String,
    #[prost(uint32, tag="7")]
    pub response_revision: u32,
}
// ==================== Session 上报 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadSessionsRequest {
    #[prost(message, repeated, tag="1")]
    pub sessions: ::prost::alloc::vec::Vec<SessionEvent>,
    #[prost(string, tag="2")]
    pub compression: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadSessionsResponse {
    #[prost(int32, tag="1")]
    pub accepted: i32,
    #[prost(int32, tag="2")]
    pub rejected: i32,
    #[prost(string, repeated, tag="3")]
    pub rejected_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, tag="4")]
    pub message: ::prost::alloc::string::String,
}
// ==================== PCAP Index 上报 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadPcapIndexRequest {
    #[prost(message, optional, tag="1")]
    pub index: ::core::option::Option<PcapIndexMeta>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadPcapIndexResponse {
    #[prost(bool, tag="1")]
    pub success: bool,
    #[prost(string, tag="2")]
    pub message: ::prost::alloc::string::String,
}
// ==================== Passive asset binding upload ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadAssetBindingsRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(message, repeated, tag="3")]
    pub bindings: ::prost::alloc::vec::Vec<MacIpBinding>,
    /// Highest exact-set response revision the probe can durably apply.
    #[prost(uint32, tag="4")]
    pub accepted_response_revision: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AssetBindingItemResult {
    #[prost(uint32, tag="1")]
    pub input_index: u32,
    #[prost(string, tag="2")]
    pub observation_id: ::prost::alloc::string::String,
    #[prost(enumeration="AssetBindingItemDisposition", tag="3")]
    pub disposition: i32,
    #[prost(string, tag="4")]
    pub reason_code: ::prost::alloc::string::String,
    /// KAFKA_RECORD only after required-acks=all succeeds.
    #[prost(string, tag="5")]
    pub ack_scope: ::prost::alloc::string::String,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct UploadAssetBindingsResponse {
    #[prost(int32, tag="1")]
    pub accepted: i32,
    #[prost(int32, tag="2")]
    pub rejected: i32,
    #[prost(message, repeated, tag="3")]
    pub item_results: ::prost::alloc::vec::Vec<AssetBindingItemResult>,
    #[prost(uint32, tag="4")]
    pub response_revision: u32,
    #[prost(string, tag="5")]
    pub message: ::prost::alloc::string::String,
}
// ==================== 心跳 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HeartbeatRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(message, optional, tag="3")]
    pub status: ::core::option::Option<ProbeStatus>,
    /// Final receipts are retried by the Agent until the authenticated Gateway
    /// durably accepts them. The Gateway must reject receipts whose tenant/probe
    /// identity differs from the authenticated context.
    #[prost(message, repeated, tag="4")]
    pub operation_acks: ::prost::alloc::vec::Vec<ProbeOperationAck>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HeartbeatResponse {
    #[prost(bool, tag="1")]
    pub ok: bool,
    #[prost(message, optional, tag="2")]
    pub config: ::core::option::Option<ProbeConfig>,
    /// Commands are selected by the Gateway after authenticating tenant_id and
    /// probe_id. The Agent still validates both identities, expiry, revision and
    /// the deterministic command hash before execution.
    #[prost(message, repeated, tag="3")]
    pub operation_commands: ::prost::alloc::vec::Vec<ProbeOperationCommand>,
    /// The Agent removes a locally persisted ACK only when its operation_id is
    /// returned here. An empty list is not an acknowledgement.
    #[prost(string, repeated, tag="4")]
    pub accepted_ack_operation_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// ProbeOperationCommand is the edge-delivery representation of an accepted
/// traffic.probe.v2.OperationRequested event. Kafka remains the durable command
/// source; this message prevents exposing shared broker credentials to probes.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeOperationCommand {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub operation_id: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub operation_type: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub command_revision: i64,
    #[prost(string, tag="7")]
    pub desired_version: ::prost::alloc::string::String,
    #[prost(string, tag="8")]
    pub command_hash: ::prost::alloc::string::String,
    #[prost(int64, tag="9")]
    pub expires_at_ms: i64,
    #[prost(string, tag="10")]
    pub trace_id: ::prost::alloc::string::String,
    #[prost(bytes="vec", tag="11")]
    pub command_json: ::prost::alloc::vec::Vec<u8>,
}
/// ProbeOperationAck is produced only after the Agent has persisted the
/// operation result locally. applied=false is a real terminal failure, never a
/// transport acknowledgement or optimistic success.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeOperationAck {
    #[prost(string, tag="1")]
    pub operation_id: ::prost::alloc::string::String,
    #[prost(int64, tag="2")]
    pub command_revision: i64,
    #[prost(string, tag="3")]
    pub reported_version: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub reported_hash: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub agent_version: ::prost::alloc::string::String,
    #[prost(bool, tag="6")]
    pub applied: bool,
    #[prost(string, tag="7")]
    pub error: ::prost::alloc::string::String,
    #[prost(int64, tag="8")]
    pub acknowledged_at_ms: i64,
    #[prost(bytes="vec", tag="9")]
    pub detail_json: ::prost::alloc::vec::Vec<u8>,
}
/// ProbeGroupReadinessReceiptV1 is a durable, cross-process statement about
/// one broker-owned consumer-group generation. It is not a process health
/// signal: receivers must validate the Kafka key and headers, the bounded
/// lease, and the monotonic owner epoch before it can authorize work.
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeGroupReadinessReceiptV1 {
    #[prost(string, tag="1")]
    pub receipt_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub consumer_group: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub observed_topic: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub member_id: ::prost::alloc::string::String,
    #[prost(int32, tag="5")]
    pub generation_id: i32,
    #[prost(int64, tag="6")]
    pub owner_epoch: i64,
    #[prost(enumeration="ProbeGroupReadinessStateV1", tag="7")]
    pub state: i32,
    #[prost(int64, tag="8")]
    pub observed_at_ms: i64,
    #[prost(int64, tag="9")]
    pub expires_at_ms: i64,
    #[prost(string, tag="10")]
    pub publisher_instance_id: ::prost::alloc::string::String,
}
// ==================== 探针注册 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RegisterProbeRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(message, optional, tag="3")]
    pub hardware: ::core::option::Option<HardwareInfo>,
    #[prost(string, tag="4")]
    pub software_version: ::prost::alloc::string::String,
    #[prost(string, tag="5")]
    pub build_commit: ::prost::alloc::string::String,
    #[prost(int64, tag="6")]
    pub build_timestamp: i64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct RegisterProbeResponse {
    #[prost(bool, tag="1")]
    pub success: bool,
    #[prost(string, tag="2")]
    pub message: ::prost::alloc::string::String,
    #[prost(message, optional, tag="3")]
    pub initial_config: ::core::option::Option<ProbeConfig>,
}
// ==================== 探针状态与配置 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeStatus {
    #[prost(float, tag="1")]
    pub cpu_usage: f32,
    #[prost(float, tag="2")]
    pub memory_usage: f32,
    #[prost(uint64, tag="3")]
    pub capture_pps: u64,
    #[prost(uint64, tag="4")]
    pub upload_bps: u64,
    #[prost(uint64, tag="5")]
    pub packets_captured: u64,
    #[prost(uint64, tag="6")]
    pub packets_dropped: u64,
    #[prost(int64, tag="7")]
    pub uptime_seconds: i64,
    #[prost(message, repeated, tag="10")]
    pub interfaces: ::prost::alloc::vec::Vec<InterfaceStatus>,
    /// Additive authority breakdown. The legacy packets_dropped field equals
    /// capture_allocation_drops + capture_kernel_drops and excludes parser,
    /// downstream queue, and sender failures.
    #[prost(uint64, tag="11")]
    pub capture_allocation_drops: u64,
    #[prost(uint64, tag="12")]
    pub capture_kernel_drops: u64,
    #[prost(uint64, tag="13")]
    pub capture_errors: u64,
    #[prost(uint64, tag="14")]
    pub capture_bytes: u64,
    #[prost(uint64, tag="15")]
    pub capture_counter_revision: u64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct InterfaceStatus {
    #[prost(string, tag="1")]
    pub name: ::prost::alloc::string::String,
    #[prost(bool, tag="2")]
    pub link_up: bool,
    #[prost(uint64, tag="3")]
    pub speed_mbps: u64,
    #[prost(uint64, tag="4")]
    pub rx_packets: u64,
    #[prost(uint64, tag="5")]
    pub tx_packets: u64,
    #[prost(uint64, tag="6")]
    pub rx_bytes: u64,
    #[prost(uint64, tag="7")]
    pub tx_bytes: u64,
    #[prost(uint64, tag="8")]
    pub rx_errors: u64,
    #[prost(uint64, tag="9")]
    pub tx_errors: u64,
    #[prost(uint64, tag="10")]
    pub rx_crc_errors: u64,
    #[prost(uint64, tag="11")]
    pub rx_dropped: u64,
    #[prost(uint64, tag="12")]
    pub collisions: u64,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeConfig {
    #[prost(string, tag="1")]
    pub config_version: ::prost::alloc::string::String,
    #[prost(float, tag="2")]
    pub sample_rate: f32,
    #[prost(string, tag="3")]
    pub bpf_filter: ::prost::alloc::string::String,
    #[prost(uint32, tag="4")]
    pub idle_timeout_sec: u32,
    #[prost(uint32, tag="5")]
    pub active_timeout_sec: u32,
    #[prost(uint32, tag="6")]
    pub batch_size: u32,
    #[prost(string, tag="7")]
    pub feature_set_version: ::prost::alloc::string::String,
    #[prost(message, optional, tag="10")]
    pub nic_config: ::core::option::Option<NetworkInterfaceConfig>,
    #[prost(uint32, tag="11")]
    pub ring_buffer_size: u32,
    #[prost(uint32, tag="12")]
    pub batch_drain_timeout_ms: u32,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct NetworkInterfaceConfig {
    #[prost(string, tag="1")]
    pub interface_name: ::prost::alloc::string::String,
    #[prost(bool, tag="2")]
    pub promiscuous_mode: bool,
    #[prost(string, repeated, tag="3")]
    pub bpf_filters: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(uint32, tag="4")]
    pub ring_buffer_size_mb: u32,
    #[prost(string, tag="5")]
    pub driver_mode: ::prost::alloc::string::String,
    #[prost(message, optional, tag="6")]
    pub cpu_affinity: ::core::option::Option<CpuAffinityConfig>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct CpuAffinityConfig {
    #[prost(uint32, repeated, tag="1")]
    pub cpu_cores: ::prost::alloc::vec::Vec<u32>,
    #[prost(bool, tag="2")]
    pub numa_aware: bool,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HardwareInfo {
    #[prost(string, tag="1")]
    pub cpu_model: ::prost::alloc::string::String,
    #[prost(uint32, tag="2")]
    pub cpu_cores: u32,
    #[prost(uint64, tag="3")]
    pub memory_mb: u64,
    #[prost(string, tag="4")]
    pub os_version: ::prost::alloc::string::String,
    #[prost(message, repeated, tag="5")]
    pub nics: ::prost::alloc::vec::Vec<Nic>,
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Nic {
    #[prost(string, tag="1")]
    pub name: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub mac_address: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub pci_address: ::prost::alloc::string::String,
    #[prost(string, tag="4")]
    pub driver: ::prost::alloc::string::String,
    #[prost(uint64, tag="5")]
    pub speed_mbps: u64,
    #[prost(string, tag="6")]
    pub driver_version: ::prost::alloc::string::String,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum FlowItemDisposition {
    Unspecified = 0,
    /// The exact message received the configured Kafka broker acknowledgement.
    KafkaAcked = 1,
    /// The same tenant/probe/event identity was already broker committed.
    DuplicateCommitted = 2,
    /// Deterministic validation failure; retrying identical bytes cannot help.
    RejectedInvalid = 3,
    /// Broker or dependency reported a retryable failure before a durable ACK.
    Retryable = 4,
    /// The remote outcome is ambiguous; retry must preserve the same event ID.
    OutcomeUnknown = 5,
}
impl FlowItemDisposition {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            FlowItemDisposition::Unspecified => "FLOW_ITEM_DISPOSITION_UNSPECIFIED",
            FlowItemDisposition::KafkaAcked => "FLOW_ITEM_DISPOSITION_KAFKA_ACKED",
            FlowItemDisposition::DuplicateCommitted => "FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED",
            FlowItemDisposition::RejectedInvalid => "FLOW_ITEM_DISPOSITION_REJECTED_INVALID",
            FlowItemDisposition::Retryable => "FLOW_ITEM_DISPOSITION_RETRYABLE",
            FlowItemDisposition::OutcomeUnknown => "FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "FLOW_ITEM_DISPOSITION_UNSPECIFIED" => Some(Self::Unspecified),
            "FLOW_ITEM_DISPOSITION_KAFKA_ACKED" => Some(Self::KafkaAcked),
            "FLOW_ITEM_DISPOSITION_DUPLICATE_COMMITTED" => Some(Self::DuplicateCommitted),
            "FLOW_ITEM_DISPOSITION_REJECTED_INVALID" => Some(Self::RejectedInvalid),
            "FLOW_ITEM_DISPOSITION_RETRYABLE" => Some(Self::Retryable),
            "FLOW_ITEM_DISPOSITION_OUTCOME_UNKNOWN" => Some(Self::OutcomeUnknown),
            _ => None,
        }
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum AssetBindingItemDisposition {
    Unspecified = 0,
    KafkaAcked = 1,
    DuplicateCommitted = 2,
    RejectedInvalid = 3,
    Retryable = 4,
    OutcomeUnknown = 5,
}
impl AssetBindingItemDisposition {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            AssetBindingItemDisposition::Unspecified => "ASSET_BINDING_ITEM_DISPOSITION_UNSPECIFIED",
            AssetBindingItemDisposition::KafkaAcked => "ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED",
            AssetBindingItemDisposition::DuplicateCommitted => "ASSET_BINDING_ITEM_DISPOSITION_DUPLICATE_COMMITTED",
            AssetBindingItemDisposition::RejectedInvalid => "ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID",
            AssetBindingItemDisposition::Retryable => "ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE",
            AssetBindingItemDisposition::OutcomeUnknown => "ASSET_BINDING_ITEM_DISPOSITION_OUTCOME_UNKNOWN",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "ASSET_BINDING_ITEM_DISPOSITION_UNSPECIFIED" => Some(Self::Unspecified),
            "ASSET_BINDING_ITEM_DISPOSITION_KAFKA_ACKED" => Some(Self::KafkaAcked),
            "ASSET_BINDING_ITEM_DISPOSITION_DUPLICATE_COMMITTED" => Some(Self::DuplicateCommitted),
            "ASSET_BINDING_ITEM_DISPOSITION_REJECTED_INVALID" => Some(Self::RejectedInvalid),
            "ASSET_BINDING_ITEM_DISPOSITION_RETRYABLE" => Some(Self::Retryable),
            "ASSET_BINDING_ITEM_DISPOSITION_OUTCOME_UNKNOWN" => Some(Self::OutcomeUnknown),
            _ => None,
        }
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash, PartialOrd, Ord, ::prost::Enumeration)]
#[repr(i32)]
pub enum ProbeGroupReadinessStateV1 {
    Unspecified = 0,
    Assigned = 1,
    Ready = 2,
    Revoked = 3,
    Stopped = 4,
}
impl ProbeGroupReadinessStateV1 {
    /// String value of the enum field names used in the ProtoBuf definition.
    ///
    /// The values are not transformed in any way and thus are considered stable
    /// (if the ProtoBuf definition does not change) and safe for programmatic use.
    pub fn as_str_name(&self) -> &'static str {
        match self {
            ProbeGroupReadinessStateV1::Unspecified => "PROBE_GROUP_READINESS_STATE_V1_UNSPECIFIED",
            ProbeGroupReadinessStateV1::Assigned => "PROBE_GROUP_READINESS_STATE_V1_ASSIGNED",
            ProbeGroupReadinessStateV1::Ready => "PROBE_GROUP_READINESS_STATE_V1_READY",
            ProbeGroupReadinessStateV1::Revoked => "PROBE_GROUP_READINESS_STATE_V1_REVOKED",
            ProbeGroupReadinessStateV1::Stopped => "PROBE_GROUP_READINESS_STATE_V1_STOPPED",
        }
    }
    /// Creates an enum from field names used in the ProtoBuf definition.
    pub fn from_str_name(value: &str) -> ::core::option::Option<Self> {
        match value {
            "PROBE_GROUP_READINESS_STATE_V1_UNSPECIFIED" => Some(Self::Unspecified),
            "PROBE_GROUP_READINESS_STATE_V1_ASSIGNED" => Some(Self::Assigned),
            "PROBE_GROUP_READINESS_STATE_V1_READY" => Some(Self::Ready),
            "PROBE_GROUP_READINESS_STATE_V1_REVOKED" => Some(Self::Revoked),
            "PROBE_GROUP_READINESS_STATE_V1_STOPPED" => Some(Self::Stopped),
            _ => None,
        }
    }
}
include!("traffic.v1.serde.rs");
include!("traffic.v1.tonic.rs");
// @@protoc_insertion_point(module)