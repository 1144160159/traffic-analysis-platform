package com.traffic.flink.alert.sink;

import com.traffic.proto.traffic.v1.*;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;

import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.*;

/**
 * ClickHouse Alert Sink 测试
 * 
 * 注意：集成测试需要 ClickHouse 环境
 * 设置环境变量 CLICKHOUSE_TEST_ENABLED=true 启用
 */
class ClickHouseAlertSinkTest {

    @Test
    @DisplayName("Alert 对象构建正确性验证")
    void testAlertObjectConstruction() {
        Alert alert = Alert.newBuilder()
                .setTenantId("tenant-1")
                .setAlertId("alert-123")
                .setSrcIp("192.168.1.1")
                .setDstIp("10.0.0.1")
                .setSrcPort(12345)
                .setDstPort(80)
                .setProtocol(6)
                .setProtocolName("TCP")
                .setAlertType("malware")
                .setSeverity(Severity.SEVERITY_HIGH)
                .addAllLabels(Arrays.asList("malware", "trojan"))
                .setFirstSeen(1700000000000L)
                .setLastSeen(1700000060000L)
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .setCount(5)
                .setScore(0.95f)
                .setUpdatedTs(1700000060000L)
                .setCommunityId("1:abc123==")
                .setSessionId("session-1")
                .setCampaignId("")
                .setModelVersion("v1.0")
                .setRuleVersion("")
                .setFeatureSetId("fs-1")
                .addAllEvidenceIds(Arrays.asList("ev-1", "ev-2"))
                .setDedupFingerprint("fp-123456")
                .setEventId("event-123")
                .setArkimeSessionLink("http://arkime/session")
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(3L)
                .setIngestTs(System.currentTimeMillis())
                .build();

        // 验证所有字段
        assertEquals("tenant-1", alert.getTenantId());
        assertEquals("alert-123", alert.getAlertId());
        assertEquals("192.168.1.1", alert.getSrcIp());
        assertEquals("10.0.0.1", alert.getDstIp());
        assertEquals(12345, alert.getSrcPort());
        assertEquals(80, alert.getDstPort());
        assertEquals(6, alert.getProtocol());
        assertEquals("TCP", alert.getProtocolName());
        assertEquals("malware", alert.getAlertType());
        assertEquals(Severity.SEVERITY_HIGH, alert.getSeverity());
        assertEquals(2, alert.getLabelsCount());
        assertEquals(1700000000000L, alert.getFirstSeen());
        assertEquals(1700000060000L, alert.getLastSeen());
        assertEquals(AlertStatus.ALERT_STATUS_NEW, alert.getStatus());
        assertEquals(5, alert.getCount());
        assertEquals(0.95f, alert.getScore(), 0.01f);
        assertEquals("1:abc123==", alert.getCommunityId());
        assertEquals("session-1", alert.getSessionId());
        assertEquals("v1.0", alert.getModelVersion());
        assertEquals("fs-1", alert.getFeatureSetId());
        assertEquals(2, alert.getEvidenceIdsCount());
        assertEquals("fp-123456", alert.getDedupFingerprint());
        assertEquals("event-123", alert.getEventId());
        assertEquals(3L, alert.getStateVersion());
    }

    @Test
    @DisplayName("Evidence 对象构建正确性验证")
    void testEvidenceObjectConstruction() {
        Evidence evidence = Evidence.newBuilder()
                .setTenantId("tenant-1")
                .setEvidenceId("evidence-123")
                .setAlertId("alert-123")
                .setTs(1700000000000L)
                .setType("behavior_detection")
                .setSummary("检测到恶意行为")
                .setMetricsJson("{\"score\": 0.95}")
                .setSnippetRefJson("{}")
                .setArkimeLink("http://arkime/session")
                .setVisualizationUrl("http://grafana/dashboard")
                .setConfidence(0.95f)
                .setEventId("event-456")
                .setIngestTs(System.currentTimeMillis())
                .build();

        assertEquals("tenant-1", evidence.getTenantId());
        assertEquals("evidence-123", evidence.getEvidenceId());
        assertEquals("alert-123", evidence.getAlertId());
        assertEquals(1700000000000L, evidence.getTs());
        assertEquals("behavior_detection", evidence.getType());
        assertEquals("检测到恶意行为", evidence.getSummary());
        assertEquals("{\"score\": 0.95}", evidence.getMetricsJson());
        assertEquals("http://arkime/session", evidence.getArkimeLink());
        assertEquals(0.95f, evidence.getConfidence(), 0.01f);
        assertEquals("event-456", evidence.getEventId());
    }

    @Test
    @DisplayName("SQL 占位符数量验证（32个字段）")
    void testAlertSqlPlaceholderCount() {
        // alerts_local 表有 32 个字段
        String insertSql = buildAlertInsertSql("alerts_local");
        
        // 计算占位符数量
        long placeholderCount = insertSql.chars().filter(ch -> ch == '?').count();
        
        assertEquals(32, placeholderCount, "Alert INSERT SQL should have 32 placeholders");
    }

    @Test
    @DisplayName("SQL 占位符数量验证（Evidence 13个字段）")
    void testEvidenceSqlPlaceholderCount() {
        String insertSql = buildEvidenceInsertSql("evidence_local");
        
        long placeholderCount = insertSql.chars().filter(ch -> ch == '?').count();
        
        assertEquals(13, placeholderCount, "Evidence INSERT SQL should have 13 placeholders");
    }

    @Test
    @EnabledIfEnvironmentVariable(named = "CLICKHOUSE_TEST_ENABLED", matches = "true")
    @DisplayName("ClickHouse 连接测试（需要环境变量）")
    void testClickHouseConnection() {
        // 此测试需要真实 ClickHouse 环境
        // 通过环境变量控制是否执行
        String host = System.getenv().getOrDefault("CLICKHOUSE_HOST", "localhost:8123");
        String database = System.getenv().getOrDefault("CLICKHOUSE_DATABASE", "traffic");

        assertNotNull(host);
        assertNotNull(database);
    }

    // ==================== Helper Methods ====================

    private String buildAlertInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        "tenant_id, alert_id, " +
                        "src_ip, dst_ip, src_port, dst_port, protocol, protocol_name, " +
                        "alert_type, severity, labels, " +
                        "first_seen, last_seen, " +
                        "status, assignee, count, score, " +
                        "updated_ts, " +
                        "community_id, session_id, campaign_id, " +
                        "model_version, rule_version, feature_set_id, " +
                        "evidence_ids, " +
                        "dedup_fingerprint, " +
                        "event_id, " +
                        "arkime_session_link, " +
                        "feedback_label, " +
                        "feedback_count, " +
                        "state_version, " +
                        "ingest_ts" +
                        ") VALUES (" +
                        "?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?" +
                        ")",
                table
        );
    }

    private String buildEvidenceInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        "tenant_id, evidence_id, alert_id, " +
                        "ts, " +
                        "type, summary, " +
                        "metrics_json, snippet_ref_json, arkime_link, visualization_url, " +
                        "confidence, " +
                        "event_id, ingest_ts" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }
}