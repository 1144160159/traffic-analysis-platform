use anyhow::Result;
use etherparse::{NetSlice, SlicedPacket, TransportSlice};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use tracing::{debug, trace};

pub mod arp;
pub mod dhcp;
pub mod dns;

#[derive(Debug, Clone)]
pub struct ParsedPacket {
    pub src_ip: IpAddr,

    pub dst_ip: IpAddr,

    pub src_port: u16,

    pub dst_port: u16,

    pub protocol: u8,

    pub tcp_flags: u8,

    pub payload_len: u16,

    pub total_len: u16,

    pub timestamp: u64,

    pub is_fragment: bool,

    pub fragment_offset: u16,

    pub more_fragments: bool,

    pub vlan_id: Option<u16>,

    pub ttl: u8,

    pub tos: u8,

    pub fragment_id: Option<u32>,
}

impl Default for ParsedPacket {
    fn default() -> Self {
        Self {
            src_ip: IpAddr::V4(Ipv4Addr::UNSPECIFIED),
            dst_ip: IpAddr::V4(Ipv4Addr::UNSPECIFIED),
            src_port: 0,
            dst_port: 0,
            protocol: 0,
            tcp_flags: 0,
            payload_len: 0,
            total_len: 0,
            timestamp: 0,
            is_fragment: false,
            fragment_offset: 0,
            more_fragments: false,
            vlan_id: None,
            ttl: 0,
            tos: 0,
            fragment_id: None,
        }
    }
}

impl ParsedPacket {
    #[inline]
    pub fn dscp(&self) -> u8 {
        self.tos >> 2
    }

    #[inline]
    pub fn ecn(&self) -> u8 {
        self.tos & 0x03
    }

    #[inline]
    pub fn is_tcp(&self) -> bool {
        self.protocol == protocols::TCP
    }

    #[inline]
    pub fn is_udp(&self) -> bool {
        self.protocol == protocols::UDP
    }

    #[inline]
    pub fn is_icmp(&self) -> bool {
        self.protocol == protocols::ICMP || self.protocol == protocols::ICMPV6
    }

    #[inline]
    pub fn is_ipv4(&self) -> bool {
        matches!(self.src_ip, IpAddr::V4(_))
    }

    #[inline]
    pub fn is_ipv6(&self) -> bool {
        matches!(self.src_ip, IpAddr::V6(_))
    }

    #[inline]
    pub fn is_first_fragment(&self) -> bool {
        self.is_fragment && self.fragment_offset == 0
    }
}

pub mod tcp_flags {
    pub const FIN: u8 = 0x01;
    pub const SYN: u8 = 0x02;
    pub const RST: u8 = 0x04;
    pub const PSH: u8 = 0x08;
    pub const ACK: u8 = 0x10;
    pub const URG: u8 = 0x20;
    pub const ECE: u8 = 0x40;
    pub const CWR: u8 = 0x80;

    pub fn to_string(flags: u8) -> String {
        let mut s = String::new();
        if flags & SYN != 0 {
            s.push('S');
        }
        if flags & ACK != 0 {
            s.push('A');
        }
        if flags & FIN != 0 {
            s.push('F');
        }
        if flags & RST != 0 {
            s.push('R');
        }
        if flags & PSH != 0 {
            s.push('P');
        }
        if flags & URG != 0 {
            s.push('U');
        }
        if flags & ECE != 0 {
            s.push('E');
        }
        if flags & CWR != 0 {
            s.push('C');
        }
        if s.is_empty() {
            s.push('.');
        }
        s
    }

    pub fn from_string(s: &str) -> u8 {
        let mut flags = 0u8;
        for c in s.chars() {
            match c {
                'S' | 's' => flags |= SYN,
                'A' | 'a' => flags |= ACK,
                'F' | 'f' => flags |= FIN,
                'R' | 'r' => flags |= RST,
                'P' | 'p' => flags |= PSH,
                'U' | 'u' => flags |= URG,
                'E' | 'e' => flags |= ECE,
                'C' | 'c' => flags |= CWR,
                _ => {}
            }
        }
        flags
    }
}

