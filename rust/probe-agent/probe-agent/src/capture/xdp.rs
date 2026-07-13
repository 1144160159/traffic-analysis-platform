use anyhow::{bail, Context, Result};
use std::env;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::{debug, error, info, trace, warn};

use super::promisc::PromiscuousMode;
use super::ring::XdpDesc;
use super::umem::{Umem, UmemConfig};
use super::xdp_socket::{XskSocket, XskSocketConfig};
use super::xdp_sys::XDP_COPY;
use super::{CaptureConfig, CaptureMode, CaptureStats, Capturer};
use super::{FrameInfo, PacketBatch};
use crate::metrics;
use aya::maps::XskMap;
use aya::programs::{Xdp, XdpFlags};
use aya::Bpf;

const XSK_BATCH_SIZE: usize = 64;

pub struct XdpCapture {
    config: CaptureConfig,
    umem: Arc<Umem>,
    xsk_socket: Option<XskSocket>,
    bpf: Option<Bpf>,
    stats: CaptureStats,
    running: bool,
    pending_fill_addrs: Vec<u64>,
    promisc_guard: Option<PromiscuousMode>,
    ifindex: i32,
}

impl XdpCapture {
    pub async fn new(config: &CaptureConfig) -> Result<Self> {
        info!("Creating XDP capture on interface: {}", config.interface);

        // Get interface index first
        let ifindex = Self::get_ifindex(&config.interface)?;
        debug!("Interface {} has ifindex {}", config.interface, ifindex);

        // Create UMEM
        let umem_config = UmemConfig {
            frame_size: config.frame_size,
            frame_count: config.frame_count,
            fill_queue_size: 2048,
            comp_queue_size: 2048,
            headroom: 0,
            use_huge_pages: false,
        };
        let umem = Arc::new(Umem::new(&umem_config)?);
        info!(
            "UMEM created: {} frames × {} bytes = {} MB",
            umem.frame_count(),
            umem.frame_size(),
            umem.size() / 1024 / 1024
        );

        Ok(Self {
            config: config.clone(),
            umem,
            xsk_socket: None,
            bpf: None,
            stats: CaptureStats::default(),
            running: false,
            pending_fill_addrs: Vec::with_capacity(XSK_BATCH_SIZE),
            promisc_guard: None,
            ifindex,
        })
    }

    fn get_ifindex(ifname: &str) -> Result<i32> {
        let ifname_cstr = std::ffi::CString::new(ifname).context("Invalid interface name")?;

        let index = unsafe { libc::if_nametoindex(ifname_cstr.as_ptr()) };

        if index == 0 {
            let err = std::io::Error::last_os_error();
            bail!("Interface '{}' not found: {}", ifname, err);
        }

        Ok(index as i32)
    }

    fn get_xdp_bind_flags(&self) -> u16 {
        match self.config.mode {
            CaptureMode::Xdp => 0,           // Native mode, try zero-copy first
            CaptureMode::XdpSkb => XDP_COPY, // SKB mode, use copy
            CaptureMode::XdpOffload => 0,    // Offload mode
            _ => XDP_COPY,                   // Default to copy mode
        }
    }

    fn get_xdp_attach_flags(&self) -> XdpFlags {
        match self.config.mode {
            CaptureMode::Xdp => XdpFlags::DRV_MODE,
            CaptureMode::XdpSkb => XdpFlags::SKB_MODE,
            CaptureMode::XdpOffload => XdpFlags::HW_MODE,
            _ => XdpFlags::SKB_MODE,
        }
    }

