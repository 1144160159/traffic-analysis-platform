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

    @JsonProperty("tenant_id")
    private String tenantId;

    @JsonProperty("event_id")
    private String eventId;

    @JsonProperty("schema_version")
    private int schemaVersion;

    @JsonProperty("model_name")
    private String modelName;

    @JsonProperty("model_type")
    private String modelType;

    @JsonProperty("version")
    private String version;

    @JsonProperty("artifact_uri")
    private String artifactUri;

    @JsonProperty("artifact_manifest_uri")
    private String artifactManifestUri;

    @JsonProperty("artifact_manifest_sha256")
    private String artifactManifestSha256;

    @JsonProperty("package_id")
    private String packageId;

    @JsonProperty("package_sha256")
    private String packageSha256;

    @JsonProperty("evaluation_sha256")
    private String evaluationSha256;

    @JsonProperty("explanation_sha256")
    private String explanationSha256;

    @JsonProperty("graph_snapshot_id")
    private String graphSnapshotId;

    @JsonProperty("graph_snapshot_sha256")
    private String graphSnapshotSha256;

    @JsonProperty("aggregate_revision")
    private long aggregateRevision;

    @JsonProperty("rollback_id")
    private String rollbackId;

    @JsonProperty("rollback_phase")
    private String rollbackPhase;

    @JsonProperty("rollback_from_version")
    private String rollbackFromVersion;

    @JsonProperty("expected_active_revision")
    private long expectedActiveRevision;

    @JsonProperty("consumer_deployment_id")
    private String consumerDeploymentId;

    @JsonProperty("consumer_profile_sha256")
    private String consumerProfileSha256;

    @JsonProperty("compatibility")
    private Compatibility compatibility;

    @JsonProperty("action")
    private String action;  // registered, activated, deprecated

    @JsonProperty("metrics")
    private Metrics metrics;

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

    public String getTenantId() { return tenantId; }
    public void setTenantId(String tenantId) { this.tenantId = tenantId; }

    public String getEventId() { return eventId; }
    public void setEventId(String eventId) { this.eventId = eventId; }

    public int getSchemaVersion() { return schemaVersion; }
    public void setSchemaVersion(int schemaVersion) { this.schemaVersion = schemaVersion; }

    public String getModelName() { return modelName; }
    public void setModelName(String modelName) { this.modelName = modelName; }

    public String getModelType() { return modelType; }
    public void setModelType(String modelType) { this.modelType = modelType; }

    public String getVersion() { return version; }
    public void setVersion(String version) { this.version = version; }

    public String getArtifactUri() { return artifactUri; }
    public void setArtifactUri(String artifactUri) { this.artifactUri = artifactUri; }
    public String getArtifactManifestUri() { return artifactManifestUri; }
    public void setArtifactManifestUri(String value) { this.artifactManifestUri = value; }
    public String getArtifactManifestSha256() { return artifactManifestSha256; }
    public void setArtifactManifestSha256(String value) { this.artifactManifestSha256 = value; }
    public String getPackageId() { return packageId; }
    public void setPackageId(String value) { this.packageId = value; }
    public String getPackageSha256() { return packageSha256; }
    public void setPackageSha256(String value) { this.packageSha256 = value; }
    public String getEvaluationSha256() { return evaluationSha256; }
    public void setEvaluationSha256(String value) { this.evaluationSha256 = value; }
    public String getExplanationSha256() { return explanationSha256; }
    public void setExplanationSha256(String value) { this.explanationSha256 = value; }
    public String getGraphSnapshotId() { return graphSnapshotId; }
    public void setGraphSnapshotId(String value) { this.graphSnapshotId = value; }
    public String getGraphSnapshotSha256() { return graphSnapshotSha256; }
    public void setGraphSnapshotSha256(String value) { this.graphSnapshotSha256 = value; }
    public long getAggregateRevision() { return aggregateRevision; }
    public void setAggregateRevision(long value) { this.aggregateRevision = value; }
    public String getRollbackId() { return rollbackId; }
    public void setRollbackId(String value) { this.rollbackId = value; }
    public String getRollbackPhase() { return rollbackPhase; }
    public void setRollbackPhase(String value) { this.rollbackPhase = value; }
    public String getRollbackFromVersion() { return rollbackFromVersion; }
    public void setRollbackFromVersion(String value) { this.rollbackFromVersion = value; }
    public long getExpectedActiveRevision() { return expectedActiveRevision; }
    public void setExpectedActiveRevision(long value) { this.expectedActiveRevision = value; }
    public String getConsumerDeploymentId() { return consumerDeploymentId; }
    public void setConsumerDeploymentId(String value) { this.consumerDeploymentId = value; }
    public String getConsumerProfileSha256() { return consumerProfileSha256; }
    public void setConsumerProfileSha256(String value) { this.consumerProfileSha256 = value; }
    public Compatibility getCompatibility() { return compatibility; }
    public void setCompatibility(Compatibility value) { this.compatibility = value; }

    public String getAction() { return action; }
    public void setAction(String action) { this.action = action; }

    public Metrics getMetrics() { return metrics; }
    public void setMetrics(Metrics metrics) { this.metrics = metrics; }

    public String getTimestamp() { return timestamp; }
    public void setTimestamp(String timestamp) { this.timestamp = timestamp; }

    // ==================== Helpers ====================

    public boolean isActivation() {
        return "activated".equals(action)
                || "activate".equals(action)
                || "rollback-activated".equals(action)
                || "rollback-compensate".equals(action);
    }

    public boolean isDeprecation() {
        return "deprecated".equals(action) || "deprecate".equals(action);
    }

    public float getF1Score() {
        return metrics == null || metrics.f1Score == null ? 0.0f : metrics.f1Score;
    }

    public String getArtifactSha256() {
        if (metrics == null) {
            return "";
        }
        return metrics.artifactSha256 == null ? "" : metrics.artifactSha256;
    }

    public float getThreshold(float defaultValue) {
        if (metrics == null) {
            return defaultValue;
        }
        return metrics.threshold == null ? defaultValue : metrics.threshold;
    }

    @Override
    public String toString() {
        return String.format("ModelUpdateEvent{eventId=%s, model=%s, version=%s, action=%s, artifact=%s}",
                eventId, modelName, version, action, artifactUri);
    }

    /** Strongly typed Flink state; extra signed-manifest details are verified from the artifact. */
    @JsonIgnoreProperties(ignoreUnknown = true)
    public static class Compatibility implements Serializable {
        private static final long serialVersionUID = 1L;

        @JsonProperty("runtime_contract") private String runtimeContract;
        @JsonProperty("runtime_version") private String runtimeVersion;
        @JsonProperty("feature_schema_version") private int featureSchemaVersion;
        @JsonProperty("graph_schema_version") private int graphSchemaVersion;

        public Compatibility() {}

        public String getRuntimeContract() { return runtimeContract; }
        public void setRuntimeContract(String value) { this.runtimeContract = value; }
        public String getRuntimeVersion() { return runtimeVersion; }
        public void setRuntimeVersion(String value) { this.runtimeVersion = value; }
        public int getFeatureSchemaVersion() { return featureSchemaVersion; }
        public void setFeatureSchemaVersion(int value) { this.featureSchemaVersion = value; }
        public int getGraphSchemaVersion() { return graphSchemaVersion; }
        public void setGraphSchemaVersion(int value) { this.graphSchemaVersion = value; }
    }

    /** Only fields consumed by the runtime are retained; arbitrary maps never enter Flink state. */
    @JsonIgnoreProperties(ignoreUnknown = true)
    public static class Metrics implements Serializable {
        private static final long serialVersionUID = 1L;

        @JsonProperty("f1_score") private Float f1Score;
        @JsonProperty("artifact_sha256") private String artifactSha256;
        @JsonProperty("threshold") private Float threshold;

        public Metrics() {}

        public Float getF1Score() { return f1Score; }
        public void setF1Score(Float value) { this.f1Score = value; }
        public String getArtifactSha256() { return artifactSha256; }
        public void setArtifactSha256(String value) { this.artifactSha256 = value; }
        public Float getThreshold() { return threshold; }
        public void setThreshold(Float value) { this.threshold = value; }
    }
}
