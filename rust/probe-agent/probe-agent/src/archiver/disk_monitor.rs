use anyhow::{bail, Context, Result};
use std::os::unix::fs::MetadataExt;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use tokio::time::{interval, Duration};
use tracing::{debug, error, info, warn};

use super::upload_journal::{CleanupClaim, UploadJournal};

pub trait PcapCleanupAuthority: Send + Sync {
    fn cleanup_claims(&self) -> Result<Vec<CleanupClaim>>;
    fn record_deleted(&self, claim: &CleanupClaim) -> Result<()>;
}

impl PcapCleanupAuthority for UploadJournal {
    fn cleanup_claims(&self) -> Result<Vec<CleanupClaim>> {
        UploadJournal::cleanup_claims(self)
    }

    fn record_deleted(&self, claim: &CleanupClaim) -> Result<()> {
        UploadJournal::record_deleted(self, claim)
    }
}

#[derive(Clone, Debug)]
pub struct DiskMonitorConfig {
    pub path: String,

    pub check_interval: Duration,

    pub warning_threshold_percent: f64,

    pub critical_threshold_percent: f64,

    pub auto_cleanup: bool,

    pub min_free_bytes: u64,

    pub cleanup_target_percent: f64,
    pub min_cleanup_interval: Duration,
}

impl Default for DiskMonitorConfig {
    fn default() -> Self {
        Self {
            path: "/var/lib/probe-agent/cache".to_string(),
            check_interval: Duration::from_secs(60),
            warning_threshold_percent: 80.0,
            critical_threshold_percent: 90.0,
            auto_cleanup: true,
            min_free_bytes: 10 * 1024 * 1024 * 1024,
            cleanup_target_percent: 70.0,
            min_cleanup_interval: Duration::from_secs(300),
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DiskStatus {
    Normal,

    Warning,

    Critical,

    Full,
}

impl std::fmt::Display for DiskStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DiskStatus::Normal => write!(f, "Normal"),
            DiskStatus::Warning => write!(f, "Warning"),
            DiskStatus::Critical => write!(f, "Critical"),
            DiskStatus::Full => write!(f, "Full"),
        }
    }
}

#[derive(Debug, Clone)]
pub struct DiskStats {
    pub total_bytes: u64,

    pub used_bytes: u64,

    pub free_bytes: u64,

    pub usage_percent: f64,

    pub status: DiskStatus,

    pub timestamp: std::time::Instant,
}

impl DiskStats {
    pub fn empty() -> Self {
        Self {
            total_bytes: 0,
            used_bytes: 0,
            free_bytes: 0,
            usage_percent: 0.0,
            status: DiskStatus::Normal,
            timestamp: std::time::Instant::now(),
        }
    }

    pub fn needs_cleanup(&self) -> bool {
        matches!(self.status, DiskStatus::Critical | DiskStatus::Full)
    }

    pub fn format_bytes(bytes: u64) -> String {
        const KB: u64 = 1024;
        const MB: u64 = KB * 1024;
        const GB: u64 = MB * 1024;
        const TB: u64 = GB * 1024;

        if bytes >= TB {
            format!("{:.2} TB", bytes as f64 / TB as f64)
        } else if bytes >= GB {
            format!("{:.2} GB", bytes as f64 / GB as f64)
        } else if bytes >= MB {
            format!("{:.2} MB", bytes as f64 / MB as f64)
        } else if bytes >= KB {
            format!("{:.2} KB", bytes as f64 / KB as f64)
        } else {
            format!("{} B", bytes)
        }
    }

    pub fn summary(&self) -> String {
        format!(
            "Disk: {} / {} used ({:.1}%), {} free, status: {}",
            Self::format_bytes(self.used_bytes),
            Self::format_bytes(self.total_bytes),
            self.usage_percent,
            Self::format_bytes(self.free_bytes),
            self.status
        )
    }
}

pub struct DiskMonitor {
    config: DiskMonitorConfig,
    running: Arc<AtomicBool>,

    latest_stats: Arc<tokio::sync::RwLock<DiskStats>>,
    /// Set once the monitor has performed at least one real disk measurement;
    /// the target-watermark stop in cleanup only applies after a measurement.
    measured: AtomicBool,

    warning_count: AtomicU64,

    critical_count: AtomicU64,

