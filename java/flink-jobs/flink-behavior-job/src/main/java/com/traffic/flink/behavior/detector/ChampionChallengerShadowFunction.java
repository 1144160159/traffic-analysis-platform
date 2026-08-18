package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.BehaviorModel;
import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ShadowEvaluationRequest;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.streaming.api.functions.async.ResultFuture;
import org.apache.flink.streaming.api.functions.async.RichAsyncFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Independent observer branch. It never returns a DetectionBehavior and never
 * writes ModelRegistry's active maps, so challenger output cannot become a
 * serving result even when the branch is enabled.
 */
public final class ChampionChallengerShadowFunction
        extends RichAsyncFunction<ShadowEvaluationRequest, ChampionChallengerObservation> {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ChampionChallengerShadowFunction.class);

    private final BehaviorJobConfig config;
    private transient ModelRegistry championRegistry;
    private transient ExecutorService observationExecutor;
    private transient ThreadPoolExecutor challengerExecutor;
    private transient ChampionChallengerShadowEvaluator evaluator;
    private transient Counter sampled;
    private transient Counter compared;
    private transient Counter decisionsChanged;
    private transient Counter challengerTimeouts;
    private transient Counter challengerErrors;
    private transient Counter missingCandidates;
    private transient Counter observerErrors;

    public ChampionChallengerShadowFunction(BehaviorJobConfig config) {
        this.config = config;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        config.validateChampionChallengerShadowConfig();
        championRegistry = new ModelRegistry(config);
        AtomicInteger observerId = new AtomicInteger();
        observationExecutor = Executors.newFixedThreadPool(
                Math.max(1, config.getInferenceThreads()), runnable -> daemonThread(
                        runnable, "champion-shadow-observer-" + observerId.incrementAndGet()));
        AtomicInteger challengerId = new AtomicInteger();
        challengerExecutor = new ThreadPoolExecutor(
                config.getModelShadowChallengerThreads(),
                config.getModelShadowChallengerThreads(),
                0L,
                TimeUnit.MILLISECONDS,
                new ArrayBlockingQueue<>(config.getModelShadowChallengerQueueCapacity()),
                runnable -> daemonThread(
                        runnable, "challenger-inference-" + challengerId.incrementAndGet()),
                new ThreadPoolExecutor.AbortPolicy());
        evaluator = new ChampionChallengerShadowEvaluator(
                challengerExecutor, config.getModelShadowChallengerTimeoutMs(),
                System::currentTimeMillis);

        MetricGroup metrics = getRuntimeContext().getMetricGroup()
                .addGroup("champion_challenger_shadow");
        sampled = metrics.counter("sampled_total");
        compared = metrics.counter("compared_total");
        decisionsChanged = metrics.counter("decision_changed_total");
        challengerTimeouts = metrics.counter("challenger_timeout_total");
        challengerErrors = metrics.counter("challenger_error_total");
        missingCandidates = metrics.counter("candidate_missing_total");
        observerErrors = metrics.counter("observer_error_total");
        LOG.info("Champion/challenger shadow observer opened: sampleRate={}, timeoutMs={}, "
                        + "challengerThreads={}, queueCapacity={}, outputTopic={}",
                config.getModelShadowSampleRate(), config.getModelShadowChallengerTimeoutMs(),
                config.getModelShadowChallengerThreads(),
                config.getModelShadowChallengerQueueCapacity(),
                config.getModelShadowObservationTopic());
    }

    @Override
    public void asyncInvoke(
            ShadowEvaluationRequest request,
            ResultFuture<ChampionChallengerObservation> resultFuture) {
        if (!config.isModelShadowEvaluationEnabled()) {
            resultFuture.complete(Collections.emptyList());
            return;
        }
        FeatureStat feature = request == null ? null : request.getFeature();
        String tenantId = feature != null && feature.hasHeader()
                ? feature.getHeader().getTenantId() : "";
        String eventId = feature != null && feature.hasHeader()
                ? feature.getHeader().getEventId() : "";
        if (tenantId == null || tenantId.isBlank() || eventId == null || eventId.isBlank()) {
            observerErrors.inc();
            resultFuture.complete(Collections.emptyList());
            return;
        }
        List<ModelUpdateEvent> candidates = request == null
                ? Collections.emptyList() : request.getCandidates();
        if (candidates.isEmpty()) {
            missingCandidates.inc();
            resultFuture.complete(Collections.emptyList());
            return;
        }
        int bucket = ChampionChallengerShadowEvaluator.sampleBucket(tenantId, eventId);
        if (!ChampionChallengerShadowEvaluator.selected(
                bucket, config.getModelShadowSampleRate())) {
            resultFuture.complete(Collections.emptyList());
            return;
        }
        sampled.inc();

        CompletableFuture.supplyAsync(() -> evaluate(feature, candidates, bucket), observationExecutor)
                .thenAccept(resultFuture::complete)
                .exceptionally(failed -> {
                    observerErrors.inc();
                    LOG.warn("Champion/challenger observer failed for event {}: {}",
                            eventId, failed.getMessage());
                    resultFuture.complete(Collections.emptyList());
                    return null;
                });
    }

    private List<ChampionChallengerObservation> evaluate(
            FeatureStat feature,
            List<ModelUpdateEvent> candidates,
            int bucket) {
        ChampionChallengerShadowEvaluator.ChampionOutcome champion = runChampion(feature);
        List<ModelUpdateEvent> ordered = new ArrayList<>(candidates);
        ordered.sort(Comparator.comparing(ModelUpdateEvent::getModelId));
        List<ChampionChallengerObservation> observations = new ArrayList<>(ordered.size());
        for (ModelUpdateEvent event : ordered) {
            ChampionChallengerShadowEvaluator.Challenger challenger;
            try {
                championRegistry.stageShadowPackage(event);
                GovernedModelPackageLoader.ShadowPackage candidate =
                        championRegistry.getLocalShadowPackage(
                                event.getTenantId(), event.getModelId());
                if (candidate == null) {
                    throw new IllegalStateException("staged shadow package is not locally visible");
                }
                challenger = new PackageChallenger(candidate);
            } catch (Exception stageFailure) {
                challenger = new FailedPackageChallenger(event, stageFailure);
            }
            ChampionChallengerObservation observation = evaluator.evaluate(
                    feature, champion, challenger, bucket);
            record(observation);
            observations.add(observation);
        }
        return observations;
    }

    private ChampionChallengerShadowEvaluator.ChampionOutcome runChampion(FeatureStat feature) {
        long started = System.nanoTime();
        List<ModelInferenceResult> results = new ArrayList<>();
        String tenantId = feature.hasHeader() ? feature.getHeader().getTenantId() : "";
        String lastError = "";
        for (Map.Entry<String, BehaviorModel> entry
                : championRegistry.getModelsForTenant(tenantId).entrySet()) {
            String modelName = entry.getKey();
            BehaviorModel model = entry.getValue();
            try {
                if (!model.isReady()) continue;
                ModelInferenceResult result = inferWithRetry(modelName, model, feature);
                if (result != null && !result.hasError()) {
                    results.add(result);
                    championRegistry.recordInvocation(modelName);
                } else if (result != null) {
                    lastError = result.getErrorMessage();
                    championRegistry.recordError(modelName);
                }
            } catch (Exception error) {
                lastError = error.getMessage();
                championRegistry.recordError(modelName);
            }
        }
        ModelInferenceResult best = selectBestResult(results);
        String version = best == null ? ""
                : championRegistry.getModelVersion(tenantId, best.getModelName());
        if ((version == null || version.isBlank()) && best != null) version = best.getModelVersion();
        return new ChampionChallengerShadowEvaluator.ChampionOutcome(
                best, version, System.nanoTime() - started, lastError);
    }

    private ModelInferenceResult inferWithRetry(
            String modelName, BehaviorModel model, FeatureStat feature) throws Exception {
        int attempts = Math.max(1, config.getAsyncMaxRetries() + 1);
        Exception lastFailure = null;
        for (int attempt = 1; attempt <= attempts; attempt++) {
            try {
                return model.infer(feature);
            } catch (Exception failed) {
                lastFailure = failed;
                if (attempt < attempts) {
                    try {
                        Thread.sleep(Math.min(500L, 50L << (attempt - 1)));
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw interrupted;
                    }
                }
            }
        }
        throw new IllegalStateException("champion inference failed for " + modelName, lastFailure);
    }

    static ModelInferenceResult selectBestResult(List<ModelInferenceResult> results) {
        if (results == null || results.isEmpty()) return null;
        ModelInferenceResult best = null;
        float bestScore = 0.0f;
        for (ModelInferenceResult result : results) {
            if (result.isDetected() && result.getTopScore() > bestScore) {
                bestScore = result.getTopScore();
                best = result;
            }
        }
        if (best == null) {
            for (ModelInferenceResult result : results) {
                if (result.getTopScore() > bestScore) {
                    bestScore = result.getTopScore();
                    best = result;
                }
            }
        }
        return best;
    }

    private void record(ChampionChallengerObservation observation) {
        switch (observation.getStatus()) {
            case "compared":
                compared.inc();
                if (Boolean.TRUE.equals(observation.getDecisionChanged())) decisionsChanged.inc();
                break;
            case "timeout":
                challengerTimeouts.inc();
                break;
            case "error":
            case "overloaded":
                challengerErrors.inc();
                break;
            default:
                break;
        }
    }

    @Override
    public void timeout(
            ShadowEvaluationRequest request,
            ResultFuture<ChampionChallengerObservation> resultFuture) {
        observerErrors.inc();
        FeatureStat feature = request == null ? null : request.getFeature();
        String tenantId = feature != null && feature.hasHeader()
                ? feature.getHeader().getTenantId() : "";
        String eventId = feature != null && feature.hasHeader()
                ? feature.getHeader().getEventId() : "";
        int bucket = ChampionChallengerShadowEvaluator.sampleBucket(tenantId, eventId);
        List<ChampionChallengerObservation> timedOut = new ArrayList<>();
        List<ModelUpdateEvent> candidates = request == null
                ? Collections.emptyList() : request.getCandidates();
        for (ModelUpdateEvent candidate : candidates) {
            timedOut.add(ChampionChallengerObservation.builder()
                    .observationId(ChampionChallengerShadowEvaluator.observationId(
                            tenantId, eventId, candidate.getModelId(),
                            candidate.getPackageId(), candidate.getAggregateRevision()))
                    .tenantId(tenantId)
                    .sourceEventId(eventId)
                    .objectId(feature == null ? "" : feature.getObjectId())
                    .communityId(feature == null ? "" : feature.getCommunityId())
                    .eventTimeMs(feature == null ? 0L : feature.getTs())
                    .observedAtMs(System.currentTimeMillis())
                    .sampleBucket(bucket)
                    .challengerModelId(candidate.getModelId())
                    .challengerVersion(candidate.getVersion())
                    .challengerPackageId(candidate.getPackageId())
                    .challengerPackageSha256(candidate.getPackageSha256())
                    .challengerAggregateRevision(candidate.getAggregateRevision())
                    .status("timeout")
                    .errorCode("shadow_operator_timeout")
                    .errorMessage("shadow observer exceeded the Flink async timeout")
                    .build());
        }
        resultFuture.complete(timedOut);
    }

    @Override
    public void close() throws Exception {
        shutdown(observationExecutor);
        shutdown(challengerExecutor);
        if (championRegistry != null) championRegistry.close();
        super.close();
    }

    private static Thread daemonThread(Runnable runnable, String name) {
        Thread thread = new Thread(runnable, name);
        thread.setDaemon(true);
        return thread;
    }

    private static void shutdown(ExecutorService executor) {
        if (executor == null) return;
        executor.shutdown();
        try {
            if (!executor.awaitTermination(10, TimeUnit.SECONDS)) executor.shutdownNow();
        } catch (InterruptedException interrupted) {
            executor.shutdownNow();
            Thread.currentThread().interrupt();
        }
    }

    private static final class PackageChallenger
            implements ChampionChallengerShadowEvaluator.Challenger {
        private final GovernedModelPackageLoader.ShadowPackage candidate;

        private PackageChallenger(GovernedModelPackageLoader.ShadowPackage candidate) {
            this.candidate = candidate;
        }

        @Override public String modelId() { return candidate.getModelId(); }
        @Override public String version() { return candidate.getVersion(); }
        @Override public String packageId() { return candidate.getPackageId(); }
        @Override public String packageSha256() { return candidate.getPackageSha256(); }
        @Override public long aggregateRevision() { return candidate.getAggregateRevision(); }
        @Override public float threshold() { return candidate.getThreshold(); }
        @Override public float predict(FeatureStat feature) throws Exception {
            return candidate.predict(feature);
        }
    }

    private static final class FailedPackageChallenger
            implements ChampionChallengerShadowEvaluator.Challenger {
        private final ModelUpdateEvent event;
        private final Exception failure;

        private FailedPackageChallenger(ModelUpdateEvent event, Exception failure) {
            this.event = event;
            this.failure = failure;
        }

        @Override public String modelId() { return event.getModelId(); }
        @Override public String version() { return event.getVersion(); }
        @Override public String packageId() { return event.getPackageId(); }
        @Override public String packageSha256() { return event.getPackageSha256(); }
        @Override public long aggregateRevision() { return event.getAggregateRevision(); }
        @Override public float threshold() { return event.getThreshold(0.5f); }
        @Override public float predict(FeatureStat feature) throws Exception { throw failure; }
    }
}
