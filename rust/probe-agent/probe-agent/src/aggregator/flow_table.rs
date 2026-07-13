use super::community_id;
use super::online_stats::OnlineStats;
use dashmap::DashMap;
use std::hash::{Hash, Hasher};
use std::net::IpAddr;
use std::sync::atomic::{AtomicU16, AtomicU32, AtomicU64, AtomicU8, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};
use tracing::warn;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum TosUpdatePolicy {
    FirstNonZero,
    HighestDscp,
    LastSeen,
    Bitmap,
}

impl Default for TosUpdatePolicy {
    fn default() -> Self {
        TosUpdatePolicy::HighestDscp
    }
}

impl TosUpdatePolicy {
    pub fn as_str(&self) -> &'static str {
        match self {
            TosUpdatePolicy::FirstNonZero => "first_non_zero",
            TosUpdatePolicy::HighestDscp => "highest_dscp",
            TosUpdatePolicy::LastSeen => "last_seen",
            TosUpdatePolicy::Bitmap => "bitmap",
        }
    }
    pub fn from_str(s: &str) -> Option<Self> {
        match s {
            "first_non_zero" => Some(TosUpdatePolicy::FirstNonZero),
            "highest_dscp" => Some(TosUpdatePolicy::HighestDscp),
            "last_seen" => Some(TosUpdatePolicy::LastSeen),
            "bitmap" => Some(TosUpdatePolicy::Bitmap),
            _ => None,
        }
    }
}

#[derive(Clone, Debug)]
pub struct FlowKey {
    pub src_ip: IpAddr,
    pub dst_ip: IpAddr,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
    pub is_forward: bool,
    cached_hash: u64,
    cached_community_id: std::sync::OnceLock<String>,
}

impl FlowKey {
    pub fn new(src_ip: IpAddr, dst_ip: IpAddr, src_port: u16, dst_port: u16, protocol: u8) -> Self {
        let (a_ip, a_port, b_ip, b_port, is_fwd) =
            if src_ip < dst_ip || (src_ip == dst_ip && src_port <= dst_port) {
                (src_ip, src_port, dst_ip, dst_port, true)
            } else {
                (dst_ip, dst_port, src_ip, src_port, false)
            };

        let mut hasher = ahash::AHasher::default();
        a_ip.hash(&mut hasher);
        b_ip.hash(&mut hasher);
        a_port.hash(&mut hasher);
        b_port.hash(&mut hasher);
        protocol.hash(&mut hasher);

        Self {
            src_ip: a_ip,
            dst_ip: b_ip,
            src_port: a_port,
            dst_port: b_port,
            protocol,
            is_forward: is_fwd,
            cached_hash: hasher.finish(),
            cached_community_id: std::sync::OnceLock::new(),
        }
    }

    #[inline]
    pub fn normalize(&self) -> &Self {
        self
    }

    #[inline]
    pub fn is_forward(&self) -> bool {
        self.is_forward
    }

    #[inline]
    pub fn cached_hash(&self) -> u64 {
        self.cached_hash
    }

    pub fn community_id(&self) -> &str {
        self.cached_community_id.get_or_init(|| {
            community_id::compute_community_id(
                self.src_ip,
                self.dst_ip,
                self.src_port,
                self.dst_port,
                self.protocol,
            )
        })
    }
}

impl PartialEq for FlowKey {
    fn eq(&self, other: &Self) -> bool {
        self.cached_hash == other.cached_hash
            && self.protocol == other.protocol
            && self.src_port == other.src_port
            && self.dst_port == other.dst_port
            && self.src_ip == other.src_ip
            && self.dst_ip == other.dst_ip
    }
}

impl Eq for FlowKey {}

impl Hash for FlowKey {
    #[inline]
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.cached_hash.hash(state);
    }
}

pub struct FastStats {
    count: AtomicU64,
    sum: AtomicU64,
    sum_sq: AtomicU64,
    min: AtomicU32,
    max: AtomicU32,
}

impl FastStats {
    pub const fn new() -> Self {
        Self {
            count: AtomicU64::new(0),
            sum: AtomicU64::new(0),
            sum_sq: AtomicU64::new(0),
            min: AtomicU32::new(u32::MAX),
            max: AtomicU32::new(0),
        }
    }

