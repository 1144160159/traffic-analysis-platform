use anyhow::Result;
use tracing::{info, error, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};
use tokio::sync::mpsc;
use tokio::signal;
use std::sync::Arc;
use std::time::Duration;

mod config;
mod capture;
mod parser;
mod aggregator;
mod archiver;
mod sender;
mod metrics;

use config::ProbeConfig;
use aggregator::{FlowTable, Eviction, EvictionConfig};
use archiver::{DoubleBuffer, Uploader, UploaderConfig, UploadTask};
use sender::{GrpcSender, GrpcSenderConfig};
use metrics::MetricsServer;

#[tokio::main]
async fn main() -> Result<()> {
    // 初始化日志
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "probe_agent=debug,info".into()),
        )
        .with(tracing_subscriber::fmt::layer().json())
        .init();

    info!("🚀 NTA Probe Agent starting...");

    // 加载配置
    let config_path = std::env::var("CONFIG_PATH").unwrap_or_else(|_| "config.yaml".to_string());
    let config = ProbeConfig::from_file(&config_path)?;
    info!("✓ Configuration loaded from {}", config_path);
    info!("  Tenant ID: {}", config.tenant_id);
    info!("  Probe ID: {}", config.probe_id);
    info!("  Interface: {}", config.capture.interface);

    // 启动 Metrics Server
    let metrics_server = if config.metrics.enabled {
        info!("📊 Starting Metrics Server on {}", config.metrics.listen_addr);
        let server = MetricsServer::new(&config.metrics.listen_addr)?;
        tokio::spawn(async move {
            if let Err(e) = server.run().await {
                error!("Metrics server error: {}", e);
            }
        });
        Some(())
    } else {
        None
    };

    // 创建流表
    let flow_table = Arc::new(FlowTable::new(config.aggregator.flow_table_capacity));
    info!("✓ Flow table created (capacity: {})", config.aggregator.flow_table_capacity);

    // 创建通道
    let (flow_tx, flow_rx) = mpsc::channel(10000);
    let (upload_tx, upload_rx) = crossbeam_channel::bounded(100);

    // 启动流老化任务
    let eviction_config = EvictionConfig {
        idle_timeout: Duration::from_secs(config.aggregator.idle_timeout_sec),
        active_timeout: Duration::from_secs(config.aggregator.active_timeout_sec),
        scan_interval: Duration::from_secs(config.aggregator.scan_interval_sec),
    };
    
    let eviction = Eviction::new(
        eviction_config,
        flow_table.clone(),
        flow_tx.clone(),
        config.tenant_id.clone(),
        config.probe_id.clone(),
    );
    
    info!("✓ Flow eviction started (idle: {}s, active: {}s)", 
        config.aggregator.idle_timeout_sec, 
        config.aggregator.active_timeout_sec
    );
    
    tokio::spawn(async move {
        eviction.run().await;
    });

    // 启动 PCAP 上传器（如果启用）
    if config.archiver.enabled {
        let uploader_config = UploaderConfig {
            s3_bucket: config.archiver.s3_bucket.clone(),
            s3_region: config.archiver.s3_region.clone(),
            s3_endpoint: config.archiver.s3_endpoint.clone(),
            s3_access_key: config.archiver.s3_access_key.clone(),
            s3_secret_key: config.archiver.s3_secret_key.clone(),
            max_concurrent: config.archiver.max_concurrent_uploads,
            queue_size: 100,
            zstd_level: config.archiver.zstd_level,
        };
        
        let uploader = Uploader::new(uploader_config, upload_rx)?;
        
        info!("✓ PCAP uploader started (bucket: {})", config.archiver.s3_bucket);
        
        tokio::spawn(async move {
            uploader.run().await;
        });
    }

    // 启动 gRPC 发送器
    let sender_config = GrpcSenderConfig {
        gateway_addr: config.sender.gateway_addr.clone(),
        tls_ca_cert: config.sender.tls_ca_cert.clone(),
        tls_client_cert: config.sender.tls_client_cert.clone(),
        tls_client_key: config.sender.tls_client_key.clone(),
        batch_size: config.sender.batch_size,
        batch_timeout: Duration::from_millis(config.sender.batch_timeout_ms),
        max_retries: config.sender.max_retries,
        tenant_id: config.tenant_id.clone(),
        probe_id: config.probe_id.clone(),
        local_cache_path: config.sender.local_cache_path.clone(),
    };
    
    let mut sender = GrpcSender::new(sender_config, flow_rx).await?;
    
    info!("✓ gRPC sender initialized (gateway: {})", config.sender.gateway_addr);
    
    tokio::spawn(async move {
        sender.run().await;
    });

    // TODO: 启动捕获模块（S1.3）
    info!("⚠️  Capture module not yet implemented (S1.3)");

    info!("✅ Probe Agent is running");
    info!("   Press Ctrl+C to shutdown gracefully");

    // 等待关闭信号
    shutdown_signal().await;

    info!("🛑 Shutting down gracefully...");

    // TODO: 优雅关闭各模块
    tokio::time::sleep(Duration::from_secs(2)).await;

    info!("✅ Probe Agent stopped");
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("Failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("Failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}