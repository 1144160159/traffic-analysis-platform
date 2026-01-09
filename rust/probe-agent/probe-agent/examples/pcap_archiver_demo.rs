// rust/probe-agent/probe-agent/examples/pcap_archiver_demo.rs
use probe_agent::archiver::{DoubleBuffer, Uploader, UploaderConfig, UploadTask};
use std::time::Duration;
use crossbeam_channel::bounded;
use std::sync::Arc;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt::init();

    // 1. 创建双缓冲
    let buffer = Arc::new(DoubleBuffer::new(
        10 * 1024 * 1024, // 10MB
        Duration::from_secs(30),
    ));

    // 2. 创建上传队列
    let (upload_tx, upload_rx) = bounded(10);

    // 3. 启动上传器
    let config = UploaderConfig {
        s3_bucket: "test-pcap".to_string(),
        s3_region: "us-east-1".to_string(),
        s3_endpoint: "http://localhost:9000".to_string(),
        s3_access_key: "minioadmin".to_string(),
        s3_secret_key: "minioadmin".to_string(),
        max_concurrent: 2,
        queue_size: 10,
        zstd_level: 3,
    };

    let uploader = Uploader::new(config, upload_rx)?;

    tokio::spawn(async move {
        uploader.run().await;
    });

    // 4. 模拟写入
    let buffer_clone = buffer.clone();
    let handle = tokio::spawn(async move {
        for i in 0..1000 {
            let packet_data = vec![0u8; 1500];
            
            if let Some(snapshot) = buffer_clone.write_packet(i * 1000, &packet_data) {
                println!("Buffer swapped: {}MB", snapshot.data.len() / 1024 / 1024);
                
                upload_tx.send(UploadTask {
                    snapshot,
                    tenant_id: "tenant-001".to_string(),
                    probe_id: "probe-01".to_string(),
                }).ok();
            }
        }
    });

    handle.await?;

    Ok(())
}