pub mod protocols {
    pub const ICMP: u8 = 1;
    pub const TCP: u8 = 6;
    pub const UDP: u8 = 17;
    pub const GRE: u8 = 47;
    pub const ESP: u8 = 50;
    pub const AH: u8 = 51;
    pub const ICMPV6: u8 = 58;

    pub fn name(proto: u8) -> &'static str {
        match proto {
            ICMP => "ICMP",
            TCP => "TCP",
            UDP => "UDP",
            ICMPV6 => "ICMPv6",
            GRE => "GRE",
            ESP => "ESP",
            AH => "AH",
            _ => "Unknown",
        }
    }

    pub fn has_ports(proto: u8) -> bool {
        matches!(proto, TCP | UDP)
    }
}

pub mod dscp_values {

    pub const BE: u8 = 0;

    pub const EF: u8 = 46;

    pub const AF11: u8 = 10;

    pub const AF12: u8 = 12;

    pub const AF13: u8 = 14;

    pub const AF21: u8 = 18;

    pub const AF22: u8 = 20;

    pub const AF23: u8 = 22;

    pub const AF31: u8 = 26;

    pub const AF32: u8 = 28;

    pub const AF33: u8 = 30;

    pub const AF41: u8 = 34;

    pub const AF42: u8 = 36;

    pub const AF43: u8 = 38;

    pub const CS1: u8 = 8;

    pub const CS2: u8 = 16;

    pub const CS3: u8 = 24;

    pub const CS4: u8 = 32;

    pub const CS5: u8 = 40;

    pub const CS6: u8 = 48;

    pub const CS7: u8 = 56;

    pub fn name(dscp: u8) -> &'static str {
        match dscp {
            BE => "BE",
            EF => "EF",
            AF11 => "AF11",
            AF12 => "AF12",
            AF13 => "AF13",
            AF21 => "AF21",
            AF22 => "AF22",
            AF23 => "AF23",
            AF31 => "AF31",
            AF32 => "AF32",
            AF33 => "AF33",
            AF41 => "AF41",
            AF42 => "AF42",
            AF43 => "AF43",
            CS1 => "CS1",
            CS2 => "CS2",
            CS3 => "CS3",
            CS4 => "CS4",
            CS5 => "CS5",
            CS6 => "CS6",
            CS7 => "CS7",
            _ => "Unknown",
        }
    }
}

#[derive(Debug)]
pub enum ParseResult {
    Ok(ParsedPacket),

    Skip(SkipReason),

    Error(ParseError),
}

#[derive(Debug, Clone, Copy)]
pub enum SkipReason {
    NotEthernet,

    NotIp,

    FragmentNonFirst,

    UnsupportedProtocol(u8),

    TooShort,
}

impl std::fmt::Display for SkipReason {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SkipReason::NotEthernet => write!(f, "Not Ethernet"),
            SkipReason::NotIp => write!(f, "Not IP"),
            SkipReason::FragmentNonFirst => write!(f, "Non-first fragment"),
            SkipReason::UnsupportedProtocol(p) => write!(f, "Unsupported protocol: {}", p),
            SkipReason::TooShort => write!(f, "Packet too short"),
        }
    }
}

#[derive(Debug)]
pub enum ParseError {
    MalformedPacket(String),

    Other(String),
}

impl std::fmt::Display for ParseError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ParseError::MalformedPacket(s) => write!(f, "Malformed packet: {}", s),
            ParseError::Other(s) => write!(f, "Parse error: {}", s),
        }
    }
}

impl std::error::Error for ParseError {}

pub struct PacketParser;

