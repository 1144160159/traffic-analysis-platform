package com.traffic.flink.behavior.detector;

import ai.onnxruntime.OrtEnvironment;
import ai.onnxruntime.OrtSession;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.MapperFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.traffic.flink.behavior.model.FeatureStatVectorizer;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.api.common.functions.RichMapFunction;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.runtime.util.EnvironmentInformation;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.util.CloseableIterator;

import java.io.BufferedReader;
import java.io.Serializable;
import java.lang.management.ManagementFactory;
import java.lang.management.ThreadMXBean;
import java.nio.channels.FileChannel;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.TreeMap;

/**
 * Runs the production FeatureStat vectorizer and ONNX probability parser in a
 * bounded Flink DataStream for the M08 internal parity profile.
 *
 * <p>This executable writes a route receipt only. The Python finalizer owns
 * cross-runtime comparison and explicitly cannot authorize activation,
 * production promotion, or a CNAS claim.</p>
 */
public final class ModelInferenceParityMain {
    private static final ObjectMapper MAPPER = new ObjectMapper()
            .enable(MapperFeature.SORT_PROPERTIES_ALPHABETICALLY)
            .enable(SerializationFeature.ORDER_MAP_ENTRIES_BY_KEYS);
    private static final Set<String> SUPPORTED_COLUMNS = Set.of(
            "pps", "bps", "up_down_ratio", "pktlen_mean", "pktlen_std",
            "iat_mean_ms", "iat_std_ms", "active_mean_ms", "idle_mean_ms",
            "duration_ms", "tcp_flag_syn_cnt", "tcp_flag_ack_cnt",
            "tcp_init_win_bytes_fwd", "tcp_init_win_bytes_bwd", "protocol");

    private ModelInferenceParityMain() {}

