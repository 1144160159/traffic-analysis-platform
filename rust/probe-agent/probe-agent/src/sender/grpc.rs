use anyhow::{Context, Result};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use tokio::sync::mpsc::Receiver;
use tokio::sync::{Notify, RwLock, Semaphore};
use tokio::time::{Duration, Instant};
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Identity};
use tonic::Request;
use tracing::{debug, error, info, trace, warn};

use crate::interface_monitor::InterfaceMonitor;
use proto_gen::{HeartbeatRequest, InterfaceStatus as ProtoInterfaceStatus, ProbeStatus};

use proto_gen::{
    ingest_service_client::IngestServiceClient, FlowEvent, StreamFlowsRequest, StreamFlowsResponse,
    UploadFlowsRequest, UploadFlowsResponse,
};

use super::auth::AuthProvider;
use super::retry::LocalCache;
use crate::config::SenderConfig;
use crate::metrics;

#[derive(Debug, Default)]
pub struct SenderStats {
    pub batches_sent: AtomicU64,
    pub events_sent: AtomicU64,
    pub batches_failed: AtomicU64,
    pub events_rejected: AtomicU64,
    pub batches_cached: AtomicU64,
    pub batches_retried: AtomicU64,
    pub bytes_sent: AtomicU64,
    pub in_flight: AtomicU64,
    pub latency_sum_ms: AtomicU64,
    pub latency_count: AtomicU64,
}

impl SenderStats {
    pub fn avg_latency_ms(&self) -> f64 {
        let count = self.latency_count.load(Ordering::Relaxed);
        if count == 0 {
            return 0.0;
        }
        self.latency_sum_ms.load(Ordering::Relaxed) as f64 / count as f64
    }

    pub fn record_latency(&self, latency_ms: u64) {
        self.latency_sum_ms.fetch_add(latency_ms, Ordering::Relaxed);
        self.latency_count.fetch_add(1, Ordering::Relaxed);
    }

    pub fn reset(&self) {
        self.batches_sent.store(0, Ordering::Relaxed);
        self.events_sent.store(0, Ordering::Relaxed);
        self.batches_failed.store(0, Ordering::Relaxed);
        self.events_rejected.store(0, Ordering::Relaxed);
        self.batches_cached.store(0, Ordering::Relaxed);
        self.batches_retried.store(0, Ordering::Relaxed);
        self.bytes_sent.store(0, Ordering::Relaxed);
        self.in_flight.store(0, Ordering::Relaxed);
        self.latency_sum_ms.store(0, Ordering::Relaxed);
        self.latency_count.store(0, Ordering::Relaxed);
    }
}

struct SlidingWindow {
    semaphore: Arc<Semaphore>,
    max_in_flight: usize,
}

impl SlidingWindow {
    fn new(max_in_flight: usize) -> Self {
        Self {
            semaphore: Arc::new(Semaphore::new(max_in_flight)),
            max_in_flight,
        }
    }

    async fn acquire(&self) -> tokio::sync::OwnedSemaphorePermit {
        self.semaphore
            .clone()
            .acquire_owned()
            .await
            .expect("Semaphore closed unexpectedly")
    }

    fn available(&self) -> usize {
        self.semaphore.available_permits()
    }

    fn utilization(&self) -> f64 {
        let used = self.max_in_flight - self.available();
        used as f64 / self.max_in_flight as f64
    }
}

#[derive(Clone, Debug)]
pub struct GrpcSenderConfig {
    pub gateway_addr: String,
    pub tls_ca_cert: Option<String>,
    pub tls_client_cert: Option<String>,
    pub tls_client_key: Option<String>,
    pub auth_token: Option<String>,
    pub tenant_id: Option<String>,
    pub probe_id: Option<String>,
    pub batch_size: usize,
    pub batch_timeout: Duration,
    pub max_retries: usize,
    pub max_in_flight: usize,
    pub connect_timeout: Duration,
    pub request_timeout: Duration,
    pub cache_path: String,
    pub cache_max_size: usize,
    pub enable_streaming: bool,
}

