use std::io::{Write, Result as IoResult};
use byteorder::{LittleEndian, WriteBytesExt};

/// PCAP 全局头
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PcapGlobalHeader {
    pub magic_number: u32,   // 0xa1b2c3d4
    pub version_major: u16,  // 2
    pub version_minor: u16,  // 4
    pub thiszone: i32,       // GMT offset (0)
    pub sigfigs: u32,        // accuracy (0)
    pub snaplen: u32,        // max packet length (65535)
    pub network: u32,        // link layer type (1 = Ethernet)
}

impl Default for PcapGlobalHeader {
    fn default() -> Self {
        Self {
            magic_number: 0xa1b2c3d4,
            version_major: 2,
            version_minor: 4,
            thiszone: 0,
            sigfigs: 0,
            snaplen: 65535,
            network: 1,  // DLT_EN10MB
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
}

/// PCAP 包头
#[repr(C)]
#[derive(Debug, Clone, Copy)]
pub struct PcapPacketHeader {
    pub ts_sec: u32,      // timestamp seconds
    pub ts_usec: u32,     // timestamp microseconds
    pub incl_len: u32,    // saved length
    pub orig_len: u32,    // original length
}

impl PcapPacketHeader {
    pub fn new(timestamp_us: u64, packet_len: u32) -> Self {
        Self {
            ts_sec: (timestamp_us / 1_000_000) as u32,
            ts_usec: (timestamp_us % 1_000_000) as u32,
            incl_len: packet_len,
            orig_len: packet_len,
        }
    }

    pub fn write_to<W: Write>(&self, writer: &mut W) -> IoResult<()> {
        writer.write_u32::<LittleEndian>(self.ts_sec)?;
        writer.write_u32::<LittleEndian>(self.ts_usec)?;
        writer.write_u32::<LittleEndian>(self.incl_len)?;
        writer.write_u32::<LittleEndian>(self.orig_len)?;
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_pcap_header_size() {
        let mut buf = Vec::new();
        let header = PcapGlobalHeader::default();
        header.write_to(&mut buf).unwrap();
        assert_eq!(buf.len(), 24, "PCAP global header should be 24 bytes");
    }

    #[test]
    fn test_pcap_packet_header() {
        let mut buf = Vec::new();
        let header = PcapPacketHeader::new(1234567890000000, 1500);
        header.write_to(&mut buf).unwrap();
        assert_eq!(buf.len(), 16, "PCAP packet header should be 16 bytes");
    }
}