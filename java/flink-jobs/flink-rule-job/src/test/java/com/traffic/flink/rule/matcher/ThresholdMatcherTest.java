package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import java.util.*;

import static org.assertj.core.api.Assertions.assertThat;

class ThresholdMatcherTest {

    private ThresholdMatcher matcher;
    private MatchContext context;

    @BeforeEach
    void setUp() {
        matcher = new ThresholdMatcher();
        context = new MatchContext();
    }

    private Rule createThresholdRule(String feature, String operator, double value) {
        Rule rule = new Rule();
        rule.setRuleId("rule-threshold-test");
        rule.setTenantId("tenant-1");
        rule.setName("Test Threshold");
        rule.setType(RuleType.THRESHOLD);
        rule.setEnabled(true);
        rule.setSeverity("high");
        rule.setLabels(Arrays.asList("anomaly"));

        Map<String, Object> conditions = new HashMap<>();
        conditions.put("feature", feature);
        conditions.put("operator", operator);
        conditions.put("value", value);
        rule.setConditions(conditions);

        return rule;
    }

    private FeatureStat createFeature(float pps, float bps, float upDownRatio) {
        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setPps(pps)
                .setBps(bps)
                .setUpDownRatio(upDownRatio)
                .build();
    }

    @Test
    @DisplayName("PPS 超过阈值")
    void testPpsExceedsThreshold() {
        Rule rule = createThresholdRule("pps", ">", 10000);
        FeatureStat feature = createFeature(50000, 0, 0);

        Optional<DetectionResult> result = matcher.match(feature, rule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getEvidence()).containsEntry("feature", "pps");
        assertThat(result.get().getEvidence()).containsKey("exceeded_by");
    }

    @Test
    @DisplayName("PPS 未超过阈值")
    void testPpsBelowThreshold() {
        Rule rule = createThresholdRule("pps", ">", 10000);
        FeatureStat feature = createFeature(5000, 0, 0);

        Optional<DetectionResult> result = matcher.match(feature, rule, context);

        assertThat(result).isEmpty();
    }

    @ParameterizedTest
    @DisplayName("测试各种比较运算符")
    @CsvSource({
            ">,  10000, 15000, true",
            ">,  10000, 5000,  false",
            "<,  10000, 5000,  true",
            "<,  10000, 15000, false",
            ">=, 10000, 10000, true",
            ">=, 10000, 9999,  false",
            "<=, 10000, 10000, true",
            "<=, 10000, 10001, false",
            "==, 10000, 10000, true",
            "==, 10000, 10001, false"
    })
    void testOperators(String operator, double threshold, float actualPps, boolean shouldMatch) {
        Rule rule = createThresholdRule("pps", operator, threshold);
        FeatureStat feature = createFeature(actualPps, 0, 0);

        Optional<DetectionResult> result = matcher.match(feature, rule, context);

        assertThat(result.isPresent()).isEqualTo(shouldMatch);
    }

    @Test
    @DisplayName("测试分数计算 - 超出阈值越多分数越高")
    void testScoreCalculation() {
        Rule rule = createThresholdRule("pps", ">", 10000);

        // 刚超过阈值
        FeatureStat feature1 = createFeature(11000, 0, 0);
        Optional<DetectionResult> result1 = matcher.match(feature1, rule, context);
        assertThat(result1).isPresent();
        float score1 = result1.get().getScore();

        // 大幅超过阈值
        FeatureStat feature2 = createFeature(50000, 0, 0);
        Optional<DetectionResult> result2 = matcher.match(feature2, rule, context);
        assertThat(result2).isPresent();
        float score2 = result2.get().getScore();

        assertThat(score2).isGreaterThan(score1);
    }
}