    last_cleanup: Arc<tokio::sync::Mutex<Option<std::time::Instant>>>,
    cleanup_authority: Option<Arc<dyn PcapCleanupAuthority>>,
    canonical_spool_root: Option<CanonicalSpoolRoot>,
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct PcapCleanupReport {
    pub claims_inspected: usize,
    pub files_deleted: usize,
    pub tombstones_written: usize,
}

#[derive(Debug, Clone)]
pub struct CanonicalSpoolRoot {
    path: PathBuf,
}

impl CanonicalSpoolRoot {
    pub fn new(path: impl AsRef<Path>) -> Result<Self> {
        let path = std::fs::canonicalize(path.as_ref()).with_context(|| {
            format!(
                "failed to canonicalize cleanup spool root {}",
                path.as_ref().display()
            )
        })?;
        Ok(Self { path })
    }

    pub async fn unlink_claimed(&self, claim: &CleanupClaim) -> Result<()> {
        let original = PathBuf::from(&claim.canonical_path);
        let deletion = PathBuf::from(&claim.deletion_path);
        if !original.starts_with(&self.path)
            || !deletion.starts_with(&self.path)
            || original.parent() != deletion.parent()
        {
            bail!("cleanup claim path is outside the canonical spool root");
        }
        let parent = original
            .parent()
            .context("cleanup claim path has no parent directory")?;
        let canonical_parent = std::fs::canonicalize(parent)?;
        if !canonical_parent.starts_with(&self.path) || canonical_parent != parent {
            bail!("cleanup claim parent is not canonical beneath the spool root");
        }
        if original.exists() && deletion.exists() {
            bail!("cleanup claim has both source and deletion paths");
        }
        if original.exists() {
            verify_claimed_inode(&original, claim)?;
            tokio::fs::rename(&original, &deletion)
                .await
                .context("failed to atomically claim cleanup path")?;
            sync_directory(parent).await?;
        }
        if deletion.exists() {
            verify_claimed_inode(&deletion, claim)?;
            tokio::fs::remove_file(&deletion)
                .await
                .context("failed to unlink claimed PCAP evidence")?;
            sync_directory(parent).await?;
        }
        Ok(())
    }
}

impl DiskMonitor {
    pub fn new(config: DiskMonitorConfig) -> Self {
        info!(
            "DiskMonitor created: path={}, warning={}%, critical={}%",
            config.path, config.warning_threshold_percent, config.critical_threshold_percent
        );

        Self {
            config,
            running: Arc::new(AtomicBool::new(false)),
            latest_stats: Arc::new(tokio::sync::RwLock::new(DiskStats::empty())),
            measured: AtomicBool::new(false),
            warning_count: AtomicU64::new(0),
            critical_count: AtomicU64::new(0),
            last_cleanup: Arc::new(tokio::sync::Mutex::new(None)),
            cleanup_authority: None,
            canonical_spool_root: None,
        }
    }

    pub fn with_pcap_cleanup_authority(
        config: DiskMonitorConfig,
        cleanup_authority: Arc<dyn PcapCleanupAuthority>,
        spool_root: impl AsRef<Path>,
    ) -> Result<Self> {
        let mut monitor = Self::new(config);
        monitor.cleanup_authority = Some(cleanup_authority);
        monitor.canonical_spool_root = Some(CanonicalSpoolRoot::new(spool_root)?);
        Ok(monitor)
    }

    pub async fn start(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        self.running.store(true, Ordering::Release);

        info!(
            "Starting disk monitoring: path={}, interval={}s",
            self.config.path,
            self.config.check_interval.as_secs()
        );

        let monitor = Arc::clone(&self);

        tokio::spawn(async move {
            monitor.run().await;
        })
    }

    pub fn stop(&self) {
        self.running.store(false, Ordering::Release);
        info!("Disk monitoring stopped");
    }

    pub async fn run(&self) {
        let mut ticker = interval(self.config.check_interval);
        let mut iteration = 0u64;

        while self.running.load(Ordering::Acquire) {
            ticker.tick().await;
            iteration += 1;

            match self.check_disk_space().await {
                Ok(stats) => {
                    self.measured.store(true, Ordering::Release);
                    *self.latest_stats.write().await = stats.clone();

                    if iteration % 10 == 0 {
                        debug!("{}", stats.summary());
                    }

                    match stats.status {
                        DiskStatus::Normal => {}
                        DiskStatus::Warning => {
                            self.warning_count.fetch_add(1, Ordering::Relaxed);
                            warn!(
                                "⚠ Disk usage warning: {:.1}% used ({} / {})",
                                stats.usage_percent,
                                DiskStats::format_bytes(stats.used_bytes),
                                DiskStats::format_bytes(stats.total_bytes)
                            );
                        }
                        DiskStatus::Critical => {
                            self.critical_count.fetch_add(1, Ordering::Relaxed);
                            error!(
                                "🔴 Disk usage critical: {:.1}% used ({} free)",
                                stats.usage_percent,
                                DiskStats::format_bytes(stats.free_bytes)
                            );

                            if self.config.auto_cleanup {
                                self.trigger_cleanup().await;
                            }
                        }
                        DiskStatus::Full => {
                            error!(
                                "🔴 Disk FULL: {:.1}% used, only {} free!",
                                stats.usage_percent,
                                DiskStats::format_bytes(stats.free_bytes)
                            );

                            self.trigger_cleanup().await;
                        }
                    }
                }
                Err(e) => {
                    error!("Failed to check disk space: {}", e);
                }
            }
        }

        info!("Disk monitor loop exited");
    }

