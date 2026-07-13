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

class PortScanMatcherTest {

    private PortScanMatcher matcher;
    private MatchContext context;
    private Rule portScanRule;

    @BeforeEach
    void setUp() {
        matcher = new PortScanMatcher();
        context = new MatchContext();
        context.setProtocol(6); // TCP

        portScanRule = new Rule();
        portScanRule.setRuleId("rule-portscan-001");
        portScanRule.setTenantId("tenant-1");
        portScanRule.setName("Port Scan Detection");
        portScanRule.setType(RuleType.PORT_SCAN);
        portScanRule.setEnabled(true);
        portScanRule.setSeverity("high");
        portScanRule.setLabels(Arrays.asList("scan", "reconnaissance"));

        Map<String, Object> conditions = new HashMap<>();
        conditions.put("min_pps", 100);
        conditions.put("max_pkt_len", 100);
        conditions.put("min_syn_cnt", 10);
        conditions.put("max_duration_ms", 60000);
        conditions.put("min_conditions", 3);
        portScanRule.setConditions(conditions);
    }

    @Test
    @DisplayName("检测典型端口扫描特征")
    void testDetectPortScan() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setProtocol(6)
                .setPps(500)        // 高 PPS
                .setPktlenMean(64)  // 小包
                .setTcpFlagSynCnt(50) // 大量 SYN
                .setDurationMs(5000)  // 短持续时间
                .build();

        Optional<DetectionResult> result = matcher.match(feature, portScanRule, context);

        assertThat(result).isPresent();
        assertThat(result.get().getRuleType()).isEqualTo(RuleType.PORT_SCAN);
        assertThat(result.get().getScore()).isGreaterThan(0.5f);
    }

    @Test
    @DisplayName("正常流量不触发检测")
    void testNormalTrafficNotDetected() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setCommunityId("1:abc123==")
                .setProtocol(6)
                .setPps(10)         // 低 PPS
                .setPktlenMean(500) // 正常包大小
                .setTcpFlagSynCnt(2)  // 少量 SYN
                .setDurationMs(300000) // 长持续时间
                .build();

        Optional<DetectionResult> result = matcher.match(feature, portScanRule, context);

        assertThat(result).isEmpty();
    }

    @Test
    @DisplayName("UDP 流量不检测端口扫描")
    void testUdpNotChecked() {
        context.setProtocol(17); // UDP

        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .build())
                .setProtocol(17)
                .setPps(500)
                .setPktlenMean(64)
                .setTcpFlagSynCnt(50)
                .setDurationMs(5000)
                .build();

        Optional<DetectionResult> result = matcher.match(feature, portScanRule, context);

        assertThat(result).isEmpty();
    }
}