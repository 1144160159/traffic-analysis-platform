package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.streaming.api.functions.async.ResultFuture;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.util.Collection;
import java.util.Collections;
import java.util.concurrent.CompletableFuture;
import java.sql.PreparedStatement;
import java.sql.Types;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * ClickHouseAsyncSinkFunction 单元测试
 * 
 * 测试要点：
 * 1. 成功写入时返回空集合
 * 2. 写入失败时返回原始数据（用于 DLQ）
 * 3. 超时时返回原始数据
 */
class ClickHouseAsyncSinkFunctionTest {

    private ClickHouseAsyncSinkFunction asyncSinkFunction;

    @BeforeEach
    void setUp() {
        // 使用无效的 JDBC URL，确保写入会失败
        asyncSinkFunction = new ClickHouseAsyncSinkFunction(
                "jdbc:clickhouse://invalid-host:8123/test",
                "sessions_local",
                "default",
                "",
                1000,
                1000,
                1,      // 只重试 1 次
                2
        );
    }

    @Test
    void testTimeoutReturnsOriginalData() throws Exception {
        // 模拟 ResultFuture
        @SuppressWarnings("unchecked")
        ResultFuture<SessionEvent> resultFuture = mock(ResultFuture.class);

        SessionEvent session = createTestSession("tenant1", "session-1");

        // 调用 timeout
        asyncSinkFunction.timeout(session, resultFuture);

        // 验证返回了原始数据
        ArgumentCaptor<Collection<SessionEvent>> captor = ArgumentCaptor.forClass(Collection.class);
        verify(resultFuture).complete(captor.capture());

        Collection<SessionEvent> result = captor.getValue();
        assertEquals(1, result.size());
        assertEquals(session, result.iterator().next());
    }

    @Test
    void testSessionEventCreation() {
        // 测试 SessionEvent 构建
        SessionEvent session = createTestSession("tenant1", "session-1");

        assertNotNull(session);
        assertEquals("tenant1", session.getHeader().getTenantId());
        assertEquals("session-1", session.getSessionId());
        assertEquals("1:abc123", session.getCommunityId());
        assertEquals(1000, session.getDurationMs());
        assertEquals(150, session.getPacketsTotal());
        assertEquals(1500, session.getBytesTotal());
    }

    @Test
    void testBuildInsertSql() throws Exception {
        String sql = ClickHouseSinkFactory.buildInsertSql("sessions_local");
        assertEquals(62, sql.chars().filter(ch -> ch == '?').count());
        assertTrue(sql.contains("event_schema_version"));
        assertTrue(sql.contains("source_watermark_ms"));
        assertTrue(sql.contains("missing_fields"));
        assertFalse(sql.contains("bytes_up"));
        assertFalse(sql.contains("packets_total"));
    }

    @Test
    void testM03ContractFieldsAreBoundWithoutFabricatedWatermark() throws Exception {
        PreparedStatement statement = mock(PreparedStatement.class);
        SessionEvent session = createTestSession("tenant1", "session-1").toBuilder()
                .setIdentityVersion("session-id-v1")
                .setSessionVersion(7)
                .setEventTimeStartMs(1000)
                .setEventTimeEndMs(2000)
                .addSourceEventIds("flow-event-1")
                .addEvidenceIds("evidence-1")
                .setCompleteness(com.traffic.proto.traffic.v1.SessionCompleteness.SESSION_COMPLETENESS_PARTIAL)
                .addMissingFields("inter_arrival_statistics")
                .setHeader(createTestSession("tenant1", "header-source").getHeader().toBuilder()
                        .setSchemaVersion("v1.0")
                        .setAggregateVersion(7)
                        .setFlinkOutTs(0)
                        .build())
                .build();

        ClickHouseSinkFactory.bindStatement(statement, session);

        verify(statement).setString(51, "v1.0");
        verify(statement).setLong(52, 7L);
        verify(statement).setString(53, "session-id-v1");
        verify(statement).setLong(54, 7L);
        verify(statement).setLong(55, 1000L);
        verify(statement).setLong(56, 2000L);
        verify(statement).setNull(57, Types.BIGINT);
        verify(statement).setString(60, "SESSION_COMPLETENESS_PARTIAL");
        verify(statement).setInt(61, 1);
        verify(statement).setLong(17, 0L);
    }

    // ==================== 辅助方法 ====================

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
                .setEndReason("IDLE_TIMEOUT")
                .build();
    }
}
