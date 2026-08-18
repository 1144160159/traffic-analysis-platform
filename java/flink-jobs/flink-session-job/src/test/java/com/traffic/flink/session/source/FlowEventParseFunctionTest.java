package com.traffic.flink.session.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FlowEventParseFunctionTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void parsesValidFlowEvent() {
        FlowEvent flow = validFlowBuilder().setHeader(validHeader("evt-1")).build();

        RawKafkaRecord record = record("tenant-a:1:abc", flow.toByteArray());

        FlowEventParseFunction.ParseResult result = FlowEventParseFunction.parseRecord(record);

        assertNotNull(result.input);
        assertNull(result.failure);
        assertEquals("tenant-a", result.input.getFlow().getHeader().getTenantId());
        assertEquals("1:abc", result.input.getFlow().getCommunityId());
        assertEquals(1L, result.input.getSource().getOffset());
    }

    @Test
    void invalidProtobufBytesBecomeCanonicalDeadLetter() throws Exception {
        byte[] payload = new byte[]{0};
        RawKafkaRecord record = new RawKafkaRecord(
                "flow.events.v1",
                2,
                33L,
                4567L,
                "tenant-a:bad".getBytes(StandardCharsets.UTF_8),
                payload,
                Map.of("tenant_id", "tenant-a"));

        FlowEventParseFunction.ParseResult result = FlowEventParseFunction.parseRecord(record);

        assertNull(result.input);
        assertNotNull(result.failure);
        JsonNode dlq = JSON.readTree(result.failure.toJson());
        assertEquals("tenant-a", dlq.get("tenant_id").asText());
        assertEquals("flow.events.v1", dlq.get("original_topic").asText());
        assertEquals(2, dlq.get("original_partition").asInt());
        assertEquals(33L, dlq.get("original_offset").asLong());
        assertEquals("tenant-a:bad", dlq.get("original_key").asText());
        assertTrue(dlq.get("error_message").asText().contains("invalid FlowEvent protobuf"));
        assertEquals("AA==", dlq.get("original_value_b64").asText());
        assertEquals("flink-session-job", dlq.get("service_name").asText());
        assertEquals("traffic.v1.FlowEvent", dlq.get("proto_message_type").asText());
        assertEquals("flow.events.v1:2:33", dlq.get("metadata").get("source_tuple").asText());
    }

    @Test
    void missingTenantBecomesDeadLetterInsteadOfSilentDrop() {
        FlowEvent flow = FlowEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId("evt-missing-tenant")
                        .setRunId("run-1")
                        .build())
                .setCommunityId("1:abc")
                .build();
        RawKafkaRecord record = new RawKafkaRecord(
                "flow.events.v1",
                0,
                9L,
                1000L,
                "tenant-from-key:1:abc".getBytes(StandardCharsets.UTF_8),
                flow.toByteArray(),
                Map.of());

        FlowEventParseFunction.ParseResult result = FlowEventParseFunction.parseRecord(record);

        assertNull(result.input);
        assertNotNull(result.failure);
        assertEquals("tenant-from-key", result.failure.tenantId());
        assertTrue(result.failure.toJson().contains("missing FlowEvent tenant_id"));
    }

    @Test
    void mismatchedFeatureSequenceBecomesDeadLetter() {
        FlowEvent flow = validFlowBuilder()
                .setFeatureObservation(TrafficFeatureObservation.newBuilder()
                        .setSchemaVersion("traffic-feature-observation/v1")
                        .setAlgorithmVersion("probe-packet-feature/v1")
                        .addSignedPacketLengths(100)
                        .build())
                .build();

        FlowEventParseFunction.ParseResult result =
                FlowEventParseFunction.parseRecord(record("tenant-a:1:abc", flow.toByteArray()));

        assertNull(result.input);
        assertEquals(
                "invalid feature observation sequence cardinality",
                result.failure.fields().get("error_message"));
    }

    @Test
    void invalidPayloadAccountingBecomesDeadLetter() {
        TrafficFeatureObservation.Builder observation = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("probe-packet-feature/v1")
                .setPayloadObservedBytes(8);
        for (int i = 0; i < 16; i++) observation.addPayloadNibbleCounts(0);
        FlowEvent flow = validFlowBuilder().setFeatureObservation(observation).build();

        FlowEventParseFunction.ParseResult result =
                FlowEventParseFunction.parseRecord(record("tenant-a:1:abc", flow.toByteArray()));

        assertNull(result.input);
        assertEquals(
                "invalid feature observation payload accounting",
                result.failure.fields().get("error_message"));
    }

    @Test
    void envelopeMismatchIsRejectedAndProducesTraceableReceipt() {
        FlowEvent flow = validFlowBuilder().build();
        RawKafkaRecord valid = record("tenant-a:1:abc", flow.toByteArray());
        RawKafkaRecord duplicateHeaders = new RawKafkaRecord(
                valid.getTopic(), valid.getPartition(), valid.getOffset(), valid.getTimestamp(),
                valid.getKey(), valid.getValue(), valid.getHeaders(), Set.of("event_id"));

        FlowEventParseFunction.ParseResult result =
                FlowEventParseFunction.parseRecord(duplicateHeaders);
        SourceQualityReceipt receipt = FlowEventParseFunction.rejectionReceipt(
                duplicateHeaders, result.failure, "flink-session-job-shadow-candidate", 2000L);

        assertNull(result.input);
        assertEquals("ENVELOPE_MISMATCH", result.failure.errorCode());
        assertEquals("rejected", receipt.getCategory());
        assertEquals(1L, receipt.getOffset());
    }

    @Test
    void tenantHeaderMismatchIsRejected() {
        FlowEvent flow = validFlowBuilder().build();
        RawKafkaRecord valid = record("tenant-a:1:abc", flow.toByteArray());
        java.util.HashMap<String, String> headers = new java.util.HashMap<>(valid.getHeaders());
        headers.put("tenant_id", "tenant-b");
        RawKafkaRecord forged = new RawKafkaRecord(
                valid.getTopic(), valid.getPartition(), valid.getOffset(), valid.getTimestamp(),
                valid.getKey(), valid.getValue(), headers);

        FlowEventParseFunction.ParseResult result = FlowEventParseFunction.parseRecord(forged);

        assertNull(result.input);
        assertEquals("ENVELOPE_MISMATCH", result.failure.errorCode());
        assertTrue(result.failure.toJson().contains("tenant_id"));
    }

    private static FlowEvent.Builder validFlowBuilder() {
        return FlowEvent.newBuilder()
                .setHeader(validHeader("evt-feature"))
                .setFlowId("flow-1")
                .setCommunityId("1:abc")
                .setTsStart(1000L)
                .setTsEnd(1234L);
    }

    private static EventHeader validHeader(String eventId) {
        return EventHeader.newBuilder()
                .setTenantId("tenant-a")
                .setEventId(eventId)
                .setRunId("run-1")
                .setProbeId("probe-1")
                .setFeatureSetId("features-1")
                .setEventTs(1234L)
                .setIngestTs(1400L)
                .setKafkaTs(1500L)
                .setEventType("traffic.flow.v1")
                .setSchemaVersion("1")
                .setAggregateType("flow")
                .setAggregateId("flow-1")
                .setAggregateVersion(1L)
                .setOccurredAt(1234L)
                .setProducedAt(1400L)
                .setIdempotencyKey("flow-1:1")
                .setProducer("probe-agent")
                .build();
    }

    private static RawKafkaRecord record(String key, byte[] value) {
        FlowEvent flow;
        try {
            flow = FlowEvent.parseFrom(value);
        } catch (Exception error) {
            throw new IllegalArgumentException(error);
        }
        EventHeader header = flow.getHeader();
        return new RawKafkaRecord(
                "flow.events.v1",
                0,
                1L,
                1500L,
                key.getBytes(StandardCharsets.UTF_8),
                value,
                Map.ofEntries(
                        Map.entry("tenant_id", header.getTenantId()),
                        Map.entry("probe_id", header.getProbeId()),
                        Map.entry("event_id", header.getEventId()),
                        Map.entry("run_id", header.getRunId()),
                        Map.entry("feature_set_id", header.getFeatureSetId()),
                        Map.entry("community_id", flow.getCommunityId()),
                        Map.entry("content_type", "application/x-protobuf"),
                        Map.entry("proto_message_type", "traffic.v1.FlowEvent"),
                        Map.entry("proto_schema_version", "v1"),
                        Map.entry("proto_package", "traffic.v1"),
                        Map.entry("event_ts", String.valueOf(header.getEventTs())),
                        Map.entry("ingest_ts", String.valueOf(header.getIngestTs())),
                        Map.entry("kafka_ts", String.valueOf(header.getKafkaTs()))));
    }
}
