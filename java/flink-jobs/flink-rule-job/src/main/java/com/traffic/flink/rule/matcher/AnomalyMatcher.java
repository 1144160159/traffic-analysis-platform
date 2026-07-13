package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Optional;

/**
 * 异常检测匹配器
 * 
 * 基于统计偏差检测异常流量
 * 
 * 检测特征：
 * 1. 包长标准差异常高
 * 2. IAT 标准差异常高
 * 3. 上下行比例异常
 */
public class AnomalyMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(AnomalyMatcher.class);

    // 默认阈值
    private static final float DEFAULT_PKTLEN_STD_THRESHOLD = 500.0f;
    private static final float DEFAULT_IAT_STD_THRESHOLD = 100.0f;
    private static final float DEFAULT_EXTREME_UP_DOWN_RATIO = 100.0f;

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.ANOMALY) {
            return Optional.empty();
        }

        // 获取阈值
        float pktlenStdThreshold = (float) rule.getConditionAsDouble("pktlen_std_threshold", DEFAULT_PKTLEN_STD_THRESHOLD);
        float iatStdThreshold = (float) rule.getConditionAsDouble("iat_std_threshold", DEFAULT_IAT_STD_THRESHOLD);
        float extremeUpDownRatio = (float) rule.getConditionAsDouble("extreme_up_down_ratio", DEFAULT_EXTREME_UP_DOWN_RATIO);

        // 获取实际值
        float pktlenStd = feature.getPktlenStd();
        float iatStd = feature.getIatStdMs();
        float upDownRatio = feature.getUpDownRatio();

        // 检测异常
        int anomalyCount = 0;
        StringBuilder anomalyTypes = new StringBuilder();

        if (pktlenStd > pktlenStdThreshold) {
            anomalyCount++;
            anomalyTypes.append("high_pktlen_variance,");
        }

        if (iatStd > iatStdThreshold) {
            anomalyCount++;
            anomalyTypes.append("high_iat_variance,");
        }

        if (upDownRatio > extremeUpDownRatio || (upDownRatio > 0 && upDownRatio < 1.0f / extremeUpDownRatio)) {
            anomalyCount++;
            anomalyTypes.append("extreme_up_down_ratio,");
        }

        // 至少需要 2 种异常才触发
        int minAnomalies = rule.getConditionAsInt("min_anomalies", 2);
        
        if (anomalyCount >= minAnomalies) {
            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.ANOMALY)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score((float) anomalyCount / 3)
                    .addEvidence("anomaly_count", String.valueOf(anomalyCount))
                    .addEvidence("anomaly_types", anomalyTypes.toString())
                    .addEvidence("pktlen_std", String.valueOf(pktlenStd))
                    .addEvidence("iat_std", String.valueOf(iatStd))
                    .addEvidence("up_down_ratio", String.valueOf(upDownRatio))
                    .build());
        }

        return Optional.empty();
    }
}