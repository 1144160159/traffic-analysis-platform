package com.traffic.flink.session.source;

import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FlowEventParseFunctionTest {

    @Test
    void parsesValidFlowEvent() {
        FlowEvent flow = FlowEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-a")
                        .setEventId("evt-1")
                        .setRunId("run-1")
                        .build())
                .setCommunityId("1:abc")
                .setTsEnd(1234L)
                .build();

        RawKafkaRecord record = record("tenant-a:1:abc", flow.toByteArray());

        FlowEventParseFunction.ParseResult result = FlowEventParseFunction.parseRecord(record);

        assertNotNull(result.flowEvent);
        assertNull(result.deadLetter);
        assertEquals("tenant-a", result.flowEvent.getHeader().getTenantId());
        assertEquals("1:abc", result.flowEvent.getCommunityId());
    }

    @Test
    void invalidProtobufBytesBecomeDeadLetter() {
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

        assertNull(result.flowEvent);
        DeadLetter dlq = result.deadLetter;
        assertNotNull(dlq);
        assertEquals("flink-session-parse:flow.events.v1:2:33", dlq.getEventId());
        assertEquals("tenant-a", dlq.getTenantId());
        assertEquals("flow.events.v1", dlq.getSourceTopic());
        assertEquals("tenant-a:bad", dlq.getSourceKey());
        assertTrue(dlq.getErrorMsg().contains("invalid FlowEvent protobuf"));
        assertArrayEquals(payload, Base64.getDecoder().decode(dlq.getRawPayload()));
        assertEquals(4567L, dlq.getCreatedAt());
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

        assertNull(result.flowEvent);
        assertNotNull(result.deadLetter);
        assertEquals("tenant-from-key", result.deadLetter.getTenantId());
        assertEquals("missing FlowEvent tenant_id", result.deadLetter.getErrorMsg());
    }

    private static RawKafkaRecord record(String key, byte[] value) {
        return new RawKafkaRecord(
                "flow.events.v1",
                0,
                1L,
                1234L,
                key.getBytes(StandardCharsets.UTF_8),
                value,
                Map.of());
    }
}