    public static void main(String[] args) throws Exception {
        if (args.length != 3) {
            throw new IllegalArgumentException(
                    "usage: ModelInferenceParityMain <input.json> <model.onnx> <receipt.json>");
        }
        Path inputPath = Path.of(args[0]).toAbsolutePath().normalize();
        Path modelPath = Path.of(args[1]).toAbsolutePath().normalize();
        Path outputPath = Path.of(args[2]).toAbsolutePath().normalize();
        if (Files.exists(outputPath)) {
            throw new IllegalStateException("refusing to overwrite immutable Flink parity receipt");
        }
        InputBundle bundle = MAPPER.readValue(inputPath.toFile(), InputBundle.class);
        validateBundle(bundle, modelPath);

        List<InferenceTask> tasks = new ArrayList<>();
        int ordinal = 0;
        for (int iteration = 0; iteration < bundle.warmupIterations; iteration++) {
            for (Sample sample : bundle.samples) {
                tasks.add(new InferenceTask(ordinal++, iteration, true, sample));
            }
        }
        for (int iteration = 0; iteration < bundle.measuredIterations; iteration++) {
            for (Sample sample : bundle.samples) {
                tasks.add(new InferenceTask(ordinal++, iteration, false, sample));
            }
        }

        StreamExecutionEnvironment environment = StreamExecutionEnvironment.getExecutionEnvironment();
        environment.setParallelism(bundle.flinkParallelism);
        environment.getConfig().disableObjectReuse();
        DataStream<InferenceResult> results = environment
                .fromCollection(tasks)
                .name("m08-parity-samples")
                .map(new FlinkOnnxInference(modelPath.toString(), bundle.inputName, bundle.featureColumns))
                .name("production-feature-vectorizer-and-onnx-parser")
                .setParallelism(bundle.flinkParallelism);

        long wallStarted = System.nanoTime();
        List<InferenceResult> collected = new ArrayList<>(tasks.size());
        try (CloseableIterator<InferenceResult> iterator =
                     results.executeAndCollect("M08 Internal Model Inference Parity")) {
            while (iterator.hasNext()) {
                collected.add(iterator.next());
            }
        }
        long wallElapsed = System.nanoTime() - wallStarted;
        if (collected.size() != tasks.size()) {
            throw new IllegalStateException("Flink parity job returned an incomplete task set");
        }
        collected.sort(java.util.Comparator.comparingInt(value -> value.ordinal));

        Map<String, List<Float>> scores = new HashMap<>();
        List<Double> latencies = new ArrayList<>();
        long cpuNanos = 0L;
        int warmups = 0;
        for (InferenceResult result : collected) {
            if (!Float.isFinite(result.score) || result.score < 0.0f || result.score > 1.0f) {
                throw new IllegalStateException("Flink parity job produced an invalid probability");
            }
            if (result.warmup) {
                warmups++;
                continue;
            }
            scores.computeIfAbsent(result.sampleId, ignored -> new ArrayList<>()).add(result.score);
            latencies.add(result.latencyNanos / 1_000_000.0);
            cpuNanos += Math.max(0L, result.cpuNanos);
        }
        int expectedWarmups = bundle.samples.size() * bundle.warmupIterations;
        int measured = bundle.samples.size() * bundle.measuredIterations;
        if (warmups != expectedWarmups || latencies.size() != measured) {
            throw new IllegalStateException("Flink parity warmup or measured population is incomplete");
        }

        List<Map<String, Object>> predictions = new ArrayList<>();
        for (Sample sample : bundle.samples) {
            List<Float> repeated = scores.get(sample.sampleId);
            if (repeated == null || repeated.size() != bundle.measuredIterations) {
                throw new IllegalStateException("Flink parity sample has an incomplete repeat set");
            }
            float minimum = Collections.min(repeated);
            float maximum = Collections.max(repeated);
            if (maximum - minimum > 1.0e-9f) {
                throw new IllegalStateException("Flink repeated inference is nondeterministic");
            }
            Map<String, Object> prediction = new LinkedHashMap<>();
            prediction.put("sample_id", sample.sampleId);
            prediction.put("score", repeated.get(0));
            predictions.add(prediction);
        }

        List<Double> sortedLatency = new ArrayList<>(latencies);
        Collections.sort(sortedLatency);
        Map<String, Object> latency = new LinkedHashMap<>();
        latency.put("p50", percentile(sortedLatency, 0.50));
        latency.put("p95", percentile(sortedLatency, 0.95));
        latency.put("p99", percentile(sortedLatency, 0.99));
        latency.put("max", sortedLatency.get(sortedLatency.size() - 1));

        Map<String, Object> receipt = new LinkedHashMap<>();
        receipt.put("schema_version", 1);
        receipt.put("route", "flink");
        receipt.put("run_id", bundle.runId);
        receipt.put("profile_id", bundle.profileId);
        receipt.put("profile_sha256", bundle.profileSha256);
        receipt.put("candidate_sha256", bundle.candidateSha256);
        receipt.put("model_id", bundle.modelId);
        receipt.put("model_version", bundle.modelVersion);
        receipt.put("model_artifact_sha256", bundle.modelArtifactSha256);
        receipt.put("feature_columns_sha256", bundle.featureColumnsSha256);
        receipt.put("sample_set_sha256", bundle.sampleSetSha256);
        receipt.put("bundle_sha256", bundle.bundleSha256);
        receipt.put("engine", "flink-datastream+onnxruntime");
        receipt.put("engine_version", EnvironmentInformation.getVersion()
                + "/onnxruntime-" + OrtEnvironment.getEnvironment().getVersion());
        receipt.put("flink_execution_mode", "kubernetes-hosted-local-datastream");
        receipt.put("flink_parallelism", bundle.flinkParallelism);
        receipt.put("measured_inferences", measured);
        receipt.put("latency_ms", latency);
        receipt.put("throughput_per_second", measured / (wallElapsed / 1_000_000_000.0));
        receipt.put("cpu_seconds", cpuNanos / 1_000_000_000.0);
        receipt.put("peak_rss_bytes", peakRssBytes());
        receipt.put("predictions", predictions);
        byte[] payload = MAPPER.writerWithDefaultPrettyPrinter().writeValueAsBytes(receipt);
        try (FileChannel channel = FileChannel.open(
                outputPath, StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE)) {
            channel.write(java.nio.ByteBuffer.wrap(payload));
            channel.write(java.nio.ByteBuffer.wrap(new byte[]{'\n'}));
            channel.force(true);
        }
        System.out.println(MAPPER.writeValueAsString(Map.of(
                "phase", "flink-route-complete",
                "run_id", bundle.runId,
                "measured_inferences", measured,
                "receipt_sha256", sha256(outputPath))));
    }

