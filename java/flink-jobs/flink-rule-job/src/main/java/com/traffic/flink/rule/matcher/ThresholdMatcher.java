package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Optional;

/**
 * 阈值匹配器
 * 
 * 支持以下特征的阈值检测：
 * - pps: 包速率
 * - bps: 比特率
 * - up_down_ratio: 上下行比
 * - pktlen_mean: 平均包长
 * - iat_mean_ms: 平均到达间隔
 * - duration_ms: 持续时间
 */
public class ThresholdMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ThresholdMatcher.class);

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.THRESHOLD) {
            return Optional.empty();
        }

        String targetFeature = rule.getConditionAsString("feature", "");
        String operator = rule.getConditionAsString("operator", ">");
        double threshold = rule.getConditionAsDouble("value", 0.0);

        if (targetFeature.isEmpty()) {
            LOG.warn("Threshold rule {} has no feature specified", rule.getRuleId());
            return Optional.empty();
        }

        // 获取特征值
        double actualValue = getFeatureValue(feature, targetFeature);

        // 比较
        boolean matched = compare(actualValue, operator, threshold);

        if (matched) {
            // 计算分数：基于超出阈值的程度
            float score = calculateScore(actualValue, operator, threshold);

            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.THRESHOLD)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score(score)
                    .addEvidence("feature", targetFeature)
                    .addEvidence("actual_value", String.format("%.2f", actualValue))
                    .addEvidence("operator", operator)
                    .addEvidence("threshold", String.format("%.2f", threshold))
                    .addEvidence("exceeded_by", String.format("%.2f%%", 
                            calculateExceededPercentage(actualValue, operator, threshold)))
                    .build());
        }

        return Optional.empty();
    }

    /**
     * 获取特征值
     */
    private double getFeatureValue(FeatureStat feature, String featureName) {
        switch (featureName.toLowerCase()) {
            case "pps":
                return feature.getPps();
            case "bps":
                return feature.getBps();
            case "up_down_ratio":
                return feature.getUpDownRatio();
            case "pktlen_mean":
                return feature.getPktlenMean();
            case "pktlen_std":
                return feature.getPktlenStd();
            case "iat_mean_ms":
                return feature.getIatMeanMs();
            case "iat_std_ms":
                return feature.getIatStdMs();
            case "duration_ms":
                return feature.getDurationMs();
            case "active_mean_ms":
                return feature.getActiveMeanMs();
            case "idle_mean_ms":
                return feature.getIdleMeanMs();
            case "tcp_flag_syn_cnt":
                return feature.getTcpFlagSynCnt();
            case "tcp_flag_ack_cnt":
                return feature.getTcpFlagAckCnt();
            case "protocol":
                return feature.getProtocol();
            default:
                LOG.warn("Unknown feature: {}", featureName);
                return 0.0;
        }
    }

    /**
     * 比较操作
     */
    private boolean compare(double actual, String operator, double threshold) {
        switch (operator) {
            case ">":
                return actual > threshold;
            case "<":
                return actual < threshold;
            case ">=":
                return actual >= threshold;
            case "<=":
                return actual <= threshold;
            case "==":
            case "=":
                return Math.abs(actual - threshold) < 0.0001;
            case "!=":
            case "<>":
                return Math.abs(actual - threshold) >= 0.0001;
            default:
                LOG.warn("Unknown operator: {}", operator);
                return false;
        }
    }

    /**
     * 计算分数
     */
    private float calculateScore(double actual, String operator, double threshold) {
        if (threshold == 0) {
            return 0.8f;
        }

        double ratio;
        switch (operator) {
            case ">":
            case ">=":
                ratio = actual / threshold;
                break;
            case "<":
            case "<=":
                ratio = threshold / actual;
                break;
            default:
                return 0.5f;
        }

        // 超出阈值越多，分数越高
        // 超出 2 倍以上，分数为 1.0
        return Math.min(1.0f, (float) (0.5 + (ratio - 1.0) * 0.5));
    }

    /**
     * 计算超出阈值的百分比
     */
    private double calculateExceededPercentage(double actual, String operator, double threshold) {
        if (threshold == 0) {
            return actual * 100;
        }

        switch (operator) {
            case ">":
            case ">=":
                return ((actual - threshold) / threshold) * 100;
            case "<":
            case "<=":
                return ((threshold - actual) / threshold) * 100;
            default:
                return 0;
        }
    }
}