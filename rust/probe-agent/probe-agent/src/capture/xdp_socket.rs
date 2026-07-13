// AF_XDP Socket 完整实现，包括 Ring 映射和初始化

use anyhow::{bail, Context, Result};
use std::os::unix::io::RawFd;
use std::ptr::NonNull;
use std::sync::atomic::AtomicU32;
use tracing::{debug, info, warn};

use super::ring::{CompQueue, FillQueue, RxQueue, TxQueue, XdpDesc};
use super::umem::Umem;
use super::xdp_sys::*;

/// XDP Socket 配置
#[derive(Debug, Clone)]
pub struct XskSocketConfig {
    pub rx_ring_size: u32,
    pub tx_ring_size: u32,
    pub fill_ring_size: u32,
    pub comp_ring_size: u32,
    pub bind_flags: u16,
    pub xdp_flags: u32,
}

impl Default for XskSocketConfig {
    fn default() -> Self {
        Self {
            rx_ring_size: 2048,
            tx_ring_size: 2048,
            fill_ring_size: 2048,
            comp_ring_size: 2048,
            bind_flags: 0,
            xdp_flags: 0,
        }
    }
}

impl XskSocketConfig {
    /// Use copy mode (works with all drivers)
    pub fn copy_mode() -> Self {
        Self {
            bind_flags: XDP_COPY,
            ..Default::default()
        }
    }

    /// Use zero-copy mode (requires driver support)
    pub fn zerocopy_mode() -> Self {
        Self {
            bind_flags: XDP_ZEROCOPY,
            ..Default::default()
        }
    }

    /// Validate configuration
    pub fn validate(&self) -> Result<()> {
        // Ring sizes must be power of 2
        if !self.rx_ring_size.is_power_of_two() {
            bail!("rx_ring_size must be power of 2");
        }
        if !self.tx_ring_size.is_power_of_two() {
            bail!("tx_ring_size must be power of 2");
        }
        if !self.fill_ring_size.is_power_of_two() {
            bail!("fill_ring_size must be power of 2");
        }
        if !self.comp_ring_size.is_power_of_two() {
            bail!("comp_ring_size must be power of 2");
        }
        Ok(())
    }
}

/// Mapped ring memory region
struct MappedRing {
    ptr: NonNull<u8>,
    size: usize,
}

impl MappedRing {
    /// Create a new mapped ring
    ///
    /// # Safety
    /// The fd must be a valid XDP socket.
    unsafe fn new(fd: RawFd, offset: u64, size: usize) -> Result<Self> {
        let ptr = mmap_ring(fd, offset, size)
            .context(format!("Failed to mmap ring at offset 0x{:x}", offset))?;

        let ptr = NonNull::new(ptr).ok_or_else(|| anyhow::anyhow!("mmap returned null pointer"))?;

        Ok(Self { ptr, size })
    }

    fn as_ptr(&self) -> *mut u8 {
        self.ptr.as_ptr()
    }
}

impl Drop for MappedRing {
    fn drop(&mut self) {
        unsafe {
            if let Err(e) = munmap_ring(self.ptr.as_ptr(), self.size) {
                warn!("Failed to munmap ring: {}", e);
            }
        }
    }
}

// Safety: The mapped memory is thread-safe when accessed through atomic operations
unsafe impl Send for MappedRing {}
unsafe impl Sync for MappedRing {}

/// Complete AF_XDP socket with all rings properly initialized
pub struct XskSocket {
    fd: RawFd,
    ifindex: u32,
    queue_id: u32,
    config: XskSocketConfig,

    // Mapped ring memory regions
    rx_ring_map: Option<MappedRing>,
    tx_ring_map: Option<MappedRing>,
    fill_ring_map: Option<MappedRing>,
    comp_ring_map: Option<MappedRing>,

    // Ring queues
    pub rx_queue: RxQueue,
    pub tx_queue: TxQueue,
    pub fill_queue: FillQueue,
    pub comp_queue: CompQueue,

    // Mmap offsets from kernel
    offsets: XdpMmapOffsets,
}

