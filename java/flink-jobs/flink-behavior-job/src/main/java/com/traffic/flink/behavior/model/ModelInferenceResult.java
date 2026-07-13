package com.traffic.flink.behavior.model;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 模型推理结果
 * 
 * 包含：
 * 1. 检测到的标签列表及其置信度
 * 2. 特征重要性（用于可解释性）
 * 3. 推理耗时
 * 4. 其他元数据
 */
public class ModelInferenceResult implements Serializable {

    private static final long serialVersionUID = 1L;

    /**
     * 模型名称
     */
    private final String modelName;

    /**
     * 模型版本
     */
    private final String modelVersion;

    /**
     * 检测到的标签列表
     */
    private final List<String> labels;

    /**
     * 标签对应的置信度分数
     */
    private final List<Float> scores;

    /**
     * Top1 标签
     */
    private final String topLabel;

    /**
     * Top1 置信度
     */
    private final float topScore;

    /**
     * 是否检测到异常
     */
    private final boolean detected;

    /**
     * 特征重要性（用于可解释性）
     */
    private final Map<String, Float> featureImportance;

    /**
     * 原始模型输出（调试用）
     */
    private final Map<String, Object> rawOutput;

    /**
     * 推理耗时（毫秒）
     */
    private final long inferenceTimeMs;

    /**
     * 错误信息（如果有）
     */
    private final String errorMessage;

    private ModelInferenceResult(Builder builder) {
        this.modelName = builder.modelName;
        this.modelVersion = builder.modelVersion;
        this.labels = Collections.unmodifiableList(new ArrayList<>(builder.labels));
        this.scores = Collections.unmodifiableList(new ArrayList<>(builder.scores));
        this.topLabel = builder.topLabel;
        this.topScore = builder.topScore;
        this.detected = builder.detected;
        this.featureImportance = builder.featureImportance != null ?
                Collections.unmodifiableMap(new HashMap<>(builder.featureImportance)) :
                Collections.emptyMap();
        this.rawOutput = builder.rawOutput != null ?
                Collections.unmodifiableMap(new HashMap<>(builder.rawOutput)) :
                Collections.emptyMap();
        this.inferenceTimeMs = builder.inferenceTimeMs;
        this.errorMessage = builder.errorMessage;
    }

    /**
     * 创建成功的推理结果
     */
    public static Builder success(String modelName, String modelVersion) {
        return new Builder(modelName, modelVersion, false);
    }

    /**
     * 创建失败的推理结果
     */
    public static ModelInferenceResult failure(String modelName, String modelVersion, String errorMessage) {
        return new Builder(modelName, modelVersion, true)
                .errorMessage(errorMessage)
                .build();
    }

    /**
     * 创建空结果（无检测）
     */
    public static ModelInferenceResult empty(String modelName, String modelVersion) {
        return new Builder(modelName, modelVersion, false)
                .topLabel("normal")
                .topScore(1.0f)
                .detected(false)
                .build();
    }

    // ==================== Getters ====================

    public String getModelName() { return modelName; }
    public String getModelVersion() { return modelVersion; }
    public List<String> getLabels() { return labels; }
    public List<Float> getScores() { return scores; }
    public String getTopLabel() { return topLabel; }
    public float getTopScore() { return topScore; }
    public boolean isDetected() { return detected; }
    public Map<String, Float> getFeatureImportance() { return featureImportance; }
    public Map<String, Object> getRawOutput() { return rawOutput; }
    public long getInferenceTimeMs() { return inferenceTimeMs; }
    public String getErrorMessage() { return errorMessage; }
    public boolean hasError() { return errorMessage != null && !errorMessage.isEmpty(); }

    /**
     * 获取指定标签的置信度
     */
    public float getScoreForLabel(String label) {
        int index = labels.indexOf(label);
        if (index >= 0 && index < scores.size()) {
            return scores.get(index);
        }
        return 0.0f;
    }

    /**
     * 获取超过阈值的标签
     */
    public List<String> getLabelsAboveThreshold(float threshold) {
        List<String> result = new ArrayList<>();
        for (int i = 0; i < labels.size() && i < scores.size(); i++) {
            if (scores.get(i) >= threshold) {
                result.add(labels.get(i));
            }
        }
        return result;
    }

    @Override
    public String toString() {
        return "ModelInferenceResult{" +
                "modelName='" + modelName + '\'' +
                ", modelVersion='" + modelVersion + '\'' +
                ", topLabel='" + topLabel + '\'' +
                ", topScore=" + topScore +
                ", detected=" + detected +
                ", inferenceTimeMs=" + inferenceTimeMs +
                (errorMessage != null ? ", error='" + errorMessage + '\'' : "") +
                '}';
    }

    /**
     * Builder 模式
     */
    public static class Builder {
        private final String modelName;
        private final String modelVersion;
        private final boolean isError;
        private List<String> labels = new ArrayList<>();
        private List<Float> scores = new ArrayList<>();
        private String topLabel = "unknown";
        private float topScore = 0.0f;
        private boolean detected = false;
        private Map<String, Float> featureImportance;
        private Map<String, Object> rawOutput;
        private long inferenceTimeMs = 0;
        private String errorMessage;

        private Builder(String modelName, String modelVersion, boolean isError) {
            this.modelName = modelName;
            this.modelVersion = modelVersion;
            this.isError = isError;
        }

        public Builder addLabel(String label, float score) {
            labels.add(label);
            scores.add(score);
            
            // 自动更新 top
            if (score > topScore) {
                topLabel = label;
                topScore = score;
            }
            
            return this;
        }

        public Builder labels(List<String> labels) {
            this.labels = new ArrayList<>(labels);
            return this;
        }

        public Builder scores(List<Float> scores) {
            this.scores = new ArrayList<>(scores);
            return this;
        }

        public Builder topLabel(String topLabel) {
            this.topLabel = topLabel;
            return this;
        }

        public Builder topScore(float topScore) {
            this.topScore = topScore;
            return this;
        }

        public Builder detected(boolean detected) {
            this.detected = detected;
            return this;
        }

        public Builder featureImportance(Map<String, Float> featureImportance) {
            this.featureImportance = featureImportance;
            return this;
        }

        public Builder addFeatureImportance(String feature, float importance) {
            if (this.featureImportance == null) {
                this.featureImportance = new HashMap<>();
            }
            this.featureImportance.put(feature, importance);
            return this;
        }

        public Builder rawOutput(Map<String, Object> rawOutput) {
            this.rawOutput = rawOutput;
            return this;
        }

        public Builder inferenceTimeMs(long inferenceTimeMs) {
            this.inferenceTimeMs = inferenceTimeMs;
            return this;
        }

        public Builder errorMessage(String errorMessage) {
            this.errorMessage = errorMessage;
            return this;
        }

        public ModelInferenceResult build() {
            // 如果没有检测到任何标签，添加 normal
            // 但如果用户已显式设置了 topLabel（非默认值 "unknown"），则保留
            if (labels.isEmpty() && !isError) {
                if ("unknown".equals(topLabel)) {
                    topLabel = "normal";
                }
                // 用户可能通过 addLabel 设置了 labels，或显式设置了 topLabel
                if (topScore <= 0.0f) {
                    topScore = 1.0f;
                }
                labels.add(topLabel);
                scores.add(topScore);
            }

            return new ModelInferenceResult(this);
        }
    }
}