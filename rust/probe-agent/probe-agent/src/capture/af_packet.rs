use anyhow::{Result, Context};
use std::os::unix::io::AsRawFd;
use std::net::UdpSocket;
use std::time::{SystemTime, UNIX_EPOCH};
use super::PacketRef;

/// AF_PACKET 捕获器（备用方案，不依赖 XDP）
pub struct AfPacketCapture {
    socket: UdpSocket,
    interface: String,
    buffer: Vec<u8>,
}

impl AfPacketCapture {
    pub fn new(interface: &str, buffer_size: usize) -> Result<Self> {
        // 简化实现：使用标准 socket（生产环境需要使用 raw socket）
        let socket = UdpSocket::bind("0.0.0.0:0")
            .context("Failed to create AF_PACKET socket")?;
        
        socket.set_nonblocking(true)?;
        
        tracing::info!(
            "AF_PACKET capture initialized on {} (buffer: {} bytes)",
            interface,
            buffer_size
        );

        Ok(Self {
            socket,
            interface: interface.to_string(),
            buffer: vec![0u8; buffer_size],
        })
    }

    /// 轮询捕获数据包
    pub async fn poll(&mut self) -> Result<Vec<PacketRef>> {
        let mut packets = Vec::new();

        // 非阻塞读取
        loop {
            match self.socket.recv(&mut self.buffer) {
                Ok(len) => {
                    let timestamp = SystemTime::now()
                        .duration_since(UNIX_EPOCH)?
                        .as_micros() as u64;
                    
                    let packet = PacketRef::new(
                        self.buffer[..len].to_vec(),
                        timestamp,
                    );
                    
                    packets.push(packet);
                }
                Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                    break;
                }
                Err(e) => {
                    return Err(e.into());
                }
            }
        }

        Ok(packets)
    }
}