use anyhow::{bail, Result};
use crossbeam_queue::ArrayQueue;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use tracing::{debug, warn};

#[derive(Clone, Debug)]
pub struct UmemConfig {
    pub frame_size: usize,
    pub frame_count: usize,
    pub fill_queue_size: usize,
    pub comp_queue_size: usize,
    pub headroom: usize,
    pub use_huge_pages: bool,
}

impl Default for UmemConfig {
    fn default() -> Self {
        Self {
            frame_size: 4096,
            frame_count: 4096,
            fill_queue_size: 2048,
            comp_queue_size: 2048,
            headroom: 0,
            use_huge_pages: false,
        }
    }
}

impl UmemConfig {
    pub fn validate(&self) -> Result<()> {
        if self.frame_size % 4096 != 0 {
            bail!(
                "frame_size must be multiple of 4096, got {}",
                self.frame_size
            );
        }

        if self.frame_count == 0 {
            bail!("frame_count must be > 0");
        }

        if self.fill_queue_size == 0 {
            bail!("fill_queue_size must be > 0");
        }

        if self.comp_queue_size == 0 {
            bail!("comp_queue_size must be > 0");
        }

        if self.headroom >= self.frame_size {
            bail!(
                "headroom ({}) must be less than frame_size ({})",
                self.headroom,
                self.frame_size
            );
        }

        let total_size = self.frame_size * self.frame_count;
        if total_size > 1024 * 1024 * 1024 {
            bail!("Total UMEM size ({} bytes) exceeds 1GB limit", total_size);
        }

        Ok(())
    }

    pub fn total_size(&self) -> usize {
        self.frame_size * self.frame_count
    }

    pub fn usable_frame_size(&self) -> usize {
        self.frame_size - self.headroom
    }
}

struct UmemInner {
    addr: *mut u8,
    size: usize,
    frame_size: usize,
    frame_count: usize,
    headroom: usize,
    free_frames: Arc<ArrayQueue<usize>>,
    allocated_frames: AtomicUsize,
    batch_refcount: AtomicUsize,
    mmap_valid: bool,
}

impl UmemInner {
    fn new(config: &UmemConfig) -> Result<Self> {
        config.validate()?;

        let size = config.frame_size * config.frame_count;

        let mut flags = libc::MAP_PRIVATE | libc::MAP_ANONYMOUS | libc::MAP_POPULATE;

        if config.use_huge_pages {
            flags |= libc::MAP_HUGETLB;
        }

        let addr = unsafe {
            libc::mmap(
                std::ptr::null_mut(),
                size,
                libc::PROT_READ | libc::PROT_WRITE,
                flags,
                -1,
                0,
            )
        };

        if addr == libc::MAP_FAILED {
            let err = std::io::Error::last_os_error();

            if config.use_huge_pages {
                warn!(
                    "Huge pages allocation failed, falling back to normal pages: {}",
                    err
                );
                let addr = unsafe {
                    libc::mmap(
                        std::ptr::null_mut(),
                        size,
                        libc::PROT_READ | libc::PROT_WRITE,
                        libc::MAP_PRIVATE | libc::MAP_ANONYMOUS | libc::MAP_POPULATE,
                        -1,
                        0,
                    )
                };

                if addr == libc::MAP_FAILED {
                    bail!("mmap failed: {}", std::io::Error::last_os_error());
                }

                return Self::init_from_addr(addr as *mut u8, config);
            }

            bail!("mmap failed: {}", err);
        }

        Self::init_from_addr(addr as *mut u8, config)
    }

    fn dummy(size: usize) -> Self {
        let frame_size = size.max(4096);
        let buffer = vec![0u8; frame_size];
        Self::from_buffer(buffer)
    }

