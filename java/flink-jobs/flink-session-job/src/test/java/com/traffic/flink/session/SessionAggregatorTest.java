package com.traffic.flink.session;

import com.traffic.flink.session.aggregator.SessionAccumulator;
import com.traffic.flink.session.aggregator.SessionAggregator;
import com.traffic.proto.traffic.v1.*;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.DisplayName;

import static org.junit.jupiter.api.Assertions.*;

/**
 * SessionAggregator 单元测试（修复版）
 * 
 * 修复要点：
 * 1. ✅ 修复 end_reason 大小写断言（"rst" → "RST"）
 * 2. ✅ 新增确定性 event_id/session_id 测试
 * 3. ✅ 新增租户隔离测试
 * 4. ✅ 新增 client/server 映射测试
 * 5. ✅ 新增二阶矩统计验证测试
 */
class SessionAggregatorTest {

    private SessionAggregator aggregator;

    @BeforeEach
    void setUp() {
        aggregator = new SessionAggregator();
    }

    @Test
    @DisplayName("创建空累加器")
    void testCreateAccumulator() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        assertNotNull(acc);
        assertEquals(Long.MAX_VALUE, acc.tsStart);
        assertEquals(0, acc.tsEnd);
        assertEquals(0, acc.packetsFwd);
        assertEquals(0, acc.bytesFwd);
        assertEquals(0, acc.flowCount);
    }

    @Test
    @DisplayName("添加单个 Flow 到累加器")
    void testAddSingleFlow() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        acc = aggregator.add(flow, acc);
        
        assertEquals(1000L, acc.tsStart);
        assertEquals(2000L, acc.tsEnd);
        assertEquals(100, acc.packetsFwd);
        assertEquals(50, acc.packetsBwd);
        assertEquals(1000, acc.bytesFwd);
        assertEquals(500, acc.bytesBwd);
        assertEquals("1:abc123", acc.communityId);
        assertEquals("tenant1", acc.tenantId);
        assertEquals("run1", acc.runId);
        assertEquals(1, acc.flowCount);
    }

    @Test
    @DisplayName("添加多个 Flow 到累加器（累加验证）")
    void testAddMultipleFlows() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent flow1 = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        FlowEvent flow2 = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            2000L, 3000L,
            200, 100,
            2000, 1000
        );
        
        acc = aggregator.add(flow1, acc);
        acc = aggregator.add(flow2, acc);
        
        assertEquals(1000L, acc.tsStart);
        assertEquals(3000L, acc.tsEnd);
        assertEquals(300, acc.packetsFwd);
        assertEquals(150, acc.packetsBwd);
        assertEquals(3000, acc.bytesFwd);
        assertEquals(1500, acc.bytesBwd);
        assertEquals(2, acc.flowCount);
    }

    @Test
    @DisplayName("生成 SessionEvent（基础字段验证）")
    void testGetResult() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        acc = aggregator.add(flow, acc);
        
        SessionEvent session = aggregator.getResult(acc);
        
        assertNotNull(session);
        assertNotNull(session.getSessionId());
        assertFalse(session.getSessionId().isEmpty());
        assertEquals("1:abc123", session.getCommunityId());
        assertEquals("tenant1", session.getHeader().getTenantId());
        assertEquals("run1", session.getHeader().getRunId());
        assertEquals(1000L, session.getTsStart());
        assertEquals(2000L, session.getTsEnd());
        assertEquals(1000, session.getDurationMs());
        assertEquals(150, session.getPacketsTotal());
        assertEquals(1500, session.getBytesTotal());
    }

    @Test
    @DisplayName("生成 SessionEvent 时保留端到端链路时间戳")
    void testLatencyChainTimestamps() {
        SessionAccumulator acc = aggregator.createAccumulator();

        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );

        acc = aggregator.add(flow, acc);
        SessionEvent session = aggregator.getResult(acc);

        assertEquals(2000L, session.getHeader().getEventTs());
        assertEquals(2110L, session.getHeader().getIngestTs());
        assertEquals(2220L, session.getHeader().getKafkaTs());
        assertTrue(session.getHeader().getFlinkOutTs() >= session.getHeader().getKafkaTs());
    }

    @Test
    @DisplayName("✅ 修复：确定性 event_id 生成（重放一致性）")
    void testDeterministicEventId() {
        // 相同的输入应该产生相同的 event_id
        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        SessionAccumulator acc1 = aggregator.createAccumulator();
        acc1 = aggregator.add(flow, acc1);
        SessionEvent session1 = aggregator.getResult(acc1);
        
        SessionAccumulator acc2 = aggregator.createAccumulator();
        acc2 = aggregator.add(flow, acc2);
        SessionEvent session2 = aggregator.getResult(acc2);
        
        assertEquals(session1.getHeader().getEventId(), session2.getHeader().getEventId(),
                "相同输入应产生相同的确定性 event_id");
        assertEquals(session1.getSessionId(), session2.getSessionId(),
                "相同输入应产生相同的确定性 session_id");
    }

    @Test
    @DisplayName("✅ 新增：租户隔离测试（不同 tenant_id 应产生不同 event_id）")
    void testTenantIsolation() {
        FlowEvent flowTenant1 = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        FlowEvent flowTenant2 = createTestFlow(
            "tenant2", "run1", "1:abc123", // 相同 community_id 和 run_id
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        SessionAccumulator acc1 = aggregator.createAccumulator();
        acc1 = aggregator.add(flowTenant1, acc1);
        SessionEvent session1 = aggregator.getResult(acc1);
        
        SessionAccumulator acc2 = aggregator.createAccumulator();
        acc2 = aggregator.add(flowTenant2, acc2);
        SessionEvent session2 = aggregator.getResult(acc2);
        
        assertNotEquals(session1.getHeader().getEventId(), session2.getHeader().getEventId(),
                "不同 tenant_id 应产生不同的 event_id");
        assertNotEquals(session1.getSessionId(), session2.getSessionId(),
                "不同 tenant_id 应产生不同的 session_id");
    }

    @Test
    @DisplayName("✅ 新增：Client/Server 映射测试（知名端口判定）")
    void testClientServerMapping() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        // 目标端口 80（知名端口）应被识别为 server
        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            54321, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        acc = aggregator.add(flow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        // 验证 client/server 识别
        assertEquals("192.168.1.1", session.getClientIp(), "源 IP 应为 client");
        assertEquals("10.0.0.1", session.getServerIp(), "目标 IP 应为 server");
        assertEquals(54321, session.getClientPort());
        assertEquals(80, session.getServerPort());
        
        // ✅ 验证 bytes_up/down 映射（fwd = client→server = up）
        assertEquals(1000, session.getBytesFwd(), "bytesFwd 应映射为 bytes_up（client→server）");
        assertEquals(500, session.getBytesBwd(), "bytesBwd 应映射为 bytes_down（server→client）");
    }

    @Test
    @DisplayName("✅ 新增：Client/Server 反转测试（源端口为知名端口）")
    void testClientServerReverse() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        // 源端口 80（知名端口）应被识别为 server
        FlowEvent flow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "10.0.0.1", "192.168.1.1",
            80, 54321, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        acc = aggregator.add(flow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        // 验证 client/server 识别（应该反转）
        assertEquals("192.168.1.1", session.getClientIp(), "目标 IP 应为 client（反转）");
        assertEquals("10.0.0.1", session.getServerIp(), "源 IP 应为 server（反转）");
        assertEquals(54321, session.getClientPort());
        assertEquals(80, session.getServerPort());
        
        // ✅ 验证反转后的 bytes_up/down 映射（bwd = client→server = up）
        assertEquals(500, session.getBytesFwd(), "反转后 bytesBwd 应映射为 bytes_up");
        assertEquals(1000, session.getBytesBwd(), "反转后 bytesFwd 应映射为 bytes_down");
    }

    @Test
    @DisplayName("合并两个累加器（窗口合并）")
    void testMerge() {
        SessionAccumulator acc1 = aggregator.createAccumulator();
        SessionAccumulator acc2 = aggregator.createAccumulator();
        
        FlowEvent flow1 = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            1000L, 2000L,
            100, 50,
            1000, 500
        );
        
        FlowEvent flow2 = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "10.0.0.1",
            12345, 80, 6,
            3000L, 4000L,
            200, 100,
            2000, 1000
        );
        
        acc1 = aggregator.add(flow1, acc1);
        acc2 = aggregator.add(flow2, acc2);
        
        SessionAccumulator merged = aggregator.merge(acc1, acc2);
        
        assertEquals(1000L, merged.tsStart);
        assertEquals(4000L, merged.tsEnd);
        assertEquals(300, merged.packetsFwd);
        assertEquals(150, merged.packetsBwd);
        assertEquals(3000, merged.bytesFwd);
        assertEquals(1500, merged.bytesBwd);
        assertEquals(2, merged.flowCount);
    }

    @Test
    @DisplayName("TCP 标志位检测：SYN")
    void testTcpFlagsSyn() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent synFlow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setTcpFlagsFwd(0x02) // SYN
            .setTcpFlagsBwd(0x12) // SYN + ACK
            .build();
        
        acc = aggregator.add(synFlow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertTrue(session.getHasSyn(), "应检测到 SYN 标志");
        assertTrue(session.getIsEstablished(), "SYN + ACK 应标记为 ESTABLISHED");
        assertEquals(1, session.getFlagsSyn(), "flags_syn 应为 1（presence）");
        assertEquals(1, session.getFlagsAck(), "flags_ack 应为 1（presence）");
    }

    @Test
    @DisplayName("✅ 修复：TCP RST 检测与 end_reason 大小写")
    void testSessionWithRst() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent rstFlow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setTcpFlagsFwd(0x04) // RST
            .build();
        
        acc = aggregator.add(rstFlow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertTrue(session.getHasRst(), "应检测到 RST 标志");
        assertEquals("RST", session.getEndReason(), "✅ 修复：end_reason 应为大写 RST");
        assertEquals(1, session.getFlagsRst(), "flags_rst 应为 1（presence）");
    }

    @Test
    @DisplayName("TCP FIN 检测与 end_reason")
    void testSessionWithFin() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent finFlow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setTcpFlagsFwd(0x01) // FIN
            .build();
        
        acc = aggregator.add(finFlow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertTrue(session.getHasFin(), "应检测到 FIN 标志");
        assertEquals("FIN", session.getEndReason(), "end_reason 应为 FIN");
        assertEquals(1, session.getFlagsFin(), "flags_fin 应为 1（presence）");
    }

    @Test
    @DisplayName("TIMEOUT end_reason（无 FIN/RST）")
    void testSessionTimeout() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent normalFlow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setTcpFlagsFwd(0x10) // 仅 ACK
            .build();
        
        acc = aggregator.add(normalFlow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertFalse(session.getHasFin());
        assertFalse(session.getHasRst());
        assertEquals("TIMEOUT", session.getEndReason(), "无 FIN/RST 应标记为 TIMEOUT");
    }

    @Test
    @DisplayName("✅ 新增：包长统计二阶矩验证")
    void testPacketLengthStatistics() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        // 创建带包长统计的 Flow
        FlowEvent flow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setPacketsFwd(100)
            .setPacketsBwd(50)
            .setBytesFwd(10000)
            .setBytesBwd(5000)
            .setPktlenStats(PacketLengthStats.newBuilder()
                .setMin(64)
                .setMax(1500)
                .setMean(100.0f)
                .setStd(20.0f)
                .build())
            .build();
        
        acc = aggregator.add(flow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertEquals(150, session.getNumPkts(), "总包数应为 150");
        assertTrue(session.getAvgPayload() > 0, "平均包长应大于 0");
        assertEquals(64, session.getMinPayload(), "最小包长应为 64");
        assertEquals(1500, session.getMaxPayload(), "最大包长应为 1500");
        assertTrue(session.getStdPayload() > 0, "标准差应大于 0");
    }

    @Test
    @DisplayName("✅ 新增：IAT 统计二阶矩验证")
    void testInterArrivalTimeStatistics() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent flow = FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId("tenant1")
                .setRunId("run1")
                .setEventId("event1")
                .build())
            .setFlowId("flow1")
            .setCommunityId("1:abc123")
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .build())
            .setTsStart(1000)
            .setTsEnd(2000)
            .setPacketsFwd(100)
            .setPacketsBwd(50)
            .setIatStats(InterArrivalStats.newBuilder()
                .setMinMs(1.0f)
                .setMaxMs(100.0f)
                .setMeanMs(10.0f)
                .setStdMs(5.0f)
                .build())
            .build();
        
        acc = aggregator.add(flow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertTrue(session.getMeanIatMs() > 0, "平均 IAT 应大于 0");
        assertTrue(session.getMinIatMs() > 0, "最小 IAT 应大于 0");
        assertTrue(session.getMaxIatMs() > 0, "最大 IAT 应大于 0");
        assertTrue(session.getStdIatMs() >= 0, "IAT 标准差应 >= 0");
    }

    @Test
    @DisplayName("协议统计：DNS 识别")
    void testDnsProtocolDetection() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        FlowEvent dnsFlow = createTestFlow(
            "tenant1", "run1", "1:abc123",
            "192.168.1.1", "8.8.8.8",
            54321, 53, 17, // UDP + DNS 端口
            1000L, 2000L,
            10, 10,
            500, 500
        );
        
        acc = aggregator.add(dnsFlow, acc);
        SessionEvent session = aggregator.getResult(acc);
        
        assertEquals(20, session.getUdpPktCnt(), "UDP 包数应为 20");
        assertEquals(20, session.getDnsPktCnt(), "DNS 包数应为 20");
        assertEquals(0, session.getTcpPktCnt(), "TCP 包数应为 0");
    }

    @Test
    @DisplayName("Flow IDs 限制验证（最多 100 个）")
    void testFlowIdsLimit() {
        SessionAccumulator acc = aggregator.createAccumulator();
        
        // 添加 150 个 Flow，验证只保留前 100 个
        for (int i = 0; i < 150; i++) {
            FlowEvent flow = createTestFlow(
                "tenant1", "run1", "1:abc123",
                "192.168.1.1", "10.0.0.1",
                12345, 80, 6,
                1000L + i, 2000L + i,
                10, 5,
                100, 50
            );
            acc = aggregator.add(flow, acc);
        }
        
        SessionEvent session = aggregator.getResult(acc);
        
        assertEquals(150, acc.flowCount, "应记录 150 个 Flow");
        assertTrue(session.getFlowIdsCount() <= 100, "Flow IDs 应限制在 100 个以内");
    }

    // ==================== 辅助方法 ====================

    /**
     * 创建测试用 FlowEvent
     */
    private FlowEvent createTestFlow(
            String tenantId, String runId, String communityId,
            String srcIp, String dstIp,
            int srcPort, int dstPort, int protocol,
            long tsStart, long tsEnd,
            int packetsFwd, int packetsBwd,
            long bytesFwd, long bytesBwd) {
        
        return FlowEvent.newBuilder()
            .setHeader(EventHeader.newBuilder()
                .setTenantId(tenantId)
                .setRunId(runId)
                .setEventId("test-event-" + System.nanoTime())
                .setEventTs(tsEnd)
                .setIngestTs(tsEnd + 110)
                .setKafkaTs(tsEnd + 220)
                .setFeatureSetId("default")
                .setProbeId("probe-1")
                .build())
            .setFlowId("test-flow-" + System.nanoTime())
            .setCommunityId(communityId)
            .setTuple(FiveTuple.newBuilder()
                .setSrcIp(srcIp)
                .setDstIp(dstIp)
                .setSrcPort(srcPort)
                .setDstPort(dstPort)
                .setProtocol(protocol)
                .build())
            .setTsStart(tsStart)
            .setTsEnd(tsEnd)
            .setPacketsFwd(packetsFwd)
            .setPacketsBwd(packetsBwd)
            .setBytesFwd(bytesFwd)
            .setBytesBwd(bytesBwd)
            .build();
    }
}