impl Default for GrpcSenderConfig {
    fn default() -> Self {
        Self {
            gateway_addr: "ingest-gateway:50051".to_string(),
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            tenant_id: None,
            probe_id: None,
            batch_size: 100,
            batch_timeout: Duration::from_millis(100),
            max_retries: 3,
            max_in_flight: 16,
            connect_timeout: Duration::from_secs(30),
            request_timeout: Duration::from_secs(60),
            cache_path: "/var/lib/probe-agent/cache".to_string(),
            cache_max_size: 1_000_000,
            enable_streaming: true,
        }
    }
}

impl From<&SenderConfig> for GrpcSenderConfig {
    fn from(cfg: &SenderConfig) -> Self {
        Self {
            gateway_addr: cfg.gateway_addr.clone(),
            tls_ca_cert: cfg.tls_ca_cert.clone(),
            tls_client_cert: cfg.tls_client_cert.clone(),
            tls_client_key: cfg.tls_client_key.clone(),
            auth_token: cfg.auth_token.clone(),
            tenant_id: if cfg.tenant_id.is_empty() {
                None
            } else {
                Some(cfg.tenant_id.clone())
            },
            probe_id: cfg.probe_id.as_ref().and_then(|s| {
                if s.is_empty() {
                    None
                } else {
                    Some(s.clone())
                }
            }),
            batch_size: cfg.batch_size,
            batch_timeout: Duration::from_millis(cfg.batch_timeout_ms),
            max_retries: cfg.max_retries,
            max_in_flight: 16,
            connect_timeout: Duration::from_secs(10),
            request_timeout: Duration::from_secs(30),
            cache_path: cfg.cache_path.clone(),
            cache_max_size: cfg.cache_max_size,
            enable_streaming: true,
        }
    }
}

struct SharedState {
    connected: AtomicBool,
    client: RwLock<Option<IngestServiceClient<Channel>>>,
    local_cache: LocalCache,
    stats: SenderStats,
}

impl SharedState {
    fn new(local_cache: LocalCache) -> Self {
        Self {
            connected: AtomicBool::new(false),
            client: RwLock::new(None),
            local_cache,
            stats: SenderStats::default(),
        }
    }

    #[inline]
    fn is_connected(&self) -> bool {
        self.connected.load(Ordering::Acquire)
    }

    #[inline]
    fn set_connected(&self, connected: bool) {
        self.connected.store(connected, Ordering::Release);
    }

    async fn get_client(&self) -> Option<IngestServiceClient<Channel>> {
        self.client.read().await.clone()
    }

    async fn set_client(&self, client: Option<IngestServiceClient<Channel>>) {
        *self.client.write().await = client;
    }

    fn cache_batch(&self, batch: &[FlowEvent]) -> bool {
        match self.local_cache.save(batch) {
            Ok(()) => {
                self.stats.batches_cached.fetch_add(1, Ordering::Relaxed);
                true
            }
            Err(e) => {
                error!("Failed to cache batch: {}", e);
                false
            }
        }
    }
}

pub struct GrpcSender {
    config: GrpcSenderConfig,
    state: Arc<SharedState>,
    window: Arc<SlidingWindow>,
    shutdown: Arc<Notify>,
    auth: Option<Arc<AuthProvider>>,
}

