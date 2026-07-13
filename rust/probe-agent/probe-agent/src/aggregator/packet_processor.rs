use super::community_id;
use super::flow_table::{FlowKey, PacketInfo, UpdateResult};
use super::partitioned_flow_table::PartitionedFlowTable;
use crate::archiver::{TripleBuffer, WriteResult};
use crate::capture::PacketBatch;
use crate::metrics;
use crate::parser::{PacketParser, PassiveAssetDiscovery};
use std::sync::Arc;
use tracing::{debug, trace, warn};
#[derive(Default, Debug, Clone)]
pub struct ProcessorStats {
    pub packets_processed: u64,
    pub packets_parsed: u64,
    pub packets_failed: u64,
    pub new_flows: u64,
    pub updated_flows: u64,
    pub pcap_packets_written: u64,
    pub pcap_bytes_written: u64,
    pub pcap_write_blocked: u64,
}
impl ProcessorStats {
    pub fn merge(&mut self, other: &ProcessorStats) {
        self.packets_processed += other.packets_processed;
        self.packets_parsed += other.packets_parsed;
        self.packets_failed += other.packets_failed;
        self.new_flows += other.new_flows;
        self.updated_flows += other.updated_flows;
        self.pcap_packets_written += other.pcap_packets_written;
        self.pcap_bytes_written += other.pcap_bytes_written;
        self.pcap_write_blocked += other.pcap_write_blocked;
    }
    pub fn reset(&mut self) {
        *self = Self::default();
    }
    pub fn parse_success_rate(&self) -> f64 {
        if self.packets_processed == 0 {
            return 0.0;
        }
        self.packets_parsed as f64 / self.packets_processed as f64
    }
    pub fn new_flow_ratio(&self) -> f64 {
        let total = self.new_flows + self.updated_flows;
        if total == 0 {
            return 0.0;
        }
        self.new_flows as f64 / total as f64
    }
}
pub struct PacketProcessor {
    flow_table: Arc<PartitionedFlowTable>,
    triple_buffer: Option<Arc<TripleBuffer>>,
    discovery: Option<Arc<PassiveAssetDiscovery>>,
    stats: ProcessorStats,
    pcap_full_capture: bool,
}
impl PacketProcessor {
    pub fn new(flow_table: Arc<PartitionedFlowTable>) -> Self {
        Self {
            flow_table,
            triple_buffer: None,
            discovery: None,
            stats: ProcessorStats::default(),
            pcap_full_capture: false,
        }
    }

