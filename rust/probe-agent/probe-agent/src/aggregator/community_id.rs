use sha1::{Sha1, Digest};
use base64::{Engine as _, engine::general_purpose};
use std::net::IpAddr;

/// 计算 community_id (v1 标准)
/// 
/// 算法：
/// 1. 规范化五元组（IP 小的在前）
/// 2. 拼接 seed(0) + proto + ip_low + ip_high + port_low + port_high
/// 3. SHA1 哈希
/// 4. Base64 编码
/// 5. 前缀 "1:" 表示版本
pub fn compute_community_id(
    src_ip: IpAddr,
    dst_ip: IpAddr,
    src_port: u16,
    dst_port: u16,
    protocol: u8,
) -> String {
    // 1. 规范化（确保小的 IP 在前）
    let (ip_a, port_a, ip_b, port_b) = if src_ip < dst_ip 
        || (src_ip == dst_ip && src_port < dst_port) {
        (src_ip, src_port, dst_ip, dst_port)
    } else {
        (dst_ip, dst_port, src_ip, src_port)
    };

    // 2. 构建哈希输入
    let mut hasher = Sha1::new();
    
    // Seed (2 bytes, big-endian, value=0)
    hasher.update(&[0u8, 0u8]);
    
    // Protocol
    hasher.update(&[protocol]);
    
    // IP addresses (网络字节序)
    match ip_a {
        IpAddr::V4(addr) => hasher.update(&addr.octets()),
        IpAddr::V6(addr) => hasher.update(&addr.octets()),
    }
    match ip_b {
        IpAddr::V4(addr) => hasher.update(&addr.octets()),
        IpAddr::V6(addr) => hasher.update(&addr.octets()),
    }
    
    // Ports (big-endian)
    hasher.update(&port_a.to_be_bytes());
    hasher.update(&port_b.to_be_bytes());
    
    // 3. 计算哈希
    let hash = hasher.finalize();
    
    // 4. Base64 编码
    let b64 = general_purpose::STANDARD.encode(&hash);
    
    // 5. 添加版本前缀
    format!("1:{}", b64)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::net::Ipv4Addr;

    #[test]
    fn test_community_id_bidirectional() {
        let ip1 = IpAddr::V4(Ipv4Addr::new(192, 168, 1, 100));
        let ip2 = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1));
        
        let id1 = compute_community_id(ip1, ip2, 12345, 80, 6);
        let id2 = compute_community_id(ip2, ip1, 80, 12345, 6);
        
        assert_eq!(id1, id2, "Bidirectional flows must have same community_id");
    }

    #[test]
    fn test_community_id_format() {
        let ip1 = IpAddr::V4(Ipv4Addr::new(192, 168, 1, 100));
        let ip2 = IpAddr::V4(Ipv4Addr::new(10, 0, 0, 1));
        
        let id = compute_community_id(ip1, ip2, 12345, 80, 6);
        
        assert!(id.starts_with("1:"), "Community ID must start with '1:'");
        assert!(id.len() > 10, "Community ID must have reasonable length");
    }
}