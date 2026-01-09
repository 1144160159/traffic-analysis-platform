use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use anyhow::{Result, Context};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ProbeConfig {
    /// 租户标识
    pub tenant_id: String,
    
    /// 探针标识（默认使用主机名）
    pub probe_id: String,
    
    /// 捕获配置
    pub capture: CaptureConfig,
    
    /// 流聚合配置
    pub aggregator: AggregatorConfig,
    
    /// PCAP 归档配置
    pub archiver: ArchiverConfig,
    
    /// gRPC 发送配置
    pub sender: SenderConfig,
    
    /// Metrics 配置
    pub metrics: MetricsConfig,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct CaptureConfig {
    /// 网卡接口名称
    pub interface: String,
    
    /// 捕获模式: "xdp", "af_packet", "pcap"
    pub mode: String,
    
    /// 队列 ID（XDP 模式）
    pub queue_id: u32,
    
    /// 缓冲区大小（包数量）
    pub buffer_size: usize,
    
    /// Ring 大小（XDP/AF_PACKET）
    pub ring_size: u32,
    
    /// BPF 过滤表达式（可选）
    pub bpf_filter: Option<String>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct AggregatorConfig {
    /// 流表容量
    pub flow_table_capacity: usize,
    
    /// 空闲超时（秒）
    pub idle_timeout_sec: u64,
    
    /// 活跃超时（秒）
    pub active_timeout_sec: u64,
    
    /// 扫描间隔（秒）
    pub scan_interval_sec: u64,
    
    /// 输出队列大小
    pub output_queue_size: usize,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct ArchiverConfig {
    /// 是否启用 PCAP 归档
    pub enabled: bool,
    
    /// 双缓冲大小（MB）
    pub buffer_size_mb: usize,
    
    /// 轮转间隔（秒）
    pub rotation_interval_sec: u64,
    
    /// S3 配置
    pub s3: S3Config,
    