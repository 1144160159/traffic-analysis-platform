package com.traffic.flink.session.processor;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.source.RichParallelSourceFunction;
import org.apache.flink.streaming.api.watermark.Watermark;
import org.apache.flink.util.OutputTag;

import org.junit.jupiter.api.*;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

@TestInstance(TestInstance.Lifecycle.PER_CLASS)
class SessionizeProcessFunctionK8sTest {

    private static final Logger LOG = LoggerFactory.getLogger(SessionizeProcessFunctionK8sTest.class);
    private static final long IDLE_TIMEOUT_MS = 5_000L;
    private static final long ACTIVE_TIMEOUT_MS = 30_000L;
    private static final long STATE_TTL_MS = 60_000L;
    private static final OutputTag<FlowEvent> LATE_DATA_TAG =
            new OutputTag<FlowEvent>("late-flow-events") {};

    @Test @DisplayName("Idle Timeout")
    void testIdleTimeoutTriggersSessionOutput() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        flows.add(createFlow("tenant1", "1:abc123", 1000L, 1100L));
        List<SessionEvent> results = runPipeline(flows, 1100L + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty(), "Should emit session via idle timeout");
    }

    @Test @DisplayName("Flow 聚合")
    void testMultipleFlowsAggregation() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        long expectedPackets = 0;
        for (int i = 0; i < 5; i++) {
            long ts = 1000L + i * 1000L;
            int pf = 100 + i * 10, pb = 50 + i * 5;
            expectedPackets += pf + pb;
            flows.add(createFlowWithStats("tenant1", "1:abc123", ts, ts + 100, pf, pb, 1000L, 500L));
        }
        List<SessionEvent> results = runPipeline(flows, flows.get(4).getTsEnd() + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty());
        assertEquals(expectedPackets, results.get(0).getPacketsTotal());
    }

    @Test @DisplayName("多租户隔离")
    void testDifferentTenantsProduceSeparateSessions() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        flows.add(createFlow("tenantA", "1:abc123", 1000L, 1100L));
        flows.add(createFlow("tenantB", "1:abc123", 1000L, 1100L));
        List<SessionEvent> results = runPipeline(flows, 1100L + IDLE_TIMEOUT_MS + 500);
        assertEquals(2, results.size());
    }

    @Test @DisplayName("Late Data")
    void testLateDataRouting() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        flows.add(createFlow("tenant1", "1:abc123", 5000L, 5100L));
        // Late flow at ts 1000 should be filtered
        flows.add(createFlow("tenant1", "1:late", 1000L, 1100L));
        List<SessionEvent> results = runPipeline(flows, 5100L + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty());
    }

    @Test @DisplayName("TCP 标志位")
    void testTcpFlagsAggregation() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        flows.add(FlowEvent.newBuilder()
                .setHeader(header("tenant1")).setFlowId("f1").setCommunityId("1:abc")
                .setTuple(tuple("192.168.1.1", "10.0.0.1", 12345, 80, 6))
                .setTsStart(1000L).setTsEnd(1100L)
                .setTcpFlagsFwd(0x02).setTcpFlagsBwd(0x12)
                .setPacketsFwd(1).setPacketsBwd(1).build());
        flows.add(FlowEvent.newBuilder()
                .setHeader(header("tenant1")).setFlowId("f2").setCommunityId("1:abc")
                .setTuple(tuple("192.168.1.1", "10.0.0.1", 12345, 80, 6))
                .setTsStart(2000L).setTsEnd(2100L)
                .setTcpFlagsFwd(0x01).setTcpFlagsBwd(0x11)
                .setPacketsFwd(1).setPacketsBwd(1).build());
        List<SessionEvent> results = runPipeline(flows, 2100L + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty());
        assertTrue(results.get(0).getHasSyn());
        assertTrue(results.get(0).getHasFin());
    }

    @Test @DisplayName("客户端/服务端")
    void testClientServerDetermination() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        flows.add(createFlowWithStats("tenant1", "1:abc",
                "192.168.1.100", "10.0.0.80", 50000, 80, 6,
                1000L, 1100L, 10, 20, 10000L, 20000L));
        List<SessionEvent> results = runPipeline(flows, 1100L + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty());
        assertEquals("192.168.1.100", results.get(0).getClientIp());
        assertEquals("10.0.0.80", results.get(0).getServerIp());
    }

    @Test @DisplayName("Active Timeout 切分")
    void testActiveTimeoutForcesSessionSplit() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        long start = 1000L;
        for (int i = 0; i <= 5; i++) {
            long ts = start + i * (ACTIVE_TIMEOUT_MS / 5);
            flows.add(createFlow("tenant1", "1:abc", ts, ts + 100));
        }
        List<SessionEvent> results = runPipeline(flows, start + ACTIVE_TIMEOUT_MS + IDLE_TIMEOUT_MS + 500);
        assertFalse(results.isEmpty());
    }

    @Test @DisplayName("Idle Timer 重置")
    void testIdleTimerReset() throws Exception {
        List<FlowEvent> flows = new ArrayList<>();
        long base = 1000L;
        flows.add(createFlow("tenant1", "1:abc", base, base + 100));
        flows.add(createFlow("tenant1", "1:abc", base + IDLE_TIMEOUT_MS / 2,
                base + IDLE_TIMEOUT_MS / 2 + 100));
        List<SessionEvent> results = runPipeline(flows,
                base + IDLE_TIMEOUT_MS / 2 + 100 + IDLE_TIMEOUT_MS + 500);
        assertEquals(1, results.size(), "Only 1 session: idle timer reset by second flow");
    }

    // ==================== Static Helpers (must be static for Flink serialization) ====================

    private List<SessionEvent> runPipeline(List<FlowEvent> flows, long finalWatermark) throws Exception {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.createLocalEnvironment();
        env.setParallelism(1);
        env.getConfig().setAutoWatermarkInterval(100);

        DataStream<FlowEvent> input = env.addSource(new FlowEventSource(flows, finalWatermark))
                .returns(TypeInformation.of(FlowEvent.class))
                .assignTimestampsAndWatermarks(
                        WatermarkStrategy.<FlowEvent>forMonotonousTimestamps()
                                .withTimestampAssigner((f, ts) -> f.getTsEnd()));

        DataStream<SessionEvent> sessions = input
                .keyBy(f -> {
                    String tid = f.getHeader() != null ? f.getHeader().getTenantId() : "x";
                    String cid = f.getCommunityId() != null ? f.getCommunityId() : "x";
                    return tid + "|" + cid;
                })
                .process(new SessionizeProcessFunction(
                        IDLE_TIMEOUT_MS, ACTIVE_TIMEOUT_MS, STATE_TTL_MS, LATE_DATA_TAG));

        List<SessionEvent> results = new ArrayList<>();
        sessions.executeAndCollect().forEachRemaining(results::add);
        return results;
    }

    private static EventHeader header(String tenantId) {
        return EventHeader.newBuilder().setTenantId(tenantId).setRunId("rt")
                .setEventId("e-" + System.nanoTime()).build();
    }

    private static FiveTuple tuple(String sip, String dip, int sp, int dp, int proto) {
        return FiveTuple.newBuilder().setSrcIp(sip).setDstIp(dip)
                .setSrcPort(sp).setDstPort(dp).setProtocol(proto).build();
    }

    private static FlowEvent createFlow(String tid, String cid, long ts, long te) {
        return FlowEvent.newBuilder().setHeader(header(tid)).setFlowId("f-" + System.nanoTime())
                .setCommunityId(cid).setTuple(tuple("192.168.1.1", "10.0.0.1", 12345, 80, 6))
                .setTsStart(ts).setTsEnd(te).setPacketsFwd(100).setPacketsBwd(50)
                .setBytesFwd(1000).setBytesBwd(500).build();
    }

    private static FlowEvent createFlowWithStats(String tid, String cid,
            String sip, String dip, int sp, int dp, int proto,
            long ts, long te, int pf, int pb, long bf, long bb) {
        return FlowEvent.newBuilder().setHeader(header(tid)).setFlowId("f-" + System.nanoTime())
                .setCommunityId(cid).setTuple(tuple(sip, dip, sp, dp, proto))
                .setTsStart(ts).setTsEnd(te).setPacketsFwd(pf).setPacketsBwd(pb)
                .setBytesFwd(bf).setBytesBwd(bb).build();
    }

    private static FlowEvent createFlowWithStats(String tid, String cid,
            long ts, long te, int pf, int pb, long bf, long bb) {
        return createFlowWithStats(tid, cid, "192.168.1.1", "10.0.0.1", 12345, 80, 6,
                ts, te, pf, pb, bf, bb);
    }

    /**
     * Serializable SourceFunction: emits FlowEvents then FINAL watermark.
     */
    public static class FlowEventSource extends RichParallelSourceFunction<FlowEvent> {
        private static final long serialVersionUID = 1L;
        private final List<FlowEvent> events;
        private final long finalWatermarkTs;

        public FlowEventSource(List<FlowEvent> events, long finalWatermarkTs) {
            this.events = events;
            this.finalWatermarkTs = finalWatermarkTs;
        }

        @Override
        public void run(SourceContext<FlowEvent> ctx) throws Exception {
            long maxTs = 0;
            for (FlowEvent e : events) {
                synchronized (ctx.getCheckpointLock()) {
                    ctx.collectWithTimestamp(e, e.getTsEnd());
                }
                maxTs = Math.max(maxTs, e.getTsEnd());
            }
            synchronized (ctx.getCheckpointLock()) {
                ctx.emitWatermark(new Watermark(Math.max(finalWatermarkTs, maxTs + 1)));
            }
            Thread.sleep(200);
        }

        @Override
        public void cancel() {}
    }
}