impl XskSocket {
    /// Create and initialize a new XSK socket
    pub fn new(umem: &Umem, ifindex: u32, queue_id: u32, config: XskSocketConfig) -> Result<Self> {
        config.validate()?;

        info!(
            "Creating XSK socket: ifindex={}, queue={}, rx_size={}, tx_size={}",
            ifindex, queue_id, config.rx_ring_size, config.tx_ring_size
        );

        // 1. Create AF_XDP socket
        let fd = Self::create_socket()?;
        debug!("Created AF_XDP socket: fd={}", fd);

        // Use a guard to ensure socket is closed on error
        let socket_guard = SocketGuard(fd);

        // 2. Register UMEM
        Self::register_umem(fd, umem)?;
        debug!(
            "UMEM registered: addr={:p}, size={}",
            umem.addr(),
            umem.size()
        );

        // 3. Set ring sizes
        Self::setup_ring_sizes(fd, &config)?;
        debug!("Ring sizes configured");

        // 4. Get mmap offsets from kernel
        let offsets = XdpMmapOffsets::get(fd).context("Failed to get XDP_MMAP_OFFSETS")?;
        debug!(
            "Got mmap offsets: rx.desc={}, tx.desc={}, fr.desc={}, cr.desc={}",
            offsets.rx.desc, offsets.tx.desc, offsets.fr.desc, offsets.cr.desc
        );

        // 5. Create ring queues
        let rx_queue = RxQueue::new(config.rx_ring_size)?;
        let tx_queue = TxQueue::new(config.tx_ring_size)?;
        let fill_queue = FillQueue::new(config.fill_ring_size)?;
        let comp_queue = CompQueue::new(config.comp_ring_size)?;

        // 6. Mmap all rings
        let (rx_ring_map, tx_ring_map, fill_ring_map, comp_ring_map) =
            unsafe { Self::mmap_rings(fd, &offsets, &config)? };
        debug!("All rings mapped successfully");

        // 7. Initialize ring pointers
        let mut socket = Self {
            fd,
            ifindex,
            queue_id,
            config,
            rx_ring_map: Some(rx_ring_map),
            tx_ring_map: Some(tx_ring_map),
            fill_ring_map: Some(fill_ring_map),
            comp_ring_map: Some(comp_ring_map),
            rx_queue,
            tx_queue,
            fill_queue,
            comp_queue,
            offsets,
        };

        unsafe {
            socket.init_ring_pointers()?;
        }
        debug!("Ring pointers initialized");

        // 8. Bind to interface and queue
        socket.bind()?;
        info!(
            "XSK socket bound to ifindex={}, queue={}",
            ifindex, queue_id
        );

        // 9. Set non-blocking mode
        socket.set_nonblocking()?;

        // Release the guard since we're successful
        socket_guard.release();

        Ok(socket)
    }

    /// Create AF_XDP socket
    fn create_socket() -> Result<RawFd> {
        let fd = unsafe { libc::socket(libc::AF_XDP, libc::SOCK_RAW, 0) };
        if fd < 0 {
            bail!(
                "Failed to create AF_XDP socket: {}",
                std::io::Error::last_os_error()
            );
        }
        Ok(fd)
    }

    /// Register UMEM with the socket
    fn register_umem(fd: RawFd, umem: &Umem) -> Result<()> {
        let umem_reg = XdpUmemReg {
            addr: umem.addr() as u64,
            len: umem.size() as u64,
            chunk_size: umem.frame_size() as u32,
            headroom: umem.headroom() as u32,
            flags: 0,
        };

        register_umem(fd, &umem_reg).context("Failed to register UMEM")?;

        Ok(())
    }

    /// Setup ring sizes via setsockopt
    fn setup_ring_sizes(fd: RawFd, config: &XskSocketConfig) -> Result<()> {
        set_ring_size(fd, XDP_UMEM_FILL_RING, config.fill_ring_size)
            .context("Failed to set fill ring size")?;

        set_ring_size(fd, XDP_UMEM_COMPLETION_RING, config.comp_ring_size)
            .context("Failed to set completion ring size")?;

        set_ring_size(fd, XDP_RX_RING, config.rx_ring_size)
            .context("Failed to set RX ring size")?;

        set_ring_size(fd, XDP_TX_RING, config.tx_ring_size)
            .context("Failed to set TX ring size")?;

        Ok(())
    }

