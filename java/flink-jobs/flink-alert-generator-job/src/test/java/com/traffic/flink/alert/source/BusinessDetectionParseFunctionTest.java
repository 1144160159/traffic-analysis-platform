package com.traffic.flink.alert.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.DetectionBusiness;
import com.traffic.proto.traffic.v1.EventHeader;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;

class BusinessDetectionParseFunctionTest {

    @Test
    void acceptsLegacyBusinessDetectionContractDuringDualRead() {
        BusinessDetectionParseFunction.ParseResult result =
                BusinessDetectionParseFunction.parse(source(validDetection().toByteArray()));

        assertNotNull(result.detection);
        assertNull(result.failure);
        assertEquals("rule-v3", result.detection.getRuleVersion());
    }

    @Test
    void missingBusinessIdentityUsesCanonicalDlq() {
        DetectionBusiness invalid = validDetection().toBuilder().clearLabel().build();

        BusinessDetectionParseFunction.ParseResult result =
                BusinessDetectionParseFunction.parse(source(invalid.toByteArray()));

        assertNull(result.detection);
        assertEquals("VALIDATION_ERROR", result.failure.errorCode());
    }

    private static DetectionBusiness validDetection() {
        return DetectionBusiness.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId("business-1")
                        .setTenantId("tenant-1")
                        .setEventType("traffic.detection.business.v1")
                        .setSchemaVersion("1")
                        .setAggregateType("detection")
                        .setAggregateId("session-1")
                        .setAggregateVersion(1)
                        .setTraceId("trace-1")
                        .setCausationId("event-1")
                        .setCorrelationId("corr-1")
                        .setIdempotencyKey("business-1")
                        .setProducer("flink-rule-job")
                        .setOccurredAt(3_000L)
                        .setProducedAt(3_100L)
                        .setEventTs(3_000L))
                .setModelVersion("legacy")
                .setRuleVersion("rule-v3")
                .setTs(3_000L)
                .setCommunityId("1:community")
                .setSessionId("session-1")
                .setDetectionType("rule")
                .setLabel("blocked_destination")
                .setScore(1.0f)
                .build();
    }

    private static RawKafkaRecord source(byte[] payload) {
        return new RawKafkaRecord(
                "detections.business.v1", 2, 7L, 3_100L,
                "tenant-1:community".getBytes(StandardCharsets.UTF_8),
                payload, Map.of());
    }
}
