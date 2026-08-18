use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::collections::HashSet;
use std::fmt::Write as _;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use thiserror::Error;
use tracing::{debug, info};

use super::{
    CaptureStats, CaptureTimestamp, CaptureTimestampError, Capturer, PacketBatch,
    TimestampPrecision,
};
use crate::config::CaptureConfig;

/// PCAP file header (24 bytes)
const PCAP_GLOBAL_HEADER_SIZE: usize = 24;
/// PCAP packet header (16 bytes)
const PCAP_PACKET_HEADER_SIZE: usize = 16;
const ETHERNET_LINK_TYPE: u32 = 1;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PcapByteOrder {
    LittleEndian,
    BigEndian,
}

#[derive(Debug, Clone, Copy)]
struct PcapGlobalHeader {
    version_major: u16,
    version_minor: u16,
    _thiszone: i32,
    _sigfigs: u32,
    snaplen: u32,
    network: u32,
    byte_order: PcapByteOrder,
    timestamp_precision: TimestampPrecision,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PcapRecord {
    pub bytes: Vec<u8>,
    pub captured_at: CaptureTimestamp,
    pub captured_len: u32,
    pub original_len: u32,
}

#[derive(Debug, Error, PartialEq, Eq)]
pub enum PcapReadError {
    #[error("PCAP_TRUNCATED_PACKET_HEADER remaining={remaining}")]
    TruncatedPacketHeader { remaining: usize },
    #[error(
        "PCAP_INVALID_CAPTURED_LENGTH captured={captured} original={original} snaplen={snaplen}"
    )]
    InvalidCapturedLength {
        captured: u32,
        original: u32,
        snaplen: u32,
    },
    #[error("PCAP_TRUNCATED_PACKET_PAYLOAD expected={expected} remaining={remaining}")]
    TruncatedPacketPayload { expected: usize, remaining: usize },
    #[error(transparent)]
    InvalidTimestamp(#[from] CaptureTimestampError),
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

        let (byte_order, timestamp_precision) = match data[..4] {
            [0xd4, 0xc3, 0xb2, 0xa1] => {
                (PcapByteOrder::LittleEndian, TimestampPrecision::Microsecond)
            }
            [0xa1, 0xb2, 0xc3, 0xd4] => (PcapByteOrder::BigEndian, TimestampPrecision::Microsecond),
            [0x4d, 0x3c, 0xb2, 0xa1] => {
                (PcapByteOrder::LittleEndian, TimestampPrecision::Nanosecond)
            }
            [0xa1, 0xb2, 0x3c, 0x4d] => (PcapByteOrder::BigEndian, TimestampPrecision::Nanosecond),
            _ => anyhow::bail!("PCAP_UNSUPPORTED_MAGIC path={}", path.display()),
        };
        let mut offset = 4;

        let version_major = read_u16(&data, &mut offset, byte_order);
        let version_minor = read_u16(&data, &mut offset, byte_order);
        let thiszone = read_i32(&data, &mut offset, byte_order);
        let sigfigs = read_u32(&data, &mut offset, byte_order);
        let snaplen = read_u32(&data, &mut offset, byte_order);
        let network = read_u32(&data, &mut offset, byte_order);
        if (version_major, version_minor) != (2, 4) {
            anyhow::bail!(
                "PCAP_UNSUPPORTED_VERSION version={}.{} path={}",
                version_major,
                version_minor,
                path.display()
            );
        }
        if snaplen == 0 {
            anyhow::bail!("PCAP_INVALID_SNAPLEN path={}", path.display());
        }
        if network != ETHERNET_LINK_TYPE {
            anyhow::bail!(
                "PCAP_UNSUPPORTED_LINK_TYPE link_type={} path={}",
                network,
                path.display()
            );
        }

        let global_header = PcapGlobalHeader {
            version_major,
            version_minor,
            _thiszone: thiszone,
            _sigfigs: sigfigs,
            snaplen,
            network,
            byte_order,
            timestamp_precision,
        };

        Ok(Self {
            data,
            offset: PCAP_GLOBAL_HEADER_SIZE,
            global_header,
        })
    }

    pub fn has_next(&self) -> bool {
        self.offset < self.data.len()
    }

    pub fn next_packet(&mut self) -> Option<(Vec<u8>, u64)> {
        let record = self.next_packet_checked().ok().flatten()?;
        Some((record.bytes, record.captured_at.epoch_micros()))
    }

    pub fn next_packet_checked(
        &mut self,
    ) -> std::result::Result<Option<PcapRecord>, PcapReadError> {
        let remaining = self.data.len().saturating_sub(self.offset);
        if remaining == 0 {
            return Ok(None);
        }
        if remaining < PCAP_PACKET_HEADER_SIZE {
            return Err(PcapReadError::TruncatedPacketHeader { remaining });
        }

        let mut cursor = self.offset;
        let ts_sec = read_u32(&self.data, &mut cursor, self.global_header.byte_order);
        let ts_subsecond = read_u32(&self.data, &mut cursor, self.global_header.byte_order);
        let captured_len = read_u32(&self.data, &mut cursor, self.global_header.byte_order);
        let original_len = read_u32(&self.data, &mut cursor, self.global_header.byte_order);
        if captured_len == 0
            || captured_len > original_len
            || captured_len > self.global_header.snaplen
        {
            return Err(PcapReadError::InvalidCapturedLength {
                captured: captured_len,
                original: original_len,
                snaplen: self.global_header.snaplen,
            });
        }
        let payload_end = cursor.checked_add(captured_len as usize).ok_or(
            PcapReadError::TruncatedPacketPayload {
                expected: captured_len as usize,
                remaining: self.data.len().saturating_sub(cursor),
            },
        )?;
        if payload_end > self.data.len() {
            return Err(PcapReadError::TruncatedPacketPayload {
                expected: captured_len as usize,
                remaining: self.data.len().saturating_sub(cursor),
            });
        }
        let captured_at = CaptureTimestamp::from_unix_parts(
            ts_sec as u64,
            ts_subsecond,
            self.global_header.timestamp_precision,
        )?;
        let bytes = self.data[cursor..payload_end].to_vec();
        self.offset = payload_end;
        Ok(Some(PcapRecord {
            bytes,
            captured_at,
            captured_len,
            original_len,
        }))
    }
}