    static void validateBundle(InputBundle bundle, Path modelPath) throws Exception {
        if (bundle == null || bundle.schemaVersion != 1) {
            throw new IllegalArgumentException("parity input schema_version must be 1");
        }
        for (String digest : List.of(
                bundle.profileSha256, bundle.candidateSha256, bundle.modelArtifactSha256,
                bundle.featureColumnsSha256, bundle.sampleSetSha256, bundle.bundleSha256)) {
            if (digest == null || !digest.matches("^[0-9a-f]{64}$")) {
                throw new IllegalArgumentException("parity input has an invalid SHA-256 identity");
            }
        }
        if (!bundle.modelArtifactSha256.equals(sha256(modelPath))) {
            throw new IllegalArgumentException("Flink parity model artifact SHA-256 mismatch");
        }
        if (!"float_input".equals(bundle.inputName)) {
            throw new IllegalArgumentException("Flink parity ONNX input name drifted");
        }
        if (bundle.featureColumns == null || bundle.featureColumns.isEmpty()
                || bundle.featureColumns.size() > 15
                || !SUPPORTED_COLUMNS.containsAll(bundle.featureColumns)
                || new HashSet<>(bundle.featureColumns).size() != bundle.featureColumns.size()) {
            throw new IllegalArgumentException("Flink parity feature column contract is invalid");
        }
        if (!bundle.featureColumnsSha256.equals(sha256(MAPPER.writeValueAsBytes(bundle.featureColumns)))) {
            throw new IllegalArgumentException("Flink parity feature column hash mismatch");
        }
        if (bundle.samples == null || bundle.samples.size() < 32
                || bundle.warmupIterations < 1 || bundle.measuredIterations < 3
                || bundle.flinkParallelism < 1 || bundle.flinkParallelism > 16) {
            throw new IllegalArgumentException("Flink parity population or execution profile is invalid");
        }
        Set<String> sampleIds = new HashSet<>();
        for (Sample sample : bundle.samples) {
            if (sample == null || sample.sampleId == null || !sampleIds.add(sample.sampleId)
                    || sample.features == null
                    || !sample.features.keySet().equals(new HashSet<>(bundle.featureColumns))) {
                throw new IllegalArgumentException("Flink parity sample identity or feature set is invalid");
            }
            for (Float value : sample.features.values()) {
                if (value == null || !Float.isFinite(value)) {
                    throw new IllegalArgumentException("Flink parity sample contains a non-finite feature");
                }
            }
        }
    }

    static FeatureStat featureStat(Map<String, Float> values) {
        FeatureStat.Builder builder = FeatureStat.newBuilder();
        for (Map.Entry<String, Float> entry : values.entrySet()) {
            float value = entry.getValue();
            switch (entry.getKey()) {
                case "pps": builder.setPps(value); break;
                case "bps": builder.setBps(value); break;
                case "up_down_ratio": builder.setUpDownRatio(value); break;
                case "pktlen_mean": builder.setPktlenMean(value); break;
                case "pktlen_std": builder.setPktlenStd(value); break;
                case "iat_mean_ms": builder.setIatMeanMs(value); break;
                case "iat_std_ms": builder.setIatStdMs(value); break;
                case "active_mean_ms": builder.setActiveMeanMs(value); break;
                case "idle_mean_ms": builder.setIdleMeanMs(value); break;
                case "duration_ms": builder.setDurationMs((int) value); break;
                case "tcp_flag_syn_cnt": builder.setTcpFlagSynCnt((int) value); break;
                case "tcp_flag_ack_cnt": builder.setTcpFlagAckCnt((int) value); break;
                case "tcp_init_win_bytes_fwd": builder.setTcpInitWinBytesFwd((int) value); break;
                case "tcp_init_win_bytes_bwd": builder.setTcpInitWinBytesBwd((int) value); break;
                case "protocol": builder.setProtocol((int) value); break;
                default: throw new IllegalArgumentException(
                        "unsupported governed FeatureStat column: " + entry.getKey());
            }
        }
        return builder.build();
    }

    static double percentile(List<Double> sorted, double quantile) {
        if (sorted == null || sorted.isEmpty() || quantile <= 0.0 || quantile > 1.0) {
            throw new IllegalArgumentException("invalid percentile population or quantile");
        }
        int index = Math.max(0, (int) Math.ceil(sorted.size() * quantile) - 1);
        return sorted.get(index);
    }

    private static long peakRssBytes() {
        Path status = Path.of("/proc/self/status");
        if (Files.isRegularFile(status)) {
            try (BufferedReader reader = Files.newBufferedReader(status, StandardCharsets.UTF_8)) {
                String line;
                while ((line = reader.readLine()) != null) {
                    if (line.startsWith("VmHWM:")) {
                        String[] fields = line.trim().split("\\s+");
                        return Long.parseLong(fields[1]) * 1024L;
                    }
                }
            } catch (Exception ignored) {
                // Fall through to a positive JVM-owned memory observation.
            }
        }
        return Math.max(1L, Runtime.getRuntime().totalMemory());
    }

