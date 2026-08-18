package com.traffic.flink.behavior.user.baseline;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

class BaselineLifecycleContractTest {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final String CANDIDATE = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    private static final String SNAPSHOT = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb";
    private static final String CONSUMER = "flink-user-behavior-job";

    @Test
    void strictActivationRequestRetainsImmutableThresholdSnapshot() throws Exception {
        RawKafkaRecord record = activationRecord(CONSUMER, Map.of());
        BaselineLifecycleEvent event = BaselineLifecycleParseFunction.parse(
                record, "baseline.lifecycle.v1", CANDIDATE, CONSUMER);

        assertNotNull(event);
        assertEquals(7, ((Number) event.thresholdSpec.get("brute_force_max_failures")).intValue());
        assertEquals(1_700_000_000_000L, event.sourceTimestamp);
        assertEquals(64, event.payloadSha256.length());
        BaselineSnapshot snapshot = new BaselineSnapshot(event);
        assertEquals(7, snapshot.intThreshold("brute_force_max_failures", 5, 1, 100));
        assertEquals(300L, snapshot.longThreshold("brute_force_window_seconds", 600L, 1L, 3_600L));
    }

    @Test
    void nonTargetConsumerIsIgnoredWithoutClaimingApplication() throws Exception {
        RawKafkaRecord record = activationRecord("another-consumer", Map.of());
        assertNull(BaselineLifecycleParseFunction.parse(
                record, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));
    }

