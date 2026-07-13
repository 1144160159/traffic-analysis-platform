use super::{CaptureStats, Capturer, FrameInfo, PacketBatch, Umem, UmemConfig};
use crate::config::CaptureConfig;
use crate::metrics;
use anyhow::{bail, Result};
use libc::{
    bind, c_int, c_void, close, iovec, mmsghdr, msghdr, recvmmsg, setsockopt, sockaddr_ll, socket,
    AF_PACKET, ETH_P_ALL, SOCK_RAW, SOL_SOCKET, SO_RCVBUF,
};
use std::os::fd::RawFd;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, error, info, trace, warn};

const RECV_BATCH_SIZE: usize = 64;

#[derive(Default)]
struct AfPacketStats {
    frame_alloc_failures: AtomicU64,
    frame_write_failures: AtomicU64,
    recv_errors: AtomicU64,
    recv_would_block: AtomicU64,
    recv_success: AtomicU64,
}

pub struct AfPacketCapture {
    config: CaptureConfig,
    socket_fd: Option<RawFd>,
    recv_buffers: Vec<Vec<u8>>,
    umem: Arc<Umem>,
    stats: CaptureStats,
    internal_stats: AfPacketStats,
    running: bool,
    ifindex: i32,
}

impl AfPacketCapture {
    pub fn new(config: &CaptureConfig) -> Result<Self> {
        let umem_config = UmemConfig {
            frame_size: config.frame_size,
            frame_count: config.frame_count,
            fill_queue_size: 4096,
            comp_queue_size: 4096,
            headroom: 0,
            use_huge_pages: false,
        };
        let umem = Arc::new(Umem::new(&umem_config)?);
        let ifindex = Self::get_interface_index_static(&config.interface)?;
        info!(
            "AF_PACKET capturer created: interface={} (ifindex={}), frame_size={}, frame_count={}",
            config.interface, ifindex, config.frame_size, config.frame_count
        );
        let recv_buffers = (0..RECV_BATCH_SIZE).map(|_| vec![0u8; 65536]).collect();
        Ok(Self {
            config: config.clone(),
            socket_fd: None,
            recv_buffers,
            umem,
            stats: CaptureStats::default(),
            internal_stats: AfPacketStats::default(),
            running: false,
            ifindex,
        })
    }

    fn get_interface_index_static(interface: &str) -> Result<i32> {
        use std::ffi::CString;
        let ifname = CString::new(interface)?;
        unsafe {
            let idx = libc::if_nametoindex(ifname.as_ptr());
            if idx == 0 {
                bail!("Interface {} not found", interface);
            }
            Ok(idx as i32)
        }
    }

    fn try_allocate_frame(&self) -> Option<usize> {
        match self.umem.alloc_frame() {
            Some(idx) => Some(idx),
            None => {
                self.internal_stats
                    .frame_alloc_failures
                    .fetch_add(1, Ordering::Relaxed);
                None
            }
        }
    }

    fn write_to_frame(&self, frame_idx: usize, data: &[u8]) -> bool {
        let frame_size = self.umem.frame_size();
        let addr = frame_idx * frame_size;
        if data.len() > frame_size {
            self.internal_stats
                .frame_write_failures
                .fetch_add(1, Ordering::Relaxed);
            warn!(
                "Packet too large for frame: {} > {}",
                data.len(),
                frame_size
            );
            return false;
        }
        if let Some(frame_data) = self.umem.get_data_mut(addr, data.len()) {
            frame_data.copy_from_slice(data);
            true
        } else {
            self.internal_stats
                .frame_write_failures
                .fetch_add(1, Ordering::Relaxed);
            false
        }
    }

    fn set_recv_timeout(fd: RawFd, timeout_ms: i32) -> Result<()> {
        unsafe {
            let timeval = libc::timeval {
                tv_sec: 0,
                tv_usec: (timeout_ms * 1000) as i64,
            };
            let ret = libc::setsockopt(
                fd,
                libc::SOL_SOCKET,
                libc::SO_RCVTIMEO,
                &timeval as *const _ as *const c_void,
                std::mem::size_of::<libc::timeval>() as u32,
            );
            if ret < 0 {
                bail!(
                    "Failed to set recv timeout: {}",
                    std::io::Error::last_os_error()
                );
            }
        }
        Ok(())
    }

