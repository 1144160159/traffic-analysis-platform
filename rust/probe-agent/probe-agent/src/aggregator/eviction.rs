use super::flow_table::{FlowFeatureSnapshot, FlowKey, FlowValue};
use super::online_stats::OnlineStatsSnapshot;
use super::partitioned_flow_table::PartitionedFlowTable;
use crate::metrics;
use crate::parser::tcp_flags;
use proto_gen::{
    ActiveIdleStats, EventHeader, FiveTuple, FlowEvent, InterArrivalStats, PacketLengthStats,
    TrafficFeatureObservation, TransportSecurityProtocol,
};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use tokio::sync::mpsc::Sender;
use tokio::time::{interval, Duration, Instant};
use tracing::{debug, error, info, warn};

/// 淘汰原因 — 用于监控和分析流生命周期
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EvictionReason {
    IdleTimeout,   // 空闲超时 (默认120s)
    ActiveTimeout, // 活动超时 (默认1800s)
    ForcedCleanup, // 强制清理 (内存压力/关闭)
    TCPFlagFinish, // TCP FIN/RST 正常结束
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum EvictionClockMode {
    LiveProcessing,
    OfflineWatermark,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct EvictionNow {
    pub epoch_millis: u64,
    pub end_of_input: bool,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum EvictionClockError {
    #[error("offline eviction watermark is not initialized")]
    MissingOfflineWatermark,
    #[error("system clock precedes the Unix epoch")]
    InvalidSystemClock,
}

#[derive(Debug)]
pub struct EvictionClock {
    mode: EvictionClockMode,
    capture_watermark_ms: AtomicU64,
    end_of_input: AtomicBool,
}

impl EvictionClock {
    pub fn live() -> Self {
        Self {
            mode: EvictionClockMode::LiveProcessing,
            capture_watermark_ms: AtomicU64::new(0),
            end_of_input: AtomicBool::new(false),
        }
    }

    pub fn offline() -> Self {
        Self {
            mode: EvictionClockMode::OfflineWatermark,
            capture_watermark_ms: AtomicU64::new(0),
            end_of_input: AtomicBool::new(false),
        }
    }

    pub fn mode(&self) -> EvictionClockMode {
        self.mode
    }

    pub fn observe_capture_micros(&self, timestamp_micros: u64) {
        let candidate = timestamp_micros / 1_000;
        self.capture_watermark_ms
            .fetch_max(candidate, Ordering::AcqRel);
    }

    pub fn mark_end_of_input(&self) {
        self.end_of_input.store(true, Ordering::Release);
    }

    pub fn eviction_now(&self) -> Result<EvictionNow, EvictionClockError> {
        match self.mode {
            EvictionClockMode::LiveProcessing => {
                let epoch_millis = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map_err(|_| EvictionClockError::InvalidSystemClock)?
                    .as_millis() as u64;
                Ok(EvictionNow {
                    epoch_millis,
                    end_of_input: false,
                })
            }
            EvictionClockMode::OfflineWatermark => {
                let epoch_millis = self.capture_watermark_ms.load(Ordering::Acquire);
                if epoch_millis == 0 {
                    return Err(EvictionClockError::MissingOfflineWatermark);
                }
                Ok(EvictionNow {
                    epoch_millis,
                    end_of_input: self.end_of_input.load(Ordering::Acquire),
                })
            }
        }
    }
}

fn deterministic_flow_uuid(kind: &str, identity: &str) -> String {
    uuid::Uuid::new_v5(
        &uuid::Uuid::NAMESPACE_OID,
        format!("traffic-probe-flow/v1\0{kind}\0{identity}").as_bytes(),
    )
    .to_string()
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FlowSnapshotError {
    #[error("removed flow has an empty event-time window")]
    EmptyTimeWindow,
    #[error("removed flow end precedes start")]
    ReversedTimeWindow,
    #[error("removed flow has no packets")]
    EmptyCounters,
}

pub struct RemovedFlow {
    pub key: FlowKey,
    pub value: FlowValue,
}

impl std::fmt::Debug for RemovedFlow {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("RemovedFlow")
            .field("key", &self.key)
            .finish_non_exhaustive()
    }
}

#[derive(Clone, Debug)]
pub struct FlowSnapshot {
    pub key: FlowKey,
    pub community_id: String,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packets_fwd: u64,
    pub packets_bwd: u64,
    pub bytes_fwd: u64,
    pub bytes_bwd: u64,
    pub tcp_flags_fwd: u16,
    pub tcp_flags_bwd: u16,
    pub tos: u8,
    pub pktlen_count: u64,
    pub pktlen_sum: u64,
    pub pktlen_min: u32,
    pub pktlen_max: u32,
    pub pktlen_mean: f32,
    pub pktlen_std: f32,
    pub iat_fwd_count: u64,
    pub iat_fwd_sum: u64,
    pub iat_fwd_min: u32,
    pub iat_fwd_max: u32,
    pub iat_fwd_std: f32,
    pub iat_bwd_count: u64,
    pub iat_bwd_sum: u64,
    pub iat_bwd_min: u32,
    pub iat_bwd_max: u32,
    pub iat_bwd_std: f32,
    pub active: OnlineStatsSnapshot,
    pub idle: OnlineStatsSnapshot,
    pub feature: FlowFeatureSnapshot,
}

impl FlowSnapshot {
    /// A value reaches this function only after it has been removed from the
    /// table, so no concurrent writer can mutate its atomics. The resulting
    /// object is the sole input to event mapping and identity derivation.
    pub fn try_from_removed(
        removed: RemovedFlow,
    ) -> Result<Self, (FlowSnapshotError, RemovedFlow)> {
        let ts_start = removed.value.start_time.load(Ordering::Acquire);
        let ts_end = removed.value.last_seen.load(Ordering::Acquire);
        if ts_start == 0 || ts_end == 0 {
            return Err((FlowSnapshotError::EmptyTimeWindow, removed));
        }
        if ts_end < ts_start {
            return Err((FlowSnapshotError::ReversedTimeWindow, removed));
        }
        let packets_fwd = removed.value.packets_fwd.load(Ordering::Acquire);
        let packets_bwd = removed.value.packets_bwd.load(Ordering::Acquire);
        if packets_fwd + packets_bwd == 0 {
            return Err((FlowSnapshotError::EmptyCounters, removed));
        }
        let RemovedFlow { key, value } = removed;
        Ok(Self {
            community_id: key.community_id().to_owned(),
            key,
            ts_start,
            ts_end,
            packets_fwd,
            packets_bwd,
            bytes_fwd: value.bytes_fwd.load(Ordering::Acquire),
            bytes_bwd: value.bytes_bwd.load(Ordering::Acquire),
            tcp_flags_fwd: value.tcp_flags_fwd.load(Ordering::Acquire),
            tcp_flags_bwd: value.tcp_flags_bwd.load(Ordering::Acquire),
            tos: value.tos.load(Ordering::Acquire),
            pktlen_count: value.pktlen_stats.count(),
            pktlen_sum: value.pktlen_stats.sum(),
            pktlen_min: value.pktlen_stats.min(),
            pktlen_max: value.pktlen_stats.max(),
            pktlen_mean: value.pktlen_stats.mean(),
            pktlen_std: value.pktlen_stats.std(),
            iat_fwd_count: value.iat_fwd_stats.count(),
            iat_fwd_sum: value.iat_fwd_stats.sum(),
            iat_fwd_min: value.iat_fwd_stats.min(),
            iat_fwd_max: value.iat_fwd_stats.max(),
            iat_fwd_std: value.iat_fwd_stats.std(),
            iat_bwd_count: value.iat_bwd_stats.count(),
            iat_bwd_sum: value.iat_bwd_stats.sum(),
            iat_bwd_min: value.iat_bwd_stats.min(),
            iat_bwd_max: value.iat_bwd_stats.max(),
            iat_bwd_std: value.iat_bwd_stats.std(),
            active: value.active_stats.snapshot(),
            idle: value.idle_stats.snapshot(),
            feature: value.feature_state.snapshot(),
        })
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FlowEventIdentity {
    pub flow_id: String,
    pub event_id: String,
    pub idempotency_key: String,
    pub revision: u32,
}

impl FlowEventIdentity {
    pub const REVISION: u32 = 2;

    pub fn derive(config: &EvictionConfig, snapshot: &FlowSnapshot) -> Self {
        // Include the observation scope only when it is non-trivial: the
        // GlobalL3 baseline must keep its legacy deterministic identity, while
        // interface/VLAN/queue/tap-scoped flows must not collide on the same
        // five-tuple across observation points.
        let global_l3 = super::flow_table::ObservationScope::global_l3()
            .stable_discriminator_bytes();
        let scope = snapshot.key.scope_discriminator();
        let scope_fragment = if scope == global_l3.as_slice() {
            String::new()
        } else {
            let mut hex = String::with_capacity(scope.len() * 2);
            for byte in scope {
                hex.push_str(&format!("{byte:02x}"));
            }
            format!("\0scope:{hex}")
        };
        let source = format!(
            "{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}\0{}{}",
            config.tenant_id,
            config.probe_id,
            config.run_id,
            Self::REVISION,
            snapshot.community_id,
            snapshot.key.src_ip,
            snapshot.key.dst_ip,
            snapshot.key.src_port,
            snapshot.key.dst_port,
            snapshot.key.protocol,
            snapshot.ts_start,
            snapshot.ts_end,
            scope_fragment,
        );
        let flow_id = deterministic_flow_uuid("flow", &source);
        let event_id = deterministic_flow_uuid("event", &source);
        Self {
            flow_id,
            idempotency_key: event_id.clone(),
            event_id,
            revision: Self::REVISION,
        }
    }
}

#[cfg(test)]
mod deterministic_id_tests {
    use super::deterministic_flow_uuid;

    #[test]
    fn replay_identity_is_stable_and_kind_scoped() {
        let identity = "tenant-a\0probe-a\0run-a\0community-a\0192.0.2.1\0198.51.100.1\01234\0443\06\0100\0200";
        assert_eq!(
            deterministic_flow_uuid("event", identity),
            deterministic_flow_uuid("event", identity)
        );
        assert_ne!(
            deterministic_flow_uuid("event", identity),
            deterministic_flow_uuid("flow", identity)
        );
    }
}

/// 淘汰统计 — 实时监控流淘汰行为
#[derive(Debug, Default)]
pub struct EvictionStats {
    pub total_evicted: AtomicU64,
    pub idle_evicted: AtomicU64,
    pub active_evicted: AtomicU64,
    pub forced_evicted: AtomicU64,
    pub tcp_finish_evicted: AtomicU64,
    pub last_scan_duration_ms: AtomicU64,
    pub last_scan_evicted: AtomicU64,
}

impl EvictionStats {
    pub fn record(&self, reason: EvictionReason, count: u64) {
        self.total_evicted.fetch_add(count, Ordering::Relaxed);
        match reason {
            EvictionReason::IdleTimeout => self.idle_evicted.fetch_add(count, Ordering::Relaxed),
            EvictionReason::ActiveTimeout => {
                self.active_evicted.fetch_add(count, Ordering::Relaxed)
            }
            EvictionReason::ForcedCleanup => {
                self.forced_evicted.fetch_add(count, Ordering::Relaxed)
            }
            EvictionReason::TCPFlagFinish => {
                self.tcp_finish_evicted.fetch_add(count, Ordering::Relaxed)
            }
        };
    }

    pub fn snapshot(&self) -> EvictionStatsSnapshot {
        EvictionStatsSnapshot {
            total: self.total_evicted.load(Ordering::Relaxed),
            idle: self.idle_evicted.load(Ordering::Relaxed),
            active: self.active_evicted.load(Ordering::Relaxed),
            forced: self.forced_evicted.load(Ordering::Relaxed),
            tcp_finish: self.tcp_finish_evicted.load(Ordering::Relaxed),
        }
    }
}

#[derive(Debug, Clone)]
pub struct EvictionStatsSnapshot {
    pub total: u64,
    pub idle: u64,
    pub active: u64,
    pub forced: u64,
    pub tcp_finish: u64,
}
#[derive(Clone, Debug)]
pub struct EvictionConfig {
    pub idle_timeout: Duration,
    pub active_timeout: Duration,
    pub scan_interval: Duration,
    pub tenant_id: String,
    pub probe_id: String,
    pub run_id: String,
    pub feature_set_id: String,
    pub use_timewheel: bool,
    pub timewheel_slot_duration: Duration,
    pub timewheel_slot_count: usize,
}
impl Default for EvictionConfig {
    fn default() -> Self {
        Self {
            idle_timeout: Duration::from_secs(120),
            active_timeout: Duration::from_secs(1800),
            scan_interval: Duration::from_secs(1),
            tenant_id: "default".to_string(),
            probe_id: "probe-01".to_string(),
            run_id: "realtime".to_string(),
            feature_set_id: "v1".to_string(),
            use_timewheel: true,
            timewheel_slot_duration: Duration::from_secs(10),
            timewheel_slot_count: 360,
        }
    }
}
pub struct Eviction {
    config: EvictionConfig,
    flow_table: Arc<PartitionedFlowTable>,
    output_tx: Sender<FlowEvent>,
    pub stats: Arc<EvictionStats>,
    clock: Arc<EvictionClock>,
    rejected_snapshots: Mutex<Vec<RemovedFlow>>,
    send_failures: AtomicU64,
}
impl Eviction {
    pub fn new(
        config: EvictionConfig,
        flow_table: Arc<PartitionedFlowTable>,
        output_tx: Sender<FlowEvent>,
    ) -> Self {
        Self::with_clock(
            config,
            flow_table,
            output_tx,
            Arc::new(EvictionClock::live()),
        )
    }

    pub fn with_clock(
        config: EvictionConfig,
        flow_table: Arc<PartitionedFlowTable>,
        output_tx: Sender<FlowEvent>,
        clock: Arc<EvictionClock>,
    ) -> Self {
        // NOTE: the hierarchical timewheel path was never wired into the
        // update pipeline (production forces `use_timewheel=false`) and
        // contained latent cascade/tick bugs; it has been removed. Eviction
        // always uses the full scan method. The `use_timewheel` config field
        // is retained for serde compatibility and is ignored.
        info!(
            "Eviction created: idle={}s, active={}s, scan={}s, method=FullScan",
            config.idle_timeout.as_secs(),
            config.active_timeout.as_secs(),
            config.scan_interval.as_secs()
        );
        let stats = Arc::new(EvictionStats::default());
        Self {
            config,
            flow_table,
            output_tx,
            stats,
            clock,
            rejected_snapshots: Mutex::new(Vec::new()),
            send_failures: AtomicU64::new(0),
        }
    }

    pub fn rejected_snapshot_count(&self) -> usize {
        self.rejected_snapshots
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .len()
    }

    pub fn send_failure_count(&self) -> u64 {
        self.send_failures.load(Ordering::Relaxed)
    }
    pub async fn run(&self) {
        let mut ticker = interval(self.config.scan_interval);
        let mut total_evicted: u64 = 0;
        let mut last_log_time = Instant::now();
        let mut last_stats_time = Instant::now();
        info!(
            "Eviction started: idle={}s, active={}s, scan={}s, method=FullScan",
            self.config.idle_timeout.as_secs(),
            self.config.active_timeout.as_secs(),
            self.config.scan_interval.as_secs(),
        );
        loop {
            ticker.tick().await;
            let eviction_now = match self.clock.eviction_now() {
                Ok(now) => now,
                Err(EvictionClockError::MissingOfflineWatermark) => continue,
                Err(error) => {
                    warn!("Eviction clock failed closed: {}", error);
                    continue;
                }
            };
            let now_ms = eviction_now.epoch_millis;
            let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
            let active_timeout_ms = self.config.active_timeout.as_millis() as u64;
            let eviction_start = Instant::now();
            let flow_count_before = self.flow_table.len();
            let evicted = if eviction_now.end_of_input {
                self.flow_table.evict_expired(|_, _, _| true)
            } else {
                self.evict_fullscan(now_ms, idle_timeout_ms, active_timeout_ms)
            };
            let eviction_duration = eviction_start.elapsed();
            let evicted_count = evicted.len();
            if eviction_duration > Duration::from_millis(100) {
                warn!(
                    "Eviction took {}ms (>{} ms threshold), evicted={} flows",
                    eviction_duration.as_millis(),
                    100,
                    evicted_count
                );
            }
            if evicted_count > 0 {
                info!(
                    "Eviction scan: before={}, evicted={}, after={}, duration={}ms",
                    flow_count_before,
                    evicted_count,
                    self.flow_table.len(),
                    eviction_duration.as_millis()
                );
            } else if flow_count_before > 0 {
                debug!(
                    "Eviction scan: {} flows checked, 0 evicted, now_ms={}, idle_timeout={}ms, active_timeout={}ms",
                    flow_count_before, now_ms, idle_timeout_ms, active_timeout_ms
                );
                let mut sample_count = 0;
                for entry in self.flow_table.iter() {
                    if sample_count >= 5 {
                        break;
                    }
                    let value = entry.value();
                    let last_seen = value.last_seen.load(Ordering::Relaxed);
                    let start_time = value.start_time.load(Ordering::Relaxed);
                    let idle_ms = now_ms.saturating_sub(last_seen);
                    let active_ms = now_ms.saturating_sub(start_time);
                    debug!(
                        "  flow sample: src={}:{} dst={}:{} last_seen={} start_time={} idle={}ms(threshold={}ms) active={}ms(threshold={}ms)",
                        entry.key().src_ip,
                        entry.key().src_port,
                        entry.key().dst_ip,
                        entry.key().dst_port,
                        last_seen,
                        start_time,
                        idle_ms,
                        idle_timeout_ms,
                        active_ms,
                        active_timeout_ms
                    );
                    sample_count += 1;
                }
            }
            let emitted = self
                .emit_evicted(evicted, now_ms, idle_timeout_ms, active_timeout_ms)
                .await;
            total_evicted += emitted;
            metrics::ACTIVE_FLOWS.set(self.flow_table.len() as f64);
            if last_log_time.elapsed() >= Duration::from_secs(30) {
                info!(
                    "Eviction summary: evicted={}, active_flows={}, total_evicted={}, last_duration={}ms, FullScan",
                    evicted_count,
                    self.flow_table.len(),
                    total_evicted,
                    eviction_duration.as_millis()
                );
                last_log_time = Instant::now();
            }
            if last_stats_time.elapsed() >= Duration::from_secs(60) {
                last_stats_time = Instant::now();
            }
        }
    }
    /// Emit evicted flows as events. Send failures are retried with a bounded
    /// backoff; if the channel remains unavailable the failure is counted in
    /// `send_failures` and logged with the flow identity instead of being
    /// silently dropped. Returns the number of events successfully sent.
    async fn emit_evicted(
        &self,
        evicted: Vec<(FlowKey, FlowValue)>,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
    ) -> u64 {
        let mut emitted = 0u64;
        for (key, value) in evicted {
            let idle_ms = now_ms.saturating_sub(value.last_seen.load(Ordering::Relaxed));
            let active_ms = now_ms.saturating_sub(value.start_time.load(Ordering::Relaxed));
            let reason =
                self.eviction_reason(&value, now_ms, idle_timeout_ms, active_timeout_ms);
            info!(
                "Flow evicted: {}:{} <-> {}:{} proto={} idle={}ms active={}ms reason={}",
                key.src_ip,
                key.src_port,
                key.dst_ip,
                key.dst_port,
                key.protocol,
                idle_ms,
                active_ms,
                reason
            );
            let snapshot = match FlowSnapshot::try_from_removed(RemovedFlow { key, value }) {
                Ok(snapshot) => snapshot,
                Err((error, removed)) => {
                    warn!(
                        "Removed flow rejected and retained before event mapping: {}",
                        error
                    );
                    self.rejected_snapshots
                        .lock()
                        .unwrap_or_else(|poisoned| poisoned.into_inner())
                        .push(removed);
                    continue;
                }
            };
            let event = self.to_flow_event(&snapshot);
            let mut send_attempts = 0;
            loop {
                match self.output_tx.send(event.clone()).await {
                    Ok(()) => {
                        emitted += 1;
                        metrics::FLOWS_EVICTED.inc();
                        break;
                    }
                    Err(e) => {
                        send_attempts += 1;
                        if send_attempts >= 3 {
                            self.send_failures.fetch_add(1, Ordering::Relaxed);
                            error!(
                                "Failed to send flow event after {} attempts; dropping snapshot for {}:{} <-> {}:{} (send_failures={})",
                                send_attempts,
                                snapshot.key.src_ip,
                                snapshot.key.src_port,
                                snapshot.key.dst_ip,
                                snapshot.key.dst_port,
                                self.send_failures.load(Ordering::Relaxed)
                            );
                            break;
                        }
                        warn!(
                            "Failed to send flow event (attempt {}/3), retrying: {}",
                            send_attempts, e
                        );
                        tokio::time::sleep(Duration::from_millis(10)).await;
                    }
                }
            }
        }
        emitted
    }

    /// Perform one final full eviction pass, emitting every remaining flow.
    /// Used during shutdown so buffered flows are not silently dropped.
    pub async fn drain_final(&self) -> u64 {
        let now_ms = self
            .clock
            .eviction_now()
            .map(|now| now.epoch_millis)
            .unwrap_or_else(|_| {
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .map(|d| d.as_millis() as u64)
                    .unwrap_or(0)
            });
        let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
        let active_timeout_ms = self.config.active_timeout.as_millis() as u64;
        let flow_count_before = self.flow_table.len();
        let evicted = self.flow_table.evict_expired(|_, _, _| true);
        let emitted = self
            .emit_evicted(evicted, now_ms, idle_timeout_ms, active_timeout_ms)
            .await;
        metrics::ACTIVE_FLOWS.set(self.flow_table.len() as f64);
        if flow_count_before > 0 {
            info!(
                "Final eviction drain: before={}, evicted_and_emitted={}, remaining={}",
                flow_count_before,
                emitted,
                self.flow_table.len()
            );
        }
        emitted
    }
    fn eviction_reason(
        &self,
        value: &FlowValue,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
    ) -> &'static str {
        let start_time = value.start_time.load(Ordering::Relaxed);
        let last_seen = value.last_seen.load(Ordering::Relaxed);
        let idle_ms = now_ms.saturating_sub(last_seen);
        let active_ms = now_ms.saturating_sub(start_time);
        if idle_ms > idle_timeout_ms {
            return "idle_timeout";
        }
        if active_ms > active_timeout_ms {
            return "active_timeout";
        }
        let flags_fwd = value.tcp_flags_fwd.load(Ordering::Relaxed);
        let flags_bwd = value.tcp_flags_bwd.load(Ordering::Relaxed);
        let has_fin_fwd = (flags_fwd & tcp_flags::FIN as u16) != 0;
        let has_fin_bwd = (flags_bwd & tcp_flags::FIN as u16) != 0;
        let has_rst_fwd = (flags_fwd & tcp_flags::RST as u16) != 0;
        let has_rst_bwd = (flags_bwd & tcp_flags::RST as u16) != 0;
        if has_rst_fwd || has_rst_bwd {
            return "tcp_rst";
        }
        if has_fin_fwd && has_fin_bwd {
            return "tcp_fin_both";
        }
        if has_fin_fwd || has_fin_bwd {
            return "tcp_fin_one";
        }
        "unknown"
    }
    fn evict_fullscan(
        &self,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
    ) -> Vec<(FlowKey, FlowValue)> {
        self.flow_table.evict_expired(|_key, value, _now| {
            self.should_evict(value, now_ms, idle_timeout_ms, active_timeout_ms)
        })
    }
    fn should_evict(
        &self,
        value: &FlowValue,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
    ) -> bool {
        let start_time = value.start_time.load(Ordering::Relaxed);
        let last_seen = value.last_seen.load(Ordering::Relaxed);
        if last_seen == 0 || start_time == 0 {
            return false;
        }
        if last_seen > now_ms + 60_000 {
            warn!(
                "Flow has future timestamp: last_seen={}, now_ms={}, diff={}ms",
                last_seen,
                now_ms,
                last_seen - now_ms
            );
            return false;
        }
        let idle_ms = now_ms.saturating_sub(last_seen);
        let active_ms = now_ms.saturating_sub(start_time);
        if idle_ms > idle_timeout_ms {
            return true;
        }
        if active_ms > active_timeout_ms {
            return true;
        }
        let flags_fwd = value.tcp_flags_fwd.load(Ordering::Relaxed);
        let flags_bwd = value.tcp_flags_bwd.load(Ordering::Relaxed);
        let has_fin_fwd = (flags_fwd & tcp_flags::FIN as u16) != 0;
        let has_fin_bwd = (flags_bwd & tcp_flags::FIN as u16) != 0;
        let has_rst_fwd = (flags_fwd & tcp_flags::RST as u16) != 0;
        let has_rst_bwd = (flags_bwd & tcp_flags::RST as u16) != 0;
        if (has_rst_fwd || has_rst_bwd) && idle_ms > 1_000 {
            return true;
        }
        if has_fin_fwd && has_fin_bwd && idle_ms > 10_000 {
            return true;
        }
        if (has_fin_fwd || has_fin_bwd) && idle_ms > 30_000 {
            return true;
        }
        false
    }
    fn to_flow_event(&self, snapshot: &FlowSnapshot) -> FlowEvent {
        let normalized = snapshot.key.normalize();
        let community_id = snapshot.community_id.clone();
        let ts_start = snapshot.ts_start as i64;
        let ts_end = snapshot.ts_end as i64;
        let produced_at = chrono::Utc::now().timestamp_millis();
        let identity = FlowEventIdentity::derive(&self.config, snapshot);
        let iat_fwd_count = snapshot.iat_fwd_count;
        let iat_bwd_count = snapshot.iat_bwd_count;
        let total_iat_count = iat_fwd_count + iat_bwd_count;
        let iat_stats = if total_iat_count > 0 {
            let fwd_sum = snapshot.iat_fwd_sum;
            let bwd_sum = snapshot.iat_bwd_sum;
            let total_sum = fwd_sum + bwd_sum;
            let mean = total_sum as f32 / total_iat_count as f32 / 1000.0;
            let min_fwd = snapshot.iat_fwd_min;
            let min_bwd = snapshot.iat_bwd_min;
            let max_fwd = snapshot.iat_fwd_max;
            let max_bwd = snapshot.iat_bwd_max;
            let min = if min_fwd == 0 {
                min_bwd
            } else if min_bwd == 0 {
                min_fwd
            } else {
                min_fwd.min(min_bwd)
            };
            let max = max_fwd.max(max_bwd);
            let std_fwd = snapshot.iat_fwd_std;
            let std_bwd = snapshot.iat_bwd_std;
            let std = if iat_fwd_count > 0 && iat_bwd_count > 0 {
                ((std_fwd * std_fwd * iat_fwd_count as f32
                    + std_bwd * std_bwd * iat_bwd_count as f32)
                    / total_iat_count as f32)
                    .sqrt()
                    / 1000.0
            } else if iat_fwd_count > 0 {
                std_fwd / 1000.0
            } else {
                std_bwd / 1000.0
            };
            Some(InterArrivalStats {
                min_ms: min as f32 / 1000.0,
                max_ms: max as f32 / 1000.0,
                mean_ms: mean,
                std_ms: std,
            })
        } else {
            None
        };
        let pktlen_min = snapshot.pktlen_min;
        let pktlen_max = snapshot.pktlen_max;
        let pktlen_mean = snapshot.pktlen_mean;
        let pktlen_std = snapshot.pktlen_std;
        let duration_ms = snapshot.ts_end.saturating_sub(snapshot.ts_start) as u32;
        let total_packets = snapshot.packets_fwd + snapshot.packets_bwd;
        let total_bytes = snapshot.bytes_fwd + snapshot.bytes_bwd;
        let (pps, bps) = if duration_ms > 0 {
            let duration_sec = duration_ms as f32 / 1000.0;
            (
                total_packets as f32 / duration_sec,
                (total_bytes as f32 * 8.0) / duration_sec,
            )
        } else {
            (0.0, 0.0)
        };
        let active_stats = if snapshot.active.count > 0 {
            Some(ActiveIdleStats {
                min_ms: snapshot.active.min as f32 / 1000.0,
                mean_ms: snapshot.active.mean / 1000.0,
                max_ms: snapshot.active.max as f32 / 1000.0,
                std_ms: snapshot.active.std / 1000.0,
            })
        } else {
            None
        };
        let idle_stats = if snapshot.idle.count > 0 {
            Some(ActiveIdleStats {
                min_ms: snapshot.idle.min as f32 / 1000.0,
                mean_ms: snapshot.idle.mean / 1000.0,
                max_ms: snapshot.idle.max as f32 / 1000.0,
                std_ms: snapshot.idle.std / 1000.0,
            })
        } else {
            None
        };
        let feature_observation = Some(map_feature_observation(&snapshot.feature));
        FlowEvent {
            header: Some(EventHeader {
                event_id: identity.event_id.clone(),
                tenant_id: self.config.tenant_id.clone(),
                run_id: self.config.run_id.clone(),
                event_ts: ts_end,
                ingest_ts: produced_at,
                probe_id: self.config.probe_id.clone(),
                feature_set_id: self.config.feature_set_id.clone(),
                kafka_ts: 0,
                flink_out_ts: 0,
                event_type: "traffic.flow.v1".to_string(),
                schema_version: "1".to_string(),
                aggregate_type: "flow".to_string(),
                aggregate_id: identity.flow_id.clone(),
                aggregate_version: 1,
                occurred_at: ts_end,
                produced_at,
                trace_id: identity.event_id.clone(),
                causation_id: identity.event_id.clone(),
                correlation_id: community_id.clone(),
                idempotency_key: identity.idempotency_key,
                producer: "probe-agent".to_string(),
            }),
            flow_id: identity.flow_id,
            community_id,
            tuple: Some(FiveTuple {
                src_ip: normalized.src_ip.to_string(),
                dst_ip: normalized.dst_ip.to_string(),
                src_port: normalized.src_port as u32,
                dst_port: normalized.dst_port as u32,
                protocol: normalized.protocol as u32,
            }),
            direction: "bidirectional".to_string(),
            ts_start,
            ts_end,
            duration_ms,
            packets_fwd: snapshot.packets_fwd as u32,
            packets_bwd: snapshot.packets_bwd as u32,
            bytes_fwd: snapshot.bytes_fwd,
            bytes_bwd: snapshot.bytes_bwd,
            pps,
            bps,
            pktlen_stats: Some(PacketLengthStats {
                min: pktlen_min,
                max: pktlen_max,
                mean: pktlen_mean,
                std: pktlen_std,
            }),
            iat_stats,
            tcp_flags_fwd: snapshot.tcp_flags_fwd as u32,
            tcp_flags_bwd: snapshot.tcp_flags_bwd as u32,
            tos: snapshot.tos as u32,
            active_stats,
            idle_stats,
            subflow_count: 1,
            identity_revision: identity.revision,
            feature_observation,
        }
    }
}

fn map_feature_observation(snapshot: &FlowFeatureSnapshot) -> TrafficFeatureObservation {
    let security = &snapshot.security;
    let mut missing_fields = Vec::new();
    if snapshot.signed_packet_lengths.is_empty() {
        missing_fields.push("signed_packet_lengths".to_string());
    }
    if snapshot.payload_observed_bytes == 0 {
        missing_fields.push("payload_bytes".to_string());
    }
    if snapshot.sequence_truncated {
        missing_fields.push("sequence_truncated".to_string());
    }
    if security.truncated {
        missing_fields.push("security_handshake_truncated".to_string());
    }
    if security.conflict {
        missing_fields.push("security_observation_conflict".to_string());
    }
    match security.protocol {
        Some(crate::parser::security::ObservedSecurityProtocol::Tls) => {
            if security.ja3.is_none() {
                missing_fields.push("ja3".to_string());
            }
            if security.ja4.is_none() {
                missing_fields.push("ja4".to_string());
            }
            if security.sni.is_none() {
                missing_fields.push("sni".to_string());
            }
            if security.cert_sha256.is_none() {
                missing_fields.push("certificate".to_string());
            }
        }
        Some(crate::parser::security::ObservedSecurityProtocol::Quic) => {
            // QUIC Initial decryption/reassembly is not part of this packet-only parser.
            missing_fields.push("quic_client_hello".to_string());
        }
        None => missing_fields.push("transport_security".to_string()),
    }
    missing_fields.push("raw_traffic_ref".to_string());
    missing_fields.sort();
    missing_fields.dedup();
    TrafficFeatureObservation {
        schema_version: "traffic-feature-observation/v1".to_string(),
        algorithm_version: "probe-packet-feature/v1".to_string(),
        signed_packet_lengths: snapshot.signed_packet_lengths.clone(),
        packet_event_time_us: snapshot.packet_event_time_us.clone(),
        payload_nibble_counts: snapshot.payload_nibble_counts.clone(),
        payload_observed_bytes: snapshot.payload_observed_bytes,
        sequence_truncated: snapshot.sequence_truncated,
        transport_security: match security.protocol {
            Some(crate::parser::security::ObservedSecurityProtocol::Tls) => {
                TransportSecurityProtocol::Tls as i32
            }
            Some(crate::parser::security::ObservedSecurityProtocol::Quic) => {
                TransportSecurityProtocol::Quic as i32
            }
            None => TransportSecurityProtocol::Unspecified as i32,
        },
        tls_version: security.tls_version.clone().unwrap_or_default(),
        ja3: security.ja3.clone().unwrap_or_default(),
        ja4: security.ja4.clone().unwrap_or_default(),
        sni: security.sni.clone().unwrap_or_default(),
        cert_sha256: security.cert_sha256.clone().unwrap_or_default(),
        cert_is_self_signed: security.cert_is_self_signed.unwrap_or(false),
        cert_is_self_signed_known: security.cert_is_self_signed.is_some(),
        pubkey_len: security.pubkey_len.unwrap_or(0),
        pubkey_len_known: security.pubkey_len.is_some(),
        quic_version: security.quic_version.clone().unwrap_or_default(),
        raw_traffic_ref: String::new(),
        missing_fields,
    }
}

#[cfg(test)]
mod flow_event_mapping_tests {
    use super::*;
    use crate::aggregator::{
        canonicalize_observation, ObservationScope, ObservedEndpoints, PacketDirection,
    };
    use std::net::IpAddr;

    fn snapshot() -> FlowSnapshot {
        let identity = canonicalize_observation(ObservedEndpoints {
            src_ip: "192.0.2.10".parse::<IpAddr>().unwrap(),
            dst_ip: "198.51.100.20".parse::<IpAddr>().unwrap(),
            src_port: 50_000,
            dst_port: 443,
            protocol: 6,
        })
        .unwrap();
        let key = FlowKey::new(&identity, &ObservationScope::global_l3());
        let value = FlowValue::default();
        value
            .apply_event_time(1_700_000_000_123_000, PacketDirection::Forward)
            .unwrap();
        value.packets_fwd.store(1, Ordering::Release);
        value.bytes_fwd.store(74, Ordering::Release);
        value.pktlen_stats.update(74);
        FlowSnapshot::try_from_removed(RemovedFlow { key, value }).unwrap()
    }

    fn mapper() -> Eviction {
        let (tx, _rx) = tokio::sync::mpsc::channel(1);
        Eviction::new(
            EvictionConfig {
                tenant_id: "tenant-a".to_string(),
                probe_id: "probe-a".to_string(),
                run_id: "replay-a".to_string(),
                feature_set_id: "v1".to_string(),
                use_timewheel: false,
                ..EvictionConfig::default()
            },
            Arc::new(PartitionedFlowTable::new(1, 16)),
            tx,
        )
    }

    fn clear_processing_telemetry(event: &mut FlowEvent) {
        let header = event.header.as_mut().unwrap();
        header.ingest_ts = 0;
        header.produced_at = 0;
    }

    #[test]
    fn absent_interval_statistics_remain_absent() {
        let event = mapper().to_flow_event(&snapshot());
        assert!(event.pktlen_stats.is_some());
        assert!(event.iat_stats.is_none());
        assert!(event.active_stats.is_none());
        assert!(event.idle_stats.is_none());
        assert_eq!(event.direction, "bidirectional");
        assert_eq!(event.tuple.as_ref().unwrap().protocol, 6);
    }

    #[test]
    fn replay_semantic_projection_is_stable() {
        let mapper = mapper();
        let mut first = mapper.to_flow_event(&snapshot());
        let mut replay = mapper.to_flow_event(&snapshot());
        clear_processing_telemetry(&mut first);
        clear_processing_telemetry(&mut replay);
        assert_eq!(first, replay);
    }
}