impl PacketParser {
    pub fn parse(data: &[u8], timestamp: u64) -> Result<Option<ParsedPacket>> {
        if data.len() < 14 {
            trace!("Packet too short: {} bytes", data.len());
            return Ok(None);
        }

        let packet = match SlicedPacket::from_ethernet(data) {
            Ok(p) => p,
            Err(e) => {
                trace!("Failed to parse ethernet frame: {}", e);
                return Ok(None);
            }
        };

        let vlan_id = Self::extract_vlan_id(&packet);
        if let Some(vid) = vlan_id {
            trace!("VLAN detected: {}", vid);
        }

        let (
            src_ip,
            dst_ip,
            protocol,
            ttl,
            tos,
            is_fragment,
            fragment_offset,
            more_fragments,
            fragment_id,
        ) = match &packet.net {
            Some(NetSlice::Ipv4(ipv4)) => {
                let header = ipv4.header();

                let frag_offset = header.fragments_offset().value();
                let mf = header.more_fragments();
                let is_frag = frag_offset > 0 || mf;

                let frag_id = if is_frag {
                    Some(header.identification() as u32)
                } else {
                    None
                };

                let src_addr: Ipv4Addr = header.source().into();
                let dst_addr: Ipv4Addr = header.destination().into();

                let ip_header = header.to_header();

                let tos_value = (ip_header.dscp.value() << 2) | ip_header.ecn.value();

                let protocol_num = header.protocol();
                let protocol_u8: u8 = protocol_num.into();
                let ttl_val = header.ttl();

                trace!(
                    "IPv4: {} -> {}, proto={}, frag={}, offset={}, id={:?}",
                    src_addr,
                    dst_addr,
                    protocol_u8,
                    is_frag,
                    frag_offset,
                    frag_id
                );

                (
                    IpAddr::V4(src_addr),
                    IpAddr::V4(dst_addr),
                    protocol_u8,
                    ttl_val,
                    tos_value,
                    is_frag,
                    frag_offset,
                    mf,
                    frag_id,
                )
            }

            Some(NetSlice::Ipv6(ipv6)) => {
                let header = ipv6.header();

                let src_addr: Ipv6Addr = header.source().into();
                let dst_addr: Ipv6Addr = header.destination().into();

                let traffic_class = header.traffic_class();

                let protocol_num = header.next_header();
                let protocol_u8: u8 = protocol_num.into();
                let hop_limit = header.hop_limit();

                trace!("IPv6: {} -> {}, proto={}", src_addr, dst_addr, protocol_u8);

                (
                    IpAddr::V6(src_addr),
                    IpAddr::V6(dst_addr),
                    protocol_u8,
                    hop_limit,
                    traffic_class,
                    false,
                    0,
                    false,
                    None,
                )
            }
            None => {
                trace!("No IP layer found");
                return Ok(None);
            }
        };

        if is_fragment && fragment_offset > 0 {
            debug!(
                "Non-first fragment: {} -> {}, offset={}, id={:?}",
                src_ip, dst_ip, fragment_offset, fragment_id
            );

            return Ok(Some(ParsedPacket {
                src_ip,
                dst_ip,
                src_port: 0,
                dst_port: 0,
                protocol,
                tcp_flags: 0,
                payload_len: 0,
                total_len: data.len() as u16,
                timestamp,
                is_fragment,
                fragment_offset,
                more_fragments,
                vlan_id,
                ttl,
                tos,
                fragment_id,
            }));
        }

        let (src_port, dst_port, tcp_flags) = match &packet.transport {
            Some(TransportSlice::Tcp(tcp)) => {
                let flags = Self::extract_tcp_flags(tcp);
                let src = tcp.source_port();
                let dst = tcp.destination_port();

                debug!(
                    "TCP: {}:{} -> {}:{}, flags={}",
                    src_ip,
                    src,
                    dst_ip,
                    dst,
                    tcp_flags::to_string(flags)
                );

                (src, dst, flags)
            }

            Some(TransportSlice::Udp(udp)) => {
                let src = udp.source_port();
                let dst = udp.destination_port();

                debug!(
                    "UDP: {}:{} -> {}:{}, len={}",
                    src_ip,
                    src,
                    dst_ip,
                    dst,
                    udp.length()
                );

                (src, dst, 0)
            }

            Some(TransportSlice::Icmpv4(icmp)) => {
                let icmp_type = icmp.type_u8();
                let code_u8 = icmp.code_u8();

                debug!(
                    "ICMP: {} -> {}, type={}, code={}",
                    src_ip, dst_ip, icmp_type, code_u8
                );

                (icmp_type as u16, code_u8 as u16, 0)
            }

            Some(TransportSlice::Icmpv6(icmp)) => {
                let icmp_type = icmp.type_u8();
                let code_u8 = icmp.code_u8();

                debug!(
                    "ICMPv6: {} -> {}, type={}, code={}",
                    src_ip, dst_ip, icmp_type, code_u8
                );

                (icmp_type as u16, code_u8 as u16, 0)
            }

            None => {
                trace!("No transport layer: proto={}", protocol);
                (0, 0, 0)
            }
        };

        let payload_len = Self::calculate_payload_len(data, &packet);
        let total_len = data.len() as u16;

        Ok(Some(ParsedPacket {
            src_ip,
            dst_ip,
            src_port,
            dst_port,
            protocol,
            tcp_flags,
            payload_len,
            total_len,
            timestamp,
            is_fragment,
            fragment_offset,
            more_fragments,
            vlan_id,
            ttl,
            tos,
            fragment_id,
        }))
    }

