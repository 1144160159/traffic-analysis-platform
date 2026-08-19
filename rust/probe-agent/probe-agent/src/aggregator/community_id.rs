use super::flow_table::CommunityTuple;
use base64::{engine::general_purpose, Engine as _};
use sha1::{Digest, Sha1};
use std::net::IpAddr;

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct CommunityId(String);

impl CommunityId {
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

/// Community ID v1.0 标准实现
/// https://github.com/corelight/community-id-spec
///
/// 字节顺序:
///   1. Seed (2 bytes, big-endian, 0x0000)
///   2. IP 1 (4 or 16 bytes)
///   3. IP 2 (4 or 16 bytes)
///   4. Protocol (1 byte)
///   5. Padding (1 byte, 0x00)
///   6. Port 1 (2 bytes, big-endian)
///   7. Port 2 (2 bytes, big-endian)
///
/// TCP/UDP/IPv6 向量与 Java Flink CommunityIdUtil 输出一致。ICMP(protocol
/// 1/58)按 community-id-spec 编码:type 与 code 打包进单个 16-bit 端口值
/// `(type << 8) | code`,两个哈希端口均取该值;请求/应答对的 type 已在
/// `canonicalize_observation` 中归一化为请求侧 type,因此往返两个方向哈希
/// 一致。Java 侧 ICMP 映射对齐列为后续工作,当前以本实现为基准。
pub fn compute_community_id(tuple: &CommunityTuple) -> CommunityId {
    let mut hasher = Sha1::new();

    // 1. Seed (2 bytes, big-endian, 0x0000)
    hasher.update([0u8, 0u8]);

    // 2-3. IP addresses
    update_ip(&mut hasher, tuple.ip_a);
    update_ip(&mut hasher, tuple.ip_b);

    // 4. Protocol (1 byte)
    hasher.update([tuple.protocol]);

    // 5. Padding (1 byte, 0x00)
    hasher.update([0u8]);

    // 6-7. Ports (2 bytes each, big-endian)
    if matches!(tuple.protocol, 1 | 58) {
        // ICMP: a single 16-bit value `(type << 8) | code` for both ports.
        // `port_a` carries the (already request-normalized) ICMP type and
        // `port_b` carries the ICMP code.
        let port = ((tuple.port_a as u16 & 0xff) << 8) | (tuple.port_b as u16 & 0xff);
        hasher.update(port.to_be_bytes());
        hasher.update(port.to_be_bytes());
    } else {
        hasher.update(tuple.port_a.to_be_bytes());
        hasher.update(tuple.port_b.to_be_bytes());
    }

    let hash = hasher.finalize();
    let b64 = general_purpose::STANDARD.encode(&hash);

    CommunityId(format!("1:{}", b64))
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct IcmpPair {
    pub request_type: u8,
    pub is_request: bool,
}

pub fn icmp_pair(protocol: u8, icmp_type: u8) -> Option<IcmpPair> {
    let mappings = match protocol {
        1 => icmp_mapping::ICMPV4_REQUEST_TO_REPLY,
        58 => icmp_mapping::ICMPV6_REQUEST_TO_REPLY,
        _ => return None,
    };
    mappings.iter().find_map(|&(request, reply)| {
        if icmp_type == request {
            Some(IcmpPair {
                request_type: request,
                is_request: true,
            })
        } else if icmp_type == reply {
            Some(IcmpPair {
                request_type: request,
                is_request: false,
            })
        } else {
            None
        }
    })
}

pub fn icmp_request_type(protocol: u8, icmp_type: u8) -> Option<u8> {
    icmp_pair(protocol, icmp_type).map(|pair| pair.request_type)
}

fn update_ip(hasher: &mut Sha1, ip: IpAddr) {
    match ip {
        IpAddr::V4(addr) => hasher.update(addr.octets()),
        IpAddr::V6(addr) => hasher.update(addr.octets()),
    }
}

mod icmp_mapping {
    pub const ICMPV4_REQUEST_TO_REPLY: &[(u8, u8)] = &[(8, 0), (13, 14), (15, 16), (17, 18)];
    pub const ICMPV6_REQUEST_TO_REPLY: &[(u8, u8)] =
        &[(128, 129), (130, 131), (133, 134), (135, 136)];
}

pub mod icmpv4_types {
    pub const ECHO_REPLY: u8 = 0;
    pub const DEST_UNREACH: u8 = 3;
    pub const SOURCE_QUENCH: u8 = 4;
    pub const REDIRECT: u8 = 5;
    pub const ECHO_REQUEST: u8 = 8;
    pub const ROUTER_ADVERT: u8 = 9;
    pub const ROUTER_SOLICIT: u8 = 10;
    pub const TIME_EXCEEDED: u8 = 11;
    pub const PARAM_PROBLEM: u8 = 12;
    pub const TIMESTAMP_REQUEST: u8 = 13;
    pub const TIMESTAMP_REPLY: u8 = 14;
    pub const INFO_REQUEST: u8 = 15;
    pub const INFO_REPLY: u8 = 16;
    pub const ADDR_MASK_REQUEST: u8 = 17;
    pub const ADDR_MASK_REPLY: u8 = 18;

    pub fn is_reply(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REPLY | TIMESTAMP_REPLY | INFO_REPLY | ADDR_MASK_REPLY
        )
    }

    pub fn is_request(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REQUEST | TIMESTAMP_REQUEST | INFO_REQUEST | ADDR_MASK_REQUEST
        )
    }

