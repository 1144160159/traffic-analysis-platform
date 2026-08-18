package com.traffic.flink.alert.source;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.DetectionBusiness;
import com.traffic.proto.traffic.v1.EventHeader;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Decodes the compatibility business-detection stream and routes rejects to canonical DLQ. */
public final class BusinessDetectionParseFunction
        extends ProcessFunction<RawKafkaRecord, DetectionBusiness> {

    private static final long serialVersionUID = 1L;
    private final OutputTag<CanonicalDlqMessage> dlqTag;

    public BusinessDetectionParseFunction(OutputTag<CanonicalDlqMessage> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord source, Context context, Collector<DetectionBusiness> out) {
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
                    "empty DetectionBusiness payload");
        }

        DetectionBusiness detection;
        try {
            detection = ProtoDeserializer.withoutUnknownFields(
                    DetectionBusiness.parseFrom(payload));
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid DetectionBusiness protobuf: " + error.getMessage());
        }

        String validationError = validate(detection);
        if (validationError != null) {
            String code = validationError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "VALIDATION_ERROR";
            return failure(source, detection, code, "validation_error", validationError);
        }
        return new ParseResult(detection, null);
    }

    static String validate(DetectionBusiness detection) {
        if (!detection.hasHeader()) return "missing DetectionBusiness header";
        EventHeader header = detection.getHeader();
        // 与 BehaviorDetectionParseFunction 对齐：EventHeader 信封上可用的
        // 契约/幂等校验不再缺失，防止错误 event_type 或重放直接生成告警。
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
                return "missing DetectionBusiness envelope " + field[0];
            }
        }
        if (header.getAggregateVersion() == 0) {
            return "missing DetectionBusiness aggregate_version";
        }
        if (header.getOccurredAt() <= 0 || header.getProducedAt() <= 0) {
            return "timestamp: DetectionBusiness occurred_at/produced_at must be positive";
        }
        if (!header.getEventId().equals(header.getIdempotencyKey())) {
            return "DetectionBusiness idempotency_key must equal event_id";
        }
        if (detection.getCommunityId().trim().isEmpty()) return "missing DetectionBusiness community_id";
        if (detection.getDetectionType().trim().isEmpty()) return "missing DetectionBusiness detection_type";
        if (detection.getLabel().trim().isEmpty()) return "missing DetectionBusiness label";
        if (!Float.isFinite(detection.getScore()) || detection.getScore() <= 0.0f) {
            return "missing or invalid DetectionBusiness score";
        }
        if (detection.getTs() <= 0 || header.getEventTs() <= 0) {
            return "timestamp: DetectionBusiness event timestamps must be positive";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord source,
            DetectionBusiness detection,
            String code,
            String type,
            String message) {
        EventHeader header = detection != null && detection.hasHeader()
                ? detection.getHeader() : EventHeader.getDefaultInstance();
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, code, type, message,
                header.getTenantId().isEmpty() ? source.header("tenant_id") : header.getTenantId(),
                header.getEventId(), header.getTraceId(), header.getRunId(), header.getProbeId(),
                "flink-alert-generator-job", "traffic.v1.DetectionBusiness", "v1");
        return new ParseResult(null, failure);
    }

    static final class ParseResult {
        final DetectionBusiness detection;
        final CanonicalDlqMessage failure;

        ParseResult(DetectionBusiness detection, CanonicalDlqMessage failure) {
            this.detection = detection;
            this.failure = failure;
        }
    }
}
