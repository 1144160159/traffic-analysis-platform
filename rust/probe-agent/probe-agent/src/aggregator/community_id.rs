use base64::{engine::general_purpose, Engine as _};
use sha1::{Digest, Sha1};
use std::net::IpAddr;

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
/// 与 Java Flink CommunityIdUtil 输出一致。
pub fn compute_community_id(
    src_ip: IpAddr,
    dst_ip: IpAddr,
    src_port: u16,
    dst_port: u16,
    protocol: u8,
) -> String {
    let (ip_a, port_a, ip_b, port_b) = normalize(src_ip, src_port, dst_ip, dst_port, protocol);

    let mut hasher = Sha1::new();

    // 1. Seed (2 bytes, big-endian, 0x0000)
    hasher.update([0u8, 0u8]);

    // 2-3. IP addresses
    update_ip(&mut hasher, ip_a);
    update_ip(&mut hasher, ip_b);

    // 4. Protocol (1 byte)
    hasher.update([protocol]);

    // 5. Padding (1 byte, 0x00)
    hasher.update([0u8]);

    // 6-7. Ports (2 bytes each, big-endian)
    hasher.update(port_a.to_be_bytes());
    hasher.update(port_b.to_be_bytes());

    let hash = hasher.finalize();
    let b64 = general_purpose::STANDARD.encode(&hash);

    format!("1:{}", b64)
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
    use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

    #[test]
    fn test_community_id_tcp() {
        let cid = compute_community_id(
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
        let cid = compute_community_id(
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
        let cid = compute_community_id(
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
        let cid = compute_community_id(
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
        let cid = compute_community_id(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            80,
            443,
            6,
        );
        assert_eq!(cid, "1:ha3Gdb1ffmoH0pMumBteoAOl/+U=");
    }

    #[test]
    fn test_community_id_deterministic() {
        let a = compute_community_id(
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1)),
            IpAddr::V4(Ipv4Addr::new(10, 0, 0, 2)),
            80,
            443,
            6,
        );
        let b = compute_community_id(
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
