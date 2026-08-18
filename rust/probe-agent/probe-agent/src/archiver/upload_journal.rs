use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};
use sled::Db;
use std::os::unix::fs::MetadataExt;
use std::path::Path;
use std::sync::Arc;
use tracing::{debug, error, info};

use super::spool::JournaledUploadRef;
use super::uploader::ObjectWriteReceipt;
use super::UploadTask;

const JOURNAL_VERSION: u16 = 2;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum JournalObjectState {
    Pending,
    ObjectWritten,
    MetadataAccepted,
    CleanupAuthorized,
    Deleted,
    RetryWait,
    Quarantined,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CleanupClaim {
    pub claim_id: String,
    pub task_id: String,
    pub expected_revision: u64,
    pub claimed_revision: u64,
    pub canonical_path: String,
    #[serde(default)]
    pub deletion_path: String,
    pub device: u64,
    pub inode: u64,
    pub manifest_hash: String,
    pub authorized_at_ms: u64,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct CleanupReconcileReport {
    pub inspected: usize,
    pub resumable: usize,
    pub tombstones_recorded: usize,
}

impl Default for JournalObjectState {
    fn default() -> Self {
        Self::Pending
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct JournalEntry {
    #[serde(default)]
    pub journal_version: u16,
    #[serde(default)]
    pub object_state: JournalObjectState,
    #[serde(default)]
    pub object_write_receipt: Option<ObjectWriteReceipt>,
    #[serde(default)]
    pub quarantine_reason: Option<String>,
    #[serde(default)]
    pub retry_from_state: Option<JournalObjectState>,
    #[serde(default)]
    pub capture_uuid: String,
    #[serde(default)]
    pub manifest_hash: String,
    #[serde(default)]
    pub expected_object_key: String,
    #[serde(default)]
    pub revision: u64,
    #[serde(default)]
    pub cleanup_claim: Option<CleanupClaim>,
    #[serde(default)]
    pub deleted_at: Option<u64>,
    pub task_id: String,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packet_count: u64,
    #[serde(default)]
    pub original_size: usize,
    #[serde(default)]
    pub compressed_size: usize,
    #[serde(default)]
    pub sha256: String,
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
    /// Old entries had no explicit version/state. Preserve their evidence and
    /// upgrade their field combination deterministically when they are next
    /// written; never infer OBJECT_WRITTEN without at least the legacy key.
    pub fn effective_object_state(&self) -> JournalObjectState {
        if self.journal_version > 0 {
            return self.object_state;
        }
        if self.metadata_synced && self.s3_key.is_some() {
            JournalObjectState::MetadataAccepted
        } else if self.s3_key.is_some() {
            JournalObjectState::ObjectWritten
        } else {
            JournalObjectState::Pending
        }
    }

    pub fn is_complete(&self) -> bool {
        matches!(
            self.effective_object_state(),
            JournalObjectState::MetadataAccepted
                | JournalObjectState::CleanupAuthorized
                | JournalObjectState::Deleted
        )
    }

    pub fn is_s3_uploaded(&self) -> bool {
        matches!(
            self.effective_object_state(),
            JournalObjectState::ObjectWritten
                | JournalObjectState::MetadataAccepted
                | JournalObjectState::CleanupAuthorized
                | JournalObjectState::Deleted
        )
    }

    pub fn needs_s3_upload(&self) -> bool {
        self.effective_object_state() == JournalObjectState::Pending
    }

    pub fn needs_metadata_sync(&self) -> bool {
        self.effective_object_state() == JournalObjectState::ObjectWritten
    }

    pub fn has_complete_manifest(&self) -> bool {
        self.original_size > 0 && self.compressed_size > 0 && !self.sha256.is_empty()
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

    pub fn record_pending(
        &self,
        task: &UploadTask,
        local_path: &str,
        original_size: usize,
        compressed_size: usize,
        sha256: &str,
    ) -> Result<String> {
        let task_id = uuid::Uuid::new_v4().to_string();

        let entry = JournalEntry {
            journal_version: JOURNAL_VERSION,
            object_state: JournalObjectState::Pending,
            object_write_receipt: None,
            quarantine_reason: None,
            retry_from_state: None,
            capture_uuid: String::new(),
            manifest_hash: String::new(),
            expected_object_key: String::new(),
            revision: 1,
            cleanup_claim: None,
            deleted_at: None,
            task_id: task_id.clone(),
            ts_start: task.ts_start,
            ts_end: task.ts_end,
            packet_count: task.packet_count,
            original_size,
            compressed_size,
            sha256: sha256.to_string(),
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

    pub fn record_spooled_pending(&self, upload: &JournaledUploadRef) -> Result<String> {
        upload.validate_identity()?;
        let canonical_root = std::fs::canonicalize(&upload.spool_root).with_context(|| {
            format!(
                "failed to canonicalize spool root {}",
                upload.spool_root.display()
            )
        })?;
        let canonical_path = std::fs::canonicalize(&upload.local_path).with_context(|| {
            format!(
                "failed to canonicalize spool file {}",
                upload.local_path.display()
            )
        })?;
        if !canonical_path.starts_with(&canonical_root) {
            bail!(
                "spool path {} escapes canonical root {}",
                canonical_path.display(),
                canonical_root.display()
            );
        }
        let metadata = std::fs::symlink_metadata(&upload.local_path)?;
        if metadata.file_type().is_symlink() || !metadata.is_file() {
            bail!("spool path is not a regular owned file");
        }
        if metadata.len() != upload.stored_size {
            bail!("spool size differs from immutable upload reference");
        }

        let task_id = upload.task_id.clone();
        let entry = JournalEntry {
            journal_version: JOURNAL_VERSION,
            object_state: JournalObjectState::Pending,
            object_write_receipt: None,
            quarantine_reason: None,
            retry_from_state: None,
            capture_uuid: upload.capture_uuid.clone(),
            manifest_hash: upload.manifest_hash.clone(),
            expected_object_key: upload.object_key.clone(),
            revision: 1,
            cleanup_claim: None,
            deleted_at: None,
            task_id: task_id.clone(),
            ts_start: upload.ts_start,
            ts_end: upload.ts_end,
            packet_count: upload.packet_count,
            original_size: usize::try_from(upload.original_size)
                .context("original spool size exceeds platform usize")?,
            compressed_size: usize::try_from(upload.stored_size)
                .context("stored spool size exceeds platform usize")?,
            sha256: upload.sha256.clone(),
            tenant_id: upload.tenant_id.clone(),
            probe_id: upload.probe_id.clone(),
            local_path: Some(canonical_path.to_string_lossy().into_owned()),
            s3_key: None,
            metadata_synced: false,
            created_at: chrono::Utc::now().timestamp_millis() as u64,
            retry_count: 0,
            last_error: None,
            s3_uploaded_at: None,
            metadata_synced_at: None,
        };
        let serialized = serde_json::to_vec(&entry)?;
        match self
            .db
            .compare_and_swap(task_id.as_bytes(), None::<&[u8]>, Some(serialized))?
        {
            Ok(()) => {
                self.db.flush()?;
                Ok(task_id)
            }
            Err(conflict) => {
                let existing_bytes = conflict
                    .current
                    .context("journal CAS conflict did not return the existing value")?;
                let existing: JournalEntry = serde_json::from_slice(&existing_bytes)?;
                if pending_identity_matches(&existing, &entry) {
                    Ok(task_id)
                } else {
                    bail!("stable journal task ID conflicts with an existing manifest")
                }
            }
        }
    }

    pub fn mark_object_written(&self, task_id: &str, receipt: &ObjectWriteReceipt) -> Result<()> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("journal task not found: {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)
                .with_context(|| format!("failed to deserialize journal task {task_id}"))?;

            if let Err(reason) = validate_receipt(&entry, receipt) {
                entry.journal_version = JOURNAL_VERSION;
                entry.object_state = JournalObjectState::Quarantined;
                entry.quarantine_reason = Some(reason.to_string());
                entry.retry_from_state = None;
                entry.revision = entry.revision.max(1).saturating_add(1);
                let serialized = serde_json::to_vec(&entry)?;
                match self.db.compare_and_swap(
                    task_id.as_bytes(),
                    Some(old.as_ref()),
                    Some(serialized),
                )? {
                    Ok(()) => {
                        self.db.flush()?;
                        return Err(reason);
                    }
                    Err(_) => continue,
                }
            }

            match entry.effective_object_state() {
                JournalObjectState::Pending => {}
                JournalObjectState::ObjectWritten => {
                    if entry.object_write_receipt.as_ref() == Some(receipt) {
                        return Ok(());
                    }
                    // Legacy key-only entries are allowed to acquire their
                    // first trusted receipt. Any already persisted differing
                    // receipt is an identity conflict.
                    if entry.object_write_receipt.is_some() {
                        let reason = "conflicting object receipt for OBJECT_WRITTEN task";
                        entry.journal_version = JOURNAL_VERSION;
                        entry.object_state = JournalObjectState::Quarantined;
                        entry.quarantine_reason = Some(reason.to_string());
                        entry.retry_from_state = None;
                        entry.revision = entry.revision.max(1).saturating_add(1);
                        let serialized = serde_json::to_vec(&entry)?;
                        match self.db.compare_and_swap(
                            task_id.as_bytes(),
                            Some(old.as_ref()),
                            Some(serialized),
                        )? {
                            Ok(()) => {
                                self.db.flush()?;
                                bail!(reason);
                            }
                            Err(_) => continue,
                        }
                    }
                }
                state => bail!(
                    "journal task {task_id} cannot transition from {state:?} to OBJECT_WRITTEN"
                ),
            }

            entry.journal_version = JOURNAL_VERSION;
            entry.object_state = JournalObjectState::ObjectWritten;
            entry.object_write_receipt = Some(receipt.clone());
            entry.retry_from_state = None;
            entry.s3_key = Some(receipt.key.clone());
            entry.s3_uploaded_at = Some(chrono::Utc::now().timestamp_millis() as u64);
            entry.last_error = None;
            entry.revision = entry.revision.max(1).saturating_add(1);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    debug!(
                        "Durably marked OBJECT_WRITTEN: task_id={}, key={}",
                        task_id, receipt.key
                    );
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn mark_metadata_accepted(&self, task_id: &str) -> Result<()> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("journal task not found: {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            match entry.effective_object_state() {
                JournalObjectState::MetadataAccepted => return Ok(()),
                JournalObjectState::ObjectWritten if entry.object_write_receipt.is_some() => {}
                JournalObjectState::ObjectWritten => {
                    bail!("journal task {task_id} has no durable object receipt")
                }
                state => bail!(
                    "journal task {task_id} cannot transition from {state:?} to METADATA_ACCEPTED"
                ),
            }

            entry.journal_version = JOURNAL_VERSION;
            entry.object_state = JournalObjectState::MetadataAccepted;
            entry.retry_from_state = None;
            entry.metadata_synced = true;
            entry.metadata_synced_at = Some(chrono::Utc::now().timestamp_millis() as u64);
            entry.last_error = None;
            entry.revision = entry.revision.max(1).saturating_add(1);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    debug!(
                        "Durably marked METADATA_ACCEPTED; evidence retained: task_id={}",
                        task_id
                    );
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn update_retry(&self, task_id: &str, error: &str) -> Result<()> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("cannot record retry for missing task {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            if matches!(
                entry.effective_object_state(),
                JournalObjectState::Deleted | JournalObjectState::Quarantined
            ) {
                bail!(
                    "cannot record retry for terminal task {task_id} in state {:?}",
                    entry.effective_object_state()
                );
            }
            entry.retry_count = entry.retry_count.saturating_add(1);
            entry.last_error = Some(error.to_string());
            entry.revision = entry.revision.max(1).saturating_add(1);

            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    debug!(
                        "Updated retry count: task_id={}, retry_count={}",
                        task_id, entry.retry_count
                    );
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn mark_retry_wait(&self, task_id: &str, error: &str) -> Result<()> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("cannot defer missing task {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            let retry_from_state = match entry.effective_object_state() {
                JournalObjectState::Pending => JournalObjectState::Pending,
                JournalObjectState::ObjectWritten => JournalObjectState::ObjectWritten,
                JournalObjectState::RetryWait => entry
                    .retry_from_state
                    .context("RETRY_WAIT task has no durable resume state")?,
                state => bail!("cannot defer terminal task {task_id} from state {state:?}"),
            };
            if !matches!(
                retry_from_state,
                JournalObjectState::Pending | JournalObjectState::ObjectWritten
            ) {
                bail!("task {task_id} has invalid retry resume state {retry_from_state:?}");
            }
            entry.journal_version = JOURNAL_VERSION;
            entry.object_state = JournalObjectState::RetryWait;
            entry.retry_from_state = Some(retry_from_state);
            entry.retry_count = entry.retry_count.saturating_add(1);
            entry.last_error = Some(error.to_string());
            entry.revision = entry.revision.max(1).saturating_add(1);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn resume_retry(&self, task_id: &str) -> Result<JournalObjectState> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("cannot resume missing task {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            if entry.effective_object_state() != JournalObjectState::RetryWait {
                bail!(
                    "cannot resume task {task_id} from state {:?}",
                    entry.effective_object_state()
                );
            }
            let target = entry
                .retry_from_state
                .context("RETRY_WAIT task has no durable resume state")?;
            if !matches!(
                target,
                JournalObjectState::Pending | JournalObjectState::ObjectWritten
            ) {
                bail!("task {task_id} has invalid retry resume state {target:?}");
            }
            if target == JournalObjectState::ObjectWritten && entry.object_write_receipt.is_none() {
                bail!("OBJECT_WRITTEN retry task {task_id} has no durable object receipt");
            }
            entry.object_state = target;
            entry.retry_from_state = None;
            entry.revision = entry.revision.max(1).saturating_add(1);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    return Ok(target);
                }
                Err(_) => continue,
            }
        }
    }

    pub fn recover_pending(&self) -> Vec<(String, JournalEntry)> {
        self.recover_by_state(usize::MAX).unwrap_or_else(|error| {
            error!("Failed to enumerate upload journal: {}", error);
            Vec::new()
        })
    }

    pub fn recover_by_state(&self, limit: usize) -> Result<Vec<(String, JournalEntry)>> {
        let mut entries = Vec::new();
        for item in self.db.iter() {
            let (key, value) = item.context("failed to read upload journal record")?;
            let task_id = String::from_utf8(key.to_vec())
                .context("upload journal contains a non-UTF-8 task ID")?;
            let entry: JournalEntry = serde_json::from_slice(&value)
                .with_context(|| format!("upload journal task {task_id} is corrupt"))?;
            if entry.task_id != task_id {
                bail!(
                    "upload journal key/task identity mismatch: key={task_id}, value={}",
                    entry.task_id
                );
            }
            entries.push((task_id, entry));
        }
        entries.sort_by(|left, right| left.0.cmp(&right.0));
        entries.truncate(limit);
        Ok(entries)
    }

    pub fn quarantine(&self, task_id: &str, reason: &str) -> Result<()> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("journal task not found: {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            if entry.effective_object_state() == JournalObjectState::Quarantined
                && entry.quarantine_reason.as_deref() == Some(reason)
            {
                return Ok(());
            }
            entry.journal_version = JOURNAL_VERSION;
            entry.object_state = JournalObjectState::Quarantined;
            entry.quarantine_reason = Some(reason.to_string());
            entry.retry_from_state = None;
            entry.last_error = Some(reason.to_string());
            entry.revision = entry.revision.max(1).saturating_add(1);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn recover_needs_s3_upload(&self) -> Vec<(String, JournalEntry)> {
        self.recover_pending()
            .into_iter()
            .filter(|(_, entry)| entry.needs_s3_upload())
            .collect()
    }

    pub fn claim_cleanup_authority(
        &self,
        task_id: &str,
        expected_revision: u64,
        canonical_spool_root: &Path,
    ) -> Result<CleanupClaim> {
        loop {
            let old = self
                .db
                .get(task_id.as_bytes())?
                .with_context(|| format!("journal task not found: {task_id}"))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            let revision = entry.revision.max(1);
            if revision != expected_revision {
                bail!(
                    "cleanup claim revision conflict: expected {expected_revision}, observed {revision}"
                );
            }
            if entry.effective_object_state() != JournalObjectState::MetadataAccepted {
                bail!(
                    "cleanup can only be claimed from METADATA_ACCEPTED, observed {:?}",
                    entry.effective_object_state()
                );
            }
            if entry.object_write_receipt.is_none() || !entry.metadata_synced {
                bail!("cleanup authority requires durable object and metadata receipts");
            }
            let local_path = entry
                .local_path
                .as_deref()
                .context("cleanup candidate has no local path")?;
            let root = std::fs::canonicalize(canonical_spool_root)?;
            let path = std::fs::canonicalize(local_path)?;
            let symlink_metadata = std::fs::symlink_metadata(local_path)?;
            if !path.starts_with(&root)
                || symlink_metadata.file_type().is_symlink()
                || !symlink_metadata.is_file()
            {
                bail!("cleanup candidate is not a regular file beneath the canonical spool root");
            }
            let claimed_revision = revision.saturating_add(1);
            let claim_id = uuid::Uuid::new_v5(
                &uuid::Uuid::NAMESPACE_OID,
                format!(
                    "pcap-cleanup\0{task_id}\0{claimed_revision}\0{}\0{}\0{}",
                    path.display(),
                    symlink_metadata.dev(),
                    symlink_metadata.ino()
                )
                .as_bytes(),
            )
            .to_string();
            let deletion_path = path
                .parent()
                .context("cleanup candidate has no parent directory")?
                .join(format!(".cleanup-{claim_id}"));
            let claim = CleanupClaim {
                claim_id,
                task_id: task_id.to_string(),
                expected_revision,
                claimed_revision,
                canonical_path: path.to_string_lossy().into_owned(),
                deletion_path: deletion_path.to_string_lossy().into_owned(),
                device: symlink_metadata.dev(),
                inode: symlink_metadata.ino(),
                manifest_hash: entry.manifest_hash.clone(),
                authorized_at_ms: chrono::Utc::now().timestamp_millis() as u64,
            };
            entry.journal_version = JOURNAL_VERSION;
            entry.object_state = JournalObjectState::CleanupAuthorized;
            entry.revision = claimed_revision;
            entry.cleanup_claim = Some(claim.clone());
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    return Ok(claim);
                }
                Err(_) => continue,
            }
        }
    }

    pub fn record_deleted(&self, claim: &CleanupClaim) -> Result<()> {
        loop {
            let old = self
                .db
                .get(claim.task_id.as_bytes())?
                .with_context(|| format!("journal task not found: {}", claim.task_id))?;
            let mut entry: JournalEntry = serde_json::from_slice(&old)?;
            if entry.effective_object_state() == JournalObjectState::Deleted
                && entry.cleanup_claim.as_ref() == Some(claim)
            {
                return Ok(());
            }
            if entry.effective_object_state() != JournalObjectState::CleanupAuthorized
                || entry.revision != claim.claimed_revision
                || entry.cleanup_claim.as_ref() != Some(claim)
            {
                bail!("deleted tombstone does not match the durable cleanup claim");
            }
            if Path::new(&claim.canonical_path).exists() || Path::new(&claim.deletion_path).exists()
            {
                bail!("cannot record DELETED while claimed evidence still exists");
            }
            entry.object_state = JournalObjectState::Deleted;
            entry.revision = entry.revision.saturating_add(1);
            entry.deleted_at = Some(chrono::Utc::now().timestamp_millis() as u64);
            let serialized = serde_json::to_vec(&entry)?;
            match self.db.compare_and_swap(
                claim.task_id.as_bytes(),
                Some(old.as_ref()),
                Some(serialized),
            )? {
                Ok(()) => {
                    self.db.flush()?;
                    return Ok(());
                }
                Err(_) => continue,
            }
        }
    }

    pub fn reconcile_cleanup_interruption(&self) -> Result<CleanupReconcileReport> {
        let mut report = CleanupReconcileReport::default();
        for (task_id, entry) in self.recover_by_state(usize::MAX)? {
            report.inspected += 1;
            match entry.effective_object_state() {
                JournalObjectState::CleanupAuthorized => {
                    let claim = entry.cleanup_claim.as_ref().with_context(|| {
                        format!("CLEANUP_AUTHORIZED task {task_id} has no cleanup claim")
                    })?;
                    if entry.revision != claim.claimed_revision || claim.task_id != task_id {
                        bail!("CLEANUP_AUTHORIZED task {task_id} has a stale or foreign claim");
                    }
                    let original = Path::new(&claim.canonical_path);
                    let deletion = Path::new(&claim.deletion_path);
                    if original.exists() && deletion.exists() {
                        bail!(
                            "cleanup interruption for task {task_id} has both source and deletion paths"
                        );
                    }
                    let path = if original.exists() {
                        Some(original)
                    } else if deletion.exists() {
                        Some(deletion)
                    } else {
                        None
                    };
                    if let Some(path) = path {
                        let metadata = std::fs::symlink_metadata(path)?;
                        if metadata.file_type().is_symlink()
                            || !metadata.is_file()
                            || metadata.dev() != claim.device
                            || metadata.ino() != claim.inode
                        {
                            bail!(
                                "cleanup interruption for task {task_id} has a path identity mismatch"
                            );
                        }
                        report.resumable += 1;
                    } else {
                        self.record_deleted(claim)?;
                        report.tombstones_recorded += 1;
                    }
                }
                JournalObjectState::Pending
                | JournalObjectState::ObjectWritten
                | JournalObjectState::MetadataAccepted
                | JournalObjectState::RetryWait
                | JournalObjectState::Quarantined => {
                    if let Some(local_path) = entry.local_path.as_deref() {
                        if !Path::new(local_path).exists() {
                            bail!(
                                "journal task {task_id} lost local evidence without cleanup authority"
                            );
                        }
                    }
                }
                JournalObjectState::Deleted => {
                    let claim = entry.cleanup_claim.as_ref().with_context(|| {
                        format!("DELETED task {task_id} has no preserved cleanup claim")
                    })?;
                    if Path::new(&claim.canonical_path).exists() {
                        bail!("DELETED task {task_id} has reappeared local evidence");
                    }
                }
            }
        }
        Ok(report)
    }

    pub fn recover_needs_metadata_sync(&self) -> Vec<(String, JournalEntry)> {
        self.recover_pending()
            .into_iter()
            .filter(|(_, entry)| entry.needs_metadata_sync())
            .collect()
    }

    pub fn cleanup_claims(&self) -> Result<Vec<CleanupClaim>> {
        let mut claims = Vec::new();
        for (task_id, entry) in self.recover_by_state(usize::MAX)? {
            if entry.effective_object_state() == JournalObjectState::CleanupAuthorized {
                let claim = entry.cleanup_claim.with_context(|| {
                    format!("CLEANUP_AUTHORIZED task {task_id} has no durable claim")
                })?;
                if claim.task_id != task_id || claim.claimed_revision != entry.revision {
                    bail!("CLEANUP_AUTHORIZED task {task_id} has a stale or foreign claim");
                }
                claims.push(claim);
            }
        }
        claims.sort_by(|left, right| left.task_id.cmp(&right.task_id));
        Ok(claims)
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
        let _ = task_id;
        bail!("journal evidence removal is disabled; retain DELETED tombstones")
    }

    pub fn size(&self) -> Result<usize> {
        Ok(self.db.len())
    }

    pub fn clear(&self) -> Result<()> {
        bail!("journal clearing is disabled; retain durable recovery evidence")
    }

    pub fn cleanup_old_entries(&self, max_age_hours: i64) -> Result<usize> {
        // Reclaim only DELETED tombstones older than `max_age_hours`; all
        // active recovery evidence (pending/object-written/metadata/retry/
        // quarantine) is never removed. This bounds long-term journal growth
        // while preserving the audit trail in the form of the retained
        // DELETED entries' cleanup claims until they age out.
        if max_age_hours <= 0 {
            bail!("journal tombstone cleanup requires a positive max_age_hours");
        }
        let cutoff_ms = chrono::Utc::now().timestamp_millis()
            - (max_age_hours as i64).saturating_mul(3600 * 1000);
        let mut batch = sled::Batch::default();
        let mut reclaimed = 0usize;
        for item in self.db.iter() {
            let (key, value) = item?;
            let entry: JournalEntry = match serde_json::from_slice(&value) {
                Ok(entry) => entry,
                Err(_) => continue,
            };
            let deleted_at_ms = entry.deleted_at.unwrap_or(0);
            if entry.effective_object_state() == JournalObjectState::Deleted
                && deleted_at_ms > 0
                && (deleted_at_ms as i64) < cutoff_ms
            {
                batch.remove(key);
                reclaimed += 1;
            }
        }
        if reclaimed > 0 {
            self.db.apply_batch(batch)?;
            self.db.flush()?;
        }
        Ok(reclaimed)
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

fn validate_receipt(entry: &JournalEntry, receipt: &ObjectWriteReceipt) -> Result<()> {
    if !entry.has_complete_manifest() {
        bail!("journal task has no complete size/hash manifest");
    }
    if receipt.bucket.trim().is_empty()
        || receipt.key.trim().is_empty()
        || receipt.etag.trim().is_empty()
        || receipt.sha256.len() != 64
        || !receipt.sha256.bytes().all(|byte| byte.is_ascii_hexdigit())
    {
        bail!("object receipt is incomplete");
    }
    if receipt.stored_size != entry.compressed_size as u64 {
        bail!(
            "object receipt size conflicts with journal manifest: expected {}, got {}",
            entry.compressed_size,
            receipt.stored_size
        );
    }
    if receipt.sha256 != entry.sha256 {
        bail!("object receipt sha256 conflicts with journal manifest");
    }
    let expected_prefix = format!("{}/{}/", entry.tenant_id, entry.probe_id);
    if !receipt.key.starts_with(&expected_prefix) {
        bail!("object receipt key conflicts with tenant/probe identity");
    }
    if let Some(existing_key) = entry.s3_key.as_deref() {
        if existing_key != receipt.key {
            bail!("object receipt key conflicts with persisted key");
        }
    }
    if !entry.expected_object_key.is_empty() && entry.expected_object_key != receipt.key {
        bail!("object receipt key conflicts with immutable spool identity");
    }
    Ok(())
}

fn pending_identity_matches(left: &JournalEntry, right: &JournalEntry) -> bool {
    left.task_id == right.task_id
        && left.capture_uuid == right.capture_uuid
        && left.manifest_hash == right.manifest_hash
        && left.expected_object_key == right.expected_object_key
        && left.ts_start == right.ts_start
        && left.ts_end == right.ts_end
        && left.packet_count == right.packet_count
        && left.original_size == right.original_size
        && left.compressed_size == right.compressed_size
        && left.sha256 == right.sha256
        && left.tenant_id == right.tenant_id
        && left.probe_id == right.probe_id
        && left.local_path == right.local_path
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

#[cfg(test)]
mod tests {
    use super::{JournalObjectState, ObjectWriteReceipt, UploadJournal, JOURNAL_VERSION};
    use crate::archiver::UploadTask;

    fn receipt(etag: &str) -> ObjectWriteReceipt {
        ObjectWriteReceipt {
            bucket: "pcap-archive".to_string(),
            key: "tenant-a/probe-a/1970-01-01/000001-000002-1.pcap.zst".to_string(),
            version_id: "version-1".to_string(),
            etag: etag.to_string(),
            stored_size: 15,
            sha256: "a".repeat(64),
        }
    }

    fn pending_fixture() -> (tempfile::TempDir, UploadJournal, String, std::path::PathBuf) {
        let directory = tempfile::tempdir().expect("tempdir");
        let local_path = directory.path().join("capture.pcap.zst");
        std::fs::write(&local_path, b"compressed-pcap").expect("local evidence");
        let journal = UploadJournal::new(directory.path()).expect("journal");
        let task = UploadTask {
            data: vec![1, 2, 3],
            ts_start: 1_000_000,
            ts_end: 2_000_000,
            packet_count: 1,
            tenant_id: "tenant-a".to_string(),
            probe_id: "probe-a".to_string(),
        };
        let task_id = journal
            .record_pending(
                &task,
                local_path.to_str().expect("path"),
                3,
                15,
                &"a".repeat(64),
            )
            .expect("pending");
        (directory, journal, task_id, local_path)
    }

    #[test]
    fn durable_metadata_ack_retains_local_evidence_and_complete_manifest() {
        let (_directory, journal, task_id, local_path) = pending_fixture();

        journal
            .mark_object_written(&task_id, &receipt("etag-1"))
            .expect("object receipt");
        journal
            .mark_metadata_accepted(&task_id)
            .expect("metadata ack");

        let entry = journal
            .get_entry(&task_id)
            .expect("entry lookup")
            .expect("retained journal entry");
        assert!(entry.metadata_synced);
        assert_eq!(
            entry.effective_object_state(),
            JournalObjectState::MetadataAccepted
        );
        assert_eq!(entry.object_write_receipt, Some(receipt("etag-1")));
        assert!(entry.has_complete_manifest());
        assert!(
            local_path.exists(),
            "ACK must not delete the local evidence"
        );
    }

    #[test]
    fn object_receipt_transition_is_idempotent_and_conflict_quarantines() {
        let (_directory, journal, task_id, _local_path) = pending_fixture();
        let first = receipt("etag-1");
        journal
            .mark_object_written(&task_id, &first)
            .expect("first receipt");
        journal
            .mark_object_written(&task_id, &first)
            .expect("same receipt is idempotent");

        let error = journal
            .mark_object_written(&task_id, &receipt("etag-conflict"))
            .expect_err("different receipt must conflict");
        assert!(error.to_string().contains("conflicting object receipt"));
        let entry = journal.get_entry(&task_id).expect("lookup").expect("entry");
        assert_eq!(
            entry.effective_object_state(),
            JournalObjectState::Quarantined
        );
        assert_eq!(entry.object_write_receipt, Some(first));
    }

    #[test]
    fn missing_task_and_metadata_without_receipt_fail_closed() {
        let (_directory, journal, task_id, _local_path) = pending_fixture();
        let missing = journal
            .mark_object_written("missing-task", &receipt("etag-1"))
            .expect_err("missing task must not be warn plus success");
        assert!(missing.to_string().contains("journal task not found"));

        let error = journal
            .mark_metadata_accepted(&task_id)
            .expect_err("PENDING cannot skip object receipt");
        assert!(error.to_string().contains("cannot transition"));
        assert_eq!(
            journal
                .get_entry(&task_id)
                .expect("lookup")
                .expect("entry")
                .effective_object_state(),
            JournalObjectState::Pending
        );
    }

    #[test]
    fn retry_wait_resumes_the_exact_durable_phase_without_regression() {
        let (_directory, journal, task_id, _local_path) = pending_fixture();
        journal
            .mark_retry_wait(&task_id, "object transport unknown")
            .expect("pending retry wait");
        let waiting = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("waiting entry");
        assert_eq!(
            waiting.effective_object_state(),
            JournalObjectState::RetryWait
        );
        assert_eq!(waiting.retry_from_state, Some(JournalObjectState::Pending));
        assert_eq!(
            journal.resume_retry(&task_id).expect("resume pending"),
            JournalObjectState::Pending
        );

        journal
            .mark_object_written(&task_id, &receipt("etag-1"))
            .expect("object receipt");
        journal
            .mark_retry_wait(&task_id, "metadata unavailable")
            .expect("metadata retry wait");
        assert_eq!(
            journal.resume_retry(&task_id).expect("resume metadata"),
            JournalObjectState::ObjectWritten
        );
        let resumed = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("resumed entry");
        assert_eq!(resumed.object_write_receipt, Some(receipt("etag-1")));
        assert_eq!(resumed.retry_from_state, None);
    }

    #[test]
    fn legacy_key_only_entry_upgrades_deterministically_with_trusted_receipt() {
        let (_directory, journal, task_id, _local_path) = pending_fixture();
        let mut value =
            serde_json::to_value(journal.get_entry(&task_id).expect("lookup").expect("entry"))
                .expect("serialize");
        let object = value.as_object_mut().expect("object");
        object.remove("journal_version");
        object.remove("object_state");
        object.remove("object_write_receipt");
        object.remove("quarantine_reason");
        object.insert(
            "s3_key".to_string(),
            serde_json::Value::String(receipt("etag-1").key),
        );
        journal
            .db
            .insert(
                task_id.as_bytes(),
                serde_json::to_vec(&value).expect("legacy bytes"),
            )
            .expect("insert legacy");
        journal.db.flush().expect("flush legacy");

        let legacy = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("legacy entry");
        assert_eq!(legacy.journal_version, 0);
        assert_eq!(
            legacy.effective_object_state(),
            JournalObjectState::ObjectWritten
        );

        journal
            .mark_object_written(&task_id, &receipt("etag-1"))
            .expect("upgrade trusted receipt");
        let upgraded = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("upgraded");
        assert_eq!(upgraded.journal_version, JOURNAL_VERSION);
        assert_eq!(upgraded.object_write_receipt, Some(receipt("etag-1")));
    }

    #[test]
    fn cleanup_claim_requires_exact_revision_and_missing_without_claim_blocks_startup() {
        let (directory, journal, task_id, local_path) = pending_fixture();
        journal
            .mark_object_written(&task_id, &receipt("etag-1"))
            .expect("object");
        journal.mark_metadata_accepted(&task_id).expect("metadata");
        let accepted = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("accepted");
        let stale = journal
            .claim_cleanup_authority(&task_id, accepted.revision - 1, directory.path())
            .expect_err("stale revision");
        assert!(stale.to_string().contains("revision conflict"));

        std::fs::remove_file(&local_path).expect("inject unexplained loss");
        let error = journal
            .reconcile_cleanup_interruption()
            .expect_err("missing evidence without claim blocks admission");
        assert!(error.to_string().contains("without cleanup authority"));
    }

    #[test]
    fn authorized_missing_file_reconciles_to_deleted_tombstone() {
        let (directory, journal, task_id, local_path) = pending_fixture();
        journal
            .mark_object_written(&task_id, &receipt("etag-1"))
            .expect("object");
        journal.mark_metadata_accepted(&task_id).expect("metadata");
        let accepted = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("accepted");
        let claim = journal
            .claim_cleanup_authority(&task_id, accepted.revision, directory.path())
            .expect("claim");
        std::fs::remove_file(&local_path).expect("simulate unlink before tombstone");

        let report = journal
            .reconcile_cleanup_interruption()
            .expect("reconcile exact claim");
        assert_eq!(report.tombstones_recorded, 1);
        let deleted = journal
            .get_entry(&task_id)
            .expect("lookup")
            .expect("deleted");
        assert_eq!(
            deleted.effective_object_state(),
            JournalObjectState::Deleted
        );
        assert_eq!(deleted.cleanup_claim, Some(claim));
        assert!(deleted.deleted_at.is_some());
    }
}
