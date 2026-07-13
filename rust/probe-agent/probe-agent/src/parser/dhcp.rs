// DHCP Passive Discovery Parser — 从DHCP流量中被动发现资产信息
//
// 业务价值:
//   - 解析 DHCP DISCOVER/OFFER/REQUEST/ACK 建立 MAC↔IP↔Hostname 绑定
//   - 识别新设备上线 (首次DHCP请求)
//   - 检测 DHCP 耗尽攻击
//   - 识别操作系统指纹 (DHCP Option 55/60)
//   - 被动 VLAN 发现 (DHCP Option 82)

use std::collections::HashMap;
use std::net::Ipv4Addr;

/// MAC 地址类型 (6 字节)
pub type MacAddr = [u8; 6];

/// 从字节创建 MAC 地址
fn mac_new(a: u8, b: u8, c: u8, d: u8, e: u8, f: u8) -> MacAddr {
    [a, b, c, d, e, f]
}

/// MAC 地址转字符串
fn mac_to_string(mac: &MacAddr) -> String {
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]
    )
}

/// DHCP 消息类型
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DhcpMessageType {
    Discover = 1,
    Offer = 2,
    Request = 3,
    Decline = 4,
    Ack = 5,
    Nak = 6,
    Release = 7,
    Inform = 8,
}

/// DHCP 选项码
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DhcpOptionCode {
    SubnetMask = 1,
    Router = 3,
    DomainNameServer = 6,
    HostName = 12,
    DomainName = 15,
    BroadcastAddress = 28,
    RequestedIPAddress = 50,
    LeaseTime = 51,
    MessageType = 53,
    ServerIdentifier = 54,
    ParameterRequestList = 55,
    VendorClassIdentifier = 60,
    ClientIdentifier = 61,
    TFTPServerName = 66,
    BootfileName = 67,
    RelayAgentInformation = 82,
    AutoConfigure = 116,
    ClasslessStaticRoute = 121,
}

/// DHCP 租约信息
#[derive(Debug, Clone)]
pub struct DhcpLease {
    /// 客户端 MAC 地址
    pub mac_address: MacAddr,
    /// 分配的 IP 地址
    pub assigned_ip: Ipv4Addr,
    /// DHCP 服务器 IP
    pub server_ip: Ipv4Addr,
    /// 网关 IP
    pub gateway_ip: Option<Ipv4Addr>,
    /// 子网掩码
    pub subnet_mask: Option<Ipv4Addr>,
    /// DNS 服务器列表
    pub dns_servers: Vec<Ipv4Addr>,
    /// 主机名 (DHCP Option 12)
    pub hostname: Option<String>,
    /// 域名 (DHCP Option 15)
    pub domain_name: Option<String>,
    /// 厂商识别 (DHCP Option 60 — OS指纹)
    pub vendor_class: Option<String>,
    /// 客户端标识 (DHCP Option 61)
    pub client_identifier: Option<Vec<u8>>,
    /// 租约时间 (秒)
    pub lease_time: u32,
    /// 消息类型
    pub message_type: DhcpMessageType,
    /// 事务 ID
    pub transaction_id: u32,
    /// 中继代理信息 (DHCP Option 82 → VLAN)
    pub relay_agent: Option<DhcpRelayInfo>,
    /// 时间戳
    pub timestamp_ms: u64,
    /// 是否为新设备 (首次出现)
    pub is_new_device: bool,
}

/// DHCP 中继代理信息 (VLAN 发现)
#[derive(Debug, Clone)]
pub struct DhcpRelayInfo {
    /// 交换机标识 (Circuit ID)
    pub circuit_id: Option<String>,
    /// 远程标识 (Remote ID — 通常是交换机MAC)
    pub remote_id: Option<MacAddr>,
    /// VLAN ID (从 Circuit ID 解析)
    pub vlan_id: Option<u16>,
    /// 接入端口
    pub port: Option<String>,
}

/// OS指纹 — 基于 DHCP Option 60
#[derive(Debug, Clone)]
pub struct OsFingerprint {
    pub vendor_class: String,
    pub os_name: String,
    pub os_version: Option<String>,
    pub device_type: String,
}