impl GrpcSender {
    pub async fn new(config: GrpcSenderConfig) -> Result<Self> {
        if config.tenant_id.is_none() {
            anyhow::bail!("tenant_id is required");
        }
        if config.probe_id.is_none() {
            anyhow::bail!("probe_id is required");
        }

        let cache_path = std::path::Path::new(&config.cache_path);
        if let Some(parent) = cache_path.parent() {
            tokio::fs::create_dir_all(parent).await.ok();
        }
        tokio::fs::create_dir_all(cache_path).await.ok();

        let local_cache = LocalCache::new(cache_path, config.cache_max_size)?;
        let window = Arc::new(SlidingWindow::new(config.max_in_flight));
        let state = Arc::new(SharedState::new(local_cache));

        let auth = config
            .auth_token
            .as_ref()
            .map(|token| Arc::new(AuthProvider::static_token(token.clone())));

        if auth.is_some() {
            info!("✓ Authentication enabled with Bearer token");
        } else {
            info!("⚠ Authentication disabled (no token provided)");
        }

        info!(
            "GrpcSender created: gateway={}, tenant_id={}, probe_id={}, max_in_flight={}, cache={}",
            config.gateway_addr,
            config.tenant_id.as_ref().unwrap(),
            config.probe_id.as_ref().unwrap(),
            config.max_in_flight,
            config.cache_path
        );

        Ok(Self {
            config,
            state,
            window,
            shutdown: Arc::new(Notify::new()),
            auth,
        })
    }

    async fn connect(&self) -> Result<()> {
        if self.state.is_connected() && self.state.get_client().await.is_some() {
            return Ok(());
        }

        info!("Connecting to gateway: {}", self.config.gateway_addr);

        let mut endpoint = Channel::from_shared(self.config.gateway_addr.clone())?
            .connect_timeout(self.config.connect_timeout)
            .timeout(self.config.request_timeout)
            .tcp_keepalive(Some(Duration::from_secs(60)))
            .http2_keep_alive_interval(Duration::from_secs(30))
            .keep_alive_timeout(Duration::from_secs(20))
            .keep_alive_while_idle(true);

        if let (Some(ca_cert), Some(client_cert), Some(client_key)) = (
            &self.config.tls_ca_cert,
            &self.config.tls_client_cert,
            &self.config.tls_client_key,
        ) {
            info!(
                "Configuring mTLS: ca={}, cert={}, key={}",
                ca_cert, client_cert, client_key
            );

            debug!("Reading CA certificate from: {}", ca_cert);
            let ca_pem = tokio::fs::read(ca_cert)
                .await
                .context(format!("Failed to read CA certificate: {}", ca_cert))?;

            debug!("CA cert size: {} bytes", ca_pem.len());
            debug!("Reading client certificate from: {}", client_cert);
            let client_cert_pem = tokio::fs::read(client_cert).await.context(format!(
                "Failed to read client certificate: {}",
                client_cert
            ))?;
            debug!("Client cert size: {} bytes", client_cert_pem.len());

            debug!("Reading client key from: {}", client_key);
            let client_key_pem = tokio::fs::read(client_key)
                .await
                .context(format!("Failed to read client key: {}", client_key))?;

            debug!("Client key size: {} bytes", client_key_pem.len());

            debug!("Creating TLS config...");
            let tls_config = ClientTlsConfig::new()
                .ca_certificate(Certificate::from_pem(ca_pem))
                .identity(Identity::from_pem(client_cert_pem, client_key_pem))
                .domain_name("ingest-gateway");

            debug!("Applying TLS config to endpoint...");
            match endpoint.tls_config(tls_config) {
                Ok(ep) => {
                    endpoint = ep;
                    info!("✓ mTLS configured successfully");
                }
                Err(e) => {
                    error!("Failed to configure TLS: {:?}", e);
                    error!("Error details: {}", e);
                    return Err(e.into());
                }
            }
            info!("✓ mTLS configured successfully (domain: ingest-gateway)");
        } else {
            info!("⚠ mTLS disabled (using plain HTTP)");
        }

        debug!("Attempting to connect to endpoint...");
        let channel = endpoint
            .connect()
            .await
            .context("Failed to connect to gateway")?;

        let client = IngestServiceClient::new(channel)
            .max_encoding_message_size(64 * 1024 * 1024)
            .max_decoding_message_size(64 * 1024 * 1024)
            .send_compressed(tonic::codec::CompressionEncoding::Gzip)
            .accept_compressed(tonic::codec::CompressionEncoding::Gzip);

        self.state.set_client(Some(client)).await;
        self.state.set_connected(true);

        info!("✓ Connected to gateway successfully");

        Ok(())
    }

