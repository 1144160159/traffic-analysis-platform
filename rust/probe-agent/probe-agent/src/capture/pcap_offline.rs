use anyhow::{Context, Result};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tracing::{debug, info};

use super::{CaptureStats, Capturer, PacketBatch};
use crate::config::CaptureConfig;

/// PCAP file header (24 bytes)
const PCAP_GLOBAL_HEADER_SIZE: usize = 24;
/// PCAP packet header (16 bytes)
const PCAP_PACKET_HEADER_SIZE: usize = 16;
/// PCAP magic number (little-endian)
const PCAP_MAGIC: u32 = 0xa1b2c3d4;
/// PCAP nanosecond magic number
const PCAP_NS_MAGIC: u32 = 0xa1b23c4d;

#[derive(Debug, Clone, Copy)]
struct PcapGlobalHeader {
    magic: u32,
    _version_major: u16,
    _version_minor: u16,
    _thiszone: i32,
    _sigfigs: u32,
    snaplen: u32,
    network: u32,
    is_nanosecond: bool,
}

#[derive(Debug, Clone, Copy)]
struct PcapPacketHeader {
    ts_sec: u32,
    ts_usec: u32,
    incl_len: u32,
    orig_len: u32,
}

pub struct PcapReader {
    data: Vec<u8>,
    offset: usize,
    global_header: PcapGlobalHeader,
}

impl PcapReader {
    pub fn from_file(path: &Path) -> Result<Self> {
        let data = std::fs::read(path).context(format!("Failed to read pcap file: {:?}", path))?;

        if data.len() < PCAP_GLOBAL_HEADER_SIZE {
            anyhow::bail!("File too small to be a valid pcap: {:?}", path);
        }

        let magic = u32::from_le_bytes([data[0], data[1], data[2], data[3]]);
        let is_nanosecond = magic == PCAP_NS_MAGIC;
        let is_swapped_magic =
            magic.swap_bytes() == PCAP_MAGIC || magic.swap_bytes() == PCAP_NS_MAGIC;

        if magic != PCAP_MAGIC && magic != PCAP_NS_MAGIC && !is_swapped_magic {
            anyhow::bail!("Invalid pcap magic number: 0x{:08x} in {:?}", magic, path);
        }

        let need_swap = is_swapped_magic;
        let mut offset = 4;

        let version_major = read_u16(&data, &mut offset, need_swap);
        let version_minor = read_u16(&data, &mut offset, need_swap);
        let thiszone = read_i32(&data, &mut offset, need_swap);
        let sigfigs = read_u32(&data, &mut offset, need_swap);
        let snaplen = read_u32(&data, &mut offset, need_swap);
        let network = read_u32(&data, &mut offset, need_swap);

        let global_header = PcapGlobalHeader {
            magic,
            _version_major: version_major,
            _version_minor: version_minor,
            _thiszone: thiszone,
            _sigfigs: sigfigs,
            snaplen,
            network,
            is_nanosecond,
        };

        Ok(Self {
            data,
            offset: PCAP_GLOBAL_HEADER_SIZE,
            global_header,
        })
    }

    pub fn has_next(&self) -> bool {
        self.offset + PCAP_PACKET_HEADER_SIZE <= self.data.len()
    }

    pub fn next_packet(&mut self) -> Option<(Vec<u8>, u64)> {
        if !self.has_next() {
            return None;
        }

        let offset = self.offset;
        let ts_sec = u32::from_le_bytes([
            self.data[offset],
            self.data[offset + 1],
            self.data[offset + 2],
            self.data[offset + 3],
        ]);
        let ts_usec = u32::from_le_bytes([
            self.data[offset + 4],
            self.data[offset + 5],
            self.data[offset + 6],
            self.data[offset + 7],
        ]);
        let incl_len = u32::from_le_bytes([
            self.data[offset + 8],
            self.data[offset + 9],
            self.data[offset + 10],
            self.data[offset + 11],
        ]) as usize;
        let _orig_len = u32::from_le_bytes([
            self.data[offset + 12],
            self.data[offset + 13],
            self.data[offset + 14],
            self.data[offset + 15],
        ]);

        self.offset += PCAP_PACKET_HEADER_SIZE;

        if self.offset + incl_len > self.data.len() {
            debug!(
                "Packet truncated: incl_len={}, remaining={}",
                incl_len,
                self.data.len() - self.offset
            );
            return None;
        }

        let packet_data = self.data[self.offset..self.offset + incl_len].to_vec();
        self.offset += incl_len;

        let timestamp = if self.global_header.is_nanosecond {
            ts_sec as u64 * 1_000_000_000 + ts_usec as u64
        } else {
            ts_sec as u64 * 1_000_000 + ts_usec as u64
        };

        Some((packet_data, timestamp))
    }
}

