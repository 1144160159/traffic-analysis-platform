package com.traffic.flink.rule.util;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Community ID 解析器
 * 
 * Community ID 格式：1:base64hash
 * 由于 hash 不可逆，无法直接提取 IP/端口
 * 
 * 本解析器采用以下策略：
 * 1. 优先从 FeatureStat.objectId 解析（若格式为 "srcIP:srcPort-dstIP:dstPort"）
 * 2. 否则返回空（需从 Redis/外部缓存查询）
 */
public class CommunityIdParser implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(CommunityIdParser.class);

    // 匹配 objectId 格式：192.168.1.1:443-10.0.0.1:52345
    private static final Pattern OBJECT_ID_PATTERN = Pattern.compile(
            "^([0-9a-fA-F:.]+):(\\d+)-([0-9a-fA-F:.]+):(\\d+)$"
    );

    /**
     * 五元组结果
     */
    public static class FiveTuple implements Serializable {
        private static final long serialVersionUID = 1L;
        
        public final String srcIp;
        public final String dstIp;
        public final int srcPort;
        public final int dstPort;

        public FiveTuple(String srcIp, int srcPort, String dstIp, int dstPort) {
            this.srcIp = srcIp;
            this.srcPort = srcPort;
            this.dstIp = dstIp;
            this.dstPort = dstPort;
        }
    }

    /**
     * 从 objectId 解析五元组
     * 
     * @param objectId 格式如 "192.168.1.1:443-10.0.0.1:52345"
     * @return 五元组，解析失败返回 null
     */
    public static FiveTuple parseObjectId(String objectId) {
        if (objectId == null || objectId.isEmpty()) {
            return null;
        }

        Matcher matcher = OBJECT_ID_PATTERN.matcher(objectId);
        if (!matcher.matches()) {
            LOG.debug("Object ID does not match expected format: {}", objectId);
            return null;
        }

        try {
            String srcIp = matcher.group(1);
            int srcPort = Integer.parseInt(matcher.group(2));
            String dstIp = matcher.group(3);
            int dstPort = Integer.parseInt(matcher.group(4));

            return new FiveTuple(srcIp, srcPort, dstIp, dstPort);
        } catch (NumberFormatException e) {
            LOG.warn("Failed to parse port from objectId: {}", objectId, e);
            return null;
        }
    }

    /**
     * 从 Community ID 提取协议（当前无法实现，返回 null）
     * 
     * Community ID 是不可逆 hash，无法提取原始信息
     * 此方法预留用于未来扩展（如维护 community_id -> 五元组映射缓存）
     */ 
    public static FiveTuple parseCommunityId(String communityId) {
        // Community ID 是 SHA-1 hash，无法逆向解析
        // 需要从外部缓存（Redis）查询
        LOG.debug("Community ID cannot be parsed directly: {}", communityId);
        return null;
    }

}