    fn from_buffer(mut buffer: Vec<u8>) -> Self {
        let size = buffer.len();
        let frame_size = size.max(4096);
        let frame_count = 1usize;
        let addr = buffer.as_mut_ptr();
        std::mem::forget(buffer); // Transfer ownership to raw pointer

        let free_frames = Arc::new(ArrayQueue::new(frame_count));
        free_frames.push(0).ok();

        debug!("UMEM from_buffer created: {} bytes", frame_size);

        Self {
            addr,
            size: frame_size,
            frame_size,
            frame_count,
            headroom: 0,
            free_frames,
            allocated_frames: AtomicUsize::new(0),
            batch_refcount: AtomicUsize::new(0),
            mmap_valid: false,
        }
    }

    fn init_from_addr(addr: *mut u8, config: &UmemConfig) -> Result<Self> {
        let size = config.frame_size * config.frame_count;

        let free_frames = Arc::new(ArrayQueue::new(config.frame_count));
        for i in 0..config.frame_count {
            free_frames.push(i).ok();
        }

        debug!(
            "UMEM allocated: {} frames × {} bytes = {} MB (headroom: {})",
            config.frame_count,
            config.frame_size,
            size / 1024 / 1024,
            config.headroom
        );

        Ok(Self {
            addr,
            size,
            frame_size: config.frame_size,
            frame_count: config.frame_count,
            headroom: config.headroom,
            free_frames,
            allocated_frames: AtomicUsize::new(0),
            batch_refcount: AtomicUsize::new(0),
            mmap_valid: true,
        })
    }
}

impl Drop for UmemInner {
    fn drop(&mut self) {
        if !self.mmap_valid {
            debug!("UMEM not mapped, skipping munmap");
            return;
        }

        debug!(
            "UMEM Drop starting: allocated_frames={}, batch_refcount={}",
            self.allocated_frames.load(Ordering::Acquire),
            self.batch_refcount.load(Ordering::Acquire)
        );

        let timeout = std::time::Duration::from_secs(10);
        let start = std::time::Instant::now();

        while self.batch_refcount.load(Ordering::Acquire) > 0 {
            if start.elapsed() > timeout {
                panic!(
                    "CRITICAL: Timeout waiting for {} PacketBatch references to be released. \
                     This indicates a logic bug where PacketBatch is held too long. \
                     Allocated frames: {}, Free frames: {}",
                    self.batch_refcount.load(Ordering::Acquire),
                    self.allocated_frames.load(Ordering::Acquire),
                    self.free_frames.len()
                );
            }
            std::thread::sleep(std::time::Duration::from_millis(10));
        }

        debug!(
            "All PacketBatch references released, checking frame allocation: allocated={}, free={}",
            self.allocated_frames.load(Ordering::Acquire),
            self.free_frames.len()
        );

        let allocated = self.allocated_frames.load(Ordering::Acquire);
        if allocated > 0 {
            panic!(
                "CRITICAL: UMEM dropped with {} frames still allocated! \
                 This indicates a logic bug (frames not properly freed). \
                 Total frames: {}, Free frames: {}, Batch refs: {}",
                allocated,
                self.frame_count,
                self.free_frames.len(),
                self.batch_refcount.load(Ordering::Acquire)
            );
        }

        let additional_timeout = std::time::Duration::from_secs(5);
        let additional_start = std::time::Instant::now();

        while self.allocated_frames.load(Ordering::Acquire) > 0 {
            if additional_start.elapsed() > additional_timeout {
                panic!(
                    "CRITICAL: Timeout waiting for frames to be released. \
                     {} frames still allocated after {} seconds.",
                    self.allocated_frames.load(Ordering::Acquire),
                    timeout.as_secs() + additional_timeout.as_secs()
                );
            }
            std::thread::sleep(std::time::Duration::from_millis(10));
        }

        debug!("All frames released, proceeding with munmap");

        let ret = unsafe { libc::munmap(self.addr as *mut libc::c_void, self.size) };

        if ret != 0 {
            panic!(
                "CRITICAL: munmap failed for UMEM (addr={:p}, size={}): {}. \
                 This will cause memory leak of {} MB!",
                self.addr,
                self.size,
                std::io::Error::last_os_error(),
                self.size / 1024 / 1024
            );
        }

        debug!(
            "UMEM released successfully: {} bytes ({} MB)",
            self.size,
            self.size / 1024 / 1024
        );
    }
}