    fn calculate_payload_len(data: &[u8], packet: &SlicedPacket) -> u16 {
        let mut header_len = 0usize;

        header_len += 14;

        if packet.vlan.is_some() {
            header_len += 4;
        }

        match &packet.net {
            Some(NetSlice::Ipv4(ip)) => {
                header_len += (ip.header().ihl() as usize) * 4;
            }
            Some(NetSlice::Ipv6(ip)) => {
                header_len += 40;

                header_len += ip.extensions().slice().len();
            }
            None => {}
        }

        match &packet.transport {
            Some(TransportSlice::Tcp(tcp)) => {
                header_len += tcp.header_len() as usize;
            }
            Some(TransportSlice::Udp(_)) => {
                header_len += 8;
            }
            Some(TransportSlice::Icmpv4(_)) | Some(TransportSlice::Icmpv6(_)) => {
                header_len += 8;
            }
            None => {}
        }

        data.len().saturating_sub(header_len) as u16
    }

    pub fn parse_detailed(data: &[u8], timestamp: u64) -> ParseResult {
        if data.len() < 14 {
            return ParseResult::Skip(SkipReason::TooShort);
        }

        match Self::parse(data, timestamp) {
            Ok(Some(pkt)) => ParseResult::Ok(pkt),
            Ok(None) => ParseResult::Skip(SkipReason::NotIp),
            Err(e) => ParseResult::Error(ParseError::Other(e.to_string())),
        }
    }

    fn extract_vlan_id(packet: &SlicedPacket) -> Option<u16> {
        use etherparse::VlanSlice;
        match &packet.vlan {
            Some(VlanSlice::SingleVlan(s)) => Some(s.vlan_identifier().value()),
            Some(VlanSlice::DoubleVlan(d)) => Some(d.outer().vlan_identifier().value()),
            None => None,
        }
    }

    fn extract_tcp_flags(tcp: &etherparse::TcpSlice) -> u8 {
        let mut flags = 0u8;
        if tcp.fin() {
            flags |= tcp_flags::FIN;
        }
        if tcp.syn() {
            flags |= tcp_flags::SYN;
        }
        if tcp.rst() {
            flags |= tcp_flags::RST;
        }
        if tcp.psh() {
            flags |= tcp_flags::PSH;
        }
        if tcp.ack() {
            flags |= tcp_flags::ACK;
        }
        if tcp.urg() {
            flags |= tcp_flags::URG;
        }
        if tcp.ece() {
            flags |= tcp_flags::ECE;
        }
        if tcp.cwr() {
            flags |= tcp_flags::CWR;
        }
        flags
    }

    pub fn is_tcp_state_change(flags: u8) -> bool {
        (flags & tcp_flags::SYN) != 0
            || (flags & tcp_flags::FIN) != 0
            || (flags & tcp_flags::RST) != 0
    }

