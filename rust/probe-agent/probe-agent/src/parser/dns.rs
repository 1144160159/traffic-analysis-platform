// DNS Passive Discovery Parser — 从DNS流量中被动发现资产信息
//
// 业务价值:
//   - 解析DNS查询/响应 → 建立域名↔IP映射
//   - 识别内部DNS服务器
//   - 检测DNS隧道 (高熵域名、异常长度)
//   - 识别CDN/云服务使用情况
//   - 被动资产清单: hostname → IP 映射

use std::collections::HashMap;
use std::net::IpAddr;

/// DNS 操作码
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DnsOpcode {
    Query = 0,
    IQuery = 1,
    Status = 2,
    Notify = 4,
    Update = 5,
}

/// DNS 响应码
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DnsRcode {
    NoError = 0,
    FormErr = 1,
    ServFail = 2,
    NXDomain = 3,
    NotImp = 4,
    Refused = 5,
    YXDomain = 6,
    YXRRSet = 7,
    NXRRSet = 8,
    NotAuth = 9,
    NotZone = 10,
}

/// DNS 资源记录类型
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DnsRRType {
    A = 1,
    NS = 2,
    CNAME = 5,
    SOA = 6,
    PTR = 12,
    MX = 15,
    TXT = 16,
    AAAA = 28,
    SRV = 33,
    ANY = 255,
}

/// DNS 解析记录 (被动发现的资产关联)
#[derive(Debug, Clone)]
pub struct DnsRecord {
    /// 查询的域名
    pub query_name: String,
    /// 查询类型
    pub query_type: DnsRRType,
    /// 响应的 IP 地址 (A/AAAA记录)
    pub resolved_ips: Vec<IpAddr>,
    /// CNAME 链
    pub cname_chain: Vec<String>,
    /// 权威DNS服务器 (NS记录)
    pub authoritative_servers: Vec<String>,
    /// 邮件服务器 (MX记录)
    pub mail_servers: Vec<(String, u16)>, // (hostname, priority)
    /// TTL (秒)
    pub ttl: u32,
    /// DNS 服务器 IP
    pub dns_server: IpAddr,
    /// 客户端 IP (查询发起方)
    pub client_ip: IpAddr,
    /// 响应码
    pub rcode: DnsRcode,
    /// 是否为内部DNS查询
    pub is_internal: bool,
    /// 响应数据包大小
    pub response_size: u16,
    /// 时间戳 (ms)
    pub timestamp_ms: u64,
}

/// DNS 查询统计 (用于异常检测)
#[derive(Debug, Clone, Default)]
pub struct DnsQueryStats {
    /// 唯一域名数
    pub unique_domains: usize,
    /// 总查询数
    pub total_queries: u64,
    /// 唯一查询类型数
    pub unique_types: usize,
    /// NXDomain 响应数 (可能指示DGA)
    pub nxdomain_count: u64,
    /// 平均域名长度
    pub avg_domain_length: f32,
    /// 最大域名长度
    pub max_domain_length: usize,
    /// 高熵域名数 (可能指示DNS隧道)
    pub high_entropy_domains: u64,
    /// AAAA/AAAA查询比例
    pub aaaa_ratio: f32,
}

/// 被动资产发现结果
#[derive(Debug, Clone)]
pub struct PassiveAssetDiscovery {
    /// hostname → IP 映射
    pub hostname_to_ip: HashMap<String, Vec<IpAddr>>,
    /// IP → hostname 映射
    pub ip_to_hostname: HashMap<IpAddr, Vec<String>>,
    /// 发现的内部DNS服务器
    pub dns_servers: Vec<IpAddr>,
    /// 发现的邮件服务器
    pub mail_servers: Vec<(String, IpAddr)>,
    /// CDN 域名
    pub cdn_domains: Vec<String>,
}

/// DNS 隧道检测器
pub struct DnsTunnelDetector {
    /// 域名长度阈值 (超过此值可疑)
    pub max_domain_length: usize,
    /// 域名熵阈值 (超过此值可疑)
    pub entropy_threshold: f32,
    /// 子域名数量阈值
    pub max_subdomain_count: usize,
    /// 单域名查询频率阈值 (次/分钟)
    pub query_rate_threshold: u64,
    /// 已知合法的高熵域名 (如 CDN)
    pub whitelist_domains: Vec<String>,
}

