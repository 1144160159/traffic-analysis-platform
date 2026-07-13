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
}
// ==================== Flow 流式上报 ====================

#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct StreamFlowsRequest {
    #[prost(message, optional, tag="1")]
    pub event: ::core::option::Option<FlowEvent>,
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
}
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HeartbeatResponse {
    #[prost(bool, tag="1")]
    pub ok: bool,
    #[prost(message, optional, tag="2")]
    pub config: ::core::option::Option<ProbeConfig>,
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
    /// uint64 tx_dropped = 13;
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
include!("traffic.v1.serde.rs");
include!("traffic.v1.tonic.rs");
// @@protoc_insertion_point(module)