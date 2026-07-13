use std::sync::atomic::{AtomicU32, AtomicU64, Ordering};

#[derive(Debug)]
pub struct OnlineStats {
    count: AtomicU64,
    mean: AtomicU64,
    m2: AtomicU64,
    min: AtomicU32,
    max: AtomicU32,
}

impl Default for OnlineStats {
    fn default() -> Self {
        Self::new()
    }
}

impl OnlineStats {
    pub const fn new() -> Self {
        Self {
            count: AtomicU64::new(0),
            mean: AtomicU64::new(0),
            m2: AtomicU64::new(0),
            min: AtomicU32::new(u32::MAX),
            max: AtomicU32::new(0),
        }
    }

    #[inline]
    pub fn update(&self, value: u32) {
        let n = self.count.fetch_add(1, Ordering::Relaxed) + 1;

        let old_mean = self.mean.load(Ordering::Relaxed) as f64 / 1e6;
        let delta = value as f64 - old_mean;
        let new_mean = old_mean + delta / n as f64;
        let delta2 = value as f64 - new_mean;
        let old_m2 = self.m2.load(Ordering::Relaxed) as f64 / 1e6;
        let new_m2 = old_m2 + delta * delta2;

        self.mean.store((new_mean * 1e6) as u64, Ordering::Relaxed);
        self.m2.store((new_m2 * 1e6) as u64, Ordering::Relaxed);

        let mut current_min = self.min.load(Ordering::Acquire);
        while value < current_min {
            match self.min.compare_exchange_weak(
                current_min,
                value,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => break,
                Err(c) => current_min = c,
            }
        }

        let mut current_max = self.max.load(Ordering::Acquire);
        while value > current_max {
            match self.max.compare_exchange_weak(
                current_max,
                value,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => break,
                Err(c) => current_max = c,
            }
        }
    }

    #[inline]
    pub fn update_float(&self, value: f32) {
        self.update((value * 1000.0) as u32);
    }

    #[inline]
    pub fn count(&self) -> u64 {
        self.count.load(Ordering::Relaxed)
    }

    #[inline]
    pub fn sum(&self) -> u64 {
        let count = self.count.load(Ordering::Relaxed);
        let mean = self.mean.load(Ordering::Relaxed) as f64 / 1e6;
        (mean * count as f64) as u64
    }

    #[inline]
    pub fn sum_sq(&self) -> u64 {
        let count = self.count.load(Ordering::Relaxed);
        let mean = self.mean.load(Ordering::Relaxed) as f64 / 1e6;
        let m2 = self.m2.load(Ordering::Relaxed) as f64 / 1e6;
        let variance = if count > 1 {
            m2 / (count - 1) as f64
        } else {
            0.0
        };
        ((mean * mean + variance) * count as f64) as u64
    }

    #[inline]
    pub fn mean(&self) -> f32 {
        let count = self.count.load(Ordering::Relaxed);
        if count == 0 {
            return 0.0;
        }
        (self.mean.load(Ordering::Relaxed) as f64 / 1e6) as f32
    }

    #[inline]
    pub fn mean_float(&self) -> f32 {
        self.mean() / 1000.0
    }

    #[inline]
    pub fn std(&self) -> f32 {
        let count = self.count.load(Ordering::Relaxed);
        if count <= 1 {
            return 0.0;
        }

        let m2 = self.m2.load(Ordering::Relaxed) as f64 / 1e6;
        let variance = m2 / (count - 1) as f64;

        if variance > 0.0 {
            variance.sqrt() as f32
        } else {
            0.0
        }
    }

    #[inline]
    pub fn std_float(&self) -> f32 {
        self.std() / 1000.0
    }

    #[inline]
    pub fn min(&self) -> u32 {
        let v = self.min.load(Ordering::Acquire);
        if v == u32::MAX {
            0
        } else {
            v
        }
    }

    #[inline]
    pub fn min_float(&self) -> f32 {
        self.min() as f32 / 1000.0
    }

    #[inline]
    pub fn max(&self) -> u32 {
        self.max.load(Ordering::Acquire)
    }

    #[inline]
    pub fn max_float(&self) -> f32 {
        self.max() as f32 / 1000.0
    }

    pub fn reset(&self) {
        self.count.store(0, Ordering::Release);
        self.mean.store(0, Ordering::Release);
        self.m2.store(0, Ordering::Release);
        self.min.store(u32::MAX, Ordering::Release);
        self.max.store(0, Ordering::Release);
    }

    pub fn snapshot(&self) -> OnlineStatsSnapshot {
        let count = self.count();
        let mean = self.mean();
        let std = self.std();
        let min = self.min();
        let max = self.max();

        OnlineStatsSnapshot {
            count,
            sum: self.sum(),
            sum_sq: self.sum_sq(),
            mean,
            std,
            min,
            max,
        }
    }

