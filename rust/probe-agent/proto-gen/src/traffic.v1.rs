// @generated
/// EventHeader 是所有事件的通用头部
/// 包含用于追踪、租户隔离和幂等处理的关键字段
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct EventHeader {
    /// event_id: UUID v4，全局唯一，用于幂等处理
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    /// tenant_id: 租户标识，用于数据隔离
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    /// run_id: 运行批次ID，用于区分实时流量与回放流量
    #[prost(string, tag="3")]
    pub run_id: ::prost::alloc::string::String,
    /// event_ts: 事件发生时间戳 (毫秒)
    #[prost(int64, tag="4")]
    pub event_ts: i64,
    /// ingest_ts: 接入网关接收时间戳 (毫秒)
    #[prost(int64, tag="5")]
    pub ingest_ts: i64,
    /// probe_id: 探针标识
    #[prost(string, tag="6")]
    pub probe_id: ::prost::alloc::string::String,
    /// feature_set_id: 特征集版本，用于模型训练一致性
    #[prost(string, tag="7")]
    pub feature_set_id: ::prost::alloc::string::String,
}
/// FiveTuple 网络五元组
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FiveTuple {
    /// 源 IP (IPv4/IPv6 字符串格式)
    #[prost(string, tag="1")]
    pub src_ip: ::prost::alloc::string::String,
    /// 目的 IP
    #[prost(string, tag="2")]
    pub dst_ip: ::prost::alloc::string::String,
    /// 源端口
    #[prost(uint32, tag="3")]
    pub src_port: u32,
    /// 目的端口
    #[prost(uint32, tag="4")]
    pub dst_port: u32,
    /// 协议号: 6=TCP, 17=UDP, 1=ICMP
    #[prost(uint32, tag="5")]
    pub protocol: u32,
}
/// TcpFlags TCP 标志位统计
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct TcpFlags {
    #[prost(uint32, tag="1")]
    pub syn: u32,
    #[prost(uint32, tag="2")]
    pub ack: u32,
    #[prost(uint32, tag="3")]
    pub fin: u32,
    #[prost(uint32, tag="4")]
    pub rst: u32,
    #[prost(uint32, tag="5")]
    pub psh: u32,
    #[prost(uint32, tag="6")]
    pub urg: u32,
}
/// Alert 告警对象
/// 由 Alert Service 从 DetectionEvent 聚合生成
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Alert {
    /// 基础标识
    #[prost(string, tag="1")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    /// 去重指纹 (MD5(alert_type + src_ip + dst_ip + dst_port))
    #[prost(string, tag="3")]
    pub fingerprint: ::prost::alloc::string::String,
    /// ==================== 关联信息 ====================
    #[prost(string, tag="10")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="11")]
    pub session_id: ::prost::alloc::string::String,
    /// 所属战役ID (可选)
    #[prost(string, tag="12")]
    pub campaign_id: ::prost::alloc::string::String,
    /// ==================== 网络信息 ====================
    #[prost(message, optional, tag="20")]
    pub tuple: ::core::option::Option<FiveTuple>,
    /// ==================== 分类信息 ====================
    ///
    /// 告警类型
    #[prost(string, tag="30")]
    pub alert_type: ::prost::alloc::string::String,
    /// 标签列表
    #[prost(string, repeated, tag="31")]
    pub labels: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 综合评分
    #[prost(float, tag="32")]
    pub score: f32,
    /// 严重程度
    #[prost(string, tag="33")]
    pub severity: ::prost::alloc::string::String,
    /// ==================== 时间信息 ====================
    ///
    /// 首次发现时间
    #[prost(int64, tag="40")]
    pub first_seen: i64,
    /// 最后发现时间
    #[prost(int64, tag="41")]
    pub last_seen: i64,
    /// 聚合计数
    #[prost(int32, tag="42")]
    pub count: i32,
    /// ==================== 状态管理 ====================
    /// status: "new" | "triage" | "assigned" | "closed"
    #[prost(string, tag="50")]
    pub status: ::prost::alloc::string::String,
    /// 分配给谁处理
    #[prost(string, tag="51")]
    pub assignee: ::prost::alloc::string::String,
    /// ==================== 证据 ====================
    #[prost(string, repeated, tag="60")]
    pub evidence_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// ==================== 版本信息 ====================
    #[prost(string, tag="70")]
    pub rule_version: ::prost::alloc::string::String,
    #[prost(string, tag="71")]
    pub model_version: ::prost::alloc::string::String,
    #[prost(string, tag="72")]
    pub feature_set_id: ::prost::alloc::string::String,
}
/// AlertFeedback 告警反馈
/// 用于回灌标注
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct AlertFeedback {
    #[prost(string, tag="1")]
    pub alert_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    /// label: "TP" (True Positive) | "FP" (False Positive)
    #[prost(string, tag="3")]
    pub label: ::prost::alloc::string::String,
    /// 误报原因码
    #[prost(string, tag="4")]
    pub reason_code: ::prost::alloc::string::String,
    /// 备注
    #[prost(string, tag="5")]
    pub comment: ::prost::alloc::string::String,
    /// 操作者
    #[prost(string, tag="6")]
    pub user_id: ::prost::alloc::string::String,
    #[prost(int64, tag="7")]
    pub timestamp: i64,
    /// 是否加入白名单
    #[prost(bool, tag="8")]
    pub add_to_whitelist: bool,
}
/// Campaign 攻击战役
/// 由 Flink CEP Job 通过关联分析生成
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Campaign {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    /// 战役标识
    #[prost(string, tag="2")]
    pub campaign_id: ::prost::alloc::string::String,
    /// 时间范围
    #[prost(int64, tag="3")]
    pub ts_start: i64,
    #[prost(int64, tag="4")]
    pub ts_end: i64,
    /// 关联的告警 ID 列表
    #[prost(string, repeated, tag="5")]
    pub alert_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 涉及的实体 (IP、域名、用户等)
    #[prost(string, repeated, tag="6")]
    pub entities: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 综合评分
    #[prost(float, tag="7")]
    pub score: f32,
    /// 战役摘要描述
    #[prost(string, tag="8")]
    pub summary: ::prost::alloc::string::String,
    /// 战役类型 (如 "APT", "Ransomware", "DataExfiltration")
    #[prost(string, tag="9")]
    pub campaign_type: ::prost::alloc::string::String,
    /// 攻击阶段 (参考 ATT&CK)
    #[prost(string, repeated, tag="10")]
    pub attack_phases: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 关联的检测规则/模型
    #[prost(string, repeated, tag="11")]
    pub rule_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    #[prost(string, repeated, tag="12")]
    pub model_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// DetectionEvent 检测结果事件
/// 由规则引擎或模型推理产生
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct DetectionEvent {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    /// 检测标识
    #[prost(string, tag="2")]
    pub detection_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    /// 来源信息
    ///
    /// 规则ID (规则检测时)
    #[prost(string, tag="10")]
    pub rule_id: ::prost::alloc::string::String,
    /// 规则版本
    #[prost(string, tag="11")]
    pub rule_version: ::prost::alloc::string::String,
    /// 模型ID (模型检测时)
    #[prost(string, tag="12")]
    pub model_id: ::prost::alloc::string::String,
    /// 模型版本
    #[prost(string, tag="13")]
    pub model_version: ::prost::alloc::string::String,
    /// ==================== 检测结果 ====================
    /// detection_type: "rule" | "behavior" | "business" | "anomaly"
    #[prost(string, tag="20")]
    pub detection_type: ::prost::alloc::string::String,
    /// 标签列表 (如 \["PortScan", "Reconnaissance"\])
    #[prost(string, repeated, tag="21")]
    pub labels: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 置信度得分 (0.0 - 1.0)
    #[prost(float, tag="22")]
    pub score: f32,
    /// 严重程度: "low" | "medium" | "high" | "critical"
    #[prost(string, tag="23")]
    pub severity: ::prost::alloc::string::String,
    /// ==================== 上下文信息 ====================
    #[prost(message, optional, tag="30")]
    pub tuple: ::core::option::Option<FiveTuple>,
    #[prost(int64, tag="31")]
    pub ts_start: i64,
    #[prost(int64, tag="32")]
    pub ts_end: i64,
    /// ==================== 证据数据 ====================
    /// evidence 包含用于解释检测结果的关键信息
    #[prost(message, repeated, tag="40")]
    pub evidence: ::prost::alloc::vec::Vec<EvidenceEntry>,
    /// 关联的 session/flow ID
    #[prost(string, tag="41")]
    pub session_id: ::prost::alloc::string::String,
    #[prost(string, tag="42")]
    pub flow_id: ::prost::alloc::string::String,
}
/// EvidenceEntry 证据键值对
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct EvidenceEntry {
    #[prost(string, tag="1")]
    pub key: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub value: ::prost::alloc::string::String,
}
/// FeatureStatV1 L1 统计特征
/// 由 Flink Feature Job 计算生成，用于规则检测和模型推理
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureStatV1 {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    /// 关联标识
    #[prost(string, tag="2")]
    pub community_id: ::prost::alloc::string::String,
    /// "flow" 或 "session"
    #[prost(string, tag="3")]
    pub object_type: ::prost::alloc::string::String,
    /// flow_id 或 session_id
    #[prost(string, tag="4")]
    pub object_id: ::prost::alloc::string::String,
    /// ==================== 速率特征 ====================
    ///
    /// 包速率 (packets per second)
    #[prost(float, tag="10")]
    pub pps: f32,
    /// 比特率 (bits per second)
    #[prost(float, tag="11")]
    pub bps: f32,
    /// ==================== 方向特征 ====================
    ///
    /// 上下行比例
    #[prost(float, tag="20")]
    pub up_down_ratio: f32,
    /// 正向字节占比
    #[prost(float, tag="21")]
    pub bytes_fwd_ratio: f32,
    /// ==================== TCP 标志特征 ====================
    ///
    /// SYN 包占比
    #[prost(float, tag="30")]
    pub syn_ratio: f32,
    /// FIN 包占比
    #[prost(float, tag="31")]
    pub fin_ratio: f32,
    /// RST 包占比
    #[prost(float, tag="32")]
    pub rst_ratio: f32,
    /// ACK 包占比
    #[prost(float, tag="33")]
    pub ack_ratio: f32,
    /// PSH 包占比
    #[prost(float, tag="34")]
    pub psh_ratio: f32,
    /// ==================== 时间特征 ====================
    ///
    /// 平均包间隔
    #[prost(float, tag="40")]
    pub iat_mean_ms: f32,
    /// 包间隔标准差
    #[prost(float, tag="41")]
    pub iat_std_ms: f32,
    /// 最小包间隔
    #[prost(float, tag="42")]
    pub iat_min_ms: f32,
    /// 最大包间隔
    #[prost(float, tag="43")]
    pub iat_max_ms: f32,
    /// ==================== 包长特征 ====================
    ///
    /// 平均包长
    #[prost(float, tag="50")]
    pub pktlen_mean: f32,
    /// 包长标准差
    #[prost(float, tag="51")]
    pub pktlen_std: f32,
    /// 最小包长
    #[prost(float, tag="52")]
    pub pktlen_min: f32,
    /// 最大包长
    #[prost(float, tag="53")]
    pub pktlen_max: f32,
    /// ==================== 活跃/空闲特征 ====================
    ///
    /// 平均活跃时间
    #[prost(float, tag="60")]
    pub active_mean_ms: f32,
    /// 平均空闲时间
    #[prost(float, tag="61")]
    pub idle_mean_ms: f32,
    /// ==================== 扩展特征 (用于模型) ====================
    ///
    /// 额外的数值特征
    #[prost(float, repeated, tag="100")]
    pub extra: ::prost::alloc::vec::Vec<f32>,
    /// 标签键列表
    #[prost(string, repeated, tag="101")]
    pub tag_keys: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// 标签值列表 (与 tag_keys 一一对应)
    #[prost(string, repeated, tag="102")]
    pub tag_values: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// FeatureSeqV1 L2 序列特征
/// 用于加密流量分析和行为检测
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FeatureSeqV1 {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    #[prost(string, tag="2")]
    pub community_id: ::prost::alloc::string::String,
    #[prost(string, tag="3")]
    pub session_id: ::prost::alloc::string::String,
    /// 滑动窗口ID
    #[prost(string, tag="4")]
    pub window_id: ::prost::alloc::string::String,
    /// 时间范围
    #[prost(int64, tag="10")]
    pub ts_start: i64,
    #[prost(int64, tag="11")]
    pub ts_end: i64,
    /// 序列数据 (前 N 个包)
    ///
    /// 包长序列
    #[prost(uint32, repeated, tag="20")]
    pub pktlen_seq: ::prost::alloc::vec::Vec<u32>,
    /// 方向序列 (1=fwd, -1=bwd)
    #[prost(int32, repeated, tag="21")]
    pub dir_seq: ::prost::alloc::vec::Vec<i32>,
    /// IAT 序列
    #[prost(float, repeated, tag="22")]
    pub iat_seq: ::prost::alloc::vec::Vec<f32>,
    /// 序列统计
    ///
    /// 包长序列哈希
    #[prost(string, tag="30")]
    pub pktlen_seq_hash: ::prost::alloc::string::String,
    /// IAT 序列哈希
    #[prost(string, tag="31")]
    pub iat_seq_hash: ::prost::alloc::string::String,
    /// 小波特征
    #[prost(float, tag="40")]
    pub wavelet_energy_fwd: f32,
    #[prost(float, tag="41")]
    pub wavelet_energy_bwd: f32,
    #[prost(float, tag="42")]
    pub wavelet_entropy_fwd: f32,
    #[prost(float, tag="43")]
    pub wavelet_entropy_bwd: f32,
}
/// FlowEvent 单向流事件
/// 这是探针上报的核心数据结构，代表一个单向网络流的统计信息
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FlowEvent {
    /// 通用头部
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    /// 流标识
    ///
    /// 流唯一ID
    #[prost(string, tag="2")]
    pub flow_id: ::prost::alloc::string::String,
    /// 双向会话ID (用于关联正反向流)
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    /// 五元组
    #[prost(message, optional, tag="4")]
    pub tuple: ::core::option::Option<FiveTuple>,
    /// 方向定义: "c2s" (client to server) 或 "s2c" (server to client)
    #[prost(string, tag="5")]
    pub direction: ::prost::alloc::string::String,
    /// ==================== 包/字节统计 ====================
    ///
    /// 正向包数
    #[prost(uint64, tag="10")]
    pub packets_fwd: u64,
    /// 反向包数
    #[prost(uint64, tag="11")]
    pub packets_bwd: u64,
    /// 正向字节数
    #[prost(uint64, tag="12")]
    pub bytes_fwd: u64,
    /// 反向字节数
    #[prost(uint64, tag="13")]
    pub bytes_bwd: u64,
    /// ==================== 时间信息 ====================
    ///
    /// 流开始时间 (毫秒)
    #[prost(int64, tag="14")]
    pub ts_start: i64,
    /// 流结束时间 (毫秒)
    #[prost(int64, tag="15")]
    pub ts_end: i64,
    /// 持续时间 (毫秒)
    #[prost(uint32, tag="16")]
    pub duration_ms: u32,
    /// ==================== TCP 标志位 ====================
    ///
    /// 正向 TCP flags OR 值
    #[prost(uint32, tag="20")]
    pub tcp_flags_fwd: u32,
    /// 反向 TCP flags OR 值
    #[prost(uint32, tag="21")]
    pub tcp_flags_bwd: u32,
    /// ==================== 包长统计 ====================
    ///
    /// 最小包长
    #[prost(uint32, tag="30")]
    pub pktlen_min: u32,
    /// 最大包长
    #[prost(uint32, tag="31")]
    pub pktlen_max: u32,
    /// 平均包长
    #[prost(float, tag="32")]
    pub pktlen_mean: f32,
    /// 包长标准差
    #[prost(float, tag="33")]
    pub pktlen_std: f32,
    /// ==================== 包间隔统计 (IAT) ====================
    ///
    /// 最小 IAT (毫秒)
    #[prost(float, tag="40")]
    pub iat_min_ms: f32,
    /// 最大 IAT
    #[prost(float, tag="41")]
    pub iat_max_ms: f32,
    /// 平均 IAT
    #[prost(float, tag="42")]
    pub iat_mean_ms: f32,
    /// IAT 标准差
    #[prost(float, tag="43")]
    pub iat_std_ms: f32,
    /// ==================== 流结束原因 ====================
    /// "timeout_idle": 空闲超时
    /// "timeout_active": 活跃超时
    /// "tcp_fin": TCP FIN
    /// "tcp_rst": TCP RST
    /// "forced": 强制刷新
    #[prost(string, tag="50")]
    pub end_reason: ::prost::alloc::string::String,
}
/// BatchUploadRequest 批量上报请求
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct BatchUploadRequest {
    #[prost(message, repeated, tag="1")]
    pub events: ::prost::alloc::vec::Vec<FlowEvent>,
    /// 可选的压缩信息
    ///
    /// "none", "zstd", "lz4"
    #[prost(string, tag="2")]
    pub compression: ::prost::alloc::string::String,
}
/// BatchUploadResponse 批量上报响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct BatchUploadResponse {
    /// 接受的事件数
    #[prost(int32, tag="1")]
    pub accepted: i32,
    /// 拒绝的事件数
    #[prost(int32, tag="2")]
    pub rejected: i32,
    /// 可选的消息
    #[prost(string, tag="3")]
    pub message: ::prost::alloc::string::String,
    /// 被拒绝的 event_id 列表 (用于重试)
    #[prost(string, repeated, tag="4")]
    pub rejected_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// PcapIndexMeta PCAP 索引元数据
/// 由探针上报，用于快速定位 PCAP 文件位置
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapIndexMeta {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub probe_id: ::prost::alloc::string::String,
    /// S3 存储路径
    #[prost(string, tag="3")]
    pub file_key: ::prost::alloc::string::String,
    /// 时间范围
    #[prost(int64, tag="4")]
    pub ts_start: i64,
    #[prost(int64, tag="5")]
    pub ts_end: i64,
    /// 文件信息
    #[prost(uint64, tag="6")]
    pub byte_size: u64,
    /// 压缩级别
    #[prost(uint32, tag="7")]
    pub zstd_level: u32,
    /// 文件校验和
    #[prost(string, tag="8")]
    pub sha256: ::prost::alloc::string::String,
    /// 可选: IP BloomFilter (Base64 编码)
    /// 用于快速判断某个 IP 是否可能存在于此文件
    #[prost(string, tag="9")]
    pub bloom_filter_b64: ::prost::alloc::string::String,
    /// 可选: 包含的 community_id 列表
    #[prost(string, repeated, tag="10")]
    pub community_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
}
/// PcapCutRequest PCAP 裁剪请求
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapCutRequest {
    #[prost(string, tag="1")]
    pub tenant_id: ::prost::alloc::string::String,
    /// 五元组过滤条件 (可选，为空表示不过滤)
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
    /// 时间范围 (必填)
    #[prost(int64, tag="7")]
    pub start_time: i64,
    #[prost(int64, tag="8")]
    pub end_time: i64,
    /// 可选: community_id (精确匹配)
    #[prost(string, tag="9")]
    pub community_id: ::prost::alloc::string::String,
    /// 可选: 最大返回包数
    #[prost(uint32, tag="10")]
    pub max_packets: u32,
}
/// PcapCutResponse PCAP 裁剪响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapCutResponse {
    #[prost(string, tag="1")]
    pub job_id: ::prost::alloc::string::String,
    /// status: "queued" | "processing" | "completed" | "failed"
    #[prost(string, tag="2")]
    pub status: ::prost::alloc::string::String,
    /// 完成后的下载 URL
    #[prost(string, tag="3")]
    pub download_url: ::prost::alloc::string::String,
    /// 进度信息
    #[prost(int32, tag="4")]
    pub progress_percent: i32,
    /// 错误信息
    #[prost(string, tag="5")]
    pub error_message: ::prost::alloc::string::String,
    /// 结果统计
    #[prost(uint64, tag="6")]
    pub total_packets: u64,
    #[prost(uint64, tag="7")]
    pub total_bytes: u64,
}
/// PcapIndexResponse PCAP 索引上报响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct PcapIndexResponse {
    #[prost(bool, tag="1")]
    pub success: bool,
    #[prost(string, tag="2")]
    pub message: ::prost::alloc::string::String,
}
/// FlowAck 流式上报确认
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct FlowAck {
    #[prost(string, tag="1")]
    pub event_id: ::prost::alloc::string::String,
    #[prost(bool, tag="2")]
    pub accepted: bool,
    #[prost(string, tag="3")]
    pub error: ::prost::alloc::string::String,
}
/// HeartbeatRequest 心跳请求
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HeartbeatRequest {
    #[prost(string, tag="1")]
    pub probe_id: ::prost::alloc::string::String,
    #[prost(string, tag="2")]
    pub tenant_id: ::prost::alloc::string::String,
    #[prost(int64, tag="3")]
    pub timestamp: i64,
    /// 探针状态信息
    #[prost(message, optional, tag="4")]
    pub status: ::core::option::Option<ProbeStatus>,
}
/// HeartbeatResponse 心跳响应
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct HeartbeatResponse {
    #[prost(bool, tag="1")]
    pub ok: bool,
    /// 可选: 服务端下发的配置更新
    #[prost(message, optional, tag="2")]
    pub config: ::core::option::Option<ProbeConfig>,
}
/// ProbeStatus 探针状态
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeStatus {
    /// CPU 使用率 (0-100)
    #[prost(float, tag="1")]
    pub cpu_usage: f32,
    /// 内存使用率 (0-100)
    #[prost(float, tag="2")]
    pub memory_usage: f32,
    /// 已捕获包数
    #[prost(uint64, tag="3")]
    pub packets_captured: u64,
    /// 丢包数
    #[prost(uint64, tag="4")]
    pub packets_dropped: u64,
    /// 当前捕获速率
    #[prost(float, tag="5")]
    pub capture_pps: f32,
    /// 当前上报速率
    #[prost(float, tag="6")]
    pub upload_bps: f32,
}
/// ProbeConfig 探针配置
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct ProbeConfig {
    #[prost(string, tag="1")]
    pub config_version: ::prost::alloc::string::String,
    /// 采样率 (0.0-1.0, 1.0 表示全量)
    #[prost(float, tag="2")]
    pub sample_rate: f32,
    /// BPF 过滤表达式
    #[prost(string, tag="3")]
    pub bpf_filter: ::prost::alloc::string::String,
    /// 流超时配置
    #[prost(uint32, tag="4")]
    pub idle_timeout_sec: u32,
    #[prost(uint32, tag="5")]
    pub active_timeout_sec: u32,
    /// 上报批次大小
    #[prost(uint32, tag="6")]
    pub batch_size: u32,
}
/// SessionEvent 双向会话事件
/// 由 Flink Session Job 从 FlowEvent 聚合生成
#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct SessionEvent {
    #[prost(message, optional, tag="1")]
    pub header: ::core::option::Option<EventHeader>,
    /// 会话标识
    ///
    /// 会话唯一ID
    #[prost(string, tag="2")]
    pub session_id: ::prost::alloc::string::String,
    /// 社区ID (与 FlowEvent 中相同)
    #[prost(string, tag="3")]
    pub community_id: ::prost::alloc::string::String,
    /// 五元组 (以客户端视角)
    #[prost(message, optional, tag="4")]
    pub tuple: ::core::option::Option<FiveTuple>,
    /// ==================== 时间信息 ====================
    ///
    /// 会话开始时间
    #[prost(int64, tag="10")]
    pub ts_start: i64,
    /// 会话结束时间
    #[prost(int64, tag="11")]
    pub ts_end: i64,
    /// 持续时间
    #[prost(uint32, tag="12")]
    pub duration_ms: u32,
    /// ==================== 统计信息 ====================
    ///
    /// 总包数
    #[prost(uint64, tag="20")]
    pub packets_total: u64,
    /// 总字节数
    #[prost(uint64, tag="21")]
    pub bytes_total: u64,
    /// 上行字节数 (C2S)
    #[prost(uint64, tag="22")]
    pub bytes_fwd: u64,
    /// 下行字节数 (S2C)
    #[prost(uint64, tag="23")]
    pub bytes_bwd: u64,
    /// 上下行比例
    #[prost(float, tag="24")]
    pub up_down_ratio: f32,
    /// ==================== TCP 状态 ====================
    ///
    /// 是否有 SYN
    #[prost(bool, tag="30")]
    pub has_syn: bool,
    /// 是否有 FIN
    #[prost(bool, tag="31")]
    pub has_fin: bool,
    /// 是否有 RST
    #[prost(bool, tag="32")]
    pub has_rst: bool,
    /// 是否建立连接
    #[prost(bool, tag="33")]
    pub is_established: bool,
    /// ==================== 聚合的 Flow ID 列表 ====================
    #[prost(string, repeated, tag="40")]
    pub flow_ids: ::prost::alloc::vec::Vec<::prost::alloc::string::String>,
    /// ==================== 会话结束原因 ====================
    #[prost(string, tag="50")]
    pub end_reason: ::prost::alloc::string::String,
}
include!("traffic.v1.tonic.rs");
// @@protoc_insertion_point(module)