    async fn check_disk_space(&self) -> Result<DiskStats> {
        let path = Path::new(&self.config.path);

        if !path.exists() {
            tokio::fs::create_dir_all(path)
                .await
                .context("Failed to create directory")?;
        }

        let stats = tokio::task::spawn_blocking({
            let path = self.config.path.clone();
            move || get_disk_stats(&path)
        })
        .await
        .context("Failed to spawn blocking task")??;

        let status = self.determine_status(&stats);

        Ok(DiskStats { status, ..stats })
    }

    fn determine_status(&self, stats: &DiskStats) -> DiskStatus {
        let usage = stats.usage_percent;

        if usage >= self.config.critical_threshold_percent as f64 {
            DiskStatus::Critical
        } else if usage >= self.config.warning_threshold_percent as f64 {
            DiskStatus::Warning
        } else if stats.free_bytes < self.config.min_free_bytes {
            DiskStatus::Warning
        } else {
            DiskStatus::Normal
        }
    }

    async fn trigger_cleanup(&self) {
        let mut last_cleanup = self.last_cleanup.lock().await;

        if let Some(last) = *last_cleanup {
            // Honor the configured minimum interval instead of a hardcoded
            // 300s throttle.
            if last.elapsed() < self.config.min_cleanup_interval {
                debug!("Cleanup triggered too frequently, skipping");
                return;
            }
        }

        info!("Triggering disk cleanup...");

        match self.cleanup_old_files().await {
            Ok(report) => {
                info!("✓ Cleanup completed: {:?}", report);
                *last_cleanup = Some(std::time::Instant::now());
            }
            Err(e) => {
                error!("Cleanup failed: {}", e);
            }
        }
    }

    pub async fn cleanup_old_files(&self) -> Result<PcapCleanupReport> {
        let authority = self
            .cleanup_authority
            .as_ref()
            .context("disk cleanup has no PCAP cleanup authority")?;
        let spool_root = self
            .canonical_spool_root
            .as_ref()
            .context("disk cleanup has no canonical spool root")?;
        let claims = authority.cleanup_claims()?;
        let mut report = PcapCleanupReport {
            claims_inspected: claims.len(),
            ..PcapCleanupReport::default()
        };
        let target_usage = self.config.cleanup_target_percent;
        for claim in claims {
            // Honor the configured target water mark: stop deleting once the
            // *measured* usage is at or below the target, instead of deleting
            // every authorized claim unconditionally. Before any real
            // measurement exists (e.g. tests or a monitor that has not ticked
            // yet), proceed with the authorized claims.
            let measured = self.measured.load(Ordering::Acquire);
            let usage = self.latest_stats.read().await.usage_percent;
            if measured && usage <= target_usage {
                debug!(
                    "Cleanup reached target usage {:.1}% (target {:.1}%); stopping",
                    usage, target_usage
                );
                break;
            }
            spool_root.unlink_claimed(&claim).await?;
            report.files_deleted += 1;
            authority.record_deleted(&claim)?;
            report.tombstones_written += 1;
        }
        Ok(report)
    }

    pub async fn get_stats(&self) -> DiskStats {
        self.latest_stats.read().await.clone()
    }

