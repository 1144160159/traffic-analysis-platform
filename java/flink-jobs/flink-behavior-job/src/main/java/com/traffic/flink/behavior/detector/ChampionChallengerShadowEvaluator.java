package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.lang.management.ManagementFactory;
import java.lang.management.ThreadMXBean;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Objects;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Future;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.function.LongSupplier;

/** Pure comparison boundary. Challenger failures are converted to observations. */
public final class ChampionChallengerShadowEvaluator {
    public static final int SAMPLE_BUCKETS = 10_000;

    private final ExecutorService challengerExecutor;
    private final long timeoutMs;
    private final LongSupplier wallClockMs;

    public ChampionChallengerShadowEvaluator(
            ExecutorService challengerExecutor, long timeoutMs, LongSupplier wallClockMs) {
        this.challengerExecutor = Objects.requireNonNull(challengerExecutor, "challengerExecutor");
        if (timeoutMs <= 0) throw new IllegalArgumentException("challenger timeout must be positive");
        this.timeoutMs = timeoutMs;
        this.wallClockMs = Objects.requireNonNull(wallClockMs, "wallClockMs");
    }

    public ChampionChallengerObservation evaluate(
            FeatureStat feature, ChampionOutcome champion, Challenger challenger, int sampleBucket) {
        ChampionChallengerObservation.Builder observation = base(
                feature, champion, challenger, sampleBucket, wallClockMs.getAsLong());
        long waitStarted = System.nanoTime();
        Future<ChallengerRun> future;
        try {
            future = challengerExecutor.submit(() -> runChallenger(feature, challenger));
        } catch (RejectedExecutionException overloaded) {
            return observation.status("overloaded")
                    .errorCode("challenger_queue_full")
                    .errorMessage(overloaded.getMessage())
                    .build();
        }
        try {
            ChallengerRun run = future.get(timeoutMs, TimeUnit.MILLISECONDS);
            boolean challengerDetected = run.score >= challenger.threshold();
            String challengerLabel = challengerDetected ? "malicious" : "benign";
            observation.challengerScore(run.score)
                    .challengerDetected(challengerDetected)
                    .challengerLabel(challengerLabel)
                    .challengerLatencyNanos(run.latencyNanos)
                    .challengerCpuNanos(run.cpuNanos)
                    .challengerHeapDeltaBytes(run.heapDeltaBytes);
            if (champion.result == null) {
                return observation.status("champion_unavailable")
                        .errorCode("champion_no_result")
                        .errorMessage(champion.errorMessage)
                        .build();
            }
            return observation.status("compared")
                    .absoluteScoreDelta(Math.abs(champion.result.getTopScore() - run.score))
                    .decisionChanged(champion.result.isDetected() != challengerDetected)
                    .labelChanged(!safe(champion.result.getTopLabel()).equals(challengerLabel))
                    .build();
        } catch (TimeoutException timeout) {
            future.cancel(true);
            return observation.status("timeout")
                    .challengerLatencyNanos(System.nanoTime() - waitStarted)
                    .errorCode("challenger_timeout")
                    .errorMessage("challenger exceeded " + timeoutMs + "ms")
                    .build();
        } catch (InterruptedException interrupted) {
            future.cancel(true);
            Thread.currentThread().interrupt();
            return observation.status("error")
                    .challengerLatencyNanos(System.nanoTime() - waitStarted)
                    .errorCode("observer_interrupted")
                    .errorMessage(interrupted.getMessage())
                    .build();
        } catch (ExecutionException failed) {
            Throwable cause = failed.getCause() == null ? failed : failed.getCause();
            return observation.status("error")
                    .challengerLatencyNanos(System.nanoTime() - waitStarted)
                    .errorCode("challenger_error")
                    .errorMessage(cause.getMessage())
                    .build();
        }
    }

    private static ChallengerRun runChallenger(FeatureStat feature, Challenger challenger)
            throws Exception {
        ThreadMXBean threads = ManagementFactory.getThreadMXBean();
        boolean cpuSupported = threads.isCurrentThreadCpuTimeSupported();
        long cpuBefore = cpuSupported ? threads.getCurrentThreadCpuTime() : -1L;
        Runtime runtime = Runtime.getRuntime();
        long heapBefore = runtime.totalMemory() - runtime.freeMemory();
        long started = System.nanoTime();
        float score = challenger.predict(feature);
        long elapsed = System.nanoTime() - started;
        long heapAfter = runtime.totalMemory() - runtime.freeMemory();
        long cpuAfter = cpuSupported ? threads.getCurrentThreadCpuTime() : -1L;
        if (!Float.isFinite(score) || score < 0.0f || score > 1.0f) {
            throw new IllegalArgumentException("challenger returned an invalid probability");
        }
        return new ChallengerRun(score, elapsed,
                cpuSupported ? Math.max(0L, cpuAfter - cpuBefore) : -1L,
                heapAfter - heapBefore);
    }

