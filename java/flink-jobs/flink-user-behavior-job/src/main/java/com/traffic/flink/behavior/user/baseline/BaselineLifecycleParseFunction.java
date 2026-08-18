package com.traffic.flink.behavior.user.baseline;

import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.UUID;
import java.util.regex.Pattern;

/** Fails the checkpoint on malformed, cross-candidate or identity-changing lifecycle records. */
public final class BaselineLifecycleParseFunction
        extends ProcessFunction<RawKafkaRecord, BaselineLifecycleEvent> {
    private static final long serialVersionUID = 1L;
    private static final Pattern SHA256 = Pattern.compile("[0-9a-f]{64}");
    private static final Set<String> EVENT_TYPES = Set.of(
            "baseline.build.requested.v1", "baseline.version.frozen.v1",
            "baseline.version.failed.v1", "baseline.activation.requested.v1",
            "baseline.version.activated.v1", "baseline.version.retired.v1");
    private static final ObjectMapper JSON = new ObjectMapper()
            .enable(JsonParser.Feature.STRICT_DUPLICATE_DETECTION)
            .enable(DeserializationFeature.FAIL_ON_TRAILING_TOKENS)
            .enable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES);

    private final String topic;
    private final String candidateSha256;
    private final String consumerId;

    public BaselineLifecycleParseFunction(String topic, String candidateSha256, String consumerId) {
        if (!"baseline.lifecycle.v1".equals(topic)
                || !SHA256.matcher(candidateSha256 == null ? "" : candidateSha256).matches()
                || consumerId == null || consumerId.isBlank()) {
            throw new IllegalArgumentException("exact baseline lifecycle topic, candidate and consumer are required");
        }
        this.topic = topic;
        this.candidateSha256 = candidateSha256;
        this.consumerId = consumerId;
    }

    @Override
    public void processElement(RawKafkaRecord record, Context context, Collector<BaselineLifecycleEvent> out)
            throws Exception {
        BaselineLifecycleEvent event = parse(record, topic, candidateSha256, consumerId);
        if (event != null) out.collect(event);
    }

    static BaselineLifecycleEvent parse(
            RawKafkaRecord record, String topic, String candidateSha256, String consumerId) throws Exception {
        if (record == null || !topic.equals(record.getTopic()) || record.getValue() == null
                || record.getValue().length == 0 || !record.getDuplicateHeaderNames().isEmpty()
                || record.getTimestamp() <= 0) {
            throw new IllegalArgumentException("invalid behavior baseline lifecycle source record");
        }
        BaselineLifecycleEvent event = JSON.readValue(record.getValue(), BaselineLifecycleEvent.class);
        if (event.schemaVersion != 1 || !EVENT_TYPES.contains(event.eventType)
                || blank(event.eventId) || blank(event.tenantId) || blank(event.baselineId)
                || blank(event.traceId) || !candidateSha256.equals(event.candidateSha256)
                || !(event.tenantId + ":" + event.baselineId).equals(event.partitionKey)
                || !event.partitionKey.equals(record.keyAsString())) {
            throw new IllegalArgumentException("behavior baseline lifecycle envelope is invalid");
        }
        UUID.fromString(event.eventId);
        requireHeader(record, "event_id", event.eventId);
        requireHeader(record, "event_type", event.eventType);
        requireHeader(record, "schema_version", "1");
        requireHeader(record, "tenant_id", event.tenantId);
        requireHeader(record, "baseline_id", event.baselineId);
        requireHeader(record, "candidate_sha256", event.candidateSha256);
        requireHeader(record, "trace_id", event.traceId);
        requireHeader(record, "target_topic", topic);
        if ((event.isActivationRequested() || event.isActivated() || event.isRetired())
                && (!Long.toString(event.baselineVersion).equals(record.header("baseline_version"))
                || event.baselineVersion <= 0 || !SHA256.matcher(nullToEmpty(event.snapshotSha256)).matches()
                || !event.snapshotSha256.equals(record.header("snapshot_sha256")))) {
            throw new IllegalArgumentException("behavior baseline version header or snapshot is invalid");
        }
        if (event.isRetired() && (event.retiredByVersion <= 0 || event.retiredByVersion == event.baselineVersion)) {
            throw new IllegalArgumentException("behavior baseline retirement replacement is invalid");
        }
        if (event.isActivationRequested()) {
            List<String> expected = event.expectedConsumers == null
                    ? List.of() : new ArrayList<>(event.expectedConsumers);
            if (event.definitionRevision <= 0 || blank(event.baselineKind) || blank(event.algorithmVersion)
                    || event.thresholdSpec == null || event.statistics == null || blank(event.approvalId)
                    || expected.isEmpty() || new HashSet<>(expected).size() != expected.size()) {
                throw new IllegalArgumentException("behavior baseline activation request is incomplete");
            }
            List<String> sorted = new ArrayList<>(expected);
            sorted.sort(String::compareTo);
            if (!sorted.equals(expected)) {
                throw new IllegalArgumentException("behavior baseline target consumers must be sorted");
            }
            if (!event.addressedTo(consumerId)) return null;
        }
        if (!event.isActivationRequested() && !event.isActivated()) return null;
        event.payloadSha256 = hex(MessageDigest.getInstance("SHA-256").digest(record.getValue()));
        event.sourceTimestamp = record.getTimestamp();
        return event;
    }

    private static void requireHeader(RawKafkaRecord record, String name, String value) {
        if (!nullToEmpty(value).equals(record.header(name))) {
            throw new IllegalArgumentException("behavior baseline lifecycle " + name + " header/body mismatch");
        }
    }

    private static boolean blank(String value) { return value == null || value.isBlank(); }
    private static String nullToEmpty(String value) { return value == null ? "" : value; }

    private static String hex(byte[] bytes) {
        StringBuilder result = new StringBuilder(bytes.length * 2);
        for (byte value : bytes) result.append(String.format("%02x", value & 0xff));
        return result.toString();
    }
}
