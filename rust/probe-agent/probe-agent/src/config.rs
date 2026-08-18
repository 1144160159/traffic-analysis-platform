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

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ParserRoute {
    #[default]
    Full,
    Fast,
    Shadow,
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

        config.from_env()?;
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

    pub fn from_env(&mut self) -> Result<()> {
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
        if let Ok(enabled) = std::env::var("M02_CAPTURE_PRODUCER_V1_ENABLED") {
            self.capture.producer_enabled =
                parse_strict_bool("M02_CAPTURE_PRODUCER_V1_ENABLED", &enabled)?;
        }
        if let Ok(probe_ids) = std::env::var("M02_CAPTURE_CANARY_PROBE_IDS") {
            self.capture.producer_scope_probe_ids = probe_ids
                .split(',')
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
                .collect();
        }
        if let Ok(gateway) = std::env::var("GATEWAY_ADDR") {
            self.sender.gateway_addr = gateway;
        }
        if let Ok(token) = std::env::var("AUTH_TOKEN") {
            self.sender.auth_token = Some(token);
        }
        if let Ok(enabled) = std::env::var("M06_ASSET_BINDING_UPLOAD_V1_ENABLED") {
            self.sender.asset_binding_upload_enabled =
                parse_strict_bool("M06_ASSET_BINDING_UPLOAD_V1_ENABLED", &enabled)?;
        }
        if let Ok(tenant_id) = std::env::var("M06_ASSET_BINDING_CANARY_TENANT_ID") {
            self.sender.asset_binding_canary_tenant_id = tenant_id.trim().to_owned();
        }
        if let Ok(probe_ids) = std::env::var("M06_ASSET_BINDING_CANARY_PROBE_IDS") {
            self.sender.asset_binding_canary_probe_ids = probe_ids
                .split(',')
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
                .collect();
        }
        Ok(())
    }

    fn validate(&self) -> Result<()> {
        if self.tenant_id.is_empty() {
            anyhow::bail!("tenant_id cannot be empty");
        }
        if self.probe_id.is_empty() {
            anyhow::bail!("probe_id cannot be empty");
        }
        if !self.capture.mode.is_pcap_offline() && self.capture.interface.trim().is_empty() {
            anyhow::bail!("capture.interface cannot be empty");
        }
        if self.sender.gateway_addr.is_empty() {
            anyhow::bail!("sender.gateway_addr cannot be empty");
        }
        validate_gateway_transport(&self.sender)?;
        let mut producer_scope = std::collections::BTreeSet::new();
        for probe_id in &self.capture.producer_scope_probe_ids {
            if probe_id.trim().is_empty() || probe_id == "*" {
                anyhow::bail!("M02_CAPTURE_CANARY_PROBE_IDS_REQUIRES_EXACT_PROBE_IDS");
            }
            if !producer_scope.insert(probe_id) {
                anyhow::bail!("M02_CAPTURE_CANARY_PROBE_IDS_DUPLICATE");
            }
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
        let mut binding_scope = std::collections::BTreeSet::new();
        for probe_id in &self.sender.asset_binding_canary_probe_ids {
            if probe_id.trim().is_empty() || probe_id == "*" {
                anyhow::bail!("M06_ASSET_BINDING_CANARY_PROBE_IDS_REQUIRES_EXACT_PROBE_IDS");
            }
            if !binding_scope.insert(probe_id) {
                anyhow::bail!("M06_ASSET_BINDING_CANARY_PROBE_IDS_DUPLICATE");
            }
        }
        if self.sender.asset_binding_upload_enabled
            && (self.sender.asset_binding_canary_tenant_id != self.tenant_id
                || binding_scope.is_empty()
                || !binding_scope.contains(&self.probe_id))
        {
            anyhow::bail!("M06_ASSET_BINDING_UPLOAD_REQUIRES_EXACT_TENANT_PROBE_SCOPE");
        }

        if !self.capture.mode.is_pcap_offline() && self.capture.frame_size % 4096 != 0 {
            anyhow::bail!(
                "capture.frame_size ({}) must be multiple of 4096 (Kunpeng requirement)",
                self.capture.frame_size
            );
        }

        if self.capture.pcap_manifest_route_enabled {
            if !self.capture.mode.is_pcap_offline() {
                anyhow::bail!("PCAP_MANIFEST_ROUTE_REQUIRES_OFFLINE_MODE");
            }
            if self.capture.pcap_dir.is_some() {
                anyhow::bail!("PCAP_MANIFEST_ROUTE_CONFLICTS_WITH_LEGACY_DIRECTORY");
            }
            let manifest_path = self
                .capture
                .pcap_manifest_path
                .as_deref()
                .filter(|value| !value.trim().is_empty())
                .ok_or_else(|| anyhow::anyhow!("PCAP_MANIFEST_PATH_REQUIRED"))?;
            if manifest_path.contains('\0') {
                anyhow::bail!("PCAP_MANIFEST_PATH_INVALID");
            }
            let manifest_hash = self
                .capture
                .pcap_manifest_sha256
                .as_deref()
                .filter(|value| is_lower_hex_sha256(value))
                .ok_or_else(|| anyhow::anyhow!("PCAP_MANIFEST_SHA256_REQUIRED"))?;
            debug_assert_eq!(manifest_hash.len(), 64);
        } else if self.capture.pcap_manifest_path.is_some()
            || self.capture.pcap_manifest_sha256.is_some()
        {
            anyhow::bail!("PCAP_MANIFEST_FIELDS_REQUIRE_ROUTE_ENABLED");
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

    pub fn capture_producer_admitted(&self) -> bool {
        self.capture.producer_enabled
            && (self.capture.producer_scope_probe_ids.is_empty()
                || self
                    .capture
                    .producer_scope_probe_ids
                    .iter()
                    .any(|probe_id| probe_id == &self.probe_id))
    }

    pub fn asset_binding_upload_admitted(&self) -> bool {
        self.sender.asset_binding_upload_enabled
            && self.sender.asset_binding_canary_tenant_id == self.tenant_id
            && self
                .sender
                .asset_binding_canary_probe_ids
                .iter()
                .any(|probe_id| probe_id == &self.probe_id)
    }
}

fn parse_strict_bool(name: &str, value: &str) -> Result<bool> {
    match value {
        "true" => Ok(true),
        "false" => Ok(false),
        _ => anyhow::bail!("{name} must be exactly true or false"),
    }
}

fn validate_gateway_transport(sender: &SenderConfig) -> Result<()> {
    let tls_file_count = [
        sender.tls_ca_cert.as_deref(),
        sender.tls_client_cert.as_deref(),
        sender.tls_client_key.as_deref(),
    ]
    .into_iter()
    .filter(|value| value.is_some_and(|path| !path.trim().is_empty()))
    .count();

    if tls_file_count != 0 && tls_file_count != 3 {
        anyhow::bail!(
            "sender TLS configuration must include CA certificate, client certificate and client key together"
        );
    }

    if sender.gateway_addr.starts_with("https://") {
        if tls_file_count != 3 {
            anyhow::bail!("https gateway requires the complete mTLS certificate set");
        }
        return Ok(());
    }

    if sender.gateway_addr.starts_with("http://") {
        if tls_file_count != 0 {
            anyhow::bail!("TLS certificate files cannot be used with a plaintext gateway URL");
        }
        if !is_loopback_http_endpoint(&sender.gateway_addr) {
            anyhow::bail!(
                "plaintext gateway transport is restricted to loopback development endpoints"
            );
        }
        return Ok(());
    }

    anyhow::bail!(
        "sender.gateway_addr must use an explicit https scheme, or loopback http for development"
    )
}

fn is_loopback_http_endpoint(endpoint: &str) -> bool {
    let authority = endpoint
        .strip_prefix("http://")
        .unwrap_or("")
        .split('/')
        .next()
        .unwrap_or("");
    if authority.contains('@') {
        return false;
    }
    if authority.starts_with("[::1]") {
        return authority.len() == 5 || authority.as_bytes().get(5) == Some(&b':');
    }
    let host = authority.split(':').next().unwrap_or("");
    host == "localhost" || host == "127.0.0.1"
}

fn is_lower_hex_sha256(value: &str) -> bool {
    value.len() == 64
        && value
            .as_bytes()
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CaptureConfig {
    pub interface: String,
    /// Runtime admission gate for packet production. Existing standalone
    /// configurations remain enabled; deployment manifests set this false and
    /// turn it on only after consumer readiness has been proven.
    #[serde(default = "default_capture_producer_enabled")]
    pub producer_enabled: bool,
    /// Exact probe allow-list for managed canary activation. Empty preserves the
    /// legacy standalone configuration behavior.
    #[serde(default)]
    pub producer_scope_probe_ids: Vec<String>,
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
    /// Candidate-bound offline manifest path. Disabled unless the explicit route is enabled.
    #[serde(default)]
    pub pcap_manifest_path: Option<String>,
    /// SHA-256 of the exact manifest body loaded at startup.
    #[serde(default)]
    pub pcap_manifest_sha256: Option<String>,
    /// Opt-in switch for the manifest-only route. Defaults false for compatibility.
    #[serde(default)]
    pub pcap_manifest_route_enabled: bool,
    /// 测试阶段 wire 回放接口 allowlist:pcap_replay 命令携带 interface 时的
    /// AF_PACKET 注入目标(veth 对输入端)。空 = 全部拒绝;生产探针默认关闭。
    #[serde(default)]
    pub wire_replay_interfaces: Vec<String>,
}

fn default_buffer_size() -> usize {
    64 * 1024 * 1024
}

fn default_capture_producer_enabled() -> bool {
    true
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
            producer_enabled: default_capture_producer_enabled(),
            producer_scope_probe_ids: Vec::new(),
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
            pcap_manifest_path: None,
            pcap_manifest_sha256: None,
            pcap_manifest_route_enabled: false,
            wire_replay_interfaces: Vec::new(),
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
    /// Explicit parser route. `full` is the compatibility-safe default;
    /// `fast` falls back to the full decoder outside its proven subset and
    /// `shadow` compares both decoders before one table commit.
    #[serde(default)]
    pub parser_route: ParserRoute,
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
            parser_route: ParserRoute::Full,
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
    #[serde(default)]
    pub durable_spool_enabled: bool,
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
    #[serde(default)]
    pub s3_ca_cert: Option<String>,
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
            durable_spool_enabled: false,
            buffer_size_mb: default_archiver_buffer_size(),
            rotation_interval_sec: default_rotation_interval(),
            zstd_level: default_zstd_level(),
            s3_endpoint: default_s3_endpoint(),
            s3_bucket: default_s3_bucket(),
            s3_region: default_s3_region(),
            s3_access_key: std::env::var("PROBE_S3_ACCESS_KEY").unwrap_or_default(),
            s3_secret_key: std::env::var("PROBE_S3_SECRET_KEY").unwrap_or_default(),
            s3_ca_cert: None,
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
    #[serde(default)]
    pub asset_binding_upload_enabled: bool,
    #[serde(default)]
    pub asset_binding_canary_tenant_id: String,
    #[serde(default)]
    pub asset_binding_canary_probe_ids: Vec<String>,
}

/// Derive the TLS SNI name from a gateway address. Uses the host portion when
/// it is a DNS name; falls back to the well-known service name for IP
/// addresses or malformed URLs (the mTLS certificates are issued for the
/// service DNS name).
pub fn gateway_sni(gateway_addr: &str) -> String {
    let host = gateway_addr
        .split("://")
        .nth(1)
        .and_then(|rest| rest.split([':', '/']).next())
        .unwrap_or("ingest-gateway");
    if host.is_empty() || host.parse::<std::net::IpAddr>().is_ok() {
        "ingest-gateway".to_string()
    } else {
        host.to_string()
    }
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
            asset_binding_upload_enabled: false,
            asset_binding_canary_tenant_id: String::new(),
            asset_binding_canary_probe_ids: Vec::new(),
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

#[cfg(test)]
mod transport_tests {
    use super::{
        validate_gateway_transport, AggregatorConfig, ArchiverConfig, CaptureConfig, CaptureMode,
        MetricsConfig, ProbeConfig, SenderConfig,
    };

    fn sender(endpoint: &str) -> SenderConfig {
        SenderConfig {
            gateway_addr: endpoint.to_string(),
            ..SenderConfig::default()
        }
    }

    fn with_mtls(mut config: SenderConfig) -> SenderConfig {
        config.tls_ca_cert = Some("/run/pki/ca.crt".to_string());
        config.tls_client_cert = Some("/run/pki/tls.crt".to_string());
        config.tls_client_key = Some("/run/pki/tls.key".to_string());
        config
    }

    fn probe(capture: CaptureConfig) -> ProbeConfig {
        ProbeConfig {
            tenant_id: "tenant-a".to_string(),
            probe_id: "probe-a".to_string(),
            run_id: Some("run-a".to_string()),
            capture,
            aggregator: AggregatorConfig::default(),
            archiver: ArchiverConfig::default(),
            sender: sender("http://127.0.0.1:50051"),
            metrics: MetricsConfig::default(),
        }
    }

    #[test]
    fn remote_https_requires_complete_mtls_identity() {
        let result = validate_gateway_transport(&sender("https://ingest-gateway:50051"));
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("complete mTLS"));
    }

    #[test]
    fn partial_mtls_identity_is_rejected() {
        let mut config = sender("https://ingest-gateway:50051");
        config.tls_ca_cert = Some("/run/pki/ca.crt".to_string());
        let result = validate_gateway_transport(&config);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("must include"));
    }

    #[test]
    fn remote_plaintext_transport_is_rejected() {
        let result = validate_gateway_transport(&sender("http://ingest-gateway:50051"));
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("restricted to loopback"));
    }

    #[test]
    fn loopback_plaintext_is_allowed_only_without_tls_files() {
        assert!(validate_gateway_transport(&sender("http://127.0.0.1:50051")).is_ok());
        assert!(validate_gateway_transport(&sender("http://localhost:50051")).is_ok());
        assert!(validate_gateway_transport(&sender("http://[::1]:50051")).is_ok());
        let result = validate_gateway_transport(&with_mtls(sender("http://127.0.0.1:50051")));
        assert!(result.is_err());
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("plaintext gateway URL"));
    }

    #[test]
    fn remote_https_with_complete_identity_is_allowed() {
        assert!(validate_gateway_transport(&with_mtls(sender(
            "https://ingest-gateway.traffic-analysis.svc:50051"
        )))
        .is_ok());
    }

    #[test]
    fn manifest_route_is_default_off_and_requires_exact_offline_identity() {
        let default = CaptureConfig::default();
        assert!(!default.pcap_manifest_route_enabled);
        assert!(default.pcap_manifest_path.is_none());
        assert!(default.pcap_manifest_sha256.is_none());

        let capture = CaptureConfig {
            mode: CaptureMode::PcapOffline,
            interface: String::new(),
            pcap_manifest_route_enabled: true,
            pcap_manifest_path: Some("/fixtures/manifest.json".to_string()),
            pcap_manifest_sha256: Some("a".repeat(64)),
            ..CaptureConfig::default()
        };
        assert!(probe(capture).validate().is_ok());
    }

    #[test]
    fn manifest_route_rejects_wrong_mode_legacy_mix_and_partial_fields() {
        let base = CaptureConfig {
            mode: CaptureMode::PcapOffline,
            pcap_manifest_route_enabled: true,
            pcap_manifest_path: Some("/fixtures/manifest.json".to_string()),
            pcap_manifest_sha256: Some("a".repeat(64)),
            ..CaptureConfig::default()
        };
        let mut wrong_mode = base.clone();
        wrong_mode.mode = CaptureMode::AfPacket;
        assert!(probe(wrong_mode)
            .validate()
            .unwrap_err()
            .to_string()
            .contains("REQUIRES_OFFLINE_MODE"));

        let mut mixed = base.clone();
        mixed.pcap_dir = Some("/legacy".to_string());
        assert!(probe(mixed)
            .validate()
            .unwrap_err()
            .to_string()
            .contains("CONFLICTS_WITH_LEGACY_DIRECTORY"));

        let mut invalid_hash = base;
        invalid_hash.pcap_manifest_sha256 = Some("A".repeat(64));
        assert!(probe(invalid_hash)
            .validate()
            .unwrap_err()
            .to_string()
            .contains("SHA256_REQUIRED"));

        let partial = CaptureConfig {
            mode: CaptureMode::PcapOffline,
            pcap_manifest_path: Some("/fixtures/manifest.json".to_string()),
            ..CaptureConfig::default()
        };
        assert!(probe(partial)
            .validate()
            .unwrap_err()
            .to_string()
            .contains("FIELDS_REQUIRE_ROUTE_ENABLED"));
    }

    #[test]
    fn capture_producer_admission_is_default_on_but_exact_scope_fail_closed() {
        let default = probe(CaptureConfig::default());
        assert!(default.capture_producer_admitted());

        let mut disabled = default.clone();
        disabled.capture.producer_enabled = false;
        assert!(!disabled.capture_producer_admitted());

        let mut scoped = default;
        scoped.capture.producer_scope_probe_ids = vec!["probe-canary".to_string()];
        assert!(!scoped.capture_producer_admitted());
        scoped.probe_id = "probe-canary".to_string();
        assert!(scoped.capture_producer_admitted());
    }

    #[test]
    fn asset_binding_upload_is_default_off_and_requires_exact_scope() {
        let default = probe(CaptureConfig::default());
        assert!(!default.asset_binding_upload_admitted());

        let mut missing_scope = default.clone();
        missing_scope.sender.asset_binding_upload_enabled = true;
        assert!(missing_scope.validate().is_err());

        let mut admitted = default;
        admitted.sender.asset_binding_upload_enabled = true;
        admitted.sender.asset_binding_canary_tenant_id = "tenant-a".to_owned();
        admitted.sender.asset_binding_canary_probe_ids = vec!["probe-a".to_owned()];
        assert!(admitted.validate().is_ok());
        assert!(admitted.asset_binding_upload_admitted());
    }
}

// ============================================================================
// 配置验证 — 启动时检查关键参数合法性，提前发现配置错误
// ============================================================================