    fn create_xsk_socket(&mut self) -> Result<()> {
        let socket_config = XskSocketConfig {
            rx_ring_size: 2048,
            tx_ring_size: 2048,
            fill_ring_size: 2048,
            comp_ring_size: 2048,
            bind_flags: self.get_xdp_bind_flags(),
            xdp_flags: 0,
        };

        let xsk_socket = XskSocket::new(
            &self.umem,
            self.ifindex as u32,
            self.config.queue_id,
            socket_config,
        )
        .context("Failed to create XSK socket")?;

        info!(
            "XSK socket created: fd={}, ifindex={}, queue={}",
            xsk_socket.fd(),
            self.ifindex,
            self.config.queue_id
        );

        self.xsk_socket = Some(xsk_socket);
        Ok(())
    }

    fn load_ebpf(&mut self) -> Result<()> {
        info!(
            "Loading eBPF program for interface: {}",
            self.config.interface
        );

        // Try multiple paths for the eBPF program
        let ebpf_paths = [
            "/usr/lib/probe-agent/xdp_redirect.o",
            "/usr/local/lib/probe-agent/xdp_redirect.o",
            "./target/bpfel-unknown-none/release/xdp_redirect",
            "./xdp_redirect.o",
        ];

        let mut bpf = None;
        let mut last_error = None;

        for path in &ebpf_paths {
            if std::path::Path::new(path).exists() {
                match Bpf::load_file(path) {
                    Ok(b) => {
                        info!("Loaded eBPF program from: {}", path);
                        bpf = Some(b);
                        break;
                    }
                    Err(e) => {
                        debug!("Failed to load eBPF from {}: {}", path, e);
                        last_error = Some(e);
                    }
                }
            }
        }

        let mut bpf = bpf.ok_or_else(|| {
            anyhow::anyhow!(
                "Failed to load eBPF program from any path. Last error: {:?}",
                last_error
            )
        })?;

        // Get and attach the XDP program
        let program: &mut Xdp = bpf
            .program_mut("xdp_redirect")
            .context("XDP program 'xdp_redirect' not found")?
            .try_into()
            .context("Program is not XDP type")?;

        program.load().context("Failed to load XDP program")?;

        let flags = self.get_xdp_attach_flags();

        match program.attach(&self.config.interface, flags) {
            Ok(link) => {
                // Keep the link alive by forgetting it (program stays attached)
                std::mem::forget(link);
                info!(
                    "XDP program attached to {} with {:?} mode",
                    self.config.interface, flags
                );
            }
            Err(e) => {
                if !flags.contains(XdpFlags::SKB_MODE) {
                    warn!("Failed to attach in DRV mode, trying SKB mode: {}", e);
                    let link = program
                        .attach(&self.config.interface, XdpFlags::SKB_MODE)
                        .context("Failed to attach XDP program in SKB mode")?;
                    std::mem::forget(link);
                    info!("XDP program attached in SKB mode (fallback)");
                } else {
                    return Err(e.into());
                }
            }
        }

        // Register XSK socket in the map
        if let Some(ref xsk_socket) = self.xsk_socket {
            let fd = xsk_socket.fd();
            if let Some(xsk_map) = bpf.map_mut("XSKS_MAP") {
                if let Ok(mut xsk_map) = XskMap::try_from(xsk_map) {
                    xsk_map.set(self.config.queue_id, fd, 0)?;
                    debug!(
                        "XSK socket registered in XSKS_MAP at index {}",
                        self.config.queue_id
                    );
                } else {
                    warn!("Failed to convert map to XskMap");
                }
            } else {
                warn!("XSKS_MAP not found in eBPF program");
            }
        }

        self.bpf = Some(bpf);

        info!("eBPF program loaded and attached successfully");
        Ok(())
    }

