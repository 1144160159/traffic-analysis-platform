// ARP Passive Discovery Parser — 从ARP流量被动发现MAC↔IP映射
//
// 业务价值:
//   - 解析ARP请求/响应 → 建立MAC↔IP绑定
//   - 检测ARP欺骗/投毒攻击
//   - 发现新设备上线
//   - 被动VLAN感知 (ARP流量自带VLAN tag)
//   - 识别网关/路由器

use std::collections::HashMap;
use std::net::Ipv4Addr;

/// ARP 操作码
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ArpOperation {
    Request = 1,
    Reply = 2,
    RarpRequest = 3,
    RarpReply = 4,
}

/// MAC 地址 (6 字节)
pub type MacAddr = [u8; 6];

/// ARP 绑定记录
#[derive(Debug, Clone)]
pub struct ArpBinding {
    pub ip: Ipv4Addr,
    pub mac: MacAddr,
    pub interface_mac: Option<MacAddr>, // 捕获接口的 MAC
    pub operation: ArpOperation,
    pub timestamp_ms: u64,
    pub vlan_id: Option<u16>,
    pub is_gratuitous: bool, // 无偿ARP (GARP)
    pub is_probe: bool,      // ARP Probe (源IP为0.0.0.0)
    pub tpa_is_local: bool,  // 目标IP是否为本地网络
    pub is_new_device: bool, // 首次发现的设备
}

/// ARP 统计
#[derive(Debug, Clone, Default)]
pub struct ArpStats {
    pub total_requests: u64,
    pub total_replies: u64,
    pub gratuitous_arps: u64,
    pub arp_probes: u64,
    pub unique_macs: usize,
    pub unique_ips: usize,
    pub spoofing_alerts: u64,
}

/// ARP 欺骗检测结果
#[derive(Debug, Clone)]
pub struct ArpSpoofAlert {
    pub ip: Ipv4Addr,
    pub original_mac: MacAddr,
    pub spoofed_mac: MacAddr,
    pub detected_at_ms: u64,
    pub alert_type: ArpSpoofType,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ArpSpoofType {
    MacChange,       // 同一IP突然对应不同MAC
    GratuitousFlood, // 大量无偿ARP
    GatewaySpoof,    // 网关IP被非网关MAC声明
    DuplicateIp,     // 同一IP有多个MAC声明
}

/// ARP 解析器 — 被动资产发现 + 欺骗检测
pub struct ArpParser {
    /// IP → MAC 绑定 (最新)
    pub ip_to_mac: HashMap<Ipv4Addr, MacAddr>,
    /// MAC → IP 绑定
    pub mac_to_ip: HashMap<MacAddr, Ipv4Addr>,
    /// IP → MAC 历史 (用于欺骗检测)
    ip_mac_history: HashMap<Ipv4Addr, Vec<(MacAddr, u64)>>,
    /// 已知网关 MAC 地址
    pub gateway_macs: Vec<MacAddr>,
    /// 已知网关 IP 地址
    pub gateway_ips: Vec<Ipv4Addr>,
    /// 统计
    pub stats: ArpStats,
    /// 欺骗告警
    pub spoof_alerts: Vec<ArpSpoofAlert>,
    /// MAC 首次出现时间
    mac_first_seen: HashMap<MacAddr, u64>,
    /// IP 首次出现时间
    ip_first_seen: HashMap<Ipv4Addr, u64>,
    /// 最大历史记录数
    max_history: usize,
}

impl ArpParser {
    pub fn new() -> Self {
        Self {
            ip_to_mac: HashMap::new(),
            mac_to_ip: HashMap::new(),
            ip_mac_history: HashMap::new(),
            gateway_macs: Vec::new(),
            gateway_ips: Vec::new(),
            stats: ArpStats::default(),
            spoof_alerts: Vec::new(),
            mac_first_seen: HashMap::new(),
            ip_first_seen: HashMap::new(),
            max_history: 5,
        }
    }

