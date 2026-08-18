package com.traffic.flink.common.sourcefact;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.Base64;
import java.util.Set;

/**
 * Immutable source fact written after a rail has accepted a Kafka record.
 *
 * <p>The record deliberately keeps the original protobuf bytes and broker
 * coordinates. A ClickHouse table built from this carrier can therefore be
 * replayed without inventing event time, source identity, or a processing-time
 * version.</p>
 */
public final class SourceFactRecord implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final Set<String> RAILS =
            Set.of("flow", "asset", "device_log", "user_behavior");

    private final String rail;
    private final String tenantId;
    private final String aggregateId;
    private final String eventId;
    private final long eventTimeMs;
    private final long ingestTimeMs;
    private final String schemaVersion;
    private final String sourceTopic;
    private final int sourcePartition;
    private final long sourceOffset;
    private final long sourceTimestampMs;
    private final String sourcePayloadSha256;
    private final long sourceVersion;
    private final String projectionIdentity;
    private final String sourceQualityReceiptId;
    private final String payloadBase64;
    private final String projectionHash;

    private SourceFactRecord(
            String rail,
            String tenantId,
            String aggregateId,
            String eventId,
            long eventTimeMs,
            long ingestTimeMs,
            String schemaVersion,
            RawKafkaRecord source,
            String consumerGroup,
            long sourceVersion) {
        this.rail = requireText(rail, "rail");
        if (!RAILS.contains(this.rail)) {
            throw new IllegalArgumentException("unsupported source-fact rail: " + rail);
        }
        this.tenantId = requireText(tenantId, "tenantId");
        this.aggregateId = requireText(aggregateId, "aggregateId");
        this.eventId = requireText(eventId, "eventId");
        if (eventTimeMs <= 0L || ingestTimeMs <= 0L) {
            throw new IllegalArgumentException("event and ingest time must be positive");
        }
        this.eventTimeMs = eventTimeMs;
        this.ingestTimeMs = ingestTimeMs;
        this.schemaVersion = requireText(schemaVersion, "schemaVersion");
        if (source == null) throw new IllegalArgumentException("source is required");
        this.sourceTopic = requireText(source.getTopic(), "sourceTopic");
        if (source.getPartition() < 0 || source.getOffset() < 0L || source.getTimestamp() <= 0L) {
            throw new IllegalArgumentException("source coordinates and timestamp must be valid");
        }
        this.sourcePartition = source.getPartition();
        this.sourceOffset = source.getOffset();
        this.sourceTimestampMs = source.getTimestamp();
        if (sourceVersion <= 0L) {
            throw new IllegalArgumentException("sourceVersion must be positive");
        }
        this.sourceVersion = sourceVersion;
        byte[] payload = source.getValue();
        this.sourcePayloadSha256 = SourceQualityReceipt.hashSource(payload);
        this.payloadBase64 = Base64.getEncoder().encodeToString(
                payload == null ? new byte[0] : payload);
        this.projectionIdentity = sha256Hex(
                "source-fact/v1\0" + this.rail + "\0" + this.tenantId + "\0" + this.eventId);
        this.sourceQualityReceiptId = "source-quality-" + sha256Hex(
                "source-quality/v1\0" + this.tenantId + "\0" + this.rail + "\0"
                        + requireText(consumerGroup, "consumerGroup") + "\0"
                        + this.sourceTopic + "\0" + this.sourcePartition + "\0" + this.sourceOffset);
        this.projectionHash = sha256Hex(
                "source-fact-projection/v1\0" + this.projectionIdentity + "\0"
                        + this.sourceVersion + "\0" + this.sourcePayloadSha256);
    }

    public static SourceFactRecord fromAccepted(
            String rail,
            String tenantId,
            String aggregateId,
            String eventId,
            long eventTimeMs,
            long ingestTimeMs,
            String schemaVersion,
            RawKafkaRecord source,
            String consumerGroup,
            long sourceVersion) {
        return new SourceFactRecord(
                rail, tenantId, aggregateId, eventId, eventTimeMs, ingestTimeMs,
                schemaVersion, source, consumerGroup, sourceVersion);
    }

    public String getRail() { return rail; }
    public String getTenantId() { return tenantId; }
    public String getAggregateId() { return aggregateId; }
    public String getEventId() { return eventId; }
    public long getEventTimeMs() { return eventTimeMs; }
    public long getIngestTimeMs() { return ingestTimeMs; }
    public String getSchemaVersion() { return schemaVersion; }
    public String getSourceTopic() { return sourceTopic; }
    public int getSourcePartition() { return sourcePartition; }
    public long getSourceOffset() { return sourceOffset; }
    public long getSourceTimestampMs() { return sourceTimestampMs; }
    public String getSourcePayloadSha256() { return sourcePayloadSha256; }
    public long getSourceVersion() { return sourceVersion; }
    public String getProjectionIdentity() { return projectionIdentity; }
    public String getSourceQualityReceiptId() { return sourceQualityReceiptId; }
    public String getPayloadBase64() { return payloadBase64; }
    public String getProjectionHash() { return projectionHash; }

    private static String requireText(String value, String field) {
        String normalized = value == null ? "" : value.trim();
        if (normalized.isEmpty()) throw new IllegalArgumentException(field + " is required");
        return normalized;
    }

    private static String sha256Hex(String value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256")
                    .digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(64);
            for (byte item : digest) result.append(String.format("%02x", item & 0xff));
            return result.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }
}