    pub fn internal_stats(&self) -> (u64, u64, u64, u64, u64) {
        (
            self.internal_stats
                .frame_alloc_failures
                .load(Ordering::Relaxed),
            self.internal_stats
                .frame_write_failures
                .load(Ordering::Relaxed),
            self.internal_stats.recv_errors.load(Ordering::Relaxed),
            self.internal_stats.recv_would_block.load(Ordering::Relaxed),
            self.internal_stats.recv_success.load(Ordering::Relaxed),
        )
    }
}

// Apply a BPF filter to an AF_PACKET socket using raw BPF bytecode.
// The filter expression should be in the format produced by `tcpdump -ddd`:
// a comma-separated list of 4-tuples: "code,jt,jf,k;code,jt,jf,k;..."
// Example: "40,0,0,12;21,0,7,2048;..." (captures only IP packets)
fn apply_bpf_filter(fd: RawFd, bpf_expr: &str) -> Result<()> {
    if bpf_expr.is_empty() {
        return Ok(());
    }
    // Parse BPF bytecode in format: "code,jt,jf,k;code,jt,jf,k;..."
    let mut instructions: Vec<libc::sock_filter> = Vec::new();
    for instr_str in bpf_expr.split(';') {
        let instr_str = instr_str.trim();
        if instr_str.is_empty() {
            continue;
        }
        let parts: Vec<&str> = instr_str.split(',').collect();
        if parts.len() != 4 {
            bail!(
                "Invalid BPF instruction format: expected 'code,jt,jf,k', got '{}'",
                instr_str
            );
        }
        let code: u16 = parts[0]
            .trim()
            .parse()
            .map_err(|e| anyhow::anyhow!("Invalid BPF code '{}': {}", parts[0], e))?;
        let jt: u8 = parts[1]
            .trim()
            .parse()
            .map_err(|e| anyhow::anyhow!("Invalid BPF jt '{}': {}", parts[1], e))?;
        let jf: u8 = parts[2]
            .trim()
            .parse()
            .map_err(|e| anyhow::anyhow!("Invalid BPF jf '{}': {}", parts[2], e))?;
        let k: u32 = parts[3]
            .trim()
            .parse()
            .map_err(|e| anyhow::anyhow!("Invalid BPF k '{}': {}", parts[3], e))?;
        instructions.push(libc::sock_filter { code, jt, jf, k });
    }
    if instructions.is_empty() {
        return Ok(());
    }
    let prog = libc::sock_fprog {
        len: instructions.len() as u16,
        filter: instructions.as_ptr() as *mut libc::sock_filter,
    };
    // SAFETY: instructions vector is valid, fd is a valid socket
    unsafe {
        let ret = libc::setsockopt(
            fd,
            libc::SOL_SOCKET,
            libc::SO_ATTACH_FILTER,
            &prog as *const _ as *const libc::c_void,
            std::mem::size_of::<libc::sock_fprog>() as u32,
        );
        if ret < 0 {
            bail!(
                "Failed to attach BPF filter: {}",
                std::io::Error::last_os_error()
            );
        }
    }
    Ok(())
}

// SAFETY: AfPacketCapture owns a RawFd (socket) which is safe to send across threads
// because the kernel handles concurrent access to socket descriptors atomically.
// The Umem uses Arc for thread-safe shared access.
unsafe impl Send for AfPacketCapture {}
// SAFETY: The socket fd supports concurrent recvmmsg calls from multiple threads.
// Internal stats use AtomicU64 for lock-free updates. Arc<Umem> is Sync.
unsafe impl Sync for AfPacketCapture {}

