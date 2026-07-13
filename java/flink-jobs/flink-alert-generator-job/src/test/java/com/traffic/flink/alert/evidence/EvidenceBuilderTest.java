package com.traffic.flink.alert.evidence;

import com.traffic.proto.traffic.v1.Evidence;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * EvidenceBuilder 单元测试
 */
class EvidenceBuilderTest {

    @Test
    @DisplayName("基本构建测试")
    void testBasicBuild() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");

        Evidence evidence = builder
                .setType("behavior_detection")
                .setSummary("检测到恶意行为")
                .setConfidence(0.95f)
                .build(1700000000000L);

        assertEquals("tenant-1", evidence.getTenantId());
        assertEquals("evidence-1", evidence.getEvidenceId());
        assertEquals("alert-1", evidence.getAlertId());
        assertEquals("behavior_detection", evidence.getType());
        assertEquals("检测到恶意行为", evidence.getSummary());
        assertEquals(0.95f, evidence.getConfidence(), 0.01f);
        assertEquals(1700000000000L, evidence.getTs());
        assertTrue(evidence.getIngestTs() > 0);
    }

    @Test
    @DisplayName("Metrics JSON 序列化测试")
    void testMetricsJson() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");

        Evidence evidence = builder
                .addMetric("model_version", "v1.0")
                .addMetric("score", 0.95)
                .addMetric("count", 100)
                .build(1000L);

        String metricsJson = evidence.getMetricsJson();
        assertNotNull(metricsJson);
        assertTrue(metricsJson.contains("model_version"));
        assertTrue(metricsJson.contains("v1.0"));
        assertTrue(metricsJson.contains("score"));
        assertTrue(metricsJson.contains("0.95"));
    }

    @Test
    @DisplayName("Snippets JSON 序列化测试")
    void testSnippetsJson() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");

        Evidence evidence = builder
                .addSnippet("payload", "GET /malware HTTP/1.1")
                .addSnippet("response", "200 OK")
                .build(1000L);

        String snippetsJson = evidence.getSnippetRefJson();
        assertNotNull(snippetsJson);
        assertTrue(snippetsJson.contains("payload"));
        assertTrue(snippetsJson.contains("GET /malware"));
    }

    @Test
    @DisplayName("Arkime 链接设置测试")
    void testArkimeLink() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");

        String arkimeLink = "http://arkime:8005?expression=test";
        Evidence evidence = builder
                .setArkimeLink(arkimeLink)
                .build(1000L);

        assertEquals(arkimeLink, evidence.getArkimeLink());
    }

    @Test
    @DisplayName("Visualization URL 设置测试")
    void testVisualizationUrl() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");

        String vizUrl = "http://grafana/dashboard/123";
        Evidence evidence = builder
                .setVisualizationUrl(vizUrl)
                .build(1000L);

        assertEquals(vizUrl, evidence.getVisualizationUrl());
    }

    @Test
    @DisplayName("event_id 确定性生成测试")
    void testDeterministicEventId() {
        EvidenceBuilder builder1 = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");
        Evidence evidence1 = builder1.build(1700000000000L);

        EvidenceBuilder builder2 = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");
        Evidence evidence2 = builder2.build(1700000000000L);

        // 相同输入应生成相同的 event_id
        assertEquals(evidence1.getEventId(), evidence2.getEventId());
    }

    @Test
    @DisplayName("空 Metrics 生成空 JSON 对象")
    void testEmptyMetrics() {
        EvidenceBuilder builder = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1");
        Evidence evidence = builder.build(1000L);

        assertEquals("{}", evidence.getMetricsJson());
        assertEquals("{}", evidence.getSnippetRefJson());
    }

    @Test
    @DisplayName("链式调用测试")
    void testChainedCalls() {
        Evidence evidence = new EvidenceBuilder("tenant-1", "evidence-1", "alert-1")
                .setType("rule_match")
                .setSummary("Rule triggered")
                .setConfidence(0.99f)
                .setArkimeLink("http://arkime")
                .setVisualizationUrl("http://grafana")
                .addMetric("rule_id", "rule-123")
                .addMetric("hits", 5)
                .addSnippet("match", "SELECT * FROM users")
                .build(System.currentTimeMillis());

        assertNotNull(evidence);
        assertEquals("rule_match", evidence.getType());
        assertEquals(0.99f, evidence.getConfidence(), 0.01f);
    }
}