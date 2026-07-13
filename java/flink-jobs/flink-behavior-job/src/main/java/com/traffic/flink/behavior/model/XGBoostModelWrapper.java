////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/XGBoostModelWrapper.java
// XGBoost Model Wrapper — 加载 XGBoost JSON 模型并执行推理
//
// 功能:
//   1. 从本地路径加载 XGBoost model.json
//   2. 构建特征向量 (对齐 feature_columns.json 顺序)
//   3. 执行推理 → 输出概率 + 标签
//   4. 兼容现有 BehaviorModel 接口
//
// Maven 依赖:
//   ml.dmlc:xgboost4j_2.12:2.0.3
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.*;
import java.nio.file.*;
import java.util.*;

/**
 * XGBoost 模型包装器 — 加载 Python 训练导出的 model.json 并在 JVM 中推理
 *
 * 推理流程:
 *   1. XGBoost4j Booster.loadModel(modelPath)
 *   2. buildFeatureVector(featureStat) → float[]
 *   3. DMatrix dmat = new DMatrix(features, 1, n_features, NaN)
 *   4. booster.predict(dmat) → float[][]
 *   5. 阈值判断 → ModelInferenceResult
 */
public class XGBoostModelWrapper implements Serializable, AutoCloseable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(XGBoostModelWrapper.class);

    // 模型元数据
    private final String modelId;
    private final String version;
    private final String artifactUri;
    private final Path localModelPath;
    private final List<String> featureColumns;
    private final float threshold;

    // XGBoost Booster (transient — TaskManager 端通过 JNI 初始化)
    private transient Object booster;  // ml.dmlc.xgboost4j.scala.Booster

    // 是否已加载
    private volatile boolean loaded = false;

    public XGBoostModelWrapper(String modelId, String version, String artifactUri,
                               Path localModelPath, List<String> featureColumns, float threshold) {
        this.modelId = modelId;
        this.version = version;
        this.artifactUri = artifactUri;
        this.localModelPath = localModelPath;
        this.featureColumns = featureColumns;
        this.threshold = threshold;
    }

    /**
     * 加载 XGBoost 模型（在 Flink RichFunction.open() 中调用）
     */
    public void initialize() {
        try {
            Path modelFile = localModelPath.resolve("model.json");
            if (!Files.exists(modelFile)) {
                LOG.warn("Model file not found: {}", modelFile);
                return;
            }

            // XGBoost4j: Booster.loadModel(modelFile.toAbsolutePath().toString())
            Class<?> boosterClass = Class.forName("ml.dmlc.xgboost4j.scala.Booster");
            this.booster = boosterClass
                .getMethod("loadModel", String.class)
                .invoke(null, modelFile.toAbsolutePath().toString());

            loaded = true;
            LOG.info("XGBoost model loaded: modelId={}, version={}, features={}",
                    modelId, version, featureColumns.size());

            // 预热: 空推理一次验证模型可用
            float[] dummyFeatures = new float[featureColumns.size()];
            Arrays.fill(dummyFeatures, 0.0f);
            runInference(dummyFeatures);

        } catch (ClassNotFoundException e) {
            LOG.warn("XGBoost4j not available on classpath. Model inference will use fallback. " +
                     "Add ml.dmlc:xgboost4j_2.12:2.0.3 to pom.xml");
        } catch (Exception e) {
            LOG.error("Failed to load XGBoost model {}: {}", modelId, e.getMessage());
        }
    }

    /**
     * 从 FeatureStat 构建特征向量
     *
     * 特征顺序必须严格对齐训练时的 feature_columns.json
     */
    public float[] buildFeatureVector(FeatureStat feature) {
        float[] feats = new float[featureColumns.size()];

        for (int i = 0; i < featureColumns.size(); i++) {
            String col = featureColumns.get(i);
            feats[i] = extractFeatureValue(feature, col);
        }
        return feats;
    }

    /**
     * 从 FeatureStat Protobuf 消息中提取单个特征值
     */
    private float extractFeatureValue(FeatureStat feature, String columnName) {
        switch (columnName) {
            case "pps":           return feature.getPps();
            case "bps":           return feature.getBps();
            case "up_down_ratio":  return feature.getUpDownRatio();
            case "pktlen_mean":   return feature.getPktlenMean();
            case "pktlen_std":    return feature.getPktlenStd();
            case "iat_mean_ms":   return feature.getIatMeanMs();
            case "iat_std_ms":    return feature.getIatStdMs();
            case "active_mean_ms": return feature.getActiveMeanMs();
            case "idle_mean_ms":  return feature.getIdleMeanMs();
            case "duration_ms":   return (float) feature.getDurationMs();
            case "tcp_flag_syn_cnt": return (float) feature.getTcpFlagSynCnt();
            case "tcp_flag_ack_cnt": return (float) feature.getTcpFlagAckCnt();
            case "tcp_init_win_bytes_fwd": return (float) feature.getTcpInitWinBytesFwd();
            case "tcp_init_win_bytes_bwd": return (float) feature.getTcpInitWinBytesBwd();
            case "protocol":      return (float) feature.getProtocol();
            default:
                LOG.trace("Unknown feature column: {}", columnName);
                return 0.0f;
        }
    }

    /**
     * 执行 XGBoost 推理
     *
     * @param features 特征向量 (长度 = featureColumns.size())
     * @return 预测概率 (0~1)，失败返回 -1
     */
    public float predict(float[] features) {
        if (!loaded || booster == null) {
            return -1.0f;
        }
        return runInference(features);
    }

    /**
     * 对 FeatureStat 执行推理，返回 ModelInferenceResult
     */
    public ModelInferenceResult infer(FeatureStat feature) {
        float[] feats = buildFeatureVector(feature);
        float score = predict(feats);

        boolean isMalicious = score >= threshold;
        return ModelInferenceResult.success(modelId, version)
                .topLabel(isMalicious ? "malicious" : "benign")
                .topScore(score)
                .detected(isMalicious)
                .addLabel("benign", 1.0f - score)
                .addLabel("malicious", score)
                .build();
    }

    /**
     * 通过反射调用 XGBoost4j Booster.predict()
     */
    private float runInference(float[] features) {
        try {
            if (booster == null) return -1.0f;

            // DMatrix dmat = new DMatrix(features, 1, features.length, Float.NaN)
            Class<?> dmatrixClass = Class.forName("ml.dmlc.xgboost4j.scala.DMatrix");
            Object dmat = dmatrixClass
                .getConstructor(float[].class, int.class, int.class, float.class)
                .newInstance(features, 1, features.length, Float.NaN);

            // float[][] preds = booster.predict(dmat)
            Object preds = booster.getClass()
                .getMethod("predict", dmatrixClass)
                .invoke(booster, dmat);

            float[][] result = (float[][]) preds;
            return result[0][0];  // 二分类: [probability_of_class_1]

        } catch (Exception e) {
            LOG.debug("XGBoost inference failed: {}", e.getMessage());
            return -1.0f;
        }
    }

    // ================================================
    // Getters
    // ================================================

    public String getModelId() { return modelId; }
    public String getVersion() { return version; }
    public String getArtifactUri() { return artifactUri; }
    public List<String> getFeatureColumns() { return featureColumns; }
    public float getThreshold() { return threshold; }
    public boolean isLoaded() { return loaded; }

    @Override
    public void close() {
        try {
            if (booster != null) {
                booster.getClass().getMethod("dispose").invoke(booster);
            }
        } catch (Exception ignored) {}
        booster = null;
        loaded = false;
    }
}