    pub fn is_tcp_handshake(flags: u8) -> bool {
        flags == tcp_flags::SYN || flags == (tcp_flags::SYN | tcp_flags::ACK)
    }

    pub fn is_tcp_data(flags: u8) -> bool {
        (flags & tcp_flags::PSH) != 0 || (flags & tcp_flags::ACK) != 0
    }

    pub fn is_ip_packet(data: &[u8]) -> bool {
        if data.len() < 14 {
            return false;
        }

        let ethertype = u16::from_be_bytes([data[12], data[13]]);
        matches!(ethertype, 0x0800 | 0x86DD | 0x8100)
    }

    pub fn quick_five_tuple(data: &[u8]) -> Option<(IpAddr, IpAddr, u16, u16, u8)> {
        if data.len() < 34 {
            return None;
        }

        let ethertype = u16::from_be_bytes([data[12], data[13]]);

        let ip_offset = if ethertype == 0x8100 { 18 } else { 14 };

        if data.len() < ip_offset + 20 {
            return None;
        }

        let ip_version = (data[ip_offset] >> 4) & 0x0F;

        match ip_version {
            4 => {
                let protocol = data[ip_offset + 9];
                let src_ip = IpAddr::V4(Ipv4Addr::new(
                    data[ip_offset + 12],
                    data[ip_offset + 13],
                    data[ip_offset + 14],
                    data[ip_offset + 15],
                ));
                let dst_ip = IpAddr::V4(Ipv4Addr::new(
                    data[ip_offset + 16],
                    data[ip_offset + 17],
                    data[ip_offset + 18],
                    data[ip_offset + 19],
                ));

                let ihl = (data[ip_offset] & 0x0F) as usize * 4;
                let transport_offset = ip_offset + ihl;

                if data.len() < transport_offset + 4 {
                    return Some((src_ip, dst_ip, 0, 0, protocol));
                }

                let src_port =
                    u16::from_be_bytes([data[transport_offset], data[transport_offset + 1]]);
                let dst_port =
                    u16::from_be_bytes([data[transport_offset + 2], data[transport_offset + 3]]);

                Some((src_ip, dst_ip, src_port, dst_port, protocol))
            }
            6 => {
                if data.len() < ip_offset + 40 {
                    return None;
                }

                let protocol = data[ip_offset + 6];

                let mut src_bytes = [0u8; 16];
                let mut dst_bytes = [0u8; 16];
                src_bytes.copy_from_slice(&data[ip_offset + 8..ip_offset + 24]);
                dst_bytes.copy_from_slice(&data[ip_offset + 24..ip_offset + 40]);

                let src_ip = IpAddr::V6(Ipv6Addr::from(src_bytes));
                let dst_ip = IpAddr::V6(Ipv6Addr::from(dst_bytes));

                let transport_offset = ip_offset + 40;

                if data.len() < transport_offset + 4 {
                    return Some((src_ip, dst_ip, 0, 0, protocol));
                }

                let src_port =
                    u16::from_be_bytes([data[transport_offset], data[transport_offset + 1]]);
                let dst_port =
                    u16::from_be_bytes([data[transport_offset + 2], data[transport_offset + 3]]);

                Some((src_ip, dst_ip, src_port, dst_port, protocol))
            }
            _ => None,
        }
    }

    pub fn quick_tos(data: &[u8]) -> Option<u8> {
        if data.len() < 15 {
            return None;
        }

        let ethertype = u16::from_be_bytes([data[12], data[13]]);

        let ip_offset = if ethertype == 0x8100 { 18 } else { 14 };

        if data.len() < ip_offset + 2 {
            return None;
        }

        let ip_version = (data[ip_offset] >> 4) & 0x0F;

        match ip_version {
            4 => Some(data[ip_offset + 1]),
            6 => {
                let tc = ((data[ip_offset] & 0x0F) << 4) | ((data[ip_offset + 1] >> 4) & 0x0F);
                Some(tc)
            }
            _ => None,
        }
    }
}

