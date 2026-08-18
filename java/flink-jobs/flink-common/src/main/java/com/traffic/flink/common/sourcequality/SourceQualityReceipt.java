package com.traffic.flink.common.sourcequality;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;

/** Canonical source-quality receipt encoded as the existing AuditEventV1 JSON shape. */
public final class SourceQualityReceipt implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final Set<String> RAILS = Set.of("flow", "asset", "device_log", "user_behavior");
    private static final Set<String> CATEGORIES = Set.of(
            "accepted", "rejected", "invalid", "late", "duplicate", "conflict", "missing");

    private final String receiptId;
    private final String tenantId;
    private final String rail;
    private final String consumerGroup;
    private final String topic;
    private final int partition;
    private final long offset;
    private final String category;
    private final String eventId;
    private final String sourceSha256;
    private final long watermarkMs;
    private final long observedAtMs;
    private final String reasonCode;

    public SourceQualityReceipt(
            String tenantId,
            String rail,
            String consumerGroup,
            String topic,
            int partition,
            long offset,
            String category,
            String eventId,
            String sourceSha256,
            long watermarkMs,
            long observedAtMs,
            String reasonCode) {
        this.tenantId = requireText(tenantId, "tenantId");
        this.rail = requireMember(rail, RAILS, "rail");
        this.consumerGroup = requireText(consumerGroup, "consumerGroup");
        this.topic = requireText(topic, "topic");
        if (partition < 0 || offset < 0L) {
            throw new IllegalArgumentException("source partition and offset must not be negative");
        }
        this.partition = partition;
        this.offset = offset;
        this.category = requireMember(category, CATEGORIES, "category");
        this.eventId = eventId == null ? "" : eventId.trim();
        this.sourceSha256 = requireSha256(sourceSha256);
        this.watermarkMs = watermarkMs;
        if (observedAtMs <= 0L) throw new IllegalArgumentException("observedAtMs must be positive");
        this.observedAtMs = observedAtMs;
        this.reasonCode = reasonCode == null ? "" : reasonCode.trim();
        if ("accepted".equals(category) && !this.reasonCode.isEmpty()) {
            throw new IllegalArgumentException("accepted receipt must not have a reasonCode");
        }
        if (!"accepted".equals(category) && this.reasonCode.isEmpty()) {
            throw new IllegalArgumentException("non-accepted receipt requires reasonCode");
        }
        this.receiptId = "source-quality-" + sha256Hex(
                "source-quality/v1\0" + this.tenantId + "\0" + this.rail + "\0"
                        + this.consumerGroup + "\0" + this.topic + "\0"
                        + this.partition + "\0" + this.offset);
    }

    public byte[] toAuditEventJson() {
        Map<String, Object> detail = new LinkedHashMap<>();
        detail.put("contract_version", "source-quality-receipt/v1");
        detail.put("receipt_id", receiptId);
        detail.put("tenant_id", tenantId);
        detail.put("rail", rail);
        detail.put("consumer_group", consumerGroup);
        Map<String, Object> source = new LinkedHashMap<>();
        source.put("topic", topic);
        source.put("partition", partition);
        source.put("offset", offset);
        detail.put("source", source);
        detail.put("category", category);
        detail.put("event_id", eventId);
        detail.put("source_sha256", sourceSha256);
        detail.put("watermark_ms", watermarkMs);
        detail.put("observed_at_ms", observedAtMs);
        detail.put("reason_code", reasonCode);

        Map<String, Object> event = new LinkedHashMap<>();
        event.put("event_id", receiptId);
        event.put("tenant_id", tenantId);
        event.put("user_id", "system:" + consumerGroup);
        event.put("action", "SOURCE_QUALITY_" + category);
        event.put("resource_type", "source_quality_receipt");
        event.put("object_type", "source_quality_receipt");
        event.put("resource_id", receiptId);
        event.put("object_id", receiptId);
        event.put("detail", detail);
        event.put("timestamp", Instant.ofEpochMilli(observedAtMs).toString());
        try {
            return JSON.writeValueAsBytes(event);
        } catch (JsonProcessingException error) {
            throw new IllegalStateException("source quality receipt JSON encoding failed", error);
        }
    }

    public String getReceiptId() { return receiptId; }
    public String getTenantId() { return tenantId; }
    public String getConsumerGroup() { return consumerGroup; }
    public String getTopic() { return topic; }
    public int getPartition() { return partition; }
    public long getOffset() { return offset; }
    public String getCategory() { return category; }

    public static String hashSource(byte[] value) {
        return sha256Hex(value == null ? new byte[0] : value);
    }

    private static String requireText(String value, String field) {
        String normalized = value == null ? "" : value.trim();
        if (normalized.isEmpty()) throw new IllegalArgumentException(field + " is required");
        return normalized;
    }

    private static String requireMember(String value, Set<String> allowed, String field) {
        String normalized = requireText(value, field);
        if (!allowed.contains(normalized)) throw new IllegalArgumentException(field + " is unsupported");
        return normalized;
    }

    private static String requireSha256(String value) {
        String normalized = requireText(value, "sourceSha256");
        if (!normalized.matches("[0-9a-f]{64}")) {
            throw new IllegalArgumentException("sourceSha256 must be lowercase SHA-256");
        }
        return normalized;
    }

    private static String sha256Hex(String value) {
        return sha256Hex(value.getBytes(StandardCharsets.UTF_8));
    }

    private static String sha256Hex(byte[] value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(value);
            StringBuilder result = new StringBuilder(64);
            for (byte item : digest) result.append(String.format("%02x", item & 0xff));
            return result.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }
}
