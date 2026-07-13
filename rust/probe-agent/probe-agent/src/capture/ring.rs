use anyhow::{bail, Result};
use std::ptr;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use tracing::{debug, error, trace};

#[repr(C)]
#[derive(Debug, Clone, Copy, Default)]
pub struct XdpDesc {
    pub addr: u64,
    pub len: u32,
    pub options: u32,
}

/// Internal ring structure with producer/consumer pointers
struct RingInner {
    producer: *mut AtomicU32,
    consumer: *mut AtomicU32,
    flags: *mut u32,
    mask: u32,
    size: u32,
}

impl RingInner {
    fn new(size: u32) -> Result<Self> {
        if size == 0 || (size & (size - 1)) != 0 {
            bail!("Ring size must be a power of 2, got {}", size);
        }

        Ok(Self {
            producer: ptr::null_mut(),
            consumer: ptr::null_mut(),
            flags: ptr::null_mut(),
            mask: size - 1,
            size,
        })
    }

    fn is_initialized(&self) -> bool {
        !self.producer.is_null() && !self.consumer.is_null()
    }

    fn available_for_producer(&self) -> u32 {
        if !self.is_initialized() {
            return 0;
        }

        unsafe {
            let prod = (*self.producer).load(Ordering::Acquire);
            let cons = (*self.consumer).load(Ordering::Acquire);
            self.size.wrapping_sub(prod.wrapping_sub(cons))
        }
    }

    fn available_for_consumer(&self) -> u32 {
        if !self.is_initialized() {
            return 0;
        }

        unsafe {
            let prod = (*self.producer).load(Ordering::Acquire);
            let cons = (*self.consumer).load(Ordering::Acquire);
            prod.wrapping_sub(cons)
        }
    }
}

// Safety: The pointers point to shared memory that is accessed atomically
unsafe impl Send for RingInner {}
unsafe impl Sync for RingInner {}

/// Fill Queue - used to provide empty buffers to the kernel
pub struct FillQueue {
    inner: RingInner,
    ring: *mut u64, // Array of UMEM addresses
    cached_prod: u32,
    cached_cons: u32,
    initialized: AtomicBool,
}

impl FillQueue {
    pub fn new(size: u32) -> Result<Self> {
        Ok(Self {
            inner: RingInner::new(size)?,
            ring: ptr::null_mut(),
            cached_prod: 0,
            cached_cons: 0,
            initialized: AtomicBool::new(false),
        })
    }

    /// Set ring pointers from mmap'd memory
    ///
    /// # Safety
    /// The pointers must be valid and properly aligned.
    pub unsafe fn set_ring_ptrs(
        &mut self,
        producer: *mut AtomicU32,
        consumer: *mut AtomicU32,
        ring: *mut u64,
    ) -> Result<()> {
        if producer.is_null() {
            bail!("Producer pointer is null");
        }
        if consumer.is_null() {
            bail!("Consumer pointer is null");
        }
        if ring.is_null() {
            bail!("Ring pointer is null");
        }

        // Check alignment
        if (producer as usize) % std::mem::align_of::<AtomicU32>() != 0 {
            bail!("Producer pointer not aligned");
        }
        if (consumer as usize) % std::mem::align_of::<AtomicU32>() != 0 {
            bail!("Consumer pointer not aligned");
        }
        if (ring as usize) % std::mem::align_of::<u64>() != 0 {
            bail!("Ring pointer not aligned");
        }

        self.inner.producer = producer;
        self.inner.consumer = consumer;
        self.ring = ring;

        // Initialize cached values
        self.cached_prod = (*producer).load(Ordering::Acquire);
        self.cached_cons = (*consumer).load(Ordering::Acquire);

        self.initialized.store(true, Ordering::Release);

        debug!(
            "FillQueue initialized: producer={:p}, consumer={:p}, ring={:p}, size={}",
            producer, consumer, ring, self.inner.size
        );

        Ok(())
    }

