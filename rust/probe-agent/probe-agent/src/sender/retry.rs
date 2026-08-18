use anyhow::{bail, Context, Result};
use prost::Message;
use proto_gen::{FlowEvent, FlowItemDisposition, FlowItemResult};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use sled::transaction::{ConflictableTransactionError, TransactionError};
use sled::Db;
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tracing::{debug, error, info, warn};

const CACHE_RECORD_VERSION: u32 = 1;
const CLAIM_LEASE: Duration = Duration::from_secs(60);
const META_PENDING_BATCHES: &[u8] = b"meta:pending_batches";
const META_PENDING_BYTES: &[u8] = b"meta:pending_bytes";
const MAX_ENCODED_EVENT_BYTES: usize = 64 * 1024;

#[derive(Clone, Copy, PartialEq, Eq)]
enum FlushMode {
    /// Synchronously flush inside the write path (tests / sync callers).
    Sync,
    /// Defer the flush to an async `spawn_blocking` helper (async callers).
    Deferred,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CachedBatchRef {
    pub batch_id: String,
    pub revision: u64,
    pub item_count: usize,
}

#[derive(Debug, Clone)]
pub struct ClaimedBatch {
    pub batch_ref: CachedBatchRef,
    pub events: Vec<FlowEvent>,
}

#[derive(Debug, Clone)]
pub struct AckPartition {
    pub response_revision: u32,
    pub item_results: Vec<FlowItemResult>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct AckApplication {
    pub terminal: usize,
    pub quarantined: usize,
    pub pending: usize,
    pub completed: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
enum CachedBatchState {
    DurableReady,
    Claimed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CachedItemRecord {
    event_id: String,
    payload_sha256: String,
    payload: Vec<u8>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
struct CachedBatchRecord {
    version: u32,
    batch_id: String,
    revision: u64,
    state: CachedBatchState,
    claim_until_ms: u64,
    items: Vec<CachedItemRecord>,
}

fn prefixed_key(prefix: &str, identity: &str) -> Vec<u8> {
    format!("{prefix}:{identity}").into_bytes()
}

fn sha256_hex(bytes: &[u8]) -> String {
    Sha256::digest(bytes)
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect()
}

fn now_epoch_millis() -> Result<u64> {
    Ok(u64::try_from(
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_millis(),
    )?)
}

fn encode_record(record: &CachedBatchRecord) -> Result<Vec<u8>> {
    serde_json::to_vec(record).context("failed to encode cached batch record")
}

fn decode_record(bytes: &[u8]) -> Result<CachedBatchRecord> {
    let record: CachedBatchRecord =
        serde_json::from_slice(bytes).context("failed to decode cached batch record")?;
    if record.version != CACHE_RECORD_VERSION || record.batch_id.is_empty() {
        bail!("unsupported cached batch record")
    }
    Ok(record)
}

fn batch_record(batch: &[FlowEvent]) -> Result<CachedBatchRecord> {
    if batch.is_empty() {
        bail!("cannot cache an empty FlowEvent batch")
    }
    let mut identities = HashSet::with_capacity(batch.len());
    let mut source = b"traffic-flow-wal/v1\0".to_vec();
    let mut items = Vec::with_capacity(batch.len());
    for event in batch {
        let event_id = event
            .header
            .as_ref()
            .map(|header| header.event_id.trim())
            .unwrap_or_default();
        if event_id.is_empty() || !identities.insert(event_id.to_owned()) {
            bail!("FlowEvent batch requires unique nonempty event_id values")
        }
        let payload = event.encode_to_vec();
        let payload_sha256 = sha256_hex(&payload);
        source.extend_from_slice(&(event_id.len() as u32).to_be_bytes());
        source.extend_from_slice(event_id.as_bytes());
        source.extend_from_slice(payload_sha256.as_bytes());
        items.push(CachedItemRecord {
            event_id: event_id.to_owned(),
            payload_sha256,
            payload,
        });
    }
    Ok(CachedBatchRecord {
        version: CACHE_RECORD_VERSION,
        batch_id: sha256_hex(&source),
        revision: 1,
        state: CachedBatchState::DurableReady,
        claim_until_ms: 0,
        items,
    })
}

fn events_from_record(record: &CachedBatchRecord) -> Result<Vec<FlowEvent>> {
    record
        .items
        .iter()
        .map(|item| {
            if sha256_hex(&item.payload) != item.payload_sha256 {
                bail!("cached FlowEvent payload hash mismatch")
            }
            FlowEvent::decode(item.payload.as_slice()).context("cached FlowEvent decode failed")
        })
        .collect()
}

fn legacy_decode_batch(data: &[u8]) -> Result<Vec<FlowEvent>> {
    let mut offset = 0usize;
    let mut batch = Vec::new();
    while offset < data.len() {
        if offset + 4 > data.len() {
            bail!("corrupted legacy batch: missing length prefix")
        }
        let len = u32::from_be_bytes(data[offset..offset + 4].try_into()?) as usize;
        offset += 4;
        let end = offset
            .checked_add(len)
            .ok_or_else(|| anyhow::anyhow!("corrupted legacy batch length overflow"))?;
        if end > data.len() {
            bail!("corrupted legacy batch: length out of bounds")
        }
        batch.push(FlowEvent::decode(&data[offset..end])?);
        offset = end;
    }
    if batch.is_empty() {
        bail!("corrupted legacy batch: empty payload")
    }
    Ok(batch)
}

fn record_payload_bytes(record: &CachedBatchRecord) -> usize {
    record.items.iter().map(|item| item.payload.len()).sum()
}

fn decode_counter(value: Option<sled::IVec>) -> std::result::Result<u64, String> {
    match value {
        Some(bytes) => serde_json::from_slice(&bytes).map_err(|error| error.to_string()),
        None => Ok(0),
    }
}

#[derive(Debug, Clone)]
pub struct CompactionStats {
    pub entries: usize,
    pub old_size: u64,
    pub new_size: u64,
    pub duration: Duration,
    pub space_saved: u64,
    pub compaction_ratio: f64,
}

impl std::fmt::Display for CompactionStats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "Compaction: {} entries, {} -> {} bytes ({:.1}% saved), took {:?}",
            self.entries,
            self.old_size,
            self.new_size,
            self.compaction_ratio * 100.0,
            self.duration
        )
    }
}

struct CacheMetrics {
    delete_count: AtomicU64,
    insert_count: AtomicU64,
    last_compaction: parking_lot::Mutex<Option<Instant>>,
    total_compactions: AtomicUsize,
}

impl CacheMetrics {
    fn new() -> Self {
        Self {
            delete_count: AtomicU64::new(0),
            insert_count: AtomicU64::new(0),
            last_compaction: parking_lot::Mutex::new(None),
            total_compactions: AtomicUsize::new(0),
        }
    }

