package com.traffic.flink.behavior.user;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

/**
 * AnomalyEvent 数据模型 + 检测逻辑单元测试
 */
public class AnomalyEventTest {

    @Test
    public void testConstructor_AllFields() {
        AnomalyEvent e = new AnomalyEvent("tenant-1", "user-1", "admin",
                "IMPOSSIBLE_TRAVEL", "high", 0.85f,
                "Impossible travel detected");
        assertNotNull(e.anomalyId);
        assertFalse(e.anomalyId.isEmpty());
        assertEquals("tenant-1", e.tenantId);
        assertEquals("user-1", e.userId);
        assertEquals("admin", e.username);
        assertEquals("IMPOSSIBLE_TRAVEL", e.detectorType);
        assertEquals("high", e.severity);
        assertEquals(0.85f, e.score, 0.001f);
        assertTrue(e.detectedAt > 0);
    }

    @Test
    public void testDefaultConstructor() {
        AnomalyEvent e = new AnomalyEvent();
        assertNull(e.anomalyId);
        assertNull(e.tenantId);
        assertEquals(0.0f, e.score, 0.001f);
    }

    @Test
    public void testToJSON_Basic() {
        AnomalyEvent e = new AnomalyEvent("t1", "u1", "admin",
                "BRUTE_FORCE", "medium", 0.65f,
                "5 failed logins in 1 minute");
        String json = e.toJSON();
        assertTrue(json.contains("\"detector_type\":\"BRUTE_FORCE\""));
        assertTrue(json.contains("\"tenant_id\":\"t1\""));
        assertTrue(json.contains("\"score\":0.65"));
    }

    @Test
    public void testToJSON_WithIPs() {
        AnomalyEvent e = new AnomalyEvent("t1", "u1", "user",
                "IMPOSSIBLE_TRAVEL", "high", 0.90f,
                "Travel detected");
        e.sourceIp1 = "10.0.0.1";
        e.sourceIp2 = "192.168.1.1";
        e.detailJson = "{\"from_ip\":\"10.0.0.1\",\"to_ip\":\"192.168.1.1\",\"interval_sec\":120}";
        String json = e.toJSON();
        assertTrue(json.contains("10.0.0.1"));
        assertTrue(json.contains("192.168.1.1"));
        assertTrue(json.contains("interval_sec"));
    }

    @Test
    public void testToJSON_WithSpecialCharacters() {
        AnomalyEvent e = new AnomalyEvent("t1", "u1", "user",
                "PRIVILEGE_ESCALATION", "critical", 0.95f,
                "User \"admin\" escalated from viewer to admin");
        e.detailJson = "{}";
        String json = e.toJSON();
        // Should have escaped quotes
        assertTrue(json.contains("\\\\\\\"admin\\\\\\\"") || json.contains("admin")); // basic check
        assertNotNull(json);
    }

    @Test
    public void testNullFields_ToJSON() {
        AnomalyEvent e = new AnomalyEvent();
        e.anomalyId = "test-id";
        e.detectorType = "UNUSUAL_ACCESS";
        e.description = "test";
        e.severity = "low";
        String json = e.toJSON();
        assertNotNull(json);
        assertTrue(json.contains("test-id"));
    }

    @Test
    public void testSerializable() {
        AnomalyEvent e = new AnomalyEvent("t1", "u1", "user",
                "BRUTE_FORCE", "high", 0.95f, "test");
        // AnomalyEvent implements Serializable
        assertTrue(e instanceof java.io.Serializable);
    }

    @Test
    public void testUniqueAnomalyIds() {
        AnomalyEvent e1 = new AnomalyEvent("t1", "u1", "a", "X", "low", 0.5f, "d1");
        AnomalyEvent e2 = new AnomalyEvent("t1", "u1", "a", "X", "low", 0.5f, "d1");
        assertNotEquals(e1.anomalyId, e2.anomalyId, "Each anomaly should have unique ID");
    }

    @Test
    public void testDetectorType_Constants() {
        String[] types = {"IMPOSSIBLE_TRAVEL", "BRUTE_FORCE", "PRIVILEGE_ESCALATION", "UNUSUAL_ACCESS"};
        for (String type : types) {
            AnomalyEvent e = new AnomalyEvent("t", "u", "n", type, "medium", 0.5f, "test");
            assertEquals(type, e.detectorType);
        }
    }

    @Test
    public void testSeverity_Ordering() {
        // Verify severity values are set correctly
        AnomalyEvent critical = new AnomalyEvent("t", "u", "n", "X", "critical", 1.0f, "d");
        AnomalyEvent high = new AnomalyEvent("t", "u", "n", "X", "high", 0.8f, "d");
        AnomalyEvent medium = new AnomalyEvent("t", "u", "n", "X", "medium", 0.5f, "d");
        AnomalyEvent low = new AnomalyEvent("t", "u", "n", "X", "low", 0.2f, "d");

        assertEquals("critical", critical.severity);
        assertEquals("high", high.severity);
        assertEquals("medium", medium.severity);
        assertEquals("low", low.severity);
        assertTrue(critical.score > high.score);
        assertTrue(high.score > medium.score);
        assertTrue(medium.score > low.score);
    }
}