#[async_trait::async_trait]
impl Capturer for AfPacketCapture {
    async fn start(&mut self) -> Result<()> {
        if self.running {
            return Ok(());
        }
        if self.config.promiscuous_mode {
            use super::promisc::set_promiscuous_mode;
            match set_promiscuous_mode(&self.config.interface, true) {
                Ok(_) => {
                    info!("✓ Promiscuous mode enabled on {}", self.config.interface);
                }
                Err(e) => {
                    warn!(
                        "Failed to enable promiscuous mode: {}. Continuing without it.",
                        e
                    );
                    warn!("Hint: Run with sudo or grant CAP_NET_ADMIN capability");
                }
            }
        }
        info!(
            "Starting AF_PACKET capture on interface: {}",
            self.config.interface
        );
        unsafe {
            let fd = socket(AF_PACKET, SOCK_RAW, (ETH_P_ALL as u16).to_be() as c_int);
            if fd < 0 {
                bail!(
                    "Failed to create AF_PACKET socket: {}",
                    std::io::Error::last_os_error()
                );
            }
            debug!("Socket created: fd={}", fd);
            let bufsize: c_int = self.config.buffer_size as c_int;
            debug!("Setting socket buffer size: {} bytes", bufsize);
            if setsockopt(
                fd,
                SOL_SOCKET,
                SO_RCVBUF,
                &bufsize as *const _ as *const c_void,
                std::mem::size_of::<c_int>() as u32,
            ) < 0
            {
                warn!(
                    "Failed to set socket buffer size: {}",
                    std::io::Error::last_os_error()
                );
            } else {
                debug!("Socket buffer size set successfully");
            }
            // Apply BPF filter if configured
            if let Some(ref bpf_expr) = self.config.bpf_filter {
                match apply_bpf_filter(fd, bpf_expr) {
                    Ok(_) => debug!("BPF filter applied: {}", bpf_expr),
                    Err(e) => warn!("Failed to apply BPF filter '{}': {}", bpf_expr, e),
                }
            }
            let mut addr: sockaddr_ll = std::mem::zeroed();
            addr.sll_family = AF_PACKET as u16;
            addr.sll_protocol = (ETH_P_ALL as u16).to_be();
            addr.sll_ifindex = self.ifindex;
            debug!(
                "Binding socket: ifindex={}, protocol=0x{:04x}",
                self.ifindex,
                (ETH_P_ALL as u16).to_be()
            );
            if bind(
                fd,
                &addr as *const _ as *const libc::sockaddr,
                std::mem::size_of::<sockaddr_ll>() as u32,
            ) < 0
            {
                close(fd);
                bail!("Failed to bind socket: {}", std::io::Error::last_os_error());
            }
            debug!("Socket bound successfully");
            if let Err(e) = Self::set_recv_timeout(fd, 100) {
                warn!("Failed to set recv timeout: {}", e);
            } else {
                debug!("Recv timeout set to 100ms");
            }
            self.socket_fd = Some(fd);
        }
        self.running = true;
        info!("AF_PACKET capture started successfully");
        Ok(())
    }

    async fn stop(&mut self) -> Result<()> {
        if !self.running {
            return Ok(());
        }
        info!("Stopping AF_PACKET capture");
        if let Some(fd) = self.socket_fd.take() {
            unsafe {
                close(fd);
            }
        }
        let (alloc_fail, write_fail, recv_err, would_block, recv_ok) = self.internal_stats();
        info!(
            "AF_PACKET stats: recv_ok={}, would_block={}, alloc_fail={}, write_fail={}, recv_err={}",
            recv_ok, would_block, alloc_fail, write_fail, recv_err
        );
        if self.config.promiscuous_mode {
            use super::promisc::set_promiscuous_mode;
            if let Err(e) = set_promiscuous_mode(&self.config.interface, false) {
                warn!("Failed to disable promiscuous mode: {}", e);
            } else {
                info!("✓ Promiscuous mode disabled on {}", self.config.interface);
            }
        }
        self.running = false;
        info!("AF_PACKET capture stopped");
        Ok(())
    }