    fn record_insert(&self) {
        self.insert_count.fetch_add(1, Ordering::Relaxed);
    }

    fn record_delete(&self) {
        self.delete_count.fetch_add(1, Ordering::Relaxed);
    }

    fn record_compaction(&self) {
        *self.last_compaction.lock() = Some(Instant::now());
        self.total_compactions.fetch_add(1, Ordering::Relaxed);
        self.delete_count.store(0, Ordering::Relaxed);
    }

    fn time_since_last_compaction(&self) -> Option<Duration> {
        self.last_compaction
            .lock()
            .as_ref()
            .map(|instant| instant.elapsed())
    }

    fn delete_count(&self) -> u64 {
        self.delete_count.load(Ordering::Relaxed)
    }
}

#[derive(Clone)]
pub struct LocalCache {
    db: Arc<Db>,
    db_path: PathBuf,
    max_size: usize,
    metrics: Arc<CacheMetrics>,
    compaction_threshold_deletes: u64,
    compaction_threshold_size_ratio: f64,
    compaction_interval: Duration,
}

impl LocalCache {
    pub fn new(path: &Path, max_size: usize) -> Result<Self> {
        let db_path = path.join("flow_cache");
        let config = sled::Config::new()
            .path(&db_path)
            .mode(sled::Mode::HighThroughput)
            .flush_every_ms(Some(100))
            .use_compression(false);
        let db = config
            .open()
            .context("Failed to open local cache database")?;
        let cache = Self {
            db: Arc::new(db),
            db_path,
            max_size,
            metrics: Arc::new(CacheMetrics::new()),
            compaction_threshold_deletes: 10000,
            compaction_threshold_size_ratio: 2.0,
            compaction_interval: Duration::from_secs(24 * 3600),
        };
        cache.migrate_legacy_records()?;
        cache.reconcile_usage()?;
        info!(
            "Local cache opened at {:?}, pending_batches={}",
            cache.db_path,
            cache.size()?
        );
        Ok(cache)
    }

    pub fn with_compaction_config(
        path: &Path,
        max_size: usize,
        threshold_deletes: u64,
        size_ratio: f64,
        interval: Duration,
    ) -> Result<Self> {
        let mut cache = Self::new(path, max_size)?;
        cache.compaction_threshold_deletes = threshold_deletes;
        cache.compaction_threshold_size_ratio = size_ratio;
        cache.compaction_interval = interval;
        Ok(cache)
    }

