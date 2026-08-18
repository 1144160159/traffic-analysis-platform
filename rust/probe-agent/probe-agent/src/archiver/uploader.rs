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
use super::spool::{DurablePcapSpool, JournaledUploadRef};
use super::upload_journal::{JournalEntry, JournalObjectState, UploadJournal};
use crate::config::ArchiverConfig;
use crate::metrics::{PCAP_METADATA_WITHOUT_OBJECT, PCAP_OBJECT_HASH_MISMATCHES};

const MULTIPART_THRESHOLD: usize = 100 * 1024 * 1024;
const CHUNK_SIZE: usize = 10 * 1024 * 1024;
const MAX_MULTIPART_PARTS: usize = 10_000;
const MAX_RETRIES: usize = 3;
const MAX_PART_RETRIES: usize = 2;
const INITIAL_BACKOFF_SECS: u64 = 2;
const MAX_BACKOFF_SECS: u64 = 60;
const MAX_ABORT_RETRIES: usize = 3;
const MAX_METADATA_RETRIES: usize = 3;
const ORPHAN_CLEANUP_THRESHOLD_HOURS: i64 = 24;
const MAX_RECOVERY_ENTRIES: usize = 1_000;

#[derive(Clone, Debug)]
pub struct UploaderConfig {
    pub s3_bucket: String,
    pub s3_region: String,
    pub s3_endpoint: String,
    pub s3_access_key: String,
    pub s3_secret_key: String,
    pub s3_ca_cert: Option<String>,
    pub max_concurrent: usize,
    pub zstd_level: i32,
    pub gateway_addr: Option<String>,
    pub tls_ca_cert: Option<String>,
    pub tls_client_cert: Option<String>,
    pub tls_client_key: Option<String>,
    pub auth_token: Option<String>,
    pub cache_path: String,
    /// Identity used to scope S3 maintenance operations (e.g. orphan upload
    /// cleanup) to this probe's own `{tenant}/{probe}` prefix.
    pub tenant_id: String,
    pub probe_id: String,
}

