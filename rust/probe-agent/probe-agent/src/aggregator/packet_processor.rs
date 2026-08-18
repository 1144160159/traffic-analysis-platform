use super::flow_table::{FlowUpdateError, ObservationScope, UpdateResult};
use super::partitioned_flow_table::PartitionedFlowTable;
use crate::archiver::{TripleBuffer, WriteResult};
use crate::capture::{CaptureFrame, CaptureTimestamp, PacketBatch};
use crate::config::ParserRoute;
use crate::metrics;
use crate::parser::security::PacketFeatureObservation;
use crate::parser::{
    FastDecodeError, FlowDecodeError, FlowFields, FlowSampleBuilder, FlowSampleError, PacketParser,
    PassiveAssetDiscovery,
};
use std::sync::Arc;
use tracing::{trace, warn};

fn decode_full(data: &[u8]) -> Result<Option<FlowFields>, FlowDecodeError> {
    match PacketParser::decode_flow_fields(data) {
        Ok(fields) => Ok(Some(fields)),
        Err(crate::parser::FlowDecodeError::UnsupportedEtherType(_)) => Ok(None),
        Err(error) => Err(error),
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FlowUpdateOutcome {
    NewFlow,
    Updated,
    Skipped,
}

#[derive(Debug, thiserror::Error)]
pub enum PacketProcessError {
    #[error(transparent)]
    Decode(#[from] FlowDecodeError),
    #[error(transparent)]
    Sample(#[from] FlowSampleError),
    #[error(transparent)]
    Update(#[from] FlowUpdateError),
    #[error("SHADOW_FLOW_SAMPLE_MISMATCH")]
    ShadowMismatch,
}

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
    observation_scope: ObservationScope,
    parser_route: ParserRoute,
}
impl PacketProcessor {
    pub fn new(flow_table: Arc<PartitionedFlowTable>) -> Self {
        Self {
            flow_table,
            triple_buffer: None,
            discovery: None,
            stats: ProcessorStats::default(),
            pcap_full_capture: false,
            observation_scope: ObservationScope::global_l3(),
            parser_route: ParserRoute::Full,
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
            observation_scope: ObservationScope::global_l3(),
            parser_route: ParserRoute::Full,
        }
    }

    pub fn with_observation_scope(mut self, scope: ObservationScope) -> Self {
        self.observation_scope = scope;
        self
    }
    pub fn with_parser_route(mut self, route: ParserRoute) -> Self {
        self.parser_route = route;
        self
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
                let timestamp = info.captured_at.epoch_micros();
                if self.pcap_full_capture {
                    self.write_to_pcap(data, timestamp);
                }
                let frame = CaptureFrame::new(data, info.captured_at);
                let result = match self.parser_route {
                    ParserRoute::Full => self.process_packet(&frame),
                    ParserRoute::Fast => self.process_packet_fast(&frame),
                    ParserRoute::Shadow => self.process_packet_shadow(&frame),
                };
                if let Err(error) = result {
                    trace!(route = ?self.parser_route, "Packet route rejected frame: {}", error);
                }
            }
        }
        let elapsed = batch_start.elapsed();
        metrics::PROCESSOR_LATENCY.observe(elapsed.as_secs_f64());
        if batch_size > 0 {
            trace!("Processed batch: {} packets in {:?}", batch_size, elapsed);
        }
    }
    fn process_decoded_frame(
        &mut self,
        frame: &CaptureFrame<'_>,
        decoded: Result<Option<FlowFields>, PacketProcessError>,
    ) -> Result<FlowUpdateOutcome, PacketProcessError> {
        self.stats.packets_processed += 1;
        metrics::PARSE_TOTAL.inc();
        metrics::inc_processed_local();
        let parse_start = std::time::Instant::now();
        let fields = match decoded {
            Ok(Some(fields)) => fields,
            Ok(None) => {
                self.stats.packets_failed += 1;
                metrics::PARSE_SKIPPED.inc();
                metrics::inc_parse_result_local(false);
                if let Some(ref discovery) = self.discovery {
                    discovery.process_arp_packet(frame.bytes, frame.captured_at.epoch_micros());
                }
                return Ok(FlowUpdateOutcome::Skipped);
            }
            Err(error) => {
                self.stats.packets_failed += 1;
                metrics::PARSE_FAILED.inc();
                metrics::inc_parse_result_local(false);
                return Err(error);
            }
        };

        let sample =
            match FlowSampleBuilder::build(fields, frame.captured_at, &self.observation_scope) {
                Ok(sample) => sample,
                Err(error) => {
                    self.stats.packets_failed += 1;
                    metrics::PARSE_FAILED.inc();
                    metrics::inc_parse_result_local(false);
                    return Err(error.into());
                }
            };

        let feature_observation = PacketFeatureObservation::from_decoded_frame(
            &sample.fields,
            sample.packet.direction,
            sample.packet.timestamp,
            frame.bytes,
        );
        let outcome = match self
            .flow_table
            .update_sample_with_feature(&sample, &feature_observation)
        {
            Ok(UpdateResult::NewFlow) => {
                self.stats.new_flows += 1;
                metrics::FLOWS_CREATED.inc();
                metrics::PROCESSOR_NEW_FLOWS.inc();
                metrics::inc_flow_update_local(true);
                FlowUpdateOutcome::NewFlow
            }
            Ok(UpdateResult::Updated) => {
                self.stats.updated_flows += 1;
                metrics::FLOW_TABLE_UPDATES.inc();
                metrics::PROCESSOR_UPDATED_FLOWS.inc();
                metrics::inc_flow_update_local(false);
                FlowUpdateOutcome::Updated
            }
            Err(error) => {
                self.stats.packets_failed += 1;
                metrics::inc_parse_result_local(false);
                return Err(error.into());
            }
        };
        self.stats.packets_parsed += 1;
        metrics::PARSE_SUCCESS.inc();
        metrics::inc_parse_result_local(true);
        metrics::PARSE_LATENCY.observe(parse_start.elapsed().as_secs_f64());

        if let Some(ref discovery) = self.discovery {
            discovery.process_flow_sample(
                frame.bytes,
                &sample.fields,
                frame.captured_at.epoch_micros(),
            );
        }
        Ok(outcome)
    }

    fn process_packet(
        &mut self,
        frame: &CaptureFrame<'_>,
    ) -> Result<FlowUpdateOutcome, PacketProcessError> {
        let decoded = decode_full(frame.bytes).map_err(PacketProcessError::from);
        self.process_decoded_frame(frame, decoded)
    }

    pub fn process_packet_fast(
        &mut self,
        frame: &CaptureFrame<'_>,
    ) -> Result<FlowUpdateOutcome, PacketProcessError> {
        let decoded = match PacketParser::decode_flow_fields_fast(frame.bytes) {
            Ok(fields) => Ok(Some(fields)),
            Err(FastDecodeError::Fallback(_)) => {
                decode_full(frame.bytes).map_err(PacketProcessError::from)
            }
            Err(FastDecodeError::Invalid(FlowDecodeError::UnsupportedEtherType(_))) => Ok(None),
            Err(FastDecodeError::Invalid(error)) => Err(error.into()),
        };
        self.process_decoded_frame(frame, decoded)
    }

    fn process_packet_shadow(
        &mut self,
        frame: &CaptureFrame<'_>,
    ) -> Result<FlowUpdateOutcome, PacketProcessError> {
        let full = decode_full(frame.bytes)?;
        let decoded = match (full, PacketParser::decode_flow_fields_fast(frame.bytes)) {
            (None, Err(FastDecodeError::Invalid(FlowDecodeError::UnsupportedEtherType(_)))) => {
                Ok(None)
            }
            (Some(full), Ok(fast)) if full == fast => Ok(Some(full)),
            (Some(full), Err(FastDecodeError::Fallback(_))) => Ok(Some(full)),
            (Some(_), Err(FastDecodeError::Invalid(error))) => Err(error.into()),
            (Some(_), Ok(_))
            | (None, Ok(_))
            | (None, Err(FastDecodeError::Fallback(_)))
            | (None, Err(FastDecodeError::Invalid(_))) => Err(PacketProcessError::ShadowMismatch),
        };
        self.process_decoded_frame(frame, decoded)
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
    pub fn process_frame_for_test(
        &mut self,
        data: &[u8],
        captured_at: CaptureTimestamp,
    ) -> Result<FlowUpdateOutcome, PacketProcessError> {
        let frame = CaptureFrame::new(data, captured_at);
        match self.parser_route {
            ParserRoute::Full => self.process_packet(&frame),
            ParserRoute::Fast => self.process_packet_fast(&frame),
            ParserRoute::Shadow => self.process_packet_shadow(&frame),
        }
    }
    pub fn triple_buffer(&self) -> Option<&Arc<TripleBuffer>> {
        self.triple_buffer.as_ref()
    }
}