fn read_u16(data: &[u8], offset: &mut usize, byte_order: PcapByteOrder) -> u16 {
    let bytes = [data[*offset], data[*offset + 1]];
    *offset += 2;
    match byte_order {
        PcapByteOrder::LittleEndian => u16::from_le_bytes(bytes),
        PcapByteOrder::BigEndian => u16::from_be_bytes(bytes),
    }
}

fn read_u32(data: &[u8], offset: &mut usize, byte_order: PcapByteOrder) -> u32 {
    let bytes = [
        data[*offset],
        data[*offset + 1],
        data[*offset + 2],
        data[*offset + 3],
    ];
    *offset += 4;
    match byte_order {
        PcapByteOrder::LittleEndian => u32::from_le_bytes(bytes),
        PcapByteOrder::BigEndian => u32::from_be_bytes(bytes),
    }
}

fn read_i32(data: &[u8], offset: &mut usize, byte_order: PcapByteOrder) -> i32 {
    read_u32(data, offset, byte_order) as i32
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OfflinePcapManifest {
    pub schema_version: String,
    pub dataset_id: String,
    pub run_id: String,
    pub base_dir: String,
    pub entries: Vec<OfflinePcapEntry>,
    #[serde(skip)]
    pub body_sha256: String,
    #[serde(skip)]
    manifest_path: PathBuf,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct OfflinePcapEntry {
    pub entry_id: String,
    pub relative_path: String,
    pub sha256: String,
    pub size_bytes: u64,
    pub byte_order: PcapByteOrder,
    pub timestamp_precision: TimestampPrecision,
    pub link_type: u32,
    pub packet_count: u64,
}

impl OfflinePcapManifest {
    pub fn load_and_validate(path: &Path) -> Result<Self> {
        let body = std::fs::read(path)
            .with_context(|| format!("PCAP_MANIFEST_READ_FAILED path={}", path.display()))?;
        let mut manifest: Self = serde_json::from_slice(&body)
            .with_context(|| format!("PCAP_MANIFEST_PARSE_FAILED path={}", path.display()))?;
        if manifest.schema_version != "1.0.0" {
            anyhow::bail!("PCAP_MANIFEST_UNSUPPORTED_SCHEMA");
        }
        if manifest.dataset_id.trim().is_empty()
            || manifest.run_id.trim().is_empty()
            || manifest.entries.is_empty()
        {
            anyhow::bail!("PCAP_MANIFEST_IDENTITY_REQUIRED");
        }
        manifest.body_sha256 = sha256_hex(&body);
        manifest.manifest_path = path
            .canonicalize()
            .with_context(|| format!("PCAP_MANIFEST_PATH_INVALID path={}", path.display()))?;
        let base_dir = manifest.canonical_base_dir()?;
        let mut entry_ids = HashSet::new();
        let mut relative_paths = HashSet::new();
        for entry in &manifest.entries {
            if !entry_ids.insert(entry.entry_id.as_str()) || entry.entry_id.trim().is_empty() {
                anyhow::bail!("PCAP_MANIFEST_DUPLICATE_ENTRY_ID");
            }
            if !relative_paths.insert(entry.relative_path.as_str()) {
                anyhow::bail!("PCAP_MANIFEST_DUPLICATE_PATH");
            }
            validate_manifest_entry(&base_dir, entry)?;
        }
        Ok(manifest)
    }

    fn canonical_base_dir(&self) -> Result<PathBuf> {
        let configured = Path::new(&self.base_dir);
        let base = if configured.is_absolute() {
            configured.to_path_buf()
        } else {
            self.manifest_path
                .parent()
                .ok_or_else(|| anyhow::anyhow!("PCAP_MANIFEST_BASE_DIR_INVALID"))?
                .join(configured)
        };
        base.canonicalize()
            .context("PCAP_MANIFEST_BASE_DIR_INVALID")
    }
}

fn validate_manifest_entry(base_dir: &Path, entry: &OfflinePcapEntry) -> Result<PathBuf> {
    let relative = Path::new(&entry.relative_path);
    if relative.as_os_str().is_empty()
        || relative.is_absolute()
        || relative
            .components()
            .any(|component| !matches!(component, std::path::Component::Normal(_)))
    {
        anyhow::bail!("PCAP_MANIFEST_PATH_ESCAPE entry_id={}", entry.entry_id);
    }
    if !is_lower_hex_sha256(&entry.sha256) {
        anyhow::bail!(
            "PCAP_MANIFEST_ENTRY_SHA256_INVALID entry_id={}",
            entry.entry_id
        );
    }
    let joined = base_dir.join(relative);
    let canonical = joined
        .canonicalize()
        .with_context(|| format!("PCAP_MANIFEST_ENTRY_MISSING entry_id={}", entry.entry_id))?;
    if !canonical.starts_with(base_dir) {
        anyhow::bail!("PCAP_MANIFEST_PATH_ESCAPE entry_id={}", entry.entry_id);
    }
    let metadata = std::fs::metadata(&canonical)?;
    if !metadata.is_file() || metadata.len() != entry.size_bytes {
        anyhow::bail!(
            "PCAP_MANIFEST_ENTRY_SIZE_MISMATCH entry_id={}",
            entry.entry_id
        );
    }
    let bytes = std::fs::read(&canonical)?;
    if sha256_hex(&bytes) != entry.sha256 {
        anyhow::bail!(
            "PCAP_MANIFEST_ENTRY_HASH_MISMATCH entry_id={}",
            entry.entry_id
        );
    }
    let mut reader = PcapReader::from_file(&canonical)?;
    if reader.global_header.byte_order != entry.byte_order
        || reader.global_header.timestamp_precision != entry.timestamp_precision
        || reader.global_header.network != entry.link_type
        || reader.global_header.version_major != 2
        || reader.global_header.version_minor != 4
    {
        anyhow::bail!(
            "PCAP_MANIFEST_ENTRY_FORMAT_MISMATCH entry_id={}",
            entry.entry_id
        );
    }
    let mut packet_count = 0u64;
    while reader.next_packet_checked()?.is_some() {
        packet_count += 1;
    }
    if packet_count != entry.packet_count {
        anyhow::bail!(
            "PCAP_MANIFEST_PACKET_COUNT_MISMATCH entry_id={}",
            entry.entry_id
        );
    }
    Ok(canonical)
}

fn sha256_hex(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    let mut output = String::with_capacity(64);
    for byte in digest {
        write!(&mut output, "{byte:02x}").expect("writing to String cannot fail");
    }
    output
}

fn is_lower_hex_sha256(value: &str) -> bool {
    value.len() == 64
        && value
            .as_bytes()
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
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
                    if !mult.is_finite() || mult <= 0.0 {
                        anyhow::bail!("PCAP_REPLAY_SPEED_INVALID");
                    }
                    Ok(Self::Multiplier(mult))
                } else {
                    anyhow::bail!("PCAP_REPLAY_SPEED_INVALID")
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
    first_packet_at: Option<CaptureTimestamp>,
    packets_sent: u64,
    snaplen: u32,
    network: u32,
    exhausted: bool,
}

impl PcapReplayer {
    /// Create from a config, reading pcap files from the configured pcap directory
    pub fn from_config(config: &CaptureConfig) -> Result<Self> {
        let pcap_dir = config.pcap_dir.as_deref().unwrap_or("./pcap");
        let speed = ReplaySpeed::from_str(config.replay_speed.as_deref().unwrap_or("original"))
            .context("PCAP_REPLAY_SPEED_INVALID")?;

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
            first_packet_at: None,
            packets_sent: 0,
            snaplen: 65535,
            network: 1,
            exhausted: false,
        })
    }

    fn open_next_file(&mut self) -> Result<()> {
        if self.current_file_idx >= self.pcap_files.len() {
            if self.loop_replay {
                info!("PCAP replay loop: restarting from first file");
                self.current_file_idx = 0;
                self.first_packet_at = None;
                self.start_time = Some(Instant::now());
                self.exhausted = false;
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

pub struct ManifestReplayState {
    pub manifest_body_sha256: String,
    pub current_entry_id: Option<String>,
    manifest: OfflinePcapManifest,
    base_dir: PathBuf,
    next_entry_index: usize,
    current_reader: Option<PcapReader>,
    first_packet_at: Option<CaptureTimestamp>,
    replay_started_at: Option<Instant>,
    pub stats: CaptureStats,
    pub packets_sent: u64,
}

pub struct ManifestPcapReplayer {
    state: ManifestReplayState,
    speed: ReplaySpeed,
    loop_replay: bool,
    started: bool,
    stopped: Arc<AtomicBool>,
    exhausted: bool,
}

impl ManifestPcapReplayer {
    pub fn from_manifest(
        manifest: OfflinePcapManifest,
        speed: ReplaySpeed,
        loop_replay: bool,
    ) -> Result<Self> {
        if manifest.body_sha256.is_empty() || manifest.entries.is_empty() {
            anyhow::bail!("PCAP_MANIFEST_NOT_VALIDATED");
        }
        let base_dir = manifest.canonical_base_dir()?;
        Ok(Self {
            state: ManifestReplayState {
                manifest_body_sha256: manifest.body_sha256.clone(),
                current_entry_id: None,
                manifest,
                base_dir,
                next_entry_index: 0,
                current_reader: None,
                first_packet_at: None,
                replay_started_at: None,
                stats: CaptureStats::default(),
                packets_sent: 0,
            },
            speed,
            loop_replay,
            started: false,
            stopped: Arc::new(AtomicBool::new(false)),
            exhausted: false,
        })
    }

    fn reset_loop(&mut self) {
        self.state.next_entry_index = 0;
        self.state.current_reader = None;
        self.state.current_entry_id = None;
        self.state.first_packet_at = None;
        self.state.replay_started_at = Some(Instant::now());
        self.exhausted = false;
    }

    pub fn open_next_manifest_entry(&mut self) -> Result<bool> {
        if self.state.next_entry_index >= self.state.manifest.entries.len() {
            if !self.loop_replay {
                self.state.current_reader = None;
                self.exhausted = true;
                return Ok(false);
            }
            self.reset_loop();
        }
        let entry = &self.state.manifest.entries[self.state.next_entry_index];
        let canonical = validate_manifest_entry(&self.state.base_dir, entry)?;
        let reader = PcapReader::from_file(&canonical)?;
        self.state.current_entry_id = Some(entry.entry_id.clone());
        self.state.current_reader = Some(reader);
        self.state.next_entry_index += 1;
        Ok(true)
    }

    pub fn start_manifest(&mut self) -> Result<()> {
        if self.started {
            anyhow::bail!("PCAP_MANIFEST_REPLAYER_ALREADY_STARTED");
        }
        self.reset_loop();
        if !self.open_next_manifest_entry()? {
            anyhow::bail!("PCAP_MANIFEST_EMPTY");
        }
        self.stopped.store(false, Ordering::SeqCst);
        self.started = true;
        Ok(())
    }

    pub fn poll_manifest(&mut self) -> Result<Option<PacketBatch>> {
        if !self.started || self.stopped.load(Ordering::SeqCst) {
            return Ok(None);
        }
        let mut packets = Vec::with_capacity(64);
        while packets.len() < 64 {
            if self.state.current_reader.is_none() && !self.open_next_manifest_entry()? {
                break;
            }
            let record = match self
                .state
                .current_reader
                .as_mut()
                .ok_or_else(|| anyhow::anyhow!("pcap reader not initialized after manifest open"))?
                .next_packet_checked()?
            {
                Some(record) => record,
                None => {
                    self.state.current_reader = None;
                    continue;
                }
            };
            if self.state.first_packet_at.is_none() {
                self.state.first_packet_at = Some(record.captured_at);
                self.state.replay_started_at = Some(Instant::now());
            }
            rate_limit_packet(
                self.speed,
                self.state.replay_started_at,
                self.state.first_packet_at,
                record.captured_at,
            )?;
            packets.push((record.bytes, record.captured_at));
        }
        if packets.is_empty() {
            return Ok(None);
        }
        self.state.packets_sent += packets.len() as u64;
        self.state.stats.packets_received += packets.len() as u64;
        self.state.stats.bytes_received += packets
            .iter()
            .map(|(data, _)| data.len() as u64)
            .sum::<u64>();
        Ok(Some(create_owned_batch(packets)))
    }

    pub fn manifest_body_sha256(&self) -> &str {
        &self.state.manifest_body_sha256
    }

    pub fn current_entry_id(&self) -> Option<&str> {
        self.state.current_entry_id.as_deref()
    }
}

#[async_trait::async_trait]
impl Capturer for ManifestPcapReplayer {
    async fn start(&mut self) -> Result<()> {
        self.start_manifest()
    }

    async fn stop(&mut self) -> Result<()> {
        self.stopped.store(true, Ordering::SeqCst);
        self.started = false;
        Ok(())
    }

    fn poll(&mut self) -> Result<Option<PacketBatch>> {
        self.poll_manifest()
    }

    fn stats(&self) -> CaptureStats {
        self.state.stats.clone()
    }

    fn end_of_input(&self) -> bool {
        self.exhausted && !self.loop_replay
    }
}

/// Rate-limit packet replay to match original or scaled speed.
fn replay_delay(
    speed: ReplaySpeed,
    first: CaptureTimestamp,
    current: CaptureTimestamp,
) -> Result<Duration> {
    let delta_micros = current
        .epoch_micros()
        .checked_sub(first.epoch_micros())
        .ok_or_else(|| anyhow::anyhow!("PCAP_REPLAY_TIMESTAMP_REORDERED"))?;
    match speed {
        ReplaySpeed::MaxSpeed => Ok(Duration::ZERO),
        ReplaySpeed::Original => Ok(Duration::from_micros(delta_micros)),
        ReplaySpeed::Multiplier(multiplier) => {
            if !multiplier.is_finite() || multiplier <= 0.0 {
                anyhow::bail!("PCAP_REPLAY_SPEED_INVALID");
            }
            let scaled = delta_micros as f64 / multiplier;
            if !scaled.is_finite() || scaled > u64::MAX as f64 {
                anyhow::bail!("PCAP_REPLAY_DELAY_OVERFLOW");
            }
            Ok(Duration::from_micros(scaled as u64))
        }
    }
}

fn rate_limit_packet(
    speed: ReplaySpeed,
    start_time: Option<Instant>,
    first: Option<CaptureTimestamp>,
    current: CaptureTimestamp,
) -> Result<()> {
    if let (Some(start), Some(first)) = (start_time, first) {
        let target_elapsed = replay_delay(speed, first, current)?;
        let elapsed_real = start.elapsed();
        if target_elapsed > elapsed_real {
            let sleep_time = target_elapsed - elapsed_real;
            if sleep_time > Duration::from_micros(100) {
                std::thread::sleep(sleep_time);
            }
        }
    }
    Ok(())
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
        self.exhausted = false;

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
        let reader_exhausted = match self.current_reader.as_ref() {
            Some(reader) => !reader.has_next(),
            None => true,
        };
        if reader_exhausted {
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
                    self.exhausted = true;
                    return Ok(None);
                }
            }
        }

        // Read a batch of packets (up to 64 per poll for efficiency)
        let mut packets = Vec::new();
        let reader = self
            .current_reader
            .as_mut()
            .ok_or_else(|| anyhow::anyhow!("pcap reader not initialized before batch read"))?;
        let speed = self.speed;
        let mut start_time = self.start_time;
        let mut first_at = self.first_packet_at;

        for _ in 0..64 {
            if let Some(record) = reader.next_packet_checked()? {
                if first_at.is_none() {
                    first_at = Some(record.captured_at);
                    start_time = Some(Instant::now());
                }

                rate_limit_packet(speed, start_time, first_at, record.captured_at)?;
                packets.push((record.bytes, record.captured_at));
            } else {
                break;
            }
        }
        let _ = reader; // explicitly drop reader to release resources
        self.first_packet_at = first_at;
        self.start_time = start_time;

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

    fn end_of_input(&self) -> bool {
        self.exhausted && !self.loop_replay
    }
}

fn create_owned_batch(packets: Vec<(Vec<u8>, CaptureTimestamp)>) -> PacketBatch {
    PacketBatch::from_owned_packets(packets)
}

#[cfg(test)]
mod replay_delay_tests {
    use super::{replay_delay, ReplaySpeed};
    use crate::capture::{CaptureTimestamp, TimestampPrecision};
    use std::time::Duration;

    fn micros(value: u64) -> CaptureTimestamp {
        CaptureTimestamp::from_unix_parts(
            value / 1_000_000,
            (value % 1_000_000) as u32,
            TimestampPrecision::Microsecond,
        )
        .unwrap()
    }

    #[test]
    fn replay_delay_uses_microseconds_and_explicit_multiplier() {
        let first = micros(1_000_000);
        let current = micros(1_250_000);
        assert_eq!(
            replay_delay(ReplaySpeed::Original, first, current).unwrap(),
            Duration::from_millis(250)
        );
        assert_eq!(
            replay_delay(ReplaySpeed::Multiplier(2.0), first, current).unwrap(),
            Duration::from_millis(125)
        );
        assert_eq!(
            replay_delay(ReplaySpeed::MaxSpeed, first, current).unwrap(),
            Duration::ZERO
        );
    }

    #[test]
    fn replay_delay_rejects_reordered_time_and_invalid_speed() {
        assert!(replay_delay(ReplaySpeed::Original, micros(2), micros(1)).is_err());
        assert!(replay_delay(ReplaySpeed::Multiplier(0.0), micros(1), micros(2)).is_err());
    }
}