    fn refill_fill_queue(&mut self) {
        let xsk_socket = match self.xsk_socket.as_mut() {
            Some(s) => s,
            None => return,
        };

        self.pending_fill_addrs.clear();

        // Allocate frames and get their addresses
        while self.pending_fill_addrs.len() < XSK_BATCH_SIZE {
            if let Some(idx) = self.umem.alloc_frame() {
                let addr = self.umem.frame_addr_raw(idx) as u64;
                self.pending_fill_addrs.push(addr);
            } else {
                break;
            }
        }

        if self.pending_fill_addrs.is_empty() {
            return;
        }

        let filled = xsk_socket.fill_queue.fill(&self.pending_fill_addrs);

        // Free frames that couldn't be added to fill queue
        if filled < self.pending_fill_addrs.len() {
            for addr in &self.pending_fill_addrs[filled..] {
                let frame_idx = self.umem.addr_to_frame(*addr as usize);
                self.umem.free_frame(frame_idx);
            }
            trace!(
                "Fill queue partial: filled {}/{}, released {} frames",
                filled,
                self.pending_fill_addrs.len(),
                self.pending_fill_addrs.len() - filled
            );
        }
    }

    fn process_completions(&mut self) {
        let xsk_socket = match self.xsk_socket.as_mut() {
            Some(s) => s,
            None => return,
        };

        let mut addrs = [0u64; XSK_BATCH_SIZE];

        let completed = xsk_socket.comp_queue.complete(&mut addrs);

        if completed > 0 {
            for addr in &addrs[..completed] {
                let frame_idx = self.umem.addr_to_frame(*addr as usize);
                self.umem.free_frame(frame_idx);
            }
            trace!("Completed {} frames", completed);
        }
    }

    fn wait_for_data(&self, timeout_ms: i32) -> bool {
        if let Some(ref xsk_socket) = self.xsk_socket {
            match xsk_socket.poll(timeout_ms) {
                Ok(has_data) => has_data,
                Err(e) => {
                    error!("poll() failed: {}", e);
                    false
                }
            }
        } else {
            false
        }
    }
}

// SAFETY: XdpCapture wraps XskSocket which internally manages AF_XDP rings and UMEM.
// The underlying XSK socket fd and mmap'd memory are safe to access from any thread
// when accessed through the atomic ring operations (producer/consumer fences).
// The Arc<Umem> ensures shared memory lifetime management.
unsafe impl Send for XdpCapture {}
// SAFETY: All mutable state is protected by Arc or internal atomics.
// XskSocket operations use volatile reads/writes with proper memory ordering.
unsafe impl Sync for XdpCapture {}

#[async_trait::async_trait]
impl Capturer for XdpCapture {
    async fn start(&mut self) -> Result<()> {
        if self.running {
            return Ok(());
        }

        // Test helper: when PROBE_AGENT_FAKE_XDP is set, skip real AF_XDP/socket/eBPF
        // operations and mark the capturer as running so tests can exercise the
        // XDP code path without kernel/eBPF support.
        if env::var("PROBE_AGENT_FAKE_XDP").is_ok() {
            info!("Fake XDP test mode enabled; skipping real socket/eBPF attach");
            self.running = true;
            return Ok(());
        }

        info!(
            "Starting XDP capture on interface: {}",
            self.config.interface
        );

        // Enable promiscuous mode if configured
        if self.config.promiscuous_mode {
            match PromiscuousMode::enable(&self.config.interface) {
                Ok(promisc_guard) => {
                    info!(
                        "✓ Promiscuous mode enabled on interface: {}",
                        self.config.interface
                    );
                    self.promisc_guard = Some(promisc_guard);
                }
                Err(e) => {
                    warn!(
                        "Failed to enable promiscuous mode on {}: {}. \
                         Mirrored traffic may not be captured. \
                         Hint: Run with root or grant CAP_NET_ADMIN capability.",
                        self.config.interface, e
                    );
                }
            }
        } else {
            debug!(
                "Promiscuous mode disabled in config for interface: {}",
                self.config.interface
            );
        }

        // Create XSK socket with proper initialization
        self.create_xsk_socket()?;

        // Load and attach eBPF program
        self.load_ebpf()?;

        // Initial fill of the fill queue
        self.refill_fill_queue();

        self.running = true;
        info!("XDP capture started successfully");

        Ok(())
    }

