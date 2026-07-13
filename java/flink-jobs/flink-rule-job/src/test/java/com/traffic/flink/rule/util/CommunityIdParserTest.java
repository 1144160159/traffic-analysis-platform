package com.traffic.flink.rule.util;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class CommunityIdParserTest {

    @Test
    @DisplayName("解析有效的 objectId（IPv4）")
    void testParseValidObjectIdIPv4() {
        String objectId = "192.168.1.1:443-10.0.0.1:52345";
        
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(objectId);
        
        assertThat(tuple).isNotNull();
        assertThat(tuple.srcIp).isEqualTo("192.168.1.1");
        assertThat(tuple.srcPort).isEqualTo(443);
        assertThat(tuple.dstIp).isEqualTo("10.0.0.1");
        assertThat(tuple.dstPort).isEqualTo(52345);
    }

    @Test
    @DisplayName("解析有效的 objectId（IPv6）")
    void testParseValidObjectIdIPv6() {
        String objectId = "2001:db8::1:80-2001:db8::2:443";
        
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(objectId);
        
        assertThat(tuple).isNotNull();
        assertThat(tuple.srcIp).isEqualTo("2001:db8::1");
        assertThat(tuple.srcPort).isEqualTo(80);
        assertThat(tuple.dstIp).isEqualTo("2001:db8::2");
        assertThat(tuple.dstPort).isEqualTo(443);
    }

    @Test
    @DisplayName("解析无效的 objectId（缺少端口）")
    void testParseInvalidObjectIdMissingPort() {
        String objectId = "192.168.1.1-10.0.0.1";
        
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(objectId);
        
        assertThat(tuple).isNull();
    }

    @Test
    @DisplayName("解析无效的 objectId（格式错误）")
    void testParseInvalidObjectIdBadFormat() {
        String objectId = "invalid-format";
        
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(objectId);
        
        assertThat(tuple).isNull();
    }

    @Test
    @DisplayName("解析空字符串")
    void testParseEmptyString() {
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId("");
        
        assertThat(tuple).isNull();
    }

    @Test
    @DisplayName("解析 null")
    void testParseNull() {
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(null);
        
        assertThat(tuple).isNull();
    }

    @Test
    @DisplayName("Community ID 无法解析（返回 null）")
    void testParseCommunityIdReturnsNull() {
        String communityId = "1:abc123==";
        
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseCommunityId(communityId);
        
        assertThat(tuple).isNull();
    }
}