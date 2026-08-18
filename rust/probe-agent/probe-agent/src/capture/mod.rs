pub mod af_packet;
pub mod frame_allocator;
pub mod packet_batch;
pub mod pcap_offline;
pub mod pcap_replay_op;
pub mod promisc;
pub mod ring;
pub mod umem;
pub mod wire_inject;
pub mod xdp;
pub mod xdp_socket;
pub mod xdp_sys;

use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use thiserror::Error;

pub use af_packet::AfPacketCapture;
pub use frame_allocator::{FrameAllocator, FrameAllocatorStats};
pub use packet_batch::{IndexedPacket, PacketBatchBuilder, PacketBatchExt, PacketBatchStats};
pub use pcap_offline::{ManifestPcapReplayer, OfflinePcapManifest, PcapReplayer, ReplaySpeed};
pub use promisc::PromiscuousMode;
pub use ring::{CompQueue, FillQueue, RxQueue, TxQueue, XdpDesc};
pub use umem::{Umem, UmemConfig};
pub use xdp::XdpCapture;
pub use xdp_socket::{XskSocket, XskSocketConfig};
pub use xdp_sys::{XdpMmapOffsets, XdpStatistics, XdpUmemReg};

pub use crate::config::{CaptureConfig, CaptureMode};

/// Precision carried by the capture source before normalization.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TimestampPrecision {
    Microsecond,
    Nanosecond,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CaptureTimestampProvenance {
    SourceRecord,
    KernelPerFrame,
    CorrelatedMonotonic,
    BatchWallClockDegraded,
    DescriptorWallClockDegraded,
}

/// Immutable view passed from a capture batch into either parser route.
/// Keeping the bytes and their checked timestamp together prevents adapters
/// from silently changing time units or frame length between routes.
#[derive(Debug, Clone, Copy)]
pub struct CaptureFrame<'a> {
    pub bytes: &'a [u8],
    pub captured_at: CaptureTimestamp,
}

impl<'a> CaptureFrame<'a> {
    pub fn new(bytes: &'a [u8], captured_at: CaptureTimestamp) -> Self {
        Self { bytes, captured_at }
    }
}

/// Checked capture time used by all deterministic offline readers.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CaptureTimestamp {
    epoch_micros: u64,
    pub source_precision: TimestampPrecision,
    pub precision_loss_nanos: u16,
    pub provenance: CaptureTimestampProvenance,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Error)]
pub enum CaptureTimestampError {
    #[error("CAPTURE_TIMESTAMP_SUBSECOND_OUT_OF_RANGE")]
    SubsecondOutOfRange,
    #[error("CAPTURE_TIMESTAMP_OVERFLOW")]
    Overflow,
}

impl CaptureTimestamp {
    pub fn from_unix_parts(
        seconds: u64,
        subsecond: u32,
        precision: TimestampPrecision,
    ) -> std::result::Result<Self, CaptureTimestampError> {
        let (subsecond_micros, precision_loss_nanos) = match precision {
            TimestampPrecision::Microsecond if subsecond < 1_000_000 => (subsecond as u64, 0),
            TimestampPrecision::Nanosecond if subsecond < 1_000_000_000 => {
                ((subsecond / 1_000) as u64, (subsecond % 1_000) as u16)
            }
            _ => return Err(CaptureTimestampError::SubsecondOutOfRange),
        };
        let epoch_micros = seconds
            .checked_mul(1_000_000)
            .and_then(|value| value.checked_add(subsecond_micros))
            .ok_or(CaptureTimestampError::Overflow)?;
        Ok(Self {
            epoch_micros,
            source_precision: precision,
            precision_loss_nanos,
            provenance: CaptureTimestampProvenance::SourceRecord,
        })
    }

    pub fn from_epoch_micros(epoch_micros: u64, provenance: CaptureTimestampProvenance) -> Self {
        Self {
            epoch_micros,
            source_precision: TimestampPrecision::Microsecond,
            precision_loss_nanos: 0,
            provenance,
        }
    }

    pub fn epoch_micros(self) -> u64 {
        self.epoch_micros
    }

    pub fn with_provenance(mut self, provenance: CaptureTimestampProvenance) -> Self {
        self.provenance = provenance;
        self
    }
}

#[derive(Debug, Clone, Copy)]
pub struct FrameInfo {
    pub idx: usize,
    pub offset: u32,
    pub len: u32,
    pub captured_at: CaptureTimestamp,
}

impl Default for FrameInfo {
    fn default() -> Self {
        Self {
            idx: 0,
            offset: 0,
            len: 0,
            captured_at: CaptureTimestamp::from_epoch_micros(
                0,
                CaptureTimestampProvenance::BatchWallClockDegraded,
            ),
        }
    }
}

