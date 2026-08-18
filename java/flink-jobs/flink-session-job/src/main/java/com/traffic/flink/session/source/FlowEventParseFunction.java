package com.traffic.flink.session.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.util.regex.Pattern;

/**
 * Parses raw Kafka bytes into FlowEvent and emits bad input records to DLQ.
 */
public class FlowEventParseFunction extends ProcessFunction<RawKafkaRecord, ValidatedFlowInput> {

    private static final long serialVersionUID = 1L;
    private static final int FEATURE_SEQUENCE_LIMIT = 256;
    private static final int MAX_PAYLOAD_BYTES = 1_048_576;
    private static final Pattern LOWER_HEX_32 = Pattern.compile("[0-9a-f]{32}");
    private static final Pattern LOWER_HEX_64 = Pattern.compile("[0-9a-f]{64}");

    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final String inputTopic;
    private final String consumerGroup;
    private final EventTimePolicy eventTimePolicy;
    private final OutputTag<SourceQualityReceipt> qualityTag;

    public FlowEventParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this("flow.events.v1", "", new EventTimePolicy(0L, 1L, 0L, 300_000L, 0L),
                dlqTag, null);
    }

    public FlowEventParseFunction(
            String inputTopic,
            String consumerGroup,
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag) {
        if (inputTopic == null || inputTopic.isBlank()) {
            throw new IllegalArgumentException("flow input topic is required");
        }
        if (eventTimePolicy == null) {
            throw new IllegalArgumentException("event-time policy is required");
        }
        if (dlqTag == null) throw new IllegalArgumentException("flow DLQ tag is required");
        this.inputTopic = inputTopic;
        this.consumerGroup = consumerGroup == null ? "" : consumerGroup;
        this.eventTimePolicy = eventTimePolicy;
        this.dlqTag = dlqTag;
        this.qualityTag = qualityTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord record,
            Context ctx,
            Collector<ValidatedFlowInput> out) {
        ParseResult result = parseRecord(
                record, inputTopic, eventTimePolicy.getMaxFutureSkewMs());
        if (result.input != null) {
            out.collect(result.input);
            return;
        }
        ctx.output(dlqTag, result.failure);
        if (qualityTag != null) {
            ctx.output(qualityTag, rejectionReceipt(
                    record, result.failure, consumerGroup,
                    ctx.timerService().currentProcessingTime()));
        }
    }

    static ParseResult parseRecord(RawKafkaRecord record) {
        return parseRecord(record, "flow.events.v1", 300_000L);
    }

    static ParseResult parseRecord(
            RawKafkaRecord record, String inputTopic, long maxFutureSkewMs) {
        if (record == null) throw new IllegalArgumentException("source record is required");
        if (!inputTopic.equals(record.getTopic())) {
            return failure(record, null, "WRONG_SOURCE_TOPIC", "contract_violation",
                    "expected source topic " + inputTopic);
        }
        byte[] payload = record.getValue();
        if (payload == null || payload.length == 0) {
            return failure(record, null, "BAD_SCHEMA", "parse_error", "empty FlowEvent payload");
        }
        if (payload.length > MAX_PAYLOAD_BYTES) {
            return failure(record, null, "PAYLOAD_TOO_LARGE", "validation_error",
                    "FlowEvent payload exceeds " + MAX_PAYLOAD_BYTES + " bytes");
        }

        FlowEvent flow;
        try {
            flow = FlowEvent.parseFrom(payload);
        } catch (InvalidProtocolBufferException e) {
            return failure(record, null, "BAD_SCHEMA", "parse_error",
                    "invalid FlowEvent protobuf: " + e.getMessage());
        }
        if (!flow.getUnknownFields().asMap().isEmpty()) {
            return failure(record, flow, "BAD_SCHEMA", "schema_version_mismatch",
                    "FlowEvent contains unknown protobuf fields");
        }

        if (!flow.hasHeader()) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error",
                    "missing FlowEvent header");
        }
        EventHeader header = flow.getHeader();
        if (isBlank(header.getTenantId())) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error",
                    "missing FlowEvent tenant_id");
        }
        if (isBlank(header.getEventId())) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error",
                    "missing FlowEvent event_id");
        }
        if (isBlank(flow.getFlowId())) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error",
                    "missing FlowEvent flow_id");
        }
        if (isBlank(flow.getCommunityId())) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error",
                    "missing FlowEvent community_id");
        }
        if (flow.getTsStart() <= 0L || flow.getTsEnd() < flow.getTsStart()) {
            return failure(record, flow, "BAD_TIMESTAMP", "event_time_contract",
                    "FlowEvent ts_start/ts_end range is invalid");
        }
        String featureError = validateFeatureObservation(flow);
        if (featureError != null) {
            return failure(record, flow, "VALIDATION_ERROR", "validation_error", featureError);
        }
        String envelopeError = validateEnvelope(record, flow, maxFutureSkewMs);
        if (envelopeError != null) {
            String code = envelopeError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "ENVELOPE_MISMATCH";
            return failure(record, flow, code, "contract_violation", envelopeError);
        }
        return ParseResult.flow(new ValidatedFlowInput(record, flow));
    }

    static String validateEnvelope(
            RawKafkaRecord record, FlowEvent flow, long maxFutureSkewMs) {
        if (!record.getDuplicateHeaderNames().isEmpty()) {
            return "Kafka envelope contains duplicate headers";
        }
        EventHeader header = flow.getHeader();
        if (!(header.getTenantId() + ":" + flow.getCommunityId()).equals(record.keyAsString())) {
            return "Kafka key must equal tenant_id:community_id";
        }
        String[][] requiredHeaders = {
                {"tenant_id", header.getTenantId()},
                {"probe_id", header.getProbeId()},
                {"event_id", header.getEventId()},
                {"run_id", header.getRunId()},
                {"feature_set_id", header.getFeatureSetId()},
                {"community_id", flow.getCommunityId()},
                {"content_type", "application/x-protobuf"},
                {"proto_message_type", "traffic.v1.FlowEvent"},
                {"proto_schema_version", "v1"},
                {"proto_package", "traffic.v1"},
                {"event_ts", String.valueOf(header.getEventTs())},
                {"ingest_ts", String.valueOf(header.getIngestTs())},
                {"kafka_ts", String.valueOf(header.getKafkaTs())}
        };
        for (String[] required : requiredHeaders) {
            if (!record.getHeaders().containsKey(required[0])
                    || !required[1].equals(record.header(required[0]))) {
                return "Kafka header " + required[0] + " is missing or inconsistent";
            }
        }
        if (header.getEventTs() != flow.getTsEnd()
                || header.getOccurredAt() != flow.getTsEnd()
                || header.getKafkaTs() != record.getTimestamp()
                || header.getIngestTs() <= 0L || header.getProducedAt() <= 0L
                || record.getTimestamp() <= 0L) {
            return "timestamp: FlowEvent body and Kafka time envelope are inconsistent";
        }
        if (!"traffic.flow.v1".equals(header.getEventType())
                || !"1".equals(header.getSchemaVersion())
                || !"flow".equals(header.getAggregateType())
                || !flow.getFlowId().equals(header.getAggregateId())
                || header.getAggregateVersion() <= 0L
                || isBlank(header.getIdempotencyKey())
                || isBlank(header.getProducer())) {
            return "FlowEvent additive identity envelope is incomplete";
        }
        if (EventTimePolicy.isFuture(
                flow.getTsEnd(), record.getTimestamp(), maxFutureSkewMs)) {
            return "timestamp: FlowEvent event time exceeds Kafka ingest time plus future skew";
        }
        return null;
    }

    private static String validateFeatureObservation(FlowEvent flow) {
        if (!flow.hasFeatureObservation()) {
            return null; // Additive compatibility: legacy FlowEvent remains valid.
        }
        TrafficFeatureObservation observation = flow.getFeatureObservation();
        if (isBlank(observation.getSchemaVersion()) || isBlank(observation.getAlgorithmVersion())) {
            return "invalid feature observation version";
        }
        int lengths = observation.getSignedPacketLengthsCount();
        if (lengths != observation.getPacketEventTimeUsCount() || lengths > FEATURE_SEQUENCE_LIMIT) {
            return "invalid feature observation sequence cardinality";
        }
        for (int i = 0; i < lengths; i++) {
            int length = observation.getSignedPacketLengths(i);
            long eventTimeUs = observation.getPacketEventTimeUs(i);
            if (length == 0 || Math.abs((long) length) > 65_535L || eventTimeUs <= 0) {
                return "invalid feature observation sequence value";
            }
        }
        int buckets = observation.getPayloadNibbleCountsCount();
        if (buckets != 0 && buckets != 16) {
            return "invalid feature observation nibble bucket cardinality";
        }
        long nibbleTotal = 0L;
        try {
            for (long value : observation.getPayloadNibbleCountsList()) {
                if (value < 0) return "invalid feature observation unsigned nibble count";
                nibbleTotal = Math.addExact(nibbleTotal, value);
            }
            if (observation.getPayloadObservedBytes() < 0
                    || nibbleTotal != Math.multiplyExact(observation.getPayloadObservedBytes(), 2L)) {
                return "invalid feature observation payload accounting";
            }
        } catch (ArithmeticException e) {
            return "invalid feature observation payload accounting overflow";
        }
        if (observation.getTransportSecurity() == TransportSecurityProtocol.UNRECOGNIZED) {
            return "invalid feature observation transport security enum";
        }
        if (!observation.getJa3().isEmpty() && !LOWER_HEX_32.matcher(observation.getJa3()).matches()) {
            return "invalid feature observation JA3";
        }
        if (!observation.getCertSha256().isEmpty()
                && !LOWER_HEX_64.matcher(observation.getCertSha256()).matches()) {
            return "invalid feature observation certificate SHA-256";
        }
        if (observation.getTransportSecurity() == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED
                && (!observation.getTlsVersion().isEmpty()
                    || !observation.getJa3().isEmpty()
                    || !observation.getJa4().isEmpty()
                    || !observation.getQuicVersion().isEmpty())) {
            return "security detail requires an observed transport security protocol";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord record,
            FlowEvent parsedFlow,
            String code,
            String type,
            String errorMsg) {
        String sourceKey = record.keyAsString();
        String tenantId = tenantFrom(record, parsedFlow, sourceKey);
        EventHeader header = parsedFlow != null && parsedFlow.hasHeader()
                ? parsedFlow.getHeader() : EventHeader.getDefaultInstance();
        return ParseResult.deadLetter(CanonicalDlqMessage.failure(
                record, code, type, errorMsg,
                tenantId, header.getEventId(), header.getTraceId(), header.getRunId(),
                header.getProbeId(),
                "flink-session-job", "traffic.v1.FlowEvent", "v1"));
    }

    static SourceQualityReceipt rejectionReceipt(
            RawKafkaRecord record,
            CanonicalDlqMessage failure,
            String consumerGroup,
            long processingTimeMs) {
        String category = "WRONG_SOURCE_TOPIC".equals(failure.errorCode())
                || "ENVELOPE_MISMATCH".equals(failure.errorCode()) ? "rejected" : "invalid";
        Object eventId = failure.fields().get("event_id");
        long observedAt = record.getTimestamp() > 0L
                ? record.getTimestamp() : Math.max(1L, processingTimeMs);
        return new SourceQualityReceipt(
                failure.tenantId(), "flow", consumerGroup,
                record.getTopic(), record.getPartition(), record.getOffset(), category,
                eventId == null ? "" : String.valueOf(eventId),
                SourceQualityReceipt.hashSource(record.getValue()), Long.MIN_VALUE,
                observedAt, failure.errorCode());
    }

    private static String tenantFrom(RawKafkaRecord record, FlowEvent parsedFlow, String sourceKey) {
        if (parsedFlow != null && parsedFlow.hasHeader() && !isBlank(parsedFlow.getHeader().getTenantId())) {
            return parsedFlow.getHeader().getTenantId();
        }
        String headerTenant = record.header("tenant_id");
        if (!isBlank(headerTenant)) {
            return headerTenant;
        }
        int separator = sourceKey.indexOf(':');
        if (separator > 0) {
            return sourceKey.substring(0, separator);
        }
        return "unknown";
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    static class ParseResult {
        final ValidatedFlowInput input;
        final CanonicalDlqMessage failure;

        private ParseResult(ValidatedFlowInput input, CanonicalDlqMessage failure) {
            this.input = input;
            this.failure = failure;
        }

        static ParseResult flow(ValidatedFlowInput input) {
            return new ParseResult(input, null);
        }

        static ParseResult deadLetter(CanonicalDlqMessage failure) {
            return new ParseResult(null, failure);
        }
    }
}
