use std::fs::OpenOptions;
use std::io::Write;
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, AtomicU8, AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use parking_lot::Mutex;
use tracing::{debug, error, info, trace, warn};

struct Buffer {
    data: Vec<u8>,
    pos: AtomicUsize,
    packet_count: AtomicU64,
    ts_start: AtomicU64,
    ts_end: AtomicU64,
    created_at: Mutex<Instant>,
}

impl Buffer {
    fn new(capacity: usize) -> Self {
        Self {
            data: vec![0u8; capacity],
            pos: AtomicUsize::new(0),
            packet_count: AtomicU64::new(0),
            ts_start: AtomicU64::new(u64::MAX),
            ts_end: AtomicU64::new(0),
            created_at: Mutex::new(Instant::now()),
        }
    }

    fn reset(&self) {
        self.pos.store(0, Ordering::Release);
        self.packet_count.store(0, Ordering::Release);
        self.ts_start.store(u64::MAX, Ordering::Release);
        self.ts_end.store(0, Ordering::Release);
        *self.created_at.lock() = Instant::now();
    }

    fn write_packet(&self, timestamp: u64, data: &[u8]) -> bool {
        const HEADER_SIZE: usize = 16;
        let total_size = HEADER_SIZE + data.len();
        let old_pos = self.pos.fetch_add(total_size, Ordering::AcqRel);
        let new_pos = old_pos + total_size;

        if new_pos > self.data.len() {
            self.pos.fetch_sub(total_size, Ordering::AcqRel);
            return false;
        }

        let ts_sec = (timestamp / 1_000_000) as u32;
        let ts_usec = (timestamp % 1_000_000) as u32;
        let incl_len = data.len() as u32;
        let orig_len = data.len() as u32;

        let mut header = [0u8; HEADER_SIZE];
        header[0..4].copy_from_slice(&ts_sec.to_le_bytes());
        header[4..8].copy_from_slice(&ts_usec.to_le_bytes());
        header[8..12].copy_from_slice(&incl_len.to_le_bytes());
        header[12..16].copy_from_slice(&orig_len.to_le_bytes());

        unsafe {
            let base = self.data.as_ptr().add(old_pos) as *mut u8;
            std::ptr::copy_nonoverlapping(header.as_ptr(), base, HEADER_SIZE);
            std::ptr::copy_nonoverlapping(data.as_ptr(), base.add(HEADER_SIZE), data.len());
        }

        let new_count = self.packet_count.fetch_add(1, Ordering::Relaxed) + 1;
        let min_ts = if timestamp == 0 { 1 } else { timestamp };
        self.ts_start.fetch_min(min_ts, Ordering::Relaxed);
        self.ts_end.fetch_max(min_ts, Ordering::Relaxed);

        if new_count % 1000 == 0 {
            info!(
                "PCAP write progress: {} packets written to buffer, {} bytes used",
                new_count, new_pos
            );
        }

        true
    }

    fn pos(&self) -> usize {
        self.pos.load(Ordering::Acquire)
    }

    fn packet_count(&self) -> u64 {
        self.packet_count.load(Ordering::Relaxed)
    }

    fn ts_range(&self) -> (u64, u64) {
        (
            self.ts_start.load(Ordering::Relaxed),
            self.ts_end.load(Ordering::Relaxed),
        )
    }

    fn duration(&self) -> Duration {
        self.created_at.lock().elapsed()
    }

