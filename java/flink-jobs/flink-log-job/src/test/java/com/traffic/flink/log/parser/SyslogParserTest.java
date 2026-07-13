package com.traffic.flink.log.parser;

import com.traffic.proto.traffic.v1.DeviceLog;
import org.junit.jupiter.api.Test;
import static org.junit.jupiter.api.Assertions.*;

/**
 * SyslogParser 单元测试 — RFC 5424 / RFC 3164 解析
 */
public class SyslogParserTest {

    private final SyslogParser parser = new SyslogParser();

    // ==================== RFC 5424 ====================

    @Test
    public void testRfc5424_FullFormat() {
        // <134>1 2024-01-01T00:00:00Z hostname app procid msgid [key="val"] message
        String raw = "<134>1 2024-01-01T00:00:00Z core-sw01 sshd 12345 login [auth@0 user=\"admin\"] Accepted password for admin";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l1").setTenantId("t1")
                .setDeviceIp("10.0.0.1").setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertNotNull(result);
        // 134 / 8 = 16 (local0), 134 % 8 = 6 (info)
        assertEquals(16, result.getFacility());
        assertEquals(6, result.getSeverity());
        assertTrue(result.getParsed().contains("sshd"));
    }

    @Test
    public void testRfc5424_EmergencyPriority() {
        // <0> emergency
        String raw = "<0>1 2024-01-01T00:00:00Z fw01 kernel - - - Firewall crashed";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l2").setTenantId("t1")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertEquals(0, result.getFacility());   // kern
        assertEquals(0, result.getSeverity());   // emerg
    }

    @Test
    public void testRfc5424_DebugPriority() {
        // <191> debug (local7.debug)
        String raw = "<191>1 2024-01-01T00:00:00Z srv01 app - - - Debug message";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l3").setTenantId("t1")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertEquals(23, result.getFacility());  // local7
        assertEquals(7, result.getSeverity());   // debug
    }

    // ==================== RFC 3164 ====================

    @Test
    public void testRfc3164_StandardFormat() {
        // <134>Jan  1 00:00:00 hostname message
        String raw = "<134>Jan  1 00:00:00 core-sw01 Interface GigabitEthernet0/1 is up";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l4").setTenantId("t1")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertNotNull(result);
        assertEquals(16, result.getFacility());
        assertEquals(6, result.getSeverity());
        assertEquals("switch", result.getDeviceType());
    }

    @Test
    public void testRfc3164_RouterHostname() {
        String raw = "<129>Feb 14 08:30:45 rt-core-01 OSPF neighbor state changed";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l5").setTenantId("t1")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertEquals("router", result.getDeviceType());
        assertEquals(16, result.getFacility()); // local0
        assertEquals(1, result.getSeverity());  // alert
    }

    // ==================== Fallback PRI-only ====================

    @Test
    public void testPriOnly_UnrecognizedFormat() {
        // Unrecognized format with PRI prefix
        String raw = "<192> custom app log entry here";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l6").setTenantId("t1")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertNotNull(result);
        assertEquals(24, result.getFacility()); // local7+1 = daemon... wait, 192/8=24
        assertEquals(0, result.getSeverity());   // 192%8=0
    }

    // ==================== Device Type Inference ====================

    @Test
    public void testDeviceType_Switch() {
        String raw = "<134>Jan  1 00:00:00 access-sw-03 Port security violation";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l7")
                .setMessage(raw).build();
        assertEquals("switch", parser.map(input).getDeviceType());
    }

    @Test
    public void testDeviceType_Firewall() {
        String raw = "<131>Jan  1 00:00:00 ngfw-cluster-01 Connection table full";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l8")
                .setMessage(raw).build();
        assertEquals("firewall", parser.map(input).getDeviceType());
    }

    @Test
    public void testDeviceType_Server() {
        String raw = "<134>Jan  1 00:00:00 app-srv-01 Disk usage 95%";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l9")
                .setMessage(raw).build();
        assertEquals("server", parser.map(input).getDeviceType());
    }

    @Test
    public void testDeviceType_Wireless() {
        String raw = "<134>Jan  1 00:00:00 corp-ap-05 Client disassociated";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l10")
                .setMessage(raw).build();
        assertEquals("wireless", parser.map(input).getDeviceType());
    }

    @Test
    public void testDeviceType_Unknown() {
        String raw = "<134>Jan  1 00:00:00 mysterious-box CPU high";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l11")
                .setMessage(raw).build();
        assertEquals("network_device", parser.map(input).getDeviceType());
    }

    // ==================== Edge Cases ====================

    @Test
    public void testNullMessage() {
        DeviceLog input = DeviceLog.newBuilder().setLogId("l12").setTenantId("t1").build();
        DeviceLog result = parser.map(input);
        assertNotNull(result);
        assertEquals("l12", result.getLogId()); // unchanged
    }

    @Test
    public void testEmptyMessage() {
        DeviceLog input = DeviceLog.newBuilder().setLogId("l13").setTenantId("t1")
                .setMessage("").build();
        DeviceLog result = parser.map(input);
        assertNotNull(result);
        assertEquals("l13", result.getLogId());
    }

    @Test
    public void testMalformedInput() {
        String raw = "garbage without pri brackets";
        DeviceLog input = DeviceLog.newBuilder().setLogId("l14")
                .setMessage(raw).build();
        DeviceLog result = parser.map(input);
        assertNotNull(result); // should not throw, return original
        assertEquals("garbage without pri brackets", result.getMessage());
    }
}