    #[inline(always)]
    pub fn update(&self, value: u32) {
        self.count.fetch_add(1, Ordering::Relaxed);
        self.sum.fetch_add(value as u64, Ordering::Relaxed);
        self.sum_sq
            .fetch_add((value as u64) * (value as u64), Ordering::Relaxed);

        let mut current_min = self.min.load(Ordering::Acquire);
        while value < current_min {
            match self.min.compare_exchange_weak(
                current_min,
                value,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => break,
                Err(c) => current_min = c,
            }
        }

        let mut current_max = self.max.load(Ordering::Acquire);
        while value > current_max {
            match self.max.compare_exchange_weak(
                current_max,
                value,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => break,
                Err(c) => current_max = c,
            }
        }
    }

    #[inline]
    pub fn count(&self) -> u64 {
        self.count.load(Ordering::Relaxed)
    }

    #[inline]
    pub fn sum(&self) -> u64 {
        self.sum.load(Ordering::Relaxed)
    }

    #[inline]
    pub fn mean(&self) -> f32 {
        let count = self.count.load(Ordering::Relaxed);
        if count == 0 {
            return 0.0;
        }
        self.sum.load(Ordering::Relaxed) as f32 / count as f32
    }

    #[inline]
    pub fn std(&self) -> f32 {
        let count = self.count.load(Ordering::Relaxed);
        if count <= 1 {
            return 0.0;
        }
        let mean = self.mean();
        let sum_sq = self.sum_sq.load(Ordering::Relaxed) as f32;
        let variance = (sum_sq / count as f32) - (mean * mean);
        if variance > 0.0 {
            variance.sqrt()
        } else {
            0.0
        }
    }

    #[inline]
    pub fn min(&self) -> u32 {
        let v = self.min.load(Ordering::Acquire);
        if v == u32::MAX {
            0
        } else {
            v
        }
    }

    #[inline]
    pub fn max(&self) -> u32 {
        self.max.load(Ordering::Acquire)
    }

    #[inline]
    pub fn min_float(&self) -> f32 {
        self.min() as f32 / 1000.0
    }

    #[inline]
    pub fn max_float(&self) -> f32 {
        self.max() as f32 / 1000.0
    }

    #[inline]
    pub fn mean_float(&self) -> f32 {
        self.mean() / 1000.0
    }

    #[inline]
    pub fn std_float(&self) -> f32 {
        self.std() / 1000.0
    }
}

pub struct FlowValue {
    pub start_time: AtomicU64,
    pub last_seen: AtomicU64,
    pub packets_fwd: AtomicU64,
    pub packets_bwd: AtomicU64,
    pub bytes_fwd: AtomicU64,
    pub bytes_bwd: AtomicU64,
    pub tcp_flags_fwd: AtomicU16,
    pub tcp_flags_bwd: AtomicU16,
    pub pktlen_stats: FastStats,
    pub iat_fwd_stats: FastStats,
    pub iat_bwd_stats: FastStats,
    pub last_pkt_time_fwd: AtomicU64,
    pub last_pkt_time_bwd: AtomicU64,
    pub tos: AtomicU8,
    pub dscp_bitmap: AtomicU64,
    pub active_stats: OnlineStats,
    pub idle_stats: OnlineStats,
    pub tos_update_policy: TosUpdatePolicy,
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
            pktlen_stats: FastStats::new(),
            iat_fwd_stats: FastStats::new(),
            iat_bwd_stats: FastStats::new(),
            last_pkt_time_fwd: AtomicU64::new(0),
            last_pkt_time_bwd: AtomicU64::new(0),
            tos: AtomicU8::new(0),
            dscp_bitmap: AtomicU64::new(0),
            active_stats: OnlineStats::new(),
            idle_stats: OnlineStats::new(),
            tos_update_policy: TosUpdatePolicy::default(),
        }
    }
}

impl FlowValue {
    pub fn with_policy(policy: TosUpdatePolicy) -> Self {
        let mut value = Self::default();
        value.tos_update_policy = policy;
        value
    }

    pub fn set_tos_policy(&mut self, policy: TosUpdatePolicy) {
        self.tos_update_policy = policy;
    }