    pub fn is_error_type(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            DEST_UNREACH | SOURCE_QUENCH | REDIRECT | TIME_EXCEEDED | PARAM_PROBLEM
        )
    }

    pub fn to_request_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REPLY => Some(ECHO_REQUEST),
            TIMESTAMP_REPLY => Some(TIMESTAMP_REQUEST),
            INFO_REPLY => Some(INFO_REQUEST),
            ADDR_MASK_REPLY => Some(ADDR_MASK_REQUEST),
            _ => None,
        }
    }

    pub fn to_reply_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REQUEST => Some(ECHO_REPLY),
            TIMESTAMP_REQUEST => Some(TIMESTAMP_REPLY),
            INFO_REQUEST => Some(INFO_REPLY),
            ADDR_MASK_REQUEST => Some(ADDR_MASK_REPLY),
            _ => None,
        }
    }

    pub fn type_name(icmp_type: u8) -> &'static str {
        match icmp_type {
            ECHO_REPLY => "Echo Reply",
            DEST_UNREACH => "Destination Unreachable",
            SOURCE_QUENCH => "Source Quench",
            REDIRECT => "Redirect",
            ECHO_REQUEST => "Echo Request",
            ROUTER_ADVERT => "Router Advertisement",
            ROUTER_SOLICIT => "Router Solicitation",
            TIME_EXCEEDED => "Time Exceeded",
            PARAM_PROBLEM => "Parameter Problem",
            TIMESTAMP_REQUEST => "Timestamp Request",
            TIMESTAMP_REPLY => "Timestamp Reply",
            INFO_REQUEST => "Information Request",
            INFO_REPLY => "Information Reply",
            ADDR_MASK_REQUEST => "Address Mask Request",
            ADDR_MASK_REPLY => "Address Mask Reply",
            _ => "Unknown",
        }
    }
}

fn normalize(
    src_ip: IpAddr,
    src_port: u16,
    dst_ip: IpAddr,
    dst_port: u16,
    protocol: u8,
) -> (IpAddr, u16, IpAddr, u16) {
    if protocol == 1 {
        return normalize_icmpv4(src_ip, src_port, dst_ip, dst_port);
    } else if protocol == 58 {
        return normalize_icmpv6(src_ip, src_port, dst_ip, dst_port);
    }

    if src_ip < dst_ip || (src_ip == dst_ip && src_port <= dst_port) {
        (src_ip, src_port, dst_ip, dst_port)
    } else {
        (dst_ip, dst_port, src_ip, src_port)
    }
}