    fn migrate_legacy_records(&self) -> Result<()> {
        let legacy: Vec<_> = self
            .db
            .iter()
            .filter_map(|entry| match entry {
                Ok((key, value)) if key.len() == 8 => Some(Ok((key.to_vec(), value.to_vec()))),
                Ok(_) => None,
                Err(error) => Some(Err(error)),
            })
            .collect::<std::result::Result<_, _>>()?;
        for (legacy_key, legacy_value) in legacy {
            let batch = match legacy_decode_batch(&legacy_value) {
                Ok(batch) => batch,
                Err(error) => {
                    warn!("Quarantining corrupt legacy Flow WAL record: {}", error);
                    self.quarantine_raw_record(&legacy_key, &legacy_value, "CORRUPT_LEGACY_WAL")?;
                    continue;
                }
            };
            let record = match batch_record(&batch) {
                Ok(record) => record,
                Err(error) => {
                    warn!("Quarantining invalid legacy Flow WAL record: {}", error);
                    self.quarantine_raw_record(
                        &legacy_key,
                        &legacy_value,
                        "INVALID_LEGACY_IDENTITY",
                    )?;
                    continue;
                }
            };
            let new_key = prefixed_key("batch", &record.batch_id);
            let encoded = encode_record(&record)?;
            (&*self.db)
                .transaction(|tree| {
                    if let Some(existing) = tree.get(&new_key)? {
                        if existing.as_ref() != encoded.as_slice() {
                            return Err(ConflictableTransactionError::Abort(
                                "legacy Flow WAL migration conflicts with an existing batch"
                                    .to_owned(),
                            ));
                        }
                    } else {
                        tree.insert(new_key.clone(), encoded.clone())?;
                    }
                    tree.remove(legacy_key.clone())?;
                    Ok(())
                })
                .map_err(|error| match error {
                    TransactionError::Abort(message) => anyhow::anyhow!(message),
                    TransactionError::Storage(error) => anyhow::Error::from(error),
                })?;
        }
        if !self.db.is_empty() {
            self.db.flush()?;
        }
        Ok(())
    }

    fn quarantine_raw_record(&self, key: &[u8], value: &[u8], reason: &str) -> Result<()> {
        let quarantine_key =
            prefixed_key("quarantine-raw", &format!("{}:{}", reason, sha256_hex(key)));
        let quarantine_value = serde_json::to_vec(&(reason, key, sha256_hex(value), value))?;
        (&*self.db)
            .transaction(|tree| {
                tree.insert(quarantine_key.clone(), quarantine_value.clone())?;
                tree.remove(key.to_vec())?;
                Ok::<(), ConflictableTransactionError<String>>(())
            })
            .map_err(|error| match error {
                TransactionError::Abort(message) => anyhow::anyhow!(message),
                TransactionError::Storage(error) => anyhow::Error::from(error),
            })?;
        self.db.flush()?;
        Ok(())
    }

    fn reconcile_usage(&self) -> Result<()> {
        let mut batch_count = 0u64;
        let mut payload_bytes = 0u64;
        let mut corrupt = Vec::new();
        for entry in self.db.scan_prefix(b"batch:") {
            let (key, value) = entry?;
            let record = match decode_record(&value) {
                Ok(record) => record,
                Err(error) => {
                    warn!(
                        "Quarantining corrupt Flow WAL record during startup: {}",
                        error
                    );
                    corrupt.push((key.to_vec(), value.to_vec()));
                    continue;
                }
            };
            batch_count = batch_count.saturating_add(1);
            payload_bytes = payload_bytes.saturating_add(record_payload_bytes(&record) as u64);
        }
        for (key, value) in corrupt {
            self.quarantine_raw_record(&key, &value, "CORRUPT_NEW_WAL")?;
        }
        (&*self.db)
            .transaction(|tree| {
                tree.insert(
                    META_PENDING_BATCHES.to_vec(),
                    serde_json::to_vec(&batch_count)
                        .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
                )?;
                tree.insert(
                    META_PENDING_BYTES.to_vec(),
                    serde_json::to_vec(&payload_bytes)
                        .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
                )?;
                Ok(())
            })
            .map_err(|error| match error {
                TransactionError::Abort(message) => anyhow::anyhow!(message),
                TransactionError::Storage(error) => anyhow::Error::from(error),
            })?;
        self.db.flush()?;
        Ok(())
    }

    pub fn save(&self, batch: &[FlowEvent]) -> Result<CachedBatchRef> {
        self.save_inner(batch, FlushMode::Sync)
    }

    /// Async variant used from async contexts: the durable `flush` runs on a
    /// blocking thread so the async caller (gRPC sender) is not stalled by
    /// synchronous disk I/O.
    pub async fn save_async(&self, batch: &[FlowEvent]) -> Result<CachedBatchRef> {
        let batch_ref = self.save_inner(batch, FlushMode::Deferred)?;
        self.flush_async().await?;
        Ok(batch_ref)
    }

    async fn flush_async(&self) -> Result<()> {
        let db = self.db.clone();
        tokio::task::spawn_blocking(move || db.flush())
            .await
            .context("cache flush task panicked")??;
        Ok(())
    }

