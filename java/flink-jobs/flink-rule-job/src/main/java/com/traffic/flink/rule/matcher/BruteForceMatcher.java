package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Optional;

/**
 * 暴力破解检测匹配器
 * 
 * 检测特征：
 * 1. 高频连接尝试（高 PPS）
 * 2. 大量 SYN 或认证失败（通过 TCP Flags 或应用层特征）
 * 3. 短持续时间（快速尝试多次）
 * 4. 特定端口（SSH/RDP/FTP/SMTP/POP3/IMAP）
 */
public class BruteForceMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BruteForceMatcher.class);

    // 默认阈值
    private static final int DEFAULT_MIN_PPS = 50;
    private static final int DEFAULT_MIN_SYN_CNT = 20;
    private static final int DEFAULT_MAX_DURATION_MS = 60000; // 1 分钟
    private static final int[] DEFAULT_BRUTE_FORCE_PORTS = {22, 3389, 21, 25, 110, 143, 1433, 3306, 5432};

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.BRUTE_FORCE) {
            return Optional.empty();
        }

        // 获取阈值
        int minPps = rule.getConditionAsInt("min_pps", DEFAULT_MIN_PPS);
        int minSynCnt = rule.getConditionAsInt("min_syn_cnt", DEFAULT_MIN_SYN_CNT);
        int maxDurationMs = rule.getConditionAsInt("max_duration_ms", DEFAULT_MAX_DURATION_MS);
        
        // 获取目标端口列表
        int[] targetPorts = getTargetPorts(rule);

        // 获取实际值
        float pps = feature.getPps();
        int synCnt = feature.getTcpFlagSynCnt();
        int durationMs = feature.getDurationMs();
        int dstPort = context.getDstPort();

        // 检测条件
        boolean highPps = pps >= minPps;
        boolean highSynCount = synCnt >= minSynCnt;
        boolean shortDuration = durationMs > 0 && durationMs <= maxDurationMs;
        boolean targetPort = isTargetPort(dstPort, targetPorts);

        // 计算匹配条件数
        int matchedConditions = 0;
        int totalConditions = 4;

        if (highPps) matchedConditions++;
        if (highSynCount) matchedConditions++;
        if (shortDuration) matchedConditions++;
        if (targetPort) matchedConditions++;

        // 需要至少 3 个条件匹配
        int minMatchRequired = rule.getConditionAsInt("min_conditions", 3);
        
        if (matchedConditions >= minMatchRequired) {
            float score = (float) matchedConditions / totalConditions;

            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.BRUTE_FORCE)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score(score)
                    .addEvidence("pps", String.format("%.2f", pps))
                    .addEvidence("syn_count", String.valueOf(synCnt))
                    .addEvidence("duration_ms", String.valueOf(durationMs))
                    .addEvidence("dst_port", String.valueOf(dstPort))
                    .addEvidence("matched_conditions", matchedConditions + "/" + totalConditions)
                    .addEvidence("detection_type", "brute_force")
                    .addEvidence("target_service", getServiceName(dstPort))
                    .build());
        }

        return Optional.empty();
    }

    /**
     * 获取目标端口列表
     */
    private int[] getTargetPorts(Rule rule) {
        // 尝试从规则条件中获取
        String portsStr = rule.getConditionAsString("target_ports", "");
        if (!portsStr.isEmpty()) {
            String[] parts = portsStr.split(",");
            int[] ports = new int[parts.length];
            for (int i = 0; i < parts.length; i++) {
                try {
                    ports[i] = Integer.parseInt(parts[i].trim());
                } catch (NumberFormatException e) {
                    LOG.warn("Invalid port in target_ports: {}", parts[i]);
                }
            }
            return ports;
        }
        
        // 使用默认端口
        return DEFAULT_BRUTE_FORCE_PORTS;
    }

    /**
     * 检查是否为目标端口
     */
    private boolean isTargetPort(int port, int[] targetPorts) {
        if (port == 0) {
            return false; // 端口未知
        }
        
        for (int targetPort : targetPorts) {
            if (port == targetPort) {
                return true;
            }
        }
        return false;
    }

    /**
     * 获取服务名称
     */
    private String getServiceName(int port) {
        switch (port) {
            case 22: return "SSH";
            case 3389: return "RDP";
            case 21: return "FTP";
            case 25: return "SMTP";
            case 110: return "POP3";
            case 143: return "IMAP";
            case 1433: return "MSSQL";
            case 3306: return "MySQL";
            case 5432: return "PostgreSQL";
            default: return "Unknown";
        }
    }
}