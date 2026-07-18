package com.traffic.flink.behavior.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import com.fasterxml.jackson.databind.ObjectMapper;

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
    @JsonProperty("error") public String error;
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

    public static ModelUpdateAppliedAck failed(ModelUpdateEvent event, String error,
                                               int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = base(event, subtaskIndex, parallelism);
        ack.status = "failed";
        ack.error = error;
        return ack;
    }

    private static ModelUpdateAppliedAck base(ModelUpdateEvent event,
                                              int subtaskIndex, int parallelism) {
        ModelUpdateAppliedAck ack = new ModelUpdateAppliedAck();
        ack.eventId = event.getEventId();
        ack.tenantId = event.getTenantId();
        ack.modelId = event.getModelId();
        ack.version = event.getVersion();
        ack.artifactUri = event.getArtifactUri();
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