    pub fn merge(&self, other: &OnlineStats) -> OnlineStatsSnapshot {
        let count1 = self.count();
        let count2 = other.count();
        let total_count = count1 + count2;

        if total_count == 0 {
            return OnlineStatsSnapshot::empty();
        }

        let mean1 = self.mean.load(Ordering::Relaxed) as f64 / 1e6;
        let mean2 = other.mean.load(Ordering::Relaxed) as f64 / 1e6;
        let m2_1 = self.m2.load(Ordering::Relaxed) as f64 / 1e6;
        let m2_2 = other.m2.load(Ordering::Relaxed) as f64 / 1e6;

        let delta = mean2 - mean1;
        let mean = (count1 as f64 * mean1 + count2 as f64 * mean2) / total_count as f64;
        let m2 = m2_1 + m2_2 + delta * delta * count1 as f64 * count2 as f64 / total_count as f64;

        let variance = if total_count > 1 {
            m2 / (total_count - 1) as f64
        } else {
            0.0
        };

        let std = if variance > 0.0 {
            variance.sqrt() as f32
        } else {
            0.0
        };

        let min1 = self.min();
        let min2 = other.min();
        let min = if min1 == 0 {
            min2
        } else if min2 == 0 {
            min1
        } else {
            min1.min(min2)
        };

        let max = self.max().max(other.max());

        OnlineStatsSnapshot {
            count: total_count,
            sum: (mean * total_count as f64) as u64,
            sum_sq: ((mean * mean + variance) * total_count as f64) as u64,
            mean: mean as f32,
            std,
            min,
            max,
        }
    }

    pub fn variance(&self) -> f32 {
        let std = self.std();
        std * std
    }

    pub fn coefficient_of_variation(&self) -> f32 {
        let mean = self.mean();
        if mean == 0.0 {
            return 0.0;
        }
        self.std() / mean
    }
}

#[derive(Debug, Clone, Copy)]
pub struct OnlineStatsSnapshot {
    pub count: u64,
    pub sum: u64,
    pub sum_sq: u64,
    pub mean: f32,
    pub std: f32,
    pub min: u32,
    pub max: u32,
}

impl Default for OnlineStatsSnapshot {
    fn default() -> Self {
        Self::empty()
    }
}

impl OnlineStatsSnapshot {
    pub fn empty() -> Self {
        Self {
            count: 0,
            sum: 0,
            sum_sq: 0,
            mean: 0.0,
            std: 0.0,
            min: 0,
            max: 0,
        }
    }

    pub fn from_values(values: &[u32]) -> Self {
        if values.is_empty() {
            return Self::empty();
        }

        let count = values.len() as u64;

        let mut mean = 0.0;
        let mut m2 = 0.0;

        for (i, &value) in values.iter().enumerate() {
            let n = (i + 1) as f64;
            let delta = value as f64 - mean;
            mean += delta / n;
            let delta2 = value as f64 - mean;
            m2 += delta * delta2;
        }

        let variance = if count > 1 {
            m2 / (count - 1) as f64
        } else {
            0.0
        };
        let std = if variance > 0.0 {
            variance.sqrt() as f32
        } else {
            0.0
        };

        let min = *values.iter().min().unwrap();
        let max = *values.iter().max().unwrap();

        Self {
            count,
            sum: (mean * count as f64) as u64,
            sum_sq: ((mean * mean + variance) * count as f64) as u64,
            mean: mean as f32,
            std,
            min,
            max,
        }
    }

    pub fn variance(&self) -> f32 {
        self.std * self.std
    }
}

#[derive(Debug)]
pub struct DirectionalStats {
    pub fwd: OnlineStats,
    pub bwd: OnlineStats,
}

impl Default for DirectionalStats {
    fn default() -> Self {
        Self::new()
    }
}

impl DirectionalStats {
    pub const fn new() -> Self {
        Self {
            fwd: OnlineStats::new(),
            bwd: OnlineStats::new(),
        }
    }

    #[inline]
    pub fn update(&self, value: u32, is_forward: bool) {
        if is_forward {
            self.fwd.update(value);
        } else {
            self.bwd.update(value);
        }
    }

    pub fn fwd_snapshot(&self) -> OnlineStatsSnapshot {
        self.fwd.snapshot()
    }

    pub fn bwd_snapshot(&self) -> OnlineStatsSnapshot {
        self.bwd.snapshot()
    }

    pub fn merged(&self) -> OnlineStatsSnapshot {
        self.fwd.merge(&self.bwd)
    }

    pub fn total_count(&self) -> u64 {
        self.fwd.count() + self.bwd.count()
    }

    pub fn reset(&self) {
        self.fwd.reset();
        self.bwd.reset();
    }
}