pub struct PacketBatch {
    pub umem: Arc<Umem>,
    pub frames: Vec<FrameInfo>,
    ownership_transferred: bool,
}

impl PacketBatch {
    pub fn new(umem: Arc<Umem>, frames: Vec<FrameInfo>) -> Self {
        umem.inc_batch_refcount();
        Self {
            umem,
            frames,
            ownership_transferred: false,
        }
    }

    pub fn empty(umem: Arc<Umem>) -> Self {
        umem.inc_batch_refcount();
        Self {
            umem,
            frames: Vec::new(),
            ownership_transferred: false,
        }
    }

    #[inline]
    pub fn get_packet(&self, i: usize) -> Option<&[u8]> {
        let info = self.frames.get(i)?;
        let addr = self.umem.frame_addr(info.idx) + info.offset as usize;
        self.umem.get_data(addr, info.len as usize)
    }

    #[inline]
    pub fn len(&self) -> usize {
        self.frames.len()
    }

    #[inline]
    pub fn is_empty(&self) -> bool {
        self.frames.is_empty()
    }

    pub fn total_bytes(&self) -> usize {
        self.frames.iter().map(|f| f.len as usize).sum()
    }

    pub fn iter(&self) -> PacketBatchIter<'_> {
        PacketBatchIter {
            batch: self,
            index: 0,
        }
    }

    pub fn copy_packets(&self) -> Vec<(Vec<u8>, CaptureTimestamp)> {
        let mut result = Vec::with_capacity(self.frames.len());
        for (i, info) in self.frames.iter().enumerate() {
            if let Some(data) = self.get_packet(i) {
                result.push((data.to_vec(), info.captured_at));
            }
        }
        result
    }

    pub fn copy_packet(&self, i: usize) -> Option<(Vec<u8>, CaptureTimestamp)> {
        let info = self.frames.get(i)?;
        let data = self.get_packet(i)?;
        Some((data.to_vec(), info.captured_at))
    }

    pub fn transfer_ownership(&mut self) {
        self.ownership_transferred = true;
    }

    pub fn is_ownership_transferred(&self) -> bool {
        self.ownership_transferred
    }

    pub fn umem(&self) -> &Arc<Umem> {
        &self.umem
    }

    pub fn frame_infos(&self) -> &[FrameInfo] {
        &self.frames
    }

    pub fn time_range(&self) -> Option<(u64, u64)> {
        if self.frames.is_empty() {
            return None;
        }

        let mut timestamps = self.frames.iter().map(|f| f.captured_at.epoch_micros());
        let mut min_ts = match timestamps.next() {
            Some(first) => first,
            None => return None,
        };
        let mut max_ts = min_ts;
        for ts in timestamps {
            min_ts = min_ts.min(ts);
            max_ts = max_ts.max(ts);
        }

        Some((min_ts, max_ts))
    }

    /// Create a PacketBatch from owned packet data (for PCAP offline mode).
    /// All packets are stored in a single buffer that backs the UMEM.
    pub fn from_owned_packets(packets: Vec<(Vec<u8>, CaptureTimestamp)>) -> Self {
        let total_len: usize = packets.iter().map(|(d, _)| d.len()).sum();
        let size = total_len.max(1);
        let mut buffer: Vec<u8> = vec![0u8; size];
        let mut frames = Vec::with_capacity(packets.len());

        let mut offset = 0usize;
        for (_i, (data, ts)) in packets.iter().enumerate() {
            let len = data.len();
            if len > 0 {
                buffer[offset..offset + len].copy_from_slice(data);
            }
            frames.push(FrameInfo {
                idx: 0, // All packets in same buffer, identified by offset
                offset: offset as u32,
                len: len as u32,
                captured_at: *ts,
            });
            offset += len;
        }

        Self {
            umem: Arc::new(Umem::from_buffer(buffer)),
            frames,
            ownership_transferred: false,
        }
    }
}

impl Drop for PacketBatch {
    fn drop(&mut self) {
        if !self.ownership_transferred {
            for info in &self.frames {
                self.umem.free_frame(info.idx);
            }
        }
        self.umem.dec_batch_refcount();
    }
}

pub struct PacketBatchIter<'a> {
    batch: &'a PacketBatch,
    index: usize,
}

