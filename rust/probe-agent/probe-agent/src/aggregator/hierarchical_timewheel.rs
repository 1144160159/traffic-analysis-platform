use super::flow_table::FlowKey;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::sync::RwLock;
use std::time::{Duration, SystemTime, UNIX_EPOCH};
use tracing::{debug, info, trace};

const LEVEL0_SLOTS: usize = 60;
const LEVEL0_DURATION_SECS: u64 = 1;

const LEVEL1_SLOTS: usize = 60;
const LEVEL1_DURATION_SECS: u64 = 60;

const LEVEL2_SLOTS: usize = 24;
const LEVEL2_DURATION_SECS: u64 = 3600;

#[derive(Debug)]
struct TimeWheelSlot {
    flows: Vec<FlowKey>,
    expire_time_ms: u64,
}

impl TimeWheelSlot {
    fn new(expire_time_ms: u64) -> Self {
        Self {
            flows: Vec::with_capacity(128),
            expire_time_ms,
        }
    }

    fn add_flow(&mut self, key: FlowKey) {
        self.flows.push(key);
    }

    fn drain(&mut self) -> Vec<FlowKey> {
        std::mem::take(&mut self.flows)
    }

    fn is_empty(&self) -> bool {
        self.flows.is_empty()
    }

    fn len(&self) -> usize {
        self.flows.len()
    }

    fn clear(&mut self) {
        self.flows.clear();
    }
}

struct TimeWheelLevel {
    slots: Vec<RwLock<TimeWheelSlot>>,
    current_slot: AtomicUsize,
    slot_duration: Duration,
    slot_count: usize,
    level_id: u8,
}

impl TimeWheelLevel {
    fn new(slot_count: usize, slot_duration: Duration, level_id: u8) -> Self {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let slot_duration_ms = slot_duration.as_millis() as u64;

        let slots = (0..slot_count)
            .map(|i| {
                let expire_time_ms = now_ms + (i as u64 * slot_duration_ms);
                RwLock::new(TimeWheelSlot::new(expire_time_ms))
            })
            .collect();

        debug!(
            "TimeWheelLevel {} created: {} slots × {}ms",
            level_id, slot_count, slot_duration_ms
        );

        Self {
            slots,
            current_slot: AtomicUsize::new(0),
            slot_duration,
            slot_count,
            level_id,
        }
    }

    fn add_flow(&self, key: FlowKey, slot_offset: usize) {
        let current = self.current_slot.load(Ordering::Acquire);
        let target_slot = (current + slot_offset) % self.slot_count;

        let mut slot = self.slots[target_slot].write().unwrap();
        slot.add_flow(key);

        trace!(
            "Level {}: added flow to slot {} (offset={})",
            self.level_id,
            target_slot,
            slot_offset
        );
    }

    fn advance(&self, now_ms: u64) -> Vec<FlowKey> {
        let current = self.current_slot.load(Ordering::Acquire);
        let mut expired = Vec::new();

        let slot = &self.slots[current];
        let mut slot_guard = slot.write().unwrap();

        if slot_guard.expire_time_ms <= now_ms {
            expired.extend(slot_guard.drain());

            let slot_duration_ms = self.slot_duration.as_millis() as u64;
            slot_guard.expire_time_ms = now_ms + (self.slot_count as u64 * slot_duration_ms);

            let next_slot = (current + 1) % self.slot_count;
            self.current_slot.store(next_slot, Ordering::Release);

            if !expired.is_empty() {
                debug!(
                    "Level {}: slot {} advanced, {} flows expired, next={}",
                    self.level_id,
                    current,
                    expired.len(),
                    next_slot
                );
            }
        }

        expired
    }

    fn stats(&self) -> (usize, usize) {
        let mut total_flows = 0;
        let mut non_empty_slots = 0;

        for slot in &self.slots {
            let guard = slot.read().unwrap();
            let count = guard.len();
            if count > 0 {
                total_flows += count;
                non_empty_slots += 1;
            }
        }

        (total_flows, non_empty_slots)
    }

    fn current_slot_index(&self) -> usize {
        self.current_slot.load(Ordering::Acquire)
    }
}

pub struct HierarchicalTimeWheel {
    levels: [TimeWheelLevel; 3],
    start_time_ms: u64,
    tick_count: AtomicU64,
}

