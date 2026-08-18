package com.traffic.flink.feature.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SessionEventParseFunctionTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void acceptsValidSessionAndKeepsSourceTuple() {
        RawKafkaRecord source = source(validSession().toByteArray());
        SessionEventParseFunction.ParseResult result = SessionEventParseFunction.parse(source);

        assertNotNull(result.input);
        assertNull(result.failure);
        assertEquals("session-1", result.input.getSession().getSessionId());
        assertEquals(3, result.input.getSource().getPartition());
        assertEquals(42L, result.input.getSource().getOffset());
    }

    @Test
    void invalidProtobufProducesCanonicalSourceTraceableDlq() throws Exception {
        SessionEventParseFunction.ParseResult result =
                SessionEventParseFunction.parse(source(new byte[]{(byte) 0xff, 0x01}));

        assertNull(result.input);
        assertEquals("BAD_SCHEMA", result.failure.errorCode());
        JsonNode json = JSON.readTree(result.failure.toJson());
        assertEquals("session.events.v1", json.get("original_topic").asText());
        assertEquals(3, json.get("original_partition").asInt());
        assertEquals(42L, json.get("original_offset").asLong());
        assertEquals("tenant-from-header", json.get("tenant_id").asText());
        assertEquals("traffic.v1.SessionEvent", json.get("proto_message_type").asText());
        assertTrue(json.get("metadata").get("source_tuple").asText().endsWith(":3:42"));
        assertTrue(json.get("replay_policy").get("require_manual_ack").asBoolean());
    }

    @Test
    void invalidEventTimeRangeIsBadTimestamp() {
        SessionEvent invalid = validSession().toBuilder()
                .setTsStart(2_000)
                .setTsEnd(1_000)
                .build();

        SessionEventParseFunction.ParseResult result =
                SessionEventParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.input);
        assertEquals("BAD_TIMESTAMP", result.failure.errorCode());
    }

    @Test
    void missingTenantIsValidationError() {
        SessionEvent invalid = validSession().toBuilder()
                .setHeader(validSession().getHeader().toBuilder().clearTenantId())
                .build();

        SessionEventParseFunction.ParseResult result =
                SessionEventParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.input);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    @Test
    void malformedFeatureObservationIsRejectedBeforeFeatureCalculation() {
        TrafficFeatureObservation malformed = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("session-feature-merge/v1")
                .addSignedPacketLengths(100)
                .addSignedPacketLengths(-80)
                .addPacketEventTimeUs(2_000_000L)
                .addPacketEventTimeUs(1_000_000L)
                .build();
        SessionEvent invalid = validSession().toBuilder()
                .setFeatureObservation(malformed)
                .build();

        SessionEventParseFunction.ParseResult result =
                SessionEventParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.input);
        assertEquals("BAD_TIMESTAMP", result.failure.errorCode());
    }

    @Test
    void tlsFieldsWithoutObservedTlsAreRejected() {
        TrafficFeatureObservation malformed = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("session-feature-merge/v1")
                .setTransportSecurity(TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED)
                .setJa3("0123456789abcdef0123456789abcdef")
                .build();
        SessionEvent invalid = validSession().toBuilder()
                .setFeatureObservation(malformed)
                .build();

        SessionEventParseFunction.ParseResult result =
                SessionEventParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.input);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    private static SessionEvent validSession() {
        return SessionEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId("session-event-1")
                        .setTenantId("tenant-1")
                        .setRunId("run-1")
                        .setEventTs(2_000)
                        .setOccurredAt(2_000))
                .setSessionId("session-1")
                .setCommunityId("1:abc")
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("192.0.2.1")
                        .setDstIp("198.51.100.1")
                        .setSrcPort(12345)
                        .setDstPort(443)
                        .setProtocol(6))
                .setTsStart(1_000)
                .setTsEnd(2_000)
                .setEventTimeStartMs(1_000)
                .setEventTimeEndMs(2_000)
                .build();
    }

    private static RawKafkaRecord source(byte[] payload) {
        return new RawKafkaRecord(
                "session.events.v1",
                3,
                42L,
                2_100L,
                "tenant-1:1:abc".getBytes(StandardCharsets.UTF_8),
                payload,
                Map.of("tenant_id", "tenant-from-header"));
    }
}
