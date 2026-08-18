use crate::aggregator::{
    canonicalize_observation, FlowIdentityError, FlowKey, ObservationScope, ObservedEndpoints,
    PacketInfo, ScopePolicy, FLOW_IDENTITY_REVISION, OBSERVATION_SCOPE_REVISION,
};
use crate::capture::CaptureTimestamp;
use anyhow::Result;
use etherparse::{NetSlice, SlicedPacket, TransportSlice};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use tracing::{debug, trace, warn};

pub mod arp;
pub mod dhcp;
pub mod dns;
pub mod security;

#[derive(Debug, thiserror::Error, Clone, PartialEq, Eq)]
pub enum ApplicationDecodeError {
    #[error("{protocol} payload is truncated at {stage}: required={required} actual={actual}")]
    Truncated {
        protocol: &'static str,
        stage: &'static str,
        required: usize,
        actual: usize,
    },
    #[error("malformed {protocol} payload: {reason}")]
    Malformed {
        protocol: &'static str,
        reason: &'static str,
    },
    #[error("unsupported {protocol} value {value} at {stage}")]
    Unsupported {
        protocol: &'static str,
        stage: &'static str,
        value: u32,
    },
}

impl ApplicationDecodeError {
    pub fn is_truncated(&self) -> bool {
        matches!(self, Self::Truncated { .. })
    }