/// DHCP 数据包解析器
pub struct DhcpParser {
    /// MAC → 最近租约
    pub leases: HashMap<MacAddr, DhcpLease>,
    /// IP → MAC 映射
    pub ip_to_mac: HashMap<Ipv4Addr, MacAddr>,
    /// MAC → hostname
    pub mac_to_hostname: HashMap<MacAddr, String>,
    /// 已知MAC地址 (已见过)
    pub known_macs: Vec<MacAddr>,
    /// OS指纹数据库
    os_fingerprints: Vec<OsFingerprint>,
    /// 租约统计
    pub lease_stats: DhcpLeaseStats,
}

/// DHCP 租约统计
#[derive(Debug, Clone, Default)]
pub struct DhcpLeaseStats {
    pub total_discoveries: u64,
    pub total_requests: u64,
    pub total_acks: u64,
    pub total_naks: u64,
    pub total_releases: u64,
    pub new_devices: u64,
    pub active_leases: usize,
    pub unique_oses: usize,
}

impl DhcpParser {
    pub fn new() -> Self {
        let mut parser = Self {
            leases: HashMap::new(),
            ip_to_mac: HashMap::new(),
            mac_to_hostname: HashMap::new(),
            known_macs: Vec::new(),
            os_fingerprints: Vec::new(),
            lease_stats: DhcpLeaseStats::default(),
        };
        parser.load_os_fingerprints();
        parser
    }

    /// 加载OS指纹数据库
    fn load_os_fingerprints(&mut self) {
        self.os_fingerprints = vec![
            OsFingerprint {
                vendor_class: "MSFT 5.0".to_string(),
                os_name: "Windows".to_string(),
                os_version: Some("2000/XP".to_string()),
                device_type: "Desktop/Server".to_string(),
            },
            OsFingerprint {
                vendor_class: "MSFT 98".to_string(),
                os_name: "Windows".to_string(),
                os_version: Some("98/ME".to_string()),
                device_type: "Desktop".to_string(),
            },
            OsFingerprint {
                vendor_class: "MSFT".to_string(),
                os_name: "Windows".to_string(),
                os_version: Some("7/8/10/11".to_string()),
                device_type: "Desktop/Server".to_string(),
            },
            OsFingerprint {
                vendor_class: "dhcpcd".to_string(),
                os_name: "Linux".to_string(),
                os_version: None,
                device_type: "Generic".to_string(),
            },
            OsFingerprint {
                vendor_class: "dnsmasq".to_string(),
                os_name: "Linux".to_string(),
                os_version: None,
                device_type: "Network Device".to_string(),
            },
            OsFingerprint {
                vendor_class: "udhcp".to_string(),
                os_name: "Linux (BusyBox)".to_string(),
                os_version: None,
                device_type: "Embedded/IoT".to_string(),
            },
            OsFingerprint {
                vendor_class: "android-dhcp".to_string(),
                os_name: "Android".to_string(),
                os_version: None,
                device_type: "Mobile".to_string(),
            },
            OsFingerprint {
                vendor_class: "iOS".to_string(),
                os_name: "iOS".to_string(),
                os_version: None,
                device_type: "Mobile".to_string(),
            },
            OsFingerprint {
                vendor_class: "Mac OS X".to_string(),
                os_name: "macOS".to_string(),
                os_version: None,
                device_type: "Desktop/Laptop".to_string(),
            },
            OsFingerprint {
                vendor_class: "Cisco IOS".to_string(),
                os_name: "Cisco IOS".to_string(),
                os_version: None,
                device_type: "Network Device".to_string(),
            },
            OsFingerprint {
                vendor_class: "ArubaAP".to_string(),
                os_name: "ArubaOS".to_string(),
                os_version: None,
                device_type: "Access Point".to_string(),
            },
            OsFingerprint {
                vendor_class: "alaxy".to_string(),
                os_name: "Samsung Android".to_string(),
                os_version: None,
                device_type: "Mobile".to_string(),
            },
        ];
    }