impl Default for DnsTunnelDetector {
    fn default() -> Self {
        Self {
            max_domain_length: 52,     // RFC 1035: 标签最大63, 全域名最大253
            entropy_threshold: 3.5,    // Shannon entropy > 3.5 可疑
            max_subdomain_count: 5,    // 子域名数 > 5 可疑
            query_rate_threshold: 100, // 同一域名 > 100次/分钟 可疑
            whitelist_domains: vec![
                "cloudfront.net".to_string(),
                "akamai.net".to_string(),
                "fastly.net".to_string(),
                "azureedge.net".to_string(),
                "cdn77.com".to_string(),
            ],
        }
    }
}

/// DNS 解析器
pub struct DnsParser {
    /// 被动资产发现
    pub assets: PassiveAssetDiscovery,
    /// 查询统计
    pub stats: DnsQueryStats,
    /// 隧道检测器
    pub tunnel_detector: DnsTunnelDetector,
    /// 内部域名后缀
    internal_suffixes: Vec<String>,
    /// 内部DNS服务器
    known_dns_servers: Vec<IpAddr>,
}

impl DnsParser {
    pub fn new() -> Self {
        Self {
            assets: PassiveAssetDiscovery {
                hostname_to_ip: HashMap::new(),
                ip_to_hostname: HashMap::new(),
                dns_servers: Vec::new(),
                mail_servers: Vec::new(),
                cdn_domains: Vec::new(),
            },
            stats: DnsQueryStats::default(),
            tunnel_detector: DnsTunnelDetector::default(),
            internal_suffixes: vec![
                ".local".to_string(),
                ".internal".to_string(),
                ".lan".to_string(),
                ".home".to_string(),
                ".corp".to_string(),
            ],
            known_dns_servers: Vec::new(),
        }
    }

    /// 设置内部域名后缀
    pub fn set_internal_suffixes(&mut self, suffixes: Vec<String>) {
        self.internal_suffixes = suffixes;
    }

    /// 添加已知DNS服务器
    pub fn add_dns_server(&mut self, ip: IpAddr) {
        if !self.known_dns_servers.contains(&ip) {
            self.known_dns_servers.push(ip);
            self.assets.dns_servers.push(ip);
        }
    }

    /// 解析DNS数据包并更新资产发现
    pub fn process_dns_packet(
        &mut self,
        data: &[u8],
        dns_server_ip: IpAddr,
        client_ip: IpAddr,
        timestamp_ms: u64,
    ) -> Option<DnsRecord> {
        if data.len() < 12 {
            return None; // DNS头部最少12字节
        }

        // 解析DNS头部
        let _transaction_id = u16::from_be_bytes([data[0], data[1]]);
        let flags = u16::from_be_bytes([data[2], data[3]]);
        let questions = u16::from_be_bytes([data[4], data[5]]);
        let answers = u16::from_be_bytes([data[6], data[7]]);
        let _authorities = u16::from_be_bytes([data[8], data[9]]);
        let _additionals = u16::from_be_bytes([data[10], data[11]]);

        let is_response = (flags >> 15) & 0x1 == 1;
        let _opcode = ((flags >> 11) & 0xF) as u8;
        let rcode = (flags & 0xF) as u8;

        if questions == 0 {
            return None;
        }

        // 解析查询域名和类型
        let mut offset = 12;
        let (query_name, query_type_val, _) = self.parse_dns_question(data, &mut offset)?;

        let query_type = match query_type_val {
            1 => DnsRRType::A,
            28 => DnsRRType::AAAA,
            5 => DnsRRType::CNAME,
            2 => DnsRRType::NS,
            15 => DnsRRType::MX,
            12 => DnsRRType::PTR,
            16 => DnsRRType::TXT,
            6 => DnsRRType::SOA,
            33 => DnsRRType::SRV,
            255 => DnsRRType::ANY,
            _ => {
                return Some(self.build_empty_record(
                    &query_name,
                    query_type_val,
                    dns_server_ip,
                    client_ip,
                    rcode,
                    timestamp_ms,
                ))
            }
        };

        let mut record = DnsRecord {
            query_name: query_name.clone(),
            query_type,
            resolved_ips: Vec::new(),
            cname_chain: Vec::new(),
            authoritative_servers: Vec::new(),
            mail_servers: Vec::new(),
            ttl: 0,
            dns_server: dns_server_ip,
            client_ip,
            rcode: Self::parse_rcode(rcode),
            is_internal: self.is_internal_domain(&query_name),
            response_size: data.len() as u16,
            timestamp_ms,
        };

        // 解析应答记录
        if is_response && answers > 0 {
            for _ in 0..questions {
                if let Some((_, _, _)) = self.parse_dns_question(data, &mut offset) {
                    // skip question section
                }
            }

            for _ in 0..answers {
                if let Some((_name, rr_type, _rr_class, ttl, rdlength, rdata_offset)) =
                    self.parse_dns_rr(data, &mut offset)
                {
                    record.ttl = ttl;
                    match rr_type {
                        1 => {
                            // A 记录
                            if let Some(ip) = self.parse_a_record(data, rdata_offset, rdlength) {
                                record.resolved_ips.push(IpAddr::V4(ip));
                                self.record_passive_asset(&query_name, IpAddr::V4(ip));
                            }
                        }
                        28 => {
                            // AAAA 记录
                            if let Some(ip) = self.parse_aaaa_record(data, rdata_offset, rdlength) {
                                record.resolved_ips.push(IpAddr::V6(ip));
                                self.record_passive_asset(&query_name, IpAddr::V6(ip));
                            }
                        }
                        5 => {
                            // CNAME
                            if let Some(cname) = Self::parse_name(data, rdata_offset) {
                                record.cname_chain.push(cname);
                            }
                        }
                        2 => {
                            // NS
                            if let Some(ns) = Self::parse_name(data, rdata_offset) {
                                record.authoritative_servers.push(ns);
                            }
                        }
                        15 => {
                            // MX
                            if rdlength >= 3 {
                                let priority = u16::from_be_bytes([
                                    data[rdata_offset],
                                    data[rdata_offset + 1],
                                ]);
                                if let Some(mx) = Self::parse_name(data, rdata_offset + 2) {
                                    record.mail_servers.push((mx, priority));
                                    // 记录邮件服务器
                                    if !self
                                        .assets
                                        .mail_servers
                                        .iter()
                                        .any(|(h, _)| h == &record.query_name)
                                    {
                                        self.assets
                                            .mail_servers
                                            .push((record.query_name.clone(), dns_server_ip));
                                    }
                                }
                            }
                        }
                        _ => {}
                    }
                }
            }

            // 统计更新
            self.update_stats(&record);
        }

        // DNS隧道检测
        if self.detect_dns_tunnel(&record) {
            // 标记为可疑DNS活动
        }

        Some(record)
    }