    fn save_inner(&self, batch: &[FlowEvent], flush_mode: FlushMode) -> Result<CachedBatchRef> {
        let record = batch_record(batch)?;
        let key = prefixed_key("batch", &record.batch_id);
        let encoded = encode_record(&record)?;
        let payload_bytes = record_payload_bytes(&record) as u64;
        let max_items = self.max_size as u64;
        let max_bytes = max_items.saturating_mul(MAX_ENCODED_EVENT_BYTES as u64);
        let saved = (&*self.db).transaction(|tree| {
            if let Some(existing_bytes) = tree.get(&key)? {
                let existing: CachedBatchRecord = serde_json::from_slice(&existing_bytes)
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?;
                let same = existing
                    .items
                    .iter()
                    .map(|item| (&item.event_id, &item.payload_sha256))
                    .eq(record
                        .items
                        .iter()
                        .map(|item| (&item.event_id, &item.payload_sha256)));
                if !same {
                    return Err(ConflictableTransactionError::Abort(
                        "cached batch identity conflicts with different item bytes".to_owned(),
                    ));
                }
                return Ok((false, existing.revision, existing.items.len()));
            }
            let pending_batches = decode_counter(tree.get(META_PENDING_BATCHES)?)
                .map_err(ConflictableTransactionError::Abort)?;
            let pending_bytes = decode_counter(tree.get(META_PENDING_BYTES)?)
                .map_err(ConflictableTransactionError::Abort)?;
            if pending_batches.saturating_add(1) > max_items
                || pending_bytes.saturating_add(payload_bytes) > max_bytes
            {
                return Err(ConflictableTransactionError::Abort(format!(
                    "Cache is full (max_items={max_items}, max_bytes={max_bytes})"
                )));
            }
            tree.insert(key.clone(), encoded.clone())?;
            tree.insert(
                META_PENDING_BATCHES.to_vec(),
                serde_json::to_vec(&pending_batches.saturating_add(1))
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
            )?;
            tree.insert(
                META_PENDING_BYTES.to_vec(),
                serde_json::to_vec(&pending_bytes.saturating_add(payload_bytes))
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
            )?;
            Ok((true, record.revision, record.items.len()))
        });
        let (inserted, revision, saved_item_count) = match saved {
            Ok(value) => value,
            Err(TransactionError::Abort(message)) => bail!(message),
            Err(TransactionError::Storage(error)) => return Err(error.into()),
        };
        if matches!(flush_mode, FlushMode::Sync) {
            self.db.flush()?;
        }
        if inserted {
            self.metrics.record_insert();
        }
        debug!("Cached batch: id={}, size={}", record.batch_id, batch.len());
        Ok(CachedBatchRef {
            batch_id: record.batch_id,
            revision,
            item_count: saved_item_count,
        })
    }

    pub fn get_pending(&self, limit: usize) -> Result<Vec<(CachedBatchRef, Vec<FlowEvent>)>> {
        let mut result = Vec::new();
        let mut corrupt = Vec::new();
        for item in self.db.scan_prefix(b"batch:") {
            if result.len() >= limit {
                break;
            }
            let (key, value) = item?;
            let record = match decode_record(&value)
                .and_then(|record| events_from_record(&record).map(|events| (record, events)))
            {
                Ok(record) => record,
                Err(error) => {
                    warn!("Quarantining corrupt Flow WAL record: {}", error);
                    corrupt.push((key.to_vec(), value.to_vec()));
                    continue;
                }
            };
            let batch_ref = CachedBatchRef {
                batch_id: record.0.batch_id.clone(),
                revision: record.0.revision,
                item_count: record.0.items.len(),
            };
            result.push((batch_ref, record.1));
        }
        if !corrupt.is_empty() {
            for (key, value) in corrupt {
                self.quarantine_raw_record(&key, &value, "CORRUPT_NEW_WAL")?;
            }
            self.reconcile_usage()?;
        }
        Ok(result)
    }

    pub fn claim(&self, batch_ref: &CachedBatchRef) -> Result<ClaimedBatch> {
        let key = prefixed_key("batch", &batch_ref.batch_id);
        let bytes = self
            .db
            .get(&key)?
            .ok_or_else(|| anyhow::anyhow!("cached batch is missing"))?;
        let mut record = decode_record(&bytes)?;
        if record.revision != batch_ref.revision {
            bail!("cached batch revision mismatch")
        }
        let now_ms = now_epoch_millis()?;
        if record.state == CachedBatchState::Claimed && record.claim_until_ms > now_ms {
            bail!("cached batch already has an active claim")
        }
        record.state = CachedBatchState::Claimed;
        record.revision += 1;
        record.claim_until_ms = now_ms + CLAIM_LEASE.as_millis() as u64;
        self.db
            .compare_and_swap(&key, Some(bytes), Some(encode_record(&record)?))?
            .map_err(|_| anyhow::anyhow!("cached batch changed during claim"))?;
        self.db.flush()?;
        Ok(ClaimedBatch {
            batch_ref: CachedBatchRef {
                batch_id: record.batch_id.clone(),
                revision: record.revision,
                item_count: record.items.len(),
            },
            events: events_from_record(&record)?,
        })
    }

