use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use sled::Db;
use std::path::Path;
use std::sync::Arc;
use tracing::{debug, error, info, warn};

use super::UploadTask;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JournalEntry {
    pub task_id: String,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packet_count: u64,
    pub tenant_id: String,
    pub probe_id: String,
    pub local_path: Option<String>,
    pub s3_key: Option<String>,
    pub metadata_synced: bool,
    pub created_at: u64,
    pub retry_count: u32,
    pub last_error: Option<String>,
    pub s3_uploaded_at: Option<u64>,
    pub metadata_synced_at: Option<u64>,
}

impl JournalEntry {
    pub fn is_complete(&self) -> bool {
        self.s3_key.is_some() && self.metadata_synced
    }

    pub fn is_s3_uploaded(&self) -> bool {
        self.s3_key.is_some()
    }

    pub fn needs_s3_upload(&self) -> bool {
        self.s3_key.is_none()
    }

    pub fn needs_metadata_sync(&self) -> bool {
        self.s3_key.is_some() && !self.metadata_synced
    }
}

#[derive(Clone)]
pub struct UploadJournal {
    db: Arc<Db>,
}

impl UploadJournal {
    pub fn new<P: AsRef<Path>>(path: P) -> Result<Self> {
        let config = sled::Config::new()
            .path(path.as_ref().join("upload_journal"))
            .mode(sled::Mode::HighThroughput)
            .flush_every_ms(Some(100))
            // Disable compression in test/build environments where sled's compression
            // feature may not be enabled. Keep consistent with other local DB uses.
            .use_compression(false);

        let db = config
            .open()
            .context("Failed to open upload journal database")?;

        info!(
            "Upload journal opened at {:?}, entries: {}",
            path.as_ref(),
            db.len()
        );

        Ok(Self { db: Arc::new(db) })
    }

    pub fn record_pending(&self, task: &UploadTask, local_path: &str) -> Result<String> {
        let task_id = uuid::Uuid::new_v4().to_string();

        let entry = JournalEntry {
            task_id: task_id.clone(),
            ts_start: task.ts_start,
            ts_end: task.ts_end,
            packet_count: task.packet_count,
            tenant_id: task.tenant_id.clone(),
            probe_id: task.probe_id.clone(),
            local_path: Some(local_path.to_string()),
            s3_key: None,
            metadata_synced: false,
            created_at: chrono::Utc::now().timestamp_millis() as u64,
            retry_count: 0,
            last_error: None,
            s3_uploaded_at: None,
            metadata_synced_at: None,
        };

        let serialized = serde_json::to_vec(&entry).context("Failed to serialize journal entry")?;

        self.db
            .insert(task_id.as_bytes(), serialized)
            .context("Failed to insert journal entry")?;

        self.db.flush().context("Failed to flush journal")?;

        debug!("Recorded pending upload: task_id={}", task_id);

        Ok(task_id)
    }

    pub fn mark_s3_uploaded(&self, task_id: &str, s3_key: &str) -> Result<()> {
        if let Some(data) = self.db.get(task_id.as_bytes())? {
            let mut entry: JournalEntry =
                serde_json::from_slice(&data).context("Failed to deserialize journal entry")?;

            entry.s3_key = Some(s3_key.to_string());
            entry.s3_uploaded_at = Some(chrono::Utc::now().timestamp_millis() as u64);

            let serialized = serde_json::to_vec(&entry)?;
            self.db.insert(task_id.as_bytes(), serialized)?;
            self.db.flush()?;

            debug!("Marked S3 uploaded: task_id={}, s3_key={}", task_id, s3_key);
        } else {
            warn!("Task not found in journal: {}", task_id);
        }

        Ok(())
    }

    pub fn mark_metadata_synced(&self, task_id: &str) -> Result<()> {
        if let Some(data) = self.db.get(task_id.as_bytes())? {
            let mut entry: JournalEntry = serde_json::from_slice(&data)?;

            entry.metadata_synced = true;
            entry.metadata_synced_at = Some(chrono::Utc::now().timestamp_millis() as u64);

            if let Some(ref local_path) = entry.local_path {
                if let Err(e) = std::fs::remove_file(local_path) {
                    warn!("Failed to remove local cache file {}: {}", local_path, e);
                } else {
                    debug!("Removed local cache file: {}", local_path);
                }
            }

            self.db.remove(task_id.as_bytes())?;
            self.db.flush()?;

            debug!(
                "Marked metadata synced and removed from journal: task_id={}",
                task_id
            );
        } else {
            warn!("Task not found in journal: {}", task_id);
        }

        Ok(())
    }