    async fn stop(&mut self) -> Result<()> {
        if !self.running {
            return Ok(());
        }

        info!("Stopping XDP capture");

        // Log XDP statistics before stopping
        if let Some(ref xsk_socket) = self.xsk_socket {
            if let Ok(stats) = xsk_socket.statistics() {
                info!(
                    "XDP stats: rx_dropped={}, rx_invalid={}, tx_invalid={}, \
                     rx_ring_full={}, fill_empty={}, tx_empty={}",
                    stats.rx_dropped,
                    stats.rx_invalid_descs,
                    stats.tx_invalid_descs,
                    stats.rx_ring_full,
                    stats.rx_fill_ring_empty_descs,
                    stats.tx_ring_empty_descs
                );
            }
        }

        // Drop promiscuous mode guard (restores original state)
        if let Some(promisc_guard) = self.promisc_guard.take() {
            drop(promisc_guard);
            info!(
                "✓ Promiscuous mode restored on interface: {}",
                self.config.interface
            );
        }

        // Drop eBPF (detaches program)
        self.bpf = None;

        // Drop XSK socket (closes fd and unmaps rings)
        self.xsk_socket = None;

        self.running = false;
        info!("XDP capture stopped");

        Ok(())
    }

    fn poll(&mut self) -> Result<Option<PacketBatch>> {
        if !self.running {
            return Ok(None);
        }
        if env::var("PROBE_AGENT_FAKE_XDP").is_ok() {
            return Ok(None);
        }
        if self.xsk_socket.is_none() {
            return Ok(None);
        }
        // ① Process completions first – no outstanding borrow
        self.process_completions();
        // ② Check wakeup flag (short immutable borrow, drops at block end)
        if let Some(xsk_socket) = self.xsk_socket.as_ref() {
            if xsk_socket.rx_needs_wakeup() {
                if let Err(e) = xsk_socket.wakeup() {
                    warn!("Failed to wakeup kernel: {}", e);
                }
            }
        }
        // ③ Check rx availability (short immutable borrow)
        let rx_available = self
            .xsk_socket
            .as_ref()
            .map(|s| s.rx_queue.available())
            .unwrap_or(0);
        if rx_available == 0 {
            if !self.wait_for_data(1) {
                self.refill_fill_queue();
                return Ok(None);
            }
        }
        // ④ Receive descriptors — scoped mutable borrow
        let mut descs = [XdpDesc {
            addr: 0,
            len: 0,
            options: 0,
        }; XSK_BATCH_SIZE];
        let received = {
            let xsk_socket = match self.xsk_socket.as_mut() {
                Some(s) => s,
                None => return Ok(None),
            };
            xsk_socket.rx_queue.receive(&mut descs[..])
        }; // ← mutable borrow ends here
        if received == 0 {
            self.refill_fill_queue();
            return Ok(None);
        }
        let timestamp = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_micros() as u64;
        let mut frames = Vec::with_capacity(received);
        for desc in &descs[..received] {
            let frame_idx = self.umem.addr_to_frame(desc.addr as usize);
            let offset = (desc.addr as usize) % self.umem.frame_size();
            frames.push(FrameInfo {
                idx: frame_idx,
                offset: offset as u32,
                len: desc.len,
                timestamp,
            });
            self.stats.packets_received += 1;
            self.stats.bytes_received += desc.len as u64;
            metrics::PACKETS_CAPTURED.inc();
            metrics::BYTES_CAPTURED.inc_by(desc.len as f64);
        }
        // ⑤ Refill — no outstanding borrow
        self.refill_fill_queue();
        Ok(Some(PacketBatch::new(self.umem.clone(), frames)))
    }

    fn stats(&self) -> CaptureStats {
        self.stats.clone()
    }
}

impl Drop for XdpCapture {
    fn drop(&mut self) {
        // Resources are cleaned up via Option::take() and Drop impls
        // XskSocket, Bpf, and PromiscuousMode all have proper Drop implementations
    }
}
