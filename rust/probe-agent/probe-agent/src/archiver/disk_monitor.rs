use anyhow::{Context, Result};
use std::path::Path;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use tokio::time::{interval, Duration};
use tracing::{debug, error, info, warn};

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

    warning_count: AtomicU64,

    critical_count: AtomicU64,

    last_cleanup: Arc<tokio::sync::Mutex<Option<std::time::Instant>>>,
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
            warning_count: AtomicU64::new(0),
            critical_count: AtomicU64::new(0),
            last_cleanup: Arc::new(tokio::sync::Mutex::new(None)),
        }
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
            if last.elapsed() < Duration::from_secs(300) {
                debug!("Cleanup triggered too frequently, skipping");
                return;
            }
        }

        info!("Triggering disk cleanup...");

        match self.cleanup_old_files().await {
            Ok(cleaned) => {
                info!("✓ Cleanup completed: {} files removed", cleaned);
                *last_cleanup = Some(std::time::Instant::now());
            }
            Err(e) => {
                error!("Cleanup failed: {}", e);
            }
        }
    }

    async fn cleanup_old_files(&self) -> Result<usize> {
        let path = Path::new(&self.config.path);
        let mut cleaned = 0;

        let mut entries = tokio::fs::read_dir(path)
            .await
            .context("Failed to read directory")?;

        let mut files = Vec::new();

        while let Some(entry) = entries
            .next_entry()
            .await
            .context("Failed to read directory entry")?
        {
            if let Ok(metadata) = entry.metadata().await {
                if metadata.is_file() {
                    if let Ok(modified) = metadata.modified() {
                        files.push((entry.path(), modified));
                    }
                }
            }
        }

        files.sort_by(|a, b| a.1.cmp(&b.1));

        let to_remove = (files.len() as f64 * 0.3) as usize;

        for (file_path, _) in files.iter().take(to_remove) {
            match tokio::fs::remove_file(file_path).await {
                Ok(()) => {
                    cleaned += 1;
                    debug!("Removed old file: {:?}", file_path);
                }
                Err(e) => {
                    warn!("Failed to remove file {:?}: {}", file_path, e);
                }
            }
        }

        Ok(cleaned)
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
