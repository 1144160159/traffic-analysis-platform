use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::Path;
use std::time::Duration;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CaptureMode {
    Xdp,
    XdpSkb,
    XdpOffload,
    AfPacket,
    PcapOffline,
}

impl Default for CaptureMode {
    fn default() -> Self {
        Self::Xdp
    }
}

impl CaptureMode {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Xdp => "xdp",
            Self::XdpSkb => "xdp_skb",
            Self::XdpOffload => "xdp_offload",
            Self::AfPacket => "af_packet",
            Self::PcapOffline => "pcap_offline",
        }
    }

    /// 是否为离线 PCAP 回放模式（不需要网卡和 frame_size 限制）
    pub fn is_pcap_offline(&self) -> bool {
        matches!(self, Self::PcapOffline)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProbeConfig {
    pub tenant_id: String,
    pub probe_id: String,
    #[serde(default = "default_run_id")]
    pub run_id: Option<String>,
    pub capture: CaptureConfig,
    pub aggregator: AggregatorConfig,
    #[serde(default)]
    pub archiver: ArchiverConfig,
    pub sender: SenderConfig,
    #[serde(default)]
    pub metrics: MetricsConfig,
}

fn default_run_id() -> Option<String> {
    Some("realtime".to_string())
}

impl ProbeConfig {
    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self> {
        let content = std::fs::read_to_string(path.as_ref())
            .context(format!("Failed to read config file: {:?}", path.as_ref()))?;

        let content = Self::expand_env_vars(&content)?;

        let mut config: Self =
            serde_yaml::from_str(&content).context("Failed to parse YAML config")?;

        config.from_env();
        config.validate()?;

        Ok(config)
    }

    fn expand_env_vars(content: &str) -> Result<String> {
        use once_cell::sync::Lazy;
        use regex::Regex;

        static RE_WITH_DEFAULT: Lazy<Regex> =
            Lazy::new(|| Regex::new(r"\$\{([A-Z_][A-Z0-9_]*):-([^}]+)\}").unwrap());

        static RE_STANDARD: Lazy<Regex> =
            Lazy::new(|| Regex::new(r"\$\{([A-Z_][A-Z0-9_]*)\}").unwrap());

        let mut result = content.to_string();

        for cap in RE_WITH_DEFAULT.captures_iter(content) {
            let var_name = &cap[1];
            let default_value = &cap[2];
            let value = std::env::var(var_name).unwrap_or_else(|_| default_value.to_string());
            let placeholder = format!("${{{}:-{}}}", var_name, default_value);
            result = result.replace(&placeholder, &value);
        }

        for cap in RE_STANDARD.captures_iter(&result.clone()) {
            let var_name = &cap[1];
            if let Ok(value) = std::env::var(var_name) {
                let placeholder = format!("${{{}}}", var_name);
                result = result.replace(&placeholder, &value);
            }
        }

        Ok(result)
    }

    pub fn from_env(&mut self) {
        if let Ok(tenant_id) = std::env::var("TENANT_ID") {
            self.tenant_id = tenant_id;
        }
        if let Ok(probe_id) = std::env::var("PROBE_ID") {
            self.probe_id = probe_id;
        }
        if let Ok(run_id) = std::env::var("RUN_ID") {
            self.run_id = Some(run_id);
        }
        if let Ok(interface) = std::env::var("CAPTURE_INTERFACE") {
            self.capture.interface = interface;
        }
        if let Ok(gateway) = std::env::var("GATEWAY_ADDR") {
            self.sender.gateway_addr = gateway;
        }
        if let Ok(token) = std::env::var("AUTH_TOKEN") {
            self.sender.auth_token = Some(token);
        }
    }

    fn validate(&self) -> Result<()> {
        if self.tenant_id.is_empty() {
            anyhow::bail!("tenant_id cannot be empty");
        }
        if self.probe_id.is_empty() {
            anyhow::bail!("probe_id cannot be empty");
        }
        if self.capture.interface.is_empty() {
            anyhow::bail!("capture.interface cannot be empty");
        }
        if self.sender.gateway_addr.is_empty() {
            anyhow::bail!("sender.gateway_addr cannot be empty");
        }

        if !self
            .tenant_id
            .chars()
            .all(|c| c.is_alphanumeric() || c == '-' || c == '_')
        {
            anyhow::bail!("tenant_id contains invalid characters (allowed: alphanumeric, -, _)");
        }
        if !self
            .probe_id
            .chars()
            .all(|c| c.is_alphanumeric() || c == '-' || c == '_')
        {
            anyhow::bail!("probe_id contains invalid characters");
        }

        if self.aggregator.flow_capacity == 0 {
            anyhow::bail!("aggregator.flow_capacity must be > 0");
        }
        if self.aggregator.flow_capacity > 100_000_000 {
            anyhow::bail!("aggregator.flow_capacity exceeds maximum (100M)");
        }

        if self.aggregator.idle_timeout_sec == 0 {
            anyhow::bail!("aggregator.idle_timeout_sec must be > 0");
        }
        if self.aggregator.idle_timeout_sec > 3600 {
            anyhow::bail!("aggregator.idle_timeout_sec exceeds 1 hour");
        }

        if self.sender.batch_size == 0 {
            anyhow::bail!("sender.batch_size must be > 0");
        }
        if self.sender.batch_size > 10_000 {
            anyhow::bail!("sender.batch_size exceeds maximum (10k)");
        }

        if self.capture.frame_size % 4096 != 0 {
            anyhow::bail!(
                "capture.frame_size ({}) must be multiple of 4096 (Kunpeng requirement)",
                self.capture.frame_size
            );
        }

        if self.archiver.enabled && self.archiver.buffer_size_mb < 64 {
            anyhow::bail!("archiver.buffer_size_mb should be >= 64 MB");
        }

        Ok(())
    }

    pub fn batch_timeout(&self) -> Duration {
        Duration::from_millis(self.sender.batch_timeout_ms)
    }

    pub fn idle_timeout(&self) -> Duration {
        self.aggregator.idle_timeout()
    }

    pub fn active_timeout(&self) -> Duration {
        self.aggregator.active_timeout()
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CaptureConfig {
    pub interface: String,
    #[serde(default)]
    pub mode: CaptureMode,
    #[serde(default)]
    pub queue_id: u32,
    #[serde(default = "default_buffer_size")]
    pub buffer_size: usize,
    #[serde(default = "default_frame_size")]
    pub frame_size: usize,
    #[serde(default = "default_frame_count")]
    pub frame_count: usize,
    pub bpf_filter: Option<String>,
    #[serde(default = "default_promiscuous_mode")]
    pub promiscuous_mode: bool,
    #[serde(default)]
    pub cpu_cores: Vec<u32>,
    #[serde(default)]
    pub numa_aware: bool,
    /// PCAP offline mode: directory or file path for pcap files
    #[serde(default)]
    pub pcap_dir: Option<String>,
    /// PCAP replay speed: "original", "max", or "2x", "5x" etc.
    #[serde(default)]
    pub replay_speed: Option<String>,
    /// Loop replay when all files are consumed
    #[serde(default)]
    pub loop_replay: Option<bool>,
}

fn default_buffer_size() -> usize {
    64 * 1024 * 1024
}

fn default_frame_size() -> usize {
    4096
}

fn default_frame_count() -> usize {
    16384
}

fn default_promiscuous_mode() -> bool {
    false
}

impl Default for CaptureConfig {
    fn default() -> Self {
        Self {
            interface: "eth0".to_string(),
            mode: CaptureMode::default(),
            queue_id: 0,
            buffer_size: default_buffer_size(),
            frame_size: default_frame_size(),
            frame_count: default_frame_count(),
            bpf_filter: None,
            promiscuous_mode: default_promiscuous_mode(),
            cpu_cores: Vec::new(),
            numa_aware: false,
            pcap_dir: None,
            replay_speed: None,
            loop_replay: None,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AggregatorConfig {
    #[serde(default = "default_flow_capacity")]
    pub flow_capacity: usize,
    #[serde(default = "default_idle_timeout")]
    pub idle_timeout_sec: u64,
    #[serde(default = "default_active_timeout")]
    pub active_timeout_sec: u64,
    #[serde(default = "default_scan_interval")]
    pub scan_interval_sec: u64,
    /// 使用分代流表 (young/old/tenured 三层)，默认使用分区流表
    #[serde(default)]
    pub use_generational: bool,
}

fn default_flow_capacity() -> usize {
    1_000_000
}

fn default_idle_timeout() -> u64 {
    120
}

fn default_active_timeout() -> u64 {
    1800
}

fn default_scan_interval() -> u64 {
    1
}

impl Default for AggregatorConfig {
    fn default() -> Self {
        Self {
            flow_capacity: default_flow_capacity(),
            idle_timeout_sec: default_idle_timeout(),
            active_timeout_sec: default_active_timeout(),
            scan_interval_sec: default_scan_interval(),
            use_generational: false,
        }
    }
}

impl AggregatorConfig {
    pub fn idle_timeout(&self) -> Duration {
        Duration::from_secs(self.idle_timeout_sec)
    }

    pub fn active_timeout(&self) -> Duration {
        Duration::from_secs(self.active_timeout_sec)
    }

    pub fn scan_interval(&self) -> Duration {
        Duration::from_secs(self.scan_interval_sec)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArchiverConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_archiver_buffer_size")]
    pub buffer_size_mb: usize,
    #[serde(default = "default_rotation_interval")]
    pub rotation_interval_sec: u64,
    #[serde(default = "default_zstd_level")]
    pub zstd_level: i32,
    #[serde(default = "default_s3_endpoint")]
    pub s3_endpoint: String,
    #[serde(default = "default_s3_bucket")]
    pub s3_bucket: String,
    #[serde(default = "default_s3_region")]
    pub s3_region: String,
    #[serde(default)]
    pub s3_access_key: String,
    #[serde(default)]
    pub s3_secret_key: String,
    #[serde(default = "default_max_uploads")]
    pub max_concurrent_uploads: usize,
    #[serde(default = "default_cache_path")]
    pub cache_path: String,
}

fn default_true() -> bool {
    true
}

fn default_archiver_buffer_size() -> usize {
    256
}

fn default_rotation_interval() -> u64 {
    60
}

fn default_zstd_level() -> i32 {
    3
}

fn default_s3_endpoint() -> String {
    "minio.minio.svc:9000".to_string()
}

fn default_s3_bucket() -> String {
    "pcap-archive".to_string()
}

fn default_s3_region() -> String {
    "us-east-1".to_string()
}

fn default_max_uploads() -> usize {
    4
}

fn default_cache_path() -> String {
    "/var/lib/probe-agent/cache".to_string()
}

impl Default for ArchiverConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            buffer_size_mb: default_archiver_buffer_size(),
            rotation_interval_sec: default_rotation_interval(),
            zstd_level: default_zstd_level(),
            s3_endpoint: default_s3_endpoint(),
            s3_bucket: default_s3_bucket(),
            s3_region: default_s3_region(),
            s3_access_key: std::env::var("PROBE_S3_ACCESS_KEY").unwrap_or_default(),
            s3_secret_key: std::env::var("PROBE_S3_SECRET_KEY").unwrap_or_default(),
            max_concurrent_uploads: default_max_uploads(),
            cache_path: default_cache_path(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SenderConfig {
    pub gateway_addr: String,
    #[serde(default = "default_batch_size")]
    pub batch_size: usize,
    #[serde(default = "default_batch_timeout")]
    pub batch_timeout_ms: u64,
    #[serde(default = "default_max_retries")]
    pub max_retries: usize,
    pub tls_ca_cert: Option<String>,
    pub tls_client_cert: Option<String>,
    pub tls_client_key: Option<String>,
    pub auth_token: Option<String>,
    pub tenant_id: String,
    pub probe_id: Option<String>,
    #[serde(default = "default_cache_path")]
    pub cache_path: String,
    #[serde(default = "default_cache_max_size")]
    pub cache_max_size: usize,
}

fn default_batch_size() -> usize {
    100
}

fn default_batch_timeout() -> u64 {
    100
}

fn default_max_retries() -> usize {
    3
}

fn default_cache_max_size() -> usize {
    1_000_000
}

impl Default for SenderConfig {
    fn default() -> Self {
        Self {
            gateway_addr: "https://ingest-gateway:50051".to_string(),
            batch_size: 100,
            batch_timeout_ms: 100,
            max_retries: 3,
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            tenant_id: String::new(),
            probe_id: None,
            cache_path: "/var/lib/probe-agent/cache".to_string(),
            cache_max_size: 1_000_000,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_metrics_listen")]
    pub listen_addr: String,
}

fn default_metrics_listen() -> String {
    "0.0.0.0:9091".to_string()
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            listen_addr: default_metrics_listen(),
        }
    }
}

// ============================================================================
// 配置验证 — 启动时检查关键参数合法性，提前发现配置错误
// ============================================================================
