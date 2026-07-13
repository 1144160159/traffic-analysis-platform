use super::flow_table::{FlowKey, FlowValue, PacketInfo, UpdateResult};
use super::partitioned_flow_table::PartitionedFlowTable;
use parking_lot::Mutex;
use std::sync::Arc;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};
use tracing::{debug, info};

#[derive(Clone, Debug)]
pub struct GenerationalConfig {
    pub young_capacity: usize,
    pub old_capacity: usize,
    pub tenured_capacity: usize,
    pub promotion_threshold: Duration,
    pub tenuring_threshold: Duration,
    pub young_scan_interval: Duration,
    pub old_scan_interval: Duration,
    pub tenured_scan_interval: Duration,
    pub idle_timeout: Duration,
    pub active_timeout: Duration,
}

impl Default for GenerationalConfig {
    fn default() -> Self {
        Self {
            young_capacity: 500_000,
            old_capacity: 300_000,
            tenured_capacity: 200_000,
            promotion_threshold: Duration::from_secs(5 * 60),
            tenuring_threshold: Duration::from_secs(30 * 60),
            young_scan_interval: Duration::from_secs(1),
            old_scan_interval: Duration::from_secs(10),
            tenured_scan_interval: Duration::from_secs(60),
            idle_timeout: Duration::from_secs(120),
            active_timeout: Duration::from_secs(1800),
        }
    }
}

#[derive(Debug, Clone)]
pub struct GenerationalFlowTableStats {
    pub young_flows: usize,
    pub old_flows: usize,
    pub tenured_flows: usize,
    pub promotions_to_old: u64,
    pub promotions_to_tenured: u64,
    pub demotions_to_young: u64,
    pub young_evicted: u64,
    pub old_evicted: u64,
    pub tenured_evicted: u64,
    pub total_evicted: u64,
}

impl GenerationalFlowTableStats {
    pub fn total_flows(&self) -> usize {
        self.young_flows + self.old_flows + self.tenured_flows
    }

    pub fn young_ratio(&self) -> f64 {
        let total = self.total_flows();
        if total == 0 {
            0.0
        } else {
            self.young_flows as f64 / total as f64
        }
    }

    pub fn old_ratio(&self) -> f64 {
        let total = self.total_flows();
        if total == 0 {
            0.0
        } else {
            self.old_flows as f64 / total as f64
        }
    }

    pub fn tenured_ratio(&self) -> f64 {
        let total = self.total_flows();
        if total == 0 {
            0.0
        } else {
            self.tenured_flows as f64 / total as f64
        }
    }
}

impl std::fmt::Display for GenerationalFlowTableStats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "Flows[Y:{} O:{} T:{}] Promotions[O:{} T:{}] Demotions[Y:{}] Evicted[Y:{} O:{} T:{}]",
            self.young_flows,
            self.old_flows,
            self.tenured_flows,
            self.promotions_to_old,
            self.promotions_to_tenured,
            self.demotions_to_young,
            self.young_evicted,
            self.old_evicted,
            self.tenured_evicted
        )
    }
}

pub struct GenerationalFlowTable {
    young: Arc<PartitionedFlowTable>,
    old: Arc<PartitionedFlowTable>,
    tenured: Arc<PartitionedFlowTable>,
    config: GenerationalConfig,
    last_young_scan: Mutex<Instant>,
    last_old_scan: Mutex<Instant>,
    last_tenured_scan: Mutex<Instant>,
    promotions_to_old: Mutex<u64>,
    promotions_to_tenured: Mutex<u64>,
    demotions_to_young: Mutex<u64>,
    young_evicted: Mutex<u64>,
    old_evicted: Mutex<u64>,
    tenured_evicted: Mutex<u64>,
}

impl GenerationalFlowTable {
    pub fn new(num_partitions: usize, config: GenerationalConfig) -> Self {
        let young_per_partition = config.young_capacity / num_partitions;
        let old_per_partition = config.old_capacity / num_partitions;
        let tenured_per_partition = config.tenured_capacity / num_partitions;

        info!(
            "Creating generational flow table: young={}K, old={}K, tenured={}K",
            config.young_capacity / 1000,
            config.old_capacity / 1000,
            config.tenured_capacity / 1000
        );

        Self {
            young: Arc::new(PartitionedFlowTable::new(
                num_partitions,
                young_per_partition,
            )),
            old: Arc::new(PartitionedFlowTable::new(num_partitions, old_per_partition)),
            tenured: Arc::new(PartitionedFlowTable::new(
                num_partitions,
                tenured_per_partition,
            )),
            config,
            last_young_scan: Mutex::new(Instant::now()),
            last_old_scan: Mutex::new(Instant::now()),
            last_tenured_scan: Mutex::new(Instant::now()),
            promotions_to_old: Mutex::new(0),
            promotions_to_tenured: Mutex::new(0),
            demotions_to_young: Mutex::new(0),
            young_evicted: Mutex::new(0),
            old_evicted: Mutex::new(0),
            tenured_evicted: Mutex::new(0),
        }
    }