    pub fn release_claim(&self, batch_ref: &CachedBatchRef) -> Result<()> {
        let key = prefixed_key("batch", &batch_ref.batch_id);
        let bytes = self
            .db
            .get(&key)?
            .ok_or_else(|| anyhow::anyhow!("cached batch is missing"))?;
        let mut record = decode_record(&bytes)?;
        if record.revision != batch_ref.revision || record.state != CachedBatchState::Claimed {
            bail!("cached batch claim revision mismatch")
        }
        record.state = CachedBatchState::DurableReady;
        record.revision += 1;
        record.claim_until_ms = 0;
        self.db
            .compare_and_swap(&key, Some(bytes), Some(encode_record(&record)?))?
            .map_err(|_| anyhow::anyhow!("cached batch changed during claim release"))?;
        self.db.flush()?;
        Ok(())
    }

    pub fn apply_ack(
        &self,
        batch_ref: &CachedBatchRef,
        ack: &AckPartition,
    ) -> Result<AckApplication> {
        if ack.response_revision != 1 {
            bail!("unsupported Flow ACK response revision")
        }
        let batch_key = prefixed_key("batch", &batch_ref.batch_id);
        let application_key = prefixed_key(
            "ack",
            &format!("{}:{}", batch_ref.batch_id, batch_ref.revision),
        );
        let quarantine_prefix = prefixed_key("quarantine", &batch_ref.batch_id);
        let application = (&*self.db).transaction(|tree| {
            if let Some(bytes) = tree.get(&application_key)? {
                return serde_json::from_slice::<AckApplication>(&bytes)
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()));
            }
            let bytes = tree.get(&batch_key)?.ok_or_else(|| {
                ConflictableTransactionError::Abort("cached batch is missing".to_owned())
            })?;
            let mut record: CachedBatchRecord = serde_json::from_slice(&bytes)
                .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?;
            if record.revision != batch_ref.revision || record.state != CachedBatchState::Claimed {
                return Err(ConflictableTransactionError::Abort(
                    "cached batch ACK revision mismatch".to_owned(),
                ));
            }
            if ack.item_results.len() != record.items.len() {
                return Err(ConflictableTransactionError::Abort(
                    "Flow ACK exact-set cardinality mismatch".to_owned(),
                ));
            }
            let expected: HashSet<_> = record
                .items
                .iter()
                .map(|item| item.event_id.as_str())
                .collect();
            let mut seen = HashSet::with_capacity(ack.item_results.len());
            for result in &ack.item_results {
                if !expected.contains(result.event_id.as_str())
                    || !seen.insert(result.event_id.as_str())
                    || result.input_index as usize >= record.items.len()
                    || record.items[result.input_index as usize].event_id != result.event_id
                {
                    return Err(ConflictableTransactionError::Abort(
                        "Flow ACK contains missing duplicate foreign or misindexed identity"
                            .to_owned(),
                    ));
                }
            }

            let mut by_index: Vec<Option<&FlowItemResult>> = vec![None; record.items.len()];
            for result in &ack.item_results {
                by_index[result.input_index as usize] = Some(result);
            }
            let prior_bytes = record_payload_bytes(&record) as u64;
            let mut terminal = 0usize;
            let mut quarantined = 0usize;
            let mut pending = Vec::new();
            for (input_index, item) in record.items.into_iter().enumerate() {
                let result = by_index[input_index].expect("exact-set validated above");
                match FlowItemDisposition::try_from(result.disposition)
                    .unwrap_or(FlowItemDisposition::Unspecified)
                {
                    FlowItemDisposition::KafkaAcked | FlowItemDisposition::DuplicateCommitted => {
                        terminal += 1
                    }
                    FlowItemDisposition::RejectedInvalid => {
                        if result.reason_code.trim().is_empty() {
                            return Err(ConflictableTransactionError::Abort(
                                "invalid Flow ACK requires a stable reason code".to_owned(),
                            ));
                        }
                        let key =
                            [quarantine_prefix.as_slice(), b":", item.event_id.as_bytes()].concat();
                        let value = serde_json::to_vec(&(item, result.reason_code.clone()))
                            .map_err(|error| {
                                ConflictableTransactionError::Abort(error.to_string())
                            })?;
                        tree.insert(key, value)?;
                        quarantined += 1;
                    }
                    FlowItemDisposition::Retryable | FlowItemDisposition::OutcomeUnknown => {
                        pending.push(item)
                    }
                    FlowItemDisposition::Unspecified => {
                        return Err(ConflictableTransactionError::Abort(
                            "unspecified Flow ACK disposition".to_owned(),
                        ));
                    }
                }
            }
            let completed = pending.is_empty();
            let remaining_bytes: u64 = pending.iter().map(|item| item.payload.len() as u64).sum();
            let result = AckApplication {
                terminal,
                quarantined,
                pending: pending.len(),
                completed,
            };
            if completed {
                tree.remove(batch_key.clone())?;
            } else {
                record.items = pending;
                record.state = CachedBatchState::DurableReady;
                record.revision += 1;
                record.claim_until_ms = 0;
                tree.insert(
                    batch_key.clone(),
                    serde_json::to_vec(&record)
                        .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
                )?;
            }
            let pending_batches = decode_counter(tree.get(META_PENDING_BATCHES)?)
                .map_err(ConflictableTransactionError::Abort)?;
            let pending_bytes = decode_counter(tree.get(META_PENDING_BYTES)?)
                .map_err(ConflictableTransactionError::Abort)?;
            tree.insert(
                META_PENDING_BATCHES.to_vec(),
                serde_json::to_vec(&pending_batches.saturating_sub(u64::from(completed)))
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
            )?;
            tree.insert(
                META_PENDING_BYTES.to_vec(),
                serde_json::to_vec(&pending_bytes.saturating_sub(prior_bytes - remaining_bytes))
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
            )?;
            tree.insert(
                application_key.clone(),
                serde_json::to_vec(&result)
                    .map_err(|error| ConflictableTransactionError::Abort(error.to_string()))?,
            )?;
            Ok(result)
        });
        let application = match application {
            Ok(application) => application,
            Err(TransactionError::Abort(message)) => bail!(message),
            Err(TransactionError::Storage(error)) => return Err(error.into()),
        };
        self.db.flush()?;
        if application.completed {
            self.metrics.record_delete();
        }
        Ok(application)
    }