    pub fn update_retry(&self, task_id: &str, error: &str) -> Result<()> {
        if let Some(data) = self.db.get(task_id.as_bytes())? {
            let mut entry: JournalEntry = serde_json::from_slice(&data)?;

            entry.retry_count += 1;
            entry.last_error = Some(error.to_string());

            let serialized = serde_json::to_vec(&entry)?;
            self.db.insert(task_id.as_bytes(), serialized)?;
            self.db.flush()?;

            debug!(
                "Updated retry count: task_id={}, retry_count={}",
                task_id, entry.retry_count
            );
        }

        Ok(())
    }

    pub fn recover_pending(&self) -> Vec<(String, JournalEntry)> {
        self.db
            .iter()
            .filter_map(|r| r.ok())
            .filter_map(|(k, v)| {
                let task_id = String::from_utf8(k.to_vec()).ok()?;
                let entry: JournalEntry = serde_json::from_slice(&v).ok()?;
                Some((task_id, entry))
            })
            .collect()
    }

    pub fn recover_needs_s3_upload(&self) -> Vec<(String, JournalEntry)> {
        self.recover_pending()
            .into_iter()
            .filter(|(_, entry)| entry.needs_s3_upload())
            .collect()
    }

    pub fn recover_needs_metadata_sync(&self) -> Vec<(String, JournalEntry)> {
        self.recover_pending()
            .into_iter()
            .filter(|(_, entry)| entry.needs_metadata_sync())
            .collect()
    }

    pub fn get_entry(&self, task_id: &str) -> Result<Option<JournalEntry>> {
        if let Some(data) = self.db.get(task_id.as_bytes())? {
            let entry: JournalEntry = serde_json::from_slice(&data)?;
            Ok(Some(entry))
        } else {
            Ok(None)
        }
    }

    pub fn remove_entry(&self, task_id: &str) -> Result<()> {
        self.db.remove(task_id.as_bytes())?;
        self.db.flush()?;
        debug!("Removed journal entry: task_id={}", task_id);
        Ok(())
    }

    pub fn size(&self) -> Result<usize> {
        Ok(self.db.len())
    }

    pub fn clear(&self) -> Result<()> {
        self.db.clear()?;
        self.db.flush()?;
        info!("Upload journal cleared");
        Ok(())
    }

    pub fn cleanup_old_entries(&self, max_age_hours: i64) -> Result<usize> {
        let now = chrono::Utc::now().timestamp_millis() as u64;
        let max_age_ms = (max_age_hours * 3600 * 1000) as u64;

        let mut removed = 0;

        let to_remove: Vec<String> = self
            .recover_pending()
            .into_iter()
            .filter(|(_, entry)| {
                let age_ms = now.saturating_sub(entry.created_at);
                age_ms > max_age_ms
            })
            .map(|(task_id, entry)| {
                if let Some(ref local_path) = entry.local_path {
                    std::fs::remove_file(local_path).ok();
                }
                task_id
            })
            .collect();

        for task_id in to_remove {
            if let Err(e) = self.remove_entry(&task_id) {
                warn!("Failed to remove old journal entry {}: {}", task_id, e);
            } else {
                removed += 1;
            }
        }

        if removed > 0 {
            info!(
                "Cleaned up {} old journal entries (> {}h)",
                removed, max_age_hours
            );
        }

        Ok(removed)
    }

    pub fn stats(&self) -> JournalStats {
        let entries = self.recover_pending();

        let total = entries.len();
        let needs_s3 = entries.iter().filter(|(_, e)| e.needs_s3_upload()).count();
        let needs_metadata = entries
            .iter()
            .filter(|(_, e)| e.needs_metadata_sync())
            .count();
        let complete = entries.iter().filter(|(_, e)| e.is_complete()).count();

        JournalStats {
            total_entries: total,
            needs_s3_upload: needs_s3,
            needs_metadata_sync: needs_metadata,
            complete_but_pending: complete,
        }
    }
}

impl Drop for UploadJournal {
    fn drop(&mut self) {
        if Arc::strong_count(&self.db) == 1 {
            if let Err(e) = self.db.flush() {
                error!("Failed to flush upload journal on drop: {}", e);
            } else {
                debug!("Upload journal flushed successfully on drop");
            }
        }
    }
}

#[derive(Debug, Clone)]
pub struct JournalStats {
    pub total_entries: usize,
    pub needs_s3_upload: usize,
    pub needs_metadata_sync: usize,
    pub complete_but_pending: usize,
}

impl std::fmt::Display for JournalStats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "UploadJournal: total={}, needs_s3={}, needs_metadata={}, complete_pending={}",
            self.total_entries,
            self.needs_s3_upload,
            self.needs_metadata_sync,
            self.complete_but_pending
        )
    }
}
