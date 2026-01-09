use tonic::transport::{Channel, ClientTlsConfig, Certificate, Identity};
use tonic::Request;
use tokio::sync::mpsc::Receiver;
use tokio::time::{interval, Duration};
use std::path::PathBuf;
use std::fs;
use anyhow::{Result, Context};
use tracing::{info, warn, error};

use proto_gen::ingest_service_client::IngestServiceClient;
use proto_gen::{FlowEvent, BatchUploadRequest};

use super::retry::LocalCache;

#[derive(Clone)]
pub struct GrpcSenderConfig {
    pub gateway_addr: String,
    pub tls_ca_cert: Option<PathBuf>,
    pub tls_client_cert: Option<PathBuf>,
    pub tls_client_key: Option<PathBuf>,
    pub batch_size: usize,
    pub batch_timeout: Duration,
    pub max_retries: usize,
    pub tenant_id: String,
    pub probe_id: String,
    pub local_cache_path: PathBuf,
}

pub struct GrpcSender {
    config: GrpcSenderConfig,
    client: Option<IngestServiceClient<Channel>>,
    rx: Receiver<FlowEvent>,
    local_cache: LocalCache,
    batch_buffer: Vec<FlowEvent>,
    stats: SenderStats,
}

#[derive(Default)]
struct SenderStats {
    total_sent: u64,
    total_failed: u64,
    total_cached: u64,
}

impl GrpcSender {
    pub async fn new(
        config: GrpcSenderConfig,
        rx: Receiver<FlowEvent>,
    ) -> Result<Self> {
        // 初始化本地缓存
        fs::create_dir_all(&config.local_cache_path)?;
        let local_cache = LocalCache::new(&config.local_cache_path, 1_000_000)?;
        
        Ok(Self {
            config,
            client: None,
            rx,
            local_cache,
            batch_buffer: Vec::with_capacity(100),
            stats: SenderStats::default(),
        })
    }

    /// 初始化 gRPC 连接
    async fn init_client(&mut self) -> Result<()> {
        let mut endpoint = Channel::from_shared(self.config.gateway_addr.clone())?
            .timeout(Duration::from_secs(10))
            .connect_timeout(Duration::from_secs(5));
        
        // 配置 mTLS
        if let (Some(ca_cert), Some(client_cert), Some(client_key)) = (
            &self.config.tls_ca_cert,
            &self.config.tls_client_cert,
            &self.config.tls_client_key,
        ) {
            let ca_cert_pem = fs::read_to_string(ca_cert)
                .context("Failed to read CA certificate")?;
            let client_cert_pem = fs::read_to_string(client_cert)
                .context("Failed to read client certificate")?;
            let client_key_pem = fs::read_to_string(client_key)
                .context("Failed to read client key")?;
            
            let tls = ClientTlsConfig::new()
                .ca_certificate(Certificate::from_pem(ca_cert_pem))
                .identity(Identity::from_pem(client_cert_pem, client_key_pem));
            
            endpoint = endpoint.tls_config(tls)?;
        }
        
        let channel = endpoint.connect().await?;
        self.client = Some(IngestServiceClient::new(channel));
        
        info!("✓ gRPC client connected to {}", self.config.gateway_addr);
        Ok(())
    }

    /// 主循环
    pub async fn run(&mut self) {
        let mut ticker = interval(self.config.batch_timeout);
        
        // 尝试初始化客户端
        if let Err(e) = self.init_client().await {
            error!("Failed to initialize gRPC client: {}", e);
        }
        
        loop {
            tokio::select! {
                // 接收新事件
                Some(event) = self.rx.recv() => {
                    self.batch_buffer.push(event);
                    
                    if self.batch_buffer.len() >= self.config.batch_size {
                        self.flush_batch().await;
                    }
                }
                
                // 定时器触发
                _ = ticker.tick() => {
                    if !self.batch_buffer.is_empty() {
                        self.flush_batch().await;
                    }
                    
                    // 尝试发送缓存的数据
                    self.retry_cached().await;
                    
                    // 定期打印统计
                    if self.stats.total_sent % 10000 == 0 && self.stats.total_sent > 0 {
                        info!(
                            "📊 Sender stats: sent={}, failed={}, cached={}",
                            self.stats.total_sent,
                            self.stats.total_failed,
                            self.stats.total_cached
                        );
                    }
                }
            }
        }
    }

    /// 发送批量数据
    async fn flush_batch(&mut self) {
        if self.batch_buffer.is_empty() {
            return;
        }
        
        let batch = std::mem::replace(
            &mut self.batch_buffer,
            Vec::with_capacity(self.config.batch_size),
        );
        
        let batch_size = batch.len();
        
        if let Err(e) = self.send_batch(batch.clone()).await {
            warn!("Failed to send batch (size={}): {}", batch_size, e);
            self.handle_failure(batch).await;
        } else {
            self.stats.total_sent += batch_size as u64;
        }
    }

    /// 发送单个批次
    async fn send_batch(&mut self, batch: Vec<FlowEvent>) -> Result<()> {
        // 确保客户端已连接
        if self.client.is_none() {
            self.init_client().await?;
        }
        
        let client = self.client.as_mut()
            .context("gRPC client not initialized")?;
        
        let request = Request::new(BatchUploadRequest {
            events: batch.clone(),
            compression: "none".to_string(),
        });
        
        let response = client.upload_flows(request).await
            .context("gRPC call failed")?;
        
        let result = response.into_inner();
        
        if result.rejected > 0 {
            warn!(
                "Batch partially rejected: {} accepted, {} rejected",
                result.accepted,
                result.rejected
            );
        }
        
        Ok(())
    }

    /// 处理发送失败
    async fn handle_failure(&mut self, batch: Vec<FlowEvent>) {
        self.stats.total_failed += batch.len() as u64;
        
        if let Err(e) = self.local_cache.save(&batch) {
            error!("Failed to cache batch: {}", e);
            self.stats.total_cached += batch.len() as u64;
        }
        
        // 断开连接，下次重连
        self.client = None;
    }

    /// 重试缓存的数据
    async fn retry_cached(&mut self) {
        if self.client.is_none() {
            return;
        }
        
        match self.local_cache.get_pending(10) {
            Ok(batches) => {
                for (key, batch) in batches {
                    if self.send_batch(batch).await.is_ok() {
                        self.local_cache.remove(key).ok();
                        info!("✓ Retried cached batch: {}", key);
                    } else {
                        break;  // 停止重试，等待下次
                    }
                }
            }
            Err(e) => {
                warn!("Failed to read cached batches: {}", e);
            }
        }
    }
}