fn normalize_icmpv4(
    src_ip: IpAddr,
    icmp_type: u16,
    dst_ip: IpAddr,
    icmp_code: u16,
) -> (IpAddr, u16, IpAddr, u16) {
    let type_u8 = icmp_type as u8;

    for &(reply_type, request_type) in icmp_mapping::ICMPV4_REPLY_TO_REQUEST {
        if type_u8 == reply_type {
            if dst_ip < src_ip {
                return (dst_ip, request_type as u16, src_ip, icmp_code);
            } else {
                return (src_ip, request_type as u16, dst_ip, icmp_code);
            }
        }
    }

    for &(request_type, _reply_type) in icmp_mapping::ICMPV4_REQUEST_TO_REPLY {
        if type_u8 == request_type {
            if src_ip < dst_ip {
                return (src_ip, request_type as u16, dst_ip, icmp_code);
            } else {
                return (dst_ip, request_type as u16, src_ip, icmp_code);
            }
        }
    }

    if icmp_mapping::is_error_type_v4(type_u8) {
        if src_ip < dst_ip {
            (src_ip, icmp_type, dst_ip, icmp_code)
        } else {
            (dst_ip, icmp_code, src_ip, icmp_type)
        }
    } else {
        if src_ip < dst_ip {
            (src_ip, icmp_type, dst_ip, icmp_code)
        } else {
            (dst_ip, icmp_code, src_ip, icmp_type)
        }
    }
}

fn normalize_icmpv6(
    src_ip: IpAddr,
    icmp_type: u16,
    dst_ip: IpAddr,
    icmp_code: u16,
) -> (IpAddr, u16, IpAddr, u16) {
    let type_u8 = icmp_type as u8;

    for &(reply_type, request_type) in icmp_mapping::ICMPV6_REPLY_TO_REQUEST {
        if type_u8 == reply_type {
            if dst_ip < src_ip {
                return (dst_ip, request_type as u16, src_ip, icmp_code);
            } else {
                return (src_ip, request_type as u16, dst_ip, icmp_code);
            }
        }
    }

    for &(request_type, _reply_type) in icmp_mapping::ICMPV6_REQUEST_TO_REPLY {
        if type_u8 == request_type {
            if src_ip < dst_ip {
                return (src_ip, request_type as u16, dst_ip, icmp_code);
            } else {
                return (dst_ip, request_type as u16, src_ip, icmp_code);
            }
        }
    }

    if icmp_mapping::is_error_type_v6(type_u8) {
        if src_ip < dst_ip {
            (src_ip, icmp_type, dst_ip, icmp_code)
        } else {
            (dst_ip, icmp_code, src_ip, icmp_type)
        }
    } else {
        if src_ip < dst_ip {
            (src_ip, icmp_type, dst_ip, icmp_code)
        } else {
            (dst_ip, icmp_code, src_ip, icmp_type)
        }
    }
}

fn update_ip(hasher: &mut Sha1, ip: IpAddr) {
    match ip {
        IpAddr::V4(addr) => hasher.update(addr.octets()),
        IpAddr::V6(addr) => hasher.update(addr.octets()),
    }
}

pub fn is_forward(src_ip: IpAddr, src_port: u16, dst_ip: IpAddr, dst_port: u16) -> bool {
    src_ip < dst_ip || (src_ip == dst_ip && src_port <= dst_port)
}

mod icmp_mapping {
    pub const ICMPV4_REPLY_TO_REQUEST: &[(u8, u8)] = &[(0, 8), (14, 13), (16, 15), (18, 17)];

    pub const ICMPV4_REQUEST_TO_REPLY: &[(u8, u8)] = &[(8, 0), (13, 14), (15, 16), (17, 18)];

    pub const ICMPV6_REPLY_TO_REQUEST: &[(u8, u8)] = &[
        (129, 128),
        (131, 130),
        (132, 130),
        (134, 133),
        (136, 135),
        (143, 130),
    ];

