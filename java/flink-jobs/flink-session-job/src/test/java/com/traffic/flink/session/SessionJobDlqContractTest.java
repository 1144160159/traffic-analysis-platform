package com.traffic.flink.session;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.session.source.FlowLatenessFunction;
import com.traffic.flink.session.source.ValidatedFlowInput;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SessionJobDlqContractTest {

    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void allSessionFailureRoutesRequireCanonicalDlq() throws Exception {
        SessionJobConfig config = SessionJobConfig.fromArgs(new String[]{});
        assertEquals("dlq.v1", config.getInputDlqTopic());
        assertEquals("dlq.v1", config.getLateDataTopic());
        assertEquals("dlq.v1", config.getChDlqTopic());
        assertEquals("dlq.v1", config.getOsDlqTopic());

        assertThrows(IllegalArgumentException.class, () -> {
            try {
                SessionJobConfig.fromArgs(
                        new String[]{"--late.data.topic", "session.late.v1"});
            } catch (Exception e) {
                if (e instanceof IllegalArgumentException) {
                    throw (IllegalArgumentException) e;
                }
                throw new AssertionError("expected IllegalArgumentException, got " + e, e);
            }
        });
    }

    @Test
    void lateFlowPreservesAuthoritativeBrokerSourceTuple() throws Exception {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("event-1")
                .setTenantId("tenant-a")
                .build();
        FlowEvent flow = FlowEvent.newBuilder()
                .setHeader(header)
                .setCommunityId("community-a")
                .setTsEnd(1_000L)
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "flow.events.v1", 5, 17L, 1_100L,
                "tenant-a:community-a".getBytes(StandardCharsets.UTF_8),
                flow.toByteArray(), Map.of());
        CanonicalDlqMessage late = FlowLatenessFunction.lateFailure(
                new ValidatedFlowInput(source, flow), 2_000L, 100L);
        JsonNode json = JSON.readTree(late.toJson());

        assertEquals("tenant-a", json.get("tenant_id").asText());
        assertEquals("SUPER_LATE_EVENT", json.get("error_code").asText());
        assertEquals(5, json.get("original_partition").asInt());
        assertEquals(17L, json.get("original_offset").asLong());
        assertEquals("flow.events.v1:5:17", json.get("metadata").get("source_tuple").asText());
        assertEquals("flink-session-job", json.get("service_name").asText());
    }

    @Test
    void rejectsStateTtlThatCanExpireBeforeEventTimeTimer() {
        assertThrows(IllegalArgumentException.class, () -> {
            try {
                SessionJobConfig.fromArgs(
                        new String[]{"--state.ttl.ms", "1800000", "--active.timeout.ms", "1800000"});
            } catch (Exception e) {
                if (e instanceof IllegalArgumentException) {
                    throw (IllegalArgumentException) e;
                }
                throw new AssertionError("expected IllegalArgumentException, got " + e, e);
            }
        });
    }

    @Test
    void rejectsDisabledStateTtlInProcessMode() {
        assertThrows(IllegalArgumentException.class, () -> {
            try {
                SessionJobConfig.fromArgs(
                        new String[]{"--state.ttl.enabled", "false", "--session.mode", "process"});
            } catch (Exception e) {
                if (e instanceof IllegalArgumentException) {
                    throw (IllegalArgumentException) e;
                }
                throw new AssertionError("expected IllegalArgumentException, got " + e, e);
            }
        });
    }

    @Test
    void acceptsWindowModeWithoutKeyedStateTtl() throws Exception {
        SessionJobConfig config = SessionJobConfig.fromArgs(
                new String[]{"--state.ttl.enabled", "false", "--session.mode", "window"});
        assertTrue(config.isWindowMode());
    }
}