impl Default for UploaderConfig {
    fn default() -> Self {
        Self {
            s3_bucket: "pcap-archive".to_string(),
            s3_region: "us-east-1".to_string(),
            s3_endpoint: "http://10.0.5.8:9002".to_string(),
            s3_access_key: std::env::var("PROBE_S3_ACCESS_KEY").unwrap_or_default(),
            s3_secret_key: std::env::var("PROBE_S3_SECRET_KEY").unwrap_or_default(),
            s3_ca_cert: None,
            max_concurrent: 4,
            zstd_level: 3,
            gateway_addr: None,
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            cache_path: "/var/lib/probe-agent/cache".to_string(),
            tenant_id: String::new(),
            probe_id: String::new(),
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
            s3_ca_cert: config.s3_ca_cert.clone(),
            max_concurrent: config.max_concurrent_uploads,
            zstd_level: config.zstd_level,
            gateway_addr: None,
            tls_ca_cert: None,
            tls_client_cert: None,
            tls_client_key: None,
            auth_token: None,
            cache_path: config.cache_path.clone(),
            tenant_id: String::new(),
            probe_id: String::new(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub struct ObjectWriteReceipt {
    pub bucket: String,
    pub key: String,
    pub version_id: String,
    pub etag: String,
    pub stored_size: u64,
    pub sha256: String,
}

#[derive(Debug, Clone)]
pub struct UploadResult {
    pub key: String,
    pub bucket: String,
    pub object_version: String,
    pub etag: String,
    pub original_size: usize,
    pub compressed_size: usize,
    pub sha256: String,
    pub duration_ms: u64,
}

impl ObjectWriteReceipt {
    fn into_upload_result(self, original_size: usize, duration_ms: u64) -> UploadResult {
        UploadResult {
            key: self.key,
            bucket: self.bucket,
            object_version: self.version_id,
            etag: self.etag,
            original_size,
            compressed_size: self.stored_size as usize,
            sha256: self.sha256,
            duration_ms,
        }
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RecoverySummary {
    pub scanned: usize,
    pub actionable: usize,
    pub deferred: usize,
    pub objects_written: usize,
    pub metadata_accepted: usize,
    pub quarantined: usize,
    pub retryable_failures: usize,
}

impl RecoverySummary {
    pub fn recovered(&self) -> usize {
        self.objects_written + self.metadata_accepted
    }

    pub fn admission_safe(&self) -> bool {
        self.quarantined == 0 && self.retryable_failures == 0 && self.deferred == 0
    }
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
    durable_spool: Arc<DurablePcapSpool>,
}

impl Uploader {
    pub fn new(config: UploaderConfig) -> Result<Self> {
        configure_s3_transport(&config)?;
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
        let durable_spool = Arc::new(DurablePcapSpool::new(
            PathBuf::from(&config.cache_path).join("pcap_spool"),
            journal.clone(),
            config.zstd_level,
        )?);

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
            durable_spool,
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
                .domain_name(&crate::config::gateway_sni(&gateway_addr));

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

    pub fn journal(&self) -> Arc<UploadJournal> {
        self.journal.clone()
    }

    pub fn durable_spool(&self) -> Arc<DurablePcapSpool> {
        self.durable_spool.clone()
    }

    pub async fn upload_journaled(&self, upload: JournaledUploadRef) -> Result<UploadResult> {
        let _permit = self.semaphore.acquire().await?;
        upload.validate_identity()?;
        let mut entry = self
            .journal
            .get_entry(&upload.task_id)?
            .with_context(|| format!("journal task not found: {}", upload.task_id))?;
        validate_upload_ref_matches_entry(&upload, &entry)?;

        let receipt = match entry.effective_object_state() {
            JournalObjectState::Pending => {
                let compressed = self.read_verified_spool(&entry).await.map_err(|error| {
                    let reason = format!("spool manifest verification failed: {error}");
                    if let Err(quarantine_error) = self.journal.quarantine(&upload.task_id, &reason)
                    {
                        return anyhow::anyhow!(
                            "{reason}; additionally failed to quarantine: {quarantine_error}"
                        );
                    }
                    anyhow::anyhow!(reason)
                })?;
                let receipt = match self
                    .upload_to_s3_with_retry_receipt(&upload.object_key, &compressed)
                    .await
                {
                    Ok(receipt) => receipt,
                    Err(error) => {
                        // Preserve the original upload error even if the
                        // journal bookkeeping also fails.
                        if let Err(journal_error) = self.journal.mark_retry_wait(
                            &upload.task_id,
                            &format!("object upload failed: {error}"),
                        ) {
                            return Err(anyhow::anyhow!(
                                "object upload failed: {error}; additionally failed to mark retry-wait: {journal_error}"
                            ));
                        }
                        return Err(error);
                    }
                };
                self.journal
                    .mark_object_written(&upload.task_id, &receipt)?;
                receipt
            }
            JournalObjectState::ObjectWritten => entry
                .object_write_receipt
                .clone()
                .context("OBJECT_WRITTEN task has no durable object receipt")?,
            JournalObjectState::MetadataAccepted => {
                return entry
                    .object_write_receipt
                    .clone()
                    .context("METADATA_ACCEPTED task has no durable object receipt")
                    .map(|receipt| receipt.into_upload_result(entry.original_size, 0));
            }
            state => bail!(
                "journal task {} is not uploadable from state {state:?}",
                upload.task_id
            ),
        };
        entry = self
            .journal
            .get_entry(&upload.task_id)?
            .context("journal task disappeared after OBJECT_WRITTEN flush")?;
        if entry.object_write_receipt.as_ref() != Some(&receipt)
            || entry.effective_object_state() != JournalObjectState::ObjectWritten
        {
            bail!("journal did not retain the exact OBJECT_WRITTEN receipt");
        }

        let result = receipt.clone().into_upload_result(entry.original_size, 0);
        let task = UploadTask {
            data: Vec::new(),
            ts_start: entry.ts_start,
            ts_end: entry.ts_end,
            packet_count: entry.packet_count,
            tenant_id: entry.tenant_id,
            probe_id: entry.probe_id,
        };
        if let Err(error) = self.upload_metadata(&receipt.key, &task, &result).await {
            self.journal
                .mark_retry_wait(&upload.task_id, &format!("metadata upload failed: {error}"))?;
            return Err(error);
        }
        self.journal.mark_metadata_accepted(&upload.task_id)?;
        Ok(result)
    }

    pub async fn upload_with_journal(&self, task: UploadTask) -> Result<UploadResult> {
        let upload = UploadData {
            data: task.data,
            ts_start: task.ts_start,
            ts_end: task.ts_end,
            packet_count: task.packet_count,
        };
        let upload_ref = self
            .durable_spool
            .persist_rotated(upload, &task.tenant_id, &task.probe_id)
            .await
            .context("legacy UploadTask could not cross the durable spool barrier")?;
        self.upload_journaled(upload_ref).await
    }

    pub async fn recover_pending_uploads(&self) -> Result<RecoverySummary> {
        info!("Starting recovery of pending uploads...");

        let inventory = self.journal.recover_by_state(usize::MAX)?;
        let actionable = inventory
            .iter()
            .filter(|(_, entry)| {
                matches!(
                    entry.effective_object_state(),
                    JournalObjectState::Pending
                        | JournalObjectState::ObjectWritten
                        | JournalObjectState::RetryWait
                        | JournalObjectState::Quarantined
                )
            })
            .count();
        let entries: Vec<_> = inventory
            .into_iter()
            .filter(|(_, entry)| {
                matches!(
                    entry.effective_object_state(),
                    JournalObjectState::Pending
                        | JournalObjectState::ObjectWritten
                        | JournalObjectState::RetryWait
                        | JournalObjectState::Quarantined
                )
            })
            .take(MAX_RECOVERY_ENTRIES)
            .collect();
        let mut summary = RecoverySummary {
            scanned: entries.len(),
            actionable,
            deferred: actionable.saturating_sub(entries.len()),
            ..RecoverySummary::default()
        };
        for (task_id, mut entry) in entries {
            if entry.effective_object_state() == JournalObjectState::RetryWait {
                match self.journal.resume_retry(&task_id) {
                    Ok(_) => {
                        entry = self
                            .journal
                            .get_entry(&task_id)?
                            .context("resumed retry task disappeared from journal")?;
                    }
                    Err(error) => {
                        let reason = format!("invalid retry state: {error}");
                        self.journal.quarantine(&task_id, &reason)?;
                        summary.quarantined += 1;
                        continue;
                    }
                }
            }
            match entry.effective_object_state() {
                JournalObjectState::Pending => {
                    let compressed = match self.read_verified_spool(&entry).await {
                        Ok(bytes) => bytes,
                        Err(error) => {
                            let reason = format!("spool manifest verification failed: {error}");
                            self.journal.quarantine(&task_id, &reason)?;
                            summary.quarantined += 1;
                            continue;
                        }
                    };
                    let key = if entry.expected_object_key.is_empty() {
                        Self::generate_key_from_entry(&entry)
                    } else {
                        entry.expected_object_key.clone()
                    };
                    match self
                        .upload_to_s3_with_retry_receipt(&key, &compressed)
                        .await
                    {
                        Ok(receipt) => {
                            self.journal.mark_object_written(&task_id, &receipt)?;
                            entry.object_write_receipt = Some(receipt);
                            entry.s3_key = Some(key);
                            entry.object_state = JournalObjectState::ObjectWritten;
                            entry.journal_version = 2;
                            summary.objects_written += 1;
                        }
                        Err(error) => {
                            self.journal.mark_retry_wait(
                                &task_id,
                                &format!("object recovery failed: {error}"),
                            )?;
                            summary.retryable_failures += 1;
                            continue;
                        }
                    }
                }
                JournalObjectState::ObjectWritten => {
                    if entry.object_write_receipt.is_none() {
                        let Some(key) = entry.s3_key.as_deref() else {
                            PCAP_METADATA_WITHOUT_OBJECT.inc();
                            let reason = "OBJECT_WRITTEN journal task has neither receipt nor key";
                            self.journal.quarantine(&task_id, reason)?;
                            summary.quarantined += 1;
                            continue;
                        };
                        match self
                            .read_trusted_object_receipt(key, entry.compressed_size, &entry.sha256)
                            .await
                        {
                            Ok(Some(receipt)) => {
                                self.journal.mark_object_written(&task_id, &receipt)?;
                                entry.object_write_receipt = Some(receipt);
                                entry.journal_version = 2;
                            }
                            Ok(None) => {
                                PCAP_METADATA_WITHOUT_OBJECT.inc();
                                let reason =
                                    "legacy OBJECT_WRITTEN key is absent in object storage";
                                self.journal.quarantine(&task_id, reason)?;
                                summary.quarantined += 1;
                                continue;
                            }
                            Err(error) => {
                                self.journal.mark_retry_wait(
                                    &task_id,
                                    &format!("legacy receipt recovery failed: {error}"),
                                )?;
                                summary.retryable_failures += 1;
                                continue;
                            }
                        }
                    }
                }
                JournalObjectState::MetadataAccepted
                | JournalObjectState::CleanupAuthorized
                | JournalObjectState::Deleted => continue,
                JournalObjectState::Quarantined => {
                    summary.quarantined += 1;
                    continue;
                }
                JournalObjectState::RetryWait => {
                    // The retry state is normalized at the top of the loop;
                    // if it is still present, treat it as retryable instead of
                    // crashing the whole recovery pass.
                    warn!(
                        "Recovery encountered an un-normalized RetryWait task {}; deferring",
                        task_id
                    );
                    summary.retryable_failures += 1;
                    continue;
                }
            }

            // From here the record is OBJECT_WRITTEN with a durable receipt.
            // Recovery reuses it and never calls an object PUT again.
            let receipt = entry
                .object_write_receipt
                .clone()
                .context("OBJECT_WRITTEN recovery lost its durable receipt")?;
            let task = UploadTask {
                data: Vec::new(),
                ts_start: entry.ts_start,
                ts_end: entry.ts_end,
                packet_count: entry.packet_count,
                tenant_id: entry.tenant_id,
                probe_id: entry.probe_id,
            };
            let result = receipt.clone().into_upload_result(entry.original_size, 0);
            match self.upload_metadata(&receipt.key, &task, &result).await {
                Ok(()) => {
                    self.journal.mark_metadata_accepted(&task_id)?;
                    summary.metadata_accepted += 1;
                }
                Err(error) => {
                    self.journal
                        .mark_retry_wait(&task_id, &format!("metadata recovery failed: {error}"))?;
                    summary.retryable_failures += 1;
                }
            }
        }

        info!("Recovery completed: {:?}", summary);
        Ok(summary)
    }

    async fn read_verified_spool(&self, entry: &JournalEntry) -> Result<Vec<u8>> {
        if !entry.has_complete_manifest() {
            bail!("journal entry has no complete size/hash manifest");
        }
        let path = entry
            .local_path
            .as_deref()
            .context("journal entry has no local spool path")?;
        let expected_root = if entry.journal_version == 0 {
            PathBuf::from(&self.config.cache_path)
        } else {
            self.durable_spool.root().to_path_buf()
        };
        let canonical_root = tokio::fs::canonicalize(&expected_root)
            .await
            .with_context(|| {
                format!(
                    "failed to canonicalize spool root {}",
                    expected_root.display()
                )
            })?;
        let canonical_path = tokio::fs::canonicalize(path)
            .await
            .with_context(|| format!("failed to canonicalize spool file {path}"))?;
        if !canonical_path.starts_with(&canonical_root) {
            bail!(
                "spool path {} escapes canonical root {}",
                canonical_path.display(),
                canonical_root.display()
            );
        }
        let metadata = tokio::fs::symlink_metadata(path)
            .await
            .with_context(|| format!("failed to stat spool file {path}"))?;
        if metadata.file_type().is_symlink() || !metadata.is_file() {
            bail!("spool path {path} is not a regular owned file");
        }
        if metadata.len() != entry.compressed_size as u64 {
            bail!(
                "spool size mismatch: expected {}, got {}",
                entry.compressed_size,
                metadata.len()
            );
        }
        let bytes = tokio::fs::read(&canonical_path)
            .await
            .with_context(|| format!("failed to read spool file {}", canonical_path.display()))?;
        let observed_sha256 = sha256_hex(&bytes);
        if observed_sha256 != entry.sha256 {
            bail!("spool sha256 mismatch");
        }
        Ok(bytes)
    }

    pub fn spawn_recovery_task(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(300));

            loop {
                interval.tick().await;

                match self.recover_pending_uploads().await {
                    Ok(summary) => {
                        if summary.recovered() > 0 {
                            info!("Background recovery completed: {:?}", summary);
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
                bail!("gRPC metadata client is not initialized");
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
            created_ts: task.ts_end as i64,
            bucket: result.bucket.clone(),
            object_version: result.object_version.clone(),
            etag: result.etag.clone(),
            original_size: result.original_size as u64,
            stored_size: result.compressed_size as u64,
            compression: "zstd".to_string(),
            manifest_version: 2,
            // 契约补齐:上报归档窗口内的真实包数,消费端(pcap-index job)据此填充
            // ClickHouse packet_count 列;旧字段零值保持 wire 兼容。
            packet_count: task.packet_count,
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

    async fn load_object_receipt(
        &self,
        key: &str,
        original_size: usize,
        compressed_size: usize,
        sha256: String,
        duration_ms: u64,
    ) -> Result<UploadResult> {
        self.read_trusted_object_receipt(key, compressed_size, &sha256)
            .await?
            .with_context(|| format!("object receipt for {key} was not found"))
            .map(|receipt| receipt.into_upload_result(original_size, duration_ms))
    }

    async fn upload_to_s3_with_retry(&self, key: &str, data: &[u8]) -> Result<()> {
        self.upload_to_s3_with_retry_receipt(key, data)
            .await
            .map(|_| ())
    }

    async fn upload_to_s3_with_retry_receipt(
        &self,
        key: &str,
        data: &[u8],
    ) -> Result<ObjectWriteReceipt> {
        let mut last_error = None;

        for attempt in 0..MAX_RETRIES {
            match if data.len() > MULTIPART_THRESHOLD {
                self.upload_multipart_with_receipt(key, data).await
            } else {
                self.do_upload_to_s3_with_receipt(key, data).await
            } {
                Ok(receipt) => {
                    if attempt > 0 {
                        info!("S3 upload succeeded after {} retries", attempt);
                    }
                    return Ok(receipt);
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
        self.do_upload_to_s3_with_receipt(key, data)
            .await
            .map(|_| ())
    }

    async fn do_upload_to_s3_with_receipt(
        &self,
        key: &str,
        data: &[u8],
    ) -> Result<ObjectWriteReceipt> {
        let sha256 = sha256_hex(data);
        if let Some(existing) = self
            .read_trusted_object_receipt(key, data.len(), &sha256)
            .await?
        {
            return Ok(existing);
        }

        let mut write_bucket = (*self.bucket).clone();
        write_bucket.add_header("x-amz-meta-sha256", &sha256);
        write_bucket.add_header("if-none-match", "*");
        let response = write_bucket
            .put_object(key, data)
            .await
            .context("Failed to upload to S3")?;
        if !(200..300).contains(&response.status_code()) {
            bail!(
                "direct object write for {key} returned HTTP status {}",
                response.status_code()
            );
        }

        debug!("Direct upload completed: key={}, size={}", key, data.len());
        self.read_trusted_object_receipt(key, data.len(), &sha256)
            .await?
            .with_context(|| format!("direct object write for {key} has no trusted HEAD receipt"))
    }

    async fn read_trusted_object_receipt(
        &self,
        key: &str,
        expected_size: usize,
        expected_sha256: &str,
    ) -> Result<Option<ObjectWriteReceipt>> {
        let (head, status) = match self.bucket.head_object(key).await {
            Ok(result) => result,
            Err(s3::error::S3Error::Http(404, _)) => return Ok(None),
            Err(error) => {
                return Err(error)
                    .with_context(|| format!("failed to read object receipt for {key}"))
            }
        };
        if status == 404 {
            return Ok(None);
        }
        if !(200..300).contains(&status) {
            bail!("object receipt for {key} returned HTTP status {status}");
        }
        if head.content_length != Some(expected_size as i64) {
            bail!(
                "object identity conflict for {key}: expected size {expected_size}, got {:?}",
                head.content_length
            );
        }
        let observed_sha256 = head
            .metadata
            .as_ref()
            .and_then(|metadata| metadata.get("sha256"))
            .filter(|value| !value.trim().is_empty())
            .with_context(|| {
                format!("object receipt for {key} has no trusted x-amz-meta-sha256 checksum proof")
            })?;
        if observed_sha256 != expected_sha256 {
            PCAP_OBJECT_HASH_MISMATCHES.inc();
            bail!("object identity conflict for {key}: trusted checksum does not match manifest");
        }
        let etag = head
            .e_tag
            .filter(|value| !value.trim().is_empty())
            .with_context(|| format!("object receipt for {key} has no ETag"))?;
        Ok(Some(ObjectWriteReceipt {
            bucket: self.config.s3_bucket.clone(),
            key: key.to_string(),
            version_id: head.version_id.unwrap_or_default(),
            etag,
            stored_size: expected_size as u64,
            sha256: expected_sha256.to_string(),
        }))
    }

    async fn upload_multipart(&self, key: &str, data: &[u8]) -> Result<()> {
        self.upload_multipart_with_receipt(key, data)
            .await
            .map(|_| ())
    }

    async fn upload_multipart_with_receipt(
        &self,
        key: &str,
        data: &[u8],
    ) -> Result<ObjectWriteReceipt> {
        let sha256 = sha256_hex(data);
        if let Some(existing) = self
            .read_trusted_object_receipt(key, data.len(), &sha256)
            .await?
        {
            return Ok(existing);
        }

        let mut bucket = (*self.bucket).clone();
        bucket.add_header("x-amz-meta-sha256", &sha256);
        let bucket = Arc::new(bucket);
        let upload_response = bucket
            .initiate_multipart_upload(key, "application/octet-stream")
            .await?;
        let upload_id = upload_response.upload_id.clone();

        debug!(
            "Multipart upload initiated: key={}, upload_id=[{}...], total_size={}MB",
            key,
            &upload_id[..std::cmp::min(16, upload_id.len())],
            data.len() / 1024 / 1024
        );

        let guard = AbortGuard::new(bucket.clone(), key.to_string(), upload_id.clone());

        let mut parts: Vec<Part> = Vec::new();
        let total_chunks = (data.len() + CHUNK_SIZE - 1) / CHUNK_SIZE;

        // AWS S3 and MinIO reject multipart uploads with more than 10,000
        // parts; reject such objects up front instead of failing at the
        // 10,001st part after uploading everything before it.
        if total_chunks > MAX_MULTIPART_PARTS {
            bail!(
                "object {key} requires {total_chunks} parts, exceeding the S3/MinIO limit of {MAX_MULTIPART_PARTS}"
            );
        }

        for (i, chunk) in data.chunks(CHUNK_SIZE).enumerate() {
            let part_number = (i + 1) as u32;

            debug!(
                "Uploading part {}/{}: size={} bytes",
                part_number,
                total_chunks,
                chunk.len()
            );

            match self
                .upload_part_with_retry(
                    bucket.clone(),
                    chunk.to_vec(),
                    key,
                    part_number,
                    &upload_id,
                )
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

        let response = bucket
            .complete_multipart_upload(key, &upload_id, parts)
            .await
            .context("Failed to complete multipart upload")?;
        if !(200..300).contains(&response.status_code()) {
            guard.abort().await?;
            bail!(
                "multipart completion for {key} returned HTTP status {}",
                response.status_code()
            );
        }

        guard.release().await;

        debug!(
            "Multipart upload completed: key={}, total_parts={}",
            key, total_chunks
        );

        self.read_trusted_object_receipt(key, data.len(), &sha256)
            .await?
            .with_context(|| {
                format!("multipart object write for {key} has no trusted HEAD receipt")
            })
    }

    async fn upload_part_with_retry(
        &self,
        bucket: Arc<Bucket>,
        chunk: Vec<u8>,
        key: &str,
        part_number: u32,
        upload_id: &str,
    ) -> Result<Part> {
        let max_retries = MAX_PART_RETRIES;
        let mut last_error = None;

        for attempt in 0..max_retries {
            match bucket
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

        // Scope cleanup to this probe's own `{tenant}/{probe}` prefix so a
        // shared bucket is never touched for other tenants'/probes' legal
        // long-running multipart uploads.
        let identity_prefix = if self.config.tenant_id.is_empty() {
            None
        } else {
            Some(format!("{}/{}", self.config.tenant_id, self.config.probe_id))
        };

        let upload_results = self.bucket.list_multiparts_uploads(None, None).await?;

        let now = chrono::Utc::now();
        let mut cleaned = 0;
        let mut failed = 0;

        for result in upload_results {
            for upload in result.uploads {
                if let Some(prefix) = identity_prefix.as_deref() {
                    if !upload.key.starts_with(prefix) {
                        continue;
                    }
                }
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
        let tenant_id = &entry.tenant_id;
        let probe_id = &entry.probe_id;
        // Prefer the content-derived capture identity when available.
        if !entry.capture_uuid.is_empty() {
            return format!("{tenant_id}/{probe_id}/{}.pcap.zst", entry.capture_uuid);
        }
        // Legacy fallback: derive a deterministic key from the durable content
        // hash instead of the old date+second+packet_count scheme, which
        // collided for two objects in the same second with the same packet
        // count (the second upload could overwrite the first object).
        let identity = format!(
            "{tenant_id}\0{probe_id}\0{}\0{}\0{}\0{}",
            entry.ts_start, entry.ts_end, entry.packet_count, entry.sha256
        );
        let digest = sha256_hex(identity.as_bytes());
        format!(
            "{tenant_id}/{probe_id}/{}-{}-{}-{}.pcap.zst",
            entry.ts_start / 1_000_000,
            entry.ts_end / 1_000_000,
            entry.packet_count,
            &digest[..16]
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
        let needs_metadata: Vec<(String, JournalEntry)> =
            self.journal.recover_needs_metadata_sync();

        Ok(UploadStatistics {
            total_uploads: 0,
            successful_uploads: 0,
            failed_uploads: 0,
            pending_tasks: pending.len(),
            s3_uploaded_not_synced: needs_metadata.len(),
            total_bytes_uploaded: 0,
            average_compression_ratio: 0.0,
        })
    }

    pub fn list_pending_tasks(&self) -> Result<Vec<(String, JournalEntry)>> {
        Ok(self.journal.recover_pending())
    }
}

fn configure_s3_transport(config: &UploaderConfig) -> Result<()> {
    let endpoint = config.s3_endpoint.trim();
    let ca_file = config
        .s3_ca_cert
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty());

    if endpoint.starts_with("https://") {
        let ca_file = ca_file.context("HTTPS S3 endpoint requires archiver.s3_ca_cert")?;
        let pem = std::fs::read_to_string(ca_file)
            .with_context(|| format!("failed to read S3 CA certificate {ca_file}"))?;
        if !pem.contains("-----BEGIN CERTIFICATE-----")
            || !pem.contains("-----END CERTIFICATE-----")
        {
            bail!("S3 CA certificate does not contain a PEM certificate");
        }
        if let Ok(existing) = std::env::var("SSL_CERT_FILE") {
            if !existing.trim().is_empty() && existing != ca_file {
                bail!(
                    "SSL_CERT_FILE is already set to a different path; refusing to replace process trust roots"
                );
            }
        }
        // rust-s3 0.33 constructs its reqwest/native-tls client internally and
        // exposes no custom client builder. OpenSSL reads SSL_CERT_FILE when
        // that client is built; set it once during single-threaded startup.
        std::env::set_var("SSL_CERT_FILE", ca_file);
        return Ok(());
    }

    if ca_file.is_some() {
        bail!("archiver.s3_ca_cert cannot be configured for a non-HTTPS S3 endpoint");
    }
    Ok(())
}

fn sha256_hex(data: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(data);
    format!("{:x}", hasher.finalize())
}

fn validate_upload_ref_matches_entry(
    upload: &JournaledUploadRef,
    entry: &JournalEntry,
) -> Result<()> {
    let expected_path = entry
        .local_path
        .as_deref()
        .context("journal task has no local spool path")?;
    if entry.task_id != upload.task_id
        || entry.tenant_id != upload.tenant_id
        || entry.probe_id != upload.probe_id
        || entry.capture_uuid != upload.capture_uuid
        || entry.manifest_hash != upload.manifest_hash
        || entry.expected_object_key != upload.object_key
        || expected_path != upload.local_path.to_string_lossy()
        || entry.original_size as u64 != upload.original_size
        || entry.compressed_size as u64 != upload.stored_size
        || entry.sha256 != upload.sha256
        || entry.ts_start != upload.ts_start
        || entry.ts_end != upload.ts_end
        || entry.packet_count != upload.packet_count
    {
        bail!("JournaledUploadRef conflicts with the persisted journal manifest");
    }
    Ok(())
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
    use super::{
        complete_part_from_upload, configure_s3_transport, sha256_hex, JournalObjectState,
        UploadTask, Uploader, UploaderConfig,
    };
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

    #[test]
    fn https_s3_requires_private_ca() {
        let config = UploaderConfig {
            s3_endpoint: "https://minio.minio.svc:9000".to_string(),
            s3_ca_cert: None,
            ..UploaderConfig::default()
        };
        let error = configure_s3_transport(&config).expect_err("HTTPS without CA must fail");
        assert!(error.to_string().contains("requires archiver.s3_ca_cert"));
    }

    #[test]
    fn plaintext_s3_rejects_ca_configuration() {
        let config = UploaderConfig {
            s3_endpoint: "http://minio.minio.svc:9000".to_string(),
            s3_ca_cert: Some("/etc/minio/ca.crt".to_string()),
            ..UploaderConfig::default()
        };
        let error = configure_s3_transport(&config).expect_err("plaintext plus CA must fail");
        assert!(error.to_string().contains("non-HTTPS"));
    }

    #[test]
    fn startup_admission_rejects_quarantine_retry_failures_and_deferred_work() {
        let clean = super::RecoverySummary {
            scanned: 2,
            actionable: 2,
            objects_written: 1,
            metadata_accepted: 1,
            ..super::RecoverySummary::default()
        };
        assert!(clean.admission_safe());
        for unsafe_summary in [
            super::RecoverySummary {
                quarantined: 1,
                ..clean.clone()
            },
            super::RecoverySummary {
                retryable_failures: 1,
                ..clean.clone()
            },
            super::RecoverySummary {
                deferred: 1,
                ..clean
            },
        ] {
            assert!(!unsafe_summary.admission_safe());
        }
    }

    #[tokio::test]
    async fn real_minio_returns_trusted_receipt_and_rejects_identity_conflict() {
        let Ok(endpoint) = std::env::var("M02_MINIO_ENDPOINT") else {
            return;
        };
        let cache = tempfile::tempdir().expect("cache");
        let config = UploaderConfig {
            s3_bucket: std::env::var("M02_MINIO_BUCKET")
                .unwrap_or_else(|_| "pcap-archive".to_string()),
            s3_endpoint: endpoint,
            s3_access_key: std::env::var("M02_MINIO_ACCESS_KEY").expect("M02_MINIO_ACCESS_KEY"),
            s3_secret_key: std::env::var("M02_MINIO_SECRET_KEY").expect("M02_MINIO_SECRET_KEY"),
            cache_path: cache.path().to_string_lossy().into_owned(),
            ..UploaderConfig::default()
        };
        let uploader = Uploader::new(config).expect("uploader");
        let key = "tenant-a/probe-a/receipt-integration.pcap.zst";
        let bytes = b"immutable compressed pcap";

        let first = uploader
            .do_upload_to_s3_with_receipt(key, bytes)
            .await
            .expect("first conditional upload");
        assert_eq!(first.key, key);
        assert_eq!(first.stored_size, bytes.len() as u64);
        assert_eq!(first.sha256, sha256_hex(bytes));
        assert!(!first.etag.is_empty());

        let replay = uploader
            .do_upload_to_s3_with_receipt(key, bytes)
            .await
            .expect("same identity is idempotent");
        assert_eq!(replay, first);

        let conflict = uploader
            .do_upload_to_s3_with_receipt(key, b"different bytes")
            .await
            .expect_err("same key with a different manifest must fail");
        assert!(conflict.to_string().contains("identity conflict"));

        let payload = b"legacy-compatible-packet";
        let mut record = Vec::new();
        record.extend_from_slice(&1u32.to_le_bytes());
        record.extend_from_slice(&23u32.to_le_bytes());
        record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        record.extend_from_slice(&(payload.len() as u32).to_le_bytes());
        record.extend_from_slice(payload);
        let metadata_error = uploader
            .upload_with_journal(UploadTask {
                data: record,
                ts_start: 1_000_023,
                ts_end: 1_000_023,
                packet_count: 1,
                tenant_id: "tenant-a".to_string(),
                probe_id: "probe-a".to_string(),
            })
            .await
            .expect_err("metadata client is deliberately absent");
        assert!(metadata_error
            .to_string()
            .contains("gRPC metadata client is not initialized"));
        let entries = uploader
            .journal()
            .recover_by_state(usize::MAX)
            .expect("journal entries");
        let (_, compatible) = entries
            .iter()
            .find(|(_, entry)| entry.capture_uuid.len() == 36)
            .expect("legacy entry crossed durable spool and journal barriers");
        assert_eq!(
            compatible.effective_object_state(),
            JournalObjectState::RetryWait
        );
        assert_eq!(
            compatible.retry_from_state,
            Some(JournalObjectState::ObjectWritten)
        );
        assert!(compatible.object_write_receipt.is_some());
        assert!(compatible
            .local_path
            .as_deref()
            .is_some_and(|path| std::path::Path::new(path).exists()));
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