    pub fn remove(&self, key: u64) -> Result<()> {
        // Legacy key-only records remain readable during migration. New WAL
        // callers must use exact-set `apply_ack` and cannot delete whole batches.
        self.db.remove(&key.to_be_bytes())?;
        self.db.flush()?;
        self.metrics.record_delete();
        Ok(())
    }

    pub fn size(&self) -> Result<usize> {
        Ok(
            decode_counter(self.db.get(META_PENDING_BATCHES)?).map_err(anyhow::Error::msg)?
                as usize,
        )
    }

    pub fn clear(&self) -> Result<()> {
        self.db.clear()?;
        self.db
            .insert(META_PENDING_BATCHES, serde_json::to_vec(&0u64)?)?;
        self.db
            .insert(META_PENDING_BYTES, serde_json::to_vec(&0u64)?)?;
        self.db.flush()?;
        info!("Local cache cleared");
        Ok(())
    }

    pub fn flush(&self) -> Result<()> {
        self.db.flush()?;
        Ok(())
    }

    pub fn should_compact(&self) -> bool {
        let delete_threshold_reached =
            self.metrics.delete_count() >= self.compaction_threshold_deletes;
        let time_threshold_reached = self
            .metrics
            .time_since_last_compaction()
            .map(|elapsed| elapsed >= self.compaction_interval)
            .unwrap_or(true);
        let size_threshold_reached = match self.check_size_ratio() {
            Ok(ratio_exceeded) => ratio_exceeded,
            Err(e) => {
                debug!("Failed to check size ratio: {}", e);
                false
            }
        };
        let should_compact =
            delete_threshold_reached || time_threshold_reached || size_threshold_reached;
        if should_compact {
            debug!(
                "Compaction needed: deletes={} (threshold={}), time_elapsed={} (threshold={}s), size_ratio={}",
                self.metrics.delete_count(),
                self.compaction_threshold_deletes,
                self.metrics
                    .time_since_last_compaction()
                    .map(|d| format!("{}s", d.as_secs()))
                    .unwrap_or_else(|| "never".to_string()),
                self.compaction_interval.as_secs(),
                size_threshold_reached
            );
        }
        should_compact
    }

    fn check_size_ratio(&self) -> Result<bool> {
        let physical_size = calculate_dir_size(&self.db_path)?;
        let logical_size = self.db.len() * 1024;
        if logical_size == 0 {
            return Ok(false);
        }
        let ratio = physical_size as f64 / logical_size as f64;
        Ok(ratio > self.compaction_threshold_size_ratio)
    }

