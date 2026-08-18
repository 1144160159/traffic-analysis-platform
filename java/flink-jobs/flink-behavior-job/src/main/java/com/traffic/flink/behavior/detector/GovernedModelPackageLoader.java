package com.traffic.flink.behavior.detector;

import ai.onnxruntime.OnnxTensor;
import ai.onnxruntime.OrtEnvironment;
import ai.onnxruntime.OrtSession;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.FeatureStatVectorizer;
import com.traffic.flink.behavior.model.MinioModelLoader;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.TrustedModelSigningKey;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.nio.file.Files;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.Signature;
import java.util.ArrayList;
import java.util.Base64;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.TreeMap;
import java.util.UUID;
import java.util.zip.ZipEntry;
import java.util.zip.ZipFile;

/** Verifies and warms a signed baseline/GNN package without exposing it as active. */
public final class GovernedModelPackageLoader implements AutoCloseable {
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Set<String> REQUIRED_GNN_ENTRIES = Set.of(
            "feature_mean.npy", "feature_std.npy", "w0.npy", "b0.npy",
            "w1.npy", "b1.npy", "best_epoch.npy", "validation_loss.npy");
    private static final UUID PACKAGE_NAMESPACE =
            UUID.fromString("612ba5a8-cdb8-5cf2-9de8-273eab2c24d2");

    private final BehaviorJobConfig config;
    private final MinioModelLoader loader;
    private final PublicKey trustedPublicKey;

    public GovernedModelPackageLoader(BehaviorJobConfig config) {
        this(config, "default");
    }

    public GovernedModelPackageLoader(BehaviorJobConfig config, String cacheNamespace) {
        this.config = config;
        if (cacheNamespace == null || !cacheNamespace.matches("^[a-zA-Z0-9._-]+$")) {
            throw new IllegalArgumentException("model cache namespace is invalid");
        }
        this.loader = new MinioModelLoader(
                Path.of(config.getModelPath(), cacheNamespace).toString(), config.getModelCacheSize());
        this.loader.initialize();
        this.trustedPublicKey = TrustedModelSigningKey.fromConfig(config).publicKey();
    }

    public ShadowPackage stage(ModelUpdateEvent event) throws Exception {
        validateEvent(event);
        Path manifestPath = loader.downloadImmutableFile(
                event.getArtifactManifestUri(), event.getArtifactManifestSha256(), event.getPackageSha256());
        ObjectNode manifest = (ObjectNode) MAPPER.readTree(manifestPath.toFile());
        requireText(manifest, "package_id", event.getPackageId());
        requireText(manifest, "package_sha256", event.getPackageSha256());
        requireText(manifest, "model_version", event.getVersion());
        requireText(manifest, "tenant_id", event.getTenantId());
        requireText(manifest, "model_id", event.getModelId());
        requireText(manifest, "evaluation_sha256", event.getEvaluationSha256());
        requireText(manifest, "explanation_sha256", event.getExplanationSha256());
        if (manifest.path("activation_authorized").asBoolean(true)) {
            throw new IllegalArgumentException("signed model package cannot authorize activation");
        }
        String meaningSha = sha256(canonicalWithout(manifest,
                Set.of("schema_version", "package_id", "state", "package_sha256", "signature")));
        if (!meaningSha.equals(event.getPackageSha256())) {
            throw new IllegalArgumentException("model package meaning hash mismatch");
        }
        if (!uuid5(PACKAGE_NAMESPACE, meaningSha).toString().equals(event.getPackageId())) {
            throw new IllegalArgumentException("model package ID does not match package hash");
        }
        verifySignature(manifest);
        validateCompatibility(manifest.path("compatibility"), event);
        JsonNode graph = manifest.path("graph_snapshot");
        if (!event.getGraphSnapshotId().equals(graph.path("snapshot_id").asText())
                || !event.getGraphSnapshotSha256().equals(graph.path("manifest_sha256").asText())) {
            throw new IllegalArgumentException("model package graph snapshot identity mismatch");
        }

        JsonNode artifacts = manifest.path("artifacts");
        if (!artifacts.isObject()) {
            throw new IllegalArgumentException("model package artifacts are missing");
        }
        Path baseline = downloadArtifact(event, artifacts, "baseline-model.onnx", "baseline_model");
        Path gnn = downloadArtifact(event, artifacts, "gnn-full-model.npz", "gnn_model");
        Path graphSchema = downloadArtifact(event, artifacts, "inference-graph-schema.json", "inference_graph_schema");
        downloadArtifact(event, artifacts, "compatibility-metadata.json", "compatibility_metadata");
        validateGNNArchive(gnn);
        JsonNode graphSchemaValue = MAPPER.readTree(graphSchema.toFile());
        if (graphSchemaValue.path("graph_snapshot_schema_version").asInt() != config.getModelGraphSchemaVersion()) {
            throw new IllegalArgumentException("inference graph schema version is incompatible");
        }
        JsonNode baselineContract = manifest.path("compatibility").path("baseline");
        int featureCount = baselineContract.path("input_shape").path(1).asInt(0);
        String inputName = baselineContract.path("input_name").asText();
        List<String> featureColumns = new ArrayList<>();
        baselineContract.path("feature_columns").forEach(item -> featureColumns.add(item.asText()));
        String featureColumnsSha256 = baselineContract.path("feature_columns_sha256").asText();
        if (featureCount <= 0 || inputName.isBlank() || featureColumns.size() != featureCount
                || !featureColumnsSha256.matches("^[0-9a-f]{64}$")) {
            throw new IllegalArgumentException("baseline input compatibility contract is invalid");
        }
        if (!featureColumnsSha256.equals(sha256(MAPPER.writeValueAsBytes(featureColumns)))) {
            throw new IllegalArgumentException("baseline feature column identity mismatch");
        }
        OrtEnvironment environment = OrtEnvironment.getEnvironment();
        OrtSession session = environment.createSession(baseline.toString(), new OrtSession.SessionOptions());
        try {
            float warmupScore = runBaseline(environment, session, inputName, new float[featureCount]);
            if (!Float.isFinite(warmupScore)) {
                throw new IllegalArgumentException("ONNX shadow warmup produced a non-finite score");
            }
        } catch (Exception error) {
            session.close();
            throw error;
        }
        return new ShadowPackage(event.getTenantId(), event.getModelId(), event.getVersion(),
                event.getPackageId(), event.getPackageSha256(), event.getAggregateRevision(),
                event.getThreshold(0.5f), inputName, featureColumns,
                baseline, gnn, graphSchema, environment, session);
    }