    private static ChampionChallengerObservation.Builder base(
            FeatureStat feature, ChampionOutcome champion, Challenger challenger,
            int sampleBucket, long observedAtMs) {
        String tenantId = feature.hasHeader() ? safe(feature.getHeader().getTenantId()) : "";
        String sourceEventId = feature.hasHeader() ? safe(feature.getHeader().getEventId()) : "";
        String observationId = observationId(
                tenantId, sourceEventId, challenger.modelId(), challenger.packageId(),
                challenger.aggregateRevision());
        ChampionChallengerObservation.Builder value = ChampionChallengerObservation.builder()
                .observationId(observationId)
                .tenantId(tenantId)
                .sourceEventId(sourceEventId)
                .objectId(safe(feature.getObjectId()))
                .communityId(safe(feature.getCommunityId()))
                .eventTimeMs(feature.getTs())
                .observedAtMs(observedAtMs)
                .sampleBucket(sampleBucket)
                .championLatencyNanos(champion.latencyNanos)
                .challengerModelId(challenger.modelId())
                .challengerVersion(challenger.version())
                .challengerPackageId(challenger.packageId())
                .challengerPackageSha256(challenger.packageSha256())
                .challengerAggregateRevision(challenger.aggregateRevision());
        if (champion.result != null) {
            value.championModelId(safe(champion.result.getModelName()))
                    .championVersion(safe(champion.version))
                    .championLabel(safe(champion.result.getTopLabel()))
                    .championScore(champion.result.getTopScore())
                    .championDetected(champion.result.isDetected());
        }
        return value;
    }

    public static int sampleBucket(String tenantId, String sourceEventId) {
        byte[] digest = sha256(safe(tenantId) + '\u0000' + safe(sourceEventId));
        long unsigned = Integer.toUnsignedLong(ByteBuffer.wrap(digest).getInt());
        return (int) (unsigned % SAMPLE_BUCKETS);
    }

    public static boolean selected(int bucket, double sampleRate) {
        if (!Double.isFinite(sampleRate) || sampleRate <= 0.0d || sampleRate > 1.0d) {
            throw new IllegalArgumentException("sample rate must be in (0,1]");
        }
        return bucket >= 0 && bucket < Math.ceil(sampleRate * SAMPLE_BUCKETS);
    }

    public static String observationId(String tenantId, String sourceEventId,
                                       String modelId, String packageId, long revision) {
        return hex(sha256("champion-challenger-v1\u0000"
                + safe(tenantId) + '\u0000' + safe(sourceEventId) + '\u0000'
                + safe(modelId) + '\u0000' + safe(packageId) + '\u0000' + revision));
    }

    private static byte[] sha256(String value) {
        try {
            return MessageDigest.getInstance("SHA-256")
                    .digest(value.getBytes(StandardCharsets.UTF_8));
        } catch (Exception impossible) {
            throw new IllegalStateException("SHA-256 is unavailable", impossible);
        }
    }

    private static String hex(byte[] bytes) {
        StringBuilder value = new StringBuilder(bytes.length * 2);
        for (byte item : bytes) value.append(String.format("%02x", item));
        return value.toString();
    }

    private static String safe(String value) { return value == null ? "" : value; }

    public interface Challenger {
        String modelId();
        String version();
        String packageId();
        String packageSha256();
        long aggregateRevision();
        float threshold();
        float predict(FeatureStat feature) throws Exception;
    }

    public static final class ChampionOutcome {
        private final ModelInferenceResult result;
        private final String version;
        private final long latencyNanos;
        private final String errorMessage;

        public ChampionOutcome(ModelInferenceResult result, String version,
                               long latencyNanos, String errorMessage) {
            this.result = result;
            this.version = safe(version);
            this.latencyNanos = Math.max(0L, latencyNanos);
            this.errorMessage = safe(errorMessage);
        }

        public ModelInferenceResult getResult() { return result; }
        public String getVersion() { return version; }
        public long getLatencyNanos() { return latencyNanos; }
        public String getErrorMessage() { return errorMessage; }
    }

    private static final class ChallengerRun {
        private final float score;
        private final long latencyNanos;
        private final long cpuNanos;
        private final long heapDeltaBytes;

        private ChallengerRun(float score, long latencyNanos, long cpuNanos, long heapDeltaBytes) {
            this.score = score;
            this.latencyNanos = latencyNanos;
            this.cpuNanos = cpuNanos;
            this.heapDeltaBytes = heapDeltaBytes;
        }
    }
}
