pub mod buffer;
pub mod disk_monitor;
pub mod index;
pub mod pcap;
pub mod upload_journal;
pub mod uploader;

pub use pcap::{PcapGlobalHeader, PcapPacketHeader, PcapWriter, PCAP_MAGIC};

pub use buffer::{TripleBuffer, TripleBufferConfig, UploadData, WriteResult};

pub use disk_monitor::{DiskMonitor, DiskMonitorConfig};
pub use index::PcapIndexMeta;
pub use upload_journal::{JournalEntry, UploadJournal};
pub use uploader::{UploadTask, Uploader, UploaderConfig};

use std::sync::Arc;

#[derive(Debug, Clone)]
pub struct PcapPacketData {
    pub data: Vec<u8>,
    pub timestamp: u64,
}

impl PcapPacketData {
    pub fn new(data: Vec<u8>, timestamp: u64) -> Self {
        Self { data, timestamp }
    }

    pub fn from_slice(data: &[u8], timestamp: u64) -> Self {
        Self {
            data: data.to_vec(),
            timestamp,
        }
    }

    pub fn len(&self) -> usize {
        self.data.len()
    }

    pub fn is_empty(&self) -> bool {
        self.data.is_empty()
    }
}

#[derive(Debug, Clone)]
pub struct PcapWriteBatch {
    pub packets: Vec<PcapPacketData>,
    pub ts_start: u64,
    pub ts_end: u64,
}

impl PcapWriteBatch {
    pub fn new() -> Self {
        Self {
            packets: Vec::new(),
            ts_start: u64::MAX,
            ts_end: 0,
        }
    }

    pub fn with_capacity(capacity: usize) -> Self {
        Self {
            packets: Vec::with_capacity(capacity),
            ts_start: u64::MAX,
            ts_end: 0,
        }
    }

    pub fn push(&mut self, data: Vec<u8>, timestamp: u64) {
        self.ts_start = self.ts_start.min(timestamp);
        self.ts_end = self.ts_end.max(timestamp);
        self.packets.push(PcapPacketData::new(data, timestamp));
    }

    pub fn push_slice(&mut self, data: &[u8], timestamp: u64) {
        self.ts_start = self.ts_start.min(timestamp);
        self.ts_end = self.ts_end.max(timestamp);
        self.packets
            .push(PcapPacketData::from_slice(data, timestamp));
    }

    pub fn len(&self) -> usize {
        self.packets.len()
    }

    pub fn is_empty(&self) -> bool {
        self.packets.is_empty()
    }

    pub fn total_bytes(&self) -> usize {
        self.packets.iter().map(|p| p.len()).sum()
    }

    pub fn clear(&mut self) {
        self.packets.clear();
        self.ts_start = u64::MAX;
        self.ts_end = 0;
    }

    pub fn iter(&self) -> impl Iterator<Item = &PcapPacketData> {
        self.packets.iter()
    }
}

impl Default for PcapWriteBatch {
    fn default() -> Self {
        Self::new()
    }
}

pub struct PcapArchiver {
    buffer: Arc<TripleBuffer>,
    packets_written: u64,
    bytes_written: u64,
    write_errors: u64,
    pcap_full_capture: bool,
}

impl PcapArchiver {
    pub fn new(config: TripleBufferConfig) -> Self {
        Self {
            buffer: Arc::new(TripleBuffer::new(config)),
            packets_written: 0,
            bytes_written: 0,
            write_errors: 0,
            pcap_full_capture: true,
        }
    }

    pub fn from_buffer(buffer: Arc<TripleBuffer>) -> Self {
        Self {
            buffer,
            packets_written: 0,
            bytes_written: 0,
            write_errors: 0,
            pcap_full_capture: true,
        }
    }

    pub fn set_pcap_mode(&mut self, full_capture: bool) {
        self.pcap_full_capture = full_capture;
    }

    #[inline]
    pub fn write_packet(&mut self, timestamp: u64, data: &[u8]) -> WriteResult {
        let result = self.buffer.write_packet(timestamp, data);
        match result {
            WriteResult::Ok | WriteResult::Rotated => {
                self.packets_written += 1;
                self.bytes_written += data.len() as u64;
            }
            WriteResult::Fallback => {
                self.packets_written += 1;
                self.bytes_written += data.len() as u64;
                use tracing::warn;
                warn!("PCAP write fallback to disk - buffer overflow");
            }
            WriteResult::Blocked | WriteResult::Error => {
                self.write_errors += 1;
            }
        }
        result
    }

    pub fn write_batch(&mut self, batch: &PcapWriteBatch) -> (usize, usize) {
        let mut success = 0;
        let mut failed = 0;

        for packet in &batch.packets {
            match self.write_packet(packet.timestamp, &packet.data) {
                WriteResult::Ok | WriteResult::Rotated | WriteResult::Fallback => success += 1,
                WriteResult::Blocked | WriteResult::Error => failed += 1,
            }
        }

        (success, failed)
    }

    pub fn buffer(&self) -> &Arc<TripleBuffer> {
        &self.buffer
    }

    pub fn stats(&self) -> (u64, u64, u64) {
        (self.packets_written, self.bytes_written, self.write_errors)
    }

    pub fn force_rotate(&self) -> bool {
        self.buffer.force_rotate()
    }

    pub async fn wait_for_upload(&self) -> Option<UploadData> {
        self.buffer.wait_for_upload().await
    }

    pub fn complete_upload(&self, buffer_idx: u8) {
        self.buffer.complete_upload(buffer_idx);
    }

    pub fn find_uploading_buffer(&self) -> Option<u8> {
        self.buffer.find_uploading_buffer()
    }
}

pub fn batch_to_pcap_batch(batch: &crate::capture::PacketBatch) -> PcapWriteBatch {
    let mut pcap_batch = PcapWriteBatch::with_capacity(batch.len());

    for i in 0..batch.len() {
        if let Some(data) = batch.get_packet(i) {
            let timestamp = batch.frames[i].timestamp;
            pcap_batch.push(data.to_vec(), timestamp);
        }
    }

    pcap_batch
}
