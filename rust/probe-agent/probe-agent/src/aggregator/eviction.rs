use super::flow_table::{FlowKey, FlowValue};
use super::hierarchical_timewheel::HierarchicalTimeWheel;
use super::partitioned_flow_table::PartitionedFlowTable;
use crate::metrics;
use crate::parser::tcp_flags;
use proto_gen::{
    ActiveIdleStats, EventHeader, FiveTuple, FlowEvent, InterArrivalStats, PacketLengthStats,
};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use tokio::sync::mpsc::Sender;
use tokio::time::{interval, Duration, Instant};
use tracing::{debug, info, warn};

/// 淘汰原因 — 用于监控和分析流生命周期
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EvictionReason {
    IdleTimeout,   // 空闲超时 (默认120s)
    ActiveTimeout, // 活动超时 (默认1800s)
    ForcedCleanup, // 强制清理 (内存压力/关闭)
    TCPFlagFinish, // TCP FIN/RST 正常结束
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
    timewheel: Option<Arc<HierarchicalTimeWheel>>,
    pub stats: Arc<EvictionStats>,
}
impl Eviction {
    pub fn new(
        config: EvictionConfig,
        flow_table: Arc<PartitionedFlowTable>,
        output_tx: Sender<FlowEvent>,
    ) -> Self {
        let timewheel = if config.use_timewheel {
            Some(Arc::new(HierarchicalTimeWheel::new()))
        } else {
            None
        };
        info!(
            "Eviction created: idle={}s, active={}s, scan={}s, timewheel={}",
            config.idle_timeout.as_secs(),
            config.active_timeout.as_secs(),
            config.scan_interval.as_secs(),
            config.use_timewheel
        );
        let stats = Arc::new(EvictionStats::default());
        Self {
            config,
            flow_table,
            output_tx,
            timewheel,
            stats,
        }
    }
    pub async fn run(&self) {
        let mut ticker = interval(self.config.scan_interval);
        let mut total_evicted: u64 = 0;
        let mut last_log_time = Instant::now();
        let mut last_stats_time = Instant::now();
        info!(
            "Eviction started: idle={}s, active={}s, scan={}s, method={}",
            self.config.idle_timeout.as_secs(),
            self.config.active_timeout.as_secs(),
            self.config.scan_interval.as_secs(),
            if self.config.use_timewheel {
                "TimeWheel"
            } else {
                "FullScan"
            }
        );
        loop {
            ticker.tick().await;
            let now_ms = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64;
            let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
            let active_timeout_ms = self.config.active_timeout.as_millis() as u64;
            let eviction_start = Instant::now();
            let flow_count_before = self.flow_table.len();
            let evicted = if self.config.use_timewheel {
                self.evict_with_timewheel(now_ms, idle_timeout_ms, active_timeout_ms)
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
                let event = self.to_flow_event(&key, &value, now_ms);
                if let Err(e) = self.output_tx.send(event).await {
                    warn!("Failed to send flow event: {}", e);
                }
                total_evicted += 1;
                metrics::FLOWS_EVICTED.inc();
            }
            metrics::ACTIVE_FLOWS.set(self.flow_table.len() as f64);
            if last_log_time.elapsed() >= Duration::from_secs(30) {
                let stats_msg = if let Some(ref tw) = self.timewheel {
                    let tw_stats = tw.stats();
                    format!(
                        "TimeWheel: L0={}, L1={}, L2={}, ticks={}",
                        tw_stats.level0_flows,
                        tw_stats.level1_flows,
                        tw_stats.level2_flows,
                        tw_stats.tick_count
                    )
                } else {
                    "FullScan".to_string()
                };
                info!(
                    "Eviction summary: evicted={}, active_flows={}, total_evicted={}, last_duration={}ms, {}",
                    evicted_count,
                    self.flow_table.len(),
                    total_evicted,
                    eviction_duration.as_millis(),
                    stats_msg
                );
                last_log_time = Instant::now();
            }
            if last_stats_time.elapsed() >= Duration::from_secs(60) {
                if let Some(ref tw) = self.timewheel {
                    let stats = tw.stats();
                    debug!(
                        "TimeWheel stats: total={}, L0={}/{}, L1={}/{}, L2={}/{}, avg_flows={:.1}",
                        stats.total_flows,
                        stats.level0_flows,
                        stats.level0_slots_used,
                        stats.level1_flows,
                        stats.level1_slots_used,
                        stats.level2_flows,
                        stats.level2_slots_used,
                        stats.avg_flows_per_slot()
                    );
                }
                last_stats_time = Instant::now();
            }
        }
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
    fn evict_with_timewheel(
        &self,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
    ) -> Vec<(FlowKey, FlowValue)> {
        let tw = match self.timewheel.as_ref() {
            Some(tw) => tw,
            None => return self.evict_fullscan(now_ms, idle_timeout_ms, active_timeout_ms),
        };
        let expired_keys = tw.tick();
        let mut evicted = Vec::new();
        for key in expired_keys {
            if let Some((k, v)) = self.flow_table.remove(&key) {
                evicted.push((k, v));
            }
        }
        let sampled_evicted =
            self.evict_fullscan_sampled(now_ms, idle_timeout_ms, active_timeout_ms, 10);
        evicted.extend(sampled_evicted);
        evicted
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
    fn evict_fullscan_sampled(
        &self,
        now_ms: u64,
        idle_timeout_ms: u64,
        active_timeout_ms: u64,
        sample_rate: usize,
    ) -> Vec<(FlowKey, FlowValue)> {
        let mut evicted = Vec::new();
        let mut count = 0;
        for entry in self.flow_table.iter() {
            count += 1;
            if count % sample_rate != 0 {
                continue;
            }
            let key = entry.key();
            let value = entry.value();
            if self.should_evict(value, now_ms, idle_timeout_ms, active_timeout_ms) {
                if let Some(kv) = self.flow_table.remove(key) {
                    evicted.push(kv);
                }
            }
        }
        evicted
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
    fn to_flow_event(&self, key: &FlowKey, value: &FlowValue, _now_ms: u64) -> FlowEvent {
        let normalized = key.normalize();
        let iat_fwd_count = value.iat_fwd_stats.count();
        let iat_bwd_count = value.iat_bwd_stats.count();
        let total_iat_count = iat_fwd_count + iat_bwd_count;
        let (iat_mean_ms, iat_std_ms, iat_min_ms, iat_max_ms) = if total_iat_count > 0 {
            let fwd_sum = value.iat_fwd_stats.sum();
            let bwd_sum = value.iat_bwd_stats.sum();
            let total_sum = fwd_sum + bwd_sum;
            let mean = total_sum as f32 / total_iat_count as f32 / 1000.0;
            let min_fwd = value.iat_fwd_stats.min();
            let min_bwd = value.iat_bwd_stats.min();
            let max_fwd = value.iat_fwd_stats.max();
            let max_bwd = value.iat_bwd_stats.max();
            let min = if min_fwd == 0 {
                min_bwd
            } else if min_bwd == 0 {
                min_fwd
            } else {
                min_fwd.min(min_bwd)
            };
            let max = max_fwd.max(max_bwd);
            let std_fwd = value.iat_fwd_stats.std();
            let std_bwd = value.iat_bwd_stats.std();
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
            (mean, std, min as f32 / 1000.0, max as f32 / 1000.0)
        } else {
            (0.0, 0.0, 0.0, 0.0)
        };
        let pktlen_min = value.pktlen_stats.min();
        let pktlen_max = value.pktlen_stats.max();
        let pktlen_mean = value.pktlen_stats.mean();
        let pktlen_std = value.pktlen_stats.std();
        let duration_ms = value.duration_ms();
        let total_packets = value.total_packets();
        let total_bytes = value.total_bytes();
        let (pps, bps) = if duration_ms > 0 {
            let duration_sec = duration_ms as f32 / 1000.0;
            (
                total_packets as f32 / duration_sec,
                (total_bytes as f32 * 8.0) / duration_sec,
            )
        } else {
            (0.0, 0.0)
        };
        let active_stats = if value.active_stats.count() > 0 {
            Some(ActiveIdleStats {
                min_ms: value.active_stats.min_float(),
                mean_ms: value.active_stats.mean_float(),
                max_ms: value.active_stats.max_float(),
                std_ms: value.active_stats.std_float(),
            })
        } else {
            Some(ActiveIdleStats {
                min_ms: 0.0,
                mean_ms: 0.0,
                max_ms: 0.0,
                std_ms: 0.0,
            })
        };
        let idle_stats = if value.idle_stats.count() > 0 {
            Some(ActiveIdleStats {
                min_ms: value.idle_stats.min_float(),
                mean_ms: value.idle_stats.mean_float(),
                max_ms: value.idle_stats.max_float(),
                std_ms: value.idle_stats.std_float(),
            })
        } else {
            Some(ActiveIdleStats {
                min_ms: 0.0,
                mean_ms: 0.0,
                max_ms: 0.0,
                std_ms: 0.0,
            })
        };
        FlowEvent {
            header: Some(EventHeader {
                event_id: uuid::Uuid::new_v4().to_string(),
                tenant_id: self.config.tenant_id.clone(),
                run_id: self.config.run_id.clone(),
                event_ts: value.last_seen.load(Ordering::Relaxed) as i64,
                ingest_ts: chrono::Utc::now().timestamp_millis(),
                probe_id: self.config.probe_id.clone(),
                feature_set_id: self.config.feature_set_id.clone(),
            }),
            flow_id: uuid::Uuid::new_v4().to_string(),
            community_id: key.community_id().to_string(),
            tuple: Some(FiveTuple {
                src_ip: normalized.src_ip.to_string(),
                dst_ip: normalized.dst_ip.to_string(),
                src_port: normalized.src_port as u32,
                dst_port: normalized.dst_port as u32,
                protocol: normalized.protocol as u32,
            }),
            direction: "c2s".to_string(),
            ts_start: value.start_time.load(Ordering::Relaxed) as i64,
            ts_end: value.last_seen.load(Ordering::Relaxed) as i64,
            duration_ms,
            packets_fwd: value.packets_fwd.load(Ordering::Relaxed) as u32,
            packets_bwd: value.packets_bwd.load(Ordering::Relaxed) as u32,
            bytes_fwd: value.bytes_fwd.load(Ordering::Relaxed),
            bytes_bwd: value.bytes_bwd.load(Ordering::Relaxed),
            pps,
            bps,
            pktlen_stats: Some(PacketLengthStats {
                min: pktlen_min,
                max: pktlen_max,
                mean: pktlen_mean,
                std: pktlen_std,
            }),
            iat_stats: Some(InterArrivalStats {
                min_ms: iat_min_ms,
                max_ms: iat_max_ms,
                mean_ms: iat_mean_ms,
                std_ms: iat_std_ms,
            }),
            tcp_flags_fwd: value.tcp_flags_fwd.load(Ordering::Relaxed) as u32,
            tcp_flags_bwd: value.tcp_flags_bwd.load(Ordering::Relaxed) as u32,
            tos: value.tos.load(Ordering::Acquire) as u32,
            active_stats,
            idle_stats,
            subflow_count: 1,
        }
    }
}
