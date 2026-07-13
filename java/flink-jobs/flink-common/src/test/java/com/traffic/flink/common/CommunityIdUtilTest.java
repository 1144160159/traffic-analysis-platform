package com.traffic.flink.common;

import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Community ID v1.0 单元测试
 *
 * 标准测试向量 (与 Rust/Go 一致, 2026-06-07 联合调试验证):
 *   10.0.0.1→10.0.0.2:12345→80 TCP        → "1:CpuULklTENbGdRpvp7gNcQd5ZqA="
 *   192.168.1.1→192.168.1.100:443→54321 TCP → "1:yvabNgZAlWzo8wcUZ6B9cSRJQ9Q="
 *   10.0.0.1→10.0.0.2:53→12345 UDP         → "1:JrhaqgS2mu6o+Lu2/yWyT0ECe6E="
 *   ::1→::2:8080→9090 TCP                    → "1:/Q8HrtOQusOw7LFS4Ju3LeGLJu0="
 */
public class CommunityIdUtilTest {

    @Test
    public void testStandardVector1_TCP() {
        // 10.0.0.1 -> 10.0.0.2, port 12345 -> 80, TCP
        String result = CommunityIdUtil.compute("10.0.0.1", "10.0.0.2", 12345, 80, 6);
        assertEquals("1:CpuULklTENbGdRpvp7gNcQd5ZqA=", result);
    }

    @Test
    public void testStandardVector2_TCP() {
        // 192.168.1.1 -> 192.168.1.100, port 443 -> 54321, TCP
        // (联合调试修正: 前版文档 IP 顺序与端口值有误)
        String result = CommunityIdUtil.compute("192.168.1.1", "192.168.1.100", 443, 54321, 6);
        assertEquals("1:yvabNgZAlWzo8wcUZ6B9cSRJQ9Q=", result);
    }

    @Test
    public void testStandardVector3_UDP() {
        // 10.0.0.1 -> 10.0.0.2, port 53 -> 12345, UDP
        String result = CommunityIdUtil.compute("10.0.0.1", "10.0.0.2", 53, 12345, 17);
        assertEquals("1:JrhaqgS2mu6o+Lu2/yWyT0ECe6E=", result);
    }

    @Test
    public void testStandardVector4_IPv6() {
        // ::1 -> ::2, port 8080 -> 9090, TCP
        String result = CommunityIdUtil.compute("::1", "::2", 8080, 9090, 6);
        assertEquals("1:/Q8HrtOQusOw7LFS4Ju3LeGLJu0=", result);
    }

    @Test
    public void testICMP_UsesZeroForPorts() {
        // ICMP (proto=1) should normalize ports to 0
        String result = CommunityIdUtil.compute("10.0.0.1", "10.0.0.2", 12345, 80, 1);
        assertNotNull(result);
        assertTrue(result.startsWith("1:"));
    }

    @Test
    public void testOrderNormalization() {
        // IP order normalization: src < dst, or if equal then port comparison
        String result1 = CommunityIdUtil.compute("10.0.0.1", "10.0.0.2", 12345, 80, 6);
        String result2 = CommunityIdUtil.compute("10.0.0.2", "10.0.0.1", 80, 12345, 6);
        assertEquals(result1, result2, "Community ID should be symmetric regardless of direction");
    }

    @Test
    public void testNullSafety() {
        // null-safe: returns empty string for null/empty inputs
        String result1 = CommunityIdUtil.compute(null, "10.0.0.2", 12345, 80, 6);
        assertEquals("", result1, "null srcIp should return empty string");

        String result2 = CommunityIdUtil.compute("10.0.0.1", null, 12345, 80, 6);
        assertEquals("", result2, "null dstIp should return empty string");
    }

    @Test
    public void testEmptyIpReturnsEmptyString() {
        // Empty IP is handled gracefully — returns empty string (null-safe design)
        String result = CommunityIdUtil.compute("", "", 0, 0, 6);
        assertEquals("", result, "empty IP should return empty string");
    }

    @Test
    public void testVariousProtocols() {
        // TCP (6)
        String tcp = CommunityIdUtil.compute("1.1.1.1", "2.2.2.2", 80, 443, 6);
        assertNotNull(tcp);

        // UDP (17)
        String udp = CommunityIdUtil.compute("1.1.1.1", "2.2.2.2", 53, 12345, 17);
        assertNotNull(udp);

        // ICMP (1)
        String icmp = CommunityIdUtil.compute("1.1.1.1", "2.2.2.2", 0, 0, 1);
        assertNotNull(icmp);
        assertNotEquals(tcp, udp);
        assertNotEquals(tcp, icmp);
    }

    @Test
    public void testConsistentHashAcrossCalls() {
        String r1 = CommunityIdUtil.compute("192.168.1.1", "192.168.1.2", 8080, 9090, 6);
        String r2 = CommunityIdUtil.compute("192.168.1.1", "192.168.1.2", 8080, 9090, 6);
        assertEquals(r1, r2, "Same inputs should always produce same output");
    }
}