    /// 解析DHCP数据包
    pub fn parse_dhcp_packet(
        &mut self,
        data: &[u8],
        source_mac: MacAddr,
        timestamp_ms: u64,
    ) -> Option<DhcpLease> {
        if data.len() < 240 {
            // DHCP最小长度: 240字节 (BOOTP头部)
            return None;
        }

        // 解析BOOTP头部
        let _op = data[0]; // 1=BOOTREQUEST, 2=BOOTREPLY
        let htype = data[1]; // 硬件类型 (1=Ethernet)
        let hlen = data[2]; // 硬件地址长度 (6 for MAC)
        let _hops = data[3];
        let transaction_id = u32::from_be_bytes([data[4], data[5], data[6], data[7]]);

        // 客户端 MAC (chaddr字段)
        let client_mac = if htype == 1 && hlen == 6 && data.len() > 34 {
            mac_new(data[28], data[29], data[30], data[31], data[32], data[33])
        } else {
            source_mac
        };

        // 分配给客户端的 IP (yiaddr)
        let assigned_ip = Ipv4Addr::new(data[16], data[17], data[18], data[19]);

        // DHCP服务器 IP (siaddr)
        let server_ip = Ipv4Addr::new(data[20], data[21], data[22], data[23]);

        // 解析DHCP选项 (从偏移量240开始，跳过4字节 magic cookie)
        let options_offset = 240;
        if options_offset + 4 > data.len() {
            return None;
        }
        let magic = &data[options_offset..options_offset + 4];
        if magic != &[99, 130, 83, 99] {
            // 无效的DHCP magic cookie
            return None;
        }

        let options = self.parse_dhcp_options(&data[options_offset + 4..])?;

        // 获取消息类型
        let message_type = match options.get(&53u8) {
            Some(val) if val.len() >= 1 => Self::parse_message_type(val[0]),
            _ => return None,
        }?;

        // 提取选项值
        let hostname = options
            .get(&12u8)
            .and_then(|v| String::from_utf8(v.clone()).ok());
        let domain_name = options
            .get(&15u8)
            .and_then(|v| String::from_utf8(v.clone()).ok());
        let vendor_class = options
            .get(&60u8)
            .and_then(|v| String::from_utf8(v.clone()).ok());
        let client_identifier = options.get(&61u8).cloned();

        let lease_time = options
            .get(&51u8)
            .and_then(|v| {
                if v.len() >= 4 {
                    Some(u32::from_be_bytes([v[0], v[1], v[2], v[3]]))
                } else {
                    None
                }
            })
            .unwrap_or(86400);

        let gateway_ip = options.get(&3u8).and_then(|v| {
            if v.len() >= 4 {
                Some(Ipv4Addr::new(v[0], v[1], v[2], v[3]))
            } else {
                None
            }
        });

        let subnet_mask = options.get(&1u8).and_then(|v| {
            if v.len() >= 4 {
                Some(Ipv4Addr::new(v[0], v[1], v[2], v[3]))
            } else {
                None
            }
        });

        let dns_servers = options
            .get(&6u8)
            .map(|v| {
                v.chunks(4)
                    .filter(|c| c.len() == 4)
                    .map(|c| Ipv4Addr::new(c[0], c[1], c[2], c[3]))
                    .collect()
            })
            .unwrap_or_default();

        // 解析中继代理信息 (Option 82 → VLAN发现)
        let relay_agent = options.get(&82u8).and_then(|v| self.parse_relay_agent(v));

        // 检查是否为新设备
        let is_new_device = !self.known_macs.contains(&client_mac);

        let lease = DhcpLease {
            mac_address: client_mac,
            assigned_ip,
            server_ip,
            gateway_ip,
            subnet_mask,
            dns_servers,
            hostname: hostname.clone(),
            domain_name,
            vendor_class: vendor_class.clone(),
            client_identifier,
            lease_time,
            message_type,
            transaction_id: transaction_id,
            relay_agent,
            timestamp_ms,
            is_new_device,
        };

        // 更新内部状态
        self.update_state(&lease);

        Some(lease)
    }

    /// 解析DHCP选项
    fn parse_dhcp_options(&self, data: &[u8]) -> Option<HashMap<u8, Vec<u8>>> {
        let mut options = HashMap::new();
        let mut offset = 0;

        while offset < data.len() {
            let code = data[offset];
            if code == 0 {
                // Padding
                offset += 1;
                continue;
            }
            if code == 255 {
                // End
                break;
            }
            if offset + 1 >= data.len() {
                break;
            }
            let len = data[offset + 1] as usize;
            offset += 2;
            if offset + len > data.len() {
                break;
            }
            let value = data[offset..offset + len].to_vec();
            options.insert(code, value);
            offset += len;
        }

        if options.is_empty() {
            None
        } else {
            Some(options)
        }
    }