impl HierarchicalTimeWheel {
    pub fn new() -> Self {
        let start_time_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let levels = [
            TimeWheelLevel::new(LEVEL0_SLOTS, Duration::from_secs(LEVEL0_DURATION_SECS), 0),
            TimeWheelLevel::new(LEVEL1_SLOTS, Duration::from_secs(LEVEL1_DURATION_SECS), 1),
            TimeWheelLevel::new(LEVEL2_SLOTS, Duration::from_secs(LEVEL2_DURATION_SECS), 2),
        ];

        info!(
            "HierarchicalTimeWheel created: L0={}×{}s, L1={}×{}s, L2={}×{}s, total_coverage={}h",
            LEVEL0_SLOTS,
            LEVEL0_DURATION_SECS,
            LEVEL1_SLOTS,
            LEVEL1_DURATION_SECS,
            LEVEL2_SLOTS,
            LEVEL2_DURATION_SECS,
            (LEVEL0_SLOTS as u64 * LEVEL0_DURATION_SECS
                + LEVEL1_SLOTS as u64 * LEVEL1_DURATION_SECS
                + LEVEL2_SLOTS as u64 * LEVEL2_DURATION_SECS)
                / 3600
        );

        Self {
            levels,
            start_time_ms,
            tick_count: AtomicU64::new(0),
        }
    }

    pub fn schedule(&self, key: FlowKey, expire_at_ms: u64) {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        if expire_at_ms < now_ms {
            debug!("Flow already expired, skipping schedule");
            return;
        }

        let delay_ms = expire_at_ms - now_ms;
        let delay_secs = delay_ms / 1000;

        let (level, slot_offset) = if delay_secs < 60 {
            (0, delay_secs as usize)
        } else if delay_secs < 3600 {
            (1, (delay_secs / 60) as usize)
        } else {
            (2, (delay_secs / 3600).min(23) as usize)
        };

        self.levels[level].add_flow(key, slot_offset);

        trace!(
            "Scheduled flow in L{} slot_offset={} (delay={}s)",
            level,
            slot_offset,
            delay_secs
        );
    }

    pub fn tick(&self) -> Vec<FlowKey> {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        let tick = self.tick_count.fetch_add(1, Ordering::Relaxed);

        let mut expired = Vec::new();

        expired.extend(self.levels[0].advance(now_ms));

        if tick % 60 == 0 {
            expired.extend(self.levels[1].advance(now_ms));
            self.cascade(1);
        }

        if tick % 3600 == 0 {
            expired.extend(self.levels[2].advance(now_ms));
            self.cascade(2);
        }

        if !expired.is_empty() {
            debug!("TimeWheel tick {}: {} flows expired", tick, expired.len());
        }

        expired
    }

    fn cascade(&self, from_level: usize) {
        if from_level == 0 || from_level >= self.levels.len() {
            return;
        }

        let current_slot = self.levels[from_level].current_slot_index();
        let slot = &self.levels[from_level].slots[current_slot];

        let flows: Vec<FlowKey> = {
            let mut guard = slot.write().unwrap();
            guard.drain()
        };

        if flows.is_empty() {
            return;
        }

        debug!(
            "Cascading {} flows from L{} to L{}",
            flows.len(),
            from_level,
            from_level - 1
        );

        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;

        for flow_key in flows {
            self.schedule(flow_key, now_ms + 60_000);
        }
    }

    pub fn stats(&self) -> TimeWheelStats {
        let (l0_flows, l0_slots) = self.levels[0].stats();
        let (l1_flows, l1_slots) = self.levels[1].stats();
        let (l2_flows, l2_slots) = self.levels[2].stats();

        TimeWheelStats {
            total_flows: l0_flows + l1_flows + l2_flows,
            level0_flows: l0_flows,
            level1_flows: l1_flows,
            level2_flows: l2_flows,
            level0_slots_used: l0_slots,
            level1_slots_used: l1_slots,
            level2_slots_used: l2_slots,
            tick_count: self.tick_count.load(Ordering::Relaxed),
        }
    }

    pub fn clear(&self) {
        for level in &self.levels {
            for slot in &level.slots {
                let mut guard = slot.write().unwrap();
                guard.clear();
            }
        }
        self.tick_count.store(0, Ordering::Relaxed);
        info!("TimeWheel cleared");
    }
}

impl Default for HierarchicalTimeWheel {
    fn default() -> Self {
        Self::new()
    }
}

#[derive(Debug, Clone, Copy)]
pub struct TimeWheelStats {
    pub total_flows: usize,
    pub level0_flows: usize,
    pub level1_flows: usize,
    pub level2_flows: usize,
    pub level0_slots_used: usize,
    pub level1_slots_used: usize,
    pub level2_slots_used: usize,
    pub tick_count: u64,
}

impl TimeWheelStats {
    pub fn avg_flows_per_slot(&self) -> f64 {
        let total_slots = self.level0_slots_used + self.level1_slots_used + self.level2_slots_used;
        if total_slots == 0 {
            return 0.0;
        }
        self.total_flows as f64 / total_slots as f64
    }
}