    @Test
    void changedCandidateUnknownFieldDuplicateHeaderAndHeaderMismatchFailClosed() throws Exception {
        RawKafkaRecord valid = activationRecord(CONSUMER, Map.of());
        assertThrows(IllegalArgumentException.class, () -> BaselineLifecycleParseFunction.parse(
                valid, "baseline.lifecycle.v1", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", CONSUMER));

        RawKafkaRecord unknown = activationRecord(CONSUMER, Map.of("unversioned_threshold", 9));
        assertThrows(Exception.class, () -> BaselineLifecycleParseFunction.parse(
                unknown, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));

        RawKafkaRecord duplicate = copy(valid, valid.getHeaders(), Set.of("event_id"));
        assertThrows(IllegalArgumentException.class, () -> BaselineLifecycleParseFunction.parse(
                duplicate, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));

        Map<String, String> wrongHeaders = valid.getHeaders();
        wrongHeaders.put("baseline_version", "8");
        RawKafkaRecord mismatch = copy(valid, wrongHeaders, Set.of());
        assertThrows(IllegalArgumentException.class, () -> BaselineLifecycleParseFunction.parse(
                mismatch, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));
    }

    @Test
    void ackBytesAndHeadersAreReplayStableAndCandidateBound() throws Exception {
        BaselineLifecycleEvent event = BaselineLifecycleParseFunction.parse(
                activationRecord(CONSUMER, Map.of()), "baseline.lifecycle.v1", CANDIDATE, CONSUMER);
        BaselineActivationAck first = BaselineActivationAck.staged(event, CONSUMER);
        BaselineActivationAck replay = BaselineActivationAck.staged(event, CONSUMER);
        assertEquals(first.eventId, replay.eventId);
        assertEquals(first.ackSha256, replay.ackSha256);
        assertArrayEquals(first.toJson(), replay.toJson());

        ProducerRecord<byte[], byte[]> record =
                new BaselineActivationAckKafkaSinkFactory.AckSerializer("baseline.activation-acks.v1")
                        .serialize(first, null, event.sourceTimestamp);
        assertEquals("tenant-a:user:user-a", new String(record.key(), StandardCharsets.UTF_8));
        assertEquals(CANDIDATE, header(record, "candidate_sha256"));
        assertEquals(SNAPSHOT, header(record, "snapshot_sha256"));
        assertEquals("7", header(record, "baseline_version"));
        assertEquals("baseline.activation-acks.v1", header(record, "target_topic"));
    }

    @Test
    void activatedEventMustMatchStagedVersionAndSnapshot() throws Exception {
        BaselineLifecycleEvent requested = BaselineLifecycleParseFunction.parse(
                activationRecord(CONSUMER, Map.of()), "baseline.lifecycle.v1", CANDIDATE, CONSUMER);
        BaselineSnapshot staged = new BaselineSnapshot(requested);
        BaselineLifecycleEvent activated = new BaselineLifecycleEvent();
        activated.tenantId = requested.tenantId;
        activated.baselineId = requested.baselineId;
        activated.baselineVersion = requested.baselineVersion;
        activated.candidateSha256 = requested.candidateSha256;
        activated.snapshotSha256 = requested.snapshotSha256;
        BaselineLifecycleProcessFunction.validateActivation(staged, activated);

        activated.snapshotSha256 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd";
        assertThrows(IllegalStateException.class,
                () -> BaselineLifecycleProcessFunction.validateActivation(staged, activated));
        assertThrows(IllegalStateException.class,
                () -> BaselineLifecycleProcessFunction.validateActivation(null, activated));
    }

    @Test
    void retirementEventValidatesReplacementIdentityBeforeBeingIgnoredByInference() throws Exception {
        RawKafkaRecord source = activationRecord(CONSUMER, Map.of());
        @SuppressWarnings("unchecked")
        Map<String, Object> body = JSON.readValue(source.getValue(), LinkedHashMap.class);
        body.put("event_id", "00000000-0000-4000-8000-000000000603");
        body.put("event_type", "baseline.version.retired.v1");
        body.put("baseline_version", 6);
        body.put("retired_by_version", 7);
        Map<String, String> headers = new LinkedHashMap<>(source.getHeaders());
        headers.put("event_id", String.valueOf(body.get("event_id")));
        headers.put("event_type", String.valueOf(body.get("event_type")));
        headers.put("baseline_version", "6");
        RawKafkaRecord retired = new RawKafkaRecord(source.getTopic(), source.getPartition(), source.getOffset(),
                source.getTimestamp(), source.getKey(), JSON.writeValueAsBytes(body), headers);
        assertNull(BaselineLifecycleParseFunction.parse(
                retired, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));

        body.put("retired_by_version", 6);
        RawKafkaRecord invalid = new RawKafkaRecord(source.getTopic(), source.getPartition(), source.getOffset(),
                source.getTimestamp(), source.getKey(), JSON.writeValueAsBytes(body), headers);
        assertThrows(IllegalArgumentException.class, () -> BaselineLifecycleParseFunction.parse(
                invalid, "baseline.lifecycle.v1", CANDIDATE, CONSUMER));
    }

    private static RawKafkaRecord activationRecord(String targetConsumer, Map<String, Object> extras) throws Exception {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("event_id", "00000000-0000-4000-8000-000000000601");
        body.put("event_type", "baseline.activation.requested.v1");
        body.put("schema_version", 1);
        body.put("partition_key", "tenant-a:user:user-a");
        body.put("tenant_id", "tenant-a");
        body.put("baseline_id", "user:user-a");
        body.put("baseline_kind", "dynamic");
        body.put("algorithm_version", "zscore-v1");
        body.put("baseline_version", 7);
        body.put("definition_revision", 3);
        body.put("candidate_sha256", CANDIDATE);
        body.put("snapshot_sha256", SNAPSHOT);
        body.put("threshold_spec", Map.of(
                "brute_force_max_failures", 7,
                "brute_force_window_seconds", 300));
        body.put("statistics", Map.of("mean", 2.5, "stddev", 0.75));
        body.put("approval_id", "00000000-0000-4000-8000-000000000602");
        body.put("expected_consumers", List.of(targetConsumer));
        body.put("trace_id", "trace-baseline-1");
        body.putAll(extras);
        byte[] payload = JSON.writeValueAsBytes(body);
        Map<String, String> headers = new LinkedHashMap<>();
        headers.put("event_id", String.valueOf(body.get("event_id")));
        headers.put("event_type", String.valueOf(body.get("event_type")));
        headers.put("schema_version", "1");
        headers.put("tenant_id", "tenant-a");
        headers.put("baseline_id", "user:user-a");
        headers.put("baseline_version", "7");
        headers.put("candidate_sha256", CANDIDATE);
        headers.put("snapshot_sha256", SNAPSHOT);
        headers.put("trace_id", "trace-baseline-1");
        headers.put("target_topic", "baseline.lifecycle.v1");
        return new RawKafkaRecord("baseline.lifecycle.v1", 1, 17L, 1_700_000_000_000L,
                "tenant-a:user:user-a".getBytes(StandardCharsets.UTF_8), payload, headers);
    }

    private static RawKafkaRecord copy(
            RawKafkaRecord source, Map<String, String> headers, Set<String> duplicates) {
        return new RawKafkaRecord(source.getTopic(), source.getPartition(), source.getOffset(),
                source.getTimestamp(), source.getKey(), source.getValue(), headers, duplicates);
    }

    private static String header(ProducerRecord<byte[], byte[]> record, String name) {
        return new String(record.headers().lastHeader(name).value(), StandardCharsets.UTF_8);
    }
}
