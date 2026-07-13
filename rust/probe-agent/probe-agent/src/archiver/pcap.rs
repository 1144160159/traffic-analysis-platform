use byteorder::{LittleEndian, WriteBytesExt};
use std::io::{Result as IoResult, Write};

pub const PCAP_MAGIC: u32 = 0xa1b2c3d4;

pub const PCAP_MAGIC_NANO: u32 = 0xa1b23c4d;

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PcapGlobalHeader {
    pub magic_number: u32,

    pub version_major: u16,

    pub version_minor: u16,

    pub thiszone: i32,

    pub sigfigs: u32,

    pub snaplen: u32,

    pub network: u32,
}

impl Default for PcapGlobalHeader {
    fn default() -> Self {
        Self {
            magic_number: PCAP_MAGIC,
            version_major: 2,
            version_minor: 4,
            thiszone: 0,
            sigfigs: 0,
            snaplen: 65535,
            network: 1,
        }
    }
}

impl PcapGlobalHeader {
    pub fn write_to<W: Write>(&self, writer: &mut W) -> IoResult<()> {
        writer.write_u32::<LittleEndian>(self.magic_number)?;
        writer.write_u16::<LittleEndian>(self.version_major)?;
        writer.write_u16::<LittleEndian>(self.version_minor)?;
        writer.write_i32::<LittleEndian>(self.thiszone)?;
        writer.write_u32::<LittleEndian>(self.sigfigs)?;
        writer.write_u32::<LittleEndian>(self.snaplen)?;
        writer.write_u32::<LittleEndian>(self.network)?;
        Ok(())
    }

    pub const fn size() -> usize {
        24
    }
}

#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PcapPacketHeader {
    pub ts_sec: u32,

    pub ts_usec: u32,

    pub incl_len: u32,

    pub orig_len: u32,
}

impl PcapPacketHeader {
    pub fn new(timestamp_us: u64, len: u32) -> Self {
        Self {
            ts_sec: (timestamp_us / 1_000_000) as u32,
            ts_usec: (timestamp_us % 1_000_000) as u32,
            incl_len: len,
            orig_len: len,
        }
    }

    pub fn write_to<W: Write>(&self, writer: &mut W) -> IoResult<()> {
        writer.write_u32::<LittleEndian>(self.ts_sec)?;
        writer.write_u32::<LittleEndian>(self.ts_usec)?;
        writer.write_u32::<LittleEndian>(self.incl_len)?;
        writer.write_u32::<LittleEndian>(self.orig_len)?;
        Ok(())
    }

    pub const fn size() -> usize {
        16
    }

    pub fn timestamp_us(&self) -> u64 {
        (self.ts_sec as u64) * 1_000_000 + (self.ts_usec as u64)
    }
}

pub struct PcapWriter {
    pub(crate) buffer: Vec<u8>,

    packet_count: u64,

    first_timestamp: Option<u64>,

    last_timestamp: Option<u64>,
}

impl PcapWriter {
    pub fn new(capacity: usize) -> Self {
        let mut buffer = Vec::with_capacity(capacity);

        let header = PcapGlobalHeader::default();
        header.write_to(&mut buffer).unwrap();

        Self {
            buffer,
            packet_count: 0,
            first_timestamp: None,
            last_timestamp: None,
        }
    }

    pub fn write_packet(&mut self, timestamp_us: u64, data: &[u8]) -> IoResult<()> {
        if self.first_timestamp.is_none() {
            self.first_timestamp = Some(timestamp_us);
        }
        self.last_timestamp = Some(timestamp_us);

        let header = PcapPacketHeader::new(timestamp_us, data.len() as u32);
        header.write_to(&mut self.buffer)?;

        self.buffer.extend_from_slice(data);

        self.packet_count += 1;
        Ok(())
    }

    pub fn size(&self) -> usize {
        self.buffer.len()
    }

    pub fn packet_count(&self) -> u64 {
        self.packet_count
    }

    pub fn time_range(&self) -> (Option<u64>, Option<u64>) {
        (self.first_timestamp, self.last_timestamp)
    }

    pub fn finish(self) -> Vec<u8> {
        self.buffer
    }

    pub fn clone_data(&self) -> Vec<u8> {
        self.buffer.clone()
    }

    pub fn capacity(&self) -> usize {
        self.buffer.capacity()
    }

    pub fn reset(&mut self) {
        self.buffer.clear();

        let header = PcapGlobalHeader::default();
        header.write_to(&mut self.buffer).unwrap();

        self.packet_count = 0;
        self.first_timestamp = None;
        self.last_timestamp = None;
    }

    pub fn is_empty(&self) -> bool {
        self.packet_count == 0
    }

    pub fn as_bytes(&self) -> &[u8] {
        &self.buffer
    }
}

impl Clone for PcapWriter {
    fn clone(&self) -> Self {
        Self {
            buffer: self.buffer.clone(),
            packet_count: self.packet_count,
            first_timestamp: self.first_timestamp,
            last_timestamp: self.last_timestamp,
        }
    }
}

impl Default for PcapWriter {
    fn default() -> Self {
        Self::new(64 * 1024)
    }
}