    private static String sha256(Path path) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (java.io.InputStream input = Files.newInputStream(path)) {
            byte[] block = new byte[1024 * 1024];
            int count;
            while ((count = input.read(block)) >= 0) {
                if (count > 0) digest.update(block, 0, count);
            }
        }
        return hex(digest.digest());
    }

    private static String sha256(byte[] value) throws Exception {
        return hex(MessageDigest.getInstance("SHA-256").digest(value));
    }

    private static String hex(byte[] value) {
        StringBuilder result = new StringBuilder(value.length * 2);
        for (byte item : value) result.append(String.format("%02x", item));
        return result.toString();
    }

    public static final class InputBundle implements Serializable {
        private static final long serialVersionUID = 1L;
        @JsonProperty("schema_version") public int schemaVersion;
        @JsonProperty("run_id") public String runId;
        @JsonProperty("profile_id") public String profileId;
        @JsonProperty("profile_sha256") public String profileSha256;
        @JsonProperty("candidate_sha256") public String candidateSha256;
        @JsonProperty("model_id") public String modelId;
        @JsonProperty("model_version") public String modelVersion;
        @JsonProperty("model_artifact_sha256") public String modelArtifactSha256;
        @JsonProperty("feature_columns_sha256") public String featureColumnsSha256;
        @JsonProperty("sample_set_sha256") public String sampleSetSha256;
        @JsonProperty("bundle_sha256") public String bundleSha256;
        @JsonProperty("input_name") public String inputName;
        @JsonProperty("feature_columns") public List<String> featureColumns;
        @JsonProperty("decision_threshold") public double decisionThreshold;
        @JsonProperty("warmup_iterations") public int warmupIterations;
        @JsonProperty("measured_iterations") public int measuredIterations;
        @JsonProperty("flink_parallelism") public int flinkParallelism;
        public List<Sample> samples;
    }

    public static final class Sample implements Serializable {
        private static final long serialVersionUID = 1L;
        @JsonProperty("sample_id") public String sampleId;
        public Map<String, Float> features;

        public Sample() {}
    }

    public static final class InferenceTask implements Serializable {
        private static final long serialVersionUID = 1L;
        public int ordinal;
        public int iteration;
        public boolean warmup;
        public Sample sample;

        public InferenceTask() {}

        InferenceTask(int ordinal, int iteration, boolean warmup, Sample sample) {
            this.ordinal = ordinal;
            this.iteration = iteration;
            this.warmup = warmup;
            this.sample = sample;
        }
    }

    public static final class InferenceResult implements Serializable {
        private static final long serialVersionUID = 1L;
        public int ordinal;
        public boolean warmup;
        public String sampleId;
        public float score;
        public long latencyNanos;
        public long cpuNanos;

        public InferenceResult() {}

        InferenceResult(int ordinal, boolean warmup, String sampleId, float score,
                        long latencyNanos, long cpuNanos) {
            this.ordinal = ordinal;
            this.warmup = warmup;
            this.sampleId = sampleId;
            this.score = score;
            this.latencyNanos = latencyNanos;
            this.cpuNanos = cpuNanos;
        }
    }

    public static final class FlinkOnnxInference
            extends RichMapFunction<InferenceTask, InferenceResult> {
        private static final long serialVersionUID = 1L;
        private final String modelPath;
        private final String inputName;
        private final List<String> featureColumns;
        private transient OrtEnvironment environment;
        private transient OrtSession session;
        private transient ThreadMXBean cpu;

        FlinkOnnxInference(String modelPath, String inputName, List<String> featureColumns) {
            this.modelPath = modelPath;
            this.inputName = inputName;
            this.featureColumns = List.copyOf(featureColumns);
        }

        @Override
        public void open(Configuration parameters) throws Exception {
            environment = OrtEnvironment.getEnvironment();
            session = environment.createSession(modelPath, new OrtSession.SessionOptions());
            cpu = ManagementFactory.getThreadMXBean();
            if (cpu.isThreadCpuTimeSupported() && !cpu.isThreadCpuTimeEnabled()) {
                cpu.setThreadCpuTimeEnabled(true);
            }
        }

        @Override
        public InferenceResult map(InferenceTask task) throws Exception {
            FeatureStat feature = featureStat(task.sample.features);
            float[] vector = FeatureStatVectorizer.vectorize(feature, featureColumns);
            long cpuStarted = cpu.isCurrentThreadCpuTimeSupported()
                    ? cpu.getCurrentThreadCpuTime() : 0L;
            long started = System.nanoTime();
            float score = GovernedModelPackageLoader.runBaseline(
                    environment, session, inputName, vector);
            long latency = System.nanoTime() - started;
            long cpuElapsed = cpuStarted == 0L ? 0L
                    : Math.max(0L, cpu.getCurrentThreadCpuTime() - cpuStarted);
            return new InferenceResult(
                    task.ordinal, task.warmup, task.sample.sampleId,
                    score, latency, cpuElapsed);
        }

        @Override
        public void close() throws Exception {
            if (session != null) session.close();
            session = null;
            environment = null;
        }
    }
}