use std::sync::atomic::{AtomicU64, Ordering as AtomicOrdering};

// ============================================================================
// PassiveAssetDiscovery: 被动资产发现协调器
// 集成 DNS/DHCP/ARP 解析器，从流量中被动发现 MAC↔IP↔Hostname 绑定
// ============================================================================
use crate::parser::arp::{ArpParser, MacAddr};
use crate::parser::dhcp::DhcpParser;
use crate::parser::dns::DnsParser;
use std::sync::Mutex;

/// 被动资产发现事件
#[derive(Debug, Clone)]
pub enum AssetDiscoveryEvent {
    /// MAC→IP 绑定 (来自 ARP)
    ArpBinding {
        mac: String,
        ip: String,
        is_gateway: bool,
        timestamp: u64,
    },
    /// MAC→IP→Hostname 绑定 (来自 DHCP)
    DhcpLease {
        mac: String,
        ip: String,
        hostname: Option<String>,
        os_type: Option<String>,
        vlan_id: Option<u16>,
    },
    /// IP↔域名映射 (来自 DNS)
    DnsMapping {
        ip: String,
        domain: String,
        is_internal: bool,
        rr_type: String,
    },
    /// DNS 隧道告警
    DnsTunnelAlert {
        domain: String,
        entropy: f64,
        length: usize,
    },
    /// ARP 欺骗告警
    ArpSpoofAlert {
        mac: String,
        ip: String,
        alert_type: String,
        description: String,
    },
    /// DHCP 耗尽告警
    DhcpExhaustionAlert {
        subnet: String,
        active_leases: usize,
    },
}

/// 被动资产发现统计
#[derive(Debug, Default)]
pub struct DiscoveryStats {
    pub arp_packets: AtomicU64,
    pub dhcp_packets: AtomicU64,
    pub dns_packets: AtomicU64,
    pub arp_bindings: AtomicU64,
    pub dhcp_leases: AtomicU64,
    pub dns_mappings: AtomicU64,
    pub alerts_generated: AtomicU64,
}

/// 被动资产发现协调器
pub struct PassiveAssetDiscovery {
    pub arp: Mutex<ArpParser>,
    pub dhcp: Mutex<DhcpParser>,
    pub dns: Mutex<DnsParser>,
    pub stats: DiscoveryStats,
    /// 最近发现的资产绑定事件 (环形缓冲区)
    pub events: Mutex<Vec<AssetDiscoveryEvent>>,
    max_events: usize,
}

impl PassiveAssetDiscovery {
    pub fn new() -> Self {
        Self {
            arp: Mutex::new(ArpParser::new()),
            dhcp: Mutex::new(DhcpParser::new()),
            dns: Mutex::new(DnsParser::new()),
            stats: DiscoveryStats::default(),
            events: Mutex::new(Vec::with_capacity(256)),
            max_events: 256,
        }
    }

