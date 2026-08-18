package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

import static org.assertj.core.api.Assertions.assertThat;

class ChampionChallengerShadowEvaluatorTest {
    private ExecutorService executor;

    @AfterEach
    void stopExecutor() {
        if (executor != null) executor.shutdownNow();
    }

    @Test
    void comparedObservationNeverChangesServingAuthority() {
        executor = Executors.newSingleThreadExecutor();
        ChampionChallengerShadowEvaluator evaluator =
                new ChampionChallengerShadowEvaluator(executor, 100L, () -> 9_000L);
        ModelInferenceResult championResult = ModelInferenceResult.success("champion-model", "v1")
                .topLabel("malicious")
                .topScore(0.8f)
                .detected(true)
                .build();
        ChampionChallengerShadowEvaluator.ChampionOutcome champion =
                new ChampionChallengerShadowEvaluator.ChampionOutcome(
                        championResult, "frozen-v1", 123L, "");

        ChampionChallengerObservation observation = evaluator.evaluate(
                feature(), champion, candidate(0.4f, 0L), 42);

        assertThat(observation.getStatus()).isEqualTo("compared");
        assertThat(observation.getServingResultSource()).isEqualTo("champion");
        assertThat(observation.getChampionModelId()).isEqualTo("champion-model");
        assertThat(observation.getChampionVersion()).isEqualTo("frozen-v1");
        assertThat(observation.getChampionScore()).isEqualTo(0.8f);
        assertThat(observation.getChallengerScore()).isEqualTo(0.4f);
        assertThat(observation.getAbsoluteScoreDelta()).isEqualTo(0.4f);
        assertThat(observation.getDecisionChanged()).isTrue();
        assertThat(observation.getLabelChanged()).isTrue();
        assertThat(observation.getObservedAtMs()).isEqualTo(9_000L);
        assertThat(observation.getChallengerLatencyNanos()).isPositive();
        assertThat(observation.getChallengerCpuNanos()).isGreaterThanOrEqualTo(-1L);
        // The immutable input remains the only serving result.
        assertThat(championResult.getTopScore()).isEqualTo(0.8f);
        assertThat(championResult.isDetected()).isTrue();
    }

    @Test
    void challengerExceptionIsAnObservationNotAnUpstreamFailure() {
        executor = Executors.newSingleThreadExecutor();
        ChampionChallengerShadowEvaluator evaluator =
                new ChampionChallengerShadowEvaluator(executor, 100L, () -> 9_001L);
        ChampionChallengerShadowEvaluator.ChampionOutcome champion =
                new ChampionChallengerShadowEvaluator.ChampionOutcome(
                        ModelInferenceResult.success("champion", "v1")
                                .topLabel("benign").topScore(0.2f).detected(false).build(),
                        "frozen-v1", 10L, "");
        TestCandidate broken = candidate(0.0f, 0L);
        broken.failure = new IllegalStateException("candidate native runtime failed");

        ChampionChallengerObservation observation =
                evaluator.evaluate(feature(), champion, broken, 5);

        assertThat(observation.getStatus()).isEqualTo("error");
        assertThat(observation.getErrorCode()).isEqualTo("challenger_error");
        assertThat(observation.getErrorMessage()).contains("native runtime failed");
        assertThat(observation.getChampionScore()).isEqualTo(0.2f);
        assertThat(observation.getChallengerScore()).isNull();
        assertThat(observation.getServingResultSource()).isEqualTo("champion");
    }

    @Test
    void challengerTimeoutIsBoundedAndObservable() {
        executor = Executors.newSingleThreadExecutor();
        ChampionChallengerShadowEvaluator evaluator =
                new ChampionChallengerShadowEvaluator(executor, 10L, () -> 9_002L);

        ChampionChallengerObservation observation = evaluator.evaluate(
                feature(),
                new ChampionChallengerShadowEvaluator.ChampionOutcome(
                        ModelInferenceResult.success("champion", "v1")
                                .topLabel("benign").topScore(0.1f).detected(false).build(),
                        "frozen-v1", 8L, ""),
                candidate(0.9f, 200L),
                8);

        assertThat(observation.getStatus()).isEqualTo("timeout");
        assertThat(observation.getErrorCode()).isEqualTo("challenger_timeout");
        assertThat(observation.getChallengerScore()).isNull();
        assertThat(observation.getServingResultSource()).isEqualTo("champion");
    }

    @Test
    void samplingAndIdentityAreStableAcrossRetryAndPartition() {
        String tenant = "tenant-1";
        String event = "event-1";
        int firstBucket = ChampionChallengerShadowEvaluator.sampleBucket(tenant, event);
        int retryBucket = ChampionChallengerShadowEvaluator.sampleBucket(tenant, event);
        String firstId = ChampionChallengerShadowEvaluator.observationId(
                tenant, event, "model-1", "package-1", 7L);
        String retryId = ChampionChallengerShadowEvaluator.observationId(
                tenant, event, "model-1", "package-1", 7L);

        assertThat(retryBucket).isEqualTo(firstBucket).isBetween(0, 9_999);
        assertThat(retryId).isEqualTo(firstId).matches("^[0-9a-f]{64}$");
        assertThat(ChampionChallengerShadowEvaluator.selected(firstBucket, 1.0d)).isTrue();
        assertThat(ChampionChallengerShadowEvaluator.selected(firstBucket, 0.0001d))
                .isEqualTo(firstBucket == 0);
    }

    @Test
    void selectionMatchesServingDetectorTieAndDetectionRules() {
        ModelInferenceResult normalHigh = ModelInferenceResult.success("normal", "v1")
                .topScore(0.95f).topLabel("benign").detected(false).build();
        ModelInferenceResult detectedLower = ModelInferenceResult.success("detected", "v1")
                .topScore(0.7f).topLabel("malicious").detected(true).build();

        assertThat(ChampionChallengerShadowFunction.selectBestResult(
                java.util.List.of(normalHigh, detectedLower))).isSameAs(detectedLower);
    }

    private static FeatureStat feature() {
        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1")
                        .build())
                .setObjectId("flow-1")
                .setCommunityId("1:community")
                .setTs(8_000L)
                .setPps(10.0f)
                .build();
    }

    private static TestCandidate candidate(float score, long delayMs) {
        return new TestCandidate(score, delayMs);
    }

    private static final class TestCandidate
            implements ChampionChallengerShadowEvaluator.Challenger {
        private final float score;
        private final long delayMs;
        private RuntimeException failure;

        private TestCandidate(float score, long delayMs) {
            this.score = score;
            this.delayMs = delayMs;
        }

        @Override public String modelId() { return "model-1"; }
        @Override public String version() { return "candidate-v2"; }
        @Override public String packageId() { return "package-1"; }
        @Override public String packageSha256() { return "a".repeat(64); }
        @Override public long aggregateRevision() { return 7L; }
        @Override public float threshold() { return 0.5f; }

        @Override
        public float predict(FeatureStat ignored) throws Exception {
            if (delayMs > 0L) Thread.sleep(delayMs);
            if (failure != null) throw failure;
            return score;
        }
    }
}
