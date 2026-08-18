package com.traffic.flink.behavior.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class BehaviorFeatureInputTest {

    @Test
    void acceptsCanonicalFeatureAndKeepsKafkaCoordinates() {
        BehaviorFeatureParseFunction.ParseResult result =
                BehaviorFeatureParseFunction.parse(source(validFeature().toByteArray()));

        assertNotNull(result.input);
        assertNull(result.failure);
        assertEquals(31L, result.input.getSource().getOffset());
        assertEquals("192.0.2.10", result.input.getFeature().getTuple().getSrcIp());
    }

    @Test
    void invalidProtobufUsesCanonicalDlq() throws Exception {
        BehaviorFeatureParseFunction.ParseResult result =
                BehaviorFeatureParseFunction.parse(source(new byte[]{(byte) 0xff, 0x01}));

        assertNull(result.input);
        assertEquals("BAD_SCHEMA", result.failure.errorCode());
        JsonNode json = new ObjectMapper().readTree(result.failure.toJson());
        assertEquals("flink-behavior-job", json.get("service_name").asText());
        assertEquals("feature.stat.v1", json.get("original_topic").asText());
        assertEquals(31L, json.get("original_offset").asLong());
    }

    @Test
    void tenantAndTupleAreRequired() {
        FeatureStat missingTenant = validFeature().toBuilder()
                .setHeader(validFeature().getHeader().toBuilder().clearTenantId())
                .build();
        FeatureStat missingTuple = validFeature().toBuilder().clearTuple().build();

        assertEquals("VALIDATION_ERROR",
                BehaviorFeatureParseFunction.parse(source(missingTenant.toByteArray()))
                        .failure.errorCode());
        assertEquals("VALIDATION_ERROR",
                BehaviorFeatureParseFunction.parse(source(missingTuple.toByteArray()))
                        .failure.errorCode());
    }

    @Test
    void latenessUsesEventTimeAndProducesReplayableFailure() throws Exception {
        BehaviorFeatureParseFunction.ParseResult parsed =
                BehaviorFeatureParseFunction.parse(source(validFeature().toByteArray()));
        assertTrue(BehaviorFeatureLatenessFunction.isLate(2_000L, 3_001L, 1_000L));
        assertFalse(BehaviorFeatureLatenessFunction.isLate(2_000L, 3_000L, 1_000L));

        CanonicalDlqMessage failure = BehaviorFeatureLatenessFunction.lateFailure(
                parsed.input, 3_001L, 1_000L);
        JsonNode json = new ObjectMapper().readTree(failure.toJson());
        assertEquals("LATE_EVENT", json.get("error_code").asText());
        assertEquals("tenant-1", json.get("tenant_id").asText());
        assertTrue(json.get("error_message").asText().contains("watermark=3001"));
    }

    private static FeatureStat validFeature() {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("feature-1")
                .setTenantId("tenant-1")
                .setEventTs(2_000L)
                .setEventType("traffic.feature.stat.v1")
                .setSchemaVersion("1")
                .setAggregateType("session")
                .setAggregateId("session-1")
                .setAggregateVersion(1)
                .setOccurredAt(2_000L)
                .setProducedAt(2_100L)
                .setTraceId("trace-1")
                .setCausationId("session-event-1")
                .setCorrelationId("1:community")
                .setIdempotencyKey("feature-1")
                .setProducer("flink-feature-job")
                .build();
        return FeatureStat.newBuilder()
                .setHeader(header)
                .setSchemaVersion("1")
                .setObjectType("session")
                .setObjectId("session-1")
                .setCommunityId("1:community")
                .setTs(2_000L)
                .setProtocol(6)
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("192.0.2.10")
                        .setDstIp("198.51.100.20")
                        .setSrcPort(54321)
                        .setDstPort(443)
                        .setProtocol(6))
                .setAvailability(FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE)
                .build();
    }

    private static RawKafkaRecord source(byte[] payload) {
        return new RawKafkaRecord(
                "feature.stat.v1", 2, 31L, 2_200L,
                "tenant-1:community".getBytes(StandardCharsets.UTF_8),
                payload, Map.of("tenant_id", "tenant-from-kafka"));
    }
}
