use anyhow::{bail, Context, Result};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::time::interval;
use tracing::{error, info, warn};

#[derive(Debug, Clone)]
pub struct InterfaceStatus {
    pub name: String,

    pub link_up: bool,

    pub speed_mbps: u64,

    pub rx_packets: u64,

    pub tx_packets: u64,

    pub rx_bytes: u64,

    pub tx_bytes: u64,

    pub rx_errors: u64,

    pub tx_errors: u64,

    pub rx_crc_errors: u64,

    pub rx_dropped: u64,

    pub tx_dropped: u64,

    pub collisions: u64,

    pub timestamp: Instant,
}

impl InterfaceStatus {
    pub fn read(interface: &str) -> Result<Self> {
        let sys_path = PathBuf::from(format!("/sys/class/net/{}", interface));

        if !sys_path.exists() {
            bail!("Interface {} not found in /sys/class/net", interface);
        }

        Ok(Self {
            name: interface.to_string(),
            link_up: read_link_status(&sys_path)?,
            speed_mbps: read_speed(&sys_path)?,
            rx_packets: read_stat(&sys_path, "statistics/rx_packets")?,
            tx_packets: read_stat(&sys_path, "statistics/tx_packets")?,
            rx_bytes: read_stat(&sys_path, "statistics/rx_bytes")?,
            tx_bytes: read_stat(&sys_path, "statistics/tx_bytes")?,
            rx_errors: read_stat(&sys_path, "statistics/rx_errors")?,
            tx_errors: read_stat(&sys_path, "statistics/tx_errors")?,
            rx_crc_errors: read_stat(&sys_path, "statistics/rx_crc_errors")?,
            rx_dropped: read_stat(&sys_path, "statistics/rx_dropped")?,
            tx_dropped: read_stat(&sys_path, "statistics/tx_dropped")?,
            collisions: read_stat(&sys_path, "statistics/collisions")?,
            timestamp: Instant::now(),
        })
    }

    pub fn delta(&self, prev: &InterfaceStatus) -> InterfaceStatusDelta {
        let duration = self.timestamp.duration_since(prev.timestamp);
        let duration_secs = duration.as_secs_f64();

        let rx_packets_delta = self.rx_packets.saturating_sub(prev.rx_packets);
        let tx_packets_delta = self.tx_packets.saturating_sub(prev.tx_packets);
        let rx_bytes_delta = self.rx_bytes.saturating_sub(prev.rx_bytes);
        let tx_bytes_delta = self.tx_bytes.saturating_sub(prev.tx_bytes);

        InterfaceStatusDelta {
            interface: self.name.clone(),
            duration,

            rx_pps: if duration_secs > 0.0 {
                rx_packets_delta as f64 / duration_secs
            } else {
                0.0
            },
            tx_pps: if duration_secs > 0.0 {
                tx_packets_delta as f64 / duration_secs
            } else {
                0.0
            },

            rx_mbps: if duration_secs > 0.0 {
                (rx_bytes_delta as f64 * 8.0) / (duration_secs * 1_000_000.0)
            } else {
                0.0
            },
            tx_mbps: if duration_secs > 0.0 {
                (tx_bytes_delta as f64 * 8.0) / (duration_secs * 1_000_000.0)
            } else {
                0.0
            },

            rx_errors_delta: self.rx_errors.saturating_sub(prev.rx_errors),
            tx_errors_delta: self.tx_errors.saturating_sub(prev.tx_errors),
            rx_dropped_delta: self.rx_dropped.saturating_sub(prev.rx_dropped),
            tx_dropped_delta: self.tx_dropped.saturating_sub(prev.tx_dropped),

            link_status_changed: self.link_up != prev.link_up,
            speed_changed: self.speed_mbps != prev.speed_mbps,
        }
    }
}

#[derive(Debug, Clone)]
pub struct InterfaceStatusDelta {
    pub interface: String,
    pub duration: Duration,

    pub rx_pps: f64,

    pub tx_pps: f64,

    pub rx_mbps: f64,

    pub tx_mbps: f64,

    pub rx_errors_delta: u64,

    pub tx_errors_delta: u64,

    pub rx_dropped_delta: u64,

    pub tx_dropped_delta: u64,

    pub link_status_changed: bool,

    pub speed_changed: bool,
}

impl InterfaceStatusDelta {
    pub fn has_critical_issues(&self) -> bool {
        const ERROR_THRESHOLD: u64 = 100;
        const DROP_THRESHOLD: u64 = 1000;

        self.rx_errors_delta > ERROR_THRESHOLD
            || self.tx_errors_delta > ERROR_THRESHOLD
            || self.rx_dropped_delta > DROP_THRESHOLD
            || self.tx_dropped_delta > DROP_THRESHOLD
            || !self.link_status_changed
    }
}