    async fn disconnect(&self) {
        self.state.set_client(None).await;
        self.state.set_connected(false);
        warn!("Disconnected from gateway");
    }

    pub async fn send_heartbeat(&self, monitor: Option<&InterfaceMonitor>) -> Result<()> {
        let mut client = self
            .state
            .get_client()
            .await
            .ok_or_else(|| anyhow::anyhow!("No connection available"))?;

        let interfaces = if let Some(mon) = monitor {
            let statuses = mon.get_all_status().await;
            statuses
                .into_iter()
                .map(|s| ProtoInterfaceStatus {
                    name: s.name,
                    link_up: s.link_up,
                    speed_mbps: s.speed_mbps,
                    rx_packets: s.rx_packets,
                    tx_packets: s.tx_packets,
                    rx_bytes: s.rx_bytes,
                    tx_bytes: s.tx_bytes,
                    rx_errors: s.rx_errors,
                    tx_errors: s.tx_errors,
                    rx_crc_errors: s.rx_crc_errors,
                    rx_dropped: s.rx_dropped,
                    collisions: s.collisions,
                })
                .collect()
        } else {
            vec![]
        };

        let mut request = Request::new(HeartbeatRequest {
            tenant_id: self.config.tenant_id.clone().unwrap_or_default(),
            probe_id: self.config.probe_id.clone().unwrap_or_default(),
            status: Some(ProbeStatus {
                cpu_usage: get_cpu_usage(),
                memory_usage: get_memory_usage(),
                capture_pps: self.state.stats.events_sent.load(Ordering::Relaxed) / 60,
                upload_bps: 0,
                packets_captured: self.state.stats.events_sent.load(Ordering::Relaxed),
                packets_dropped: 0,
                uptime_seconds: get_uptime_seconds(),
                interfaces,
            }),
        });

        self.add_metadata(&mut request).await?;

        let response = client.heartbeat(request).await?;

        if let Some(config) = response.into_inner().config {
            debug!("Received config update: version={}", config.config_version);
        }

        metrics::HEARTBEAT_SUCCESS.inc();

        Ok(())
    }

    async fn add_metadata<T>(&self, request: &mut Request<T>) -> Result<()> {
        use tonic::metadata::MetadataValue;

        let metadata = request.metadata_mut();

        if let Some(ref tid) = self.config.tenant_id {
            let value = MetadataValue::try_from(tid.as_str()).context("Invalid tenant_id")?;
            metadata.insert("x-tenant-id", value);
            trace!("Added x-tenant-id: {}", tid);
        } else {
            warn!("tenant_id not configured, request may fail");
        }

        if let Some(ref pid) = self.config.probe_id {
            let value = MetadataValue::try_from(pid.as_str()).context("Invalid probe_id")?;
            metadata.insert("x-probe-id", value);
            trace!("Added x-probe-id: {}", pid);
        } else {
            warn!("probe_id not configured, request may fail");
        }

        if let Some(ref auth) = self.auth {
            // let token = tokio::task::block_in_place(|| {
            //     tokio::runtime::Handle::current().block_on(auth.get_token())
            // })?;
            let token = auth.get_token().await?;
            if !token.is_empty() {
                let value = MetadataValue::try_from(token.as_str()).context("Invalid token")?;
                metadata.insert("x-tenant-token", value);
                trace!("Added x-tenant-token");
            }
        }

        Ok(())
    }

