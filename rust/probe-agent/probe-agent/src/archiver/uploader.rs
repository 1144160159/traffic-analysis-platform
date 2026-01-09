use s3::bucket::Bucket;
use s3::creds::Credentials;
use s3::region::Region;
use crossbeam_channel::Receiver;
use tokio::task::JoinHandle;
use anyhow::{Result, Context};
use sha2::{Sha256, Digest};
use tracing::{info, warn, error};

#[derive(Clone)]
pub struct UploaderConfig {
    pub s3_bucket: String,
    pub s3_region: String,
    pub s3_endpoint: String,
    pub s3_access_key: String,
    pub s3_secret_key: String,
    pub max_concurrent: usize,   // 最大并发上传数
    pub queue_size: usize,       // 队列大小
    pub zstd_level: i32,         // 压缩级别 (1-22, 推荐 3)
}

pub struct UploadTask {
    pub data: Vec<u8>,
    pub timestamp: u64,
    pub tenant_id: String,
    pub probe_id: String,
    pub packet_count: u64,
}

pub struct Uploader {
    config: UploaderConfig,
    bucket: Bucket,
    rx: Receiver<UploadTask>,
}

impl Uploader {
    pub fn new(config: UploaderConfig, rx: Receiver<UploadTask>) -> Result<Self> {
        // 初始化 S3 客户端
        let region = Region::Custom {
            region: config.s3_region.clone(),
            endpoint: config.s3_endpoint.clone(),
        };
        
        let credentials = Credentials::new(
            Some(&config.s3_access_key),
            Some(&config.s3_secret_key),
            None,
            None,
            None,
        )?;
        
        let bucket = Bucket::new(&config.s3_bucket, region, credentials)?
            .with_path_style();
        
        Ok(Self {
            config,
            bucket,
            rx,
        })
    }

    /// 启动上传工作线程（应在 tokio::spawn 中运行）
    pub async fn run(&self) {
        let mut workers: Vec<JoinHandle<()>> = Vec::new();
        
        for worker_id in 0..self.config.max_concurrent {
            let rx = self.rx.clone();
            let bucket = self.bucket.clone();
            let zstd_level = self.config.zstd_level;
            
            let handle = tokio::spawn(async move {
                info!("Upload worker {} started", worker_id);
                
                while let Ok(task) = rx.recv() {
                    if let Err(e) = Self::process_task_static(task, &bucket, zstd_level).await {
                        error!("Worker {} upload failed: {}", worker_id, e);
                    }
                }
                
                info!("Upload worker {} stopped", worker_id);
            });
            
            workers.push(handle);
        }
        
        // 等待所有 worker 完成
        for handle in workers {
            handle.await.ok();
        }
    }

    /// 处理单个上传任务（静态方法，供 worker 调用）
    async fn process_task_static(
        task: UploadTask,
        bucket: &Bucket,
        zstd_level: i32,
    ) -> Result<()> {
        let start = std::time::Instant::now();
        
        // 1. 压缩数据
        let compressed = zstd::encode_all(&task.data[..], zstd_level)
            .context("Zstd compression failed")?;
        
        let compression_ratio = (compressed.len() as f64 / task.data.len() as f64) * 100.0;
        
        info!(
            "Compressed PCAP: {} -> {} bytes (ratio: {:.2}%, {} packets)",
            task.data.len(),
            compressed.len(),
            compression_ratio,
            task.packet_count
        );
        
        // 2. 计算 SHA256
        let mut hasher = Sha256::new();
        hasher.update(&compressed);
        let sha256 = format!("{:x}", hasher.finalize());
        
        // 3. 生成 S3 Key: tenant/date/hour/minute_bucket/probe_id/timestamp.pcap.zst
        let datetime = chrono::DateTime::from_timestamp(task.timestamp as i64 / 1000, 0)
            .unwrap();
        let key = format!(
            "{}/{}/{:02}/{:02}/{}/{}.pcap.zst",
            task.tenant_id,
            datetime.format("%Y-%m-%d"),
            datetime.hour(),
            datetime.minute() / 10 * 10,  // 按 10 分钟分桶
            task.probe_id,
            task.timestamp
        );
        
        // 4. 上传到 S3
        bucket.put_object(&key, &compressed).await
            .context("S3 upload failed")?;
        
        let elapsed = start.elapsed();
        
        info!(
            "✓ Uploaded: s3://{}/{} (sha256: {}...{}, time: {:?})",
            bucket.name(),
            key,
            &sha256[..8],
            &sha256[sha256.len()-8..],
            elapsed
        );
        
        // TODO: 发送 PCAP 索引元数据到 Kafka (S1.8)
        
        Ok(())
    }
}