    /// Fill the queue with buffer addresses
    pub fn fill(&mut self, addrs: &[u64]) -> usize {
        if !self.initialized.load(Ordering::Acquire) {
            error!("FillQueue not initialized");
            return 0;
        }

        if addrs.is_empty() {
            return 0;
        }

        unsafe {
            // Refresh cached consumer
            self.cached_cons = (*self.inner.consumer).load(Ordering::Acquire);

            let free_slots = self
                .inner
                .size
                .wrapping_sub(self.cached_prod.wrapping_sub(self.cached_cons))
                as usize;

            let to_fill = free_slots.min(addrs.len());

            if to_fill == 0 {
                trace!(
                    "FillQueue full: prod={}, cons={}",
                    self.cached_prod,
                    self.cached_cons
                );
                return 0;
            }

            for (i, &addr) in addrs.iter().take(to_fill).enumerate() {
                let idx = (self.cached_prod.wrapping_add(i as u32) & self.inner.mask) as usize;
                ptr::write_volatile(self.ring.add(idx), addr);
            }

            // Memory barrier before updating producer
            std::sync::atomic::fence(Ordering::Release);

            self.cached_prod = self.cached_prod.wrapping_add(to_fill as u32);
            (*self.inner.producer).store(self.cached_prod, Ordering::Release);

            trace!(
                "Filled {} addresses, new prod={}",
                to_fill,
                self.cached_prod
            );
            to_fill
        }
    }

    pub fn available(&self) -> u32 {
        self.inner.available_for_producer()
    }

    pub fn size(&self) -> u32 {
        self.inner.size
    }

    pub fn is_initialized(&self) -> bool {
        self.initialized.load(Ordering::Acquire)
    }

    // Legacy compatibility methods
    pub fn producer_offset(&self) -> u64 {
        0
    }
    pub fn consumer_offset(&self) -> u64 {
        0
    }
    pub fn desc_offset(&self) -> u64 {
        0
    }
    pub fn set_offsets(&mut self, _producer: u64, _consumer: u64, _desc: u64) {}
}

unsafe impl Send for FillQueue {}
unsafe impl Sync for FillQueue {}

/// Completion Queue - kernel returns completed TX buffer addresses here
pub struct CompQueue {
    inner: RingInner,
    ring: *mut u64, // Array of UMEM addresses
    cached_prod: u32,
    cached_cons: u32,
    initialized: AtomicBool,
}

impl CompQueue {
    pub fn new(size: u32) -> Result<Self> {
        Ok(Self {
            inner: RingInner::new(size)?,
            ring: ptr::null_mut(),
            cached_prod: 0,
            cached_cons: 0,
            initialized: AtomicBool::new(false),
        })
    }

    /// Set ring pointers from mmap'd memory
    pub unsafe fn set_ring_ptrs(
        &mut self,
        producer: *mut AtomicU32,
        consumer: *mut AtomicU32,
        ring: *mut u64,
    ) -> Result<()> {
        if producer.is_null() || consumer.is_null() || ring.is_null() {
            bail!("Null pointer provided to CompQueue");
        }

        self.inner.producer = producer;
        self.inner.consumer = consumer;
        self.ring = ring;

        self.cached_prod = (*producer).load(Ordering::Acquire);
        self.cached_cons = (*consumer).load(Ordering::Acquire);

        self.initialized.store(true, Ordering::Release);

        debug!(
            "CompQueue initialized: producer={:p}, consumer={:p}, ring={:p}",
            producer, consumer, ring
        );

        Ok(())
    }

    /// Complete buffers - retrieve addresses of completed TX buffers
    pub fn complete(&mut self, addrs: &mut [u64]) -> usize {
        if !self.initialized.load(Ordering::Acquire) {
            return 0;
        }

        if addrs.is_empty() {
            return 0;
        }

        unsafe {
            // Refresh cached producer
            self.cached_prod = (*self.inner.producer).load(Ordering::Acquire);

            let available = self.cached_prod.wrapping_sub(self.cached_cons) as usize;
            let to_complete = available.min(addrs.len());

            if to_complete == 0 {
                return 0;
            }

            for i in 0..to_complete {
                let idx = (self.cached_cons.wrapping_add(i as u32) & self.inner.mask) as usize;
                addrs[i] = ptr::read_volatile(self.ring.add(idx));
            }

            // Memory barrier before updating consumer
            std::sync::atomic::fence(Ordering::Release);

            self.cached_cons = self.cached_cons.wrapping_add(to_complete as u32);
            (*self.inner.consumer).store(self.cached_cons, Ordering::Release);

            trace!("Completed {} addresses", to_complete);
            to_complete
        }
    }

