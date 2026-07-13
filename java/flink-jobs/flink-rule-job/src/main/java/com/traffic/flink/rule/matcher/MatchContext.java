package com.traffic.flink.rule.matcher;

import com.google.common.hash.BloomFilter;

import java.io.Serializable;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

/**
 * 规则匹配上下文（增强版）
 * 
 * 存储匹配过程中需要的共享数据
 * 
 * 新增字段：
 * - srcPort, dstPort（支持端口匹配）
 */
public class MatchContext implements Serializable {

    private static final long serialVersionUID = 1L;

    // IP 黑名单（按租户隔离）
    private Map<String, Set<String>> ipBlacklists = new HashMap<>();

    // BloomFilter（按租户隔离，用于快速过滤）
    private transient Map<String, BloomFilter<String>> bloomFilters = new HashMap<>();

    // 当前事件的网络信息（从 FeatureStat 提取）
    private String srcIp;
    private String dstIp;
    private int srcPort;
    private int dstPort;
    private int protocol;

    // 当前事件的租户信息
    private String tenantId;

    // 当前事件的时间戳
    private long timestamp;

    // ==================== Getters & Setters ====================

    public Map<String, Set<String>> getIpBlacklists() {
        return ipBlacklists;
    }

    public void setIpBlacklists(Map<String, Set<String>> ipBlacklists) {
        this.ipBlacklists = ipBlacklists;
    }

    public Set<String> getIpBlacklist(String tenantId) {
        return ipBlacklists.get(tenantId);
    }

    public Map<String, BloomFilter<String>> getBloomFilters() {
        if (bloomFilters == null) {
            bloomFilters = new HashMap<>();
        }
        return bloomFilters;
    }

    public BloomFilter<String> getBloomFilter(String tenantId) {
        if (bloomFilters == null) {
            bloomFilters = new HashMap<>();
        }
        return bloomFilters.get(tenantId);
    }

    public void setBloomFilter(String tenantId, BloomFilter<String> filter) {
        if (bloomFilters == null) {
            bloomFilters = new HashMap<>();
        }
        bloomFilters.put(tenantId, filter);
    }

    /**
     * 设置某租户的 IP 黑名单（测试友好 API）
     */
    public void setIpBlacklist(String tenantId, Set<String> blacklist) {
        if (this.ipBlacklists == null) {
            this.ipBlacklists = new HashMap<>();
        }
        this.ipBlacklists.put(tenantId, blacklist);
    }

    public String getSrcIp() {
        return srcIp;
    }

    public void setSrcIp(String srcIp) {
        this.srcIp = srcIp;
    }

    public String getDstIp() {
        return dstIp;
    }

    public void setDstIp(String dstIp) {
        this.dstIp = dstIp;
    }

    public int getSrcPort() {
        return srcPort;
    }

    public void setSrcPort(int srcPort) {
        this.srcPort = srcPort;
    }

    public int getDstPort() {
        return dstPort;
    }

    public void setDstPort(int dstPort) {
        this.dstPort = dstPort;
    }

    public int getProtocol() {
        return protocol;
    }

    public void setProtocol(int protocol) {
        this.protocol = protocol;
    }

    public String getTenantId() {
        return tenantId;
    }

    public void setTenantId(String tenantId) {
        this.tenantId = tenantId;
    }

    public long getTimestamp() {
        return timestamp;
    }

    public void setTimestamp(long timestamp) {
        this.timestamp = timestamp;
    }

    // 缓存: TCP flags → attack type
    private transient Map<Integer, String> tcpFlagAttackTypeCache = new HashMap<>();

    /** 获取缓存的 TCP flags 攻击类型 */
    public String getCachedAttackType(int flags) {
        if (tcpFlagAttackTypeCache == null) {
            tcpFlagAttackTypeCache = new HashMap<>();
        }
        return tcpFlagAttackTypeCache.get(flags);
    }

    /** 缓存 TCP flags 攻击类型 */
    public void cacheAttackType(int flags, String attackType) {
        if (tcpFlagAttackTypeCache == null) {
            tcpFlagAttackTypeCache = new HashMap<>();
        }
        tcpFlagAttackTypeCache.put(flags, attackType);
    }

    // ==================== 辅助方法 ====================

    /**
     * 清空当前事件的上下文信息
     */
    public void clearEventContext() {
        this.srcIp = null;
        this.dstIp = null;
        this.srcPort = 0;
        this.dstPort = 0;
        this.protocol = 0;
        this.tenantId = null;
        this.timestamp = 0;
    }
}