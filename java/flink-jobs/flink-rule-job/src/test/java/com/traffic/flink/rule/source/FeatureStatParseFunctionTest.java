package com.traffic.flink.rule.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FeatureStatParseFunctionTest {

    @Test
    void acceptsCanonicalFeatureAndRetainsTuple() {
        FeatureStatParseFunction.ParseResult result =
                FeatureStatParseFunction.parse(source(validFeature().toByteArray()));

        assertNotNull(result.feature);
        assertNull(result.failure);
        assertEquals("192.0.2.10", result.feature.getTuple().getSrcIp());
    }

    @Test
    void malformedFeatureUsesCanonicalSourceTraceableDlq() throws Exception {
        FeatureStatParseFunction.ParseResult result =
                FeatureStatParseFunction.parse(source(new byte[]{(byte) 0xff, 0x01}));

        assertNull(result.feature);
        assertEquals("BAD_SCHEMA", result.failure.errorCode());
        JsonNode json = new ObjectMapper().readTree(result.failure.toJson());
        assertEquals("feature.stat.v1", json.get("original_topic").asText());
        assertEquals(1, json.get("original_partition").asInt());
        assertEquals(27L, json.get("original_offset").asLong());
        assertEquals("flink-rule-job", json.get("service_name").asText());
        assertEquals("traffic.v1.FeatureStat", json.get("proto_message_type").asText());
    }

    @Test
    void missingTupleIsRejectedBeforeRuleMatching() {
        FeatureStat invalid = validFeature().toBuilder().clearTuple().build();

        FeatureStatParseFunction.ParseResult result =
                FeatureStatParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.feature);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
        assertTrue(result.failure.toJson().contains("source tuple"));
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
                "feature.stat.v1", 1, 27L, 2_200L,
                "tenant-1:community".getBytes(StandardCharsets.UTF_8),
                payload, Map.of("tenant_id", "tenant-from-kafka"));
    }
}