unsafe impl Send for UmemInner {}
unsafe impl Sync for UmemInner {}

pub struct Umem {
    inner: Arc<UmemInner>,
}

impl Umem {
    pub fn new(config: &UmemConfig) -> Result<Self> {
        Ok(Self {
            inner: Arc::new(UmemInner::new(config)?),
        })
    }

    /// Create a dummy UMEM for offline/PCAP replay mode.
    /// This allocates a simple Vec-based buffer instead of using AF_XDP mmap.
    pub fn dummy(size: usize) -> Self {
        Self {
            inner: Arc::new(UmemInner::dummy(size)),
        }
    }

    /// Create a dummy UMEM from an existing data buffer (for PCAP offline mode).
    /// Takes ownership of the buffer and uses it as the backing store.
    pub fn from_buffer(buffer: Vec<u8>) -> Self {
        Self {
            inner: Arc::new(UmemInner::from_buffer(buffer)),
        }
    }

    pub fn get_frame(&self, idx: usize) -> Result<&[u8]> {
        if idx >= self.inner.frame_count {
            bail!(
                "Frame index {} out of range (max: {})",
                idx,
                self.inner.frame_count - 1
            );
        }

        let offset = idx * self.inner.frame_size;

        if offset.checked_add(self.inner.frame_size).is_none() {
            bail!("Frame offset calculation overflow: idx={}", idx);
        }

        if offset + self.inner.frame_size > self.inner.size {
            bail!(
                "Frame {} (offset {} + size {}) exceeds UMEM size {}",
                idx,
                offset,
                self.inner.frame_size,
                self.inner.size
            );
        }

        let slice = unsafe {
            std::slice::from_raw_parts(
                self.inner.addr.add(offset + self.inner.headroom),
                self.inner.frame_size - self.inner.headroom,
            )
        };

        Ok(slice)
    }

    #[allow(clippy::mut_from_ref)]
    pub fn get_frame_mut(&self, idx: usize) -> Result<&mut [u8]> {
        if idx >= self.inner.frame_count {
            bail!(
                "Frame index {} out of range (max: {})",
                idx,
                self.inner.frame_count - 1
            );
        }

        let offset = idx * self.inner.frame_size;

        if offset.checked_add(self.inner.frame_size).is_none() {
            bail!("Frame offset calculation overflow: idx={}", idx);
        }

        if offset + self.inner.frame_size > self.inner.size {
            bail!(
                "Frame {} (offset {} + size {}) exceeds UMEM size {}",
                idx,
                offset,
                self.inner.frame_size,
                self.inner.size
            );
        }

        let slice = unsafe {
            std::slice::from_raw_parts_mut(
                self.inner.addr.add(offset + self.inner.headroom),
                self.inner.frame_size - self.inner.headroom,
            )
        };

        Ok(slice)
    }

    pub fn get_data(&self, addr: usize, len: usize) -> Option<&[u8]> {
        if addr >= self.inner.size {
            return None;
        }

        let end = addr.checked_add(len)?;
        if end > self.inner.size {
            return None;
        }

        unsafe {
            let slice = std::slice::from_raw_parts(self.inner.addr.add(addr), len);
            Some(slice)
        }
    }

    #[allow(clippy::mut_from_ref)]
    pub fn get_data_mut(&self, addr: usize, len: usize) -> Option<&mut [u8]> {
        if addr >= self.inner.size {
            return None;
        }

        let end = addr.checked_add(len)?;
        if end > self.inner.size {
            return None;
        }

        unsafe {
            let slice = std::slice::from_raw_parts_mut((self.inner.addr as *mut u8).add(addr), len);
            Some(slice)
        }
    }

