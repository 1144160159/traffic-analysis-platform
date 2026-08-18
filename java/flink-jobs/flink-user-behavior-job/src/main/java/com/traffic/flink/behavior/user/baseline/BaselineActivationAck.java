package com.traffic.flink.behavior.user.baseline;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.annotation.JsonPropertyOrder;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.DeterministicId;

import java.io.Serializable;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;

/** Replay-stable acknowledgement emitted after the immutable snapshot is staged. */
@JsonPropertyOrder({
        "event_id", "event_type", "schema_version", "partition_key", "tenant_id", "baseline_id",
        "baseline_version", "consumer_id", "candidate_sha256", "snapshot_sha256", "ack_sha256",
        "applied_at", "trace_id"
})
public final class BaselineActivationAck implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final ObjectMapper JSON = new ObjectMapper();

    @JsonProperty("event_id") public String eventId;
    @JsonProperty("event_type") public String eventType;
    @JsonProperty("schema_version") public int schemaVersion;
    @JsonProperty("partition_key") public String partitionKey;
    @JsonProperty("tenant_id") public String tenantId;
    @JsonProperty("baseline_id") public String baselineId;
    @JsonProperty("baseline_version") public long baselineVersion;
    @JsonProperty("consumer_id") public String consumerId;
    @JsonProperty("candidate_sha256") public String candidateSha256;
    @JsonProperty("snapshot_sha256") public String snapshotSha256;
    @JsonProperty("ack_sha256") public String ackSha256;
    @JsonProperty("applied_at") public String appliedAt;
    @JsonProperty("trace_id") public String traceId;

    public BaselineActivationAck() {}

    public static BaselineActivationAck staged(BaselineLifecycleEvent event, String consumerId) {
        if (event == null || !event.isActivationRequested() || event.sourceTimestamp <= 0) {
            throw new IllegalArgumentException("staged baseline activation event and source timestamp are required");
        }
        BaselineActivationAck ack = new BaselineActivationAck();
        ack.eventId = DeterministicId.uuid(
                "behavior-baseline-activation-ack/v1", event.eventId, consumerId);
        ack.eventType = "baseline.activation.acknowledged.v1";
        ack.schemaVersion = 1;
        ack.partitionKey = event.partitionKey;
        ack.tenantId = event.tenantId;
        ack.baselineId = event.baselineId;
        ack.baselineVersion = event.baselineVersion;
        ack.consumerId = consumerId;
        ack.candidateSha256 = event.candidateSha256;
        ack.snapshotSha256 = event.snapshotSha256;
        ack.appliedAt = Instant.ofEpochMilli(event.sourceTimestamp).toString();
        ack.traceId = event.traceId;
        ack.ackSha256 = sha256(
                event.eventId, event.payloadSha256, consumerId, event.candidateSha256,
                event.snapshotSha256, Long.toString(event.baselineVersion));
        return ack;
    }

    public byte[] toJson() {
        try {
            return JSON.writeValueAsBytes(this);
        } catch (Exception error) {
            throw new IllegalStateException("serialize behavior baseline activation ACK", error);
        }
    }

    private static String sha256(String... values) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (String value : values) {
                byte[] bytes = value == null ? null : value.getBytes(StandardCharsets.UTF_8);
                digest.update(ByteBuffer.allocate(4).putInt(bytes == null ? -1 : bytes.length).array());
                if (bytes != null) digest.update(bytes);
            }
            StringBuilder result = new StringBuilder(64);
            for (byte value : digest.digest()) result.append(String.format("%02x", value & 0xff));
            return result.toString();
        } catch (Exception error) {
            throw new IllegalStateException("SHA-256 is required", error);
        }
    }
}
