package com.traffic.flink.rule.matcher;

import com.google.common.hash.BloomFilter;
import com.google.common.hash.Funnels;
import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.util.*;

/**
 * IP 黑名单匹配器
 * 
 * 使用 BloomFilter 优化查询性能：
 * - BloomFilter 快速排除非黑名单 IP
 * - HashSet 确认最终匹配
 */
public class BlacklistMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BlacklistMatcher.class);

    // BloomFilter 预期插入数量
    private static final int EXPECTED_INSERTIONS = 100000;
    // BloomFilter 误判率
    private static final double FPP = 0.01;

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.BLACKLIST) {
            return Optional.empty();
        }

        String tenantId = feature.getHeader().getTenantId();
        String direction = rule.getConditionAsString("direction", "both");

        // 获取要检查的 IP
        String srcIp = context.getSrcIp();
        String dstIp = context.getDstIp();

        // 快速路径：使用 BloomFilter 预过滤
        BloomFilter<String> bloomFilter = context.getBloomFilter(tenantId);
        Set<String> blacklist = context.getIpBlacklist(tenantId);

        if (blacklist == null || blacklist.isEmpty()) {
            return Optional.empty();
        }

        String matchedIp = null;
        String matchedDirection = null;

        // 检查源 IP
        if (("src".equals(direction) || "both".equals(direction)) && srcIp != null) {
            if (isBlacklisted(srcIp, bloomFilter, blacklist)) {
                matchedIp = srcIp;
                matchedDirection = "source";
            }
        }

        // 检查目标 IP
        if (matchedIp == null && ("dst".equals(direction) || "both".equals(direction)) && dstIp != null) {
            if (isBlacklisted(dstIp, bloomFilter, blacklist)) {
                matchedIp = dstIp;
                matchedDirection = "destination";
            }
        }

        if (matchedIp != null) {
            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.BLACKLIST)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score(1.0f) // 黑名单匹配置信度为 1.0
                    .addEvidence("matched_ip", matchedIp)
                    .addEvidence("direction", matchedDirection)
                    .addEvidence("rule_name", rule.getName())
                    .addEvidence("blacklist_size", String.valueOf(blacklist.size()))
                    .build());
        }

        return Optional.empty();
    }

    /**
     * 检查 IP 是否在黑名单中
     */
    private boolean isBlacklisted(String ip, BloomFilter<String> bloomFilter, Set<String> blacklist) {
        if (ip == null || ip.isEmpty()) {
            return false;
        }

        // 如果有 BloomFilter，先进行快速检查
        if (bloomFilter != null) {
            if (!bloomFilter.mightContain(ip)) {
                return false; // BloomFilter 说不存在，一定不存在
            }
        }

        // BloomFilter 说可能存在，用 HashSet 确认
        return blacklist.contains(ip);
    }

    /**
     * 从规则更新黑名单和 BloomFilter
     */
    public static void updateBlacklist(Rule rule, MatchContext context) {
        String tenantId = rule.getTenantId();
        List<String> ipList = rule.getConditionAsList("ip_list");

        if (ipList == null || ipList.isEmpty()) {
            LOG.warn("Blacklist rule {} has no IP list", rule.getRuleId());
            return;
        }

        // 获取或创建黑名单
        Set<String> blacklist = context.getIpBlacklists()
                .computeIfAbsent(tenantId, k -> new HashSet<>());

        // 添加 IP
        blacklist.addAll(ipList);

        // 重建 BloomFilter
        rebuildBloomFilter(tenantId, blacklist, context);

        LOG.info("Updated blacklist for tenant {}: {} IPs (total: {})", 
                tenantId, ipList.size(), blacklist.size());
    }

    /**
     * 从黑名单移除 IP
     */
    public static void removeFromBlacklist(Rule rule, MatchContext context) {
        String tenantId = rule.getTenantId();
        List<String> ipList = rule.getConditionAsList("ip_list");

        if (ipList == null || ipList.isEmpty()) {
            return;
        }

        Set<String> blacklist = context.getIpBlacklist(tenantId);
        if (blacklist != null) {
            blacklist.removeAll(ipList);
            
            // 重建 BloomFilter（因为 BloomFilter 不支持删除）
            rebuildBloomFilter(tenantId, blacklist, context);
            
            LOG.info("Removed {} IPs from blacklist for tenant {}", ipList.size(), tenantId);
        }
    }

    /**
     * 重建 BloomFilter
     */
    private static void rebuildBloomFilter(String tenantId, Set<String> blacklist, MatchContext context) {
        if (blacklist.isEmpty()) {
            context.getBloomFilters().remove(tenantId);
            return;
        }

        BloomFilter<String> newFilter = BloomFilter.create(
                Funnels.stringFunnel(StandardCharsets.UTF_8),
                Math.max(blacklist.size() * 2, EXPECTED_INSERTIONS),
                FPP
        );

        for (String ip : blacklist) {
            newFilter.put(ip);
        }

        context.setBloomFilter(tenantId, newFilter);
    }
}