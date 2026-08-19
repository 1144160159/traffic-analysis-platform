use super::community_id;
use super::online_stats::OnlineStats;
use crate::parser::security::{
    ObservedSecurityProtocol, PacketFeatureObservation, SecurityObservation,
};
use dashmap::DashMap;
use parking_lot::Mutex;
use std::collections::BTreeSet;
use std::hash::{Hash, Hasher};
use std::net::IpAddr;
use std::sync::atomic::{AtomicBool, AtomicU16, AtomicU32, AtomicU64, AtomicU8, Ordering};

pub const FLOW_IDENTITY_REVISION: u8 = 2;
pub const OBSERVATION_SCOPE_REVISION: u8 = 1;

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum PacketDirection {
    Forward,
    Backward,
}

impl PacketDirection {
    #[inline]
    pub fn is_forward(self) -> bool {
        matches!(self, Self::Forward)
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ObservedEndpoints {
    pub src_ip: IpAddr,
    pub dst_ip: IpAddr,
    pub src_port: u16,
    pub dst_port: u16,
    pub protocol: u8,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub struct CommunityTuple {
    pub ip_a: IpAddr,
    pub ip_b: IpAddr,
    pub port_a: u16,
    pub port_b: u16,
    pub protocol: u8,
    pub one_way: bool,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct CanonicalFlowIdentity {
    pub community_tuple: CommunityTuple,
    pub packet_direction: PacketDirection,
    pub reversible: bool,
    pub revision: u8,
}

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FlowIdentityError {
    #[error("mixed IP address families are not a valid flow identity")]
    MixedAddressFamilies,
}

/// Produces the endpoint order, packet direction, and Community ID input exactly
/// once. In particular, unpaired ICMP messages retain their observed direction
/// instead of being fabricated into a request/reply pair.
pub fn canonicalize_observation(
    observed: ObservedEndpoints,
) -> Result<CanonicalFlowIdentity, FlowIdentityError> {
    if std::mem::discriminant(&observed.src_ip) != std::mem::discriminant(&observed.dst_ip) {
        return Err(FlowIdentityError::MixedAddressFamilies);
    }

    let icmp_pair = if matches!(observed.protocol, 1 | 58) {
        community_id::icmp_pair(observed.protocol, observed.src_port as u8)
    } else {
        None
    };
    let one_way = matches!(observed.protocol, 1 | 58) && icmp_pair.is_none();
    let reversible = !one_way;
    let source_is_a = if let Some(pair) = icmp_pair {
        observed.src_ip < observed.dst_ip || (observed.src_ip == observed.dst_ip && pair.is_request)
    } else {
        observed.src_ip < observed.dst_ip
            || (observed.src_ip == observed.dst_ip && observed.src_port <= observed.dst_port)
    };

    let (ip_a, ip_b, packet_direction) = if one_way || source_is_a {
        (observed.src_ip, observed.dst_ip, PacketDirection::Forward)
    } else {
        (observed.dst_ip, observed.src_ip, PacketDirection::Backward)
    };
    let (port_a, port_b) = match icmp_pair {
        Some(pair) => (pair.request_type as u16, observed.dst_port),
        None if one_way => (observed.src_port, observed.dst_port),
        None if source_is_a => (observed.src_port, observed.dst_port),
        None => (observed.dst_port, observed.src_port),
    };

    Ok(CanonicalFlowIdentity {
        community_tuple: CommunityTuple {
            ip_a,
            ip_b,
            port_a,
            port_b,
            protocol: observed.protocol,
            one_way,
        },
        packet_direction,
        reversible,
        revision: FLOW_IDENTITY_REVISION,
    })
}

#[derive(Clone, Copy, Debug, Default, PartialEq, Eq, Hash)]
pub enum ScopePolicy {
    #[default]
    GlobalL3,
    Interface,
    InterfaceAndVlan,
    FullObservation,
}

#[derive(Clone, Debug, PartialEq, Eq, Hash)]
pub struct ObservationScope {
    pub policy: ScopePolicy,
    pub interface: Option<String>,
    pub vlan_stack: Vec<u16>,
    pub queue_id: Option<u32>,
    pub tap_id: Option<String>,
    pub revision: u8,
}

impl Default for ObservationScope {
    fn default() -> Self {
        Self {
            policy: ScopePolicy::GlobalL3,
            interface: None,
            vlan_stack: Vec::new(),
            queue_id: None,
            tap_id: None,
            revision: OBSERVATION_SCOPE_REVISION,
        }
    }
}

impl ObservationScope {
    pub fn global_l3() -> Self {
        Self {
            revision: OBSERVATION_SCOPE_REVISION,
            ..Self::default()
        }
    }

    fn stable_discriminator(&self) -> Vec<u8> {
        fn push_string(output: &mut Vec<u8>, value: Option<&str>) {
            let bytes = value.unwrap_or_default().as_bytes();
            output.extend_from_slice(&(bytes.len() as u32).to_be_bytes());
            output.extend_from_slice(bytes);
        }

        let mut output = vec![self.revision, self.policy as u8];
        match self.policy {
            ScopePolicy::GlobalL3 => {}
            ScopePolicy::Interface => push_string(&mut output, self.interface.as_deref()),
            ScopePolicy::InterfaceAndVlan => {
                push_string(&mut output, self.interface.as_deref());
                output.push(self.vlan_stack.len().min(u8::MAX as usize) as u8);
                for vlan in &self.vlan_stack {
                    output.extend_from_slice(&vlan.to_be_bytes());
                }
            }
            ScopePolicy::FullObservation => {
                push_string(&mut output, self.interface.as_deref());
                output.push(self.vlan_stack.len().min(u8::MAX as usize) as u8);
                for vlan in &self.vlan_stack {
                    output.extend_from_slice(&vlan.to_be_bytes());
                }
                output.extend_from_slice(&self.queue_id.unwrap_or_default().to_be_bytes());
                push_string(&mut output, self.tap_id.as_deref());
            }
        }
        output
    }

    /// Public accessor for the stable scope discriminator bytes (interface /
    /// VLAN / queue / tap identity), used by event identity derivation.
    pub fn stable_discriminator_bytes(&self) -> Vec<u8> {
        self.stable_discriminator()
    }
}

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
    community_tuple: CommunityTuple,
    scope_discriminator: Vec<u8>,
    cached_hash: u64,
    cached_community_id: std::sync::OnceLock<community_id::CommunityId>,
}

impl FlowKey {
    pub fn new(identity: &CanonicalFlowIdentity, scope: &ObservationScope) -> Self {
        let tuple = identity.community_tuple;
        let scope_discriminator = scope.stable_discriminator();
        let cached_hash = stable_flow_hash(&tuple, &scope_discriminator);

        Self {
            src_ip: tuple.ip_a,
            dst_ip: tuple.ip_b,
            src_port: tuple.port_a,
            dst_port: tuple.port_b,
            protocol: tuple.protocol,
            community_tuple: tuple,
            scope_discriminator,
            cached_hash,
            cached_community_id: std::sync::OnceLock::new(),
        }
    }

    #[inline]
    pub fn normalize(&self) -> &Self {
        self
    }

    #[inline]
    pub fn cached_hash(&self) -> u64 {
        self.cached_hash
    }

    /// The stable observation-scope discriminator (interface / VLAN / queue /
    /// tap) carried by this flow key.
    pub fn scope_discriminator(&self) -> &[u8] {
        &self.scope_discriminator
    }

    pub fn community_id(&self) -> &str {
        self.cached_community_id
            .get_or_init(|| community_id::compute_community_id(&self.community_tuple))
            .as_str()
    }
}

pub struct FlowAggregationKey;

impl FlowAggregationKey {
    pub fn new(identity: &CanonicalFlowIdentity, scope: &ObservationScope) -> FlowKey {
        FlowKey::new(identity, scope)
    }
}

fn stable_flow_hash(tuple: &CommunityTuple, scope: &[u8]) -> u64 {
    fn write(hash: &mut u64, bytes: &[u8]) {
        for byte in bytes {
            *hash ^= *byte as u64;
            *hash = hash.wrapping_mul(0x100000001b3);
        }
    }

    let mut hash = 0xcbf29ce484222325;
    match tuple.ip_a {
        IpAddr::V4(address) => write(&mut hash, &address.octets()),
        IpAddr::V6(address) => write(&mut hash, &address.octets()),
    }
    match tuple.ip_b {
        IpAddr::V4(address) => write(&mut hash, &address.octets()),
        IpAddr::V6(address) => write(&mut hash, &address.octets()),
    }
    write(&mut hash, &tuple.port_a.to_be_bytes());
    write(&mut hash, &tuple.port_b.to_be_bytes());
    write(&mut hash, &[tuple.protocol, tuple.one_way as u8]);
    write(&mut hash, scope);
    hash
}

impl PartialEq for FlowKey {
    fn eq(&self, other: &Self) -> bool {
        self.cached_hash == other.cached_hash
            && self.protocol == other.protocol
            && self.src_port == other.src_port
            && self.dst_port == other.dst_port
            && self.src_ip == other.src_ip
            && self.dst_ip == other.dst_ip
            && self.scope_discriminator == other.scope_discriminator
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
    pub feature_state: FlowFeatureState,
}

pub const MAX_FEATURE_SEQUENCE_POINTS: usize = 256;

#[derive(Clone, Debug, PartialEq, Eq, PartialOrd, Ord)]
struct SequencePoint {
    event_time_us: i64,
    signed_packet_length: i32,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct FlowSecuritySnapshot {
    pub protocol: Option<ObservedSecurityProtocol>,
    pub tls_version: Option<String>,
    pub ja3: Option<String>,
    pub ja4: Option<String>,
    pub sni: Option<String>,
    pub cert_sha256: Option<String>,
    pub cert_is_self_signed: Option<bool>,
    pub pubkey_len: Option<u32>,
    pub quic_version: Option<String>,
    pub truncated: bool,
    pub conflict: bool,
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct FlowFeatureSnapshot {
    pub signed_packet_lengths: Vec<i32>,
    pub packet_event_time_us: Vec<i64>,
    pub payload_nibble_counts: Vec<u64>,
    pub payload_observed_bytes: u64,
    pub sequence_truncated: bool,
    pub security: FlowSecuritySnapshot,
}

#[derive(Default)]
struct FlowSecurityState {
    snapshot: FlowSecuritySnapshot,
}

pub struct FlowFeatureState {
    sequence: Mutex<Vec<SequencePoint>>,
    payload_nibble_counts: [AtomicU64; 16],
    payload_observed_bytes: AtomicU64,
    sequence_truncated: AtomicBool,
    security: Mutex<FlowSecurityState>,
}

impl Default for FlowFeatureState {
    fn default() -> Self {
        Self {
            sequence: Mutex::new(Vec::with_capacity(MAX_FEATURE_SEQUENCE_POINTS)),
            payload_nibble_counts: std::array::from_fn(|_| AtomicU64::new(0)),
            payload_observed_bytes: AtomicU64::new(0),
            sequence_truncated: AtomicBool::new(false),
            security: Mutex::new(FlowSecurityState::default()),
        }
    }
}

impl FlowFeatureState {
    pub fn observe(&self, observation: &PacketFeatureObservation) {
        {
            let mut sequence = self.sequence.lock();
            sequence.push(SequencePoint {
                event_time_us: observation.event_time_us,
                signed_packet_length: observation.signed_packet_length,
            });
            sequence.sort_unstable();
            if sequence.len() > MAX_FEATURE_SEQUENCE_POINTS {
                sequence.truncate(MAX_FEATURE_SEQUENCE_POINTS);
                self.sequence_truncated.store(true, Ordering::Release);
            }
        }
        for (target, value) in self
            .payload_nibble_counts
            .iter()
            .zip(observation.payload_nibble_counts)
        {
            target.fetch_add(value, Ordering::Relaxed);
        }
        self.payload_observed_bytes
            .fetch_add(observation.payload_observed_bytes, Ordering::Relaxed);
        self.merge_security(&observation.security);
    }

    fn merge_security(&self, observation: &SecurityObservation) {
        if observation == &SecurityObservation::default() {
            return;
        }
        let mut state = self.security.lock();
        state.snapshot.conflict |=
            merge_optional(&mut state.snapshot.protocol, observation.protocol);
        state.snapshot.conflict |= merge_optional_string(
            &mut state.snapshot.tls_version,
            observation.tls_version.as_deref(),
        );
        state.snapshot.conflict |=
            merge_optional_string(&mut state.snapshot.ja3, observation.ja3.as_deref());
        state.snapshot.conflict |=
            merge_optional_string(&mut state.snapshot.ja4, observation.ja4.as_deref());
        state.snapshot.conflict |=
            merge_optional_string(&mut state.snapshot.sni, observation.sni.as_deref());
        state.snapshot.conflict |= merge_optional_string(
            &mut state.snapshot.cert_sha256,
            observation.cert_sha256.as_deref(),
        );
        state.snapshot.conflict |= merge_optional(
            &mut state.snapshot.cert_is_self_signed,
            observation.cert_is_self_signed,
        );
        state.snapshot.conflict |=
            merge_optional(&mut state.snapshot.pubkey_len, observation.pubkey_len);
        state.snapshot.conflict |= merge_optional_string(
            &mut state.snapshot.quic_version,
            observation.quic_version.as_deref(),
        );
        state.snapshot.truncated |= observation.truncated;
    }

    pub fn snapshot(&self) -> FlowFeatureSnapshot {
        let mut sequence = self.sequence.lock().clone();
        sequence.sort_unstable();
        let signed_packet_lengths = sequence
            .iter()
            .map(|point| point.signed_packet_length)
            .collect();
        let packet_event_time_us = sequence.iter().map(|point| point.event_time_us).collect();
        FlowFeatureSnapshot {
            signed_packet_lengths,
            packet_event_time_us,
            payload_nibble_counts: self
                .payload_nibble_counts
                .iter()
                .map(|value| value.load(Ordering::Acquire))
                .collect(),
            payload_observed_bytes: self.payload_observed_bytes.load(Ordering::Acquire),
            sequence_truncated: self.sequence_truncated.load(Ordering::Acquire),
            security: self.security.lock().snapshot.clone(),
        }
    }
}

fn merge_optional<T: Copy + Ord>(target: &mut Option<T>, incoming: Option<T>) -> bool {
    if let Some(incoming) = incoming {
        match target {
            None => *target = Some(incoming),
            Some(current) if *current != incoming => {
                *current = (*current).min(incoming);
                return true;
            }
            Some(_) => {}
        }
    }
    false
}

fn merge_optional_string(target: &mut Option<String>, incoming: Option<&str>) -> bool {
    if let Some(incoming) = incoming.filter(|value| !value.is_empty()) {
        match target {
            None => *target = Some(incoming.to_owned()),
            Some(current) if current != incoming => {
                let chosen = BTreeSet::from([current.as_str(), incoming])
                    .into_iter()
                    .next()
                    .unwrap();
                *current = chosen.to_owned();
                return true;
            }
            Some(_) => {}
        }
    }
    false
}

impl Default for FlowValue {
    fn default() -> Self {
        Self {
            start_time: AtomicU64::new(0),
            last_seen: AtomicU64::new(0),
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
            feature_state: FlowFeatureState::default(),
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

    /// Applies capture event time without consulting or repairing from the
    /// processing clock. A reordered sample can move `start_time` earlier but
    /// can never move the global or per-direction high-water marks backwards.
    pub fn apply_event_time(
        &self,
        timestamp_micros: u64,
        direction: PacketDirection,
    ) -> Result<EventTimeTransition, EventTimeError> {
        if timestamp_micros == 0 {
            return Err(EventTimeError::MissingTimestamp);
        }
        let timestamp_ms = timestamp_micros / 1_000;
        if timestamp_ms == 0 {
            return Err(EventTimeError::BeforeMillisecondEpoch);
        }

        atomic_min_nonzero(&self.start_time, timestamp_ms);
        let prior_end_ms = atomic_max(&self.last_seen, timestamp_ms);
        if prior_end_ms > 0 && timestamp_ms > prior_end_ms {
            update_activity_interval(self, timestamp_ms - prior_end_ms);
        }

        let (direction_clock, iat_stats) = match direction {
            PacketDirection::Forward => (&self.last_pkt_time_fwd, &self.iat_fwd_stats),
            PacketDirection::Backward => (&self.last_pkt_time_bwd, &self.iat_bwd_stats),
        };
        let prior_direction_micros = atomic_max(direction_clock, timestamp_micros);
        let reordered = prior_direction_micros > timestamp_micros;
        if prior_direction_micros > 0 && timestamp_micros > prior_direction_micros {
            let iat = timestamp_micros - prior_direction_micros;
            const MAX_REASONABLE_IAT_US: u64 = 3_600_000_000;
            if iat <= MAX_REASONABLE_IAT_US {
                iat_stats.update(iat.min(u32::MAX as u64) as u32);
            }
        }

        Ok(EventTimeTransition {
            timestamp_micros,
            timestamp_ms,
            prior_end_ms,
            prior_direction_micros,
            reordered,
            duplicate: prior_direction_micros == timestamp_micros,
        })
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

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum EventTimeError {
    #[error("capture timestamp is required")]
    MissingTimestamp,
    #[error("capture timestamp precedes the first millisecond of the Unix epoch")]
    BeforeMillisecondEpoch,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct EventTimeTransition {
    pub timestamp_micros: u64,
    pub timestamp_ms: u64,
    pub prior_end_ms: u64,
    pub prior_direction_micros: u64,
    pub reordered: bool,
    pub duplicate: bool,
}

fn atomic_min_nonzero(target: &AtomicU64, candidate: u64) -> u64 {
    let mut current = target.load(Ordering::Acquire);
    while current == 0 || candidate < current {
        match target.compare_exchange_weak(current, candidate, Ordering::AcqRel, Ordering::Acquire)
        {
            Ok(previous) => return previous,
            Err(observed) => current = observed,
        }
    }
    current
}

fn atomic_max(target: &AtomicU64, candidate: u64) -> u64 {
    let mut current = target.load(Ordering::Acquire);
    while candidate > current {
        match target.compare_exchange_weak(current, candidate, Ordering::AcqRel, Ordering::Acquire)
        {
            Ok(previous) => return previous,
            Err(observed) => current = observed,
        }
    }
    current
}

fn update_activity_interval(value: &FlowValue, interval_ms: u64) {
    const IDLE_THRESHOLD_MS: u64 = 1_000;
    const MAX_REASONABLE_INTERVAL_MS: u64 = 3_600_000;
    if interval_ms == 0 || interval_ms > MAX_REASONABLE_INTERVAL_MS {
        return;
    }
    if interval_ms > IDLE_THRESHOLD_MS {
        value
            .idle_stats
            .update(interval_ms.min(u32::MAX as u64) as u32);
    } else {
        value
            .active_stats
            .update(interval_ms.min(u32::MAX as u64) as u32);
    }
}

#[derive(Debug, Clone, Copy)]
pub struct PacketInfo {
    pub len: u16,
    pub tcp_flags: u8,
    pub direction: PacketDirection,
    pub timestamp: u64,
    pub tos: u8,
}

impl Default for PacketInfo {
    fn default() -> Self {
        Self {
            len: 0,
            tcp_flags: 0,
            direction: PacketDirection::Forward,
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
            direction: if is_forward {
                PacketDirection::Forward
            } else {
                PacketDirection::Backward
            },
            timestamp,
            tos,
        }
    }

    pub fn without_tos(len: u16, tcp_flags: u8, is_forward: bool, timestamp: u64) -> Self {
        Self {
            len,
            tcp_flags,
            direction: if is_forward {
                PacketDirection::Forward
            } else {
                PacketDirection::Backward
            },
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

#[derive(Debug, thiserror::Error, PartialEq, Eq)]
pub enum FlowUpdateError {
    #[error(transparent)]
    EventTime(#[from] EventTimeError),
    #[error(
        "flow sample revision mismatch: identity={identity_revision}, scope={observation_scope_revision}"
    )]
    RevisionMismatch {
        identity_revision: u8,
        observation_scope_revision: u8,
    },
    #[error("flow table capacity exceeded: {capacity} entries")]
    CapacityExceeded { capacity: usize },
}

pub struct FlowTable {
    map: DashMap<FlowKey, FlowValue>,
    tos_policy: TosUpdatePolicy,
    capacity: usize,
    capacity_exceeded: AtomicU64,
}

impl FlowTable {
    pub fn new(capacity: usize) -> Self {
        Self {
            map: DashMap::with_capacity(capacity),
            tos_policy: TosUpdatePolicy::default(),
            capacity,
            capacity_exceeded: AtomicU64::new(0),
        }
    }

    pub fn with_tos_policy(capacity: usize, policy: TosUpdatePolicy) -> Self {
        Self {
            map: DashMap::with_capacity(capacity),
            tos_policy: policy,
            capacity,
            capacity_exceeded: AtomicU64::new(0),
        }
    }

    pub fn set_tos_policy(&mut self, policy: TosUpdatePolicy) {
        self.tos_policy = policy;
    }

    pub fn update(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
    ) -> Result<UpdateResult, FlowUpdateError> {
        self.update_with_feature(key, packet, None)
    }

    pub fn update_with_feature(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
        feature: Option<&PacketFeatureObservation>,
    ) -> Result<UpdateResult, FlowUpdateError> {
        self.update_inner(key, packet, 0, feature)
    }

    pub fn update_with_time(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
        now_ms: u64,
    ) -> Result<UpdateResult, FlowUpdateError> {
        self.update_inner(key, packet, now_ms, None)
    }

    fn update_inner(
        &self,
        key: &FlowKey,
        packet: &PacketInfo,
        now_ms: u64,
        feature: Option<&PacketFeatureObservation>,
    ) -> Result<UpdateResult, FlowUpdateError> {
        let timestamp_micros = packet_event_time(packet, now_ms)?;
        // Hard capacity guard for *new* flows. This must run BEFORE taking
        // the entry lock: `DashMap::len()` read-locks every shard, so calling
        // it while an `entry()` shard lock is held would deadlock. The check
        // is deliberately racy — a few concurrent insertions may overshoot —
        // which is acceptable for a growth backstop.
        if !self.map.contains_key(key) && self.map.len() >= self.capacity {
            self.capacity_exceeded.fetch_add(1, Ordering::Relaxed);
            return Err(FlowUpdateError::CapacityExceeded {
                capacity: self.capacity,
            });
        }
        use dashmap::mapref::entry::Entry;
        match self.map.entry(key.clone()) {
            Entry::Occupied(entry) => {
                self.update_flow_value(entry.get(), packet, timestamp_micros)?;
                if let Some(feature) = feature {
                    entry.get().feature_state.observe(feature);
                }
                Ok(UpdateResult::Updated)
            }
            Entry::Vacant(entry) => {
                let mut value = FlowValue::default();
                value.set_tos_policy(self.tos_policy);
                self.update_flow_value(&value, packet, timestamp_micros)?;
                if let Some(feature) = feature {
                    value.feature_state.observe(feature);
                }
                entry.insert(value);
                Ok(UpdateResult::NewFlow)
            }
        }
    }

    pub fn insert_with_value(&self, key: FlowKey, value: FlowValue) -> Option<FlowValue> {
        // Respect the hard capacity for insertions too (replay path).
        if !self.map.contains_key(&key) && self.map.len() >= self.capacity {
            self.capacity_exceeded.fetch_add(1, Ordering::Relaxed);
            return None;
        }
        self.map.insert(key, value)
    }

    /// Total number of new-flow insertions rejected because the table was at
    /// capacity. Exposed for observability of admission pressure.
    pub fn capacity_exceeded_count(&self) -> u64 {
        self.capacity_exceeded.load(Ordering::Relaxed)
    }

    fn update_flow_value(
        &self,
        value: &FlowValue,
        packet: &PacketInfo,
        timestamp_micros: u64,
    ) -> Result<(), FlowUpdateError> {
        value.apply_event_time(timestamp_micros, packet.direction)?;

        if packet.direction.is_forward() {
            value.packets_fwd.fetch_add(1, Ordering::Relaxed);
            value
                .bytes_fwd
                .fetch_add(packet.len as u64, Ordering::Relaxed);
            value
                .tcp_flags_fwd
                .fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);
        } else {
            value.packets_bwd.fetch_add(1, Ordering::Relaxed);
            value
                .bytes_bwd
                .fetch_add(packet.len as u64, Ordering::Relaxed);
            value
                .tcp_flags_bwd
                .fetch_or(packet.tcp_flags as u16, Ordering::Relaxed);
        }

        value.pktlen_stats.update(packet.len as u32);
        value.update_tos(packet.tos);
        Ok(())
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

fn packet_event_time(packet: &PacketInfo, fallback_ms: u64) -> Result<u64, FlowUpdateError> {
    let timestamp_micros = if packet.timestamp > 0 {
        packet.timestamp
    } else {
        fallback_ms.saturating_mul(1_000)
    };
    if timestamp_micros == 0 {
        Err(EventTimeError::MissingTimestamp.into())
    } else if timestamp_micros < 1_000 {
        Err(EventTimeError::BeforeMillisecondEpoch.into())
    } else {
        Ok(timestamp_micros)
    }
}
