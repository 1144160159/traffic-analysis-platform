use anyhow::{bail, Context, Result};
use sha2::{Digest, Sha256};
use std::error::Error;
use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::Semaphore;
use tracing::{debug, error, info, warn};

use s3::bucket::Bucket;
use s3::creds::Credentials;
use s3::region::Region;
use s3::serde_types::Part;

use tonic::transport::{Certificate, Channel, ClientTlsConfig, Identity};
use tonic::Request;

use proto_gen::{
    ingest_service_client::IngestServiceClient, PcapIndexMeta, UploadPcapIndexRequest,
};

use super::buffer::UploadData;
use super::upload_journal::{JournalEntry, UploadJournal};
use crate::config::ArchiverConfig;

const MULTIPART_THRESHOLD: usize = 100 * 1024 * 1024;
const CHUNK_SIZE: usize = 10 * 1024 * 1024;
const MAX_RETRIES: usize = 3;
const MAX_PART_RETRIES: usize = 2;
const INITIAL_BACKOFF_SECS: u64 = 2;
const MAX_BACKOFF_SECS: u64 = 60;
const MAX_ABORT_RETRIES: usize = 3;
const MAX_METADATA_RETRIES: usize = 3;
const ORPHAN_CLEANUP_THRESHOLD_HOURS: i64 = 24;
const MAX_S3_UPLOAD_ROUNDS: usize = 10;
const MAX_METADATA_UPLOAD_ROUNDS: usize = 5;

#[derive(Clone, Debug)]
pub struct UploaderConfig {
    pub s3_bucket: String,
    pub s3_region: String,
    pub s3_endpoint: String,
    pub s3_access_key: String,
    pub s3_secret_key: String,
    pub max_concurrent: usize,
    pub zstd_level: i32,
    pub gateway_addr: Option<String>,
    pub tls_ca_cert: Option<String>,
    pub tls_client_cert: Option<String>,
    pub tls_client_key: Option<String>,
    pub auth_token: Option<String>,
    pub cache_path: String,
}

impl Default for UploaderConfig {
    fn default() -> Self {
        Self {
            s3_bucket: "pcap-archive".to_string(),
            s3_region: "us-east-1".to_string(),
            s3_endpoint: "http://10.0.5.8:9002".to_string(),
            s3_access_key: std::env::var("PROBE_S3_ACCESS_KEY").unwrap_or_default(),
            s3_secret_key: std::env::var("PROBE_S3_SECRET_KEY").unwrap_or_default(),
            max_concurrent: 4,
            zstd_level: 3,
            gateway_addr: None,
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            cache_path: "/var/lib/probe-agent/cache".to_string(),
        }
    }
}

