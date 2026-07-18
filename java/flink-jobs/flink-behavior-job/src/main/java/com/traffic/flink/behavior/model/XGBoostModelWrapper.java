package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;
import ml.dmlc.xgboost4j.java.Booster;
import ml.dmlc.xgboost4j.java.DMatrix;
import ml.dmlc.xgboost4j.java.XGBoost;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** A production BehaviorModel backed by an immutable XGBoost JSON artifact. */
public class XGBoostModelWrapper implements BehaviorModel {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(XGBoostModelWrapper.class);

    private final String modelId;
    private final String version;
    private final String artifactUri;
    private final Path localModelPath;
    private final List<String> featureColumns;
    private final float threshold;

    private transient Booster booster;
    private volatile boolean loaded;
    private volatile float warmupScore = Float.NaN;

    public XGBoostModelWrapper(String modelId, String version, String artifactUri,
                               Path localModelPath, List<String> featureColumns, float threshold) {
        this.modelId = modelId;
        this.version = version;
        this.artifactUri = artifactUri;
        this.localModelPath = localModelPath;
        this.featureColumns = List.copyOf(featureColumns);
        this.threshold = threshold;
    }

    @Override
    public synchronized void initialize() throws Exception {
        if (loaded) {
            return;
        }
        if (!Files.isRegularFile(localModelPath) || Files.size(localModelPath) == 0) {
            throw new IllegalStateException("Model artifact is missing or empty: " + localModelPath);
        }
        if (featureColumns.isEmpty()) {
            throw new IllegalStateException("feature_columns.json must not be empty");
        }
        Booster candidate = XGBoost.loadModel(localModelPath.toAbsolutePath().toString());
        long artifactFeatureCount = candidate.getNumFeature();
        if (artifactFeatureCount != featureColumns.size()) {
            candidate.dispose();
            throw new IllegalStateException("Feature count mismatch: artifact=" + artifactFeatureCount
                    + ", contract=" + featureColumns.size());
        }
        this.booster = candidate;
        float[] warmup = new float[featureColumns.size()];
        Arrays.fill(warmup, 0.0f);
        float score = runInference(warmup);
        if (!Float.isFinite(score)) {
            candidate.dispose();
            this.booster = null;
            throw new IllegalStateException("Model warmup inference returned non-finite score");
        }
        this.warmupScore = score;
        this.loaded = true;
        LOG.info("XGBoost model initialized and warmed: modelId={}, version={}, features={}, score={}",
                modelId, version, featureColumns.size(), score);
    }

    public float[] buildFeatureVector(FeatureStat feature) {
        float[] values = new float[featureColumns.size()];
        for (int index = 0; index < featureColumns.size(); index++) {
            values[index] = extractFeatureValue(feature, featureColumns.get(index));
        }
        return values;
    }

    private float extractFeatureValue(FeatureStat feature, String columnName) {
        switch (columnName) {
            case "pps": return feature.getPps();
            case "bps": return feature.getBps();
            case "up_down_ratio": return feature.getUpDownRatio();
            case "pktlen_mean": return feature.getPktlenMean();
            case "pktlen_std": return feature.getPktlenStd();
            case "iat_mean_ms": return feature.getIatMeanMs();
            case "iat_std_ms": return feature.getIatStdMs();
            case "active_mean_ms": return feature.getActiveMeanMs();
            case "idle_mean_ms": return feature.getIdleMeanMs();
            case "duration_ms": return feature.getDurationMs();
            case "tcp_flag_syn_cnt": return feature.getTcpFlagSynCnt();
            case "tcp_flag_ack_cnt": return feature.getTcpFlagAckCnt();
            case "tcp_init_win_bytes_fwd": return feature.getTcpInitWinBytesFwd();
            case "tcp_init_win_bytes_bwd": return feature.getTcpInitWinBytesBwd();
            case "protocol": return feature.getProtocol();
            default:
                LOG.trace("Unknown feature column {}, using zero", columnName);
                return 0.0f;
        }
    }

    public float predict(float[] features) {
        if (!isReady()) {
            return Float.NaN;
        }
        return runInference(features);
    }

    private float runInference(float[] features) {
        DMatrix matrix = null;
        try {
            matrix = new DMatrix(features, 1, features.length, Float.NaN);
            float[][] predictions = booster.predict(matrix);
            if (predictions.length == 0 || predictions[0].length == 0) {
                return Float.NaN;
            }
            return predictions[0][0];
        } catch (Exception e) {
            throw new IllegalStateException("XGBoost inference failed for " + modelId, e);
        } finally {
            if (matrix != null) {
                matrix.dispose();
            }
        }
    }

    @Override
    public ModelInferenceResult infer(FeatureStat feature) {
        if (!isReady()) {
            return ModelInferenceResult.failure(modelId, version, "Model not ready");
        }
        try {
            float score = predict(buildFeatureVector(feature));
            if (!Float.isFinite(score)) {
                return ModelInferenceResult.failure(modelId, version, "Non-finite inference score");
            }
            boolean detected = score >= threshold;
            return ModelInferenceResult.success(modelId, version)
                    .topLabel(detected ? "malicious" : "benign")
                    .topScore(score)
                    .detected(detected)
                    .addLabel("benign", 1.0f - score)
                    .addLabel("malicious", score)
                    .build();
        } catch (Exception e) {
            return ModelInferenceResult.failure(modelId, version, e.getMessage());
        }
    }

    @Override
    public List<ModelInferenceResult> inferBatch(List<FeatureStat> features) {
        List<ModelInferenceResult> results = new ArrayList<>(features.size());
        for (FeatureStat feature : features) {
            results.add(infer(feature));
        }
        return results;
    }

    @Override public String getName() { return modelId; }
    @Override public String getVersion() { return version; }
    @Override public String getDescription() { return "XGBoost artifact " + artifactUri; }
    @Override public boolean isReady() { return loaded && booster != null; }
    @Override public String checkForUpdate() { return null; }
    @Override public List<String> getSupportedLabels() { return List.of("benign", "malicious"); }
    @Override public float getThreshold() { return threshold; }

    @Override
    public BehaviorModel reload() throws Exception {
        XGBoostModelWrapper replacement = new XGBoostModelWrapper(
                modelId, version, artifactUri, localModelPath, featureColumns, threshold);
        replacement.initialize();
        return replacement;
    }

    public String getModelId() { return modelId; }
    public String getArtifactUri() { return artifactUri; }
    public List<String> getFeatureColumns() { return featureColumns; }
    public float getWarmupScore() { return warmupScore; }

    @Override
    public synchronized void close() {
        if (booster != null) {
            booster.dispose();
        }
        booster = null;
        loaded = false;
    }
}
