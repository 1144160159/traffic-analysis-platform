use anyhow::{bail, Context, Result};
use prost::Message;
use proto_gen::FlowEvent;
use sled::Db;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tracing::{debug, error, info, warn};

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
        info!("Local cache opened at {:?}, entries: {}", db_path, db.len());
        Ok(Self {
            db: Arc::new(db),
            db_path,
            max_size,
            metrics: Arc::new(CacheMetrics::new()),
            compaction_threshold_deletes: 10000,
            compaction_threshold_size_ratio: 2.0,
            compaction_interval: Duration::from_secs(24 * 3600),
        })
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

    pub fn save(&self, batch: &[FlowEvent]) -> Result<()> {
        if self.size()? >= self.max_size {
            bail!("Cache is full ({} entries)", self.max_size);
        }
        let key = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)?
            .as_micros() as u64;
        let serialized = encode_batch(batch)?;
        self.db.insert(&key.to_be_bytes(), serialized)?;
        self.db.flush()?;
        self.metrics.record_insert();
        debug!("Cached batch: key={}, size={}", key, batch.len());
        Ok(())
    }

    pub fn get_pending(&self, limit: usize) -> Result<Vec<(u64, Vec<FlowEvent>)>> {
        let mut result = Vec::new();
        for item in self.db.iter().take(limit) {
            let (key, value) = item?;
            let key_bytes: [u8; 8] = key.as_ref().try_into()?;
            let key_u64 = u64::from_be_bytes(key_bytes);
            let batch = decode_batch(&value)?;
            result.push((key_u64, batch));
        }
        Ok(result)
    }

    pub fn remove(&self, key: u64) -> Result<()> {
        self.db.remove(&key.to_be_bytes())?;
        self.db.flush()?;
        self.metrics.record_delete();
        debug!("Removed cached batch: key={}", key);
        Ok(())
    }

    pub fn size(&self) -> Result<usize> {
        Ok(self.db.len())
    }

    pub fn clear(&self) -> Result<()> {
        self.db.clear()?;
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
            .expect("db_path has no parent")
            .join(format!(
                "{}_compact_{}",
                self.db_path
                    .file_name()
                    .expect("db_path has no filename")
                    .to_string_lossy(),
                chrono::Utc::now().timestamp_millis()
            ));
        let old_size = calculate_dir_size(&self.db_path)?;
        let temp_config = sled::Config::new()
            .path(&temp_path)
            .mode(sled::Mode::HighThroughput)
            .use_compression(true)
            .temporary(true);
        let temp_db = temp_config.open()?;
        let mut copied = 0;
        for item in self.db.iter() {
            let (k, v) = item?;
            temp_db.insert(k, v)?;
            copied += 1;
            if copied % 10000 == 0 {
                debug!("Compaction progress: {} entries copied", copied);
            }
        }
        temp_db.flush()?;
        drop(temp_db);
        let new_size = calculate_dir_size(&temp_path)?;
        self.db.clear()?;
        self.db.flush()?;
        let final_config = sled::Config::new()
            .path(&temp_path)
            .mode(sled::Mode::HighThroughput)
            .use_compression(true);
        let temp_db = final_config.open()?;
        for item in temp_db.iter() {
            let (k, v) = item?;
            self.db.insert(k, v)?;
        }
        self.db.flush()?;
        drop(temp_db);
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

fn encode_batch(batch: &[FlowEvent]) -> Result<Vec<u8>> {
    let mut buf = Vec::new();
    for event in batch {
        let mut tmp = Vec::with_capacity(event.encoded_len());
        event.encode(&mut tmp)?;
        let len = tmp.len() as u32;
        buf.extend_from_slice(&len.to_be_bytes());
        buf.extend_from_slice(&tmp);
    }
    Ok(buf)
}

fn decode_batch(data: &[u8]) -> Result<Vec<FlowEvent>> {
    let mut offset = 0usize;
    let mut batch = Vec::new();
    while offset < data.len() {
        if offset + 4 > data.len() {
            bail!("Corrupted batch: missing length prefix");
        }
        let len = u32::from_be_bytes(data[offset..offset + 4].try_into()?) as usize;
        offset += 4;
        let end = offset + len;
        if end > data.len() {
            bail!("Corrupted batch: length out of bounds");
        }
        let event = FlowEvent::decode(&data[offset..end])?;
        batch.push(event);
        offset = end;
    }
    Ok(batch)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn local_cache_round_trips_pending_batches_and_enforces_capacity() -> Result<()> {
        let temp_dir = TempDir::new()?;
        let cache = LocalCache::new(temp_dir.path(), 1)?;
        let batch = vec![flow_event("flow-a"), flow_event("flow-b")];

        cache.save(&batch)?;
        assert_eq!(cache.size()?, 1);

        let pending = cache.get_pending(10)?;
        assert_eq!(pending.len(), 1);
        assert_eq!(pending[0].1.len(), 2);
        assert_eq!(pending[0].1[0].flow_id, "flow-a");
        assert_eq!(pending[0].1[1].flow_id, "flow-b");

        let full_error = cache.save(&batch).expect_err("full cache should reject new batches");
        assert!(full_error.to_string().contains("Cache is full"));

        cache.remove(pending[0].0)?;
        assert_eq!(cache.size()?, 0);

        Ok(())
    }

    fn flow_event(flow_id: &str) -> FlowEvent {
        FlowEvent {
            flow_id: flow_id.to_string(),
            ..FlowEvent::default()
        }
    }
}