    /// 解析DNS问题部分
    fn parse_dns_question(&self, data: &[u8], offset: &mut usize) -> Option<(String, u16, u16)> {
        let name = Self::parse_name(data, *offset)?;
        *offset += Self::name_length(data, *offset)?;

        if *offset + 4 > data.len() {
            return None;
        }
        let qtype = u16::from_be_bytes([data[*offset], data[*offset + 1]]);
        let qclass = u16::from_be_bytes([data[*offset + 2], data[*offset + 3]]);
        *offset += 4;

        Some((name, qtype, qclass))
    }

    /// 解析DNS资源记录
    fn parse_dns_rr(
        &self,
        data: &[u8],
        offset: &mut usize,
    ) -> Option<(String, u16, u16, u32, u16, usize)> {
        let name = Self::parse_name(data, *offset)?;
        *offset += Self::name_length(data, *offset)?;

        if *offset + 10 > data.len() {
            return None;
        }
        let rr_type = u16::from_be_bytes([data[*offset], data[*offset + 1]]);
        let rr_class = u16::from_be_bytes([data[*offset + 2], data[*offset + 3]]);
        let ttl = u32::from_be_bytes([
            data[*offset + 4],
            data[*offset + 5],
            data[*offset + 6],
            data[*offset + 7],
        ]);
        let rdlength = u16::from_be_bytes([data[*offset + 8], data[*offset + 9]]);
        *offset += 10;

        let rdata_offset = *offset;
        *offset += rdlength as usize;

        Some((name, rr_type, rr_class, ttl, rdlength, rdata_offset))
    }

    /// 解析域名（支持DNS压缩指针）
    pub fn parse_name(data: &[u8], mut offset: usize) -> Option<String> {
        if offset >= data.len() {
            return None;
        }

        let mut parts: Vec<String> = Vec::new();
        let mut jumped = false;
        let mut jump_offset = 0;
        let _original_offset = offset;

        loop {
            if offset >= data.len() {
                if jumped {
                    offset = jump_offset;
                    continue;
                }
                return None;
            }

            let len = data[offset];
            if len == 0 {
                offset += 1;
                if !jumped {
                    // break out
                }
                break;
            }

            // DNS 压缩指针 (前两位为11)
            if len & 0xC0 == 0xC0 {
                if offset + 1 >= data.len() {
                    return None;
                }
                let pointer = ((len as usize & 0x3F) << 8) | data[offset + 1] as usize;
                if !jumped {
                    jump_offset = offset + 2;
                    jumped = true;
                }
                offset = pointer;
                continue;
            }

            if len as usize > 63 {
                return None; // 无效标签长度
            }

            offset += 1;
            let end = offset + len as usize;
            if end > data.len() {
                return None;
            }

            match std::str::from_utf8(&data[offset..end]) {
                Ok(label) => parts.push(label.to_lowercase()),
                Err(_) => return None,
            }
            offset = end;
        }

        if parts.is_empty() {
            None
        } else {
            Some(parts.join("."))
        }
    }

