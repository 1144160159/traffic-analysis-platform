package com.traffic.flink.alert.generator;

import com.traffic.proto.traffic.v1.*;

import org.apache.flink.api.common.typeinfo.TypeHint;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.java.tuple.Tuple2;
import org.apache.flink.streaming.api.operators.KeyedProcessOperator;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.KeyedOneInputStreamOperatorTestHarness;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * AlertGenerator 单元测试
 * 
 * 测试覆盖：
 * 1. 新告警生成
 * 2. 去重窗口内聚合
 * 3. 去重窗口外新建
 * 4. 幂等 event_id
 * 5. 状态版本递增
 */
class AlertGeneratorTest {

    private KeyedOneInputStreamOperatorTestHarness<String, DetectionBehavior, Tuple2<Alert, Evidence>> testHarness;
    private AlertGenerator alertGenerator;

    private static final long DEDUP_WINDOW_MINUTES = 10;
    private static final String ARKIME_URL = "http://arkime.test:8005";

    @BeforeEach
    void setUp() throws Exception {
        alertGenerator = new AlertGenerator(DEDUP_WINDOW_MINUTES, ARKIME_URL);

        testHarness = new KeyedOneInputStreamOperatorTestHarness<>(
                new KeyedProcessOperator<>(alertGenerator),
                detection -> {
                    if (detection == null || detection.getHeader() == null) {
                        return "null";
                    }
                    String tenant = detection.getHeader().getTenantId();
                    if (tenant == null || tenant.isEmpty()) {
                        tenant = "default";
                    }
                    String raw = tenant + ":" + detection.getTopLabel() + ":" + detection.getCommunityId();
                    return tenant + ":" + md5Hex(raw);
                },
                TypeInformation.of(String.class)
        );

        testHarness.open();
    }

    @Test
    @DisplayName("新告警生成 - 首次检测应生成 Alert 和 Evidence")
    void testNewAlertGeneration() throws Exception {
        DetectionBehavior detection = createDetection("tenant-1", "community-1", "malware", 0.95f, 1000L);

        testHarness.processElement(new StreamRecord<>(detection, 1000L));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(1, output.size());

        Tuple2<Alert, Evidence> result = output.get(0);
        Alert alert = result.f0;
        Evidence evidence = result.f1;

        // 验证 Alert
        assertNotNull(alert);
        assertEquals("tenant-1", alert.getTenantId());
        assertEquals("malware", alert.getAlertType());
        assertEquals(0.95f, alert.getScore(), 0.01f);
        assertEquals(Severity.SEVERITY_CRITICAL, alert.getSeverity());
        assertEquals(1, alert.getCount());
        assertEquals(1L, alert.getStateVersion());
        assertTrue(alert.getAlertId().startsWith("alert-tenant-1-"));
        assertFalse(alert.getEventId().isEmpty());
        assertTrue(alert.getArkimeSessionLink().contains("arkime.test"));

        // 验证 Evidence
        assertNotNull(evidence);
        assertEquals("tenant-1", evidence.getTenantId());
        assertEquals(alert.getAlertId(), evidence.getAlertId());
        assertTrue(evidence.getEvidenceId().startsWith("evidence-"));
        assertEquals("behavior_detection", evidence.getType());
        assertFalse(evidence.getEventId().isEmpty());
    }