    pub fn available(&self) -> u32 {
        self.inner.available_for_consumer()
    }

    pub fn size(&self) -> u32 {
        self.inner.size
    }

    pub fn is_initialized(&self) -> bool {
        self.initialized.load(Ordering::Acquire)
    }

    // Legacy compatibility methods
    pub fn producer_offset(&self) -> u64 {
        0
    }
    pub fn consumer_offset(&self) -> u64 {
        0
    }
    pub fn desc_offset(&self) -> u64 {
        0
    }
    pub fn set_offsets(&mut self, _producer: u64, _consumer: u64, _desc: u64) {}
}

unsafe impl Send for CompQueue {}
unsafe impl Sync for CompQueue {}

/// RX Queue - kernel places received packet descriptors here
pub struct RxQueue {
    inner: RingInner,
    ring: *mut XdpDesc,
    cached_prod: u32,
    cached_cons: u32,
    initialized: AtomicBool,
}

impl RxQueue {
    pub fn new(size: u32) -> Result<Self> {
        Ok(Self {
            inner: RingInner::new(size)?,
            ring: ptr::null_mut(),
            cached_prod: 0,
            cached_cons: 0,
            initialized: AtomicBool::new(false),
        })
    }

    /// Set ring pointers from mmap'd memory
    pub unsafe fn set_ring_ptrs(
        &mut self,
        producer: *mut AtomicU32,
        consumer: *mut AtomicU32,
        ring: *mut XdpDesc,
    ) -> Result<()> {
        if producer.is_null() || consumer.is_null() || ring.is_null() {
            bail!("Null pointer provided to RxQueue");
        }

        self.inner.producer = producer;
        self.inner.consumer = consumer;
        self.ring = ring;

        self.cached_prod = (*producer).load(Ordering::Acquire);
        self.cached_cons = (*consumer).load(Ordering::Acquire);

        self.initialized.store(true, Ordering::Release);

        debug!(
            "RxQueue initialized: producer={:p}, consumer={:p}, ring={:p}",
            producer, consumer, ring
        );

        Ok(())
    }

    /// Receive packet descriptors from the kernel
    pub fn receive(&mut self, descs: &mut [XdpDesc]) -> usize {
        if !self.initialized.load(Ordering::Acquire) {
            error!("RxQueue not initialized");
            return 0;
        }

        if descs.is_empty() {
            return 0;
        }

        unsafe {
            // Refresh cached producer
            self.cached_prod = (*self.inner.producer).load(Ordering::Acquire);

            let available = self.cached_prod.wrapping_sub(self.cached_cons) as usize;
            let to_receive = available.min(descs.len());

            if to_receive == 0 {
                return 0;
            }

            for i in 0..to_receive {
                let idx = (self.cached_cons.wrapping_add(i as u32) & self.inner.mask) as usize;
                descs[i] = ptr::read_volatile(self.ring.add(idx));
            }

            // Memory barrier before updating consumer
            std::sync::atomic::fence(Ordering::Release);

            self.cached_cons = self.cached_cons.wrapping_add(to_receive as u32);
            (*self.inner.consumer).store(self.cached_cons, Ordering::Release);

            trace!("Received {} descriptors", to_receive);
            to_receive
        }
    }

    pub fn available(&self) -> u32 {
        if !self.initialized.load(Ordering::Acquire) {
            return 0;
        }

        unsafe {
            let prod = (*self.inner.producer).load(Ordering::Acquire);
            let cons = (*self.inner.consumer).load(Ordering::Acquire);
            prod.wrapping_sub(cons)
        }
    }