    pub fn addr_to_frame(&self, addr: usize) -> usize {
        let base_addr = self.inner.addr as usize;

        if addr < base_addr {
            warn!(
                "Address 0x{:x} is below UMEM base 0x{:x}, using frame 0",
                addr, base_addr
            );
            return 0;
        }

        let offset = addr - base_addr;

        if offset >= self.inner.size {
            warn!(
                "Address offset {} exceeds UMEM size {}, clamping to last frame",
                offset, self.inner.size
            );
            return self.inner.frame_count - 1;
        }

        let frame_idx = offset / self.inner.frame_size;

        if frame_idx >= self.inner.frame_count {
            warn!(
                "Calculated frame index {} exceeds count {}, clamping",
                frame_idx, self.inner.frame_count
            );
            return self.inner.frame_count - 1;
        }

        frame_idx
    }

    #[inline]
    pub fn frame_addr(&self, idx: usize) -> usize {
        debug_assert!(
            idx < self.inner.frame_count,
            "Frame index {} out of range",
            idx
        );
        idx * self.inner.frame_size + self.inner.headroom
    }

    #[inline]
    pub fn frame_addr_raw(&self, idx: usize) -> usize {
        debug_assert!(
            idx < self.inner.frame_count,
            "Frame index {} out of range",
            idx
        );
        idx * self.inner.frame_size
    }

    pub fn alloc_frame(&self) -> Option<usize> {
        match self.inner.free_frames.pop() {
            Some(idx) => {
                self.inner.allocated_frames.fetch_add(1, Ordering::Relaxed);
                Some(idx)
            }
            None => None,
        }
    }

    pub fn alloc_frames(&self, count: usize) -> Vec<usize> {
        let mut frames = Vec::with_capacity(count);
        for _ in 0..count {
            match self.alloc_frame() {
                Some(idx) => frames.push(idx),
                None => break,
            }
        }
        frames
    }

    pub fn free_frame(&self, idx: usize) {
        debug_assert!(
            idx < self.inner.frame_count,
            "Frame index {} out of range",
            idx
        );

        if self.inner.free_frames.push(idx).is_err() {
            warn!("Free frames queue is full, frame {} lost", idx);
        } else {
            self.inner.allocated_frames.fetch_sub(1, Ordering::Relaxed);
        }
    }

    pub fn free_frames(&self, frames: &[usize]) {
        for &idx in frames {
            self.free_frame(idx);
        }
    }

    pub fn inc_batch_refcount(&self) {
        let old_count = self.inner.batch_refcount.fetch_add(1, Ordering::AcqRel);
        tracing::trace!("inc_batch_refcount: {} -> {}", old_count, old_count + 1);
    }

    pub fn dec_batch_refcount(&self) {
        let old_count = self.inner.batch_refcount.fetch_sub(1, Ordering::AcqRel);
        tracing::trace!(
            "dec_batch_refcount: {} -> {}",
            old_count,
            old_count.saturating_sub(1)
        );

        if old_count == 0 {
            warn!("dec_batch_refcount called when refcount is already 0");
        }
    }

    pub fn batch_refcount(&self) -> usize {
        self.inner.batch_refcount.load(Ordering::Acquire)
    }

    #[inline]
    pub fn as_ptr(&self) -> *const u8 {
        self.inner.addr
    }

    #[inline]
    pub fn addr(&self) -> *const u8 {
        self.inner.addr
    }

    #[inline]
    pub fn size(&self) -> usize {
        self.inner.size
    }

    #[inline]
    pub fn frame_size(&self) -> usize {
        self.inner.frame_size
    }

    #[inline]
    pub fn frame_count(&self) -> usize {
        self.inner.frame_count
    }

    #[inline]
    pub fn headroom(&self) -> usize {
        self.inner.headroom
    }

    #[inline]
    pub fn available_frames(&self) -> usize {
        self.inner.free_frames.len()
    }

    #[inline]
    pub fn allocated_frames(&self) -> usize {
        self.inner.allocated_frames.load(Ordering::Relaxed)
    }

    pub fn utilization(&self) -> f64 {
        let allocated = self.inner.allocated_frames.load(Ordering::Relaxed);
        allocated as f64 / self.inner.frame_count as f64
    }

    pub fn strong_count(&self) -> usize {
        Arc::strong_count(&self.inner)
    }

    pub fn weak_count(&self) -> usize {
        Arc::weak_count(&self.inner)
    }
}