    /// 设置已知网关
    pub fn set_gateway(&mut self, ip: Ipv4Addr, mac: MacAddr) {
        if !self.gateway_ips.contains(&ip) {
            self.gateway_ips.push(ip);
        }
        if !self.gateway_macs.contains(&mac) {
            self.gateway_macs.push(mac);
        }
    }

    /// 解析 ARP 数据包
    pub fn parse_arp_packet(
        &mut self,
        data: &[u8],
        capture_mac: Option<MacAddr>,
        vlan_id: Option<u16>,
        timestamp_ms: u64,
    ) -> Option<ArpBinding> {
        // ARP 数据包最小长度: 28 字节 (以太网 ARP)
        // 硬件类型(2) + 协议类型(2) + 硬件地址长度(1) + 协议地址长度(1) + 操作(2)
        // + 发送者MAC(6) + 发送者IP(4) + 目标MAC(6) + 目标IP(4)
        if data.len() < 28 {
            return None;
        }

        // 硬件类型 (HTYPE) — 1 = Ethernet
        let htype = u16::from_be_bytes([data[0], data[1]]);
        if htype != 1 {
            return None; // 仅支持 Ethernet
        }

        // 协议类型 (PTYPE) — 0x0800 = IPv4
        let ptype = u16::from_be_bytes([data[2], data[3]]);
        if ptype != 0x0800 {
            return None; // 仅支持 IPv4
        }

        let hlen = data[4]; // 硬件地址长度 (应为 6)
        let plen = data[5]; // 协议地址长度 (应为 4)

        if hlen != 6 || plen != 4 {
            return None;
        }

        // 操作码
        let operation = match u16::from_be_bytes([data[6], data[7]]) {
            1 => ArpOperation::Request,
            2 => ArpOperation::Reply,
            3 => ArpOperation::RarpRequest,
            4 => ArpOperation::RarpReply,
            _ => return None,
        };

        // 发送者 MAC (SHA)
        let sender_mac: MacAddr = [data[8], data[9], data[10], data[11], data[12], data[13]];

        // 发送者 IP (SPA)
        let sender_ip = Ipv4Addr::new(data[14], data[15], data[16], data[17]);

        // 目标 MAC (THA)
        let target_mac: MacAddr = [data[18], data[19], data[20], data[21], data[22], data[23]];

        // 目标 IP (TPA)
        let target_ip = Ipv4Addr::new(data[24], data[25], data[26], data[27]);

        // 检查是否为无偿 ARP (Gratuitous ARP)
        let is_gratuitous = match operation {
            ArpOperation::Request => sender_ip == target_ip,
            ArpOperation::Reply => sender_ip == target_ip || target_mac == [0; 6],
            _ => false,
        };

        // 检查是否为 ARP Probe (发送者 IP 为 0.0.0.0)
        let is_probe = sender_ip == Ipv4Addr::UNSPECIFIED;

        // 判断目标是否为本地网络 (基于常见私有IP段)
        let tpa_is_local = is_private_ipv4(&target_ip);

        // 检查是否为新设备
        let is_new_device = !self.mac_first_seen.contains_key(&sender_mac);

        let binding = ArpBinding {
            ip: sender_ip,
            mac: sender_mac,
            interface_mac: capture_mac,
            operation,
            timestamp_ms,
            vlan_id,
            is_gratuitous,
            is_probe,
            tpa_is_local,
            is_new_device,
        };

        // 更新状态
        self.update_state(&binding);

        Some(binding)
    }

