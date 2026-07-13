////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/model/ModelUpdateEvent.java
// MLOps Model Update Event — Kafka 消息序列化 POJO
//
// 对应 Go Model Registry API 的 ModelUpdateEvent
// 由 Argo Workflows → register_model.py → Go API → Kafka model-updates topic → Flink 消费
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.Serializable;
import java.util.Map;

/**
 * 模型更新事件（来自 MLOps 流水线）
 *
 * Kafka topic: model-updates
 * 生产者: Go Model Registry API (PublishModelUpdate)
 * 消费者: Flink Behavior Job (Broadcast Stream → hot-reload)
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class ModelUpdateEvent implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @JsonProperty("model_id")
    private String modelId;

    @JsonProperty("model_name")
    private String modelName;

    @JsonProperty("model_type")
    private String modelType;

    @JsonProperty("version")
    private String version;

    @JsonProperty("artifact_uri")
    private String artifactUri;

    @JsonProperty("action")
    private String action;  // registered, activated, deprecated

    @JsonProperty("metrics")
    private Map<String, Object> metrics;

    @JsonProperty("timestamp")
    private String timestamp;

    // ==================== Constructors ====================

    public ModelUpdateEvent() {}

    public ModelUpdateEvent(String modelId, String modelName, String modelType,
                           String version, String artifactUri, String action) {
        this.modelId = modelId;
        this.modelName = modelName;
        this.modelType = modelType;
        this.version = version;
        this.artifactUri = artifactUri;
        this.action = action;
    }

    // ==================== Serialization ====================

    public static ModelUpdateEvent fromJson(byte[] json) {
        try {
            return MAPPER.readValue(json, ModelUpdateEvent.class);
        } catch (Exception e) {
            throw new RuntimeException("Failed to deserialize ModelUpdateEvent", e);
        }
    }

    public byte[] toJson() {
        try {
            return MAPPER.writeValueAsBytes(this);
        } catch (Exception e) {
            throw new RuntimeException("Failed to serialize ModelUpdateEvent", e);
        }
    }

    // ==================== Getters & Setters ====================

    public String getModelId() { return modelId; }
    public void setModelId(String modelId) { this.modelId = modelId; }

    public String getModelName() { return modelName; }
    public void setModelName(String modelName) { this.modelName = modelName; }

    public String getModelType() { return modelType; }
    public void setModelType(String modelType) { this.modelType = modelType; }

    public String getVersion() { return version; }
    public void setVersion(String version) { this.version = version; }

    public String getArtifactUri() { return artifactUri; }
    public void setArtifactUri(String artifactUri) { this.artifactUri = artifactUri; }

    public String getAction() { return action; }
    public void setAction(String action) { this.action = action; }

    public Map<String, Object> getMetrics() { return metrics; }
    public void setMetrics(Map<String, Object> metrics) { this.metrics = metrics; }

    public String getTimestamp() { return timestamp; }
    public void setTimestamp(String timestamp) { this.timestamp = timestamp; }

    // ==================== Helpers ====================

    public boolean isActivation() {
        return "activated".equals(action) || "activate".equals(action);
    }

    public boolean isDeprecation() {
        return "deprecated".equals(action) || "deprecate".equals(action);
    }

    public float getF1Score() {
        if (metrics != null && metrics.containsKey("f1_score")) {
            Object f1 = metrics.get("f1_score");
            if (f1 instanceof Number) {
                return ((Number) f1).floatValue();
            }
        }
        return 0.0f;
    }

    @Override
    public String toString() {
        return String.format("ModelUpdateEvent{model=%s, version=%s, action=%s, artifact=%s}",
                modelName, version, action, artifactUri);
    }
}