impl From<&ArchiverConfig> for UploaderConfig {
    fn from(config: &ArchiverConfig) -> Self {
        Self {
            s3_bucket: config.s3_bucket.clone(),
            s3_region: config.s3_region.clone(),
            s3_endpoint: config.s3_endpoint.clone(),
            s3_access_key: config.s3_access_key.clone(),
            s3_secret_key: config.s3_secret_key.clone(),
            max_concurrent: config.max_concurrent_uploads,
            zstd_level: config.zstd_level,
            gateway_addr: None,
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            cache_path: config.cache_path.clone(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct UploadResult {
    pub key: String,
    pub original_size: usize,
    pub compressed_size: usize,
    pub sha256: String,
    pub duration_ms: u64,
}

#[derive(Debug, Clone)]
pub struct UploadTask {
    pub data: Vec<u8>,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packet_count: u64,
    pub tenant_id: String,
    pub probe_id: String,
}

impl From<UploadData> for UploadTask {
    fn from(data: UploadData) -> Self {
        Self {
            data: data.data,
            ts_start: data.ts_start,
            ts_end: data.ts_end,
            packet_count: data.packet_count,
            tenant_id: "tenant01".to_string(),
            probe_id: "probe-tenant01-001".to_string(),
        }
    }
}

struct AbortGuard {
    bucket: Arc<Bucket>,
    key: String,
    upload_id: String,
    released: Arc<tokio::sync::Mutex<bool>>,
}

impl AbortGuard {
    fn new(bucket: Arc<Bucket>, key: String, upload_id: String) -> Self {
        debug!("AbortGuard created: key={}, upload_id={}", key, upload_id);

        Self {
            bucket,
            key,
            upload_id,
            released: Arc::new(tokio::sync::Mutex::new(false)),
        }
    }

    async fn release(&self) {
        let mut released = self.released.lock().await;
        *released = true;
        debug!("AbortGuard released: key={}", self.key);
    }

    async fn abort(&self) -> Result<()> {
        let mut released = self.released.lock().await;

        if *released {
            debug!("AbortGuard already released, skipping abort");
            return Ok(());
        }

        info!(
            "Aborting multipart upload: key={}, upload_id={}",
            self.key, self.upload_id
        );

        for attempt in 0..MAX_ABORT_RETRIES {
            match self.bucket.abort_upload(&self.key, &self.upload_id).await {
                Ok(_) => {
                    info!("✓ Aborted multipart upload: key={}", self.key);
                    *released = true;
                    return Ok(());
                }
                Err(e) => {
                    error!(
                        "Failed to abort upload (attempt {}/{}): key={}, error={}",
                        attempt + 1,
                        MAX_ABORT_RETRIES,
                        self.key,
                        e
                    );

                    if attempt < MAX_ABORT_RETRIES - 1 {
                        let backoff = tokio::time::Duration::from_secs(2_u64.pow(attempt as u32));
                        tokio::time::sleep(backoff).await;
                    }
                }
            }
        }

        error!(
            "🔴 CRITICAL: Failed to abort upload after {} attempts: key={}, upload_id={}. \
             Orphan parts will remain in storage! Manual cleanup required.",
            MAX_ABORT_RETRIES, self.key, self.upload_id
        );

        bail!(
            "Failed to abort multipart upload after {} attempts",
            MAX_ABORT_RETRIES
        )
    }
}

impl Drop for AbortGuard {
    fn drop(&mut self) {
        let bucket = self.bucket.clone();
        let key = self.key.clone();
        let upload_id = self.upload_id.clone();
        let released = self.released.clone();

        tokio::spawn(async move {
            let is_released = *released.lock().await;

            if is_released {
                debug!("AbortGuard drop: already released, skipping abort");
                return;
            }

            warn!(
                "AbortGuard dropped without release! Attempting emergency abort: key={}",
                key
            );

            for attempt in 0..MAX_ABORT_RETRIES {
                match bucket.abort_upload(&key, &upload_id).await {
                    Ok(_) => {
                        info!("✓ Emergency abort succeeded: key={}", key);
                        return;
                    }
                    Err(e) => {
                        error!(
                            "Emergency abort failed (attempt {}/{}): {}",
                            attempt + 1,
                            MAX_ABORT_RETRIES,
                            e
                        );

                        if attempt < MAX_ABORT_RETRIES - 1 {
                            tokio::time::sleep(tokio::time::Duration::from_secs(1)).await;
                        }
                    }
                }
            }

            error!(
                "🔴 CRITICAL: Emergency abort failed after {} attempts: key={}, upload_id={}",
                MAX_ABORT_RETRIES, key, upload_id
            );
        });
    }
}

#[derive(Debug, Clone)]
pub struct UploadStatistics {
    pub total_uploads: u64,
    pub successful_uploads: u64,
    pub failed_uploads: u64,
    pub pending_tasks: usize,
    pub s3_uploaded_not_synced: usize,
    pub total_bytes_uploaded: u64,
    pub average_compression_ratio: f64,
}

pub struct Uploader {
    bucket: Arc<Bucket>,
    semaphore: Arc<Semaphore>,
    config: UploaderConfig,
    grpc_client: Option<IngestServiceClient<Channel>>,
    journal: Arc<UploadJournal>,
}

impl Uploader {
    pub fn new(config: UploaderConfig) -> Result<Self> {
        let credentials = Credentials::new(
            Some(&config.s3_access_key),
            Some(&config.s3_secret_key),
            None,
            None,
            None,
        )?;

        let region = Region::Custom {
            region: config.s3_region.clone(),
            endpoint: config.s3_endpoint.clone(),
        };

        let mut bucket = Bucket::new(&config.s3_bucket, region, credentials)?;
        bucket.set_path_style();

        let journal_path = PathBuf::from(&config.cache_path).join("upload_journal");
        let journal = Arc::new(UploadJournal::new(&journal_path)?);

        info!(
            "Uploader created: bucket={}, endpoint={}, max_concurrent={}, journal={:?}",
            config.s3_bucket, config.s3_endpoint, config.max_concurrent, journal_path
        );

        Ok(Self {
            bucket: Arc::new(bucket),
            semaphore: Arc::new(Semaphore::new(config.max_concurrent)),
            config,
            grpc_client: None,
            journal,
        })
    }

    pub async fn preflight_check(&self) -> Result<()> {
        info!("Running S3 preflight check...");
        info!(
            "Testing S3 connection: endpoint={}",
            self.config.s3_endpoint
        );

        match self
            .bucket
            .list_page("".to_string(), None, None, None, Some(1))
            .await
        {
            Ok(_) => info!("✓ S3 endpoint is reachable"),
            Err(e) => {
                error!("S3 list_page failed: {:#?}", e);
                error!(
                    "s3_endpoint raw bytes: {:?}",
                    self.config.s3_endpoint.as_bytes()
                );
                if let Some(source) = e.source() {
                    error!("root cause: {:#?}", source);
                }
                bail!(
                    "S3 endpoint unreachable: {}. Please check s3_endpoint={} is accessible and credentials are correct.",
                    e,
                    self.config.s3_endpoint
                );
            }
        }

        match self.bucket.head_object("/").await {
            Ok(_) => info!(
                "✓ Bucket '{}' exists and is accessible",
                self.config.s3_bucket
            ),
            Err(e) => {
                let err_msg = e.to_string();
                if err_msg.contains("404") || err_msg.contains("NoSuchBucket") {
                    error!(
                        "Bucket '{}' does not exist. Please create it manually:\n\
                         \n\
                         Using MinIO Client (mc):\n\
                         1. Configure alias: mc alias set myminio {} {} {}\n\
                         2. Create bucket:   mc mb myminio/{}\n\
                         3. Verify bucket:   mc ls myminio/{}\n\
                         \n\
                         Or using AWS CLI:\n\
                         aws s3 mb s3://{} --endpoint-url={}",
                        self.config.s3_bucket,
                        self.config.s3_endpoint,
                        self.config.s3_access_key,
                        self.config.s3_secret_key,
                        self.config.s3_bucket,
                        self.config.s3_bucket,
                        self.config.s3_bucket,
                        self.config.s3_endpoint
                    );
                    bail!("Bucket '{}' does not exist", self.config.s3_bucket);
                } else {
                    bail!("Failed to check bucket: {}", e);
                }
            }
        }

        info!("✓ S3 preflight check passed");
        Ok(())
    }

    pub async fn connect_gateway(&mut self) -> Result<()> {
        let gateway_addr = match &self.config.gateway_addr {
            Some(addr) => addr.clone(),
            None => {
                warn!("Gateway address not configured, metadata upload disabled");
                return Ok(());
            }
        };

        info!("Connecting to Ingest Gateway: {}", gateway_addr);

        let mut endpoint = Channel::from_shared(gateway_addr.clone())?
            .connect_timeout(std::time::Duration::from_secs(10))
            .timeout(std::time::Duration::from_secs(30));

        if let (Some(ca_cert), Some(client_cert), Some(client_key)) = (
            &self.config.tls_ca_cert,
            &self.config.tls_client_cert,
            &self.config.tls_client_key,
        ) {
            debug!("Configuring TLS for metadata upload");

            let ca_pem = tokio::fs::read(ca_cert)
                .await
                .context(format!("Failed to read CA certificate: {}", ca_cert))?;
            let client_cert_pem = tokio::fs::read(client_cert).await.context(format!(
                "Failed to read client certificate: {}",
                client_cert
            ))?;
            let client_key_pem = tokio::fs::read(client_key)
                .await
                .context(format!("Failed to read client key: {}", client_key))?;

            let tls_config = ClientTlsConfig::new()
                .ca_certificate(Certificate::from_pem(ca_pem))
                .identity(Identity::from_pem(client_cert_pem, client_key_pem))
                .domain_name("ingest-gateway");

            endpoint = endpoint.tls_config(tls_config)?;
        }

        let channel = endpoint
            .connect()
            .await
            .context("Failed to connect to Ingest Gateway")?;

        let client = IngestServiceClient::new(channel)
            .max_encoding_message_size(64 * 1024 * 1024)
            .max_decoding_message_size(64 * 1024 * 1024);

        self.grpc_client = Some(client);

        info!("✓ Connected to Ingest Gateway successfully");

        Ok(())
    }

    pub async fn upload_with_journal(&self, task: UploadTask) -> Result<UploadResult> {
        let start = std::time::Instant::now();

        let _permit = self.semaphore.acquire().await?;

        let original_size = task.data.len();

        let local_path = format!(
            "{}/pending_{}_{}.pcap.zst",
            self.config.cache_path, task.ts_start, task.ts_end
        );

        let compressed = match zstd::encode_all(&task.data[..], self.config.zstd_level) {
            Ok(data) => data,
            Err(e) => {
                error!("Compression failed: {}", e);
                return self.handle_compression_failure(&task, &local_path).await;
            }
        };

        let compressed_size = compressed.len();

        debug!(
            "Compressed PCAP: {} -> {} bytes (ratio: {:.2}%)",
            original_size,
            compressed_size,
            (compressed_size as f64 / original_size as f64) * 100.0
        );

        tokio::fs::write(&local_path, &compressed).await?;

        let task_id = self.journal.record_pending(&task, &local_path)?;

        info!(
            "Recorded pending upload: task_id={}, local_path={}",
            task_id, local_path
        );

        let mut hasher = Sha256::new();
        hasher.update(&compressed);
        let sha256 = format!("{:x}", hasher.finalize());

        let key = Self::generate_key(&task);

        for round in 0..MAX_S3_UPLOAD_ROUNDS {
            match self.upload_to_s3_with_retry(&key, &compressed).await {
                Ok(()) => {
                    self.journal.mark_s3_uploaded(&task_id, &key)?;
                    info!("S3 upload completed: key={}", key);
                    break;
                }
                Err(e) => {
                    if round < MAX_S3_UPLOAD_ROUNDS - 1 {
                        warn!(
                            "S3 upload failed (round {}/{}), will retry after 30s: {:#?}",
                            round + 1,
                            MAX_S3_UPLOAD_ROUNDS,
                            e
                        );
                        self.journal
                            .update_retry(&task_id, &format!("S3 upload error: {}", e))?;
                        tokio::time::sleep(tokio::time::Duration::from_secs(30)).await;
                    } else {
                        error!(
                            "S3 upload failed permanently after {} rounds: {:#?}",
                            MAX_S3_UPLOAD_ROUNDS, e
                        );
                        self.journal.update_retry(
                            &task_id,
                            &format!("S3 upload permanent failure: {}", e),
                        )?;
                        bail!("S3 upload failed after {} rounds", MAX_S3_UPLOAD_ROUNDS);
                    }
                }
            }
        }

        let duration_ms = start.elapsed().as_millis() as u64;

        let upload_result = UploadResult {
            key: key.clone(),
            original_size,
            compressed_size,
            sha256,
            duration_ms,
        };

        for round in 0..MAX_METADATA_UPLOAD_ROUNDS {
            match self.upload_metadata(&key, &task, &upload_result).await {
                Ok(()) => {
                    self.journal.mark_metadata_synced(&task_id)?;
                    info!("Metadata synced: key={}", key);

                    break;
                }
                Err(e) => {
                    if round < MAX_METADATA_UPLOAD_ROUNDS - 1 {
                        warn!(
                            "Metadata sync failed (round {}/{}), will retry after 10s: {:#?}",
                            round + 1,
                            MAX_METADATA_UPLOAD_ROUNDS,
                            e
                        );
                        self.journal
                            .update_retry(&task_id, &format!("Metadata sync error: {}", e))?;
                        tokio::time::sleep(tokio::time::Duration::from_secs(10)).await;
                    } else {
                        warn!(
                            "Metadata sync failed permanently after {} rounds: {:#?}",
                            MAX_METADATA_UPLOAD_ROUNDS, e
                        );
                        self.journal.update_retry(
                            &task_id,
                            &format!("Metadata sync permanent failure: {}", e),
                        )?;
                        break;
                    }
                }
            }
        }

        info!(
            "Upload completed: key={}, size={}KB, duration={}ms",
            key,
            compressed_size / 1024,
            duration_ms
        );

        Ok(upload_result)
    }

    async fn handle_compression_failure(
        &self,
        task: &UploadTask,
        local_path: &str,
    ) -> Result<UploadResult> {
        warn!(
            "Falling back to uncompressed PCAP due to compression failure: {} bytes",
            task.data.len()
        );

        let raw_path = local_path.replace(".pcap.zst", ".pcap.raw");
        tokio::fs::write(&raw_path, &task.data).await?;

        let _task_id = uuid::Uuid::new_v4().to_string();
        self.journal.record_pending(task, &raw_path)?;

        error!(
            "CRITICAL: Uncompressed PCAP saved: path={}, size={} MB. Manual intervention required!",
            raw_path,
            task.data.len() / 1024 / 1024
        );

        bail!("Compression failed, saved raw PCAP to {}", raw_path)
    }

    pub async fn recover_pending_uploads(&self) -> Result<usize> {
        info!("Starting recovery of pending uploads...");

        let pending: Vec<(String, JournalEntry)> = self.journal.recover_pending();
        let needs_s3: Vec<(String, JournalEntry)> = self.journal.recover_needs_s3_upload();
        let needs_metadata: Vec<(String, JournalEntry)> =
            self.journal.recover_needs_metadata_sync();

        info!(
            "Found {} pending uploads: {} need S3 upload, {} need metadata sync",
            pending.len(),
            needs_s3.len(),
            needs_metadata.len()
        );

        let mut recovered = 0;

        for (task_id, entry) in needs_s3 {
            info!(
                "Recovering S3 upload: task_id={}, local_path={:?}",
                task_id, entry.local_path
            );

            if let Some(local_path) = &entry.local_path {
                match tokio::fs::read(local_path).await {
                    Ok(compressed) => {
                        let key = Self::generate_key_from_entry(&entry);

                        match self.upload_to_s3_with_retry(&key, &compressed).await {
                            Ok(()) => {
                                self.journal.mark_s3_uploaded(&task_id, &key)?;
                                info!("✓ Recovered S3 upload: key={}", key);
                                recovered += 1;
                            }
                            Err(e) => {
                                error!("Failed to recover S3 upload: {}", e);
                            }
                        }
                    }
                    Err(e) => {
                        error!("Failed to read local cache file: {}", e);
                    }
                }
            }
        }

        for (task_id, entry) in needs_metadata {
            info!("Recovering metadata sync: task_id={}", task_id);

            if let Some(s3_key) = &entry.s3_key {
                let task = UploadTask {
                    data: vec![],
                    ts_start: entry.ts_start,
                    ts_end: entry.ts_end,
                    packet_count: entry.packet_count,
                    tenant_id: entry.tenant_id.clone(),
                    probe_id: entry.probe_id.clone(),
                };

                let result = UploadResult {
                    key: s3_key.clone(),
                    original_size: 0,
                    compressed_size: 0,
                    sha256: String::new(),
                    duration_ms: 0,
                };

                match self.upload_metadata(s3_key, &task, &result).await {
                    Ok(()) => {
                        self.journal.mark_metadata_synced(&task_id)?;
                        info!("✓ Recovered metadata sync: key={}", s3_key);
                        recovered += 1;
                    }
                    Err(e) => {
                        error!("Failed to recover metadata sync: {}", e);
                    }
                }
            }
        }

        info!("Recovery completed: {} uploads recovered", recovered);

        Ok(recovered)
    }

    pub fn spawn_recovery_task(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(300));

            loop {
                interval.tick().await;

                match self.recover_pending_uploads().await {
                    Ok(count) => {
                        if count > 0 {
                            info!("Background recovery: {} uploads recovered", count);
                        }
                    }
                    Err(e) => {
                        error!("Background recovery failed: {}", e);
                    }
                }
            }
        })
    }

    async fn upload_metadata(
        &self,
        s3_key: &str,
        task: &UploadTask,
        result: &UploadResult,
    ) -> Result<()> {
        let client = match &self.grpc_client {
            Some(c) => c.clone(),
            None => {
                debug!("gRPC client not initialized, skipping metadata upload");
                return Ok(());
            }
        };

        let index_meta = PcapIndexMeta {
            tenant_id: task.tenant_id.clone(),
            probe_id: task.probe_id.clone(),
            file_key: s3_key.to_string(),
            ts_start: task.ts_start as i64,
            ts_end: task.ts_end as i64,
            byte_size: result.compressed_size as u64,
            zstd_level: self.config.zstd_level as u32,
            sha256: result.sha256.clone(),
            community_id: String::new(),
            flow_id: String::new(),
            offset_start: 0,
            offset_end: result.compressed_size as u64,
            bloom_filter_b64: String::new(),
            community_ids: vec![],
            created_ts: chrono::Utc::now().timestamp_millis(),
        };

        let mut last_error = None;

        for attempt in 0..MAX_METADATA_RETRIES {
            let mut request = Request::new(UploadPcapIndexRequest {
                index: Some(index_meta.clone()),
            });
            {
                let metadata = request.metadata_mut();
                metadata.insert(
                    "x-tenant-id",
                    tonic::metadata::MetadataValue::try_from(task.tenant_id.as_str())
                        .context("Invalid tenant_id metadata")?,
                );
                metadata.insert(
                    "x-probe-id",
                    tonic::metadata::MetadataValue::try_from(task.probe_id.as_str())
                        .context("Invalid probe_id metadata")?,
                );
                if let Some(token) = self.config.auth_token.as_deref() {
                    if !token.is_empty() {
                        metadata.insert(
                            "x-tenant-token",
                            tonic::metadata::MetadataValue::try_from(token)
                                .context("Invalid auth token metadata")?,
                        );
                    }
                }
            }

            let mut client_clone = client.clone();

            match client_clone.upload_pcap_index(request).await {
                Ok(response) => {
                    let resp = response.into_inner();
                    if resp.success {
                        info!("✓ PCAP metadata uploaded: key={}", s3_key);
                        return Ok(());
                    } else {
                        warn!(
                            "⚠ PCAP metadata upload rejected (attempt {}/{}): {}",
                            attempt + 1,
                            MAX_METADATA_RETRIES,
                            resp.message
                        );
                        last_error = Some(anyhow::anyhow!(
                            "Metadata upload rejected: {}",
                            resp.message
                        ));
                    }
                }
                Err(e) => {
                    warn!(
                        "Failed to upload PCAP metadata (attempt {}/{}): {}",
                        attempt + 1,
                        MAX_METADATA_RETRIES,
                        e
                    );
                    last_error = Some(e.into());
                }
            }

            if attempt < MAX_METADATA_RETRIES - 1 {
                let backoff = tokio::time::Duration::from_secs(2_u64.pow(attempt as u32));
                tokio::time::sleep(backoff).await;
            }
        }

        Err(last_error.unwrap_or_else(|| anyhow::anyhow!("Metadata upload failed")))
    }

    pub async fn upload(&self, task: UploadTask) -> Result<UploadResult> {
        self.upload_with_journal(task).await
    }

    async fn upload_to_s3_with_retry(&self, key: &str, data: &[u8]) -> Result<()> {
        let mut last_error = None;

        for attempt in 0..MAX_RETRIES {
            match self.do_upload_to_s3(key, data).await {
                Ok(()) => {
                    if attempt > 0 {
                        info!("S3 upload succeeded after {} retries", attempt);
                    }
                    return Ok(());
                }
                Err(e) => {
                    last_error = Some(e);

                    if attempt < MAX_RETRIES - 1 {
                        let backoff = self.calculate_backoff(attempt);
                        warn!(
                            "S3 upload attempt {}/{} failed, retrying in {:?}",
                            attempt + 1,
                            MAX_RETRIES,
                            backoff
                        );
                        tokio::time::sleep(backoff).await;
                    }
                }
            }
        }

        Err(last_error
            .map(|e| anyhow::anyhow!("{}", e))
            .unwrap_or_else(|| anyhow::anyhow!("unknown upload error")))
    }

    fn calculate_backoff(&self, attempt: usize) -> tokio::time::Duration {
        use rand::Rng;

        let base_backoff = INITIAL_BACKOFF_SECS * 2_u64.pow(attempt as u32);
        let capped_backoff = base_backoff.min(MAX_BACKOFF_SECS);
        let jitter_factor = rand::thread_rng().gen_range(0.75..=1.25);
        let backoff_secs = (capped_backoff as f64 * jitter_factor) as u64;

        tokio::time::Duration::from_secs(backoff_secs)
    }

    async fn do_upload_to_s3(&self, key: &str, data: &[u8]) -> Result<()> {
        if data.len() > MULTIPART_THRESHOLD {
            self.upload_multipart(key, data).await
        } else {
            self.bucket
                .put_object(key, data)
                .await
                .context("Failed to upload to S3")?;

            debug!("Direct upload completed: key={}, size={}", key, data.len());

            Ok(())
        }
    }

    async fn upload_multipart(&self, key: &str, data: &[u8]) -> Result<()> {
        let upload_response = self
            .bucket
            .initiate_multipart_upload(key, "application/octet-stream")
            .await?;
        let upload_id = upload_response.upload_id.clone();

        debug!(
            "Multipart upload initiated: key={}, upload_id=[{}...], total_size={}MB",
            key,
            &upload_id[..std::cmp::min(16, upload_id.len())],
            data.len() / 1024 / 1024
        );

        let guard = AbortGuard::new(self.bucket.clone(), key.to_string(), upload_id.clone());

        let mut parts: Vec<Part> = Vec::new();
        let total_chunks = (data.len() + CHUNK_SIZE - 1) / CHUNK_SIZE;

        for (i, chunk) in data.chunks(CHUNK_SIZE).enumerate() {
            let part_number = (i + 1) as u32;

            debug!(
                "Uploading part {}/{}: size={} bytes",
                part_number,
                total_chunks,
                chunk.len()
            );

            match self
                .upload_part_with_retry(chunk.to_vec(), key, part_number, &upload_id)
                .await
            {
                Ok(part) => {
                    debug!("Part {} uploaded successfully", part_number);
                    parts.push(complete_part_from_upload(part_number, part));
                }
                Err(e) => {
                    error!("Part {} upload failed after retries: {}", part_number, e);

                    if let Err(abort_err) = guard.abort().await {
                        error!("Failed to abort upload: {}", abort_err);
                    }

                    return Err(e.into());
                }
            }
        }

        self.bucket
            .complete_multipart_upload(key, &upload_id, parts)
            .await
            .context("Failed to complete multipart upload")?;

        guard.release().await;

        debug!(
            "Multipart upload completed: key={}, total_parts={}",
            key, total_chunks
        );

        Ok(())
    }

    async fn upload_part_with_retry(
        &self,
        chunk: Vec<u8>,
        key: &str,
        part_number: u32,
        upload_id: &str,
    ) -> Result<Part> {
        let max_retries = MAX_PART_RETRIES;
        let mut last_error = None;

        for attempt in 0..max_retries {
            match self
                .bucket
                .put_multipart_chunk(
                    chunk.clone(),
                    key,
                    part_number,
                    upload_id,
                    "application/octet-stream",
                )
                .await
            {
                Ok(part) => {
                    if attempt > 0 {
                        debug!("Part {} succeeded after {} retries", part_number, attempt);
                    }
                    return Ok(part);
                }
                Err(e) => {
                    last_error = Some(e);

                    if attempt < max_retries - 1 {
                        let backoff = tokio::time::Duration::from_secs(2_u64.pow(attempt as u32));
                        warn!(
                            "Part {} attempt {}/{} failed, retrying in {:?}",
                            part_number,
                            attempt + 1,
                            max_retries,
                            backoff
                        );
                        tokio::time::sleep(backoff).await;
                    }
                }
            }
        }

        Err(last_error
            .map(|e| anyhow::anyhow!("{}", e))
            .unwrap_or_else(|| anyhow::anyhow!("unknown upload error")))
    }

    pub async fn cleanup_orphan_uploads(&self) -> Result<usize> {
        info!("Starting orphan uploads cleanup...");

        let upload_results = self.bucket.list_multiparts_uploads(None, None).await?;

        let now = chrono::Utc::now();
        let mut cleaned = 0;
        let mut failed = 0;

        for result in upload_results {
            for upload in result.uploads {
                if let Ok(initiated) = chrono::DateTime::parse_from_rfc3339(&upload.initiated) {
                    let age_hours = now.signed_duration_since(initiated).num_hours();

                    if age_hours > ORPHAN_CLEANUP_THRESHOLD_HOURS {
                        debug!(
                            "Found orphan upload: key={}, age={}h",
                            upload.key, age_hours
                        );

                        match self.bucket.abort_upload(&upload.key, &upload.id).await {
                            Ok(_) => {
                                info!(
                                    "✓ Cleaned orphan upload: key={}, age={}h",
                                    upload.key, age_hours
                                );
                                cleaned += 1;
                            }
                            Err(e) => {
                                error!(
                                    "Failed to clean orphan upload: key={}, error={}",
                                    upload.key, e
                                );
                                failed += 1;
                            }
                        }
                    }
                }
            }
        }

        if cleaned > 0 || failed > 0 {
            info!(
                "Orphan uploads cleanup completed: cleaned={}, failed={}",
                cleaned, failed
            );
        } else {
            debug!("No orphan uploads found");
        }

        Ok(cleaned)
    }

    fn generate_key(task: &UploadTask) -> String {
        let ts_start_sec = task.ts_start / 1_000_000;
        let ts_end_sec = task.ts_end / 1_000_000;

        let start_dt = chrono::DateTime::from_timestamp(ts_start_sec as i64, 0)
            .unwrap_or_else(|| chrono::Utc::now());
        let end_dt = chrono::DateTime::from_timestamp(ts_end_sec as i64, 0)
            .unwrap_or_else(|| chrono::Utc::now());

        let date = start_dt.format("%Y-%m-%d").to_string();
        let time_start = start_dt.format("%H%M%S").to_string();
        let time_end = end_dt.format("%H%M%S").to_string();

        format!(
            "{}/{}/{}/{}-{}-{}.pcap.zst",
            task.tenant_id, task.probe_id, date, time_start, time_end, task.packet_count
        )
    }

    fn generate_key_from_entry(entry: &JournalEntry) -> String {
        let ts_start_sec = entry.ts_start / 1_000_000;
        let ts_end_sec = entry.ts_end / 1_000_000;

        let start_dt = chrono::DateTime::from_timestamp(ts_start_sec as i64, 0)
            .unwrap_or_else(|| chrono::Utc::now());
        let end_dt = chrono::DateTime::from_timestamp(ts_end_sec as i64, 0)
            .unwrap_or_else(|| chrono::Utc::now());

        let date = start_dt.format("%Y-%m-%d").to_string();
        let time_start = start_dt.format("%H%M%S").to_string();
        let time_end = end_dt.format("%H%M%S").to_string();

        format!(
            "{}/{}/{}/{}-{}-{}.pcap.zst",
            entry.tenant_id, entry.probe_id, date, time_start, time_end, entry.packet_count
        )
    }

    pub fn current_uploads(&self) -> usize {
        self.config.max_concurrent - self.semaphore.available_permits()
    }

    pub fn has_capacity(&self) -> bool {
        self.semaphore.available_permits() > 0
    }

    pub fn get_upload_statistics(&self) -> Result<UploadStatistics> {
        let pending: Vec<(String, JournalEntry)> = self.journal.recover_pending();
        let needs_s3: Vec<(String, JournalEntry)> = self.journal.recover_needs_s3_upload();

        Ok(UploadStatistics {
            total_uploads: 0,
            successful_uploads: 0,
            failed_uploads: 0,
            pending_tasks: pending.len(),
            s3_uploaded_not_synced: needs_s3.len(),
            total_bytes_uploaded: 0,
            average_compression_ratio: 0.0,
        })
    }

    pub fn list_pending_tasks(&self) -> Result<Vec<(String, JournalEntry)>> {
        Ok(self.journal.recover_pending())
    }
}

fn complete_part_from_upload(part_number: u32, part: Part) -> Part {
    Part {
        // MinIO returns quoted ETags. Preserve the exact value for
        // CompleteMultipartUpload. Also keep only the ETag field returned by
        // rust-s3; Display for Part renders XML and must never be reused here.
        etag: part.etag,
        part_number,
    }
}

#[cfg(test)]
mod tests {
    use super::complete_part_from_upload;
    use s3::serde_types::Part;

    #[test]
    fn complete_part_preserves_s3_etag_quotes_without_xml_wrapping() {
        let uploaded = Part {
            part_number: 99,
            etag: "\"abc123\"".to_string(),
        };

        let part = complete_part_from_upload(3, uploaded);

        assert_eq!(part.part_number, 3);
        assert_eq!(part.etag, "\"abc123\"");
        assert!(!part.etag.contains("<Part>"));
    }
}

impl Uploader {
    pub fn start_cleanup_task(
        uploader: Arc<Uploader>,
        interval: tokio::time::Duration,
    ) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut ticker = tokio::time::interval(interval);

            info!(
                "Orphan upload cleanup task started: interval={}s",
                interval.as_secs()
            );

            loop {
                ticker.tick().await;

                match uploader.cleanup_orphan_uploads().await {
                    Ok(cleaned) => {
                        if cleaned > 0 {
                            info!("Orphan cleanup cycle completed: cleaned={}", cleaned);
                        }
                    }
                    Err(e) => {
                        error!("Orphan cleanup cycle failed: {}", e);
                    }
                }
            }
        })
    }
}