    pub fn size(&self) -> u32 {
        self.inner.size
    }

    pub fn is_initialized(&self) -> bool {
        self.initialized.load(Ordering::Acquire)
    }

    // Legacy compatibility methods
    pub fn producer_offset(&self) -> u64 {
        0
    }
    pub fn consumer_offset(&self) -> u64 {
        0
    }
    pub fn desc_offset(&self) -> u64 {
        0
    }
    pub fn set_offsets(&mut self, _producer: u64, _consumer: u64, _desc: u64) {}
}

unsafe impl Send for RxQueue {}
unsafe impl Sync for RxQueue {}

/// TX Queue - user places packet descriptors here for transmission
pub struct TxQueue {
    inner: RingInner,
    ring: *mut XdpDesc,
    cached_prod: u32,
    cached_cons: u32,
    initialized: AtomicBool,
}

impl TxQueue {
    pub fn new(size: u32) -> Result<Self> {
        Ok(Self {
            inner: RingInner::new(size)?,
            ring: ptr::null_mut(),
            cached_prod: 0,
            cached_cons: 0,
            initialized: AtomicBool::new(false),
        })
    }

    /// Set ring pointers from mmap'd memory
    pub unsafe fn set_ring_ptrs(
        &mut self,
        producer: *mut AtomicU32,
        consumer: *mut AtomicU32,
        ring: *mut XdpDesc,
    ) -> Result<()> {
        if producer.is_null() || consumer.is_null() || ring.is_null() {
            bail!("Null pointer provided to TxQueue");
        }

        self.inner.producer = producer;
        self.inner.consumer = consumer;
        self.ring = ring;

        self.cached_prod = (*producer).load(Ordering::Acquire);
        self.cached_cons = (*consumer).load(Ordering::Acquire);

        self.initialized.store(true, Ordering::Release);

        debug!(
            "TxQueue initialized: producer={:p}, consumer={:p}, ring={:p}",
            producer, consumer, ring
        );

        Ok(())
    }

    /// Submit packet descriptors for transmission
    pub fn transmit(&mut self, descs: &[XdpDesc]) -> usize {
        if !self.initialized.load(Ordering::Acquire) {
            error!("TxQueue not initialized");
            return 0;
        }

        if descs.is_empty() {
            return 0;
        }

        unsafe {
            // Refresh cached consumer
            self.cached_cons = (*self.inner.consumer).load(Ordering::Acquire);

            let free_slots = self
                .inner
                .size
                .wrapping_sub(self.cached_prod.wrapping_sub(self.cached_cons))
                as usize;

            let to_transmit = free_slots.min(descs.len());

            if to_transmit == 0 {
                return 0;
            }

            for (i, desc) in descs.iter().take(to_transmit).enumerate() {
                let idx = (self.cached_prod.wrapping_add(i as u32) & self.inner.mask) as usize;
                ptr::write_volatile(self.ring.add(idx), *desc);
            }

            // Memory barrier before updating producer
            std::sync::atomic::fence(Ordering::Release);

            self.cached_prod = self.cached_prod.wrapping_add(to_transmit as u32);
            (*self.inner.producer).store(self.cached_prod, Ordering::Release);

            trace!("Transmitted {} descriptors", to_transmit);
            to_transmit
        }
    }

    pub fn available(&self) -> u32 {
        self.inner.available_for_producer()
    }

    pub fn size(&self) -> u32 {
        self.inner.size
    }

    pub fn is_initialized(&self) -> bool {
        self.initialized.load(Ordering::Acquire)
    }

    // Legacy compatibility methods
    pub fn producer_offset(&self) -> u64 {
        0
    }
    pub fn consumer_offset(&self) -> u64 {
        0
    }
    pub fn desc_offset(&self) -> u64 {
        0
    }
    pub fn set_offsets(&mut self, _producer: u64, _consumer: u64, _desc: u64) {}
}

unsafe impl Send for TxQueue {}
unsafe impl Sync for TxQueue {}