    pub fn duration_ms(&self) -> u32 {
        let start = self.start_time.load(Ordering::Relaxed);
        let end = self.last_seen.load(Ordering::Relaxed);
        (end.saturating_sub(start)) as u32
    }

    pub fn total_packets(&self) -> u64 {
        self.packets_fwd.load(Ordering::Relaxed) + self.packets_bwd.load(Ordering::Relaxed)
    }

    pub fn total_bytes(&self) -> u64 {
        self.bytes_fwd.load(Ordering::Relaxed) + self.bytes_bwd.load(Ordering::Relaxed)
    }

    pub fn get_tos(&self) -> u8 {
        self.tos.load(Ordering::Acquire)
    }

    pub fn get_dscp(&self) -> u8 {
        self.get_tos() >> 2
    }

    pub fn get_ecn(&self) -> u8 {
        self.get_tos() & 0x03
    }

    pub fn get_dscp_bitmap(&self) -> u64 {
        self.dscp_bitmap.load(Ordering::Acquire)
    }

    pub fn get_all_seen_dscp_values(&self) -> Vec<u8> {
        let bitmap = self.get_dscp_bitmap();
        let mut dscp_values = Vec::new();
        for dscp in 0..64 {
            if (bitmap & (1u64 << dscp)) != 0 {
                dscp_values.push(dscp);
            }
        }
        dscp_values
    }

    pub fn get_highest_seen_dscp(&self) -> Option<u8> {
        let bitmap = self.get_dscp_bitmap();
        for dscp in (0..64).rev() {
            if (bitmap & (1u64 << dscp)) != 0 {
                return Some(dscp);
            }
        }
        None
    }

    pub fn update_tos(&self, packet_tos: u8) {
        if packet_tos == 0 {
            return;
        }
        match self.tos_update_policy {
            TosUpdatePolicy::FirstNonZero => {
                self.update_tos_first_non_zero(packet_tos);
            }
            TosUpdatePolicy::HighestDscp => {
                self.update_tos_highest_dscp(packet_tos);
            }
            TosUpdatePolicy::LastSeen => {
                self.update_tos_last_seen(packet_tos);
            }
            TosUpdatePolicy::Bitmap => {
                self.update_tos_bitmap(packet_tos);
            }
        }
    }

    fn update_tos_first_non_zero(&self, packet_tos: u8) {
        let current = self.tos.load(Ordering::Acquire);
        if current == 0 {
            self.tos
                .compare_exchange(0, packet_tos, Ordering::AcqRel, Ordering::Acquire)
                .ok();
        }
    }

    fn update_tos_highest_dscp(&self, packet_tos: u8) {
        let packet_dscp = packet_tos >> 2;
        let packet_ecn = packet_tos & 0x03;
        let mut current = self.tos.load(Ordering::Acquire);
        loop {
            let current_dscp = current >> 2;
            if packet_dscp <= current_dscp {
                break;
            }
            let new_tos = (packet_dscp << 2) | packet_ecn;
            match self.tos.compare_exchange_weak(
                current,
                new_tos,
                Ordering::AcqRel,
                Ordering::Acquire,
            ) {
                Ok(_) => break,
                Err(c) => current = c,
            }
        }
    }

    fn update_tos_last_seen(&self, packet_tos: u8) {
        self.tos.store(packet_tos, Ordering::Release);
    }

    fn update_tos_bitmap(&self, packet_tos: u8) {
        let dscp = packet_tos >> 2;
        if dscp < 64 {
            self.dscp_bitmap.fetch_or(1u64 << dscp, Ordering::Relaxed);
        }
        let current = self.tos.load(Ordering::Acquire);
        if current == 0 {
            self.tos.store(packet_tos, Ordering::Release);
        }
    }

    pub fn up_down_ratio(&self) -> f32 {
        let bytes_up = self.bytes_fwd.load(Ordering::Relaxed) as f32;
        let bytes_down = self.bytes_bwd.load(Ordering::Relaxed) as f32;
        if bytes_down == 0.0 {
            if bytes_up == 0.0 {
                0.0
            } else {
                f32::INFINITY
            }
        } else {
            bytes_up / bytes_down
        }
    }