fn read_u16(data: &[u8], offset: &mut usize, swap: bool) -> u16 {
    let val = u16::from_le_bytes([data[*offset], data[*offset + 1]]);
    *offset += 2;
    if swap {
        val.swap_bytes()
    } else {
        val
    }
}

fn read_u32(data: &[u8], offset: &mut usize, swap: bool) -> u32 {
    let val = u32::from_le_bytes([
        data[*offset],
        data[*offset + 1],
        data[*offset + 2],
        data[*offset + 3],
    ]);
    *offset += 4;
    if swap {
        val.swap_bytes()
    } else {
        val
    }
}

fn read_i32(data: &[u8], offset: &mut usize, swap: bool) -> i32 {
    read_u32(data, offset, swap) as i32
}

/// Rate control mode for PCAP replay
#[derive(Debug, Clone, Copy)]
pub enum ReplaySpeed {
    /// Replay at original capture speed
    Original,
    /// Replay at specified multiplier (2.0 = double speed)
    Multiplier(f64),
    /// Replay as fast as possible (no rate limiting)
    MaxSpeed,
}

impl ReplaySpeed {
    pub fn from_str(s: &str) -> Result<Self> {
        match s.to_lowercase().as_str() {
            "original" => Ok(Self::Original),
            "max" | "top" => Ok(Self::MaxSpeed),
            other => {
                if let Some(num) = other.strip_suffix('x') {
                    let mult: f64 = num.parse().context("Invalid speed multiplier")?;
                    Ok(Self::Multiplier(mult))
                } else {
                    Ok(Self::Original)
                }
            }
        }
    }
}

/// PCAP offline replayer - reads pcap files and replays them through the capture pipeline
pub struct PcapReplayer {
    pcap_files: Vec<PathBuf>,
    current_file_idx: usize,
    current_reader: Option<PcapReader>,
    speed: ReplaySpeed,
    started: bool,
    stopped: Arc<AtomicBool>,
    stats: CaptureStats,
    loop_replay: bool,
    start_time: Option<Instant>,
    first_packet_ts: Option<u64>,
    packets_sent: u64,
    snaplen: u32,
    network: u32,
}

impl PcapReplayer {
    /// Create from a config, reading pcap files from the configured pcap directory
    pub fn from_config(config: &CaptureConfig) -> Result<Self> {
        let pcap_dir = config.pcap_dir.as_deref().unwrap_or("./pcap");
        let speed = ReplaySpeed::from_str(config.replay_speed.as_deref().unwrap_or("original"))
            .unwrap_or(ReplaySpeed::Original);

        let loop_replay = config.loop_replay.unwrap_or(false);

        Self::new(pcap_dir, speed, loop_replay)
    }

    pub fn new(pcap_path: &str, speed: ReplaySpeed, loop_replay: bool) -> Result<Self> {
        let path = Path::new(pcap_path);
        let mut pcap_files = Vec::new();

        if path.is_dir() {
            let mut entries: Vec<_> = std::fs::read_dir(path)
                .context(format!("Failed to read pcap directory: {:?}", path))?
                .filter_map(|e| e.ok())
                .filter(|e| {
                    let name = e.file_name().to_string_lossy().to_lowercase();
                    name.ends_with(".pcap") || name.ends_with(".pcapng") || name.ends_with(".cap")
                })
                .map(|e| e.path())
                .collect();
            entries.sort();
            pcap_files = entries;
        } else if path.is_file() {
            pcap_files.push(path.to_path_buf());
        } else {
            anyhow::bail!("PCAP path does not exist: {:?}", path);
        }

        if pcap_files.is_empty() {
            anyhow::bail!("No pcap files found in: {:?}", path);
        }

        info!(
            "PCAP replayer: {} files, speed={:?}, loop={}",
            pcap_files.len(),
            speed,
            loop_replay
        );

        Ok(Self {
            pcap_files,
            current_file_idx: 0,
            current_reader: None,
            speed,
            started: false,
            stopped: Arc::new(AtomicBool::new(false)),
            stats: CaptureStats::default(),
            loop_replay,
            start_time: None,
            first_packet_ts: None,
            packets_sent: 0,
            snaplen: 65535,
            network: 1,
        })
    }

