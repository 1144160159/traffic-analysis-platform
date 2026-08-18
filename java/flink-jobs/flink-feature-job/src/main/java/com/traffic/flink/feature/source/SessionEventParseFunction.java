package com.traffic.flink.feature.source;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.SessionEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Parses and validates SessionEvent while retaining its Kafka source tuple. */
public final class SessionEventParseFunction
        extends ProcessFunction<RawKafkaRecord, ValidatedSessionInput> {

    private static final long serialVersionUID = 1L;
    private final OutputTag<CanonicalDlqMessage> dlqTag;

    public SessionEventParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord source, Context context, Collector<ValidatedSessionInput> out) {
        ParseResult result = parse(source);
        if (result.input != null) {
            out.collect(result.input);
        } else {
            context.output(dlqTag, result.failure);
        }
    }

    static ParseResult parse(RawKafkaRecord source) {
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0) {
            return failure(source, null, "BAD_SCHEMA", "parse_error", "empty SessionEvent payload");
        }

        SessionEvent session;
        try {
            session = ProtoDeserializer.withoutUnknownFields(SessionEvent.parseFrom(payload));
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid SessionEvent protobuf: " + error.getMessage());
        }

        String validationError = validate(session);
        if (validationError != null) {
            String code = validationError.startsWith("timestamp:") ? "BAD_TIMESTAMP" : "VALIDATION_ERROR";
            return failure(source, session, code, "validation_error", validationError);
        }
        return new ParseResult(new ValidatedSessionInput(source, session), null);
    }

    static String validate(SessionEvent session) {
        if (!session.hasHeader()) return "missing SessionEvent header";
        EventHeader header = session.getHeader();
        if (header.getTenantId().isEmpty()) return "missing SessionEvent tenant_id";
        if (header.getEventId().isEmpty()) return "missing SessionEvent event_id";
        if (session.getSessionId().isEmpty()) return "missing SessionEvent session_id";
        if (session.getCommunityId().isEmpty()) return "missing SessionEvent community_id";
        if (!session.hasTuple()) return "missing SessionEvent tuple";
        if (session.getTsStart() < 0 || session.getTsEnd() < session.getTsStart()) {
            return "timestamp: SessionEvent ts_start/ts_end range is invalid";
        }
        if (session.getEventTimeStartMs() < 0
                || (session.getEventTimeEndMs() > 0
                    && session.getEventTimeEndMs() < session.getEventTimeStartMs())) {
            return "timestamp: SessionEvent event_time range is invalid";
        }
        if (header.getEventTs() < 0 || header.getOccurredAt() < 0 || header.getProducedAt() < 0) {
            return "timestamp: SessionEvent header contains a negative timestamp";
        }
        if (session.hasFeatureObservation()) {
            String observationError = validateFeatureObservation(session.getFeatureObservation());
            if (observationError != null) return observationError;
        }
        return null;
    }

    private static String validateFeatureObservation(TrafficFeatureObservation observation) {
        if (!"traffic-feature-observation/v1".equals(observation.getSchemaVersion())) {
            return "feature_observation has an unsupported schema_version";
        }
        if (observation.getAlgorithmVersion().isEmpty()) {
            return "feature_observation is missing algorithm_version";
        }
        int sequenceCount = observation.getSignedPacketLengthsCount();
        if (sequenceCount > 256) return "feature_observation packet sequence exceeds 256 samples";
        if (observation.getPacketEventTimeUsCount() != sequenceCount) {
            return "feature_observation packet lengths and timestamps have different cardinality";
        }
        long previousTimestamp = -1L;
        for (int index = 0; index < sequenceCount; index++) {
            if (observation.getSignedPacketLengths(index) == 0) {
                return "feature_observation contains a zero signed packet length";
            }
            long timestamp = observation.getPacketEventTimeUs(index);
            if (timestamp == 0L || (previousTimestamp >= 0L && timestamp < previousTimestamp)) {
                return "timestamp: feature_observation packet timestamps are invalid";
            }
            previousTimestamp = timestamp;
        }
        int nibbleBucketCount = observation.getPayloadNibbleCountsCount();
        if (nibbleBucketCount != 0 && nibbleBucketCount != 16) {
            return "feature_observation payload_nibble_counts must contain 16 buckets";
        }
        long nibbleTotal = 0L;
        for (long count : observation.getPayloadNibbleCountsList()) {
            if (Long.MAX_VALUE - nibbleTotal < count) {
                return "feature_observation payload_nibble_counts overflow";
            }
            nibbleTotal += count;
        }
        long payloadBytes = observation.getPayloadObservedBytes();
        if (payloadBytes > Long.MAX_VALUE / 2L || nibbleTotal != payloadBytes * 2L) {
            return "feature_observation payload nibble accounting does not match observed bytes";
        }
        TransportSecurityProtocol protocol = observation.getTransportSecurity();
        if (protocol == TransportSecurityProtocol.UNRECOGNIZED) {
            return "feature_observation contains an unknown transport_security value";
        }
        if (protocol != TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS
                && (!observation.getTlsVersion().isEmpty()
                    || !observation.getJa3().isEmpty()
                    || !observation.getCertSha256().isEmpty()
                    || observation.getCertIsSelfSignedKnown()
                    || observation.getPubkeyLenKnown())) {
            return "feature_observation carries TLS-only fields without an observed TLS protocol";
        }
        if (protocol != TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS
                && protocol != TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC
                && (!observation.getJa4().isEmpty() || !observation.getSni().isEmpty())) {
            return "feature_observation carries handshake fields without an observed security protocol";
        }
        if (protocol != TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC
                && !observation.getQuicVersion().isEmpty()) {
            return "feature_observation carries quic_version without an observed QUIC protocol";
        }
        if (!observation.getJa3().isEmpty()
                && !observation.getJa3().matches("[0-9a-f]{32}")) {
            return "feature_observation ja3 is not a lowercase MD5 hex digest";
        }
        if (!observation.getCertSha256().isEmpty()
                && !observation.getCertSha256().matches("[0-9a-f]{64}")) {
            return "feature_observation cert_sha256 is not a lowercase SHA-256 hex digest";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord source,
            SessionEvent session,
            String code,
            String type,
            String message) {
        EventHeader header = session != null && session.hasHeader()
                ? session.getHeader() : EventHeader.getDefaultInstance();
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, code, type, message,
                header.getTenantId().isEmpty() ? source.header("tenant_id") : header.getTenantId(),
                header.getEventId(), header.getTraceId(), header.getRunId(), header.getProbeId());
        return new ParseResult(null, failure);
    }

    static final class ParseResult {
        final ValidatedSessionInput input;
        final CanonicalDlqMessage failure;

        ParseResult(ValidatedSessionInput input, CanonicalDlqMessage failure) {
            this.input = input;
            this.failure = failure;
        }
    }
}