    private void validateEvent(ModelUpdateEvent event) {
        if (event == null || !"shadow-load".equals(event.getAction()) || event.getSchemaVersion() != 2) {
            throw new IllegalArgumentException("shadow-load requires model update schema_version 2");
        }
        for (String value : List.of(event.getPackageSha256(), event.getArtifactManifestSha256(),
                event.getEvaluationSha256(), event.getExplanationSha256(), event.getGraphSnapshotSha256())) {
            if (value == null || !value.matches("^[0-9a-f]{64}$")) {
                throw new IllegalArgumentException("shadow-load contains an invalid SHA-256 identity");
            }
        }
        if (event.getAggregateRevision() <= 0 || event.getArtifactManifestUri() == null
                || event.getArtifactManifestUri().isBlank() || event.getPackageId() == null
                || event.getPackageId().isBlank() || event.getGraphSnapshotId() == null
                || event.getGraphSnapshotId().isBlank()) {
            throw new IllegalArgumentException("shadow-load identity or aggregate revision is missing");
        }
    }

    private void validateCompatibility(JsonNode compatibility, ModelUpdateEvent event) {
        if (!config.getModelRuntimeContract().equals(compatibility.path("runtime_contract").asText())
                || !config.getModelRuntimeVersion().equals(compatibility.path("runtime_version").asText())
                || compatibility.path("feature_schema_version").asInt() != config.getModelFeatureSchemaVersion()
                || compatibility.path("graph_schema_version").asInt() != config.getModelGraphSchemaVersion()) {
            throw new IllegalArgumentException("model package runtime, feature or graph compatibility mismatch");
        }
        if (!"onnx".equals(compatibility.path("baseline").path("format").asText())
                || !"numpy_npz_v1".equals(compatibility.path("gnn").path("format").asText())) {
            throw new IllegalArgumentException("model package baseline or GNN format is unsupported");
        }
        ModelUpdateEvent.Compatibility eventCompatibility = event.getCompatibility();
        if (eventCompatibility == null
                || !config.getModelRuntimeContract().equals(eventCompatibility.getRuntimeContract())
                || !config.getModelRuntimeVersion().equals(eventCompatibility.getRuntimeVersion())
                || eventCompatibility.getFeatureSchemaVersion() != config.getModelFeatureSchemaVersion()
                || eventCompatibility.getGraphSchemaVersion() != config.getModelGraphSchemaVersion()) {
            throw new IllegalArgumentException("model update event compatibility is missing or inconsistent");
        }
    }

