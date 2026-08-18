package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class BehaviorDetectorFunctionTest {

    @Test
    void replayKeepsIdentityEnvelopeTupleAndEvidenceStable() {
        FeatureStat feature = canonicalFeature();
        ModelInferenceResult result = ModelInferenceResult.success("scan-model", "artifact-v1")
                .addLabel("port_scan", 0.93f)
                .addLabel("normal", 0.07f)
                .detected(true)
                .build();

        DetectionBehavior first = BehaviorDetectionEventFactory.build(
                feature, result, "known-profile-v1", 3_000L);
        DetectionBehavior replay = BehaviorDetectionEventFactory.build(
                feature, result, "known-profile-v1", 4_000L);

        assertThat(replay.getHeader().getEventId())
                .isEqualTo(first.getHeader().getEventId());
        assertThat(replay.getHeader().getIdempotencyKey())
                .isEqualTo(first.getHeader().getEventId());
        assertThat(first.getHeader().getTenantId()).isEqualTo("tenant-1");
        assertThat(first.getHeader().getEventType())
                .isEqualTo("traffic.detection.behavior.v1");
        assertThat(first.getHeader().getSchemaVersion()).isEqualTo("1");
        assertThat(first.getHeader().getAggregateType()).isEqualTo("detection");
        assertThat(first.getHeader().getAggregateId()).isEqualTo("flow-1");
        assertThat(first.getHeader().getTraceId()).isEqualTo("trace-1");
        assertThat(first.getHeader().getCausationId()).isEqualTo("feature-event-1");
        assertThat(first.getHeader().getCorrelationId()).isEqualTo("correlation-1");
        assertThat(first.getHeader().getProducer()).isEqualTo("flink-behavior-job");
        assertThat(first.getHeader().getProducedAt()).isEqualTo(3_000L);
        assertThat(replay.getHeader().getProducedAt()).isEqualTo(4_000L);
        assertThat(first.getModelVersion()).isEqualTo("known-profile-v1");
        assertThat(first.getTuple()).isEqualTo(feature.getTuple());
        assertThat(first.getEvidenceIdsList())
                .containsExactly("evidence-1", "evidence-2");
        assertThat(first.getLabelsList()).containsExactly("port_scan", "normal");
        assertThat(first.getScoresList()).containsExactly(0.93f, 0.07f);
    }

    @Test
    void modelVersionParticipatesInDeterministicIdentity() {
        FeatureStat feature = canonicalFeature();
        ModelInferenceResult result = ModelInferenceResult.success("scan-model", "artifact-v1")
                .addLabel("port_scan", 0.93f)
                .detected(true)
                .build();

        DetectionBehavior oldVersion = BehaviorDetectionEventFactory.build(
                feature, result, "known-profile-v1", 3_000L);
        DetectionBehavior newVersion = BehaviorDetectionEventFactory.build(
                feature, result, "known-profile-v2", 3_000L);

        assertThat(newVersion.getHeader().getEventId())
                .isNotEqualTo(oldVersion.getHeader().getEventId());
    }

    @Test
    void incompleteTupleAndMalformedScoresFailClosed() {
        FeatureStat missingTuple = canonicalFeature().toBuilder().clearTuple().build();
        ModelInferenceResult validResult = ModelInferenceResult.success("scan-model", "artifact-v1")
                .addLabel("port_scan", 0.93f)
                .detected(true)
                .build();
        ModelInferenceResult mismatched = ModelInferenceResult.success("scan-model", "artifact-v1")
                .labels(List.of("port_scan"))
                .scores(List.of())
                .topLabel("port_scan")
                .topScore(0.93f)
                .detected(true)
                .build();

        assertThatThrownBy(() -> BehaviorDetectionEventFactory.build(
                missingTuple, validResult, "known-profile-v1", 3_000L))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("tuple");
        assertThatThrownBy(() -> BehaviorDetectionEventFactory.build(
                canonicalFeature(), mismatched, "known-profile-v1", 3_000L))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("cardinality");
    }

    private static FeatureStat canonicalFeature() {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("feature-event-1")
                .setTenantId("tenant-1")
                .setRunId("run-1")
                .setProbeId("probe-1")
                .setFeatureSetId("feature-set-1")
                .setTraceId("trace-1")
                .setCorrelationId("correlation-1")
                .setEventTs(2_000L)
                .setIngestTs(2_100L)
                .setEventType("traffic.feature.stat.v1")
                .setSchemaVersion("1")
                .setAggregateType("flow")
                .setAggregateId("flow-1")
                .setAggregateVersion(1)
                .setOccurredAt(2_000L)
                .setProducedAt(2_100L)
                .setIdempotencyKey("feature-event-1")
                .setProducer("flink-feature-job")
                .build();
        FiveTuple tuple = FiveTuple.newBuilder()
                .setSrcIp("192.0.2.10")
                .setDstIp("198.51.100.20")
                .setSrcPort(54321)
                .setDstPort(443)
                .setProtocol(6)
                .build();
        return FeatureStat.newBuilder()
                .setHeader(header)
                .setObjectType("flow")
                .setObjectId("flow-1")
                .setCommunityId("1:test-community")
                .setTs(2_000L)
                .setProtocol(6)
                .setTuple(tuple)
                .addEvidenceIds("evidence-1")
                .addEvidenceIds("evidence-2")
                .build();
    }
}