    /// 计算域名编码长度
    fn name_length(data: &[u8], mut offset: usize) -> Option<usize> {
        let start = offset;
        loop {
            if offset >= data.len() {
                return None;
            }
            let len = data[offset];
            if len == 0 {
                return Some(offset - start + 1);
            }
            if len & 0xC0 == 0xC0 {
                return Some(offset - start + 2);
            }
            offset += 1 + len as usize;
        }
    }

    /// 解析 A 记录
    fn parse_a_record(
        &self,
        data: &[u8],
        offset: usize,
        rdlength: u16,
    ) -> Option<std::net::Ipv4Addr> {
        if rdlength < 4 || offset + 4 > data.len() {
            return None;
        }
        Some(std::net::Ipv4Addr::new(
            data[offset],
            data[offset + 1],
            data[offset + 2],
            data[offset + 3],
        ))
    }

    /// 解析 AAAA 记录
    fn parse_aaaa_record(
        &self,
        data: &[u8],
        offset: usize,
        rdlength: u16,
    ) -> Option<std::net::Ipv6Addr> {
        if rdlength < 16 || offset + 16 > data.len() {
            return None;
        }
        let mut bytes = [0u8; 16];
        bytes.copy_from_slice(&data[offset..offset + 16]);
        Some(std::net::Ipv6Addr::from(bytes))
    }

    /// 解析响应码
    fn parse_rcode(rcode: u8) -> DnsRcode {
        match rcode {
            0 => DnsRcode::NoError,
            1 => DnsRcode::FormErr,
            2 => DnsRcode::ServFail,
            3 => DnsRcode::NXDomain,
            4 => DnsRcode::NotImp,
            5 => DnsRcode::Refused,
            6 => DnsRcode::YXDomain,
            7 => DnsRcode::YXRRSet,
            8 => DnsRcode::NXRRSet,
            9 => DnsRcode::NotAuth,
            10 => DnsRcode::NotZone,
            _ => DnsRcode::ServFail,
        }
    }

    /// 记录被动资产发现
    fn record_passive_asset(&mut self, hostname: &str, ip: IpAddr) {
        // hostname → IP
        self.assets
            .hostname_to_ip
            .entry(hostname.to_string())
            .or_default()
            .push(ip);

        // IP → hostname
        self.assets
            .ip_to_hostname
            .entry(ip)
            .or_default()
            .push(hostname.to_string());

        // CDN域名检测
        for cdn_suffix in &self.tunnel_detector.whitelist_domains {
            if hostname.ends_with(cdn_suffix) {
                if !self.assets.cdn_domains.contains(&hostname.to_string()) {
                    self.assets.cdn_domains.push(hostname.to_string());
                }
            }
        }
    }

    /// 判断是否为内部域名
    fn is_internal_domain(&self, name: &str) -> bool {
        for suffix in &self.internal_suffixes {
            if name.ends_with(suffix) {
                return true;
            }
        }
        false
    }

    /// 更新DNS查询统计
    fn update_stats(&mut self, record: &DnsRecord) {
        self.stats.total_queries += 1;

        if record.rcode == DnsRcode::NXDomain {
            self.stats.nxdomain_count += 1;
        }

        let domain_len = record.query_name.len();
        self.stats.max_domain_length = self.stats.max_domain_length.max(domain_len);

        // 更新平均域名长度 (简化计算)
        self.stats.avg_domain_length = (self.stats.avg_domain_length
            * (self.stats.total_queries - 1) as f32
            + domain_len as f32)
            / self.stats.total_queries as f32;

        // 高熵域名检测
        let entropy = self.calculate_shannon_entropy(&record.query_name);
        if entropy > self.tunnel_detector.entropy_threshold {
            self.stats.high_entropy_domains += 1;
        }

        // AAAA查询比例
        if matches!(record.query_type, DnsRRType::AAAA) {
            let aaaa_count =
                (self.stats.aaaa_ratio * (self.stats.total_queries - 1) as f32) as u64 + 1;
            self.stats.aaaa_ratio = aaaa_count as f32 / self.stats.total_queries as f32;
        } else {
            self.stats.aaaa_ratio = (self.stats.aaaa_ratio * (self.stats.total_queries - 1) as f32)
                / self.stats.total_queries as f32;
        }
    }