    /// 启用被动资产发现 (DNS/DHCP/ARP)
    pub fn with_discovery(mut self, discovery: Arc<PassiveAssetDiscovery>) -> Self {
        self.discovery = Some(discovery);
        self
    }
    pub fn with_pcap(
        flow_table: Arc<PartitionedFlowTable>,
        triple_buffer: Arc<TripleBuffer>,
    ) -> Self {
        Self {
            flow_table,
            triple_buffer: Some(triple_buffer),
            discovery: None,
            stats: ProcessorStats::default(),
            pcap_full_capture: true,
        }
    }
    pub fn set_pcap_mode(&mut self, full_capture: bool) {
        self.pcap_full_capture = full_capture;
    }
    pub fn process_batch(&mut self, batch: &PacketBatch) {
        let batch_start = std::time::Instant::now();
        let batch_size = batch.len();
        metrics::PROCESSOR_BATCHES.inc();
        metrics::PROCESSOR_BATCH_SIZE.observe(batch_size as f64);
        for i in 0..batch.len() {
            if let Some(data) = batch.get_packet(i) {
                let info = batch.frames[i];
                if self.pcap_full_capture {
                    self.write_to_pcap(data, info.timestamp);
                }
                self.process_packet(data, info.timestamp);
            }
        }
        let elapsed = batch_start.elapsed();
        metrics::PROCESSOR_LATENCY.observe(elapsed.as_secs_f64());
        if batch_size > 0 {
            trace!("Processed batch: {} packets in {:?}", batch_size, elapsed);
        }
    }
    #[inline]
    fn write_to_pcap(&mut self, data: &[u8], timestamp: u64) {
        if let Some(ref buffer) = self.triple_buffer {
            match buffer.write_packet(timestamp, data) {
                WriteResult::Ok => {
                    self.stats.pcap_packets_written += 1;
                    self.stats.pcap_bytes_written += data.len() as u64;
                    metrics::PCAP_WRITE_SUCCESS.inc();
                }
                WriteResult::Fallback => {
                    self.stats.pcap_packets_written += 1;
                    self.stats.pcap_bytes_written += data.len() as u64;
                    use tracing::warn;
                    warn!("PCAP write fallback to disk - buffer overflow");
                }
                WriteResult::Rotated => {
                    self.stats.pcap_packets_written += 1;
                    self.stats.pcap_bytes_written += data.len() as u64;
                    metrics::PCAP_BUFFER_ROTATIONS.inc();
                    trace!("PCAP buffer rotated");
                }
                WriteResult::Blocked => {
                    self.stats.pcap_write_blocked += 1;
                    metrics::PCAP_WRITE_BLOCKED.inc();
                    warn!("PCAP write blocked - all buffers busy");
                }
                WriteResult::Error => {
                    metrics::PCAP_WRITE_ERRORS.inc();
                    warn!("PCAP write error");
                }
            }
        }
    }
    fn process_packet(&mut self, data: &[u8], timestamp: u64) {
        self.stats.packets_processed += 1;
        metrics::PARSE_TOTAL.inc();

        // 被动资产发现: ARP (ethertype 0x0806 → 不需要 IP 层解析)
        if let Some(ref discovery) = self.discovery {
            discovery.process_arp_packet(data, timestamp);
        }

        let parse_start = std::time::Instant::now();
        let parsed = match PacketParser::parse(data, timestamp) {
            Ok(Some(p)) => {
                self.stats.packets_parsed += 1;
                metrics::PARSE_SUCCESS.inc();
                metrics::PARSE_LATENCY.observe(parse_start.elapsed().as_secs_f64());
                p
            }
            Ok(None) => {
                self.stats.packets_failed += 1;
                metrics::PARSE_SKIPPED.inc();
                return;
            }
            Err(e) => {
                self.stats.packets_failed += 1;
                metrics::PARSE_FAILED.inc();
                trace!("Parse error: {}", e);
                return;
            }
        };
        let protocol_name = match parsed.protocol {
            1 => "ICMP",
            6 => "TCP",
            17 => "UDP",
            58 => "ICMPv6",
            _ => "Other",
        };
        if parsed.protocol == 17 {
            trace!(
                "Parsed UDP: {}:{} -> {}:{}, len={}, tos={}",
                parsed.src_ip,
                parsed.src_port,
                parsed.dst_ip,
                parsed.dst_port,
                parsed.total_len,
                parsed.tos
            );
        }
        let flow_key = FlowKey::new(
            parsed.src_ip,
            parsed.dst_ip,
            parsed.src_port,
            parsed.dst_port,
            parsed.protocol,
        );
        let normalized_key = flow_key.normalize();
        let is_forward = community_id::is_forward(
            parsed.src_ip,
            parsed.src_port,
            parsed.dst_ip,
            parsed.dst_port,
        );
        let packet_info = PacketInfo {
            len: parsed.total_len,
            tcp_flags: parsed.tcp_flags,
            is_forward,
            timestamp,
            tos: parsed.tos,
        };
        match self.flow_table.update(&normalized_key, &packet_info) {
            UpdateResult::NewFlow => {
                self.stats.new_flows += 1;
                metrics::FLOWS_CREATED.inc();
                metrics::PROCESSOR_NEW_FLOWS.inc();
                if parsed.protocol == 17 {
                    debug!(
                        "New UDP flow: {}:{} <-> {}:{}, cid={}",
                        normalized_key.src_ip,
                        normalized_key.src_port,
                        normalized_key.dst_ip,
                        normalized_key.dst_port,
                        normalized_key.community_id()
                    );
                } else {
                    debug!(
                        "New {} flow: {}:{} <-> {}:{}, cid={}",
                        protocol_name,
                        normalized_key.src_ip,
                        normalized_key.src_port,
                        normalized_key.dst_ip,
                        normalized_key.dst_port,
                        normalized_key.community_id()
                    );
                }
            }
            UpdateResult::Updated => {
                self.stats.updated_flows += 1;
                metrics::FLOW_TABLE_UPDATES.inc();
                metrics::PROCESSOR_UPDATED_FLOWS.inc();
                trace!(
                    "{} flow updated: {}:{} <-> {}:{}",
                    protocol_name,
                    normalized_key.src_ip,
                    normalized_key.src_port,
                    normalized_key.dst_ip,
                    normalized_key.dst_port
                );
            }
        }

        // 被动资产发现: DNS (UDP 53) / DHCP (UDP 67/68)
        if let Some(ref discovery) = self.discovery {
            discovery.process_packet(
                data,
                parsed.src_port,
                parsed.dst_port,
                parsed.protocol,
                timestamp,
            );
        }
    }
    #[inline]
    pub fn process_packet_fast(&mut self, data: &[u8], timestamp: u64) {
        self.stats.packets_processed += 1;
        let tuple = match PacketParser::quick_five_tuple(data) {
            Some(t) => t,
            None => {
                self.stats.packets_failed += 1;
                return;
            }
        };
        let tos = PacketParser::quick_tos(data).unwrap_or(0);
        let (src_ip, dst_ip, src_port, dst_port, protocol) = tuple;
        let flow_key = FlowKey::new(src_ip, dst_ip, src_port, dst_port, protocol);
        let normalized_key = flow_key.normalize();
        let is_forward = community_id::is_forward(src_ip, src_port, dst_ip, dst_port);
        let tcp_flags = if protocol == 6 {
            Self::quick_tcp_flags(data)
        } else {
            0
        };
        let packet_info = PacketInfo {
            len: data.len() as u16,
            tcp_flags,
            is_forward,
            timestamp,
            tos,
        };
        match self.flow_table.update(&normalized_key, &packet_info) {
            UpdateResult::NewFlow => {
                self.stats.new_flows += 1;
            }
            UpdateResult::Updated => {
                self.stats.updated_flows += 1;
            }
        }
        self.stats.packets_parsed += 1;
    }
    #[inline]
    fn quick_tcp_flags(data: &[u8]) -> u8 {
        if data.len() < 14 {
            return 0;
        }
        let ethertype = u16::from_be_bytes([data[12], data[13]]);
        let ip_offset = if ethertype == 0x8100 { 18 } else { 14 };
        if data.len() < ip_offset + 20 {
            return 0;
        }
        let ip_version = (data[ip_offset] >> 4) & 0x0F;
        if ip_version != 4 && ip_version != 6 {
            return 0;
        }
        let ihl = if ip_version == 4 {
            (data[ip_offset] & 0x0F) as usize * 4
        } else {
            40
        };
        let tcp_offset = ip_offset + ihl;
        if data.len() < tcp_offset + 14 {
            return 0;
        }
        data[tcp_offset + 13]
    }
    pub fn stats(&self) -> &ProcessorStats {
        &self.stats
    }
    pub fn stats_mut(&mut self) -> &mut ProcessorStats {
        &mut self.stats
    }
    pub fn reset_stats(&mut self) {
        self.stats = ProcessorStats::default();
    }
    pub fn is_pcap_enabled(&self) -> bool {
        self.triple_buffer.is_some()
    }
    pub fn flow_table(&self) -> &Arc<PartitionedFlowTable> {
        &self.flow_table
    }
    pub fn triple_buffer(&self) -> Option<&Arc<TripleBuffer>> {
        self.triple_buffer.as_ref()
    }
}
