use dashmap::DashMap;
use std::sync::atomic::{AtomicU64, AtomicU16, Ordering};
use std::net::IpAddr;
use std::time::{SystemTime, UNIX_EPOCH};

/// 流键（五元组）
#[derive(Clone, Hash, Eq, PartialEq, Debug)]
pub struct FlowKey {
    pub src_ip: IpAddr,
    pub dst_ip: IpAddr,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
}

impl FlowKey {
    /// 规范化流键（确保 src < dst）
    pub fn normalize(&self) -> Self {
        if self.src_ip < self.dst_ip 
            || (self.src_ip == self.dst_ip && self.src_port < self.dst_port) {
            self.clone()
        } else {
            Self {
                src_ip: self.dst_ip,
                dst_ip: self.src_ip,
                src_port: self.dst_port,
                dst_port: self.src_port,
                protocol: self.protocol,
            }
        }
    }

    /// 计算 community_id
    pub fn community_id(&self) -> String {
        super::community_id::compute_community_id(
            self.src_ip,
            self.dst_ip,
            self.src_port,
            self.dst_port,
            self.protocol,
        )
    }
}

/// 流值（统计信息，使用原子类型避免锁）
pub struct FlowValue {
    pub start_time: AtomicU64,   // Unix timestamp (ms)
    pub last_seen: AtomicU64,
    pub packets_fwd: AtomicU64,
    pub packets_bwd: AtomicU64,
    pub bytes_fwd: AtomicU64,
    pub bytes_bwd: AtomicU64,
    pub tcp_flags_fwd: AtomicU16,
    pub tcp_flags_bwd: AtomicU16,
}

impl Default for FlowValue {
    fn default() -> Self {
        let now_ms = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        
        Self {
            start_time: AtomicU64::new(now_ms),
            last_seen: AtomicU64::new(now_ms),
            packets_fwd: AtomicU64::new(0),
            packets_bwd: AtomicU64::new(0),
            bytes_fwd: AtomicU64::new(0),
            bytes_bwd: AtomicU64::new(0),
            tcp_flags_fwd: AtomicU16::new(0),
            tcp_flags_bwd: AtomicU16::new(0),
        }
    }
}

/// 包信息（用于更新流表）
pub struct PacketInfo {
    pub len: u16,
    pub tcp_flags: u8,
    pub is_forward: bool,  // 是否为正向（与 FlowKey 方向一致）
    pub timestamp: u64,    // Unix timestamp (ms)
}

/// 更新结果
pub enum UpdateResult {
    Updated,
    NewFlow,
}

/// 流表
pub struct FlowTable {
    map: DashMap<FlowKey, FlowValue>,
    capacity: usize,
}

impl FlowTable {
    pub fn new(capacity: usize) -> Self {
        Self {
            map: DashMap::with_capacity(capacity),
            capacity,
        }
    }

    /// 更新流表（原子操作，无锁）
    pub fn update(&self, key: &FlowKey, packet: &PacketInfo) -> UpdateResult {
        let normalized_key = key.normalize();
        
        let entry = self.map.entry(normalized_key).or_insert_with(Default::default);
        
        // 更新时间戳
        entry.last_seen.store(packet.timestamp, Ordering::Relaxed);
        
        // 更新计数器
        if packet.is_forward {
            entry.packets_fwd.fetch_add(1, Ordering::Relaxed);
            entry.bytes_fwd.fetch_add(packet.len as u64, Ordering::Relaxed);
            entry.tcp_flags_fwd.fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);
        } else {
            entry.packets_bwd.fetch_add(1, Ordering::Relaxed);
            entry.bytes_bwd.fetch_add(packet.len as u64, Ordering::Relaxed);
            entry.tcp_flags_bwd.fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);
        }
        
        UpdateResult::Updated
    }

    /// 移除并返回流
    pub fn remove(&self, key: &FlowKey) -> Option<(FlowKey, FlowValue)> {
        self.map.remove(&key.normalize())
    }

    /// 遍历所有流（用于老化扫描）
    pub fn iter(&self) -> impl Iterator<Item = dashmap::mapref::multiple::RefMulti<FlowKey, FlowValue>> + '_ {
        self.map.iter()
    }

    /// 当前流数量
    pub fn len(&self) -> usize {
        self.map.len()
    }

    /// 是否为空
    pub fn is_empty(&self) -> bool {
        self.map.is_empty()
    }

    /// 容量
    pub fn capacity(&self) -> usize {
        self.capacity
    }
}