    pub const ICMPV6_REQUEST_TO_REPLY: &[(u8, u8)] =
        &[(128, 129), (130, 131), (133, 134), (135, 136)];

    pub fn is_error_type_v4(icmp_type: u8) -> bool {
        matches!(icmp_type, 3 | 4 | 5 | 11 | 12)
    }

    pub fn is_error_type_v6(icmp_type: u8) -> bool {
        matches!(icmp_type, 1 | 2 | 3 | 4)
    }
}

pub mod icmpv4_types {
    pub const ECHO_REPLY: u8 = 0;
    pub const DEST_UNREACH: u8 = 3;
    pub const SOURCE_QUENCH: u8 = 4;
    pub const REDIRECT: u8 = 5;
    pub const ECHO_REQUEST: u8 = 8;
    pub const ROUTER_ADVERT: u8 = 9;
    pub const ROUTER_SOLICIT: u8 = 10;
    pub const TIME_EXCEEDED: u8 = 11;
    pub const PARAM_PROBLEM: u8 = 12;
    pub const TIMESTAMP_REQUEST: u8 = 13;
    pub const TIMESTAMP_REPLY: u8 = 14;
    pub const INFO_REQUEST: u8 = 15;
    pub const INFO_REPLY: u8 = 16;
    pub const ADDR_MASK_REQUEST: u8 = 17;
    pub const ADDR_MASK_REPLY: u8 = 18;