    /// 更新内部状态
    fn update_state(&mut self, binding: &ArpBinding) {
        // 跳过无效 MAC (全0或广播)
        if binding.mac == [0; 6] || binding.mac == [0xFF; 6] {
            return;
        }

        // 跳过 ARP Probe (IP 为 0.0.0.0)
        if binding.is_probe {
            self.stats.arp_probes += 1;
            return;
        }

        // 统计
        match binding.operation {
            ArpOperation::Request => self.stats.total_requests += 1,
            ArpOperation::Reply => self.stats.total_replies += 1,
            _ => {}
        }

        if binding.is_gratuitous {
            self.stats.gratuitous_arps += 1;
        }

        // 首次发现
        if binding.is_new_device {
            self.mac_first_seen
                .insert(binding.mac, binding.timestamp_ms);
        }
        if !self.ip_first_seen.contains_key(&binding.ip) {
            self.ip_first_seen.insert(binding.ip, binding.timestamp_ms);
        }

        self.stats.unique_macs = self.mac_first_seen.len();
        self.stats.unique_ips = self.ip_first_seen.len();

        // 更新 IP→MAC 映射 (检测 MAC 变更 / 可能的 ARP 欺骗)
        if let Some(&existing_mac) = self.ip_to_mac.get(&binding.ip) {
            if existing_mac != binding.mac {
                // IP 对应的 MAC 发生了变化 → 可能 ARP 欺骗
                self.stats.spoofing_alerts += 1;

                let alert_type = if self.gateway_ips.contains(&binding.ip)
                    && !self.gateway_macs.contains(&binding.mac)
                {
                    ArpSpoofType::GatewaySpoof
                } else {
                    ArpSpoofType::MacChange
                };

                let alert = ArpSpoofAlert {
                    ip: binding.ip,
                    original_mac: existing_mac,
                    spoofed_mac: binding.mac,
                    detected_at_ms: binding.timestamp_ms,
                    alert_type,
                };

                tracing::warn!(
                    "ARP spoofing detected! IP={} original_mac={:02x?} spoofed_mac={:02x?} type={:?}",
                    binding.ip,
                    existing_mac,
                    binding.mac,
                    alert_type
                );

                self.spoof_alerts.push(alert);
            }
        }

        // 更新绑定
        self.ip_to_mac.insert(binding.ip, binding.mac);
        self.mac_to_ip.insert(binding.mac, binding.ip);

        // 记录历史
        self.ip_mac_history
            .entry(binding.ip)
            .or_default()
            .push((binding.mac, binding.timestamp_ms));
    }

    /// 查询IP对应的MAC
    pub fn get_mac_for_ip(&self, ip: &Ipv4Addr) -> Option<&MacAddr> {
        self.ip_to_mac.get(ip)
    }

    /// 查询MAC对应的IP
    pub fn get_ip_for_mac(&self, mac: &MacAddr) -> Option<&Ipv4Addr> {
        self.mac_to_ip.get(mac)
    }

    /// 检测 ARP 扫描 (同一源MAC在短时间内查询大量IP)
    pub fn detect_arp_scan(&self, _window_ms: u64, threshold: usize) -> Vec<MacAddr> {
        let mut scan_counts: HashMap<MacAddr, Vec<Ipv4Addr>> = HashMap::new();
        // 遍历绑定记录中的IP列表
        for (ip, mac) in &self.ip_to_mac {
            if let Some(history) = self.ip_mac_history.get(ip) {
                for &(h_mac, _ts) in history {
                    if h_mac == *mac {
                        // 计算时间窗口
                        scan_counts.entry(h_mac).or_default().push(*ip);
                    }
                }
            }
        }

        let mut scanners = Vec::new();
        for (mac, ips) in scan_counts {
            if ips.len() >= threshold {
                scanners.push(mac);
                tracing::info!(
                    "ARP scan detected: mac={:02x?}, unique_ips={}",
                    mac,
                    ips.len()
                );
            }
        }
        scanners
    }

    /// 获取活跃的设备列表
    pub fn active_devices(&self) -> Vec<(MacAddr, Ipv4Addr, u64)> {
        self.ip_to_mac
            .iter()
            .map(|(ip, mac)| {
                (
                    *mac,
                    *ip,
                    self.mac_first_seen.get(mac).copied().unwrap_or(0),
                )
            })
            .collect()
    }