    pub fn with_config(config: GenerationalConfig) -> Self {
        let num_partitions = num_cpus::get().max(4).next_power_of_two();
        Self::new(num_partitions, config)
    }

    pub fn evict(&self) -> Vec<(FlowKey, FlowValue)> {
        let now = Instant::now();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let mut evicted = Vec::new();

        let should_scan_young = {
            let last = self.last_young_scan.lock();
            now.duration_since(*last) >= self.config.young_scan_interval
        };

        if should_scan_young {
            let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
            let active_timeout_ms = self.config.active_timeout.as_millis() as u64;

            let young_evicted_flows: Vec<(FlowKey, FlowValue)> = self
                .young
                .iter()
                .filter_map(|entry| {
                    let value = entry.value();
                    let last_seen = value.last_seen.load(std::sync::atomic::Ordering::Relaxed);
                    let start_time = value.start_time.load(std::sync::atomic::Ordering::Relaxed);

                    let idle_ms = now_ms.saturating_sub(last_seen);
                    let active_ms = now_ms.saturating_sub(start_time);

                    if idle_ms > idle_timeout_ms || active_ms > active_timeout_ms {
                        Some(entry.key().clone())
                    } else {
                        None
                    }
                })
                .filter_map(|key| self.young.remove(&key))
                .collect();

            let count = young_evicted_flows.len();
            if count > 0 {
                *self.young_evicted.lock() += count as u64;
                debug!("Evicted {} flows from Young generation", count);
            }

            evicted.extend(young_evicted_flows);
            *self.last_young_scan.lock() = now;
        }

        let should_scan_old = {
            let last = self.last_old_scan.lock();
            now.duration_since(*last) >= self.config.old_scan_interval
        };

        if should_scan_old {
            let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
            let active_timeout_ms = self.config.active_timeout.as_millis() as u64;

            let old_evicted_flows: Vec<(FlowKey, FlowValue)> = self
                .old
                .iter()
                .filter_map(|entry| {
                    let value = entry.value();
                    let last_seen = value.last_seen.load(std::sync::atomic::Ordering::Relaxed);
                    let start_time = value.start_time.load(std::sync::atomic::Ordering::Relaxed);

                    let idle_ms = now_ms.saturating_sub(last_seen);
                    let active_ms = now_ms.saturating_sub(start_time);

                    if idle_ms > idle_timeout_ms || active_ms > active_timeout_ms {
                        Some(entry.key().clone())
                    } else {
                        None
                    }
                })
                .filter_map(|key| self.old.remove(&key))
                .collect();

            let count = old_evicted_flows.len();
            if count > 0 {
                *self.old_evicted.lock() += count as u64;
                debug!("Evicted {} flows from Old generation", count);
            }

            evicted.extend(old_evicted_flows);
            *self.last_old_scan.lock() = now;
        }

        let should_scan_tenured = {
            let last = self.last_tenured_scan.lock();
            now.duration_since(*last) >= self.config.tenured_scan_interval
        };

        if should_scan_tenured {
            let idle_timeout_ms = self.config.idle_timeout.as_millis() as u64;
            let active_timeout_ms = self.config.active_timeout.as_millis() as u64;

            let tenured_evicted_flows: Vec<(FlowKey, FlowValue)> = self
                .tenured
                .iter()
                .filter_map(|entry| {
                    let value = entry.value();
                    let last_seen = value.last_seen.load(std::sync::atomic::Ordering::Relaxed);
                    let start_time = value.start_time.load(std::sync::atomic::Ordering::Relaxed);

                    let idle_ms = now_ms.saturating_sub(last_seen);
                    let active_ms = now_ms.saturating_sub(start_time);

                    if idle_ms > idle_timeout_ms || active_ms > active_timeout_ms {
                        Some(entry.key().clone())
                    } else {
                        None
                    }
                })
                .filter_map(|key| self.tenured.remove(&key))
                .collect();

            let count = tenured_evicted_flows.len();
            if count > 0 {
                *self.tenured_evicted.lock() += count as u64;
                debug!("Evicted {} flows from Tenured generation", count);
            }

            evicted.extend(tenured_evicted_flows);
            *self.last_tenured_scan.lock() = now;
        }

        evicted
    }

    pub fn update(&self, key: &FlowKey, packet: &PacketInfo) -> UpdateResult {
        if self.young.contains_key(key) {
            return self.young.update(key, packet);
        }

        if self.old.contains_key(key) {
            if let Some((k, v)) = self.old.remove(key) {
                *self.demotions_to_young.lock() += 1;
                debug!("Demoting hot flow from Old to Young: {:?}", k);
                let dummy_packet = PacketInfo::default();
                self.young.update_with_value(&k, v, &dummy_packet);
                return self.young.update(&k, packet);
            }
        }

        if self.tenured.contains_key(key) {
            if let Some((k, v)) = self.tenured.remove(key) {
                *self.demotions_to_young.lock() += 1;
                debug!("Demoting hot flow from Tenured to Young: {:?}", k);
                let dummy_packet = PacketInfo::default();
                self.young.update_with_value(&k, v, &dummy_packet);
                return self.young.update(&k, packet);
            }
        }

        self.young.update(key, packet)
    }

