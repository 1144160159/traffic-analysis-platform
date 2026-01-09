pub mod umem;
pub mod xdp;
pub mod af_packet;
pub mod ring;

pub use umem::{Umem, UmemConfig};
pub use xdp::XdpCapture;
pub use af_packet::AfPacketCapture;

use std::fmt;

#[derive(Debug, Clone)]
pub struct PacketRef {
    pub data: Vec<u8>,
    pub timestamp: u64,  // Unix timestamp in microseconds
    pub length: usize,
}

impl PacketRef {
    pub fn new(data: Vec<u8>, timestamp: u64) -> Self {
        let length = data.len();
        Self {
            data,
            timestamp,
            length,
        }
    }
}

pub enum CaptureMode {
    Xdp,
    AfPacket,
}

impl fmt::Display for CaptureMode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            CaptureMode::Xdp => write!(f, "XDP"),
            CaptureMode::AfPacket => write!(f, "AF_PACKET"),
        }
    }
}