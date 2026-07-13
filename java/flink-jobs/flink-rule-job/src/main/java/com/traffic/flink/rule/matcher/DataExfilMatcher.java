package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Optional;

/**
 * 数据外泄检测匹配器
 * 
 * 检测特征：
 * 1. 大量上行数据
 * 2. 高上下行比（上传远大于下载）
 * 3. 相对短的持续时间（快速大量传输）
 */
public class DataExfilMatcher implements RuleMatcher {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(DataExfilMatcher.class);

    // 默认阈值
    private static final long DEFAULT_MIN_BPS = 10_000_000; // 10 Mbps
    private static final float DEFAULT_MIN_UP_DOWN_RATIO = 10.0f;
    private static final int DEFAULT_MAX_DURATION_MS = 300_000; // 5 分钟

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        if (rule.getType() != RuleType.DATA_EXFIL) {
            return Optional.empty();
        }

        // 获取阈值
        double minBps = rule.getConditionAsDouble("min_bps", DEFAULT_MIN_BPS);
        float minUpDownRatio = (float) rule.getConditionAsDouble("min_up_down_ratio", DEFAULT_MIN_UP_DOWN_RATIO);
        int maxDurationMs = rule.getConditionAsInt("max_duration_ms", DEFAULT_MAX_DURATION_MS);

        // 获取实际值
        float bps = feature.getBps();
        float upDownRatio = feature.getUpDownRatio();
        int durationMs = feature.getDurationMs();

        // 检测条件
        boolean highBps = bps >= minBps;
        boolean highRatio = upDownRatio >= minUpDownRatio;
        boolean quickTransfer = durationMs > 0 && durationMs <= maxDurationMs;

        if (highBps && highRatio && quickTransfer) {
            return Optional.of(DetectionResult.builder()
                    .ruleId(rule.getRuleId())
                    .ruleName(rule.getName())
                    .ruleType(RuleType.DATA_EXFIL)
                    .severity(rule.getSeverity())
                    .labels(rule.getLabels())
                    .score(calculateScore(bps, upDownRatio, minBps, minUpDownRatio))
                    .addEvidence("bps", String.valueOf(bps))
                    .addEvidence("up_down_ratio", String.valueOf(upDownRatio))
                    .addEvidence("duration_ms", String.valueOf(durationMs))
                    .addEvidence("estimated_bytes", String.valueOf((long) (bps / 8 * durationMs / 1000)))
                    .build());
        }

        return Optional.empty();
    }

    /**
     * 计算检测分数
     */
    private float calculateScore(float bps, float upDownRatio, double minBps, float minUpDownRatio) {
        // BPS 倍数
        float bpsScore = Math.min((float) (bps / minBps), 10.0f) / 10.0f;
        
        // 上下行比倍数
        float ratioScore = Math.min(upDownRatio / minUpDownRatio, 10.0f) / 10.0f;
        
        // 综合分数
        return (bpsScore + ratioScore) / 2;
    }
}