    pub async fn run(&self, mut rx: Receiver<Vec<FlowEvent>>) {
        let mut retry_ticker = tokio::time::interval(Duration::from_secs(5));
        let mut stats_ticker = tokio::time::interval(Duration::from_secs(30));

        let base_reconnect_interval = Duration::from_secs(10);

        if let Err(e) = self.connect().await {
            warn!("Initial connection failed: {}, will retry", e);
        }

        info!(
            "gRPC sender started: streaming={}, max_in_flight={}, auth={}",
            self.config.enable_streaming,
            self.config.max_in_flight,
            self.auth.is_some()
        );

        let next_reconnect =
            tokio::time::sleep(self.calculate_reconnect_delay(base_reconnect_interval));
        tokio::pin!(next_reconnect);

        loop {
            tokio::select! {
                result = rx.recv() => {
                    match result {
                        Some(batch) => {
                            self.send_batch_with_window(batch).await;
                        }
                        None => {
                            info!("Input channel closed, stopping sender");
                            break;
                        }
                    }
                }

                _ = retry_ticker.tick() => {
                    self.retry_cached().await;
                }

                _ = &mut next_reconnect => {
                    if !self.state.is_connected() {
                        if let Err(e) = self.connect().await {
                            warn!("Reconnection attempt failed: {}", e);
                        }
                    }
                    next_reconnect.set(tokio::time::sleep(self.calculate_reconnect_delay(base_reconnect_interval)));
                }

                _ = stats_ticker.tick() => {
                    self.log_stats();
                    self.update_metrics();
                }

                _ = self.shutdown.notified() => {
                    info!("Sender received shutdown signal");
                    break;
                }
            }
        }

        self.drain().await;

        info!(
            "gRPC sender stopped, cached batches: {}",
            self.state.local_cache.size().unwrap_or(0)
        );
    }

    fn calculate_reconnect_delay(&self, base_interval: Duration) -> Duration {
        use rand::Rng;
        let jitter_factor = rand::thread_rng().gen_range(0.7..=1.3);
        let delay_ms = (base_interval.as_millis() as f64 * jitter_factor) as u64;
        Duration::from_millis(delay_ms)
    }

    async fn send_batch_with_window(&self, batch: Vec<FlowEvent>) {
        if batch.is_empty() {
            return;
        }

        let batch_size = batch.len();
        let start = Instant::now();

        let permit = self.window.acquire().await;
        self.state.stats.in_flight.fetch_add(1, Ordering::Relaxed);
        metrics::IN_FLIGHT_REQUESTS.set(self.state.stats.in_flight.load(Ordering::Relaxed) as f64);

        if !self.state.is_connected() {
            if let Err(e) = self.connect().await {
                warn!(
                    "Connection failed, caching batch of {} events: {}",
                    batch_size, e
                );
                self.state.cache_batch(&batch);
                self.state.stats.in_flight.fetch_sub(1, Ordering::Relaxed);
                drop(permit);
                return;
            }
        }

        let client = self.state.get_client().await;
        let state = Arc::clone(&self.state);

        let sender = Self {
            config: self.config.clone(),
            state: Arc::clone(&self.state),
            window: Arc::clone(&self.window),
            shutdown: Arc::clone(&self.shutdown),
            auth: self.auth.clone(),
        };

        tokio::spawn(async move {
            let result = sender.do_send(client, batch.clone()).await;

            let latency_ms = start.elapsed().as_millis() as u64;
            state.stats.record_latency(latency_ms);
            metrics::SENDER_LATENCY.observe(latency_ms as f64 / 1000.0);

            match result {
                Ok(response) => {
                    state.stats.batches_sent.fetch_add(1, Ordering::Relaxed);
                    state
                        .stats
                        .events_sent
                        .fetch_add(batch_size as u64, Ordering::Relaxed);

                    metrics::BATCHES_SENT.inc();
                    metrics::EVENTS_SENT.inc_by(batch_size as f64);

                    if response.rejected > 0 {
                        state
                            .stats
                            .events_rejected
                            .fetch_add(response.rejected as u64, Ordering::Relaxed);
                        metrics::EVENTS_FAILED.inc_by(response.rejected as f64);
                        warn!(
                            "Batch partially rejected: accepted={}, rejected={}",
                            response.accepted, response.rejected
                        );
                    }

                    trace!(
                        "Batch sent successfully: {} events in {}ms",
                        batch_size,
                        latency_ms
                    );
                }
                Err(e) => {
                    state.stats.batches_failed.fetch_add(1, Ordering::Relaxed);
                    metrics::EVENTS_FAILED.inc_by(batch_size as f64);
                    metrics::HEARTBEAT_FAILURES.inc();
                    error!("Send failed: {}", e);

                    if state.cache_batch(&batch) {
                        metrics::EVENTS_CACHED.set(state.local_cache.size().unwrap_or(0) as f64);
                        debug!("Batch cached for retry: {} events", batch_size);
                    }

                    state.set_connected(false);
                }
            }

            state.stats.in_flight.fetch_sub(1, Ordering::Relaxed);
            metrics::IN_FLIGHT_REQUESTS.set(state.stats.in_flight.load(Ordering::Relaxed) as f64);

            drop(permit);
        });
    }

