package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Optional;

/**
 * 端口扫描检测匹配器
 * 
 * 检测特征：
 * 1. 高 PPS（大量探测包）
 * 2. 低平均包长（TCP SYN 包通常很小）
 * 3. 高 SYN 比例（大量 SYN 包，少量响应）
 * 4. 短持续时间（扫描通常很快完成）
 */
public class PortScanMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(PortScanMatcher.class);

    // 默认阈值
    private static final int DEFAULT_MIN_PPS = 100;
    private static final int DEFAULT_MAX_PKT_LEN = 100;
    private static final int DEFAULT_MIN_SYN_CNT = 10;
    private static final int DEFAULT_MAX_DURATION_MS = 60000;

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.PORT_SCAN) {
            return Optional.empty();
        }

        // 只检查 TCP 流量
        if (context.getProtocol() != 6 && feature.getProtocol() != 6) {
            return Optional.empty();
        }

        // 获取阈值
        int minPps = rule.getConditionAsInt("min_pps", DEFAULT_MIN_PPS);
        int maxPktLen = rule.getConditionAsInt("max_pkt_len", DEFAULT_MAX_PKT_LEN);
        int minSynCnt = rule.getConditionAsInt("min_syn_cnt", DEFAULT_MIN_SYN_CNT);
        int maxDurationMs = rule.getConditionAsInt("max_duration_ms", DEFAULT_MAX_DURATION_MS);

        // 检测条件
        float pps = feature.getPps();
        float pktlenMean = feature.getPktlenMean();
        int synCnt = feature.getTcpFlagSynCnt();
        int durationMs = feature.getDurationMs();

        // 计算匹配分数
        int matchedConditions = 0;
        int totalConditions = 4;

        if (pps >= minPps) {
            matchedConditions++;
        }
        if (pktlenMean <= maxPktLen && pktlenMean > 0) {
            matchedConditions++;
        }
        if (synCnt >= minSynCnt) {
            matchedConditions++;
        }
        if (durationMs <= maxDurationMs && durationMs > 0) {
            matchedConditions++;
        }

        // 需要至少 3 个条件匹配
        int minMatchRequired = rule.getConditionAsInt("min_conditions", 3);
        
        if (matchedConditions >= minMatchRequired) {
            float score = (float) matchedConditions / totalConditions;

            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.PORT_SCAN)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score(score)
                    .addEvidence("pps", String.format("%.2f", pps))
                    .addEvidence("pktlen_mean", String.format("%.2f", pktlenMean))
                    .addEvidence("syn_count", String.valueOf(synCnt))
                    .addEvidence("duration_ms", String.valueOf(durationMs))
                    .addEvidence("matched_conditions", matchedConditions + "/" + totalConditions)
                    .addEvidence("detection_type", "port_scan")
                    .build());
        }

        return Optional.empty();
    }
}