    pub fn compact(&self) -> Result<CompactionStats> {
        info!("Starting LocalCache compaction...");
        let start = Instant::now();
        let temp_path = self
            .db_path
            .parent()
            .context("db_path has no parent")?
            .join(format!(
                "{}_compact_{}",
                self.db_path
                    .file_name()
                    .context("db_path has no filename")?
                    .to_string_lossy(),
                chrono::Utc::now().timestamp_millis()
            ));
        let old_size = calculate_dir_size(&self.db_path)?;
        // NOTE: the scratch DB must NOT be `temporary(true)`: a temporary sled
        // database is deleted when dropped, so the previous implementation
        // wiped the entire WAL copy on every compaction and reloaded an empty
        // DB, silently discarding all un-ACKed events. The scratch copy is
        // persistent until we explicitly remove it after a verified reload.
        let temp_config = sled::Config::new()
            .path(&temp_path)
            .mode(sled::Mode::HighThroughput)
            .use_compression(true);
        let temp_db = temp_config
            .open()
            .context("failed to open compaction scratch DB")?;
        let original_len = self.db.len();
        let mut copied = 0usize;
        for item in self.db.iter() {
            let (k, v) = item.context("failed to read entry while copying for compaction")?;
            temp_db
                .insert(k, v)
                .context("failed to write compaction scratch entry")?;
            copied += 1;
            if copied % 10000 == 0 {
                debug!("Compaction progress: {} entries copied", copied);
            }
        }
        temp_db
            .flush()
            .context("failed to flush compaction scratch DB")?;
        let scratch_len = temp_db.len();
        if scratch_len != original_len {
            drop(temp_db);
            let _ = std::fs::remove_dir_all(&temp_path);
            bail!(
                "compaction scratch copy incomplete: source={} copied={}; original cache left intact",
                original_len,
                scratch_len
            );
        }
        drop(temp_db);
        let new_size = calculate_dir_size(&temp_path)?;
        // Only now, after a fully verified persistent copy exists, reload the
        // live DB from that copy.
        self.db
            .clear()
            .context("failed to clear cache before compaction reload")?;
        self.db
            .flush()
            .context("failed to flush cache after clear")?;
        let final_config = sled::Config::new()
            .path(&temp_path)
            .mode(sled::Mode::HighThroughput)
            .use_compression(true);
        let temp_db = final_config
            .open()
            .context("failed to reopen compaction scratch DB")?;
        let mut restored = 0usize;
        for item in temp_db.iter() {
            let (k, v) = item.context("failed to read compaction scratch entry")?;
            self.db
                .insert(k, v)
                .context("failed to restore entry after compaction")?;
            restored += 1;
        }
        self.db
            .flush()
            .context("failed to flush cache after compaction reload")?;
        drop(temp_db);
        if restored != copied {
            error!(
                "compaction reload incomplete: copied={} restored={}; verified scratch copy retained at {:?} for recovery",
                copied, restored, temp_path
            );
            return Err(anyhow::anyhow!(
                "compaction reload incomplete: copied={} restored={}",
                copied,
                restored
            ));
        }
        if let Err(e) = std::fs::remove_dir_all(&temp_path) {
            warn!("Failed to remove temporary compaction directory: {}", e);
        }
        self.metrics.record_compaction();
        let elapsed = start.elapsed();
        let space_saved = old_size.saturating_sub(new_size);
        let compaction_ratio = if old_size > 0 {
            space_saved as f64 / old_size as f64
        } else {
            0.0
        };
        let stats = CompactionStats {
            entries: copied,
            old_size,
            new_size,
            duration: elapsed,
            space_saved,
            compaction_ratio,
        };
        info!("{}", stats);
        Ok(stats)
    }

    pub fn compact_if_needed(&self) -> Result<Option<CompactionStats>> {
        if self.should_compact() {
            info!("Auto-compaction triggered");
            self.compact().map(Some)
        } else {
            Ok(None)
        }
    }

    pub fn compaction_stats(&self) -> CompactionMetrics {
        CompactionMetrics {
            total_compactions: self.metrics.total_compactions.load(Ordering::Relaxed),
            delete_count: self.metrics.delete_count(),
            time_since_last_compaction: self.metrics.time_since_last_compaction(),
        }
    }

    pub fn spawn_compaction_task(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(3600));
            info!(
                "LocalCache compaction task started: check interval={}s",
                3600
            );
            loop {
                interval.tick().await;
                match self.compact_if_needed() {
                    Ok(Some(stats)) => {
                        info!("Scheduled compaction completed: {}", stats);
                    }
                    Ok(None) => {
                        debug!("Compaction not needed");
                    }
                    Err(e) => {
                        error!("Scheduled compaction failed: {}", e);
                    }
                }
            }
        })
    }
}

impl Drop for LocalCache {
    fn drop(&mut self) {
        if Arc::strong_count(&self.db) == 1 {
            if let Err(e) = self.db.flush() {
                error!("Failed to flush cache on drop: {}", e);
            } else {
                debug!("Cache flushed successfully on drop");
            }
        }
    }
}

#[derive(Debug, Clone)]
pub struct CompactionMetrics {
    pub total_compactions: usize,
    pub delete_count: u64,
    pub time_since_last_compaction: Option<Duration>,
}

