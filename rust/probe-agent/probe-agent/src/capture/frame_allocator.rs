use crossbeam_queue::ArrayQueue;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};

pub struct FrameAllocator {
    free_frames: ArrayQueue<usize>,

    frame_size: usize,

    frame_count: usize,

    allocated: AtomicU64,

    freed: AtomicU64,

    high_watermark: AtomicUsize,
}

impl FrameAllocator {
    pub fn new(frame_count: usize, frame_size: usize) -> Self {
        let free_frames = ArrayQueue::new(frame_count);

        for i in 0..frame_count {
            free_frames.push(i).ok();
        }

        Self {
            free_frames,
            frame_size,
            frame_count,
            allocated: AtomicU64::new(0),
            freed: AtomicU64::new(0),
            high_watermark: AtomicUsize::new(0),
        }
    }

    #[inline]
    pub fn allocate(&self) -> Option<usize> {
        let frame = self.free_frames.pop()?;

        let count = self.allocated.fetch_add(1, Ordering::Relaxed) + 1;
        let in_use = count - self.freed.load(Ordering::Relaxed);

        let mut current = self.high_watermark.load(Ordering::Relaxed);
        while in_use as usize > current {
            match self.high_watermark.compare_exchange_weak(
                current,
                in_use as usize,
                Ordering::Relaxed,
                Ordering::Relaxed,
            ) {
                Ok(_) => break,
                Err(c) => current = c,
            }
        }

        Some(frame)
    }

    #[inline]
    pub fn allocate_batch(&self, count: usize) -> Vec<usize> {
        let mut frames = Vec::with_capacity(count);
        for _ in 0..count {
            match self.allocate() {
                Some(frame) => frames.push(frame),
                None => break,
            }
        }
        frames
    }

    #[inline]
    pub fn free(&self, frame: usize) {
        debug_assert!(
            frame < self.frame_count,
            "Frame index out of bounds: {} >= {}",
            frame,
            self.frame_count
        );

        self.freed.fetch_add(1, Ordering::Relaxed);

        if self.free_frames.push(frame).is_err() {
            tracing::error!("Frame allocator overflow: frame {} lost", frame);
        }
    }

    #[inline]
    pub fn free_batch(&self, frames: &[usize]) {
        for &frame in frames {
            self.free(frame);
        }
    }

    #[inline]
    pub fn available(&self) -> usize {
        self.free_frames.len()
    }

    #[inline]
    pub fn in_use(&self) -> usize {
        let allocated = self.allocated.load(Ordering::Relaxed);
        let freed = self.freed.load(Ordering::Relaxed);
        (allocated - freed) as usize
    }

    #[inline]
    pub fn frame_size(&self) -> usize {
        self.frame_size
    }

    #[inline]
    pub fn frame_count(&self) -> usize {
        self.frame_count
    }

    pub fn high_watermark(&self) -> usize {
        self.high_watermark.load(Ordering::Relaxed)
    }

    #[inline]
    pub fn frame_addr(&self, frame_idx: usize) -> usize {
        frame_idx * self.frame_size
    }

    #[inline]
    pub fn addr_to_frame(&self, addr: usize) -> usize {
        addr / self.frame_size
    }

    pub fn utilization(&self) -> f64 {
        let in_use = self.in_use() as f64;
        let total = self.frame_count as f64;
        if total == 0.0 {
            0.0
        } else {
            in_use / total
        }
    }

    pub fn stats(&self) -> FrameAllocatorStats {
        FrameAllocatorStats {
            frame_count: self.frame_count,
            frame_size: self.frame_size,
            available: self.available(),
            in_use: self.in_use(),
            total_allocated: self.allocated.load(Ordering::Relaxed),
            total_freed: self.freed.load(Ordering::Relaxed),
            high_watermark: self.high_watermark(),
        }
    }
}

#[derive(Debug, Clone)]
pub struct FrameAllocatorStats {
    pub frame_count: usize,
    pub frame_size: usize,
    pub available: usize,
    pub in_use: usize,
    pub total_allocated: u64,
    pub total_freed: u64,
    pub high_watermark: usize,
}
