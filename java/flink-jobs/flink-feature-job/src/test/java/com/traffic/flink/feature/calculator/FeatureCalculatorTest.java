package com.traffic.flink.feature.calculator;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureCategory;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;
import org.junit.jupiter.params.provider.ValueSource;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * FeatureCalculator 单元测试（v2 补全版）
 */
class FeatureCalculatorTest {

    private SessionEvent.Builder sessionBuilder;

    @BeforeEach
    void setUp() {
        // 创建基础 Session 事件
        EventHeader header = EventHeader.newBuilder()
                .setEventId("test-event-1")
                .setTenantId("tenant-1")
                .setRunId("run-1")
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId("probe-1")
                .setFeatureSetId("feature-set-1")
                .build();

        FiveTuple tuple = FiveTuple.newBuilder()
                .setSrcIp("192.168.1.100")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6) // TCP
                .build();

        sessionBuilder = SessionEvent.newBuilder()
                .setHeader(header)
                .setSessionId("session-1")
                .setCommunityId("1:abc123==")
                .setTuple(tuple)
                .setTsStart(System.currentTimeMillis() - 10000)
                .setTsEnd(System.currentTimeMillis())
                .setDurationMs(10000)
                .setProtocol(6);
    }

    // ==================== 基础特征测试 ====================

    @Test
    @DisplayName("同一 Session 重放保持 Feature event_id")
    void testReplayStableEventId() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setBytesTotal(1_000)
                .addFlowIds("flow-1")
                .addFlowIds("flow-2")
                .build();

        FeatureStat first = FeatureCalculator.calculate(session);
        FeatureStat replay = FeatureCalculator.calculate(session);
        FeatureStat errorFirst = FeatureCalculator.createErrorFeature(session, "forced");
        FeatureStat errorReplay = FeatureCalculator.createErrorFeature(session, "forced");

        assertEquals(first.getHeader().getEventId(), replay.getHeader().getEventId());
        assertEquals(errorFirst.getHeader().getEventId(), errorReplay.getHeader().getEventId());
        assertNotEquals(first.getHeader().getEventId(), errorFirst.getHeader().getEventId());
        assertEquals("traffic.feature.stat.v1", first.getHeader().getEventType());
        assertEquals("v2.0", first.getHeader().getSchemaVersion());
        assertEquals("session", first.getHeader().getAggregateType());
        assertEquals("session-1", first.getHeader().getAggregateId());
        assertEquals(1, first.getHeader().getAggregateVersion());
        assertEquals(first.getHeader().getEventId(), first.getHeader().getIdempotencyKey());
        assertEquals("flink-feature-job", first.getHeader().getProducer());
        assertEquals(session.getTuple(), first.getTuple());
        assertEquals(session.getFlowIdsList(), first.getEvidenceIdsList());
        assertEquals(FeatureCategory.FEATURE_CATEGORY_FLOW_METADATA,
                first.getFeatureCategory());
        assertEquals("feature-stat-flow-metadata-v1", first.getAlgorithmVersion());
        assertEquals("session-1", first.getWindowId());
        assertEquals("mixed:rate_bps,rate_pps,time_ms,bytes,count,ratio",
                first.getValueUnit());
    }

    @Test
    @DisplayName("计算基本速率特征 - PPS 和 BPS")
    void testCalculateRateFeatures() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(1000)
                .setBytesTotal(1500000)
                .setBytesFwd(1000000)
                .setBytesBwd(500000)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(100.0f, feature.getPps(), 1.0f);
        assertEquals(1200000.0f, feature.getBps(), 1000.0f);
    }

    @Test
    @DisplayName("零时长会话不伪造1ms速率")
    void testZeroDurationKeepsRateUnavailable() {
        SessionEvent session = sessionBuilder
                .setDurationMs(0)
                .setPacketsTotal(1)
                .setBytesTotal(128)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(0, feature.getDurationMs());
        assertEquals(0.0f, feature.getPps(), 0.0f);
        assertEquals(0.0f, feature.getBps(), 0.0f);
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE,
                feature.getAvailability());
        assertTrue(feature.getMissingFieldsList().contains("rate_features.duration_ms"));
    }

    @Test
    @DisplayName("计算上下行比例")
    void testCalculateUpDownRatio() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setBytesTotal(1500)
                .setBytesFwd(1000)
                .setBytesBwd(500)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(2.0f, feature.getUpDownRatio(), 0.01f);
    }

    // ==================== IAT 特征测试（✅ 新增 min/max）====================

    @Test
    @DisplayName("IAT 特征 - 使用 min/max 字段")
    void testIatMinMaxFeatures() {
        SessionEvent session = sessionBuilder
                .setDurationMs(10000)
                .setPacketsTotal(100)
                .setMeanIatMs(100.0f)
                .setStdIatMs(25.0f)
                .setMinIatMs(10.0f)
                .setMaxIatMs(300.0f)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // 验证基础 IAT 字段
        assertEquals(100.0f, feature.getIatMeanMs(), 0.1f);
        assertEquals(25.0f, feature.getIatStdMs(), 0.1f);

        // ✅ 验证 Extra 字段中的 min/max/range
        assertEquals(10.0f, feature.getExtra(8), 0.1f);   // extra[8]: min_iat_ms
        assertEquals(300.0f, feature.getExtra(9), 0.1f);  // extra[9]: max_iat_ms
        assertEquals(290.0f, feature.getExtra(10), 0.1f); // extra[10]: iat_range_ms
    }

    @Test
    @DisplayName("IAT 缺失时不估算并显式标记")
    void testIatMissingIsNotSynthesized() {
        SessionEvent session = sessionBuilder
                .setDurationMs(10000)
                .setPacketsTotal(100)
                .setMeanIatMs(100.0f)
                .setMinIatMs(0.0f) // 未提供
                .setMaxIatMs(0.0f) // 未提供
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(0.0f, feature.getExtra(8), 0.0f);
        assertEquals(0.0f, feature.getExtra(9), 0.0f);
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE,
                feature.getAvailability());
        assertTrue(feature.getMissingFieldsList().contains("inter_arrival_statistics"));
    }

    // ==================== 包长特征测试（✅ 新增 min/max）====================

    @Test
    @DisplayName("包长特征 - min/max 作为独立特征")
    void testPacketLengthMinMax() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setBytesTotal(150000)
                .setMinPayload(100)
                .setMaxPayload(2000)
                .setAvgPayload(1500.0f)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // 验证基础包长字段
        assertEquals(1500.0f, feature.getPktlenMean(), 1.0f);

        // ✅ 验证 Extra 字段中的 min/max/avg
        assertEquals(100.0f, feature.getExtra(5), 0.1f);   // extra[5]: min_payload
        assertEquals(2000.0f, feature.getExtra(6), 0.1f);  // extra[6]: max_payload
        assertEquals(1500.0f, feature.getExtra(7), 0.1f);  // extra[7]: avg_payload
    }

    // ==================== TCP 状态特征测试（✅ 新增）====================

    @Test
    @DisplayName("TCP is_established 特征")
    void testTcpEstablishedFeature() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setIsEstablished(true)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 extra[11]: is_established = 1.0
        assertEquals(1.0f, feature.getExtra(11), 0.01f);
    }

    @Test
    @DisplayName("TCP 未建立连接")
    void testTcpNotEstablished() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setIsEstablished(false)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 extra[11]: is_established = 0.0
        assertEquals(0.0f, feature.getExtra(11), 0.01f);
    }

    // ==================== end_reason 特征测试（✅ 新增）====================

    @ParameterizedTest
    @DisplayName("end_reason 编码测试")
    @CsvSource({
            "FIN, 1.0",
            "RST, 2.0",
            "TIMEOUT, 3.0",
            "ERROR, 4.0",
            "'', 0.0",
            "UNKNOWN, 0.0"
    })
    void testEndReasonEncoding(String endReason, float expectedCode) {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setEndReason(endReason)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 extra[12]: end_reason_code
        assertEquals(expectedCode, feature.getExtra(12), 0.01f);
    }

    // ==================== evidence_count 特征测试（✅ 新增）====================

    @Test
    @DisplayName("evidence_count 特征")
    void testEvidenceCount() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setEvidenceCount(5)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 extra[13]: evidence_count = 5.0
        assertEquals(5.0f, feature.getExtra(13), 0.01f);
    }

    // ==================== TCP Flags 详细统计测试（✅ 新增）====================

    @Test
    @DisplayName("TCP Flags 详细统计")
    void testTcpFlagsDetails() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .setFlagsSyn(1)
                .setFlagsAck(95)
                .setFlagsFin(2)
                .setFlagsPsh(50)
                .setFlagsRst(0)
                .setHasSyn(true)
                .setHasFin(true)
                .setHasRst(false)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // 验证基础 Flags 字段
        assertEquals(1, feature.getTcpFlagSynCnt());
        assertEquals(95, feature.getTcpFlagAckCnt());

        // ✅ 验证 Extra 字段中的详细 Flags
        assertEquals(2.0f, feature.getExtra(14), 0.01f);   // extra[14]: flags_fin_cnt
        assertEquals(50.0f, feature.getExtra(15), 0.01f);  // extra[15]: flags_psh_cnt
        assertEquals(0.0f, feature.getExtra(16), 0.01f);   // extra[16]: flags_rst_cnt
        assertEquals(1.0f, feature.getExtra(17), 0.01f);   // extra[17]: has_syn
        assertEquals(1.0f, feature.getExtra(18), 0.01f);   // extra[18]: has_fin
        assertEquals(0.0f, feature.getExtra(19), 0.01f);   // extra[19]: has_rst
    }

    // ==================== TCP 初始窗口测试（✅ 修复）====================

    @Test
    @DisplayName("TCP 初始窗口缺失时不使用哨兵值")
    void testTcpInitWindowUnknown() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(0, feature.getTcpInitWinBytesFwd());
        assertEquals(0, feature.getTcpInitWinBytesBwd());
        assertTrue(feature.getMissingFieldsList().contains("tcp_initial_window_fwd_bytes"));
        assertTrue(feature.getMissingFieldsList().contains("tcp_initial_window_bwd_bytes"));
        assertEquals(0.0f, feature.getActiveMeanMs(), 0.0f);
        assertEquals(0.0f, feature.getIdleMeanMs(), 0.0f);
        assertTrue(feature.getMissingFieldsList().contains("active_mean_ms"));
        assertTrue(feature.getMissingFieldsList().contains("idle_mean_ms"));
    }

    // ==================== Extra 字段完整性测试 ====================

    @Test
    @DisplayName("Extra 字段完整性 - 20 个槽位")
    void testExtraFieldsCompleteness() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(1000)
                .setDnsPktCnt(100)
                .setTcpPktCnt(800)
                .setUdpPktCnt(50)
                .setIcmpPktCnt(50)
                .setStdPayload(125.5f)
                .setMinPayload(64)
                .setMaxPayload(1500)
                .setAvgPayload(1000.0f)
                .setMeanIatMs(100.0f)
                .setMinIatMs(10.0f)
                .setMaxIatMs(300.0f)
                .setIsEstablished(true)
                .setEndReason("FIN")
                .setEvidenceCount(3)
                .setFlagsFin(2)
                .setFlagsPsh(50)
                .setFlagsRst(0)
                .setHasSyn(true)
                .setHasFin(true)
                .setHasRst(false)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 Extra 字段数量
        assertEquals(20, feature.getExtraCount(), "Extra 字段应有 20 个槽位");

        // 验证关键字段
        assertEquals(0.1f, feature.getExtra(0), 0.001f);    // dns_pkt_ratio
        assertEquals(0.8f, feature.getExtra(1), 0.001f);    // tcp_pkt_ratio
        assertEquals(64.0f, feature.getExtra(5), 0.1f);     // min_payload
        assertEquals(1500.0f, feature.getExtra(6), 0.1f);   // max_payload
        assertEquals(10.0f, feature.getExtra(8), 0.1f);     // min_iat_ms
        assertEquals(300.0f, feature.getExtra(9), 0.1f);    // max_iat_ms
        assertEquals(290.0f, feature.getExtra(10), 0.1f);   // iat_range_ms
        assertEquals(1.0f, feature.getExtra(11), 0.01f);    // is_established
        assertEquals(1.0f, feature.getExtra(12), 0.01f);    // end_reason_code (FIN=1)
        assertEquals(3.0f, feature.getExtra(13), 0.01f);    // evidence_count
    }

    @Test
    @DisplayName("Extra 字段 - 空会话")
    void testExtraEmptySession() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(0)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // 空会话所有 Extra 字段应为 0
        assertEquals(20, feature.getExtraCount());
        for (int i = 0; i < 20; i++) {
            assertEquals(0.0f, feature.getExtra(i), "Extra[" + i + "] 应为 0");
        }
    }

    // ==================== Schema 版本测试 ====================

    @Test
    @DisplayName("Schema 版本升级到 v2.0")
    void testSchemaVersionV2() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        // ✅ 验证 Schema 版本为 v2.0
        assertEquals("v2.0", feature.getSchemaVersion());
    }

    // ==================== 边界条件测试 ====================

    @Test
    @DisplayName("边界条件 - 零包会话")
    void testZeroPacketsSession() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(0)
                .setBytesTotal(0)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertNotNull(feature);
        assertEquals(0.0f, feature.getPps());
        assertEquals(0.0f, feature.getBps());
        assertEquals(20, feature.getExtraCount());
    }

    @Test
    @DisplayName("边界条件 - 超大持续时间")
    void testLongDurationOverflow() {
        SessionEvent session = sessionBuilder
                .setDurationMs(Integer.MAX_VALUE)
                .setPacketsTotal(1000)
                .setBytesTotal(10000)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertNotNull(feature);
        assertEquals(Integer.MAX_VALUE, feature.getDurationMs());
    }

    // ==================== 协议类型测试 ====================

    @ParameterizedTest
    @DisplayName("协议类型边界 - 不同协议")
    @ValueSource(ints = {1, 6, 17, 41, 47, 58, 132})  // ICMP, TCP, UDP, IPv6, GRE, ICMPv6, SCTP
    void testProtocolTypes(int protocol) {
        SessionEvent session = sessionBuilder
                .setProtocol(protocol)
                .setPacketsTotal(100)
                .setBytesTotal(1000)
                .build();

        FeatureStat feature = FeatureCalculator.calculate(session);

        assertEquals(protocol, feature.getProtocol());
    }

    // ==================== 性能测试 ====================

    @Test
    @DisplayName("性能测试 - 批量计算 1000 次")
    void testBatchCalculationPerformance() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(1000)
                .setBytesTotal(1500000)
                .setBytesFwd(1000000)
                .setBytesBwd(500000)
                .setMeanIatMs(100.0f)
                .setMinIatMs(10.0f)
                .setMaxIatMs(300.0f)
                .setIsEstablished(true)
                .setEndReason("FIN")
                .build();

        long startTime = System.nanoTime();
        for (int i = 0; i < 1000; i++) {
            FeatureStat feature = FeatureCalculator.calculate(session);
            assertNotNull(feature);
        }
        long endTime = System.nanoTime();

        long totalDurationMs = (endTime - startTime) / 1_000_000;
        assertTrue(totalDurationMs < 100, "批量计算耗时过长: " + totalDurationMs + " ms");
    }

    // ==================== 错误处理测试 ====================

    @Test
    @DisplayName("创建错误特征对象")
    void testCreateErrorFeature() {
        SessionEvent session = sessionBuilder
                .setPacketsTotal(100)
                .build();

        String errorMessage = "Test error";
        FeatureStat errorFeature = FeatureCalculator.createErrorFeature(session, errorMessage);

        assertNotNull(errorFeature);
        assertEquals("error", errorFeature.getObjectType());
        assertEquals("session-1", errorFeature.getObjectId());
        assertEquals("1:abc123==", errorFeature.getCommunityId());
        assertEquals("v2.0", errorFeature.getSchemaVersion());
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_INVALID,
                errorFeature.getAvailability());
        assertEquals("Test error", errorFeature.getMissingReason());
        assertEquals(List.of("feature_calculation"), errorFeature.getMissingFieldsList());
    }

    @Test
    @DisplayName("旧 Feature wire 未提供 M03 字段时保持显式默认")
    void testLegacyFeatureWireDefaultsRemainCompatible() throws Exception {
        byte[] legacyWire = FeatureStat.newBuilder()
                .setSchemaVersion("v2.0")
                .setObjectType("session")
                .setObjectId("legacy-session")
                .build()
                .toByteArray();

        FeatureStat decoded = FeatureStat.parseFrom(legacyWire);
        assertEquals("legacy-session", decoded.getObjectId());
        assertEquals(FeatureCategory.FEATURE_CATEGORY_UNSPECIFIED,
                decoded.getFeatureCategory());
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_UNSPECIFIED,
                decoded.getAvailability());
        assertEquals("", decoded.getAlgorithmVersion());
        assertTrue(decoded.getMissingFieldsList().isEmpty());
    }
}