fn calculate_dir_size(path: &Path) -> Result<u64> {
    let mut total_size = 0u64;
    if path.is_file() {
        return Ok(path.metadata()?.len());
    }
    if !path.exists() {
        return Ok(0);
    }
    for entry in std::fs::read_dir(path)? {
        let entry = entry?;
        let metadata = entry.metadata()?;
        if metadata.is_file() {
            total_size += metadata.len();
        } else if metadata.is_dir() {
            total_size += calculate_dir_size(&entry.path())?;
        }
    }
    Ok(total_size)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Barrier;
    use tempfile::TempDir;

    #[test]
    fn local_cache_round_trips_pending_batches_and_enforces_capacity() -> Result<()> {
        let temp_dir = TempDir::new()?;
        let cache = LocalCache::new(temp_dir.path(), 1)?;
        let batch = vec![flow_event("flow-a"), flow_event("flow-b")];

        let batch_ref = cache.save(&batch)?;
        assert_eq!(cache.size()?, 1);

        let pending = cache.get_pending(10)?;
        assert_eq!(pending.len(), 1);
        assert_eq!(pending[0].1.len(), 2);
        assert_eq!(pending[0].1[0].flow_id, "flow-a");
        assert_eq!(pending[0].1[1].flow_id, "flow-b");

        assert_eq!(cache.save(&batch)?.batch_id, batch_ref.batch_id);
        let full_error = cache
            .save(&[flow_event("flow-c")])
            .expect_err("full cache should reject a distinct batch");
        assert!(full_error.to_string().contains("Cache is full"));

        let claimed = cache.claim(&pending[0].0)?;
        let ack = AckPartition {
            response_revision: 1,
            item_results: claimed
                .events
                .iter()
                .enumerate()
                .map(|(input_index, event)| FlowItemResult {
                    input_index: input_index as u32,
                    event_id: event.header.as_ref().unwrap().event_id.clone(),
                    disposition: FlowItemDisposition::KafkaAcked as i32,
                    reason_code: String::new(),
                    ack_scope: "KAFKA_BROKER_DURABLE".to_owned(),
                })
                .collect(),
        };
        cache.apply_ack(&claimed.batch_ref, &ack)?;
        assert_eq!(cache.size()?, 0);

        Ok(())
    }

    #[test]
    fn concurrent_wal_admission_never_exceeds_item_capacity() -> Result<()> {
        let temp_dir = TempDir::new()?;
        let cache = LocalCache::new(temp_dir.path(), 1)?;
        let barrier = Arc::new(Barrier::new(32));
        let mut workers = Vec::new();
        for index in 0..32 {
            let cache = cache.clone();
            let barrier = Arc::clone(&barrier);
            workers.push(std::thread::spawn(move || {
                barrier.wait();
                cache.save(&[flow_event(&format!("concurrent-{index}"))])
            }));
        }
        let admitted = workers
            .into_iter()
            .map(|worker| worker.join().expect("worker panicked"))
            .filter(Result::is_ok)
            .count();
        assert_eq!(admitted, 1);
        assert_eq!(cache.size()?, 1);
        assert_eq!(cache.get_pending(10)?.len(), 1);
        Ok(())
    }

    #[test]
    fn startup_migrates_legacy_and_quarantines_corrupt_without_blocking_healthy() -> Result<()> {
        let temp_dir = TempDir::new()?;
        let db_path = temp_dir.path().join("flow_cache");
        let db = sled::open(&db_path)?;
        let event = flow_event("legacy-healthy");
        let payload = event.encode_to_vec();
        let mut legacy = Vec::new();
        legacy.extend_from_slice(&(payload.len() as u32).to_be_bytes());
        legacy.extend_from_slice(&payload);
        db.insert(1u64.to_be_bytes(), legacy)?;
        db.insert(2u64.to_be_bytes(), b"corrupt".as_slice())?;
        db.flush()?;
        drop(db);

        let cache = LocalCache::new(temp_dir.path(), 10)?;
        let pending = cache.get_pending(10)?;
        assert_eq!(pending.len(), 1);
        assert_eq!(
            pending[0].1[0].header.as_ref().unwrap().event_id,
            "legacy-healthy"
        );
        assert_eq!(cache.size()?, 1);
        assert_eq!(cache.db.scan_prefix(b"quarantine-raw:").count(), 1);
        assert!(cache.db.get(1u64.to_be_bytes())?.is_none());
        assert!(cache.db.get(2u64.to_be_bytes())?.is_none());
        Ok(())
    }

    fn flow_event(flow_id: &str) -> FlowEvent {
        FlowEvent {
            flow_id: flow_id.to_string(),
            header: Some(proto_gen::EventHeader {
                event_id: flow_id.to_string(),
                ..proto_gen::EventHeader::default()
            }),
            ..FlowEvent::default()
        }
    }
}
