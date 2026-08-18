package com.traffic.flink.behavior.user.baseline;

import java.io.Serializable;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/** Immutable version payload retained in Flink checkpointed broadcast state. */
public final class BaselineSnapshot implements Serializable {
    private static final long serialVersionUID = 1L;

    private final String tenantId;
    private final String baselineId;
    private final String baselineKind;
    private final String algorithmVersion;
    private final long baselineVersion;
    private final long definitionRevision;
    private final String candidateSha256;
    private final String snapshotSha256;
    private final String activationRequestEventId;
    private final Map<String, Object> thresholdSpec;
    private final Map<String, Object> statistics;

    public BaselineSnapshot(BaselineLifecycleEvent event) {
        tenantId = event.tenantId;
        baselineId = event.baselineId;
        baselineKind = event.baselineKind;
        algorithmVersion = event.algorithmVersion;
        baselineVersion = event.baselineVersion;
        definitionRevision = event.definitionRevision;
        candidateSha256 = event.candidateSha256;
        snapshotSha256 = event.snapshotSha256;
        activationRequestEventId = event.eventId;
        thresholdSpec = Collections.unmodifiableMap(new HashMap<>(event.thresholdSpec));
        statistics = Collections.unmodifiableMap(new HashMap<>(event.statistics));
    }

    public String getTenantId() { return tenantId; }
    public String getBaselineId() { return baselineId; }
    public String getBaselineKind() { return baselineKind; }
    public String getAlgorithmVersion() { return algorithmVersion; }
    public long getBaselineVersion() { return baselineVersion; }
    public long getDefinitionRevision() { return definitionRevision; }
    public String getCandidateSha256() { return candidateSha256; }
    public String getSnapshotSha256() { return snapshotSha256; }
    public String getActivationRequestEventId() { return activationRequestEventId; }
    public Map<String, Object> getThresholdSpec() { return thresholdSpec; }
    public Map<String, Object> getStatistics() { return statistics; }

    public int intThreshold(String name, int fallback, int minimum, int maximum) {
        Object value = thresholdSpec.get(name);
        if (!(value instanceof Number)) return fallback;
        long parsed = ((Number) value).longValue();
        if (parsed < minimum || parsed > maximum) return fallback;
        return (int) parsed;
    }

    public long longThreshold(String name, long fallback, long minimum, long maximum) {
        Object value = thresholdSpec.get(name);
        if (!(value instanceof Number)) return fallback;
        long parsed = ((Number) value).longValue();
        return parsed < minimum || parsed > maximum ? fallback : parsed;
    }

    public boolean matchesActivated(BaselineLifecycleEvent event) {
        return event != null
                && tenantId.equals(event.tenantId)
                && baselineId.equals(event.baselineId)
                && baselineVersion == event.baselineVersion
                && candidateSha256.equals(event.candidateSha256)
                && snapshotSha256.equals(event.snapshotSha256);
    }
}