    pub fn pps(&self) -> f32 {
        let duration_sec = self.duration_ms() as f32 / 1000.0;
        if duration_sec <= 0.0 {
            return 0.0;
        }
        self.total_packets() as f32 / duration_sec
    }

    pub fn bps(&self) -> f32 {
        let duration_sec = self.duration_ms() as f32 / 1000.0;
        if duration_sec <= 0.0 {
            return 0.0;
        }
        self.total_bytes() as f32 / duration_sec
    }

    pub fn mbps(&self) -> f32 {
        self.bps() / 1_000_000.0 * 8.0
    }
}

#[derive(Debug, Clone, Copy)]
pub struct PacketInfo {
    pub len: u16,
    pub tcp_flags: u8,
    pub is_forward: bool,
    pub timestamp: u64,
    pub tos: u8,
}

impl Default for PacketInfo {
    fn default() -> Self {
        Self {
            len: 0,
            tcp_flags: 0,
            is_forward: true,
            timestamp: 0,
            tos: 0,
        }
    }
}

impl PacketInfo {
    pub fn new(len: u16, tcp_flags: u8, is_forward: bool, timestamp: u64, tos: u8) -> Self {
        Self {
            len,
            tcp_flags,
            is_forward,
            timestamp,
            tos,
        }
    }