    /// 计算 Shannon 熵
    fn calculate_shannon_entropy(&self, s: &str) -> f32 {
        let mut freq = [0u32; 256];
        let len = s.len();
        if len == 0 {
            return 0.0;
        }

        for byte in s.bytes() {
            freq[byte as usize] += 1;
        }

        let mut entropy = 0.0f32;
        for &count in freq.iter() {
            if count > 0 {
                let p = count as f32 / len as f32;
                entropy -= p * p.log2();
            }
        }
        entropy
    }

    /// DNS隧道检测
    pub fn detect_dns_tunnel(&self, record: &DnsRecord) -> bool {
        // 检查域名长度
        if record.query_name.len() > self.tunnel_detector.max_domain_length {
            tracing::warn!(
                "DNS tunnel suspicion: long domain {} (len={})",
                record.query_name,
                record.query_name.len()
            );
            return true;
        }

        // 检查子域名数量
        let subdomain_count = record.query_name.split('.').count();
        if subdomain_count > self.tunnel_detector.max_subdomain_count {
            tracing::warn!(
                "DNS tunnel suspicion: many subdomains in {} (count={})",
                record.query_name,
                subdomain_count
            );
            return true;
        }

        // 检查域名熵值
        let entropy = self.calculate_shannon_entropy(&record.query_name);
        if entropy > self.tunnel_detector.entropy_threshold {
            // 排除已知的合法高熵域名
            for whitelist in &self.tunnel_detector.whitelist_domains {
                if record.query_name.ends_with(whitelist) {
                    return false;
                }
            }
            tracing::warn!(
                "DNS tunnel suspicion: high entropy domain {} (entropy={:.2})",
                record.query_name,
                entropy
            );
            return true;
        }

        false
    }

    /// 构建空记录 (用于未知查询类型)
    fn build_empty_record(
        &self,
        query_name: &str,
        _query_type_val: u16,
        dns_server_ip: IpAddr,
        client_ip: IpAddr,
        rcode: u8,
        timestamp_ms: u64,
    ) -> DnsRecord {
        DnsRecord {
            query_name: query_name.to_string(),
            query_type: DnsRRType::ANY,
            resolved_ips: Vec::new(),
            cname_chain: Vec::new(),
            authoritative_servers: Vec::new(),
            mail_servers: Vec::new(),
            ttl: 0,
            dns_server: dns_server_ip,
            client_ip,
            rcode: Self::parse_rcode(rcode),
            is_internal: self.is_internal_domain(query_name),
            response_size: 0,
            timestamp_ms,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_dns_name_simple() {
        // www.example.com — 编码: 3 'w' 'w' 'w' 7 'e' 'x' 'a' 'm' 'p' 'l' 'e' 3 'c' 'o' 'm' 0
        let data = &[
            3, b'w', b'w', b'w', 7, b'e', b'x', b'a', b'm', b'p', b'l', b'e', 3, b'c', b'o', b'm',
            0,
        ];
        let name = DnsParser::parse_name(data, 0);
        assert_eq!(name, Some("www.example.com".to_string()));
    }

    #[test]
    fn test_parse_dns_name_compressed() {
        // 使用压缩指针: 第一个名字是 "example.com" 在 offset 12
        // 第二个名字通过指针 0xC00C 引用它
        let data = &[
            0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // padding
            7, b'e', b'x', b'a', b'm', b'p', b'l', b'e', 3, b'c', b'o', b'm', 0, 3, b'w', b'w',
            b'w', 0xC0, 12, // 指向 offset 12 (example.com)
        ];
        let name = DnsParser::parse_name(data, 25); // start at "www"
        assert_eq!(name, Some("www.example.com".to_string()));
    }

    #[test]
    fn test_shannon_entropy() {
        let parser = DnsParser::new();

        // 低熵 (正常域名)
        let low = parser.calculate_shannon_entropy("www.google.com");
        assert!(low < 3.0);

        // 高熵 (可能的DNS隧道)
        let high = parser.calculate_shannon_entropy("aB3xK9mQ2pR7sT4v.example.com");
        assert!(high > 3.0);
    }

    #[test]
    fn test_is_internal_domain() {
        let parser = DnsParser::new();
        assert!(parser.is_internal_domain("server01.internal"));
        assert!(parser.is_internal_domain("printer.local"));
        assert!(!parser.is_internal_domain("www.google.com"));
    }
}
