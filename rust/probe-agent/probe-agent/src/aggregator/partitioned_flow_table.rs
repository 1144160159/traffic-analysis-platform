use super::flow_table::{FlowKey, FlowTable, FlowValue, PacketInfo, UpdateResult};
use std::sync::atomic::{AtomicU64, Ordering};

pub struct PartitionedFlowTable {
    partitions: Vec<FlowTable>,
    num_partitions: usize,
    capacity_per_partition: usize,
    stats: PartitionedStats,
}

struct PartitionedStats {
    updates: AtomicU64,
    new_flows: AtomicU64,
    evictions: AtomicU64,
}

impl PartitionedFlowTable {
    pub fn new(num_partitions: usize, capacity_per_partition: usize) -> Self {
        assert!(num_partitions > 0, "num_partitions must be > 0");
        let num_partitions = num_partitions.next_power_of_two();

        let partitions = (0..num_partitions)
            .map(|_| FlowTable::new(capacity_per_partition))
            .collect();

        tracing::info!(
            "Created partitioned flow table: {} partitions × {} capacity = {} total",
            num_partitions,
            capacity_per_partition,
            num_partitions * capacity_per_partition
        );

        Self {
            partitions,
            num_partitions,
            capacity_per_partition,
            stats: PartitionedStats {
                updates: AtomicU64::new(0),
                new_flows: AtomicU64::new(0),
                evictions: AtomicU64::new(0),
            },
        }
    }

    pub fn auto(total_capacity: usize) -> Self {
        let num_cpus = num_cpus::get();
        let num_partitions = (num_cpus as u32).next_power_of_two() as usize;
        let capacity_per_partition = total_capacity / num_partitions;
        Self::new(num_partitions, capacity_per_partition)
    }

    #[inline]
    fn partition_for(&self, key: &FlowKey) -> usize {
        (key.cached_hash() as usize) & (self.num_partitions - 1)
    }

    #[inline]
    pub fn update(&self, key: &FlowKey, packet: &PacketInfo) -> UpdateResult {
        let partition_idx = self.partition_for(key);
        let result = self.partitions[partition_idx].update(key, packet);

        self.stats.updates.fetch_add(1, Ordering::Relaxed);
        if matches!(result, UpdateResult::NewFlow) {
            self.stats.new_flows.fetch_add(1, Ordering::Relaxed);
        }

        result
    }

    #[inline]
    pub fn update_with_time(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
        now_ms: u64,
    ) -> UpdateResult {
        let partition_idx = self.partition_for(key);
        let result = self.partitions[partition_idx].update_with_time(key, packet, now_ms);

        self.stats.updates.fetch_add(1, Ordering::Relaxed);
        if matches!(result, UpdateResult::NewFlow) {
            self.stats.new_flows.fetch_add(1, Ordering::Relaxed);
        }

        result
    }

    pub fn update_with_value(
        &self,
        key: &FlowKey,
        value: FlowValue,
        packet: &PacketInfo,
    ) -> UpdateResult {
        let partition_idx = self.partition_for(key);
        let partition = &self.partitions[partition_idx];

        value.dscp_bitmap.store(0, Ordering::Relaxed);

        if let Some(_old_value) = partition.insert_with_value(key.clone(), value) {
            partition.update(key, packet);
            self.stats.updates.fetch_add(1, Ordering::Relaxed);
            UpdateResult::Updated
        } else {
            partition.update(key, packet);
            self.stats.updates.fetch_add(1, Ordering::Relaxed);
            self.stats.new_flows.fetch_add(1, Ordering::Relaxed);
            UpdateResult::NewFlow
        }
    }

    #[inline]
    pub fn update_batch(&self, updates: &[(FlowKey, PacketInfo)]) {
        for (key, packet) in updates {
            self.update(key, packet);
        }
    }

    pub fn remove(&self, key: &FlowKey) -> Option<(FlowKey, FlowValue)> {
        let partition_idx = self.partition_for(key);
        let result = self.partitions[partition_idx].remove(key);
        if result.is_some() {
            self.stats.evictions.fetch_add(1, Ordering::Relaxed);
        }
        result
    }

    pub fn len(&self) -> usize {
        self.partitions.iter().map(|p| p.len()).sum()
    }

    pub fn is_empty(&self) -> bool {
        self.partitions.iter().all(|p| p.len() == 0)
    }

    pub fn capacity(&self) -> usize {
        self.num_partitions * self.capacity_per_partition
    }

    pub fn num_partitions(&self) -> usize {
        self.num_partitions
    }

    pub fn partition_sizes(&self) -> Vec<usize> {
        self.partitions.iter().map(|p| p.len()).collect()
    }