    private Path downloadArtifact(ModelUpdateEvent event, JsonNode artifacts, String name, String role) {
        JsonNode artifact = artifacts.path(name);
        if (!role.equals(artifact.path("role").asText())) {
            throw new IllegalArgumentException("model package artifact role mismatch: " + name);
        }
        String sha = artifact.path("sha256").asText();
        long size = artifact.path("size_bytes").asLong(-1);
        int separator = event.getArtifactManifestUri().lastIndexOf('/');
        if (separator < 0) {
            throw new IllegalArgumentException("model artifact manifest URI has no immutable object prefix");
        }
        String prefix = event.getArtifactManifestUri().substring(0, separator + 1);
        Path path = loader.downloadImmutableFile(prefix + name, sha, event.getPackageSha256());
        try {
            if (Files.size(path) != size || size <= 0) {
                throw new IllegalArgumentException("model package artifact size mismatch: " + name);
            }
        } catch (java.io.IOException error) {
            throw new IllegalStateException("cannot inspect model package artifact: " + name, error);
        }
        return path;
    }

    private void verifySignature(ObjectNode manifest) throws Exception {
        JsonNode signatureNode = manifest.path("signature");
        if (!"ed25519".equals(signatureNode.path("algorithm").asText())) {
            throw new IllegalArgumentException("model package signature algorithm is unsupported");
        }
        String fingerprint = sha256(trustedPublicKey.getEncoded());
        if (!fingerprint.equals(signatureNode.path("public_key_sha256").asText())) {
            throw new IllegalArgumentException("model package signing key fingerprint mismatch");
        }
        byte[] payload = canonicalWithout(manifest, Set.of("signature"));
        if (!sha256(payload).equals(signatureNode.path("signed_payload_sha256").asText())) {
            throw new IllegalArgumentException("model package signed payload identity mismatch");
        }
        Signature verifier = Signature.getInstance("Ed25519", "BC");
        verifier.initVerify(trustedPublicKey);
        verifier.update(payload);
        if (!verifier.verify(Base64.getDecoder().decode(signatureNode.path("value_base64").asText()))) {
            throw new IllegalArgumentException("model package Ed25519 signature verification failed");
        }
    }

    private static byte[] canonicalWithout(ObjectNode value, Set<String> removed) throws Exception {
        ObjectNode copy = value.deepCopy();
        removed.forEach(copy::remove);
        return MAPPER.writeValueAsBytes(canonicalValue(copy));
    }

    private static Object canonicalValue(JsonNode value) {
        if (value.isObject()) {
            TreeMap<String, Object> sorted = new TreeMap<>();
            value.fields().forEachRemaining(entry -> sorted.put(entry.getKey(), canonicalValue(entry.getValue())));
            return sorted;
        }
        if (value.isArray()) {
            List<Object> values = new ArrayList<>();
            value.forEach(item -> values.add(canonicalValue(item)));
            return values;
        }
        if (value.isNull()) return null;
        if (value.isBoolean()) return value.booleanValue();
        if (value.isIntegralNumber()) return value.longValue();
        if (value.isFloatingPointNumber()) return value.decimalValue();
        return value.asText();
    }

    private static void requireText(JsonNode value, String field, String expected) {
        if (expected == null || !expected.equals(value.path(field).asText())) {
            throw new IllegalArgumentException("model package " + field + " mismatch");
        }
    }