    fn should_rotate(&self, max_duration: Duration, max_packets: u64) -> bool {
        if self.packet_count() == 0 {
            return false;
        }
        if self.duration() >= max_duration {
            return true;
        }
        if self.packet_count() >= max_packets {
            return true;
        }
        false
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WriteResult {
    Ok,
    Rotated,
    Blocked,
    Fallback,
    Error,
}

#[derive(Debug)]
pub struct UploadData {
    pub data: Vec<u8>,
    pub ts_start: u64,
    pub ts_end: u64,
    pub packet_count: u64,
}

#[derive(Clone, Debug)]
pub struct TripleBufferConfig {
    pub buffer_size: usize,
    pub max_duration: Duration,
    pub max_packets: u64,
    pub enable_fallback: bool,
    pub fallback_path: String,
    pub max_retries: usize,
    pub retry_delay: Duration,
}

impl Default for TripleBufferConfig {
    fn default() -> Self {
        Self {
            buffer_size: 256 * 1024 * 1024,
            max_duration: Duration::from_secs(60),
            max_packets: 100_000,
            enable_fallback: true,
            fallback_path: "/tmp/pcap_overflow".to_string(),
            max_retries: 3,
            retry_delay: Duration::from_millis(10),
        }
    }
}

pub struct TripleBuffer {
    buffers: [Buffer; 3],
    write_idx: AtomicU8,
    upload_idx: AtomicU8,
    config: TripleBufferConfig,
    stats: BufferStats,
    rotate_lock: tokio::sync::Mutex<()>,
}

#[derive(Debug, Default)]
pub struct BufferStats {
    rotations: AtomicU64,
    packets_written: AtomicU64,
    bytes_written: AtomicU64,
    blocks: AtomicU64,
    retries: AtomicU64,
    fallbacks: AtomicU64,
}

impl BufferStats {
    pub fn rotations(&self) -> u64 {
        self.rotations.load(Ordering::Relaxed)
    }

    pub fn packets_written(&self) -> u64 {
        self.packets_written.load(Ordering::Relaxed)
    }

    pub fn bytes_written(&self) -> u64 {
        self.bytes_written.load(Ordering::Relaxed)
    }

    pub fn blocks(&self) -> u64 {
        self.blocks.load(Ordering::Relaxed)
    }

    pub fn retries(&self) -> u64 {
        self.retries.load(Ordering::Relaxed)
    }

    pub fn fallbacks(&self) -> u64 {
        self.fallbacks.load(Ordering::Relaxed)
    }
}

impl TripleBuffer {
    pub fn new(config: TripleBufferConfig) -> Self {
        let buffers = [
            Buffer::new(config.buffer_size),
            Buffer::new(config.buffer_size),
            Buffer::new(config.buffer_size),
        ];

        if config.enable_fallback {
            std::fs::create_dir_all(&config.fallback_path).ok();
            debug!("Fallback directory created: {}", config.fallback_path);
        }

        Self {
            buffers,
            write_idx: AtomicU8::new(0),
            upload_idx: AtomicU8::new(255),
            config,
            stats: BufferStats::default(),
            rotate_lock: tokio::sync::Mutex::new(()),
        }
    }

    pub fn write_packet(&self, timestamp: u64, data: &[u8]) -> WriteResult {
        for retry in 0..self.config.max_retries {
            if retry > 0 {
                self.stats.retries.fetch_add(1, Ordering::Relaxed);
                std::thread::sleep(self.config.retry_delay);
                trace!("Retry {} for PCAP write", retry);
            }

            let write_idx = self.write_idx.load(Ordering::Acquire) as usize;
            let buffer = &self.buffers[write_idx];

            if buffer.write_packet(timestamp, data) {
                self.stats.packets_written.fetch_add(1, Ordering::Relaxed);
                self.stats
                    .bytes_written
                    .fetch_add(data.len() as u64, Ordering::Relaxed);

                if buffer.should_rotate(self.config.max_duration, self.config.max_packets) {
                    if let Ok(_guard) = self.rotate_lock.try_lock() {
                        if self.try_rotate_inner() {
                            return WriteResult::Rotated;
                        }
                    }
                }

                return WriteResult::Ok;
            }

            if let Ok(_guard) = self.rotate_lock.try_lock() {
                if self.try_rotate_inner() {
                    let new_write_idx = self.write_idx.load(Ordering::Acquire) as usize;
                    let new_buffer = &self.buffers[new_write_idx];

                    if new_buffer.write_packet(timestamp, data) {
                        self.stats.packets_written.fetch_add(1, Ordering::Relaxed);
                        self.stats
                            .bytes_written
                            .fetch_add(data.len() as u64, Ordering::Relaxed);
                        return WriteResult::Rotated;
                    }
                }
            }
        }

        if self.config.enable_fallback {
            match self.fallback_to_disk(timestamp, data) {
                Ok(()) => {
                    self.stats.fallbacks.fetch_add(1, Ordering::Relaxed);
                    debug!("Packet written to fallback disk cache");
                    return WriteResult::Fallback;
                }
                Err(e) => {
                    error!("Fallback to disk failed: {}", e);
                }
            }
        }

        self.stats.blocks.fetch_add(1, Ordering::Relaxed);
        warn!(
            "PCAP write blocked after {} retries",
            self.config.max_retries
        );
        WriteResult::Blocked
    }

    fn fallback_to_disk(&self, timestamp: u64, data: &[u8]) -> std::io::Result<()> {
        let ts_sec = timestamp / 1_000_000;
        let filename = format!("pcap_overflow_{}.pcap", ts_sec);
        let mut path = PathBuf::from(&self.config.fallback_path);
        path.push(&filename);

        let mut file = OpenOptions::new().create(true).append(true).open(&path)?;
        let file_len = file.metadata()?.len();

        if file_len == 0 {
            let global_header = self.make_pcap_global_header();
            file.write_all(&global_header)?;
        }

        let packet_header = self.make_pcap_packet_header(timestamp, data.len());
        file.write_all(&packet_header)?;
        file.write_all(data)?;
        file.sync_all()?;

        Ok(())
    }

    fn make_pcap_global_header(&self) -> Vec<u8> {
        let mut header = Vec::with_capacity(24);
        header.extend_from_slice(&0xa1b2c3d4u32.to_le_bytes());
        header.extend_from_slice(&2u16.to_le_bytes());
        header.extend_from_slice(&4u16.to_le_bytes());
        header.extend_from_slice(&0i32.to_le_bytes());
        header.extend_from_slice(&0u32.to_le_bytes());
        header.extend_from_slice(&65535u32.to_le_bytes());
        header.extend_from_slice(&1u32.to_le_bytes());
        header
    }

    fn make_pcap_packet_header(&self, timestamp: u64, len: usize) -> Vec<u8> {
        let mut header = Vec::with_capacity(16);
        let ts_sec = (timestamp / 1_000_000) as u32;
        let ts_usec = (timestamp % 1_000_000) as u32;
        let incl_len = len as u32;
        let orig_len = len as u32;
        header.extend_from_slice(&ts_sec.to_le_bytes());
        header.extend_from_slice(&ts_usec.to_le_bytes());
        header.extend_from_slice(&incl_len.to_le_bytes());
        header.extend_from_slice(&orig_len.to_le_bytes());
        header
    }

    pub fn try_rotate(&self) -> bool {
        if let Ok(_guard) = self.rotate_lock.try_lock() {
            self.try_rotate_inner()
        } else {
            false
        }
    }

    fn try_rotate_inner(&self) -> bool {
        let write_idx = self.write_idx.load(Ordering::Acquire);
        let upload_idx = self.upload_idx.load(Ordering::Acquire);
        let ready_idx = (write_idx + 1) % 3;

        if ready_idx == upload_idx {
            trace!("Rotate blocked: ready_idx={} is uploading", ready_idx);
            return false;
        }

        self.write_idx.store(ready_idx, Ordering::Release);
        self.buffers[ready_idx as usize].reset();
        self.stats.rotations.fetch_add(1, Ordering::Relaxed);

        debug!(
            "Buffer rotated: {} -> {}, packets={}, upload_idx={}",
            write_idx,
            ready_idx,
            self.buffers[write_idx as usize].packet_count(),
            upload_idx
        );

        true
    }

    pub fn force_rotate(&self) -> bool {
        let write_idx = self.write_idx.load(Ordering::Acquire) as usize;
        let packet_count = self.buffers[write_idx].packet_count();

        if packet_count == 0 {
            return false;
        }

        self.try_rotate()
    }

    pub async fn wait_for_upload(&self) -> Option<UploadData> {
        let write_idx = self.write_idx.load(Ordering::Acquire);
        let upload_idx = self.upload_idx.load(Ordering::Acquire);
        let ready_idx = if write_idx == 0 { 2 } else { write_idx - 1 };

        if upload_idx != 255 {
            return None;
        }

        let buffer = &self.buffers[ready_idx as usize];
        if buffer.packet_count() == 0 {
            return None;
        }

        match self
            .upload_idx
            .compare_exchange(255, ready_idx, Ordering::AcqRel, Ordering::Acquire)
        {
            Ok(_) => {
                let pos = buffer.pos();
                let (mut ts_start, ts_end) = buffer.ts_range();
                if ts_start == u64::MAX {
                    ts_start = ts_end;
                }
                let packet_count = buffer.packet_count();
                let mut data = vec![0u8; pos];
                data.copy_from_slice(&buffer.data[..pos]);

                debug!(
                    "Upload task ready: buffer={}, packets={}, bytes={}",
                    ready_idx, packet_count, pos
                );

                Some(UploadData {
                    data,
                    ts_start,
                    ts_end,
                    packet_count,
                })
            }
            Err(_) => None,
        }
    }

    pub fn complete_upload(&self, buffer_idx: u8) {
        let current = self.upload_idx.load(Ordering::Acquire);
        if current != buffer_idx {
            warn!(
                "complete_upload mismatch: expected {}, got {}",
                current, buffer_idx
            );
            return;
        }

        self.buffers[buffer_idx as usize].reset();
        self.upload_idx.store(255, Ordering::Release);
        trace!("Upload completed: buffer={}", buffer_idx);
    }

    pub fn find_uploading_buffer(&self) -> Option<u8> {
        let idx = self.upload_idx.load(Ordering::Acquire);
        if idx == 255 {
            None
        } else {
            Some(idx)
        }
    }

    pub fn stats(&self) -> &BufferStats {
        &self.stats
    }

    pub fn write_buffer_utilization(&self) -> f64 {
        let write_idx = self.write_idx.load(Ordering::Acquire) as usize;
        let pos = self.buffers[write_idx].pos();
        pos as f64 / self.config.buffer_size as f64
    }

    pub fn get_fallback_files(&self) -> std::io::Result<Vec<PathBuf>> {
        let mut files = Vec::new();
        let path = PathBuf::from(&self.config.fallback_path);

        if !path.exists() {
            return Ok(files);
        }

        for entry in std::fs::read_dir(path)? {
            let entry = entry?;
            let file_path = entry.path();
            if file_path.is_file() {
                if let Some(filename) = file_path.file_name() {
                    if filename.to_string_lossy().starts_with("pcap_overflow_") {
                        files.push(file_path);
                    }
                }
            }
        }

        files.sort();
        Ok(files)
    }

    pub fn cleanup_fallback_file(&self, path: &PathBuf) -> std::io::Result<()> {
        std::fs::remove_file(path)?;
        debug!("Cleaned up fallback file: {:?}", path);
        Ok(())
    }
}

trait AtomicMinMax {
    fn fetch_min(&self, val: u64, order: Ordering);
    fn fetch_max(&self, val: u64, order: Ordering);
}

impl AtomicMinMax for AtomicU64 {
    fn fetch_min(&self, val: u64, order: Ordering) {
        let mut current = self.load(Ordering::Acquire);
        loop {
            if current <= val {
                break;
            }
            match self.compare_exchange_weak(current, val, order, Ordering::Acquire) {
                Ok(_) => break,
                Err(c) => current = c,
            }
        }
    }

    fn fetch_max(&self, val: u64, order: Ordering) {
        let mut current = self.load(Ordering::Acquire);
        loop {
            if current >= val {
                break;
            }
            match self.compare_exchange_weak(current, val, order, Ordering::Acquire) {
                Ok(_) => break,
                Err(c) => current = c,
            }
        }
    }
}
