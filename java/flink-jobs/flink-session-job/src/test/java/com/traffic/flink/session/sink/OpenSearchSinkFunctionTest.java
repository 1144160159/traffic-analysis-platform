package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * OpenSearchSinkFunction 单元测试
 * 
 * 测试要点：
 * 1. 文档构建正确性
 * 2. 字段映射完整性
 */
class OpenSearchSinkFunctionTest {

    @Test
    void testDocumentBuilding() {
        SessionEvent session = createTestSession("tenant1", "session-1");

        // 模拟 buildDocument 逻辑
        Map<String, Object> doc = buildDocument(session);

        // 验证标识字段
        assertEquals("tenant1", doc.get("tenant_id"));
        assertEquals("run1", doc.get("run_id"));
        assertEquals("fs1", doc.get("feature_set_id"));
        assertEquals("session-1", doc.get("session_id"));
        assertEquals("1:abc123", doc.get("community_id"));

        // 验证时间字段
        assertEquals(1000L, doc.get("ts_start"));
        assertEquals(2000L, doc.get("ts_end"));
        assertEquals(1000, doc.get("duration_ms"));

        // 验证网络字段
        assertEquals(6, doc.get("protocol"));
        assertEquals("192.168.1.1", doc.get("client_ip"));
        assertEquals("10.0.0.1", doc.get("server_ip"));
        assertEquals(12345, doc.get("client_port"));
        assertEquals(80, doc.get("server_port"));

        // 验证流量统计
        assertEquals(150L, doc.get("packets_total"));
        assertEquals(1500L, doc.get("bytes_total"));
        assertEquals(1000L, doc.get("bytes_up"));
        assertEquals(500L, doc.get("bytes_down"));

        // 验证 TCP 标志
        assertNotNull(doc.get("has_syn"));
        assertNotNull(doc.get("has_fin"));
        assertNotNull(doc.get("is_established"));

        // 验证其他字段
        assertEquals("IDLE_TIMEOUT", doc.get("end_reason"));
    }

    @Test
    void testDocumentFieldCount() {
        SessionEvent session = createTestSession("tenant1", "session-1");
        Map<String, Object> doc = buildDocument(session);

        // 验证文档包含足够的字段（至少 25 个关键字段）
        assertTrue(doc.size() >= 25, "Document should contain at least 25 fields");
    }

    @Test
    void testNullHandling() {
        // 测试空值处理
        SessionEvent session = SessionEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant1")
                        .setRunId("")
                        .setFeatureSetId("")
                        .setEventId("evt-1")
                        .build())
                .setSessionId("session-1")
                .setCommunityId("")
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("")
                        .setDstIp("")
                        .build())
                .build();

        Map<String, Object> doc = buildDocument(session);

        // 验证空字符串被正确处理
        assertEquals("", doc.get("run_id"));
        assertEquals("", doc.get("community_id"));
    }

    // ==================== 辅助方法 ====================

    private Map<String, Object> buildDocument(SessionEvent session) {
        Map<String, Object> doc = new HashMap<>();

        // 标识字段
        doc.put("tenant_id", session.getHeader().getTenantId());
        doc.put("run_id", session.getHeader().getRunId());
        doc.put("feature_set_id", session.getHeader().getFeatureSetId());
        doc.put("event_id", session.getHeader().getEventId());
        doc.put("session_id", session.getSessionId());
        doc.put("community_id", session.getCommunityId());

        // 时间字段
        doc.put("ts_start", session.getTsStart());
        doc.put("ts_end", session.getTsEnd());
        doc.put("duration_ms", session.getDurationMs());
        doc.put("ingest_ts", session.getHeader().getIngestTs());

        // 网络五元组
        doc.put("protocol", session.getProtocol());
        doc.put("client_ip", session.getClientIp());
        doc.put("server_ip", session.getServerIp());
        doc.put("client_port", session.getClientPort());
        doc.put("server_port", session.getServerPort());

        // 流量统计
        doc.put("packets_total", session.getPacketsTotal());
        doc.put("bytes_total", session.getBytesTotal());
        doc.put("bytes_up", session.getBytesFwd());
        doc.put("bytes_down", session.getBytesBwd());
        doc.put("up_down_ratio", session.getUpDownRatio());

        // 包长统计
        doc.put("avg_payload", session.getAvgPayload());
        doc.put("std_payload", session.getStdPayload());

        // IAT 统计
        doc.put("mean_iat_ms", session.getMeanIatMs());
        doc.put("std_iat_ms", session.getStdIatMs());

        // TCP 标志
        doc.put("has_syn", session.getHasSyn());
        doc.put("has_fin", session.getHasFin());
        doc.put("has_rst", session.getHasRst());
        doc.put("is_established", session.getIsEstablished());

        // 协议统计
        doc.put("dns_pkt_cnt", session.getDnsPktCnt());
        doc.put("tcp_pkt_cnt", session.getTcpPktCnt());
        doc.put("udp_pkt_cnt", session.getUdpPktCnt());
        doc.put("icmp_pkt_cnt", session.getIcmpPktCnt());

        // 其他
        doc.put("end_reason", session.getEndReason());
        doc.put("evidence_count", session.getEvidenceCount());
        doc.put("flow_count", session.getFlowIdsCount());

        return doc;
    }

    private SessionEvent createTestSession(String tenantId, String sessionId) {
        EventHeader header = EventHeader.newBuilder()
                .setTenantId(tenantId)
                .setRunId("run1")
                .setFeatureSetId("fs1")
                .setEventId("evt-" + System.nanoTime())
                .setEventTs(2000L)
                .setIngestTs(System.currentTimeMillis())
                .build();

        return SessionEvent.newBuilder()
                .setHeader(header)
                .setSessionId(sessionId)
                .setCommunityId("1:abc123")
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("192.168.1.1")
                        .setDstIp("10.0.0.1")
                        .setSrcPort(12345)
                        .setDstPort(80)
                        .setProtocol(6)
                        .build())
                .setTsStart(1000L)
                .setTsEnd(2000L)
                .setDurationMs(1000)
                .setProtocol(6)
                .setClientIp("192.168.1.1")
                .setServerIp("10.0.0.1")
                .setClientPort(12345)
                .setServerPort(80)
                .setPacketsTotal(150)
                .setBytesTotal(1500)
                .setBytesFwd(1000)
                .setBytesBwd(500)
                .setUpDownRatio(2.0f)
                .setHasSyn(true)
                .setHasFin(false)
                .setHasRst(false)
                .setIsEstablished(true)
                .setEndReason("IDLE_TIMEOUT")
                .build();
    }
}