    /// 解析DHCP消息类型
    fn parse_message_type(val: u8) -> Option<DhcpMessageType> {
        match val {
            1 => Some(DhcpMessageType::Discover),
            2 => Some(DhcpMessageType::Offer),
            3 => Some(DhcpMessageType::Request),
            4 => Some(DhcpMessageType::Decline),
            5 => Some(DhcpMessageType::Ack),
            6 => Some(DhcpMessageType::Nak),
            7 => Some(DhcpMessageType::Release),
            8 => Some(DhcpMessageType::Inform),
            _ => None,
        }
    }

    /// 解析中继代理信息 (Option 82)
    fn parse_relay_agent(&self, data: &[u8]) -> Option<DhcpRelayInfo> {
        let sub_options = self.parse_dhcp_options(data)?;
        let circuit_id = sub_options
            .get(&1u8)
            .and_then(|v| String::from_utf8(v.clone()).ok());
        let remote_id = sub_options.get(&2u8).and_then(|v| {
            if v.len() >= 6 {
                Some(mac_new(v[0], v[1], v[2], v[3], v[4], v[5]))
            } else {
                None
            }
        });

        // 尝试从 Circuit ID 解析 VLAN ID
        let vlan_id = circuit_id.as_ref().and_then(|cid| {
            // 常见格式: "VLAN:vlan_id:port" 或 "eth0.vlan_id"
            if let Some(pos) = cid.find("VLAN:") {
                cid[pos + 5..].split(':').next()?.parse::<u16>().ok()
            } else if let Some(pos) = cid.find(".vlan") {
                cid[pos + 5..].split('.').next()?.parse::<u16>().ok()
            } else {
                None
            }
        });

        let port = circuit_id
            .clone()
            .and_then(|cid| cid.rsplit(':').next().map(|s| s.to_string()));

        Some(DhcpRelayInfo {
            circuit_id,
            remote_id,
            vlan_id,
            port,
        })
    }

    /// 更新内部状态
    fn update_state(&mut self, lease: &DhcpLease) {
        // 更新租约记录
        self.leases.insert(lease.mac_address, lease.clone());

        // 更新 IP→MAC 映射
        self.ip_to_mac.insert(lease.assigned_ip, lease.mac_address);

        // 更新 MAC→hostname
        if let Some(ref hostname) = lease.hostname {
            self.mac_to_hostname
                .insert(lease.mac_address, hostname.clone());
        }

        // 跟踪已知MAC
        if !self.known_macs.contains(&lease.mac_address) {
            self.known_macs.push(lease.mac_address);
        }

        // 统计
        match lease.message_type {
            DhcpMessageType::Discover => self.lease_stats.total_discoveries += 1,
            DhcpMessageType::Request => self.lease_stats.total_requests += 1,
            DhcpMessageType::Ack => self.lease_stats.total_acks += 1,
            DhcpMessageType::Nak => self.lease_stats.total_naks += 1,
            DhcpMessageType::Release => self.lease_stats.total_releases += 1,
            _ => {}
        }

        if lease.is_new_device {
            self.lease_stats.new_devices += 1;
        }

        self.lease_stats.active_leases = self.leases.len();
        self.lease_stats.unique_oses = self.count_unique_oses();
    }

    /// 识别OS类型 (基于DHCP Option 60)
    pub fn identify_os(&self, vendor_class: &str) -> Option<OsFingerprint> {
        for fp in &self.os_fingerprints {
            if vendor_class.contains(&fp.vendor_class) {
                return Some(fp.clone());
            }
        }
        None
    }

    /// 统计发现的唯一OS类型数
    fn count_unique_oses(&self) -> usize {
        let mut oses = std::collections::HashSet::new();
        for lease in self.leases.values() {
            if let Some(ref vc) = lease.vendor_class {
                if let Some(fp) = self.identify_os(vc) {
                    oses.insert(fp.os_name);
                }
            }
        }
        oses.len()
    }