    pub fn without_tos(len: u16, tcp_flags: u8, is_forward: bool, timestamp: u64) -> Self {
        Self {
            len,
            tcp_flags,
            is_forward,
            timestamp,
            tos: 0,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UpdateResult {
    Updated,
    NewFlow,
}

pub struct FlowTable {
    map: DashMap<FlowKey, FlowValue>,
    tos_policy: TosUpdatePolicy,
}

impl FlowTable {
    pub fn new(capacity: usize) -> Self {
        Self {
            map: DashMap::with_capacity(capacity),
            tos_policy: TosUpdatePolicy::default(),
        }
    }

    pub fn with_tos_policy(capacity: usize, policy: TosUpdatePolicy) -> Self {
        Self {
            map: DashMap::with_capacity(capacity),
            tos_policy: policy,
        }
    }

    pub fn set_tos_policy(&mut self, policy: TosUpdatePolicy) {
        self.tos_policy = policy;
    }

    pub fn update(&self, key: &FlowKey, packet: &PacketInfo) -> UpdateResult {
        self.update_with_time(key, packet, 0)
    }

    pub fn update_with_time(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
        now_ms: u64,
    ) -> UpdateResult {
        use dashmap::mapref::entry::Entry;
        match self.map.entry(key.clone()) {
            Entry::Occupied(entry) => {
                self.update_flow_value_with_time(entry.get(), packet, now_ms);
                UpdateResult::Updated
            }
            Entry::Vacant(entry) => {
                let mut value = FlowValue::default();
                value.set_tos_policy(self.tos_policy);
                if now_ms > 0 {
                    value.start_time.store(now_ms, Ordering::Relaxed);
                    value.last_seen.store(now_ms, Ordering::Relaxed);
                }
                self.update_flow_value_with_time(&value, packet, now_ms);
                entry.insert(value);
                UpdateResult::NewFlow
            }
        }
    }

    pub fn insert_with_value(&self, key: FlowKey, value: FlowValue) -> Option<FlowValue> {
        self.map.insert(key, value)
    }

    fn update_flow_value(&self, value: &FlowValue, packet: &PacketInfo) {
        self.update_flow_value_with_time(value, packet, 0)
    }

    fn update_flow_value_with_time(
        &self,
        value: &FlowValue,
        packet: &PacketInfo,
        batch_time_ms: u64,
    ) {
        let now_ms = if batch_time_ms > 0 {
            batch_time_ms
        } else {
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64
        };

        let timestamp_ms = if packet.timestamp > 0 {
            let ts_ms = packet.timestamp / 1000;
            if ts_ms > now_ms + 60_000 || (now_ms > 86_400_000 && ts_ms < now_ms - 86_400_000) {
                warn!(
                    "Invalid packet timestamp: ts_ms={}, now_ms={}, using system time",
                    ts_ms, now_ms
                );
                now_ms
            } else {
                ts_ms
            }
        } else {
            now_ms
        };

        let last_seen = value.last_seen.load(Ordering::Relaxed);
        let packet_count =
            value.packets_fwd.load(Ordering::Relaxed) + value.packets_bwd.load(Ordering::Relaxed);

        if packet_count > 0 && timestamp_ms > last_seen {
            let interval_ms = timestamp_ms - last_seen;
            const IDLE_THRESHOLD_MS: u64 = 1000;
            const MAX_INTERVAL_MS: u64 = u32::MAX as u64;
            const MAX_REASONABLE_INTERVAL_MS: u64 = 3600_000;

            if interval_ms > MAX_REASONABLE_INTERVAL_MS {
                tracing::trace!("Ignoring unreasonable interval: {}ms (> 1h)", interval_ms);
            } else if interval_ms > IDLE_THRESHOLD_MS {
                value
                    .idle_stats
                    .update(interval_ms.min(MAX_INTERVAL_MS) as u32);
            } else if interval_ms > 0 {
                value
                    .active_stats
                    .update(interval_ms.min(MAX_INTERVAL_MS) as u32);
            }
        }

        value.last_seen.store(timestamp_ms, Ordering::Relaxed);

        if packet.is_forward {
            value.packets_fwd.fetch_add(1, Ordering::Relaxed);
            value
                .bytes_fwd
                .fetch_add(packet.len as u64, Ordering::Relaxed);
            value
                .tcp_flags_fwd
                .fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);

            let last = value
                .last_pkt_time_fwd
                .swap(packet.timestamp, Ordering::Relaxed);
            if last > 0 {
                if packet.timestamp >= last {
                    let iat = packet.timestamp.saturating_sub(last);
                    const MAX_REASONABLE_IAT_US: u64 = 3_600_000_000;
                    if iat > MAX_REASONABLE_IAT_US {
                        tracing::trace!("Ignoring unreasonable IAT (fwd): {}us (> 1h)", iat);
                    } else {
                        value.iat_fwd_stats.update(iat as u32);
                    }
                } else {
                    tracing::warn!(
                        "Time went backwards (fwd): last={}, current={}",
                        last,
                        packet.timestamp
                    );
                }
            }
        } else {
            value.packets_bwd.fetch_add(1, Ordering::Relaxed);
            value
                .bytes_bwd
                .fetch_add(packet.len as u64, Ordering::Relaxed);
            value
                .tcp_flags_bwd
                .fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);

            let last = value
                .last_pkt_time_bwd
                .swap(packet.timestamp, Ordering::Relaxed);
            if last > 0 {
                if packet.timestamp >= last {
                    let iat = packet.timestamp.saturating_sub(last);
                    const MAX_REASONABLE_IAT_US: u64 = 3_600_000_000;
                    if iat > MAX_REASONABLE_IAT_US {
                        tracing::trace!("Ignoring unreasonable IAT (bwd): {}us (> 1h)", iat);
                    } else {
                        value.iat_bwd_stats.update(iat as u32);
                    }
                } else {
                    tracing::warn!(
                        "Time went backwards (bwd): last={}, current={}",
                        last,
                        packet.timestamp
                    );
                }
            }
        }

        value.pktlen_stats.update(packet.len as u32);
        value.update_tos(packet.tos);
    }

    pub fn remove(&self, key: &FlowKey) -> Option<(FlowKey, FlowValue)> {
        self.map.remove(key)
    }

    pub fn iter(
        &self,
    ) -> impl Iterator<Item = dashmap::mapref::multiple::RefMulti<'_, FlowKey, FlowValue>> + '_
    {
        self.map.iter()
    }

    pub fn len(&self) -> usize {
        self.map.len()
    }

    pub fn is_empty(&self) -> bool {
        self.map.is_empty()
    }

    pub fn clear(&self) {
        self.map.clear();
    }

    pub fn capacity(&self) -> usize {
        self.map.capacity()
    }

    pub fn get(&self, key: &FlowKey) -> Option<dashmap::mapref::one::Ref<'_, FlowKey, FlowValue>> {
        self.map.get(key)
    }

    pub fn contains_key(&self, key: &FlowKey) -> bool {
        self.map.contains_key(key)
    }
}