    pub fn promote_to_old(&self) {
        let _now = Instant::now();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let promotion_threshold_ms = self.config.promotion_threshold.as_millis() as u64;

        let to_promote: Vec<FlowKey> = self
            .young
            .iter()
            .filter_map(|entry| {
                let start_time = entry
                    .value()
                    .start_time
                    .load(std::sync::atomic::Ordering::Relaxed);
                let age_ms = now_ms.saturating_sub(start_time);

                if age_ms > promotion_threshold_ms {
                    Some(entry.key().clone())
                } else {
                    None
                }
            })
            .collect();

        let count = to_promote.len();
        if count > 0 {
            for key in to_promote {
                if let Some((k, v)) = self.young.remove(&key) {
                    let dummy_packet = PacketInfo::default();
                    self.old.update_with_value(&k, v, &dummy_packet);
                }
            }
            *self.promotions_to_old.lock() += count as u64;
            debug!("Promoted {} flows from Young to Old", count);
        }
    }

    pub fn promote_to_tenured(&self) {
        let _now = Instant::now();
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let tenuring_threshold_ms = self.config.tenuring_threshold.as_millis() as u64;

        let to_promote: Vec<FlowKey> = self
            .old
            .iter()
            .filter_map(|entry| {
                let start_time = entry
                    .value()
                    .start_time
                    .load(std::sync::atomic::Ordering::Relaxed);
                let age_ms = now_ms.saturating_sub(start_time);

                if age_ms > tenuring_threshold_ms {
                    Some(entry.key().clone())
                } else {
                    None
                }
            })
            .collect();

        let count = to_promote.len();
        if count > 0 {
            for key in to_promote {
                if let Some((k, v)) = self.old.remove(&key) {
                    let dummy_packet = PacketInfo::default();
                    self.tenured.update_with_value(&k, v, &dummy_packet);
                }
            }
            *self.promotions_to_tenured.lock() += count as u64;
            debug!("Promoted {} flows from Old to Tenured", count);
        }
    }

    pub fn demote_to_young(&self, key: &FlowKey) {
        if let Some((k, v)) = self.old.remove(key) {
            let dummy_packet = PacketInfo::default();
            self.young.update_with_value(&k, v, &dummy_packet);
            *self.demotions_to_young.lock() += 1;
            return;
        }

        if let Some((k, v)) = self.tenured.remove(key) {
            let dummy_packet = PacketInfo::default();
            self.young.update_with_value(&k, v, &dummy_packet);
            *self.demotions_to_young.lock() += 1;
        }
    }

    pub fn stats(&self) -> GenerationalFlowTableStats {
        GenerationalFlowTableStats {
            young_flows: self.young.len(),
            old_flows: self.old.len(),
            tenured_flows: self.tenured.len(),
            promotions_to_old: *self.promotions_to_old.lock(),
            promotions_to_tenured: *self.promotions_to_tenured.lock(),
            demotions_to_young: *self.demotions_to_young.lock(),
            young_evicted: *self.young_evicted.lock(),
            old_evicted: *self.old_evicted.lock(),
            tenured_evicted: *self.tenured_evicted.lock(),
            total_evicted: *self.young_evicted.lock()
                + *self.old_evicted.lock()
                + *self.tenured_evicted.lock(),
        }
    }

    /// Returns a reference to the young (active) flow table for direct use by PacketProcessor
    pub fn young_table(&self) -> &Arc<PartitionedFlowTable> {
        &self.young
    }

    /// Returns the generational configuration
    pub fn gen_config(&self) -> &GenerationalConfig {
        &self.config
    }

    pub fn len(&self) -> usize {
        self.young.len() + self.old.len() + self.tenured.len()
    }

    pub fn is_empty(&self) -> bool {
        self.young.is_empty() && self.old.is_empty() && self.tenured.is_empty()
    }

    pub fn capacity(&self) -> usize {
        self.config.young_capacity + self.config.old_capacity + self.config.tenured_capacity
    }

    pub fn young_len(&self) -> usize {
        self.young.len()
    }

    pub fn old_len(&self) -> usize {
        self.old.len()
    }

    pub fn tenured_len(&self) -> usize {
        self.tenured.len()
    }

    pub fn young_capacity(&self) -> usize {
        self.config.young_capacity
    }

    pub fn old_capacity(&self) -> usize {
        self.config.old_capacity
    }

    pub fn tenured_capacity(&self) -> usize {
        self.config.tenured_capacity
    }

    pub fn young_utilization(&self) -> f64 {
        self.young.len() as f64 / self.config.young_capacity as f64
    }

    pub fn old_utilization(&self) -> f64 {
        self.old.len() as f64 / self.config.old_capacity as f64
    }

    pub fn tenured_utilization(&self) -> f64 {
        self.tenured.len() as f64 / self.config.tenured_capacity as f64
    }

    pub fn clear(&self) {
        self.young.clear();
        self.old.clear();
        self.tenured.clear();
    }
}