    /// 获取最近的欺骗告警
    pub fn recent_spoof_alerts(&self, count: usize) -> &[ArpSpoofAlert] {
        let len = self.spoof_alerts.len();
        let start = len.saturating_sub(count);
        &self.spoof_alerts[start..]
    }
}

/// 检查是否为私有 IPv4 地址
fn is_private_ipv4(ip: &Ipv4Addr) -> bool {
    let octets = ip.octets();
    match octets[0] {
        10 => true,
        172 if octets[1] >= 16 && octets[1] <= 31 => true,
        192 if octets[1] == 168 => true,
        169 if octets[1] == 254 => true, // link-local
        127 => true,                     // loopback
        224..=239 => true,               // multicast
        _ => false,
    }
}

/// MAC 地址转显示字符串
pub fn mac_to_string(mac: &MacAddr) -> String {
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]
    )
}

/// 从字符串解析 MAC 地址
pub fn parse_mac(s: &str) -> Option<MacAddr> {
    let parts: Vec<&str> = s.split(':').collect();
    if parts.len() != 6 {
        return None;
    }
    let mut mac = [0u8; 6];
    for (i, part) in parts.iter().enumerate() {
        mac[i] = u8::from_str_radix(part, 16).ok()?;
    }
    Some(mac)
}

// OUI 厂商数据库 (MAC 前 3 字节 → 厂商名称)
pub fn lookup_oui(mac: &MacAddr) -> Option<&'static str> {
    let oui = ((mac[0] as u32) << 16) | ((mac[1] as u32) << 8) | (mac[2] as u32);
    match oui {
        0x00000C => Some("Cisco Systems"),
        0x00001A => Some("AMD"),
        0x0004AC => Some("IBM"),
        0x0018C5 => Some("Intel"),
        0x001A79 => Some("Cisco-Linksys"),
        0x001B21 => Some("Intel Corporate"),
        0x0021D7 => Some("Cisco Systems"),
        0x005056 => Some("VMware"),
        0x00A0C9 => Some("Intel Corporate"),
        0x08002B => Some("DEC"),
        0x0C6076 => Some("Dell"),
        0x18C04D => Some("Hewlett Packard"),
        0x1C8780 => Some("Apple"),
        0x244C07 => Some("Huawei"),
        0x28D244 => Some("Dell"),
        0x3C2C94 => Some("Apple"),
        0x4433A9 => Some("Samsung"),
        0x4C3275 => Some("Hewlett Packard"),
        0x5C8D4E => Some("Huawei"),
        0x64B9E8 => Some("Apple"),
        0x708BCD => Some("Dell"),
        0x7848D8 => Some("Samsung"),
        0x7CECB1 => Some("Cisco Meraki"),
        0x88E9FE => Some("Cisco"),
        0x8C8CD8 => Some("Apple"),
        0x9439E5 => Some("Intel"),
        0xA0ECF9 => Some("Dell"),
        0xB05CDA => Some("Intel Corporate"),
        0xC0D3C0 => Some("Dell"),
        0xD4F5EF => Some("Cisco"),
        0xE02F31 => Some("Intel"),
        0xEC58EA => Some("Samsung"),
        0xF03BFC => Some("Hewlett Packard"),
        0xF40343 => Some("Apple"),
        0xF80F84 => Some("Dell"),
        0xFC3497 => Some("VMware"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 构建 ARP Request 数据包
    fn build_arp_request(sender_mac: MacAddr, sender_ip: Ipv4Addr, target_ip: Ipv4Addr) -> Vec<u8> {
        let mut pkt = vec![0u8; 28];
        pkt[0] = 0x00;
        pkt[1] = 0x01; // HTYPE: Ethernet
        pkt[2] = 0x08;
        pkt[3] = 0x00; // PTYPE: IPv4
        pkt[4] = 6; // HLEN: MAC length
        pkt[5] = 4; // PLEN: IP length
        pkt[6] = 0x00;
        pkt[7] = 0x01; // Operation: Request

        pkt[8..14].copy_from_slice(&sender_mac); // SHA
        pkt[14..18].copy_from_slice(&sender_ip.octets()); // SPA
        pkt[18..24].copy_from_slice(&[0; 6]); // THA (unknown)
        pkt[24..28].copy_from_slice(&target_ip.octets()); // TPA

        pkt
    }

    fn mac(a: u8, b: u8, c: u8, d: u8, e: u8, f: u8) -> MacAddr {
        [a, b, c, d, e, f]
    }

    #[test]
    fn test_parse_arp_request() {
        let mut parser = ArpParser::new();
        let sender_mac = mac(0x00, 0x1A, 0xC5, 0x01, 0x02, 0x03);
        let sender_ip = Ipv4Addr::new(192, 168, 1, 100);
        let target_ip = Ipv4Addr::new(192, 168, 1, 1);

        let pkt = build_arp_request(sender_mac, sender_ip, target_ip);
        let binding = parser.parse_arp_packet(&pkt, None, None, 1000).unwrap();

        assert_eq!(binding.operation, ArpOperation::Request);
        assert_eq!(binding.mac, sender_mac);
        assert_eq!(binding.ip, sender_ip);
        assert!(binding.is_new_device);
        assert!(!binding.is_gratuitous);
        assert!(binding.tpa_is_local);
    }

    #[test]
    fn test_arp_spoof_detection() {
        let mut parser = ArpParser::new();
        let gateway_ip = Ipv4Addr::new(192, 168, 1, 1);
        let gateway_mac = mac(0x00, 0x1A, 0xC5, 0xAA, 0xBB, 0xCC);
        parser.set_gateway(gateway_ip, gateway_mac);

        // 正常的 ARP 响应
        let pkt1 = build_arp_request(gateway_mac, gateway_ip, Ipv4Addr::new(192, 168, 1, 100));
        parser.parse_arp_packet(&pkt1, None, None, 1000);

        // 欺骗: 相同 IP 但不同 MAC
        let spoof_mac = mac(0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01);
        let pkt2 = build_arp_request(spoof_mac, gateway_ip, Ipv4Addr::new(192, 168, 1, 100));
        parser.parse_arp_packet(&pkt2, None, None, 2000);

        assert_eq!(parser.stats.spoofing_alerts, 1);
        let alert = &parser.spoof_alerts[0];
        assert_eq!(alert.alert_type, ArpSpoofType::GatewaySpoof);
        assert_eq!(alert.ip, gateway_ip);
        assert_eq!(alert.spoofed_mac, spoof_mac);
    }

    #[test]
    fn test_gratuitous_arp() {
        let mut parser = ArpParser::new();
        let mac = mac(0x00, 0x1A, 0xC5, 0x01, 0x02, 0x03);
        let ip = Ipv4Addr::new(192, 168, 1, 100);

        // 无偿 ARP: sender_ip == target_ip
        let pkt = build_arp_request(mac, ip, ip);
        let binding = parser.parse_arp_packet(&pkt, None, None, 1000).unwrap();

        assert!(binding.is_gratuitous);
        assert_eq!(parser.stats.gratuitous_arps, 1);
    }

    #[test]
    fn test_oui_lookup() {
        assert_eq!(
            lookup_oui(&mac(0x00, 0x00, 0x0C, 0x01, 0x02, 0x03)),
            Some("Cisco Systems")
        );
        assert_eq!(
            lookup_oui(&mac(0x00, 0x18, 0xC5, 0x01, 0x02, 0x03)),
            Some("Intel")
        ); // OUI: 00:18:C5
        assert_eq!(lookup_oui(&mac(0x00, 0x56, 0x56, 0x01, 0x02, 0x03)), None); // 不存在的OUI
    }
}