    pub fn evict_expired<F>(&self, should_evict: F) -> Vec<(FlowKey, FlowValue)>
    where
        F: Fn(&FlowKey, &FlowValue, u64) -> bool + Sync,
    {
        use rayon::prelude::*;

        let now_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        self.partitions
            .par_iter()
            .flat_map(|partition| {
                let keys_to_evict: Vec<FlowKey> = partition
                    .iter()
                    .filter_map(|entry| {
                        if should_evict(entry.key(), entry.value(), now_ms) {
                            Some(entry.key().clone())
                        } else {
                            None
                        }
                    })
                    .collect();

                let mut evicted = Vec::with_capacity(keys_to_evict.len());
                for key in keys_to_evict {
                    if let Some(kv) = partition.remove(&key) {
                        evicted.push(kv);
                    }
                }
                evicted
            })
            .collect()
    }

    pub fn evict_sampled<F>(&self, should_evict: F, sample_rate: f64) -> Vec<(FlowKey, FlowValue)>
    where
        F: Fn(&FlowKey, &FlowValue, u64) -> bool + Sync,
    {
        use rand::Rng;
        use rayon::prelude::*;

        let now_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        self.partitions
            .par_iter()
            .flat_map(|partition| {
                let mut rng = rand::thread_rng();
                let keys_to_evict: Vec<FlowKey> = partition
                    .iter()
                    .filter(|_| rng.gen::<f64>() < sample_rate)
                    .filter_map(|entry| {
                        if should_evict(entry.key(), entry.value(), now_ms) {
                            Some(entry.key().clone())
                        } else {
                            None
                        }
                    })
                    .collect();

                let mut evicted = Vec::with_capacity(keys_to_evict.len());
                for key in keys_to_evict {
                    if let Some(kv) = partition.remove(&key) {
                        evicted.push(kv);
                    }
                }
                evicted
            })
            .collect()
    }

    pub fn iter(
        &self,
    ) -> impl Iterator<Item = dashmap::mapref::multiple::RefMulti<'_, FlowKey, FlowValue>> + '_
    {
        self.partitions.iter().flat_map(|p| p.iter())
    }

    pub fn stats(&self) -> PartitionedFlowTableStats {
        PartitionedFlowTableStats {
            total_flows: self.len(),
            num_partitions: self.num_partitions,
            partition_sizes: self.partition_sizes(),
            updates: self.stats.updates.load(Ordering::Relaxed),
            new_flows: self.stats.new_flows.load(Ordering::Relaxed),
            evictions: self.stats.evictions.load(Ordering::Relaxed),
        }
    }

    pub fn partition(&self, idx: usize) -> Option<&FlowTable> {
        self.partitions.get(idx)
    }

    pub fn partition_index(&self, key: &FlowKey) -> usize {
        self.partition_for(key)
    }

    pub fn clear(&self) {
        for partition in &self.partitions {
            partition.clear();
        }
    }

    pub fn fragmentation(&self) -> f64 {
        let sizes = self.partition_sizes();
        if sizes.is_empty() || self.len() == 0 {
            return 0.0;
        }

        let max = *sizes.iter().max().unwrap() as f64;
        let min = *sizes.iter().min().unwrap() as f64;
        let avg = sizes.iter().sum::<usize>() as f64 / sizes.len() as f64;

        if avg > 0.0 {
            (max - min) / avg
        } else {
            0.0
        }
    }

    pub fn utilization(&self) -> f64 {
        let total_capacity = self.capacity();
        if total_capacity == 0 {
            return 0.0;
        }
        self.len() as f64 / total_capacity as f64
    }

    pub fn contains_key(&self, key: &FlowKey) -> bool {
        let partition_idx = self.partition_for(key);
        self.partitions[partition_idx].contains_key(key)
    }

    pub fn get(&self, key: &FlowKey) -> Option<dashmap::mapref::one::Ref<'_, FlowKey, FlowValue>> {
        let partition_idx = self.partition_for(key);
        self.partitions[partition_idx].get(key)
    }
}

#[derive(Debug, Clone)]
pub struct PartitionedFlowTableStats {
    pub total_flows: usize,
    pub num_partitions: usize,
    pub partition_sizes: Vec<usize>,
    pub updates: u64,
    pub new_flows: u64,
    pub evictions: u64,
}

impl PartitionedFlowTableStats {
    pub fn load_balance_factor(&self) -> f64 {
        if self.partition_sizes.is_empty() || self.total_flows == 0 {
            return 0.0;
        }

        let mean = self.total_flows as f64 / self.num_partitions as f64;
        let variance: f64 = self
            .partition_sizes
            .iter()
            .map(|&s| (s as f64 - mean).powi(2))
            .sum::<f64>()
            / self.num_partitions as f64;

        if mean > 0.0 {
            variance.sqrt() / mean
        } else {
            0.0
        }
    }

    pub fn max_partition_size(&self) -> usize {
        self.partition_sizes.iter().copied().max().unwrap_or(0)
    }

    pub fn min_partition_size(&self) -> usize {
        self.partition_sizes.iter().copied().min().unwrap_or(0)
    }

    pub fn avg_partition_size(&self) -> f64 {
        if self.num_partitions == 0 {
            return 0.0;
        }
        self.total_flows as f64 / self.num_partitions as f64
    }

    pub fn utilization(&self, total_capacity: usize) -> f64 {
        if total_capacity == 0 {
            return 0.0;
        }
        self.total_flows as f64 / total_capacity as f64
    }
}