    /// 处理数据包，根据协议判断是否触发被动发现
    pub fn process_packet(
        &self,
        data: &[u8],
        src_port: u16,
        dst_port: u16,
        protocol: u8,
        timestamp: u64,
    ) {
        if data.len() < 20 {
            return;
        }

        // DNS: UDP port 53 (需要完整 4 参数调用)
        if protocol == protocols::UDP && (src_port == 53 || dst_port == 53) {
            self.stats.dns_packets.fetch_add(1, AtomicOrdering::Relaxed);
            if let Ok(mut parser) = self.dns.lock() {
                let fake_ip = IpAddr::V4(Ipv4Addr::UNSPECIFIED);
                if let Some(record) = parser.process_dns_packet(data, fake_ip, fake_ip, timestamp) {
                    self.stats
                        .dns_mappings
                        .fetch_add(1, AtomicOrdering::Relaxed);
                    // 检测 DNS 隧道
                    if parser.detect_dns_tunnel(&record) {
                        self.stats
                            .alerts_generated
                            .fetch_add(1, AtomicOrdering::Relaxed);
                        if let Ok(mut events) = self.events.lock() {
                            if events.len() >= self.max_events {
                                events.remove(0);
                            }
                            events.push(AssetDiscoveryEvent::DnsTunnelAlert {
                                domain: record.query_name.clone(),
                                entropy: 0.0, // computed internally in detect_dns_tunnel
                                length: record.query_name.len(),
                            });
                        }
                    }
                    // 记录 IP↔域名映射
                    for ip in &record.resolved_ips {
                        if let Ok(mut events) = self.events.lock() {
                            if events.len() >= self.max_events {
                                events.remove(0);
                            }
                            events.push(AssetDiscoveryEvent::DnsMapping {
                                ip: ip.to_string(),
                                domain: record.query_name.clone(),
                                is_internal: record.is_internal,
                                rr_type: format!("{:?}", record.query_type),
                            });
                        }
                    }
                }
            }
            return;
        }

        // DHCP: UDP port 67 (server) or 68 (client)
        if protocol == protocols::UDP
            && (src_port == 67 || dst_port == 67 || src_port == 68 || dst_port == 68)
        {
            self.stats
                .dhcp_packets
                .fetch_add(1, AtomicOrdering::Relaxed);
            if let Ok(mut parser) = self.dhcp.lock() {
                if let Some(lease) = parser.parse_dhcp_packet(data, MacAddr::default(), timestamp) {
                    self.stats.dhcp_leases.fetch_add(1, AtomicOrdering::Relaxed);
                    let os_type = parser
                        .identify_os(lease.vendor_class.as_deref().unwrap_or(""))
                        .map(|f| f.os_name.clone());
                    if let Ok(mut events) = self.events.lock() {
                        if events.len() >= self.max_events {
                            events.remove(0);
                        }
                        events.push(AssetDiscoveryEvent::DhcpLease {
                            mac: crate::parser::arp::mac_to_string(&lease.mac_address),
                            ip: lease.assigned_ip.to_string(),
                            hostname: lease.hostname.clone(),
                            os_type,
                            vlan_id: None, // DHCP relay info not available in current parser
                        });
                    }
                    // 检测 DHCP 耗尽攻击
                    if parser.detect_dhcp_exhaustion() {
                        self.stats
                            .alerts_generated
                            .fetch_add(1, AtomicOrdering::Relaxed);
                        let active = parser.active_leases().len();
                        if let Ok(mut events) = self.events.lock() {
                            if events.len() >= self.max_events {
                                events.remove(0);
                            }
                            events.push(AssetDiscoveryEvent::DhcpExhaustionAlert {
                                subnet: "unknown".to_string(),
                                active_leases: active,
                            });
                        }
                    }
                }
            }
            return;
        }
    }

    /// 处理 ARP 数据包 (需要从以太网层判断 ethertype=0x0806)
    pub fn process_arp_packet(&self, data: &[u8], timestamp: u64) {
        if data.len() < 28 {
            return;
        }
        // 检查以太网帧类型是否为 ARP (0x0806)
        let ethertype = u16::from_be_bytes([data[12], data[13]]);
        if ethertype != 0x0806 {
            return;
        }
        self.stats.arp_packets.fetch_add(1, AtomicOrdering::Relaxed);
        if let Ok(mut parser) = self.arp.lock() {
            let binding = parser.parse_arp_packet(data, None, None, timestamp);
            if let Some(b) = binding {
                self.stats
                    .arp_bindings
                    .fetch_add(1, AtomicOrdering::Relaxed);
                // 检测 ARP 欺骗
                let spoof_alerts = parser.recent_spoof_alerts(5).to_vec();
                for alert in &spoof_alerts {
                    self.stats
                        .alerts_generated
                        .fetch_add(1, AtomicOrdering::Relaxed);
                    if let Ok(mut events) = self.events.lock() {
                        if events.len() >= self.max_events {
                            events.remove(0);
                        }
                        events.push(AssetDiscoveryEvent::ArpSpoofAlert {
                            mac: crate::parser::arp::mac_to_string(&alert.spoofed_mac),
                            ip: alert.ip.to_string(),
                            alert_type: format!("{:?}", alert.alert_type),
                            description: format!(
                                "original_mac={}",
                                crate::parser::arp::mac_to_string(&alert.original_mac)
                            ),
                        });
                    }
                }
                if spoof_alerts.is_empty() {
                    if let Ok(mut events) = self.events.lock() {
                        if events.len() >= self.max_events {
                            events.remove(0);
                        }
                        events.push(AssetDiscoveryEvent::ArpBinding {
                            mac: crate::parser::arp::mac_to_string(&b.mac),
                            ip: b.ip.to_string(),
                            is_gateway: b.is_gratuitous,
                            timestamp: b.timestamp_ms,
                        });
                    }
                }
            }
        }
    }

