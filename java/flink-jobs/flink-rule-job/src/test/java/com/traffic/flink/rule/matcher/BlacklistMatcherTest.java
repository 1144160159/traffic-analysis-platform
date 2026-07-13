package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.*;

import static org.assertj.core.api.Assertions.assertThat;

class BlacklistMatcherTest {

    private BlacklistMatcher matcher;
    private MatchContext context;
    private Rule blacklistRule;
    private FeatureStat feature;

    @BeforeEach
    void setUp() {
        matcher = new BlacklistMatcher();
        context = new MatchContext();

        // 设置 IP 黑名单
        Set<String> blacklist = new HashSet<>(Arrays.asList(
                "192.168.100.100",
                "10.0.0.99",
                "172.16.0.50"
        ));
        context.setIpBlacklist("tenant-1", blacklist);

        // 创建规则
        blacklistRule = new Rule();
        blacklistRule.setRuleId("rule-blacklist-001");
        blacklistRule.setTenantId("tenant-1");
        blacklistRule.setName("Test Blacklist");
        blacklistRule.setType(RuleType.BLACKLIST);
        blacklistRule.setEnabled(true);
        blacklistRule.setSeverity("critical");
        blacklistRule.setLabels(Arrays.asList("malware", "c2"));

        Map<String, Object> conditions = new HashMap<>();
        conditions.put("direction", "both");
        conditions.put("ip_list", new ArrayList<>(blacklist));
        blacklistRule.setConditions(conditions);

        // 创建特征
        feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setObjectId("session-1")
                .build();
    }

    @Test
    @DisplayName("匹配黑名单中的源 IP")
    void testMatchBlacklistedSrcIp() {
        context.setSrcIp("192.168.100.100");
        context.setDstIp("8.8.8.8");

        Optional<DetectionResult> result = matcher.match(feature, blacklistRule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getRuleId()).isEqualTo("rule-blacklist-001");
        assertThat(result.get().getScore()).isEqualTo(1.0f);
        assertThat(result.get().getEvidence()).containsEntry("matched_ip", "192.168.100.100");
        assertThat(result.get().getEvidence()).containsEntry("direction", "source");
    }

    @Test
    @DisplayName("匹配黑名单中的目标 IP")
    void testMatchBlacklistedDstIp() {
        context.setSrcIp("1.1.1.1");
        context.setDstIp("10.0.0.99");

        Optional<DetectionResult> result = matcher.match(feature, blacklistRule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getEvidence()).containsEntry("matched_ip", "10.0.0.99");
        assertThat(result.get().getEvidence()).containsEntry("direction", "destination");
    }

    @Test
    @DisplayName("不匹配非黑名单 IP")
    void testNoMatchNonBlacklisted() {
        context.setSrcIp("1.1.1.1");
        context.setDstIp("8.8.8.8");

        Optional<DetectionResult> result = matcher.match(feature, blacklistRule, context);

        assertThat(result).isEmpty();
    }

    @Test
    @DisplayName("只检查源 IP 方向")
    void testSrcDirectionOnly() {
        Map<String, Object> conditions = new HashMap<>(blacklistRule.getConditions());
        conditions.put("direction", "src");
        blacklistRule.setConditions(conditions);

        context.setSrcIp("1.1.1.1");
        context.setDstIp("192.168.100.100"); // 在黑名单中，但只检查源

        Optional<DetectionResult> result = matcher.match(feature, blacklistRule, context);

        assertThat(result).isEmpty();
    }

    @Test
    @DisplayName("空黑名单不匹配")
    void testEmptyBlacklist() {
        context.setIpBlacklist("tenant-1", new HashSet<>());
        context.setSrcIp("192.168.100.100");

        Optional<DetectionResult> result = matcher.match(feature, blacklistRule, context);

        assertThat(result).isEmpty();
    }
}