package com.traffic.flink.alert.source;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Decodes and validates a behavior detection without losing its Kafka source coordinates. */
public final class BehaviorDetectionParseFunction
        extends ProcessFunction<RawKafkaRecord, DetectionBehavior> {

    private static final long serialVersionUID = 1L;
    private final OutputTag<CanonicalDlqMessage> dlqTag;

    public BehaviorDetectionParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord source, Context context, Collector<DetectionBehavior> out) {
        ParseResult result = parse(source);
        if (result.detection != null) {
            out.collect(result.detection);
        } else {
            context.output(dlqTag, result.failure);
        }
    }

    static ParseResult parse(RawKafkaRecord source) {
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "empty DetectionBehavior payload");
        }

        DetectionBehavior detection;
        try {
            detection = ProtoDeserializer.withoutUnknownFields(
                    DetectionBehavior.parseFrom(payload));
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid DetectionBehavior protobuf: " + error.getMessage());
        }

        String validationError = validate(detection);
        if (validationError != null) {
            String code = validationError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "VALIDATION_ERROR";
            return failure(source, detection, code, "validation_error", validationError);
        }
        return new ParseResult(detection, null);
    }

    static String validate(DetectionBehavior detection) {
        if (!detection.hasHeader()) return "missing DetectionBehavior header";
        EventHeader header = detection.getHeader();
        String[][] required = {
                {"event_id", header.getEventId()},
                {"tenant_id", header.getTenantId()},
                {"event_type", header.getEventType()},
                {"schema_version", header.getSchemaVersion()},
                {"aggregate_type", header.getAggregateType()},
                {"aggregate_id", header.getAggregateId()},
                {"trace_id", header.getTraceId()},
                {"causation_id", header.getCausationId()},
                {"correlation_id", header.getCorrelationId()},
                {"idempotency_key", header.getIdempotencyKey()},
                {"producer", header.getProducer()}
        };
        for (String[] field : required) {
            if (field[1] == null || field[1].trim().isEmpty()) {
                return "missing DetectionBehavior envelope " + field[0];
            }
        }
        if (!"traffic.detection.behavior.v1".equals(header.getEventType())
                || !"1".equals(header.getSchemaVersion())) {
            return "unsupported DetectionBehavior event contract";
        }
        if (header.getAggregateVersion() == 0) {
            return "missing DetectionBehavior aggregate_version";
        }
        if (header.getOccurredAt() <= 0 || header.getProducedAt() <= 0
                || header.getEventTs() <= 0 || detection.getTs() <= 0) {
            return "timestamp: DetectionBehavior event timestamps must be positive";
        }
        if (!header.getEventId().equals(header.getIdempotencyKey())) {
            return "DetectionBehavior idempotency_key must equal event_id";
        }
        if (detection.getCommunityId().trim().isEmpty()) {
            return "missing DetectionBehavior community_id";
        }
        if (detection.getTopLabel().trim().isEmpty()
                || !Float.isFinite(detection.getTopScore())
                || detection.getTopScore() <= 0.0f) {
            return "missing or invalid DetectionBehavior top result";
        }
        if (!detection.hasTuple()) return "missing DetectionBehavior source tuple";
        FiveTuple tuple = detection.getTuple();
        if (tuple.getSrcIp().trim().isEmpty()
                || tuple.getDstIp().trim().isEmpty()
                || tuple.getProtocol() == 0) {
            return "invalid DetectionBehavior source tuple";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord source,
            DetectionBehavior detection,
            String code,
            String type,
            String message) {
        EventHeader header = detection != null && detection.hasHeader()
                ? detection.getHeader() : EventHeader.getDefaultInstance();
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, code, type, message,
                header.getTenantId().isEmpty() ? source.header("tenant_id") : header.getTenantId(),
                header.getEventId(), header.getTraceId(), header.getRunId(), header.getProbeId(),
                "flink-alert-generator-job", "traffic.v1.DetectionBehavior", "v1");
        return new ParseResult(null, failure);
    }

    static final class ParseResult {
        final DetectionBehavior detection;
        final CanonicalDlqMessage failure;

        ParseResult(DetectionBehavior detection, CanonicalDlqMessage failure) {
            this.detection = detection;
            this.failure = failure;
        }
    }
}