    pub fn is_reply(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REPLY | TIMESTAMP_REPLY | INFO_REPLY | ADDR_MASK_REPLY
        )
    }

    pub fn is_request(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REQUEST | TIMESTAMP_REQUEST | INFO_REQUEST | ADDR_MASK_REQUEST
        )
    }

    pub fn is_error_type(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            DEST_UNREACH | SOURCE_QUENCH | REDIRECT | TIME_EXCEEDED | PARAM_PROBLEM
        )
    }

    pub fn to_request_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REPLY => Some(ECHO_REQUEST),
            TIMESTAMP_REPLY => Some(TIMESTAMP_REQUEST),
            INFO_REPLY => Some(INFO_REQUEST),
            ADDR_MASK_REPLY => Some(ADDR_MASK_REQUEST),
            _ => None,
        }
    }

    pub fn to_reply_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REQUEST => Some(ECHO_REPLY),
            TIMESTAMP_REQUEST => Some(TIMESTAMP_REPLY),
            INFO_REQUEST => Some(INFO_REPLY),
            ADDR_MASK_REQUEST => Some(ADDR_MASK_REPLY),
            _ => None,
        }
    }

    pub fn type_name(icmp_type: u8) -> &'static str {
        match icmp_type {
            ECHO_REPLY => "Echo Reply",
            DEST_UNREACH => "Destination Unreachable",
            SOURCE_QUENCH => "Source Quench",
            REDIRECT => "Redirect",
            ECHO_REQUEST => "Echo Request",
            ROUTER_ADVERT => "Router Advertisement",
            ROUTER_SOLICIT => "Router Solicitation",
            TIME_EXCEEDED => "Time Exceeded",
            PARAM_PROBLEM => "Parameter Problem",
            TIMESTAMP_REQUEST => "Timestamp Request",
            TIMESTAMP_REPLY => "Timestamp Reply",
            INFO_REQUEST => "Information Request",
            INFO_REPLY => "Information Reply",
            ADDR_MASK_REQUEST => "Address Mask Request",
            ADDR_MASK_REPLY => "Address Mask Reply",
            _ => "Unknown",
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::aggregator::flow_table::{canonicalize_observation, ObservedEndpoints};
    use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

    fn cid(src_ip: IpAddr, dst_ip: IpAddr, src_port: u16, dst_port: u16, protocol: u8) -> String {
        let identity = canonicalize_observation(ObservedEndpoints {
            src_ip,
            dst_ip,
            src_port,
            dst_port,
            protocol,
        })
        .unwrap();
        compute_community_id(&identity.community_tuple)
            .as_str()
            .to_owned()
    }

    #[test]
    fn test_community_id_tcp() {
        let cid = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            12345,
            80,
            6,
        );
        assert_eq!(cid, "1:CpuULklTENbGdRpvp7gNcQd5ZqA=");
    }

    #[test]
    fn test_community_id_swapped() {
        let cid = cid(
            IpAddr::V4(Ipv4Addr::new(192, 168, 1, 1)),
            IpAddr::V4(Ipv4Addr::new(192, 168, 1, 100)),
            443,
            54321,
            6,
        );
        assert_eq!(cid, "1:yvabNgZAlWzo8wcUZ6B9cSRJQ9Q=");
    }

    #[test]
    fn test_community_id_udp() {
        let cid = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            53,
            12345,
            17,
        );
        assert_eq!(cid, "1:JrhaqgS2mu6o+Lu2/yWyT0ECe6E=");
    }

    #[test]
    fn test_community_id_ipv6() {
        let cid = cid(
            IpAddr::V6(Ipv6Addr::new(0, 0, 0, 0, 0, 0, 0, 1)),
            IpAddr::V6(Ipv6Addr::new(0, 0, 0, 0, 0, 0, 0, 2)),
            8080,
            9090,
            6,
        );
        assert_eq!(cid, "1:/Q8HrtOQusOw7LFS4Ju3LeGLJu0=");
    }

    #[test]
    fn test_community_id_same_ip() {
        let cid = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            80,
            443,
            6,
        );
        assert_eq!(cid, "1:ha3Gdb1ffmoH0pMumBteoAOl/+U=");
    }

    #[test]
    fn test_community_id_icmp_spec_encoding() {
        // ICMP per community-id-spec: ports are a single 16-bit
        // `(request_type << 8) | code` value on both sides.
        // Echo request 10.0.0.1 -> 10.0.0.2 (type 8, code 0):
        let request = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            8,
            0,
            1,
        );
        // Echo reply 10.0.0.2 -> 10.0.0.1 (type 0, code 0) normalizes to the
        // request type, so the community ID must be identical.
        let reply = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            0,
            0,
            1,
        );
        assert_eq!(request, reply, "ICMP request/reply must share a community ID");
        assert_eq!(
            request,
            "1:T+cveHbOI4LUN7f4h/f3+I7F4uM=",
            "pinned ICMP echo vector (type<<8|code = 0x0800)"
        );
    }

    #[test]
    fn test_community_id_icmpv6_request_reply_symmetric() {
        // ICMPv6 echo request/reply (types 128/129) must share a community ID.
        let request = cid(
            IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 1)),
            IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 2)),
            128,
            0,
            58,
        );
        let reply = cid(
            IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 2)),
            IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 1)),
            129,
            0,
            58,
        );
        assert_eq!(request, reply, "ICMPv6 request/reply must share a community ID");
        assert_eq!(
            request,
            "1:yOIIKDvdmAbFOlh2kOEOMIfOa2c=",
            "pinned ICMPv6 echo vector (type<<8|code = 0x8000)"
        );
    }

    #[test]
    fn test_community_id_icmp_differs_from_raw_type_code_ports() {
        // Sanity: an unpaired ICMP error (type 3, code 1) encodes
        // (3 << 8) | 1 = 0x0301, not two independent 16-bit fields.
        let error = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            3,
            1,
            1,
        );
        let legacy_style = compute_legacy_icmp_cid_for_test(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            3,
            1,
        );
        assert_ne!(error, legacy_style, "ICMP must use the packed type<<8|code port");
    }

    /// Recomputes the old (buggy) ICMP encoding where type and code were
    /// hashed as two independent 16-bit fields, to prove the fixed encoding
    /// diverges from it.
    fn compute_legacy_icmp_cid_for_test(src_ip: IpAddr, dst_ip: IpAddr, icmp_type: u8, code: u8) -> String {
        use sha1::{Digest, Sha1};
        use base64::{engine::general_purpose, Engine as _};
        let mut hasher = Sha1::new();
        hasher.update([0u8, 0u8]);
        update_ip(&mut hasher, src_ip);
        update_ip(&mut hasher, dst_ip);
        hasher.update([1u8]);
        hasher.update([0u8]);
        hasher.update((icmp_type as u16).to_be_bytes());
        hasher.update((code as u16).to_be_bytes());
        let b64 = general_purpose::STANDARD.encode(hasher.finalize());
        format!("1:{}", b64)
    }

    #[test]
    fn test_community_id_deterministic() {
        let a = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            80,
            443,
            6,
        );
        let b = cid(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            443,
            80,
            6,
        );
        assert_eq!(a, b, "Community ID must be symmetric");
    }
}