#[derive(Debug, Clone)]
pub struct InterfaceMonitorConfig {
    pub interfaces: Vec<String>,

    pub poll_interval: Duration,

    pub enabled: bool,
}

impl Default for InterfaceMonitorConfig {
    fn default() -> Self {
        Self {
            interfaces: Vec::new(),
            poll_interval: Duration::from_secs(10),
            enabled: true,
        }
    }
}

pub struct InterfaceMonitor {
    config: InterfaceMonitorConfig,
    running: Arc<AtomicBool>,

    latest_status: Arc<tokio::sync::RwLock<std::collections::HashMap<String, InterfaceStatus>>>,

    previous_status: Arc<tokio::sync::RwLock<std::collections::HashMap<String, InterfaceStatus>>>,
}

impl InterfaceMonitor {
    pub fn new(config: InterfaceMonitorConfig) -> Self {
        Self {
            config,
            running: Arc::new(AtomicBool::new(false)),
            latest_status: Arc::new(tokio::sync::RwLock::new(std::collections::HashMap::new())),
            previous_status: Arc::new(tokio::sync::RwLock::new(std::collections::HashMap::new())),
        }
    }

    pub async fn start(self: Arc<Self>) -> tokio::task::JoinHandle<()> {
        if !self.config.enabled || self.config.interfaces.is_empty() {
            info!("Interface monitoring disabled or no interfaces configured");
            return tokio::spawn(async {});
        }

        self.running.store(true, Ordering::Release);

        info!(
            "Starting interface monitoring: interfaces={:?}, interval={}s",
            self.config.interfaces,
            self.config.poll_interval.as_secs()
        );

        let monitor = Arc::clone(&self);

        tokio::spawn(async move {
            monitor.run().await;
        })
    }

    pub fn stop(&self) {
        self.running.store(false, Ordering::Release);
        info!("Interface monitoring stopped");
    }

    pub async fn get_status(&self, interface: &str) -> Option<InterfaceStatus> {
        let status_map = self.latest_status.read().await;
        status_map.get(interface).cloned()
    }

    pub async fn get_all_status(&self) -> Vec<InterfaceStatus> {
        let status_map = self.latest_status.read().await;
        status_map.values().cloned().collect()
    }

    pub async fn get_delta(&self, interface: &str) -> Option<InterfaceStatusDelta> {
        let latest = self.latest_status.read().await;
        let previous = self.previous_status.read().await;

        let current = latest.get(interface)?;
        let prev = previous.get(interface)?;

        Some(current.delta(prev))
    }

    async fn run(&self) {
        let mut ticker = interval(self.config.poll_interval);
        let mut iteration = 0u64;

        while self.running.load(Ordering::Acquire) {
            ticker.tick().await;
            iteration += 1;

            for interface in &self.config.interfaces {
                match InterfaceStatus::read(interface) {
                    Ok(status) => {
                        if let Some(prev) = self.latest_status.read().await.get(interface) {
                            self.previous_status
                                .write()
                                .await
                                .insert(interface.clone(), prev.clone());
                        }

                        if let Some(prev) = self.previous_status.read().await.get(interface) {
                            let delta = status.delta(prev);

                            if iteration % 6 == 0 {
                                info!(
                                    "Interface {}: link={}, speed={}Mbps, rx={:.1}pps/{:.1}Mbps, tx={:.1}pps/{:.1}Mbps, errors={}↓/{}↑, drops={}↓/{}↑",
                                    interface,
                                    if status.link_up { "UP" } else { "DOWN" },
                                    status.speed_mbps,
                                    delta.rx_pps,
                                    delta.rx_mbps,
                                    delta.tx_pps,
                                    delta.tx_mbps,
                                    delta.rx_errors_delta,
                                    delta.tx_errors_delta,
                                    delta.rx_dropped_delta,
                                    delta.tx_dropped_delta,
                                );
                            }

                            self.check_alerts(&status, &delta).await;
                        }

                        self.latest_status
                            .write()
                            .await
                            .insert(interface.clone(), status);
                    }
                    Err(e) => {
                        error!("Failed to read status for interface {}: {}", interface, e);
                    }
                }
            }
        }

        info!("Interface monitor loop exited");
    }