    /// Mmap all ring buffers
    ///
    /// # Safety
    /// The fd must be a valid XDP socket with UMEM registered.
    unsafe fn mmap_rings(
        fd: RawFd,
        offsets: &XdpMmapOffsets,
        config: &XskSocketConfig,
    ) -> Result<(MappedRing, MappedRing, MappedRing, MappedRing)> {
        // Calculate mmap sizes for each ring
        let rx_size = ring_mmap_size(
            &offsets.rx,
            config.rx_ring_size,
            std::mem::size_of::<XdpDesc>(),
        );
        let tx_size = ring_mmap_size(
            &offsets.tx,
            config.tx_ring_size,
            std::mem::size_of::<XdpDesc>(),
        );
        let fill_size = ring_mmap_size(
            &offsets.fr,
            config.fill_ring_size,
            std::mem::size_of::<u64>(),
        );
        let comp_size = ring_mmap_size(
            &offsets.cr,
            config.comp_ring_size,
            std::mem::size_of::<u64>(),
        );

        debug!(
            "Mmap sizes: rx={}, tx={}, fill={}, comp={}",
            rx_size, tx_size, fill_size, comp_size
        );

        // Mmap each ring
        let rx_ring_map =
            MappedRing::new(fd, XDP_PGOFF_RX_RING, rx_size).context("Failed to mmap RX ring")?;

        let tx_ring_map =
            MappedRing::new(fd, XDP_PGOFF_TX_RING, tx_size).context("Failed to mmap TX ring")?;

        let fill_ring_map = MappedRing::new(fd, XDP_UMEM_PGOFF_FILL_RING, fill_size)
            .context("Failed to mmap Fill ring")?;

        let comp_ring_map = MappedRing::new(fd, XDP_UMEM_PGOFF_COMPLETION_RING, comp_size)
            .context("Failed to mmap Completion ring")?;

        Ok((rx_ring_map, tx_ring_map, fill_ring_map, comp_ring_map))
    }

    /// Initialize ring pointers from mmap'd memory
    ///
    /// # Safety
    /// Ring memory must be properly mapped before calling this.
    unsafe fn init_ring_pointers(&mut self) -> Result<()> {
        // RX Ring
        if let Some(ref rx_map) = self.rx_ring_map {
            let base = rx_map.as_ptr();
            let producer = base.add(self.offsets.rx.producer as usize) as *mut AtomicU32;
            let consumer = base.add(self.offsets.rx.consumer as usize) as *mut AtomicU32;
            let ring = base.add(self.offsets.rx.desc as usize) as *mut XdpDesc;

            self.rx_queue
                .set_ring_ptrs(producer, consumer, ring)
                .context("Failed to initialize RX ring pointers")?;
        }

        // TX Ring
        if let Some(ref tx_map) = self.tx_ring_map {
            let base = tx_map.as_ptr();
            let producer = base.add(self.offsets.tx.producer as usize) as *mut AtomicU32;
            let consumer = base.add(self.offsets.tx.consumer as usize) as *mut AtomicU32;
            let ring = base.add(self.offsets.tx.desc as usize) as *mut XdpDesc;

            self.tx_queue
                .set_ring_ptrs(producer, consumer, ring)
                .context("Failed to initialize TX ring pointers")?;
        }

        // Fill Ring
        if let Some(ref fill_map) = self.fill_ring_map {
            let base = fill_map.as_ptr();
            let producer = base.add(self.offsets.fr.producer as usize) as *mut AtomicU32;
            let consumer = base.add(self.offsets.fr.consumer as usize) as *mut AtomicU32;
            let ring = base.add(self.offsets.fr.desc as usize) as *mut u64;

            self.fill_queue
                .set_ring_ptrs(producer, consumer, ring)
                .context("Failed to initialize Fill ring pointers")?;
        }

        // Completion Ring
        if let Some(ref comp_map) = self.comp_ring_map {
            let base = comp_map.as_ptr();
            let producer = base.add(self.offsets.cr.producer as usize) as *mut AtomicU32;
            let consumer = base.add(self.offsets.cr.consumer as usize) as *mut AtomicU32;
            let ring = base.add(self.offsets.cr.desc as usize) as *mut u64;

            self.comp_queue
                .set_ring_ptrs(producer, consumer, ring)
                .context("Failed to initialize Completion ring pointers")?;
        }

        Ok(())
    }

