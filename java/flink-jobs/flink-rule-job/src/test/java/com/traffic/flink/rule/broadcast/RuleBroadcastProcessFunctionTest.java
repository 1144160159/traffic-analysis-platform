package com.traffic.flink.rule.broadcast;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.flink.rule.model.Severity;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;

import org.junit.jupiter.api.Test;

import java.util.Arrays;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class RuleBroadcastProcessFunctionTest {

    private final RuleBroadcastProcessFunction function = new RuleBroadcastProcessFunction();

    @Test
    void detectionCarriesCanonicalEnvelopeTupleEvidenceAndRuleVersion() {
        EventHeader sourceHeader = EventHeader.newBuilder()
                .setEventId("feature-event-1")
                .setTenantId("tenant-1")
                .setRunId("run-1")
                .setEventTs(1_720_000_000_000L)
                .setIngestTs(1_720_000_000_100L)
                .setProbeId("probe-1")
                .setFeatureSetId("feature-set-1")
                .setTraceId("trace-1")
                .setCorrelationId("correlation-1")
                .build();
        FiveTuple tuple = FiveTuple.newBuilder()
                .setSrcIp("192.0.2.10")
                .setDstIp("198.51.100.20")
                .setSrcPort(54321)
                .setDstPort(443)
                .setProtocol(6)
                .build();
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(sourceHeader)
                .setObjectType("flow")
                .setObjectId("flow-1")
                .setCommunityId("1:test-community")
                .setTs(1_720_000_000_000L)
                .setProtocol(6)
                .setTuple(tuple)
                .addEvidenceIds("source-evidence-1")
                .addEvidenceIds("source-evidence-2")
                .build();
        Rule rule = new Rule();
        rule.setRuleId("rule-7");
        rule.setTenantId("tenant-1");
        rule.setType(RuleType.PORT_SCAN);
        rule.setVersion(9);
        rule.setEnabled(true);
        DetectionResult result = DetectionResult.builder()
                .ruleId("rule-7")
                .ruleName("Port scan")
                .ruleType(RuleType.PORT_SCAN)
                .severity(Severity.HIGH)
                .labels(Arrays.asList("scan", "reconnaissance"))
                .score(0.91f)
                .build();

        DetectionBehavior first = function.buildDetectionEvent(feature, result, rule);
        DetectionBehavior replay = function.buildDetectionEvent(feature, result, rule);

        assertThat(replay.getHeader().getEventId()).isEqualTo(first.getHeader().getEventId());
        assertThat(first.getHeader().getTenantId()).isEqualTo("tenant-1");
        assertThat(first.getHeader().getEventType()).isEqualTo("traffic.detection.behavior.v1");
        assertThat(first.getHeader().getSchemaVersion()).isEqualTo("1");
        assertThat(first.getHeader().getAggregateType()).isEqualTo("detection");
        assertThat(first.getHeader().getAggregateId()).isEqualTo("flow-1");
        assertThat(first.getHeader().getAggregateVersion()).isEqualTo(9);
        assertThat(first.getHeader().getTraceId()).isEqualTo("trace-1");
        assertThat(first.getHeader().getCausationId()).isEqualTo("feature-event-1");
        assertThat(first.getHeader().getCorrelationId()).isEqualTo("correlation-1");
        assertThat(first.getHeader().getIdempotencyKey()).isEqualTo(first.getHeader().getEventId());
        assertThat(first.getHeader().getProducer()).isEqualTo("flink-rule-job");
        assertThat(first.getTuple()).isEqualTo(tuple);
        assertThat(first.getEvidenceIdsList())
                .containsExactly("source-evidence-1", "source-evidence-2");
        assertThat(first.getLabelsList())
                .contains("rule_id:rule-7", "rule_version:9", "detection_source:rule");
    }

    @Test
    void unparseableMissingTupleFailsClosed() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId("feature-event-2")
                        .setTenantId("tenant-1")
                        .build())
                .setObjectId("opaque-flow-id")
                .setCommunityId("1:one-way-id")
                .setProtocol(6)
                .setTs(1_720_000_000_000L)
                .build();
        Rule rule = new Rule();
        rule.setRuleId("rule-7");
        rule.setVersion(9);
        DetectionResult result = DetectionResult.builder()
                .ruleId("rule-7")
                .ruleType(RuleType.PORT_SCAN)
                .score(0.91f)
                .build();

        assertThatThrownBy(() -> function.buildDetectionEvent(feature, result, rule))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("observed source tuple");
    }
}
