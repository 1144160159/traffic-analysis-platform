package com.traffic.flink.behavior.user.baseline;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.io.Serializable;
import java.util.Collections;
import java.util.List;
import java.util.Map;

/** Strict JSON projection of baseline.lifecycle.v1. */
public final class BaselineLifecycleEvent implements Serializable {
    private static final long serialVersionUID = 1L;

    @JsonProperty("event_id") public String eventId;
    @JsonProperty("event_type") public String eventType;
    @JsonProperty("schema_version") public int schemaVersion;
    @JsonProperty("partition_key") public String partitionKey;
    @JsonProperty("tenant_id") public String tenantId;
    @JsonProperty("baseline_id") public String baselineId;
    @JsonProperty("baseline_kind") public String baselineKind;
    @JsonProperty("algorithm_version") public String algorithmVersion;
    @JsonProperty("job_id") public String jobId;
    @JsonProperty("definition_revision") public long definitionRevision;
    @JsonProperty("target_version") public long targetVersion;
    @JsonProperty("baseline_version") public long baselineVersion;
    @JsonProperty("version_id") public String versionId;
    @JsonProperty("sample_snapshot_id") public String sampleSnapshotId;
    @JsonProperty("candidate_sha256") public String candidateSha256;
    @JsonProperty("snapshot_sha256") public String snapshotSha256;
    @JsonProperty("window_start") public String windowStart;
    @JsonProperty("window_end") public String windowEnd;
    @JsonProperty("quality_status") public String qualityStatus;
    @JsonProperty("error_code") public String errorCode;
    @JsonProperty("threshold_spec") public Map<String, Object> thresholdSpec;
    @JsonProperty("statistics") public Map<String, Object> statistics;
    @JsonProperty("approval_id") public String approvalId;
    @JsonProperty("expected_consumers") public List<String> expectedConsumers;
    @JsonProperty("acked_consumers") public List<String> ackedConsumers;
    @JsonProperty("rollback_of_version") public long rollbackOfVersion;
    @JsonProperty("retired_by_version") public long retiredByVersion;
    @JsonProperty("trace_id") public String traceId;

    @JsonIgnore public String payloadSha256;
    @JsonIgnore public long sourceTimestamp;

    public BaselineLifecycleEvent() {
        thresholdSpec = Collections.emptyMap();
        statistics = Collections.emptyMap();
        expectedConsumers = Collections.emptyList();
        ackedConsumers = Collections.emptyList();
    }

    public boolean isActivationRequested() {
        return "baseline.activation.requested.v1".equals(eventType);
    }

    public boolean isActivated() {
        return "baseline.version.activated.v1".equals(eventType);
    }

    public boolean isRetired() {
        return "baseline.version.retired.v1".equals(eventType);
    }

    public boolean addressedTo(String consumerId) {
        return expectedConsumers != null && expectedConsumers.contains(consumerId);
    }

    public String stateKey() {
        return tenantId + '\u001f' + baselineId;
    }
}
