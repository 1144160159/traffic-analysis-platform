package com.traffic.flink.rule.source;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Validates canonical FeatureStat input while retaining Kafka source coordinates for DLQ. */
public final class FeatureStatParseFunction extends ProcessFunction<RawKafkaRecord, FeatureStat> {

    private static final long serialVersionUID = 1L;
    private final OutputTag<CanonicalDlqMessage> dlqTag;

    public FeatureStatParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(RawKafkaRecord source, Context context, Collector<FeatureStat> out) {
        ParseResult result = parse(source);
        if (result.feature != null) out.collect(result.feature);
        else context.output(dlqTag, result.failure);
    }

    static ParseResult parse(RawKafkaRecord source) {
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0) {
            return failure(source, null, "BAD_SCHEMA", "parse_error", "empty FeatureStat payload");
        }
        FeatureStat feature;
        try {
            feature = ProtoDeserializer.withoutUnknownFields(FeatureStat.parseFrom(payload));
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid FeatureStat protobuf: " + error.getMessage());
        }
        String validationError = validate(feature);
        if (validationError != null) {
            String code = validationError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "VALIDATION_ERROR";
            return failure(source, feature, code, "validation_error", validationError);
        }
        return new ParseResult(feature, null);
    }

    static String validate(FeatureStat feature) {
        if (!feature.hasHeader()) return "missing FeatureStat header";
        EventHeader header = feature.getHeader();
        if (header.getTenantId().trim().isEmpty()) return "missing FeatureStat tenant_id";
        if (header.getEventId().trim().isEmpty()) return "missing FeatureStat event_id";
        if (!"traffic.feature.stat.v1".equals(header.getEventType())
                || !"1".equals(header.getSchemaVersion())) {
            return "unsupported FeatureStat event contract";
        }
        if (header.getAggregateVersion() == 0
                || header.getTraceId().trim().isEmpty()
                || header.getCausationId().trim().isEmpty()
                || header.getCorrelationId().trim().isEmpty()
                || header.getIdempotencyKey().trim().isEmpty()
                || header.getProducer().trim().isEmpty()) {
            return "incomplete FeatureStat event envelope";
        }
        if (!header.getEventId().equals(header.getIdempotencyKey())) {
            return "FeatureStat idempotency_key must equal event_id";
        }
        if (feature.getTs() <= 0 || header.getEventTs() <= 0
                || header.getOccurredAt() <= 0 || header.getProducedAt() <= 0) {
            return "timestamp: FeatureStat event timestamps must be positive";
        }
        if (feature.getObjectId().trim().isEmpty()) return "missing FeatureStat object_id";
        if (feature.getCommunityId().trim().isEmpty()) return "missing FeatureStat community_id";
        if (feature.getAvailability() == FeatureAvailability.FEATURE_AVAILABILITY_INVALID) {
            return "FeatureStat availability is invalid";
        }
        if (!feature.hasTuple()) return "missing FeatureStat source tuple";
        FiveTuple tuple = feature.getTuple();
        if (tuple.getSrcIp().trim().isEmpty()
                || tuple.getDstIp().trim().isEmpty()
                || tuple.getProtocol() == 0
                || feature.getProtocol() == 0
                || feature.getProtocol() != tuple.getProtocol()) {
            return "invalid or inconsistent FeatureStat source tuple";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord source, FeatureStat feature,
            String code, String type, String message) {
        EventHeader header = feature != null && feature.hasHeader()
                ? feature.getHeader() : EventHeader.getDefaultInstance();
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, code, type, message,
                header.getTenantId().isEmpty() ? source.header("tenant_id") : header.getTenantId(),
                header.getEventId(), header.getTraceId(), header.getRunId(), header.getProbeId(),
                "flink-rule-job", "traffic.v1.FeatureStat", "v1");
        return new ParseResult(null, failure);
    }

    static final class ParseResult {
        final FeatureStat feature;
        final CanonicalDlqMessage failure;

        ParseResult(FeatureStat feature, CanonicalDlqMessage failure) {
            this.feature = feature;
            this.failure = failure;
        }
    }
}
