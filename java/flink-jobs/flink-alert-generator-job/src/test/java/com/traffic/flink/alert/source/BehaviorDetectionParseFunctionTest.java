package com.traffic.flink.alert.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class BehaviorDetectionParseFunctionTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void acceptsCanonicalDetectionAndPreservesObservedTuple() {
        BehaviorDetectionParseFunction.ParseResult result =
                BehaviorDetectionParseFunction.parse(source(validDetection().toByteArray()));

        assertNotNull(result.detection);
        assertNull(result.failure);
        assertEquals("192.0.2.10", result.detection.getTuple().getSrcIp());
        assertEquals("source-evidence-1", result.detection.getEvidenceIds(0));
    }

    @Test
    void malformedProtobufProducesSourceTraceableCanonicalDlq() throws Exception {
        BehaviorDetectionParseFunction.ParseResult result =
                BehaviorDetectionParseFunction.parse(source(new byte[]{(byte) 0xff, 0x01}));

        assertNull(result.detection);
        assertEquals("BAD_SCHEMA", result.failure.errorCode());
        JsonNode json = JSON.readTree(result.failure.toJson());
        assertEquals("detections.behavior.v1", json.get("original_topic").asText());
        assertEquals(4, json.get("original_partition").asInt());
        assertEquals(99L, json.get("original_offset").asLong());
        assertEquals("tenant-from-kafka", json.get("tenant_id").asText());
        assertEquals("flink-alert-generator-job", json.get("service_name").asText());
        assertEquals("traffic.v1.DetectionBehavior", json.get("proto_message_type").asText());
        assertTrue(json.get("replay_policy").get("require_manual_ack").asBoolean());
    }

    @Test
    void missingTenantIsValidationFailure() {
        DetectionBehavior invalid = validDetection().toBuilder()
                .setHeader(validDetection().getHeader().toBuilder().clearTenantId())
                .build();

        BehaviorDetectionParseFunction.ParseResult result =
                BehaviorDetectionParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.detection);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    @Test
    void missingTupleIsValidationFailureInsteadOfFabricatedIdentity() {
        DetectionBehavior invalid = validDetection().toBuilder().clearTuple().build();

        BehaviorDetectionParseFunction.ParseResult result =
                BehaviorDetectionParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.detection);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
        assertTrue(result.failure.toJson().contains("source tuple"));
    }

    private static DetectionBehavior validDetection() {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("detection-1")
                .setTenantId("tenant-1")
                .setRunId("run-1")
                .setEventTs(2_000L)
                .setIngestTs(2_100L)
                .setProbeId("probe-1")
                .setFeatureSetId("feature-set-1")
                .setEventType("traffic.detection.behavior.v1")
                .setSchemaVersion("1")
                .setAggregateType("detection")
                .setAggregateId("flow-1")
                .setAggregateVersion(9)
                .setOccurredAt(2_000L)
                .setProducedAt(2_200L)
                .setTraceId("trace-1")
                .setCausationId("feature-1")
                .setCorrelationId("correlation-1")
                .setIdempotencyKey("detection-1")
                .setProducer("flink-rule-job")
                .build();
        return DetectionBehavior.newBuilder()
                .setHeader(header)
                .setModelVersion("rule-engine-v1")
                .setCommunityId("1:community")
                .setObjectType("flow")
                .setObjectId("flow-1")
                .setTs(2_000L)
                .addLabels("port_scan")
                .addScores(0.91f)
                .setTopLabel("port_scan")
                .setTopScore(0.91f)
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("192.0.2.10")
                        .setDstIp("198.51.100.20")
                        .setSrcPort(54321)
                        .setDstPort(443)
                        .setProtocol(6))
                .addEvidenceIds("source-evidence-1")
                .build();
    }

    private static RawKafkaRecord source(byte[] payload) {
        return new RawKafkaRecord(
                "detections.behavior.v1", 4, 99L, 2_300L,
                "tenant-1:community".getBytes(StandardCharsets.UTF_8),
                payload, Map.of("tenant_id", "tenant-from-kafka"));
    }
}