    @Test
    @DisplayName("去重窗口内聚合 - 相同指纹应更新而非新建")
    void testDeduplicationWithinWindow() throws Exception {
        long baseTime = System.currentTimeMillis();

        // 第一条检测
        DetectionBehavior d1 = createDetection("tenant-1", "community-1", "malware", 0.95f, baseTime);
        testHarness.processElement(new StreamRecord<>(d1, baseTime));

        // 第二条检测（同指纹，窗口内）
        DetectionBehavior d2 = createDetection("tenant-1", "community-1", "malware", 0.96f, baseTime + 60_000);
        testHarness.processElement(new StreamRecord<>(d2, baseTime + 60_000));

        // 第三条检测（同指纹，窗口内）
        DetectionBehavior d3 = createDetection("tenant-1", "community-1", "malware", 0.97f, baseTime + 120_000);
        testHarness.processElement(new StreamRecord<>(d3, baseTime + 120_000));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(3, output.size());

        // 第一条是新告警（有 Evidence）
        Alert first = output.get(0).f0;
        Evidence firstEvidence = output.get(0).f1;
        assertEquals(1, first.getCount());
        assertEquals(1L, first.getStateVersion());
        assertNotNull(firstEvidence);

        // 第二条是更新（无 Evidence）
        Alert second = output.get(1).f0;
        Evidence secondEvidence = output.get(1).f1;
        assertEquals(2, second.getCount());
        assertEquals(2L, second.getStateVersion());
        assertNull(secondEvidence);
        assertEquals(first.getAlertId(), second.getAlertId()); // 同一 alert_id

        // 第三条也是更新
        Alert third = output.get(2).f0;
        assertEquals(3, third.getCount());
        assertEquals(3L, third.getStateVersion());
        assertEquals(first.getAlertId(), third.getAlertId());
    }

    @Test
    @DisplayName("去重窗口外新建 - 超出窗口应生成新告警")
    void testNewAlertAfterWindowExpiry() throws Exception {
        long baseTime = System.currentTimeMillis();

        // 第一条检测
        DetectionBehavior d1 = createDetection("tenant-1", "community-1", "malware", 0.95f, baseTime);
        testHarness.processElement(new StreamRecord<>(d1, baseTime));

        // 第二条检测（超出窗口：11分钟后）
        long afterWindow = baseTime + (DEDUP_WINDOW_MINUTES + 1) * 60_000;
        DetectionBehavior d2 = createDetection("tenant-1", "community-1", "malware", 0.96f, afterWindow);
        testHarness.processElement(new StreamRecord<>(d2, afterWindow));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(2, output.size());

        Alert first = output.get(0).f0;
        Alert second = output.get(1).f0;

        // 应该是两个不同的 alert_id
        assertNotEquals(first.getAlertId(), second.getAlertId());
        assertEquals(1, first.getCount());
        assertEquals(1, second.getCount());
    }

    @Test
    @DisplayName("不同指纹生成不同告警")
    void testDifferentFingerprintsDifferentAlerts() throws Exception {
        long baseTime = System.currentTimeMillis();

        // 不同 alert_type
        DetectionBehavior d1 = createDetection("tenant-1", "community-1", "malware", 0.95f, baseTime);
        DetectionBehavior d2 = createDetection("tenant-1", "community-1", "tunnel", 0.85f, baseTime + 1000);

        testHarness.processElement(new StreamRecord<>(d1, baseTime));
        testHarness.processElement(new StreamRecord<>(d2, baseTime + 1000));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(2, output.size());

        assertNotEquals(output.get(0).f0.getAlertId(), output.get(1).f0.getAlertId());
        assertEquals("malware", output.get(0).f0.getAlertType());
        assertEquals("tunnel", output.get(1).f0.getAlertType());
    }

    @Test
    @DisplayName("幂等 event_id - 相同输入应生成相同 event_id")
    void testIdempotentEventId() throws Exception {
        long baseTime = 1700000000000L; // 固定时间

        DetectionBehavior d1 = createDetection("tenant-1", "community-1", "malware", 0.95f, baseTime);
        testHarness.processElement(new StreamRecord<>(d1, baseTime));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        String eventId1 = output.get(0).f0.getEventId();

        // 重新创建 harness 模拟重启
        testHarness.close();
        setUp();

        DetectionBehavior d2 = createDetection("tenant-1", "community-1", "malware", 0.95f, baseTime);
        testHarness.processElement(new StreamRecord<>(d2, baseTime));

        List<Tuple2<Alert, Evidence>> output2 = extractOutput();
        String eventId2 = output2.get(0).f0.getEventId();

        // event_id 应该相同（确定性生成）
        assertEquals(eventId1, eventId2);
    }