pub mod icmpv6_types {
    pub const DEST_UNREACH: u8 = 1;
    pub const PACKET_TOO_BIG: u8 = 2;
    pub const TIME_EXCEEDED: u8 = 3;
    pub const PARAM_PROBLEM: u8 = 4;
    pub const ECHO_REQUEST: u8 = 128;
    pub const ECHO_REPLY: u8 = 129;
    pub const MLD_QUERY: u8 = 130;
    pub const MLD_REPORT: u8 = 131;
    pub const MLD_DONE: u8 = 132;
    pub const ROUTER_SOLICIT: u8 = 133;
    pub const ROUTER_ADVERT: u8 = 134;
    pub const NEIGHBOR_SOLICIT: u8 = 135;
    pub const NEIGHBOR_ADVERT: u8 = 136;
    pub const REDIRECT: u8 = 137;
    pub const MLD2_REPORT: u8 = 143;

    pub fn is_reply(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REPLY | MLD_REPORT | MLD2_REPORT | ROUTER_ADVERT | NEIGHBOR_ADVERT
        )
    }

    pub fn is_request(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            ECHO_REQUEST | MLD_QUERY | ROUTER_SOLICIT | NEIGHBOR_SOLICIT
        )
    }

    pub fn is_error_type(icmp_type: u8) -> bool {
        matches!(
            icmp_type,
            DEST_UNREACH | PACKET_TOO_BIG | TIME_EXCEEDED | PARAM_PROBLEM
        )
    }

    pub fn to_request_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REPLY => Some(ECHO_REQUEST),
            MLD_REPORT | MLD2_REPORT => Some(MLD_QUERY),
            ROUTER_ADVERT => Some(ROUTER_SOLICIT),
            NEIGHBOR_ADVERT => Some(NEIGHBOR_SOLICIT),
            _ => None,
        }
    }

    pub fn to_reply_type(icmp_type: u8) -> Option<u8> {
        match icmp_type {
            ECHO_REQUEST => Some(ECHO_REPLY),
            MLD_QUERY => Some(MLD_REPORT),
            ROUTER_SOLICIT => Some(ROUTER_ADVERT),
            NEIGHBOR_SOLICIT => Some(NEIGHBOR_ADVERT),
            _ => None,
        }
    }

    pub fn type_name(icmp_type: u8) -> &'static str {
        match icmp_type {
            DEST_UNREACH => "Destination Unreachable",
            PACKET_TOO_BIG => "Packet Too Big",
            TIME_EXCEEDED => "Time Exceeded",
            PARAM_PROBLEM => "Parameter Problem",
            ECHO_REQUEST => "Echo Request",
            ECHO_REPLY => "Echo Reply",
            MLD_QUERY => "MLD Query",
            MLD_REPORT => "MLD Report",
            MLD_DONE => "MLD Done",
            ROUTER_SOLICIT => "Router Solicitation",
            ROUTER_ADVERT => "Router Advertisement",
            NEIGHBOR_SOLICIT => "Neighbor Solicitation",
            NEIGHBOR_ADVERT => "Neighbor Advertisement",
            REDIRECT => "Redirect",
            MLD2_REPORT => "MLDv2 Report",
            _ => "Unknown",
        }
    }
}