impl<'a> Iterator for PacketBatchIter<'a> {
    type Item = (&'a [u8], CaptureTimestamp);

    fn next(&mut self) -> Option<Self::Item> {
        if self.index >= self.batch.len() {
            return None;
        }
        let info = &self.batch.frames[self.index];
        let data = self.batch.get_packet(self.index)?;
        self.index += 1;
        Some((data, info.captured_at))
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let remaining = self.batch.len() - self.index;
        (remaining, Some(remaining))
    }
}

impl<'a> ExactSizeIterator for PacketBatchIter<'a> {}

#[derive(Debug, Default, Clone)]
pub struct CaptureStats {
    pub packets_received: u64,
    pub packets_dropped: u64,
    pub bytes_received: u64,
    pub allocation_drops: u64,
    pub kernel_drops: u64,
}

impl CaptureStats {
    pub fn drop_rate(&self) -> f64 {
        let total = self.packets_received + self.packets_dropped;
        if total == 0 {
            return 0.0;
        }
        self.packets_dropped as f64 / total as f64
    }

    pub fn reset(&mut self) {
        self.packets_received = 0;
        self.packets_dropped = 0;
        self.bytes_received = 0;
        self.allocation_drops = 0;
        self.kernel_drops = 0;
    }

    pub fn merge(&mut self, other: &CaptureStats) {
        self.packets_received += other.packets_received;
        self.packets_dropped += other.packets_dropped;
        self.bytes_received += other.bytes_received;
        self.allocation_drops += other.allocation_drops;
        self.kernel_drops += other.kernel_drops;
    }
}

#[async_trait::async_trait]
pub trait Capturer: Send + Sync {
    async fn start(&mut self) -> Result<()>;
    async fn stop(&mut self) -> Result<()>;
    fn poll(&mut self) -> Result<Option<PacketBatch>>;
    fn stats(&self) -> CaptureStats;
    /// True only when a finite offline source has consumed its final record.
    /// A transient empty live poll is never end-of-input.
    fn end_of_input(&self) -> bool {
        false
    }
}

pub async fn create_capturer(config: &CaptureConfig) -> Result<Box<dyn Capturer>> {
    tracing::info!(
        "Creating capturer: interface={}, mode={:?}",
        config.interface,
        config.mode,
    );

    let capturer: Box<dyn Capturer> = match config.mode {
        CaptureMode::PcapOffline => {
            tracing::info!("Initializing PCAP offline replayer");
            if config.pcap_manifest_route_enabled {
                let manifest_path = config
                    .pcap_manifest_path
                    .as_deref()
                    .ok_or_else(|| anyhow::anyhow!("PCAP_MANIFEST_PATH_REQUIRED"))?;
                let expected_hash = config
                    .pcap_manifest_sha256
                    .as_deref()
                    .ok_or_else(|| anyhow::anyhow!("PCAP_MANIFEST_SHA256_REQUIRED"))?;
                let manifest =
                    OfflinePcapManifest::load_and_validate(std::path::Path::new(manifest_path))?;
                if manifest.body_sha256 != expected_hash {
                    anyhow::bail!(
                        "PCAP_MANIFEST_BODY_HASH_MISMATCH expected={} actual={}",
                        expected_hash,
                        manifest.body_sha256
                    );
                }
                let speed =
                    ReplaySpeed::from_str(config.replay_speed.as_deref().unwrap_or("original"))?;
                Box::new(ManifestPcapReplayer::from_manifest(
                    manifest,
                    speed,
                    config.loop_replay.unwrap_or(false),
                )?)
            } else {
                Box::new(PcapReplayer::from_config(config)?)
            }
        }
        CaptureMode::Xdp | CaptureMode::XdpSkb | CaptureMode::XdpOffload => {
            tracing::info!("Initializing AF_XDP capturer (mode: {:?})", config.mode);
            // Try to create XDP capturer; if the host/kernel does not support AF_XDP or
            // the interface is not available (e.g., inside some CI/docker bridge),
            // attempt a graceful fallback to AfPacketCapture so tests can still run.
            match XdpCapture::new(config).await {
                Ok(xdp) => Box::new(xdp),
                Err(xdp_err) => {
                    tracing::warn!(
                        "Failed to initialize XDP capturer: {}. Attempting fallback to AF_PACKET...",
                        xdp_err
                    );

                    match AfPacketCapture::new(config) {
                        Ok(afp) => {
                            tracing::info!(
                                "Fallback to AF_PACKET succeeded (interface={}). Continuing with AF_PACKET.",
                                config.interface
                            );
                            Box::new(afp)
                        }
                        Err(afp_err) => {
                            tracing::error!(
                                "AF_PACKET fallback failed: {}; original XDP error: {}",
                                afp_err,
                                xdp_err
                            );
                            return Err(anyhow::anyhow!(
                                "Failed to initialize capture: XDP error: {} ; AF_PACKET fallback error: {}",
                                xdp_err,
                                afp_err
                            ));
                        }
                    }
                }
            }
        }
        CaptureMode::AfPacket => {
            tracing::info!("Initializing AF_PACKET capturer");
            Box::new(AfPacketCapture::new(config)?)
        }
    };

    tracing::info!("✓ Capturer created successfully");

    Ok(capturer)
}