    /// 检测DHCP耗尽攻击
    pub fn detect_dhcp_exhaustion(&self) -> bool {
        // 短时间内大量 DISCOVER 消息来自不同MAC → 可能DHCP耗尽
        self.lease_stats.total_discoveries > 100
    }

    /// 获取所有活跃租约
    pub fn active_leases(&self) -> Vec<&DhcpLease> {
        self.leases.values().collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 构建合法的DHCP DISCOVER数据包
    fn build_discover_packet(client_mac: MacAddr, hostname: &str) -> Vec<u8> {
        let mut pkt = vec![0u8; 300];

        // BOOTP头部
        pkt[0] = 1; // BOOTREQUEST
        pkt[1] = 1; // Ethernet
        pkt[2] = 6; // MAC长度
        pkt[4..8].copy_from_slice(&0x01020304u32.to_be_bytes()); // transaction ID

        // chaddr (客户端MAC)
        pkt[28] = client_mac[0];
        pkt[29] = client_mac[1];
        pkt[30] = client_mac[2];
        pkt[31] = client_mac[3];
        pkt[32] = client_mac[4];
        pkt[33] = client_mac[5];

        // Magic Cookie
        pkt[240] = 99;
        pkt[241] = 130;
        pkt[242] = 83;
        pkt[243] = 99;

        // Option 53: DHCP Message Type = Discover
        pkt[244] = 53;
        pkt[245] = 1;
        pkt[246] = 1; // DHCPDISCOVER

        // Option 12: Hostname
        let hn = hostname.as_bytes();
        pkt[247] = 12;
        pkt[248] = hn.len() as u8;
        pkt[249..249 + hn.len()].copy_from_slice(hn);
        let next = 249 + hn.len();

        // Option 60: Vendor Class Identifier
        let vc = b"MSFT 5.0";
        pkt[next] = 60;
        pkt[next + 1] = vc.len() as u8;
        pkt[next + 2..next + 2 + vc.len()].copy_from_slice(vc);

        // Option 255: End
        pkt[next + 2 + vc.len()] = 255;

        pkt
    }

    #[test]
    fn test_parse_discover() {
        let mut parser = DhcpParser::new();
        let mac = mac_new(0x00, 0x1A, 0xC5, 0x01, 0x02, 0x03);
        let pkt = build_discover_packet(mac, "DESKTOP-01");

        let lease = parser.parse_dhcp_packet(&pkt, mac, 1000).unwrap();
        assert_eq!(lease.message_type, DhcpMessageType::Discover);
        assert_eq!(lease.mac_address, mac);
        assert_eq!(lease.hostname, Some("DESKTOP-01".to_string()));
        assert_eq!(lease.vendor_class, Some("MSFT 5.0".to_string()));
        assert!(lease.is_new_device);
        assert_eq!(parser.lease_stats.total_discoveries, 1);
    }

    #[test]
    fn test_os_identification() {
        let parser = DhcpParser::new();
        let os = parser.identify_os("MSFT 5.0").unwrap();
        assert_eq!(os.os_name, "Windows");
        assert_eq!(os.device_type, "Desktop/Server");

        let os2 = parser.identify_os("android-dhcp-9").unwrap();
        assert_eq!(os2.os_name, "Android");
    }

    #[test]
    fn test_new_device_detection() {
        let mut parser = DhcpParser::new();
        let mac1 = mac_new(0x00, 0x1A, 0xC5, 0x01, 0x02, 0x03);
        let mac2 = mac_new(0x00, 0x1A, 0xC5, 0x04, 0x05, 0x06);

        let pkt1 = build_discover_packet(mac1, "DEVICE-1");
        let pkt2 = build_discover_packet(mac2, "DEVICE-2");

        let lease1 = parser.parse_dhcp_packet(&pkt1, mac1, 1000).unwrap();
        assert!(lease1.is_new_device);

        let lease2 = parser.parse_dhcp_packet(&pkt2, mac2, 2000).unwrap();
        assert!(lease2.is_new_device);

        // 第二次同一MAC
        let lease1b = parser.parse_dhcp_packet(&pkt1, mac1, 3000).unwrap();
        assert!(!lease1b.is_new_device);
    }
}
