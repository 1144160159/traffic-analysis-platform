package com.traffic.flink.behavior.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.DeterministicId;

import java.io.Serializable;
import java.time.Instant;

/** Data-plane acknowledgement emitted only after artifact validation and warmup. */
public class ModelUpdateAppliedAck implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @JsonProperty("schema_version") public int schemaVersion = 1;
    @JsonProperty("event_id") public String eventId;
    @JsonProperty("tenant_id") public String tenantId;
    @JsonProperty("model_id") public String modelId;
    @JsonProperty("version") public String version;
    @JsonProperty("artifact_uri") public String artifactUri;
    @JsonProperty("artifact_sha256") public String artifactSha256;
    @JsonProperty("warmup_score") public float warmupScore;
    @JsonProperty("subtask_index") public int subtaskIndex;
    @JsonProperty("parallelism") public int parallelism;
    @JsonProperty("status") public String status;
    @JsonProperty("error") public String error = "";
    @JsonProperty("ack_type") public String ackType = "model_update";
    @JsonProperty("consumer_deployment_id") public String consumerDeploymentId = "";
    @JsonProperty("consumer_profile_sha256") public String consumerProfileSha256 = "";
    @JsonProperty("runtime_contract") public String runtimeContract = "";
    @JsonProperty("runtime_version") public String runtimeVersion = "";
    @JsonProperty("feature_schema_version") public int featureSchemaVersion;
    @JsonProperty("graph_schema_version") public int graphSchemaVersion;
    @JsonProperty("supported_model_formats") public String supportedModelFormats = "";
    @JsonProperty("package_id") public String packageId = "";
    @JsonProperty("package_sha256") public String packageSha256 = "";
    @JsonProperty("aggregate_revision") public long aggregateRevision;
    @JsonProperty("rollback_id") public String rollbackId = "";
    @JsonProperty("rollback_phase") public String rollbackPhase = "";
    @JsonProperty("timestamp") public String timestamp = Instant.now().toString();

    public byte[] toJson() {
        try {
            return MAPPER.writeValueAsBytes(this);
        } catch (Exception e) {
            throw new IllegalStateException("Cannot serialize model update acknowledgement", e);
        }
    }

    public static ModelUpdateAppliedAck applied(ModelUpdateEvent event,
                                                ModelRegistryReceipt receipt,
                                                int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.status = "applied";
        ack.artifactSha256 = receipt.artifactSha256;
        ack.warmupScore = receipt.warmupScore;
        return ack;
    }

    /** Re-emits the exact successful receipt after savepoint/restart replay. */
    public static ModelUpdateAppliedAck replayed(ModelUpdateEvent event,
                                                 String artifactSha256,
                                                 float warmupScore,
                                                 int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.status = "applied";
        ack.artifactSha256 = artifactSha256;
        ack.warmupScore = warmupScore;
        return ack;
    }

	public ModelUpdateAppliedAck withConsumerIdentity(String deploymentId, String profileSha256) {
		this.consumerDeploymentId = deploymentId == null ? "" : deploymentId;
		this.consumerProfileSha256 = profileSha256 == null ? "" : profileSha256;
		return this;
	}

    public static ModelUpdateAppliedAck failed(ModelUpdateEvent event, String error,
                                               int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.status = "failed";
        ack.error = error;
        return ack;
    }

    public static ModelUpdateAppliedAck consumerReady(
            String deploymentId, String profileSha256,
            String runtimeContract, String runtimeVersion,
            int featureSchemaVersion, int graphSchemaVersion,
            int subtaskIndex, int parallelism) {
        if (deploymentId == null || deploymentId.isBlank()) {
            throw new IllegalArgumentException("consumer deployment identity is required");
        }
        if (profileSha256 == null || !profileSha256.matches("^[0-9a-f]{64}$")) {
            throw new IllegalArgumentException("consumer profile SHA-256 is required");
        }
        ModelUpdateAppliedAck ack = new ModelUpdateAppliedAck();
        ack.ackType = "consumer_ready";
        ack.status = "consumer_ready";
        ack.eventId = DeterministicId.uuid(
                "model-consumer-ready/v1", deploymentId, subtaskIndex);
        ack.tenantId = "*";
        ack.modelId = "*";
        ack.version = runtimeVersion;
        ack.artifactUri = "consumer-profile://" + deploymentId;
        ack.artifactSha256 = profileSha256;
        ack.consumerDeploymentId = deploymentId;
        ack.consumerProfileSha256 = profileSha256;
        ack.runtimeContract = runtimeContract;
        ack.runtimeVersion = runtimeVersion;
        ack.featureSchemaVersion = featureSchemaVersion;
        ack.graphSchemaVersion = graphSchemaVersion;
        ack.supportedModelFormats = ModelConsumerProfile.SUPPORTED_MODEL_FORMATS;
        ack.subtaskIndex = subtaskIndex;
        ack.parallelism = parallelism;
        return ack;
    }

    public static ModelUpdateAppliedAck shadowReady(ModelUpdateEvent event,
                                                    String verifiedPackageSha256,
                                                    int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.ackType = "shadow_load";
        ack.status = "shadow_ready";
        ack.artifactUri = event.getArtifactManifestUri();
        ack.artifactSha256 = verifiedPackageSha256;
        ack.packageId = event.getPackageId();
        ack.packageSha256 = verifiedPackageSha256;
        ack.aggregateRevision = event.getAggregateRevision();
        return ack;
    }

    public static ModelUpdateAppliedAck shadowRejected(ModelUpdateEvent event, String status,
                                                       String error, int subtaskIndex, int parallelism) {
        if (!"stale".equals(status) && !"duplicate".equals(status) && !"failed".equals(status)) {
            throw new IllegalArgumentException("invalid shadow rejection status");
        }
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.ackType = "shadow_load";
        ack.status = status;
        ack.artifactUri = event.getArtifactManifestUri();
        ack.error = error == null ? "" : error;
        ack.packageId = event.getPackageId();
        ack.packageSha256 = event.getPackageSha256();
        ack.artifactSha256 = event.getPackageSha256();
        ack.aggregateRevision = event.getAggregateRevision();
        return ack;
    }

    private static ModelUpdateAppliedAck base(ModelUpdateEvent event,
                                              int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = new ModelUpdateAppliedAck();
		ack.schemaVersion = event.getSchemaVersion();
        ack.eventId = event.getEventId();
        ack.tenantId = event.getTenantId();
        ack.modelId = event.getModelId();
        ack.version = event.getVersion();
        ack.artifactUri = event.getArtifactUri();
		ack.rollbackId = event.getRollbackId() == null ? "" : event.getRollbackId();
		ack.rollbackPhase = event.getRollbackPhase() == null ? "" : event.getRollbackPhase();
        ack.subtaskIndex = subtaskIndex;
        ack.parallelism = parallelism;
        return ack;
    }

    /** Keeps the wire POJO independent of the detector package. */
    public static class ModelRegistryReceipt {
        public final String artifactSha256;
        public final float warmupScore;

        public ModelRegistryReceipt(String artifactSha256, float warmupScore) {
            this.artifactSha256 = artifactSha256;
            this.warmupScore = warmupScore;
        }
    }
}
