use super::{FlowTable, FlowKey, PacketInfo};
use std::sync::Arc;
use std::net::IpAddr;
use etherparse::{SlicedPacket, TransportSlice};
use anyhow::Result;

pub struct PacketProcessor {
    flow_table: Arc<FlowTable>,
}

impl PacketProcessor {
    pub fn new(flow_table: Arc<FlowTable>) -> Self {
        Self { flow_table }
    }

    /// 处理单个数据包
    pub fn process(&self, data: &[u8], timestamp: u64) -> Result<()> {
        // 解析以太网帧
        let packet = match SlicedPacket::from_ethernet(data) {
            Ok(p) => p,
            Err(_) => return Ok(()), // 跳过无效包
        };

        // 提取 IP 层
        let (src_ip, dst_ip, protocol) = match packet.ip {
            Some(etherparse::InternetSlice::Ipv4(ipv4, _)) => {
                (
                    IpAddr::from(ipv4.source_addr()),
                    IpAddr::from(ipv4.destination_addr()),
                    ipv4.protocol(),
                )
            }
            Some(etherparse::InternetSlice::Ipv6(ipv6, _)) => {
                (
                    IpAddr::from(ipv6.source_addr()),
                    IpAddr::from(ipv6.destination_addr()),
                    ipv6.next_header(),
                )
            }
            None => return Ok(()),
        };

        // 提取传输层
        let (src_port, dst_port, tcp_flags) = match packet.transport {
            Some(TransportSlice::Tcp(tcp)) => {
                (tcp.source_port(), tcp.destination_port(), tcp.flags())
            }
            Some(TransportSlice::Udp(udp)) => {
                (udp.source_port(), udp.destination_port(), 0)
            }
            _ => (0, 0, 0),
        };

        // 构建流键
        let flow_key = FlowKey {
            src_ip,
            dst_ip,
            src_port,
            dst_port,
            protocol,
        };

        // 构建包信息
        let packet_info = PacketInfo {
            len: data.len() as u16,
            tcp_flags,
            is_forward: true, // TODO: 判断方向
            timestamp,
        };

        // 更新流表
        self.flow_table.update(&flow_key, &packet_info);

        Ok(())
    }
}