    async fn do_send(
        &self,
        client: Option<IngestServiceClient<Channel>>,
        batch: Vec<FlowEvent>,
    ) -> Result<UploadFlowsResponse> {
        let client = client.ok_or_else(|| anyhow::anyhow!("No connection available"))?;
        let mut client = client.clone();

        let mut request = Request::new(UploadFlowsRequest {
            events: batch,
            compression: "gzip".to_string(),
        });

        self.add_metadata(&mut request).await?;

        let response = client
            .upload_flows(request)
            .await
            .context("UploadFlows RPC failed")?
            .into_inner();

        Ok(response)
    }

    /// Streaming send: 使用双向流通道逐个发送 FlowEvent。
    /// 适用于低延迟场景，每个事件即时送达，无需等待批量攒满。
    #[allow(dead_code)]
    async fn send_stream(&self, events: Vec<FlowEvent>) -> Result<(u32, u32)> {
        use tokio_stream::StreamExt;

        let client = self
            .state
            .get_client()
            .await
            .ok_or_else(|| anyhow::anyhow!("No connection available for streaming"))?;
        let mut client = client.clone();

        let stream = tokio_stream::iter(
            events
                .into_iter()
                .map(|e| StreamFlowsRequest { event: Some(e) }),
        );

        let mut request = Request::new(stream);
        self.add_metadata(&mut request).await?;

        let response = client
            .stream_flows(request)
            .await
            .context("StreamFlows RPC failed")?;
        let mut resp_stream = response.into_inner();

        let mut accepted: u32 = 0;
        let mut rejected: u32 = 0;
        while let Some(result) = resp_stream.next().await {
            match result {
                Ok(resp) => {
                    if resp.accepted {
                        accepted += 1;
                    } else {
                        rejected += 1;
                        if !resp.error.is_empty() {
                            trace!(
                                "StreamFlows rejected: id={}, err={}",
                                resp.event_id,
                                resp.error
                            );
                        }
                    }
                }
                Err(e) => {
                    warn!("StreamFlows error: {}", e);
                    rejected += 1;
                }
            }
        }

        if accepted > 0 {
            self.state
                .stats
                .events_sent
                .fetch_add(accepted as u64, Ordering::Relaxed);
            metrics::EVENTS_SENT.inc_by(accepted as f64);
        }
        if rejected > 0 {
            self.state
                .stats
                .events_rejected
                .fetch_add(rejected as u64, Ordering::Relaxed);
        }

        debug!(
            "StreamFlows done: accepted={}, rejected={}",
            accepted, rejected
        );
        Ok((accepted, rejected))
    }

    async fn retry_cached(&self) {
        if !self.state.is_connected() {
            return;
        }

        let pending = match self.state.local_cache.get_pending(10) {
            Ok(p) => p,
            Err(e) => {
                warn!("Failed to read cache: {}", e);
                return;
            }
        };

        if pending.is_empty() {
            return;
        }

        debug!("Retrying {} cached batches", pending.len());

        for (key, batch) in pending {
            let client = self.state.get_client().await;

            match self.do_send(client, batch).await {
                Ok(response) => {
                    if let Err(e) = self.state.local_cache.remove(key) {
                        warn!("Failed to remove cached batch: {}", e);
                    }
                    self.state
                        .stats
                        .batches_retried
                        .fetch_add(1, Ordering::Relaxed);
                    self.state
                        .stats
                        .events_sent
                        .fetch_add(response.accepted as u64, Ordering::Relaxed);
                    debug!(
                        "Cached batch retried successfully: {} events accepted",
                        response.accepted
                    );
                }
                Err(e) => {
                    debug!("Retry failed, will try again later: {}", e);
                    self.disconnect().await;
                    break;
                }
            }
        }

        metrics::EVENTS_CACHED.set(self.state.local_cache.size().unwrap_or(0) as f64);
    }