    fn poll(&mut self) -> Result<Option<PacketBatch>> {
        if !self.running {
            return Ok(None);
        }
        let fd = match self.socket_fd {
            Some(fd) => fd,
            None => return Ok(None),
        };
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_micros() as u64;
        let mut frames = Vec::with_capacity(RECV_BATCH_SIZE);

        let mut iovecs: Vec<iovec> = self
            .recv_buffers
            .iter_mut()
            .map(|buf| iovec {
                iov_base: buf.as_mut_ptr() as *mut c_void,
                iov_len: buf.len(),
            })
            .collect();
        let mut msgs: Vec<mmsghdr> = iovecs
            .iter_mut()
            .map(|iov| mmsghdr {
                msg_hdr: msghdr {
                    msg_name: std::ptr::null_mut(),
                    msg_namelen: 0,
                    msg_iov: iov,
                    msg_iovlen: 1,
                    msg_control: std::ptr::null_mut(),
                    msg_controllen: 0,
                    msg_flags: 0,
                },
                msg_len: 0,
            })
            .collect();

        let received = unsafe {
            recvmmsg(
                fd,
                msgs.as_mut_ptr(),
                RECV_BATCH_SIZE as u32,
                0,
                std::ptr::null_mut(),
            )
        };

        if received < 0 {
            let err = std::io::Error::last_os_error();
            match err.kind() {
                std::io::ErrorKind::WouldBlock | std::io::ErrorKind::TimedOut => {
                    self.internal_stats
                        .recv_would_block
                        .fetch_add(1, Ordering::Relaxed);
                    trace!("No data available");
                    return Ok(None);
                }
                _ => {
                    self.internal_stats
                        .recv_errors
                        .fetch_add(1, Ordering::Relaxed);
                    error!("recvmmsg() error: {:?}", err);
                    return Ok(None);
                }
            }
        }

        let received = received as usize;
        if received == 0 {
            return Ok(None);
        }

        for i in 0..received {
            let len = msgs[i].msg_len as usize;
            if len == 0 {
                continue;
            }
            self.internal_stats
                .recv_success
                .fetch_add(1, Ordering::Relaxed);
            trace!("Received packet: len={}, idx={}", len, i);
            match self.try_allocate_frame() {
                Some(frame_idx) => {
                    if self.write_to_frame(frame_idx, &self.recv_buffers[i][..len]) {
                        frames.push(FrameInfo {
                            idx: frame_idx,
                            offset: 0,
                            len: len as u32,
                            timestamp,
                        });
                        self.stats.packets_received += 1;
                        self.stats.bytes_received += len as u64;
                        metrics::PACKETS_CAPTURED.inc();
                        metrics::BYTES_CAPTURED.inc_by(len as f64);
                    } else {
                        self.umem.free_frame(frame_idx);
                        self.stats.packets_dropped += 1;
                        metrics::PACKETS_DROPPED.inc();
                    }
                }
                None => {
                    self.stats.packets_dropped += 1;
                    metrics::PACKETS_DROPPED.inc();
                    if self.stats.packets_dropped % 1000 == 0 {
                        warn!(
                            "High frame allocation failure rate: {} dropped, {} available",
                            self.stats.packets_dropped,
                            self.umem.available_frames()
                        );
                    }
                }
            }
        }

        if self.stats.packets_received % 100 == 0 && self.stats.packets_received > 0 {
            debug!(
                "AF_PACKET stats: received={}, dropped={}, frames_available={}",
                self.stats.packets_received,
                self.stats.packets_dropped,
                self.umem.available_frames()
            );
        }

        if frames.is_empty() {
            Ok(None)
        } else {
            debug!("Returning batch of {} frames", frames.len());
            Ok(Some(PacketBatch::new(self.umem.clone(), frames)))
        }
    }

    fn stats(&self) -> CaptureStats {
        self.stats.clone()
    }
}

impl Drop for AfPacketCapture {
    fn drop(&mut self) {
        if let Some(fd) = self.socket_fd.take() {
            unsafe {
                libc::close(fd);
            }
        }
    }
}
