package com.traffic.flink.alert.router;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.streaming.util.BroadcastOperatorTestHarness;
import org.apache.flink.streaming.util.ProcessFunctionTestHarnesses;
import org.junit.jupiter.api.Test;

import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;

import java.util.HashMap;
import java.util.List;
import java.util.Queue;
import java.util.stream.Collectors;
import java.util.stream.StreamSupport;

import static org.assertj.core.api.Assertions.assertThat;

class RunRouterProcessFunctionTest {

    private static final long WINDOW_START = 1700000000000L;
    private static final long WINDOW_END = 1700000600000L;

    private static RunSubscriptionRecord sub(String tenant, String runId, long start, long end, String state, long revision) {
        return RunSubscriptionRecord.of(tenant, runId, revision, state, "spec-1", start, end, null);
    }

    private static FlowEvent flow(String tenant, long tsEnd, String communityId) {
        return FlowEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId(tenant)
                        .setEventTs(tsEnd)
                        .setProbeId("probe-agent")
                        .build())
                .setFlowId("flow-1")
                .setCommunityId(communityId)
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("10.0.0.1").setDstIp("10.0.0.2")
                        .setSrcPort(12345).setDstPort(443).setProtocol(6).build())
                .setTsStart(tsEnd - 1000)
                .setTsEnd(tsEnd)
                .build();
    }

    @SuppressWarnings("unchecked")
    private static String unwrap(Object o) {
        return ((StreamRecord<String>) o).getValue();
    }

    private static RawKafkaRecord record(String topic, byte[] value) {
        return new RawKafkaRecord(topic, 0, 0L, 0L, null, value, new HashMap<>());
    }

    private static RunRouterProcessFunction function() {
        return new RunRouterProcessFunction("flow.events.v1", "test-group");
    }

    private static BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> harness(
            RunRouterProcessFunction fn) throws Exception {
        return ProcessFunctionTestHarnesses.forBroadcastProcessFunction(
                fn, RunRouterProcessFunction.SUBSCRIPTION_STATE, new MapStateDescriptor<>(
                        "unused-keyed-state", String.class, String.class));
    }

    @Test
    void routesFlowInsideWindowForActiveSubscription() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 1), 1L);
            h.processElement(record("flow.events.v1", flow("default", WINDOW_START + 10_000, "1:cafebabe").toByteArray()), 1L);
            Queue<Object> out = h.getOutput();
            assertThat(out).hasSize(1);
            String envelope = unwrap(out.poll());
            assertThat(envelope).contains("\"run_id\":\"run-1\"");
            assertThat(envelope).contains("\"tenant_id\":\"default\"");
            assertThat(envelope).contains("\"schema_version\":\"1\"");
            assertThat(envelope).contains("\"community_id\":\"1:cafebabe\"");
        }
    }

    @Test
    void doesNotRouteOutsideWindow() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 1), 1L);
            h.processElement(record("flow.events.v1", flow("default", WINDOW_END + 1, "1:cafebabe").toByteArray()), 1L);
            assertThat(h.getOutput()).isEmpty();
        }
    }

    @Test
    void doesNotRouteCrossTenant() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 1), 1L);
            h.processElement(record("flow.events.v1", flow("other-tenant", WINDOW_START + 10_000, "1:cafebabe").toByteArray()), 1L);
            assertThat(h.getOutput()).isEmpty();
        }
    }

    @Test
    void cancelRemovesSubscription() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 1), 1L);
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "CANCELLED", 2), 2L);
            h.processElement(record("flow.events.v1", flow("default", WINDOW_START + 10_000, "1:cafebabe").toByteArray()), 1L);
            assertThat(h.getOutput()).isEmpty();
        }
    }

    @Test
    void staleRevisionDoesNotOverwriteActive() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 3), 1L);
            h.processBroadcastElement(sub("default", "run-1", 0, 1, "ACTIVE", 2), 2L); // 旧 revision 回退
            h.processElement(record("flow.events.v1", flow("default", WINDOW_START + 10_000, "1:cafebabe").toByteArray()), 1L);
            Queue<Object> out = h.getOutput();
            assertThat(out).hasSize(1); // 仍按 revision 3 的窗口派生
            String envelope = unwrap(out.poll());
            assertThat(envelope).contains("\"run_id\":\"run-1\"");
        }
    }

    @Test
    void parseFailureGoesToDlqSideOutput() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processElement(record("flow.events.v1", new byte[]{1, 2, 3}), 1L);
            List<Object> dlq = StreamSupport.stream(
                    h.getSideOutput(RunRouterProcessFunction.FLOW_PARSE_DLQ_TAG).spliterator(), false)
                    .collect(Collectors.toList());
            assertThat(dlq).hasSize(1);
            assertThat(h.getOutput()).isEmpty();
        }
    }

    @Test
    void wrongSourceTopicIgnored() throws Exception {
        RunRouterProcessFunction fn = function();
        try (BroadcastOperatorTestHarness<RawKafkaRecord, RunSubscriptionRecord, String> h = harness(fn)) {
            h.open();
            h.processBroadcastElement(sub("default", "run-1", WINDOW_START, WINDOW_END, "ACTIVE", 1), 1L);
            h.processElement(record("session.events.v1", flow("default", WINDOW_START + 10_000, "1:cafebabe").toByteArray()), 1L);
            assertThat(h.getOutput()).isEmpty();
        }
    }
}