    fn open_next_file(&mut self) -> Result<()> {
        if self.current_file_idx >= self.pcap_files.len() {
            if self.loop_replay {
                info!("PCAP replay loop: restarting from first file");
                self.current_file_idx = 0;
                self.first_packet_ts = None;
                self.start_time = Some(Instant::now());
            } else {
                anyhow::bail!("All pcap files have been replayed");
            }
        }

        let path = &self.pcap_files[self.current_file_idx];
        debug!("Opening pcap file: {:?}", path);

        let reader = PcapReader::from_file(path)?;
        self.snaplen = reader.global_header.snaplen;
        self.network = reader.global_header.network;

        info!(
            "Opened pcap file {}: {:?} (snaplen={}, network={})",
            self.current_file_idx + 1,
            path.file_name().unwrap_or_default(),
            self.snaplen,
            self.network
        );

        self.current_reader = Some(reader);
        self.current_file_idx += 1;
        Ok(())
    }
}

/// Rate-limit packet replay to match original or scaled speed.
fn rate_limit_packet(
    speed: ReplaySpeed,
    start_time: Option<Instant>,
    first_ts: Option<u64>,
    packet_ts: u64,
) {
    match speed {
        ReplaySpeed::MaxSpeed => return,
        ReplaySpeed::Original => {
            if let (Some(start), Some(first)) = (start_time, first_ts) {
                let elapsed_real = start.elapsed();
                let elapsed_pcap = Duration::from_nanos(packet_ts.saturating_sub(first));
                if elapsed_pcap > elapsed_real {
                    let sleep_time = elapsed_pcap - elapsed_real;
                    if sleep_time > Duration::from_micros(100) {
                        std::thread::sleep(sleep_time);
                    }
                }
            }
        }
        ReplaySpeed::Multiplier(m) => {
            if let (Some(start), Some(first)) = (start_time, first_ts) {
                let elapsed_real = start.elapsed();
                let elapsed_pcap =
                    Duration::from_nanos((packet_ts.saturating_sub(first) as f64 / m) as u64);
                if elapsed_pcap > elapsed_real {
                    let sleep_time = elapsed_pcap - elapsed_real;
                    if sleep_time > Duration::from_micros(100) {
                        std::thread::sleep(sleep_time);
                    }
                }
            }
        }
    }
}

#[async_trait::async_trait]
impl Capturer for PcapReplayer {
    async fn start(&mut self) -> Result<()> {
        info!(
            "Starting PCAP replayer: {} files, speed={:?}",
            self.pcap_files.len(),
            self.speed
        );

        self.open_next_file()?;
        self.started = true;
        self.start_time = Some(Instant::now());
        self.stopped.store(false, Ordering::SeqCst);

        Ok(())
    }

    async fn stop(&mut self) -> Result<()> {
        info!(
            "Stopping PCAP replayer ({} packets sent)",
            self.packets_sent
        );
        self.stopped.store(true, Ordering::SeqCst);
        self.started = false;
        Ok(())
    }

    fn poll(&mut self) -> Result<Option<PacketBatch>> {
        if self.stopped.load(Ordering::SeqCst) || !self.started {
            return Ok(None);
        }

        // Check if we need to open the next file
        if self.current_reader.is_none() || !self.current_reader.as_ref().unwrap().has_next() {
            if self.current_reader.is_some() {
                info!(
                    "Finished pcap file {}/{}",
                    self.current_file_idx,
                    self.pcap_files.len()
                );
            }

            match self.open_next_file() {
                Ok(()) => {}
                Err(_) => {
                    info!(
                        "PCAP replay complete: {} packets replayed",
                        self.packets_sent
                    );
                    return Ok(None);
                }
            }
        }

        // Read a batch of packets (up to 64 per poll for efficiency)
        let mut packets = Vec::new();
        let reader = self.current_reader.as_mut().unwrap();
        let speed = self.speed;
        let start_time = self.start_time;
        let first_ts = self.first_packet_ts;

        for _ in 0..64 {
            if let Some((data, ts)) = reader.next_packet() {
                if first_ts.is_none() {
                    self.first_packet_ts = Some(ts);
                    self.start_time = Some(Instant::now());
                }

                rate_limit_packet(speed, start_time, first_ts, ts);
                packets.push((data, ts));
            } else {
                break;
            }
        }
        let _ = reader; // explicitly drop reader to release resources

        if packets.is_empty() {
            return Ok(None);
        }

        self.packets_sent += packets.len() as u64;
        self.stats.packets_received += packets.len() as u64;

        let total_bytes: u64 = packets.iter().map(|(d, _)| d.len() as u64).sum();
        self.stats.bytes_received += total_bytes;

        // Convert to owned packet data - the processing pipeline expects owned data
        let batch = create_owned_batch(packets);

        Ok(Some(batch))
    }

    fn stats(&self) -> CaptureStats {
        self.stats.clone()
    }
}

fn create_owned_batch(packets: Vec<(Vec<u8>, u64)>) -> PacketBatch {
    PacketBatch::from_owned_packets(packets)
}