    @Test
    @DisplayName("严重度映射正确性")
    void testSeverityMapping() throws Exception {
        long baseTime = System.currentTimeMillis();

        // Critical: >= 0.9
        testHarness.processElement(new StreamRecord<>(
                createDetection("t1", "c1", "test", 0.95f, baseTime), baseTime));
        // High: >= 0.7
        testHarness.processElement(new StreamRecord<>(
                createDetection("t1", "c2", "test", 0.75f, baseTime + 1000), baseTime + 1000));
        // Medium: >= 0.5
        testHarness.processElement(new StreamRecord<>(
                createDetection("t1", "c3", "test", 0.55f, baseTime + 2000), baseTime + 2000));
        // Low: >= 0.3
        testHarness.processElement(new StreamRecord<>(
                createDetection("t1", "c4", "test", 0.35f, baseTime + 3000), baseTime + 3000));
        // Info: < 0.3
        testHarness.processElement(new StreamRecord<>(
                createDetection("t1", "c5", "test", 0.2f, baseTime + 4000), baseTime + 4000));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(5, output.size());

        assertEquals(Severity.SEVERITY_CRITICAL, output.get(0).f0.getSeverity());
        assertEquals(Severity.SEVERITY_HIGH, output.get(1).f0.getSeverity());
        assertEquals(Severity.SEVERITY_MEDIUM, output.get(2).f0.getSeverity());
        assertEquals(Severity.SEVERITY_LOW, output.get(3).f0.getSeverity());
        assertEquals(Severity.SEVERITY_INFO, output.get(4).f0.getSeverity());
    }

    @Test
    @DisplayName("空值处理 - null 检测应被过滤")
    void testNullHandling() throws Exception {
        // null detection
        testHarness.processElement(new StreamRecord<>(null, 1000L));

        // detection with null header
        DetectionBehavior nullHeader = DetectionBehavior.newBuilder()
                .setTs(1000L)
                .build();
        testHarness.processElement(new StreamRecord<>(nullHeader, 1000L));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        assertEquals(0, output.size());
    }

    @Test
    @DisplayName("Arkime 链接生成正确性")
    void testArkimeLinkGeneration() throws Exception {
        long ts = 1700000000000L;
        DetectionBehavior detection = createDetection("tenant-1", "1:abc123==", "malware", 0.95f, ts);

        testHarness.processElement(new StreamRecord<>(detection, ts));

        List<Tuple2<Alert, Evidence>> output = extractOutput();
        String arkimeLink = output.get(0).f0.getArkimeSessionLink();

        assertTrue(arkimeLink.contains("arkime.test:8005"));
        assertTrue(arkimeLink.contains("community.id==1:abc123=="));
        assertTrue(arkimeLink.contains("startTime="));
        assertTrue(arkimeLink.contains("stopTime="));
    }

    // ==================== Helper Methods ====================

    private DetectionBehavior createDetection(
            String tenantId,
            String communityId,
            String topLabel,
            float topScore,
            long ts
    ) {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("det-" + ts)
                .setTenantId(tenantId)
                .setRunId("run-1")
                .setEventTs(ts)
                .setIngestTs(ts)
                .setProbeId("probe-1")
                .setFeatureSetId("fs-1")
                .build();

        return DetectionBehavior.newBuilder()
                .setHeader(header)
                .setModelVersion("v1.0")
                .setCommunityId(communityId)
                .setObjectType("session")
                .setObjectId("session-" + ts)
                .setTs(ts)
                .addLabels(topLabel)
                .addScores(topScore)
                .setTopLabel(topLabel)
                .setTopScore(topScore)
                .build();
    }

    private List<Tuple2<Alert, Evidence>> extractOutput() {
        return testHarness.extractOutputValues();
    }

    private static String md5Hex(String raw) {
        try {
            java.security.MessageDigest md = java.security.MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes(java.nio.charset.StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            return sb.toString();
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }
}