    private static String sha256(byte[] bytes) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(bytes);
        StringBuilder result = new StringBuilder(64);
        for (byte item : digest) result.append(String.format("%02x", item));
        return result.toString();
    }

    static float runBaseline(OrtEnvironment environment, OrtSession session,
                             String inputName, float[] vector) throws Exception {
        try (OnnxTensor input = OnnxTensor.createTensor(environment, new float[][]{vector});
             OrtSession.Result result = session.run(Map.of(inputName, input))) {
            for (int index = 0; index < result.size(); index++) {
                Float probability = positiveProbability(result.get(index).getValue());
                if (probability != null) {
                    if (!Float.isFinite(probability) || probability < 0.0f || probability > 1.0f) {
                        throw new IllegalArgumentException(
                                "ONNX shadow probability is outside the closed unit interval");
                    }
                    return probability;
                }
            }
        }
        throw new IllegalArgumentException(
                "ONNX shadow output does not expose a binary attack probability");
    }

    private static Float positiveProbability(Object value) {
        if (value instanceof float[][]) {
            float[][] rows = (float[][]) value;
            if (rows.length == 1 && rows[0].length >= 2) return rows[0][1];
        }
        if (value instanceof double[][]) {
            double[][] rows = (double[][]) value;
            if (rows.length == 1 && rows[0].length >= 2) return (float) rows[0][1];
        }
        if (value instanceof float[]) {
            float[] values = (float[]) value;
            if (values.length == 1) return values[0];
            if (values.length == 2) return values[1];
        }
        if (value instanceof double[]) {
            double[] values = (double[]) value;
            if (values.length == 1) return (float) values[0];
            if (values.length == 2) return (float) values[1];
        }
        if (value instanceof List<?>) {
            List<?> values = (List<?>) value;
            if (values.size() == 1) return positiveProbability(values.get(0));
        }
        if (value instanceof Map<?, ?>) {
            Map<?, ?> probabilities = (Map<?, ?>) value;
            for (Object key : List.of(1L, 1, "1")) {
                Object probability = probabilities.get(key);
                if (probability instanceof Number) return ((Number) probability).floatValue();
            }
        }
        return null;
    }

    private static UUID uuid5(UUID namespace, String name) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-1");
        digest.update(uuidBytes(namespace));
        digest.update(name.getBytes(java.nio.charset.StandardCharsets.UTF_8));
        byte[] bytes = digest.digest();
        bytes[6] = (byte) ((bytes[6] & 0x0f) | 0x50);
        bytes[8] = (byte) ((bytes[8] & 0x3f) | 0x80);
        java.nio.ByteBuffer buffer = java.nio.ByteBuffer.wrap(bytes, 0, 16);
        return new UUID(buffer.getLong(), buffer.getLong());
    }

    private static byte[] uuidBytes(UUID value) {
        return java.nio.ByteBuffer.allocate(16)
                .putLong(value.getMostSignificantBits())
                .putLong(value.getLeastSignificantBits()).array();
    }

    private static void validateGNNArchive(Path path) throws Exception {
        Set<String> entries = new HashSet<>();
        try (ZipFile archive = new ZipFile(path.toFile())) {
            java.util.Enumeration<? extends ZipEntry> enumeration = archive.entries();
            while (enumeration.hasMoreElements()) {
                ZipEntry entry = enumeration.nextElement();
                if (!entry.isDirectory() && entry.getSize() > 0) entries.add(entry.getName());
            }
        }
        if (!entries.containsAll(REQUIRED_GNN_ENTRIES)) {
            throw new IllegalArgumentException("GNN NPZ shadow package is missing governed tensors");
        }
    }

    @Override
    public void close() {
        loader.close();
    }

    public static final class ShadowPackage implements AutoCloseable {
        public final String tenantId;
        public final String modelId;
        public final String version;
        public final String packageId;
        public final String packageSha256;
        public final long aggregateRevision;
        public final float threshold;
        public final Path baselineModel;
        public final Path gnnModel;
        public final Path graphSchema;
        private final String inputName;
        private final List<String> featureColumns;
        private final OrtEnvironment environment;
        private final OrtSession baselineSession;
        private volatile boolean closed;

        private ShadowPackage(String tenantId, String modelId, String version,
                              String packageId, String packageSha256, long aggregateRevision,
                              float threshold, String inputName, List<String> featureColumns,
                              Path baselineModel, Path gnnModel, Path graphSchema,
                              OrtEnvironment environment, OrtSession baselineSession) {
            this.tenantId = tenantId;
            this.modelId = modelId;
            this.version = version;
            this.packageId = packageId;
            this.packageSha256 = packageSha256;
            this.aggregateRevision = aggregateRevision;
            this.threshold = threshold;
            this.inputName = inputName;
            this.featureColumns = List.copyOf(featureColumns);
            this.baselineModel = baselineModel;
            this.gnnModel = gnnModel;
            this.graphSchema = graphSchema;
            this.environment = environment;
            this.baselineSession = baselineSession;
        }

        public String getTenantId() { return tenantId; }
        public String getModelId() { return modelId; }
        public String getVersion() { return version; }
        public String getPackageId() { return packageId; }
        public String getPackageSha256() { return packageSha256; }
        public long getAggregateRevision() { return aggregateRevision; }
        public float getThreshold() { return threshold; }
        public List<String> getFeatureColumns() { return featureColumns; }

        public float predict(FeatureStat feature) throws Exception {
            if (closed) throw new IllegalStateException("governed shadow package is closed");
            return runBaseline(environment, baselineSession, inputName,
                    FeatureStatVectorizer.vectorize(feature, featureColumns));
        }

        @Override
        public synchronized void close() throws Exception {
            if (!closed) {
                closed = true;
                baselineSession.close();
            }
        }
    }
}
