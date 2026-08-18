package com.traffic.flink.behavior.user;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.util.Set;

/** Strict UserEvent envelope parser that never discards the source tuple. */
public final class UserEventParseFunction
        extends ProcessFunction<RawKafkaRecord, ValidatedUserEvent> {
    private static final long serialVersionUID = 1L;
    private static final int MAX_PAYLOAD_BYTES = 262_144;
    private static final Set<String> EVENT_TYPES = Set.of("user_create", "oidc_sync", "settings_update");
    private static final Set<String> RESULTS = Set.of("success", "denied", "error");

    private final String inputTopic;
    private final String consumerGroup;
    private final EventTimePolicy eventTimePolicy;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final OutputTag<SourceQualityReceipt> qualityTag;

    public UserEventParseFunction(
            String inputTopic,
            String consumerGroup,
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag) {
        this.inputTopic = inputTopic;
        this.consumerGroup = consumerGroup;
        this.eventTimePolicy = eventTimePolicy;
        this.dlqTag = dlqTag;
        this.qualityTag = qualityTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord source,
            Context context,
            Collector<ValidatedUserEvent> out) {
        ParseResult result = parse(source, inputTopic, eventTimePolicy.getMaxFutureSkewMs());
        if (result.input != null) {
            out.collect(result.input);
            return;
        }
        context.output(dlqTag, result.failure);
        context.output(qualityTag, rejectionReceipt(
                source, result.failure, consumerGroup,
                context.timerService().currentProcessingTime()));
    }

    static ParseResult parse(RawKafkaRecord source, String inputTopic, long maxFutureSkewMs) {
        if (source == null) throw new IllegalArgumentException("source record is required");
        if (!inputTopic.equals(source.getTopic())) {
            return failure(source, null, "WRONG_SOURCE_TOPIC", "expected " + inputTopic);
        }
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0 || payload.length > MAX_PAYLOAD_BYTES) {
            return failure(source, null, "BAD_SCHEMA", "UserEvent payload size is invalid");
        }
        final UserEvent event;
        try {
            event = UserEvent.parseFrom(payload);
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "invalid UserEvent protobuf");
        }
        if (!event.getUnknownFields().asMap().isEmpty()) {
            return failure(source, event, "BAD_SCHEMA", "UserEvent contains unknown fields");
        }
        String valueError = validateValue(event);
        if (valueError != null) return failure(source, event, "VALIDATION_ERROR", valueError);
        String envelopeError = validateEnvelope(source, event, maxFutureSkewMs);
        if (envelopeError != null) {
            String code = envelopeError.startsWith("timestamp:") ? "BAD_TIMESTAMP" : "ENVELOPE_MISMATCH";
            return failure(source, event, code, envelopeError);
        }
        return new ParseResult(new ValidatedUserEvent(source, event), null);
    }

    static String validateValue(UserEvent event) {
        if (blank(event.getEventId()) || event.getEventId().length() > 128) return "invalid event_id";
        if (blank(event.getTenantId()) || event.getTenantId().length() > 128) return "invalid tenant_id";
        if (blank(event.getUserId()) || event.getUserId().length() > 128) return "invalid user_id";
        if (!EVENT_TYPES.contains(event.getEventType())) return "unsupported event_type";
        if (!RESULTS.contains(event.getResult())) return "unsupported result";
        if (event.getTimestamp() <= 0L) return "invalid event timestamp";
        return null;
    }

    static String validateEnvelope(RawKafkaRecord source, UserEvent event, long maxFutureSkewMs) {
        if (!source.getDuplicateHeaderNames().isEmpty()) return "duplicate Kafka headers";
        if (!(event.getTenantId() + ":" + event.getUserId()).equals(source.keyAsString())) {
            return "Kafka key must equal tenant_id:user_id";
        }
        if (!event.getTenantId().equals(source.header("tenant_id"))) return "tenant_id header mismatch";
        if (!event.getEventId().equals(source.header("event_id"))) return "event_id header mismatch";
        if (!"1".equals(source.header("schema_version"))) return "schema_version header mismatch";
        String aggregateVersion = source.header("aggregate_version");
        try {
            if (Long.parseLong(aggregateVersion) <= 0L) return "aggregate_version must be positive";
        } catch (Exception error) {
            return "aggregate_version header is invalid";
        }
        if (blank(source.header("event_type"))) return "event_type header is required";
        if (source.getTimestamp() <= 0L) return "timestamp: Kafka ingest timestamp must be positive";
        if (EventTimePolicy.isFuture(event.getTimestamp(), source.getTimestamp(), maxFutureSkewMs)) {
            return "timestamp: UserEvent exceeds ingest time plus future skew";
        }
        return null;
    }

    static SourceQualityReceipt rejectionReceipt(
            RawKafkaRecord source,
            CanonicalDlqMessage failure,
            String consumerGroup,
            long processingTimeMs) {
        Object eventID = failure.fields().get("event_id");
        long observedAt = source.getTimestamp() > 0L
                ? source.getTimestamp() : Math.max(1L, processingTimeMs);
        String category = "ENVELOPE_MISMATCH".equals(failure.errorCode())
                || "WRONG_SOURCE_TOPIC".equals(failure.errorCode()) ? "rejected" : "invalid";
        return new SourceQualityReceipt(
                failure.tenantId(), "user_behavior", consumerGroup,
                source.getTopic(), source.getPartition(), source.getOffset(), category,
                eventID == null ? "" : String.valueOf(eventID),
                SourceQualityReceipt.hashSource(source.getValue()), Long.MIN_VALUE,
                observedAt, failure.errorCode());
    }

    private static ParseResult failure(
            RawKafkaRecord source, UserEvent event, String code, String message) {
        String tenant = event == null || blank(event.getTenantId())
                ? source.header("tenant_id") : event.getTenantId();
        if (blank(tenant)) tenant = "unknown";
        String eventID = event == null ? source.header("event_id") : event.getEventId();
        return new ParseResult(null, CanonicalDlqMessage.failure(
                source, code, "user_event_validation", message,
                tenant, eventID, source.header("trace_id"), source.header("run_id"), "",
                "flink-user-behavior-job", "traffic.v1.UserEvent", "v1"));
    }

    private static boolean blank(String value) {
        return value == null || value.trim().isEmpty();
    }

    static final class ParseResult {
        final ValidatedUserEvent input;
        final CanonicalDlqMessage failure;
        ParseResult(ValidatedUserEvent input, CanonicalDlqMessage failure) {
            this.input = input;
            this.failure = failure;
        }
    }
}