    async fn check_alerts(&self, status: &InterfaceStatus, delta: &InterfaceStatusDelta) {
        if delta.link_status_changed {
            if status.link_up {
                warn!(
                    "⚠ Interface {} LINK UP: speed={}Mbps",
                    status.name, status.speed_mbps
                );
            } else {
                error!(
                    "🔴 Interface {} LINK DOWN! Last speed={}Mbps",
                    status.name, status.speed_mbps
                );
            }
        }

        if delta.speed_changed && status.link_up {
            warn!(
                "⚠ Interface {} speed changed to {}Mbps",
                status.name, status.speed_mbps
            );
        }

        if delta.rx_errors_delta > 100 || delta.tx_errors_delta > 100 {
            warn!(
                "⚠ Interface {} high error rate: rx={}, tx={}",
                status.name, delta.rx_errors_delta, delta.tx_errors_delta
            );
        }

        if delta.rx_dropped_delta > 1000 || delta.tx_dropped_delta > 1000 {
            warn!(
                "⚠ Interface {} high drop rate: rx={}, tx={}",
                status.name, delta.rx_dropped_delta, delta.tx_dropped_delta
            );
        }
    }
}

fn read_link_status(sys_path: &Path) -> Result<bool> {
    let operstate = read_sysfs_string(sys_path, "operstate")?;
    Ok(operstate.trim() == "up")
}

fn read_speed(sys_path: &Path) -> Result<u64> {
    match read_sysfs_u64(sys_path, "speed") {
        Ok(speed) => Ok(speed),
        Err(_) => Ok(0),
    }
}

fn read_stat(sys_path: &Path, stat_name: &str) -> Result<u64> {
    read_sysfs_u64(sys_path, stat_name)
}

fn read_sysfs_string(sys_path: &Path, file: &str) -> Result<String> {
    let path = sys_path.join(file);
    std::fs::read_to_string(&path).with_context(|| format!("Failed to read {}", path.display()))
}

fn read_sysfs_u64(sys_path: &Path, file: &str) -> Result<u64> {
    let content = read_sysfs_string(sys_path, file)?;
    content
        .trim()
        .parse()
        .with_context(|| format!("Failed to parse u64 from {}", file))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_read_interface_status() {
        match InterfaceStatus::read("lo") {
            Ok(status) => {
                println!("Interface lo:");
                println!("  Link: {}", if status.link_up { "UP" } else { "DOWN" });
                println!("  Speed: {} Mbps", status.speed_mbps);
                println!(
                    "  RX: {} packets, {} bytes",
                    status.rx_packets, status.rx_bytes
                );
                println!(
                    "  TX: {} packets, {} bytes",
                    status.tx_packets, status.tx_bytes
                );
                println!("  Errors: RX={}, TX={}", status.rx_errors, status.tx_errors);
            }
            Err(e) => {
                eprintln!("Failed to read lo interface: {}", e);
            }
        }
    }

    #[test]
    fn test_status_delta() {
        let status1 = InterfaceStatus {
            name: "eth0".to_string(),
            link_up: true,
            speed_mbps: 10000,
            rx_packets: 1000,
            tx_packets: 500,
            rx_bytes: 1_000_000,
            tx_bytes: 500_000,
            rx_errors: 10,
            tx_errors: 5,
            rx_crc_errors: 2,
            rx_dropped: 100,
            tx_dropped: 50,
            collisions: 0,
            timestamp: Instant::now(),
        };

        std::thread::sleep(Duration::from_millis(100));

        let status2 = InterfaceStatus {
            name: "eth0".to_string(),
            link_up: true,
            speed_mbps: 10000,
            rx_packets: 2000,
            tx_packets: 1000,
            rx_bytes: 2_000_000,
            tx_bytes: 1_000_000,
            rx_errors: 15,
            tx_errors: 8,
            rx_crc_errors: 3,
            rx_dropped: 150,
            tx_dropped: 75,
            collisions: 0,
            timestamp: Instant::now(),
        };

        let delta = status2.delta(&status1);

        println!("Delta:");
        println!("  Duration: {:?}", delta.duration);
        println!("  RX: {:.1} pps, {:.1} Mbps", delta.rx_pps, delta.rx_mbps);
        println!("  TX: {:.1} pps, {:.1} Mbps", delta.tx_pps, delta.tx_mbps);
        println!(
            "  Errors: RX={}, TX={}",
            delta.rx_errors_delta, delta.tx_errors_delta
        );

        assert!(delta.rx_pps > 0.0);
        assert_eq!(delta.rx_errors_delta, 5);
    }

    #[tokio::test]
    async fn test_interface_monitor() {
        let config = InterfaceMonitorConfig {
            interfaces: vec!["lo".to_string()],
            poll_interval: Duration::from_secs(1),
            enabled: true,
        };

        let monitor = Arc::new(InterfaceMonitor::new(config));
        let handle = monitor.clone().start().await;

        tokio::time::sleep(Duration::from_secs(3)).await;

        if let Some(status) = monitor.get_status("lo").await {
            println!("Monitored status: link={}", status.link_up);
        }

        monitor.stop();
        handle.await.ok();
    }
}
