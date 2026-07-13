use super::umem::Umem;
use super::FrameInfo;
use std::sync::Arc;

pub struct PacketBatchBuilder {
    umem: Arc<Umem>,
    frames: Vec<FrameInfo>,
    capacity: usize,
}

impl PacketBatchBuilder {
    pub fn new(umem: Arc<Umem>, capacity: usize) -> Self {
        Self {
            umem,
            frames: Vec::with_capacity(capacity),
            capacity,
        }
    }

    #[inline]
    pub fn push(&mut self, frame: FrameInfo) -> bool {
        if self.frames.len() >= self.capacity {
            return false;
        }
        self.frames.push(frame);
        true
    }

    pub fn extend(&mut self, frames: impl IntoIterator<Item = FrameInfo>) {
        for frame in frames {
            if !self.push(frame) {
                break;
            }
        }
    }

    #[inline]
    pub fn len(&self) -> usize {
        self.frames.len()
    }

    #[inline]
    pub fn is_empty(&self) -> bool {
        self.frames.is_empty()
    }

    #[inline]
    pub fn is_full(&self) -> bool {
        self.frames.len() >= self.capacity
    }

    #[inline]
    pub fn remaining(&self) -> usize {
        self.capacity.saturating_sub(self.frames.len())
    }

    pub fn build(self) -> super::PacketBatch {
        super::PacketBatch::new(self.umem, self.frames)
    }

    pub fn clear(&mut self) {
        self.frames.clear();
    }

    pub fn umem(&self) -> &Arc<Umem> {
        &self.umem
    }
}

#[derive(Debug, Clone, Default)]
pub struct PacketBatchStats {
    pub packet_count: u64,

    pub total_bytes: u64,

    pub min_len: u32,

    pub max_len: u32,

    pub first_timestamp: u64,

    pub last_timestamp: u64,
}

impl PacketBatchStats {
    pub fn from_batch(batch: &super::PacketBatch) -> Self {
        if batch.is_empty() {
            return Self::default();
        }

        let mut stats = Self {
            packet_count: batch.len() as u64,
            total_bytes: 0,
            min_len: u32::MAX,
            max_len: 0,
            first_timestamp: u64::MAX,
            last_timestamp: 0,
        };

        for frame in &batch.frames {
            stats.total_bytes += frame.len as u64;
            stats.min_len = stats.min_len.min(frame.len);
            stats.max_len = stats.max_len.max(frame.len);
            stats.first_timestamp = stats.first_timestamp.min(frame.timestamp);
            stats.last_timestamp = stats.last_timestamp.max(frame.timestamp);
        }

        if stats.min_len == u32::MAX {
            stats.min_len = 0;
        }
        if stats.first_timestamp == u64::MAX {
            stats.first_timestamp = 0;
        }

        stats
    }

    pub fn avg_len(&self) -> f64 {
        if self.packet_count == 0 {
            0.0
        } else {
            self.total_bytes as f64 / self.packet_count as f64
        }
    }

    pub fn duration_us(&self) -> u64 {
        self.last_timestamp.saturating_sub(self.first_timestamp)
    }

    pub fn pps(&self) -> f64 {
        let duration_sec = self.duration_us() as f64 / 1_000_000.0;
        if duration_sec <= 0.0 {
            return 0.0;
        }
        self.packet_count as f64 / duration_sec
    }

    pub fn bps(&self) -> f64 {
        let duration_sec = self.duration_us() as f64 / 1_000_000.0;
        if duration_sec <= 0.0 {
            return 0.0;
        }
        (self.total_bytes * 8) as f64 / duration_sec
    }
}

#[derive(Debug)]
pub struct IndexedPacket<'a> {
    pub index: usize,

    pub data: &'a [u8],

    pub frame: &'a FrameInfo,
}

pub struct IndexedPacketIter<'a> {
    batch: &'a super::PacketBatch,
    index: usize,
}

impl<'a> IndexedPacketIter<'a> {
    pub fn new(batch: &'a super::PacketBatch) -> Self {
        Self { batch, index: 0 }
    }
}

impl<'a> Iterator for IndexedPacketIter<'a> {
    type Item = IndexedPacket<'a>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.index >= self.batch.len() {
            return None;
        }

        let frame = &self.batch.frames[self.index];
        let data = self.batch.get_packet(self.index)?;
        let index = self.index;
        self.index += 1;

        Some(IndexedPacket { index, data, frame })
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let remaining = self.batch.len() - self.index;
        (remaining, Some(remaining))
    }
}

impl<'a> ExactSizeIterator for IndexedPacketIter<'a> {}

pub trait PacketBatchExt {
    fn indexed_iter(&self) -> IndexedPacketIter<'_>;

    fn stats(&self) -> PacketBatchStats;

    fn filter_by_time(&self, start: u64, end: u64) -> Vec<usize>;

    fn time_range(&self) -> Option<(u64, u64)>;
}

impl PacketBatchExt for super::PacketBatch {
    fn indexed_iter(&self) -> IndexedPacketIter<'_> {
        IndexedPacketIter::new(self)
    }

    fn stats(&self) -> PacketBatchStats {
        PacketBatchStats::from_batch(self)
    }

    fn filter_by_time(&self, start: u64, end: u64) -> Vec<usize> {
        self.frames
            .iter()
            .enumerate()
            .filter(|(_, f)| f.timestamp >= start && f.timestamp <= end)
            .map(|(i, _)| i)
            .collect()
    }

    fn time_range(&self) -> Option<(u64, u64)> {
        if self.is_empty() {
            return None;
        }

        let mut min_ts = u64::MAX;
        let mut max_ts = 0u64;

        for frame in &self.frames {
            min_ts = min_ts.min(frame.timestamp);
            max_ts = max_ts.max(frame.timestamp);
        }

        Some((min_ts, max_ts))
    }
}
