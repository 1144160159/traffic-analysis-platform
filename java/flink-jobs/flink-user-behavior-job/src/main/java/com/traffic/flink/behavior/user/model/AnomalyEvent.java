package com.traffic.flink.behavior.user.model;

import java.io.Serializable;
import java.time.Instant;
import java.util.UUID;

/** User behavior anomaly event — output of all detectors */
public class AnomalyEvent implements Serializable {
    private static final long serialVersionUID = 1L;

    public String anomalyId;
    public String tenantId;
    public String userId;
    public String username;
    public String detectorType;  // IMPOSSIBLE_TRAVEL | BRUTE_FORCE | PRIVILEGE_ESCALATION | UNUSUAL_ACCESS
    public String severity;      // critical | high | medium | low
    public float score;
    public String description;
    public String detailJson;    // detector-specific details
    public long detectedAt;
    public String sourceIp1;
    public String sourceIp2;     // for travel detector: IP1→IP2
    public String location1;
    public String location2;

    public AnomalyEvent() {}

    public AnomalyEvent(String tenantId, String userId, String username, String detectorType,
                        String severity, float score, String description) {
        this.anomalyId = UUID.randomUUID().toString();
        this.tenantId = tenantId;
        this.userId = userId;
        this.username = username;
        this.detectorType = detectorType;
        this.severity = severity;
        this.score = score;
        this.description = description;
        this.detectedAt = System.currentTimeMillis();
    }

    public String toJSON() {
        return String.format("{\"anomaly_id\":\"%s\",\"tenant_id\":\"%s\",\"user_id\":\"%s\",\"username\":\"%s\"," +
                "\"detector_type\":\"%s\",\"severity\":\"%s\",\"score\":%.2f,\"description\":\"%s\"," +
                "\"detail\":%s,\"source_ip1\":\"%s\",\"source_ip2\":\"%s\",\"detected_at\":%d}",
                anomalyId, tenantId, userId, username, detectorType, severity, score,
                escapeJSON(description), detailJson != null ? detailJson : "{}",
                sourceIp1 != null ? sourceIp1 : "", sourceIp2 != null ? sourceIp2 : "",
                detectedAt);
    }

    private static String escapeJSON(String s) {
        return s == null ? "" : s.replace("\\","\\\\").replace("\"","\\\"");
    }
}