    pub fn is_unsupported(&self) -> bool {
        matches!(self, Self::Unsupported { .. })
    }
}

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

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct FlowFields {
    pub src_ip: IpAddr,
    pub dst_ip: IpAddr,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
    pub tcp_flags: u8,
    pub total_len: u16,
    pub vlan_stack: Vec<u16>,
    pub ttl: u8,
    pub tos: u8,
    pub is_fragment: bool,
    pub fragment_id: Option<u32>,
    pub transport_status: TransportDecodeStatus,
    pub application_payload_offset: usize,
    pub application_payload_end: usize,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TransportDecodeStatus {
    Decoded,
    FirstFragment,
    Unsupported,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FlowDecodeError {
    #[error("ethernet frame is shorter than 14 bytes")]
    TruncatedEthernet,
    #[error("truncated VLAN header at depth {depth}")]
    TruncatedVlan { depth: usize },
    #[error("more than two VLAN tags are not supported")]
    TooManyVlanTags,
    #[error("unsupported EtherType 0x{0:04x}")]
    UnsupportedEtherType(u16),
    #[error("malformed IPv4 header: {0}")]
    MalformedIpv4(&'static str),
    #[error("malformed IPv6 header or extension chain: {0}")]
    MalformedIpv6(&'static str),
    #[error("non-first IP fragment requires reassembly (protocol={protocol}, id={fragment_id})")]
    NonFirstFragment { protocol: u8, fragment_id: u32 },
    #[error("transport header is truncated for protocol {protocol}")]
    TruncatedTransport { protocol: u8 },
    #[error("malformed transport header for protocol {protocol}: {reason}")]
    MalformedTransport { protocol: u8, reason: &'static str },
    #[error("frame length exceeds the FlowEvent uint16 carrier")]
    FrameTooLarge,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FastFallback {
    QinQ,
    Ipv4Options,
    Ipv6Extension,
    Fragment,
    Protocol,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FastDecodeError {
    #[error("fast parser fallback required: {0:?}")]
    Fallback(FastFallback),
    #[error(transparent)]
    Invalid(#[from] FlowDecodeError),
}

#[derive(Debug)]
pub struct FlowSample {
    pub key: FlowKey,
    pub packet: PacketInfo,
    pub fields: FlowFields,
    pub identity_revision: u8,
    pub observation_scope_revision: u8,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FlowSampleError {
    #[error(transparent)]
    Identity(#[from] FlowIdentityError),
    #[error("capture timestamp is outside the event-time carrier")]
    InvalidTimestamp,
    #[error("observation scope revision {actual} does not match {expected}")]
    ScopeRevision { actual: u8, expected: u8 },
}

pub struct FlowSampleBuilder;

impl FlowSampleBuilder {
    pub fn build(
        fields: FlowFields,
        captured_at: CaptureTimestamp,
        scope: &ObservationScope,
    ) -> Result<FlowSample, FlowSampleError> {
        let timestamp = captured_at.epoch_micros();
        if timestamp < 1_000 {
            return Err(FlowSampleError::InvalidTimestamp);
        }
        if scope.revision != OBSERVATION_SCOPE_REVISION {
            return Err(FlowSampleError::ScopeRevision {
                actual: scope.revision,
                expected: OBSERVATION_SCOPE_REVISION,
            });
        }
        let identity = canonicalize_observation(ObservedEndpoints {
            src_ip: fields.src_ip,
            dst_ip: fields.dst_ip,
            src_port: fields.src_port,
            dst_port: fields.dst_port,
            protocol: fields.protocol,
        })?;
        // Interface is capture configuration, while VLAN/QinQ is observed on
        // each frame. Only policies that explicitly include VLAN may let the
        // parsed tag stack affect aggregation; Community ID remains based on
        // the canonical L3/L4 tuple.
        let mut effective_scope = scope.clone();
        effective_scope.vlan_stack = match scope.policy {
            ScopePolicy::InterfaceAndVlan | ScopePolicy::FullObservation => {
                fields.vlan_stack.clone()
            }
            ScopePolicy::GlobalL3 | ScopePolicy::Interface => Vec::new(),
        };
        let key = FlowKey::new(&identity, &effective_scope);
        let packet = PacketInfo {
            len: fields.total_len,
            tcp_flags: fields.tcp_flags,
            direction: identity.packet_direction,
            timestamp,
            tos: fields.tos,
        };
        Ok(FlowSample {
            key,
            packet,
            fields,
            identity_revision: FLOW_IDENTITY_REVISION,
            observation_scope_revision: OBSERVATION_SCOPE_REVISION,
        })
    }
}

struct DecodedNetwork {
    src_ip: IpAddr,
    dst_ip: IpAddr,
    protocol: u8,
    ttl: u8,
    tos: u8,
    transport_offset: usize,
    network_end: usize,
    is_fragment: bool,
    fragment_id: Option<u32>,
}

struct DecodedTransport {
    src_port: u16,
    dst_port: u16,
    tcp_flags: u8,
    application_payload_offset: usize,
    application_payload_end: usize,
    status: TransportDecodeStatus,
}

fn decode_ethernet(data: &[u8]) -> Result<(usize, u16, Vec<u16>), FlowDecodeError> {
    if data.len() < 14 {
        return Err(FlowDecodeError::TruncatedEthernet);
    }
    let mut cursor = 14;
    let mut ether_type = u16::from_be_bytes([data[12], data[13]]);
    let mut vlan_stack = Vec::new();
    while matches!(ether_type, 0x8100 | 0x88a8) {
        if vlan_stack.len() == 2 {
            return Err(FlowDecodeError::TooManyVlanTags);
        }
        if data.len() < cursor + 4 {
            return Err(FlowDecodeError::TruncatedVlan {
                depth: vlan_stack.len() + 1,
            });
        }
        let tag = u16::from_be_bytes([data[cursor], data[cursor + 1]]);
        vlan_stack.push(tag & 0x0fff);
        ether_type = u16::from_be_bytes([data[cursor + 2], data[cursor + 3]]);
        cursor += 4;
    }
    Ok((cursor, ether_type, vlan_stack))
}

fn decode_ipv4(data: &[u8], offset: usize) -> Result<DecodedNetwork, FlowDecodeError> {
    if data.len() < offset + 20 {
        return Err(FlowDecodeError::MalformedIpv4("truncated base header"));
    }
    if data[offset] >> 4 != 4 {
        return Err(FlowDecodeError::MalformedIpv4("invalid version"));
    }
    let header_len = usize::from(data[offset] & 0x0f) * 4;
    if header_len < 20 || data.len() < offset + header_len {
        return Err(FlowDecodeError::MalformedIpv4("invalid IHL"));
    }
    let declared_len = usize::from(u16::from_be_bytes([data[offset + 2], data[offset + 3]]));
    if declared_len < header_len || data.len() < offset + declared_len {
        return Err(FlowDecodeError::MalformedIpv4("invalid total length"));
    }
    let protocol = data[offset + 9];
    let fragment_bits = u16::from_be_bytes([data[offset + 6], data[offset + 7]]);
    let fragment_offset = fragment_bits & 0x1fff;
    let more_fragments = fragment_bits & 0x2000 != 0;
    let fragment_id = u16::from_be_bytes([data[offset + 4], data[offset + 5]]) as u32;
    if fragment_offset != 0 {
        return Err(FlowDecodeError::NonFirstFragment {
            protocol,
            fragment_id,
        });
    }
    Ok(DecodedNetwork {
        src_ip: IpAddr::V4(Ipv4Addr::new(
            data[offset + 12],
            data[offset + 13],
            data[offset + 14],
            data[offset + 15],
        )),
        dst_ip: IpAddr::V4(Ipv4Addr::new(
            data[offset + 16],
            data[offset + 17],
            data[offset + 18],
            data[offset + 19],
        )),
        protocol,
        ttl: data[offset + 8],
        tos: data[offset + 1],
        transport_offset: offset + header_len,
        network_end: offset + declared_len,
        is_fragment: more_fragments,
        fragment_id: more_fragments.then_some(fragment_id),
    })
}

fn decode_ipv6(data: &[u8], offset: usize) -> Result<DecodedNetwork, FlowDecodeError> {
    if data.len() < offset + 40 {
        return Err(FlowDecodeError::MalformedIpv6("truncated base header"));
    }
    if data[offset] >> 4 != 6 {
        return Err(FlowDecodeError::MalformedIpv6("invalid version"));
    }
    let payload_len = usize::from(u16::from_be_bytes([data[offset + 4], data[offset + 5]]));
    if data.len() < offset + 40 + payload_len {
        return Err(FlowDecodeError::MalformedIpv6("invalid payload length"));
    }
    let src_ip = {
        let mut octets = [0; 16];
        octets.copy_from_slice(&data[offset + 8..offset + 24]);
        IpAddr::V6(Ipv6Addr::from(octets))
    };
    let dst_ip = {
        let mut octets = [0; 16];
        octets.copy_from_slice(&data[offset + 24..offset + 40]);
        IpAddr::V6(Ipv6Addr::from(octets))
    };
    let traffic_class = ((data[offset] & 0x0f) << 4) | (data[offset + 1] >> 4);
    let mut next_header = data[offset + 6];
    let mut cursor = offset + 40;
    let payload_end = offset + 40 + payload_len;
    let mut is_fragment = false;
    let mut fragment_id = None;
    let mut extension_count = 0usize;

    loop {
        if extension_count >= 8 {
            return Err(FlowDecodeError::MalformedIpv6(
                "extension chain exceeds 8 headers",
            ));
        }
        match next_header {
            0 | 43 | 60 => {
                extension_count += 1;
                if cursor + 2 > payload_end {
                    return Err(FlowDecodeError::MalformedIpv6("truncated extension header"));
                }
                let length = (usize::from(data[cursor + 1]) + 1) * 8;
                if length < 8 || cursor + length > payload_end {
                    return Err(FlowDecodeError::MalformedIpv6("invalid extension length"));
                }
                next_header = data[cursor];
                cursor += length;
            }
            51 => {
                extension_count += 1;
                if cursor + 2 > payload_end {
                    return Err(FlowDecodeError::MalformedIpv6(
                        "truncated authentication header",
                    ));
                }
                let length = (usize::from(data[cursor + 1]) + 2) * 4;
                if length < 8 || cursor + length > payload_end {
                    return Err(FlowDecodeError::MalformedIpv6(
                        "invalid authentication length",
                    ));
                }
                next_header = data[cursor];
                cursor += length;
            }
            44 => {
                extension_count += 1;
                if cursor + 8 > payload_end {
                    return Err(FlowDecodeError::MalformedIpv6("truncated fragment header"));
                }
                let fragment_bits = u16::from_be_bytes([data[cursor + 2], data[cursor + 3]]);
                let fragment_offset = (fragment_bits & 0xfff8) >> 3;
                let more_fragments = fragment_bits & 1 != 0;
                let id = u32::from_be_bytes([
                    data[cursor + 4],
                    data[cursor + 5],
                    data[cursor + 6],
                    data[cursor + 7],
                ]);
                if fragment_offset != 0 {
                    return Err(FlowDecodeError::NonFirstFragment {
                        protocol: data[cursor],
                        fragment_id: id,
                    });
                }
                is_fragment = more_fragments;
                fragment_id = more_fragments.then_some(id);
                next_header = data[cursor];
                cursor += 8;
            }
            // IPv6 No Next Header is a valid terminal value. Preserve it as
            // an unsupported L4 protocol instead of reporting truncation.
            59 => break,
            _ => break,
        }
    }

    Ok(DecodedNetwork {
        src_ip,
        dst_ip,
        protocol: next_header,
        ttl: data[offset + 7],
        tos: traffic_class,
        transport_offset: cursor,
        network_end: payload_end,
        is_fragment,
        fragment_id,
    })
}

fn decode_transport(
    data: &[u8],
    offset: usize,
    network_end: usize,
    protocol: u8,
    is_fragment: bool,
) -> Result<DecodedTransport, FlowDecodeError> {
    let needed = match protocol {
        6 => 20,
        17 => 8,
        1 | 58 => 4,
        _ => 0,
    };
    if needed == 0 {
        return Ok(DecodedTransport {
            src_port: 0,
            dst_port: 0,
            tcp_flags: 0,
            application_payload_offset: offset,
            application_payload_end: network_end,
            status: TransportDecodeStatus::Unsupported,
        });
    }
    if network_end > data.len() || offset > network_end || network_end - offset < needed {
        return Err(FlowDecodeError::TruncatedTransport { protocol });
    }
    match protocol {
        6 => {
            let header_len = usize::from(data[offset + 12] >> 4) * 4;
            if header_len < 20 || offset + header_len > network_end {
                return Err(FlowDecodeError::MalformedTransport {
                    protocol,
                    reason: "invalid TCP data offset",
                });
            }
            Ok(DecodedTransport {
                src_port: u16::from_be_bytes([data[offset], data[offset + 1]]),
                dst_port: u16::from_be_bytes([data[offset + 2], data[offset + 3]]),
                tcp_flags: data[offset + 13],
                application_payload_offset: offset + header_len,
                application_payload_end: network_end,
                status: if is_fragment {
                    TransportDecodeStatus::FirstFragment
                } else {
                    TransportDecodeStatus::Decoded
                },
            })
        }
        17 => {
            let udp_len = usize::from(u16::from_be_bytes([data[offset + 4], data[offset + 5]]));
            if udp_len < 8 {
                return Err(FlowDecodeError::MalformedTransport {
                    protocol,
                    reason: "UDP length is shorter than the header",
                });
            }
            if !is_fragment && offset + udp_len > network_end {
                return Err(FlowDecodeError::MalformedTransport {
                    protocol,
                    reason: "UDP length exceeds the IP payload",
                });
            }
            Ok(DecodedTransport {
                src_port: u16::from_be_bytes([data[offset], data[offset + 1]]),
                dst_port: u16::from_be_bytes([data[offset + 2], data[offset + 3]]),
                tcp_flags: 0,
                application_payload_offset: offset + 8,
                application_payload_end: if is_fragment {
                    network_end
                } else {
                    offset + udp_len
                },
                status: if is_fragment {
                    TransportDecodeStatus::FirstFragment
                } else {
                    TransportDecodeStatus::Decoded
                },
            })
        }
        1 | 58 => Ok(DecodedTransport {
            src_port: data[offset] as u16,
            dst_port: data[offset + 1] as u16,
            tcp_flags: 0,
            application_payload_offset: offset + 4,
            application_payload_end: network_end,
            status: if is_fragment {
                TransportDecodeStatus::FirstFragment
            } else {
                TransportDecodeStatus::Decoded
            },
        }),
        _ => Err(FlowDecodeError::MalformedTransport {
            protocol,
            reason: "unsupported transport protocol",
        }),
    }
}

pub struct PacketParser;

impl PacketParser {
    pub fn parse(data: &[u8], timestamp: u64) -> Result<Option<ParsedPacket>> {
        match Self::decode_flow_fields(data) {
            Ok(fields) => Ok(Some(ParsedPacket {
                src_ip: fields.src_ip,
                dst_ip: fields.dst_ip,
                src_port: fields.src_port,
                dst_port: fields.dst_port,
                protocol: fields.protocol,
                tcp_flags: fields.tcp_flags,
                payload_len: 0,
                total_len: fields.total_len,
                timestamp,
                is_fragment: fields.is_fragment,
                fragment_offset: 0,
                more_fragments: fields.is_fragment,
                vlan_id: fields.vlan_stack.first().copied(),
                ttl: fields.ttl,
                tos: fields.tos,
                fragment_id: fields.fragment_id,
            })),
            // `parse` is the legacy compatibility adapter. Historically a
            // frame that could not produce an IP flow was a skip, not a hard
            // error. The production flow route calls `decode_flow_fields`
            // directly and therefore retains its typed fail-closed errors.
            Err(
                FlowDecodeError::TruncatedEthernet
                | FlowDecodeError::TruncatedVlan { .. }
                | FlowDecodeError::TooManyVlanTags
                | FlowDecodeError::UnsupportedEtherType(_)
                | FlowDecodeError::MalformedIpv4(_)
                | FlowDecodeError::MalformedIpv6(_)
                | FlowDecodeError::NonFirstFragment { .. }
                | FlowDecodeError::TruncatedTransport { .. }
                | FlowDecodeError::MalformedTransport { .. },
            ) => Ok(None),
            Err(error) => Err(anyhow::anyhow!(error)),
        }
    }

    /// Checked L2-L4 decoder shared by the full path and fast-path eligibility
    /// adapter. It accepts single VLAN and QinQ, follows the supported IPv6
    /// extension chain, and rejects non-first fragments before any zero-port
    /// flow can be fabricated.
    pub fn decode_flow_fields(data: &[u8]) -> Result<FlowFields, FlowDecodeError> {
        let total_len = u16::try_from(data.len()).map_err(|_| FlowDecodeError::FrameTooLarge)?;
        let (mut cursor, ether_type, vlan_stack) = decode_ethernet(data)?;

        let network = match ether_type {
            0x0800 => decode_ipv4(data, cursor)?,
            0x86dd => decode_ipv6(data, cursor)?,
            other => return Err(FlowDecodeError::UnsupportedEtherType(other)),
        };
        cursor = network.transport_offset;

        let transport = decode_transport(
            data,
            cursor,
            network.network_end,
            network.protocol,
            network.is_fragment,
        )?;
        Ok(FlowFields {
            src_ip: network.src_ip,
            dst_ip: network.dst_ip,
            src_port: transport.src_port,
            dst_port: transport.dst_port,
            protocol: network.protocol,
            tcp_flags: transport.tcp_flags,
            total_len,
            vlan_stack,
            ttl: network.ttl,
            tos: network.tos,
            is_fragment: network.is_fragment,
            fragment_id: network.fragment_id,
            transport_status: transport.status,
            application_payload_offset: transport.application_payload_offset,
            application_payload_end: transport.application_payload_end,
        })
    }

    /// Fast parser is deliberately a proven subset. Every other frame must be
    /// handed to `decode_flow_fields`; it never guesses around extensions or
    /// fragments.
    pub fn decode_flow_fields_fast(data: &[u8]) -> Result<FlowFields, FastDecodeError> {
        let (ip_offset, ether_type, vlan_stack) =
            decode_ethernet(data).map_err(FastDecodeError::Invalid)?;
        if vlan_stack.len() > 1 {
            return Err(FastDecodeError::Fallback(FastFallback::QinQ));
        }
        match ether_type {
            0x0800 => {
                if data.len() < ip_offset + 20 {
                    return Err(FastDecodeError::Invalid(FlowDecodeError::MalformedIpv4(
                        "truncated base header",
                    )));
                }
                if data[ip_offset] & 0x0f != 5 {
                    return Err(FastDecodeError::Fallback(FastFallback::Ipv4Options));
                }
                let fragment_bits = u16::from_be_bytes([data[ip_offset + 6], data[ip_offset + 7]]);
                if fragment_bits & 0x3fff != 0 {
                    return Err(FastDecodeError::Fallback(FastFallback::Fragment));
                }
                if !matches!(data[ip_offset + 9], 6 | 17) {
                    return Err(FastDecodeError::Fallback(FastFallback::Protocol));
                }
            }
            0x86dd => {
                if data.len() < ip_offset + 40 {
                    return Err(FastDecodeError::Invalid(FlowDecodeError::MalformedIpv6(
                        "truncated base header",
                    )));
                }
                if !matches!(data[ip_offset + 6], 6 | 17) {
                    return Err(FastDecodeError::Fallback(FastFallback::Ipv6Extension));
                }
            }
            other => {
                return Err(FastDecodeError::Invalid(
                    FlowDecodeError::UnsupportedEtherType(other),
                ));
            }
        }
        Self::decode_flow_fields(data).map_err(FastDecodeError::Invalid)
    }

    #[allow(dead_code)]
    fn parse_legacy(data: &[u8], timestamp: u64) -> Result<Option<ParsedPacket>> {
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
        let fields = Self::decode_flow_fields_fast(data).ok()?;
        Some((
            fields.src_ip,
            fields.dst_ip,
            fields.src_port,
            fields.dst_port,
            fields.protocol,
        ))
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
use crate::parser::arp::ArpParser;
use crate::parser::dhcp::{DhcpMessageType, DhcpParser};
use crate::parser::dns::DnsParser;
use std::sync::Mutex;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AssetBindingObservation {
    pub mac: String,
    pub ip: String,
    pub observed_at_ms: i64,
    pub source: &'static str,
    pub vlan_id: Option<u16>,
    pub source_event_id: String,
}

pub trait AssetBindingSink: Send + Sync {
    fn persist(&self, observation: AssetBindingObservation) -> Result<()>;
}

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
    pub truncated_payloads: AtomicU64,
    pub malformed_payloads: AtomicU64,
    pub unsupported_payloads: AtomicU64,
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
    binding_sink: Option<std::sync::Arc<dyn AssetBindingSink>>,
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
            binding_sink: None,
        }
    }

    pub fn with_binding_sink(mut self, sink: std::sync::Arc<dyn AssetBindingSink>) -> Self {
        self.binding_sink = Some(sink);
        self
    }

    fn persist_binding(&self, observation: AssetBindingObservation) {
        if let Some(sink) = &self.binding_sink {
            if let Err(error) = sink.persist(observation) {
                warn!(error = %error, "durable asset binding spool rejected observation");
            }
        }
    }

    fn record_application_error(&self, error: &ApplicationDecodeError) {
        if error.is_truncated() {
            self.stats
                .truncated_payloads
                .fetch_add(1, AtomicOrdering::Relaxed);
        } else if error.is_unsupported() {
            self.stats
                .unsupported_payloads
                .fetch_add(1, AtomicOrdering::Relaxed);
        } else {
            self.stats
                .malformed_payloads
                .fetch_add(1, AtomicOrdering::Relaxed);
        }
        trace!(error = %error, "Application discovery parser rejected payload");
    }

    /// Process an already validated flow frame. Application parsers receive
    /// only their L4 payload, never Ethernet or IP header bytes.
    pub fn process_flow_sample(&self, frame: &[u8], fields: &FlowFields, timestamp: u64) {
        if fields.application_payload_offset > fields.application_payload_end
            || fields.application_payload_end > frame.len()
            || fields.transport_status != TransportDecodeStatus::Decoded
        {
            return;
        }
        let data = &frame[fields.application_payload_offset..fields.application_payload_end];

        // DNS over TCP needs stream reassembly and is intentionally not
        // interpreted as a single packet here.
        if fields.protocol == protocols::UDP && (fields.src_port == 53 || fields.dst_port == 53) {
            self.stats.dns_packets.fetch_add(1, AtomicOrdering::Relaxed);
            if let Ok(mut parser) = self.dns.lock() {
                let (dns_server_ip, client_ip) = if fields.src_port == 53 {
                    (fields.src_ip, fields.dst_ip)
                } else {
                    (fields.dst_ip, fields.src_ip)
                };
                match parser.process_dns_packet_checked(data, dns_server_ip, client_ip, timestamp) {
                    Ok(record) => {
                        self.stats
                            .dns_mappings
                            .fetch_add(record.resolved_ips.len() as u64, AtomicOrdering::Relaxed);
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
                    Err(error) => self.record_application_error(&error),
                }
            }
            return;
        }

        // DHCP: UDP port 67 (server) or 68 (client)
        if fields.protocol == protocols::UDP
            && (fields.src_port == 67
                || fields.dst_port == 67
                || fields.src_port == 68
                || fields.dst_port == 68)
        {
            self.stats
                .dhcp_packets
                .fetch_add(1, AtomicOrdering::Relaxed);
            if let Ok(mut parser) = self.dhcp.lock() {
                let source_mac = frame
                    .get(6..12)
                    .and_then(|bytes| <[u8; 6]>::try_from(bytes).ok())
                    .unwrap_or_default();
                match parser.parse_dhcp_packet_checked(data, source_mac, timestamp) {
                    Ok(lease) => {
                        let has_bound_lease = matches!(
                            lease.message_type,
                            DhcpMessageType::Offer | DhcpMessageType::Ack
                        ) && lease.assigned_ip != Ipv4Addr::UNSPECIFIED;
                        if has_bound_lease {
                            self.stats.dhcp_leases.fetch_add(1, AtomicOrdering::Relaxed);
                        }
                        let os_type = parser
                            .identify_os(lease.vendor_class.as_deref().unwrap_or(""))
                            .map(|f| f.os_name.clone());
                        if has_bound_lease {
                            let mac = crate::parser::arp::mac_to_string(&lease.mac_address);
                            let vlan_id =
                                lease.relay_agent.as_ref().and_then(|relay| relay.vlan_id);
                            self.persist_binding(AssetBindingObservation {
                                mac: mac.clone(),
                                ip: lease.assigned_ip.to_string(),
                                observed_at_ms: (lease.timestamp_ms / 1_000) as i64,
                                source: "dhcp",
                                vlan_id,
                                source_event_id: format!("dhcp-xid:{:08x}", lease.transaction_id),
                            });
                            if let Ok(mut events) = self.events.lock() {
                                if events.len() >= self.max_events {
                                    events.remove(0);
                                }
                                events.push(AssetDiscoveryEvent::DhcpLease {
                                    mac,
                                    ip: lease.assigned_ip.to_string(),
                                    hostname: lease.hostname.clone(),
                                    os_type,
                                    vlan_id,
                                });
                            }
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
                    Err(error) => self.record_application_error(&error),
                }
            }
            return;
        }
    }

    /// 处理 ARP 数据包 (需要从以太网层判断 ethertype=0x0806)
    pub fn process_arp_packet(&self, data: &[u8], timestamp: u64) {
        if data.len() < 14 {
            return;
        }
        let (payload_offset, ethertype, vlan_stack) = match decode_ethernet(data) {
            Ok(decoded) => decoded,
            Err(_) => return,
        };
        if ethertype != 0x0806 {
            return;
        }
        let payload = match data.get(payload_offset..payload_offset + 28) {
            Some(payload) => payload,
            None => {
                self.record_application_error(&ApplicationDecodeError::Truncated {
                    protocol: "ARP",
                    stage: "payload",
                    required: payload_offset + 28,
                    actual: data.len(),
                });
                return;
            }
        };
        let capture_mac = data
            .get(6..12)
            .and_then(|bytes| <[u8; 6]>::try_from(bytes).ok());
        self.stats.arp_packets.fetch_add(1, AtomicOrdering::Relaxed);
        if let Ok(mut parser) = self.arp.lock() {
            let alert_count_before = parser.spoof_alerts.len();
            let binding = parser.parse_arp_packet_checked(
                payload,
                capture_mac,
                vlan_stack.first().copied(),
                timestamp,
            );
            if let Ok(b) = binding {
                self.stats
                    .arp_bindings
                    .fetch_add(1, AtomicOrdering::Relaxed);
                // 检测 ARP 欺骗
                let spoof_alerts = parser.spoof_alerts[alert_count_before..].to_vec();
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
                    let mac = crate::parser::arp::mac_to_string(&b.mac);
                    self.persist_binding(AssetBindingObservation {
                        mac: mac.clone(),
                        ip: b.ip.to_string(),
                        observed_at_ms: (b.timestamp_ms / 1_000) as i64,
                        source: "arp",
                        vlan_id: b.vlan_id,
                        source_event_id: format!("arp:{:?}", b.operation),
                    });
                    if let Ok(mut events) = self.events.lock() {
                        if events.len() >= self.max_events {
                            events.remove(0);
                        }
                        events.push(AssetDiscoveryEvent::ArpBinding {
                            mac,
                            ip: b.ip.to_string(),
                            is_gateway: b.is_gratuitous,
                            timestamp: b.timestamp_ms,
                        });
                    }
                }
            } else if let Err(error) = binding {
                self.record_application_error(&error);
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

    /// Keep parser rejection counters separate from valid protocol traffic.
    pub fn get_decode_error_stats(&self) -> (u64, u64, u64) {
        (
            self.stats.truncated_payloads.load(AtomicOrdering::Relaxed),
            self.stats.malformed_payloads.load(AtomicOrdering::Relaxed),
            self.stats
                .unsupported_payloads
                .load(AtomicOrdering::Relaxed),
        )
    }
}

impl Default for PassiveAssetDiscovery {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod discovery_tests {
    use super::*;

    #[derive(Default)]
    struct RecordingBindingSink {
        observations: Mutex<Vec<AssetBindingObservation>>,
    }

    impl AssetBindingSink for RecordingBindingSink {
        fn persist(&self, observation: AssetBindingObservation) -> Result<()> {
            self.observations.lock().unwrap().push(observation);
            Ok(())
        }
    }

    fn ethernet(ether_type: u16, payload: &[u8]) -> Vec<u8> {
        let mut frame = vec![0; 12];
        frame[6..12].copy_from_slice(&[0x00, 0x1a, 0xc5, 1, 2, 3]);
        frame.extend_from_slice(&ether_type.to_be_bytes());
        frame.extend_from_slice(payload);
        frame
    }

    fn ipv4_udp(src_port: u16, dst_port: u16, payload: &[u8]) -> Vec<u8> {
        let total_len = 20 + 8 + payload.len();
        let mut packet = vec![0; total_len];
        packet[0] = 0x45;
        packet[2..4].copy_from_slice(&(total_len as u16).to_be_bytes());
        packet[8] = 64;
        packet[9] = 17;
        packet[12..16].copy_from_slice(&[192, 0, 2, 20]);
        packet[16..20].copy_from_slice(&[8, 8, 8, 8]);
        packet[20..22].copy_from_slice(&src_port.to_be_bytes());
        packet[22..24].copy_from_slice(&dst_port.to_be_bytes());
        packet[24..26].copy_from_slice(&((8 + payload.len()) as u16).to_be_bytes());
        packet[28..].copy_from_slice(payload);
        ethernet(0x0800, &packet)
    }

    fn dns_a_response() -> Vec<u8> {
        let mut packet = vec![
            0x12, 0x34, 0x81, 0x80, 0, 1, 0, 1, 0, 0, 0, 0, 3, b'w', b'w', b'w', 7, b'e', b'x',
            b'a', b'm', b'p', b'l', b'e', 3, b'c', b'o', b'm', 0, 0, 1, 0, 1,
        ];
        packet.extend_from_slice(&[0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 10]);
        packet
    }

    fn dhcp_discover() -> Vec<u8> {
        let mut packet = vec![0; 244];
        packet[0] = 1;
        packet[1] = 1;
        packet[2] = 6;
        packet[4..8].copy_from_slice(&0x1020_3040u32.to_be_bytes());
        packet[28..34].copy_from_slice(&[0x00, 0x1a, 0xc5, 1, 2, 3]);
        packet[236..240].copy_from_slice(&[99, 130, 83, 99]);
        packet[240..244].copy_from_slice(&[53, 1, 1, 255]);
        packet
    }

    fn dhcp_ack() -> Vec<u8> {
        let mut packet = dhcp_discover();
        packet[16..20].copy_from_slice(&[192, 168, 1, 20]);
        packet[242] = 5;
        packet
    }

    #[test]
    fn raw_dns_frame_uses_udp_payload_and_real_endpoints() {
        let discovery = PassiveAssetDiscovery::new();
        let frame = ipv4_udp(53000, 53, &dns_a_response());
        let fields = PacketParser::decode_flow_fields(&frame).unwrap();
        discovery.process_flow_sample(&frame, &fields, 1_000);

        assert_eq!(discovery.stats.dns_packets.load(AtomicOrdering::Relaxed), 1);
        assert_eq!(
            discovery.stats.dns_mappings.load(AtomicOrdering::Relaxed),
            1
        );
        let parser = discovery.dns.lock().unwrap();
        assert_eq!(
            parser.assets.hostname_to_ip.get("www.example.com"),
            Some(&vec!["192.0.2.10".parse::<IpAddr>().unwrap()])
        );
    }

    #[test]
    fn raw_dhcp_discover_does_not_fabricate_a_bound_lease() {
        let discovery = PassiveAssetDiscovery::new();
        let frame = ipv4_udp(68, 67, &dhcp_discover());
        let fields = PacketParser::decode_flow_fields(&frame).unwrap();
        discovery.process_flow_sample(&frame, &fields, 1_000);

        assert_eq!(
            discovery.stats.dhcp_packets.load(AtomicOrdering::Relaxed),
            1
        );
        assert_eq!(discovery.stats.dhcp_leases.load(AtomicOrdering::Relaxed), 0);
        let parser = discovery.dhcp.lock().unwrap();
        assert!(parser.leases.is_empty());
        assert!(parser.ip_to_mac.is_empty());
        assert_eq!(parser.lease_stats.total_discoveries, 1);
        drop(parser);
        assert!(discovery.recent_events(10).is_empty());
    }

    #[test]
    fn raw_dhcp_ack_persists_a_durable_binding_observation() {
        let sink = std::sync::Arc::new(RecordingBindingSink::default());
        let discovery = PassiveAssetDiscovery::new().with_binding_sink(sink.clone());
        let frame = ipv4_udp(67, 68, &dhcp_ack());
        let fields = PacketParser::decode_flow_fields(&frame).unwrap();
        discovery.process_flow_sample(&frame, &fields, 2_000_000);

        let observations = sink.observations.lock().unwrap();
        assert_eq!(observations.len(), 1);
        assert_eq!(observations[0].source, "dhcp");
        assert_eq!(observations[0].ip, "192.168.1.20");
        assert_eq!(observations[0].observed_at_ms, 2_000);
    }

    #[test]
    fn raw_vlan_arp_frame_is_decoded_and_truncation_is_counted() {
        let sink = std::sync::Arc::new(RecordingBindingSink::default());
        let discovery = PassiveAssetDiscovery::new().with_binding_sink(sink.clone());
        let mut arp = vec![0; 28];
        arp[0..2].copy_from_slice(&1u16.to_be_bytes());
        arp[2..4].copy_from_slice(&0x0800u16.to_be_bytes());
        arp[4] = 6;
        arp[5] = 4;
        arp[6..8].copy_from_slice(&1u16.to_be_bytes());
        arp[8..14].copy_from_slice(&[0, 0x1a, 0xc5, 1, 2, 3]);
        arp[14..18].copy_from_slice(&[192, 168, 1, 10]);
        arp[24..28].copy_from_slice(&[192, 168, 1, 1]);
        let mut vlan_payload = Vec::new();
        vlan_payload.extend_from_slice(&100u16.to_be_bytes());
        vlan_payload.extend_from_slice(&0x0806u16.to_be_bytes());
        vlan_payload.extend_from_slice(&arp);
        discovery.process_arp_packet(&ethernet(0x8100, &vlan_payload), 1_000);
        assert_eq!(
            discovery.stats.arp_bindings.load(AtomicOrdering::Relaxed),
            1
        );
        assert_eq!(discovery.arp.lock().unwrap().ip_to_mac.len(), 1);
        let observations = sink.observations.lock().unwrap();
        assert_eq!(observations.len(), 1);
        assert_eq!(observations[0].source, "arp");
        assert_eq!(observations[0].vlan_id, Some(100));
        assert_eq!(observations[0].observed_at_ms, 1);
        drop(observations);

        discovery.process_arp_packet(&ethernet(0x0806, &[0; 10]), 2_000);
        assert_eq!(
            discovery
                .stats
                .truncated_payloads
                .load(AtomicOrdering::Relaxed),
            1
        );
    }

    #[test]
    fn truncated_application_corpus_rejects_without_state_mutation() {
        let dns_packet = dns_a_response();
        for end in 0..dns_packet.len() {
            let mut parser = DnsParser::new();
            assert!(parser
                .process_dns_packet_checked(
                    &dns_packet[..end],
                    "8.8.8.8".parse().unwrap(),
                    "192.0.2.20".parse().unwrap(),
                    1_000,
                )
                .is_err());
            assert!(parser.assets.hostname_to_ip.is_empty());
        }

        let dhcp_packet = dhcp_discover();
        for end in 0..dhcp_packet.len() {
            let mut parser = DhcpParser::new();
            assert!(parser
                .parse_dhcp_packet_checked(&dhcp_packet[..end], [0x00, 0x1a, 0xc5, 1, 2, 3], 1_000,)
                .is_err());
            assert!(parser.leases.is_empty());
            assert!(parser.ip_to_mac.is_empty());
            assert!(parser.mac_to_hostname.is_empty());
        }

        let arp_packet = vec![0; 28];
        for end in 0..arp_packet.len() {
            let mut parser = ArpParser::new();
            assert!(parser
                .parse_arp_packet_checked(&arp_packet[..end], None, None, 1_000)
                .is_err());
            assert!(parser.ip_to_mac.is_empty());
            assert!(parser.mac_to_ip.is_empty());
        }
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
