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

class BruteForceMatcherTest {

    private BruteForceMatcher matcher;
    private MatchContext context;
    private Rule bruteForceRule;

    @BeforeEach
    void setUp() {
        matcher = new BruteForceMatcher();
        context = new MatchContext();
        context.setProtocol(6); // TCP
        context.setDstPort(22); // SSH

        bruteForceRule = new Rule();
        bruteForceRule.setRuleId("rule-bruteforce-001");
        bruteForceRule.setTenantId("tenant-1");
        bruteForceRule.setName("SSH Brute Force Detection");
        bruteForceRule.setRuleTypeStr("brute_force");
        bruteForceRule.setEnabled(true);
        bruteForceRule.setSeverityStr("critical");
        bruteForceRule.setLabels(Arrays.asList("brute-force", "ssh"));

        Map<String, Object> conditions = new HashMap<>();
        conditions.put("min_pps", 50);
        conditions.put("min_syn_cnt", 20);
        conditions.put("max_duration_ms", 60000);
        conditions.put("target_ports", "22,3389,21");
        conditions.put("min_conditions", 3);
        bruteForceRule.setConditions(conditions);
    }

    @Test
    @DisplayName("检测典型 SSH 暴力破解特征")
    void testDetectSSHBruteForce() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setProtocol(6)
                .setPps(100)           // 高 PPS
                .setTcpFlagSynCnt(50)  // 大量 SYN
                .setDurationMs(30000)  // 30 秒
                .build();

        Optional<DetectionResult> result = matcher.match(feature, bruteForceRule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getRuleType()).isEqualTo(RuleType.BRUTE_FORCE);
        assertThat(result.get().getScore()).isGreaterThan(0.5f);
        assertThat(result.get().getEvidence()).containsEntry("target_service", "SSH");
    }

    @Test
    @DisplayName("正常 SSH 连接不触发检测")
    void testNormalSSHNotDetected() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setProtocol(6)
                .setPps(5)             // 低 PPS
                .setTcpFlagSynCnt(2)   // 正常 SYN
                .setDurationMs(300000) // 5 分钟
                .build();

        Optional<DetectionResult> result = matcher.match(feature, bruteForceRule, context);

        assertThat(result).isEmpty();
    }

    @Test
    @DisplayName("RDP 端口暴力破解检测")
    void testRDPBruteForce() {
        context.setDstPort(3389); // RDP

        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .build())
                .setProtocol(6)
                .setPps(80)
                .setTcpFlagSynCnt(30)
                .setDurationMs(40000)
                .build();

        Optional<DetectionResult> result = matcher.match(feature, bruteForceRule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getEvidence()).containsEntry("target_service", "RDP");
    }

    @Test
    @DisplayName("非目标端口不触发检测")
    void testNonTargetPortNotDetected() {
        context.setDstPort(80); // HTTP（非目标端口）

        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .build())
                .setProtocol(6)
                .setPps(100)           // ✓ matches min_pps=50
                .setTcpFlagSynCnt(50)  // ✓ matches min_syn_cnt=20
                .setDurationMs(120000) // ✗ exceeds max_duration_ms=60000
                .build();

        Optional<DetectionResult> result = matcher.match(feature, bruteForceRule, context);

        // PPS + SYN match, but port=80 is not target and duration=120s is too long.
        // Only 2 of 4 conditions match, below min_conditions=3.
        assertThat(result).isEmpty();
    }
}