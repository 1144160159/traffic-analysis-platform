package com.traffic.flink.feature.processor;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.feature.config.TenantConfig;
import com.traffic.flink.feature.source.ValidatedSessionInput;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.SessionEvent;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FeatureProcessFunctionV3Test {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    @DisplayName("默认租户配置不应在无显式配置时触发降级丢弃")
    void defaultTenantConfigKeepsLiveTrafficEnabled() {
        TenantConfig config = new FeatureProcessFunctionV3().createDefaultTenantConfig();

        assertEquals("default", config.getTenantId());
        assertEquals(10, config.getPriority());
        assertFalse(config.isEnableDegradation());
        assertTrue(config.isEnableL2());
        assertEquals(1.0f, config.getSamplingRate(), 0.0001f);
        assertEquals(-1, config.getMaxEventsPerSecond());
    }

    @Test
    void latenessBoundaryIsDeterministicAndOverflowSafe() {
        assertFalse(FeatureProcessFunctionV3.isTooLate(1_000L, Long.MIN_VALUE, 0L));
        assertFalse(FeatureProcessFunctionV3.isTooLate(1_000L, 1_009L, 10L));
        assertFalse(FeatureProcessFunctionV3.isTooLate(1_000L, 1_010L, 10L));
        assertTrue(FeatureProcessFunctionV3.isTooLate(999L, 1_010L, 10L));
        assertFalse(FeatureProcessFunctionV3.isTooLate(
                Long.MAX_VALUE - 1L, Long.MAX_VALUE - 1L, 10L));
        assertFalse(FeatureProcessFunctionV3.isTooLate(
                Long.MAX_VALUE - 1L, Long.MAX_VALUE, 10L));
    }

    @Test
    void superLateDlqRetainsAuthoritativeSourceTuple() throws Exception {
        SessionEvent session = SessionEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("session-event-1")
                        .setTraceId("trace-1")
                        .setRunId("run-1"))
                .setSessionId("session-1")
                .setCommunityId("1:abc")
                .setTsEnd(1_000L)
                .setEventTimeEndMs(1_000L)
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "session.events.v1", 7, 81L, 1_100L,
                "tenant-1:1:abc".getBytes(StandardCharsets.UTF_8),
                session.toByteArray(), Map.of());

        CanonicalDlqMessage failure = FeatureProcessFunctionV3.lateDataFailure(
                new ValidatedSessionInput(source, session), 2_000L, 100L);
        JsonNode json = JSON.readTree(failure.toJson());

        assertEquals("SUPER_LATE_EVENT", json.get("error_code").asText());
        assertEquals(7, json.get("original_partition").asInt());
        assertEquals(81L, json.get("original_offset").asLong());
        assertEquals("session.events.v1:7:81",
                json.get("metadata").get("source_tuple").asText());
    }
}