    /// 获取最近的发现事件
    pub fn recent_events(&self, count: usize) -> Vec<AssetDiscoveryEvent> {
        if let Ok(events) = self.events.lock() {
            let start = if events.len() > count {
                events.len() - count
            } else {
                0
            };
            events[start..].to_vec()
        } else {
            vec![]
        }
    }

    /// 获取统计信息
    pub fn get_stats(&self) -> (u64, u64, u64, u64, u64, u64, u64) {
        (
            self.stats.arp_packets.load(AtomicOrdering::Relaxed),
            self.stats.dhcp_packets.load(AtomicOrdering::Relaxed),
            self.stats.dns_packets.load(AtomicOrdering::Relaxed),
            self.stats.arp_bindings.load(AtomicOrdering::Relaxed),
            self.stats.dhcp_leases.load(AtomicOrdering::Relaxed),
            self.stats.dns_mappings.load(AtomicOrdering::Relaxed),
            self.stats.alerts_generated.load(AtomicOrdering::Relaxed),
        )
    }
}

impl Default for PassiveAssetDiscovery {
    fn default() -> Self {
        Self::new()
    }
}

pub struct ParserStats {
    pub total: AtomicU64,
    pub success: AtomicU64,
    pub tcp: AtomicU64,
    pub udp: AtomicU64,
    pub icmp: AtomicU64,
    pub fragments: AtomicU64,
    pub vlan: AtomicU64,
}

impl Default for ParserStats {
    fn default() -> Self {
        Self {
            total: AtomicU64::new(0),
            success: AtomicU64::new(0),
            tcp: AtomicU64::new(0),
            udp: AtomicU64::new(0),
            icmp: AtomicU64::new(0),
            fragments: AtomicU64::new(0),
            vlan: AtomicU64::new(0),
        }
    }
}

impl ParserStats {
    pub fn record(&self, packet: &ParsedPacket) {
        self.total.fetch_add(1, AtomicOrdering::Relaxed);
        self.success.fetch_add(1, AtomicOrdering::Relaxed);

        match packet.protocol {
            protocols::TCP => self.tcp.fetch_add(1, AtomicOrdering::Relaxed),
            protocols::UDP => self.udp.fetch_add(1, AtomicOrdering::Relaxed),
            protocols::ICMP | protocols::ICMPV6 => self.icmp.fetch_add(1, AtomicOrdering::Relaxed),
            _ => 0,
        };

        if packet.is_fragment {
            self.fragments.fetch_add(1, AtomicOrdering::Relaxed);
        }

        if packet.vlan_id.is_some() {
            self.vlan.fetch_add(1, AtomicOrdering::Relaxed);
        }
    }

    pub fn get_stats(&self) -> (u64, u64, u64, u64, u64, u64, u64) {
        (
            self.total.load(AtomicOrdering::Relaxed),
            self.success.load(AtomicOrdering::Relaxed),
            self.tcp.load(AtomicOrdering::Relaxed),
            self.udp.load(AtomicOrdering::Relaxed),
            self.icmp.load(AtomicOrdering::Relaxed),
            self.fragments.load(AtomicOrdering::Relaxed),
            self.vlan.load(AtomicOrdering::Relaxed),
        )
    }
}