    pub fn alert_counts(&self) -> (u64, u64) {
        (
            self.warning_count.load(Ordering::Relaxed),
            self.critical_count.load(Ordering::Relaxed),
        )
    }
}

fn verify_claimed_inode(path: &Path, claim: &CleanupClaim) -> Result<()> {
    let metadata = std::fs::symlink_metadata(path)?;
    if metadata.file_type().is_symlink()
        || !metadata.is_file()
        || metadata.dev() != claim.device
        || metadata.ino() != claim.inode
    {
        bail!("cleanup claim path identity changed before unlink");
    }
    Ok(())
}

async fn sync_directory(path: &Path) -> Result<()> {
    let path = path.to_path_buf();
    let sync_path = path.clone();
    tokio::task::spawn_blocking(move || std::fs::File::open(&sync_path)?.sync_all())
        .await
        .context("cleanup directory sync task failed")?
        .with_context(|| format!("failed to fsync cleanup directory {}", path.display()))
}

fn get_disk_stats(path: &str) -> Result<DiskStats> {
    #[cfg(target_os = "linux")]
    {
        use std::ffi::CString;
        use std::mem::MaybeUninit;

        let c_path = CString::new(path).context("Invalid path")?;

        let mut stat: MaybeUninit<libc::statvfs> = MaybeUninit::uninit();

        let ret = unsafe { libc::statvfs(c_path.as_ptr(), stat.as_mut_ptr()) };

        if ret != 0 {
            let err = std::io::Error::last_os_error();
            anyhow::bail!("statvfs failed: {}", err);
        }

        let stat = unsafe { stat.assume_init() };

        let block_size = stat.f_frsize as u64;
        let total_bytes = stat.f_blocks * block_size;
        let free_bytes = stat.f_bavail * block_size;
        let used_bytes = total_bytes - (stat.f_bfree * block_size);

        let usage_percent = if total_bytes > 0 {
            (used_bytes as f64 / total_bytes as f64) * 100.0
        } else {
            0.0
        };

        Ok(DiskStats {
            total_bytes,
            used_bytes,
            free_bytes,
            usage_percent,
            status: DiskStatus::Normal,
            timestamp: std::time::Instant::now(),
        })
    }

    #[cfg(not(target_os = "linux"))]
    {
        anyhow::bail!("DiskMonitor only supported on Linux");
    }
}

#[cfg(test)]
mod tests {
    use super::{DiskMonitor, DiskMonitorConfig};
    use crate::archiver::{ObjectWriteReceipt, UploadJournal, UploadTask};
    use std::sync::Arc;

    fn receipt(stored_size: u64, sha256: &str) -> ObjectWriteReceipt {
        ObjectWriteReceipt {
            bucket: "pcap-archive".to_string(),
            key: "tenant-a/probe-a/capture.pcap.zst".to_string(),
            version_id: "v1".to_string(),
            etag: "etag".to_string(),
            stored_size,
            sha256: sha256.to_string(),
        }
    }

    #[tokio::test]
    async fn cleanup_deletes_only_exact_authorized_claim() {
        let directory = tempfile::tempdir().expect("directory");
        let spool_root = directory.path().join("spool");
        let identity_root = spool_root.join("tenant-a").join("probe-a");
        std::fs::create_dir_all(&identity_root).expect("spool identity root");
        let journal = Arc::new(UploadJournal::new(directory.path()).expect("journal"));
        let task = UploadTask {
            data: vec![1],
            ts_start: 1,
            ts_end: 1,
            packet_count: 1,
            tenant_id: "tenant-a".to_string(),
            probe_id: "probe-a".to_string(),
        };
        let pending_path = identity_root.join("old-pending.pcap.zst");
        let authorized_path = identity_root.join("new-authorized.pcap.zst");
        std::fs::write(&pending_path, b"pending").expect("pending file");
        std::fs::write(&authorized_path, b"authorized").expect("authorized file");
        let pending_sha = "a".repeat(64);
        let authorized_sha = "b".repeat(64);
        let pending_id = journal
            .record_pending(
                &task,
                pending_path.to_str().expect("pending path"),
                1,
                7,
                &pending_sha,
            )
            .expect("pending entry");
        let authorized_id = journal
            .record_pending(
                &task,
                authorized_path.to_str().expect("authorized path"),
                1,
                10,
                &authorized_sha,
            )
            .expect("authorized entry");
        journal
            .mark_object_written(&authorized_id, &receipt(10, &authorized_sha))
            .expect("object");
        journal
            .mark_metadata_accepted(&authorized_id)
            .expect("metadata");
        let revision = journal
            .get_entry(&authorized_id)
            .expect("lookup")
            .expect("accepted")
            .revision;
        journal
            .claim_cleanup_authority(&authorized_id, revision, &spool_root)
            .expect("cleanup claim");

        let monitor = DiskMonitor::with_pcap_cleanup_authority(
            DiskMonitorConfig {
                path: spool_root.to_string_lossy().into_owned(),
                ..DiskMonitorConfig::default()
            },
            journal.clone(),
            &spool_root,
        )
        .expect("monitor");
        let report = monitor.cleanup_old_files().await.expect("cleanup");
        assert_eq!(report.files_deleted, 1);
        assert!(pending_path.exists(), "PENDING evidence must survive");
        assert!(!authorized_path.exists());
        assert_eq!(
            journal
                .get_entry(&pending_id)
                .expect("lookup")
                .expect("pending")
                .effective_object_state(),
            crate::archiver::JournalObjectState::Pending
        );
        assert_eq!(
            journal
                .get_entry(&authorized_id)
                .expect("lookup")
                .expect("deleted")
                .effective_object_state(),
            crate::archiver::JournalObjectState::Deleted
        );
    }
}