    /// Bind socket to interface and queue
    fn bind(&self) -> Result<()> {
        let addr = SockaddrXdp {
            sxdp_family: libc::AF_XDP as u16,
            sxdp_flags: self.config.bind_flags,
            sxdp_ifindex: self.ifindex,
            sxdp_queue_id: self.queue_id,
            sxdp_shared_umem_fd: 0,
        };

        let ret = unsafe {
            libc::bind(
                self.fd,
                &addr as *const _ as *const libc::sockaddr,
                std::mem::size_of::<SockaddrXdp>() as libc::socklen_t,
            )
        };

        if ret < 0 {
            let err = std::io::Error::last_os_error();

            // Provide helpful error messages
            if err.raw_os_error() == Some(libc::ENODEV) {
                bail!("Interface with ifindex {} not found", self.ifindex);
            }
            if err.raw_os_error() == Some(libc::EBUSY) {
                bail!(
                    "Queue {} is already in use by another XSK socket",
                    self.queue_id
                );
            }
            if err.raw_os_error() == Some(libc::EOPNOTSUPP) {
                bail!(
                    "AF_XDP not supported for this interface/queue combination. \
                     Try using XDP_COPY mode or check driver support."
                );
            }

            bail!("Failed to bind XSK socket: {}", err);
        }

        Ok(())
    }

    /// Set socket to non-blocking mode
    fn set_nonblocking(&self) -> Result<()> {
        let flags = unsafe { libc::fcntl(self.fd, libc::F_GETFL, 0) };
        if flags < 0 {
            bail!("fcntl F_GETFL failed: {}", std::io::Error::last_os_error());
        }

        let ret = unsafe { libc::fcntl(self.fd, libc::F_SETFL, flags | libc::O_NONBLOCK) };
        if ret < 0 {
            bail!("fcntl F_SETFL failed: {}", std::io::Error::last_os_error());
        }

        Ok(())
    }

    /// Get socket file descriptor
    pub fn fd(&self) -> RawFd {
        self.fd
    }

    /// Get interface index
    pub fn ifindex(&self) -> u32 {
        self.ifindex
    }

    /// Get queue ID
    pub fn queue_id(&self) -> u32 {
        self.queue_id
    }

    /// Get XDP statistics
    pub fn statistics(&self) -> Result<XdpStatistics> {
        XdpStatistics::get(self.fd).context("Failed to get XDP statistics")
    }

    /// Check if RX ring needs wakeup (for busy polling)
    pub fn rx_needs_wakeup(&self) -> bool {
        if let Some(ref rx_map) = self.rx_ring_map {
            if self.offsets.rx.flags != 0 {
                unsafe {
                    let flags_ptr =
                        rx_map.as_ptr().add(self.offsets.rx.flags as usize) as *const u32;
                    let flags = std::ptr::read_volatile(flags_ptr);
                    return (flags & XDP_RING_NEED_WAKEUP) != 0;
                }
            }
        }
        false
    }

    /// Wakeup the kernel to process packets (used with need_wakeup flag)
    pub fn wakeup(&self) -> Result<()> {
        let ret = unsafe {
            libc::sendto(
                self.fd,
                std::ptr::null(),
                0,
                libc::MSG_DONTWAIT,
                std::ptr::null(),
                0,
            )
        };

        if ret < 0 {
            let err = std::io::Error::last_os_error();
            if err.kind() != std::io::ErrorKind::WouldBlock
                && err.raw_os_error() != Some(libc::EAGAIN)
                && err.raw_os_error() != Some(libc::ENOBUFS)
            {
                bail!("sendto wakeup failed: {}", err);
            }
        }

        Ok(())
    }

    /// Poll for available data
    pub fn poll(&self, timeout_ms: i32) -> Result<bool> {
        let mut pfd = libc::pollfd {
            fd: self.fd,
            events: libc::POLLIN,
            revents: 0,
        };

        let ret = unsafe { libc::poll(&mut pfd, 1, timeout_ms) };

        if ret < 0 {
            let err = std::io::Error::last_os_error();
            if err.kind() == std::io::ErrorKind::Interrupted {
                return Ok(false);
            }
            bail!("poll failed: {}", err);
        }

        Ok(ret > 0 && (pfd.revents & libc::POLLIN) != 0)
    }
}

impl Drop for XskSocket {
    fn drop(&mut self) {
        // Rings are dropped automatically via MappedRing's Drop impl

        // Close socket
        if self.fd >= 0 {
            unsafe {
                libc::close(self.fd);
            }
            debug!("XSK socket closed: fd={}", self.fd);
        }
    }
}

// Safety: XskSocket can be sent between threads
unsafe impl Send for XskSocket {}

/// Guard to ensure socket is closed on error during construction
struct SocketGuard(RawFd);

impl SocketGuard {
    fn release(self) {
        std::mem::forget(self);
    }
}

impl Drop for SocketGuard {
    fn drop(&mut self) {
        if self.0 >= 0 {
            unsafe {
                libc::close(self.0);
            }
            debug!("SocketGuard: closed fd={} due to error", self.0);
        }
    }
}