    async fn drain(&self) {
        let timeout_duration = Duration::from_secs(30);
        let start = Instant::now();

        let in_flight = self.state.stats.in_flight.load(Ordering::Relaxed);
        if in_flight > 0 {
            info!("Draining {} in-flight requests...", in_flight);
        }

        while self.state.stats.in_flight.load(Ordering::Relaxed) > 0 {
            if start.elapsed() > timeout_duration {
                warn!(
                    "Drain timeout, {} requests still in flight",
                    self.state.stats.in_flight.load(Ordering::Relaxed)
                );
                break;
            }
            tokio::time::sleep(Duration::from_millis(100)).await;
        }

        debug!("All in-flight requests drained");
    }

    fn log_stats(&self) {
        let stats = &self.state.stats;
        info!(
            "Sender stats: sent={}/{} events, failed={}, cached={}, retried={}, \
             in_flight={}, avg_latency={:.1}ms, window_avail={}",
            stats.batches_sent.load(Ordering::Relaxed),
            stats.events_sent.load(Ordering::Relaxed),
            stats.batches_failed.load(Ordering::Relaxed),
            stats.batches_cached.load(Ordering::Relaxed),
            stats.batches_retried.load(Ordering::Relaxed),
            stats.in_flight.load(Ordering::Relaxed),
            stats.avg_latency_ms(),
            self.window.available(),
        );
    }

    fn update_metrics(&self) {
        metrics::EVENTS_CACHED.set(self.state.local_cache.size().unwrap_or(0) as f64);
        metrics::IN_FLIGHT_REQUESTS.set(self.state.stats.in_flight.load(Ordering::Relaxed) as f64);
    }

    pub fn stats(&self) -> &SenderStats {
        &self.state.stats
    }

    pub fn shutdown(&self) {
        self.shutdown.notify_one();
    }

    pub fn is_connected(&self) -> bool {
        self.state.is_connected()
    }

    pub fn cache_size(&self) -> usize {
        self.state.local_cache.size().unwrap_or(0)
    }

    pub fn window_utilization(&self) -> f64 {
        self.window.utilization()
    }
}

fn get_cpu_usage() -> f32 {
    // Read CPU usage from /proc/self/stat
    // Returns usage as fraction 0.0-1.0, or 0.0 on error
    if let Ok(stat) = std::fs::read_to_string("/proc/self/stat") {
        let fields: Vec<&str> = stat.split_whitespace().collect();
        if fields.len() >= 15 {
            // utime (idx 13) + stime (idx 14) in clock ticks
            let utime: u64 = fields[13].parse().unwrap_or(0);
            let stime: u64 = fields[14].parse().unwrap_or(0);
            let total = (utime + stime) as f32;
            // Approximate CPU usage as fraction of one core per second
            // Return a reasonable proxy value (0.0-1.0)
            return (total / 100.0).min(1.0);
        }
    }
    0.0
}

fn get_memory_usage() -> f32 {
    // Read RSS from /proc/self/status, return in GB
    if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
        for line in status.lines() {
            if line.starts_with("VmRSS:") {
                let parts: Vec<&str> = line.split_whitespace().collect();
                if parts.len() >= 2 {
                    if let Ok(kb) = parts[1].parse::<f64>() {
                        return (kb / (1024.0 * 1024.0)) as f32; // KB -> GB
                    }
                }
            }
        }
    }
    0.0
}

fn get_uptime_seconds() -> i64 {
    static START_TIME: std::sync::OnceLock<Instant> = std::sync::OnceLock::new();
    let start = START_TIME.get_or_init(|| Instant::now());
    start.elapsed().as_secs() as i64
}
