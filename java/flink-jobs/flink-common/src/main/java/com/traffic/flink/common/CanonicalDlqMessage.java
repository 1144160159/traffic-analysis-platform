package com.traffic.flink.common;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.Serializable;
import java.time.Instant;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;

/** Canonical dlq.v1 JSON payload defined by kafka-json-events-v1.schema.json. */
public final class CanonicalDlqMessage implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final ObjectMapper JSON = new ObjectMapper();

    private final Map<String, Object> fields;

    private CanonicalDlqMessage(Map<String, Object> fields) {
        this.fields = new LinkedHashMap<>(fields);
    }

    public static CanonicalDlqMessage failure(
            RawKafkaRecord source,
            String errorCode,
            String errorType,
            String errorMessage,
            String tenantId,
            String eventId,
            String traceId,
            String runId,
            String probeId) {
        return failure(
                source, errorCode, errorType, errorMessage,
                tenantId, eventId, traceId, runId, probeId,
                "flink-feature-job", "traffic.v1.SessionEvent", "v1");
    }

    public static CanonicalDlqMessage failure(
            RawKafkaRecord source,
            String errorCode,
            String errorType,
            String errorMessage,
            String tenantId,
            String eventId,
            String traceId,
            String runId,
            String probeId,
            String serviceName,
            String protoMessageType,
            String protoSchemaVersion) {
        return failure(
                source, errorCode, errorType, errorMessage,
                tenantId, eventId, traceId, runId, probeId,
                serviceName, "application/x-protobuf", protoMessageType, protoSchemaVersion);
    }

    public static CanonicalDlqMessage failure(
            RawKafkaRecord source,
            String errorCode,
            String errorType,
            String errorMessage,
            String tenantId,
            String eventId,
            String traceId,
            String runId,
            String probeId,
            String serviceName,
            String contentType,
            String messageType,
            String schemaVersion) {
        String now = Instant.now().toString();
        Map<String, Object> value = new LinkedHashMap<>();
        value.put("original_topic", source.getTopic());
        value.put("original_partition", source.getPartition());
        value.put("original_offset", source.getOffset());
        value.put("original_key", source.keyAsString());
        value.put("original_value_b64", Base64.getEncoder().encodeToString(nullToEmpty(source.getValue())));
        value.put("original_headers", source.getHeaders());
        value.put("original_timestamp", source.getTimestamp() >= 0
                ? Instant.ofEpochMilli(source.getTimestamp()).toString() : "");
        value.put("content_type", nonBlank(contentType, "application/octet-stream"));
        value.put("proto_message_type", nonBlank(messageType, "unknown"));
        value.put("proto_schema_version", nonBlank(schemaVersion, "unknown"));
        value.put("error_code", nonBlank(errorCode, "PROCESSING_FAILED"));
        value.put("error_message", truncate(errorMessage, 1024));
        value.put("error_type", nonBlank(errorType, "processing_error"));
        value.put("failed_at", now);
        value.put("retry_count", 0);
        value.put("service_name", nonBlank(serviceName, "unknown"));
        value.put("processed_at", now);
        putOptional(value, "tenant_id", tenantId);
        putOptional(value, "event_id", eventId);
        putOptional(value, "trace_id", traceId);
        putOptional(value, "run_id", runId);
        putOptional(value, "probe_id", probeId);
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("source_tuple", String.format("%s:%d:%d",
                source.getTopic(), source.getPartition(), source.getOffset()));
        value.put("metadata", metadata);
        Map<String, Object> replayPolicy = new LinkedHashMap<>();
        replayPolicy.put("max_retries", 0);
        replayPolicy.put("retryable_errors", new String[0]);
        replayPolicy.put("require_manual_ack", true);
        value.put("replay_policy", replayPolicy);
        return new CanonicalDlqMessage(value);
    }

    public String originalTopic() { return (String) fields.get("original_topic"); }
    public int originalPartition() { return (Integer) fields.get("original_partition"); }
    public long originalOffset() { return (Long) fields.get("original_offset"); }
    public String tenantId() { return (String) fields.getOrDefault("tenant_id", "unknown"); }
    public String errorCode() { return (String) fields.get("error_code"); }
    public Map<String, Object> fields() { return new LinkedHashMap<>(fields); }

    public String toJson() {
        try {
            return JSON.writeValueAsString(fields);
        } catch (JsonProcessingException e) {
            throw new IllegalStateException("canonical DLQ JSON serialization failed", e);
        }
    }

    private static void putOptional(Map<String, Object> target, String key, String value) {
        if (value != null && !value.isEmpty()) target.put(key, value);
    }

    private static String nonBlank(String value, String fallback) {
        return value == null || value.isEmpty() ? fallback : value;
    }

    private static String truncate(String value, int max) {
        if (value == null) return "";
        return value.length() <= max ? value : value.substring(0, max);
    }

    private static byte[] nullToEmpty(byte[] value) {
        return value == null ? new byte[0] : value;
    }
}
