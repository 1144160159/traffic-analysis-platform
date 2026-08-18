package com.traffic.flink.behavior.user.model;

import com.traffic.flink.common.DeterministicId;

import java.io.Serializable;

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
    public long eventVersion = 1L;
    public String replayId = "";

    public AnomalyEvent() {}

    public AnomalyEvent(String tenantId, String userId, String username, String detectorType,
                        String severity, float score, String description,
                        long detectedAt, String... sourceEventIds) {
        this.anomalyId = DeterministicId.uuidFromSorted(
                "flink-user-anomaly/v1",
                java.util.Arrays.asList(sourceEventIds),
                tenantId,
                userId,
                detectorType,
                detectedAt);
        this.tenantId = tenantId;
        this.userId = userId;
        this.username = username;
        this.detectorType = detectorType;
        this.severity = severity;
        this.score = score;
        this.description = description;
        this.detectedAt = detectedAt;
    }

    public String toJSON() {
        return String.format("{\"anomaly_id\":\"%s\",\"tenant_id\":\"%s\",\"user_id\":\"%s\",\"username\":\"%s\"," +
                "\"detector_type\":\"%s\",\"severity\":\"%s\",\"score\":%.2f,\"description\":\"%s\"," +
                "\"detail\":%s,\"source_ip1\":\"%s\",\"source_ip2\":\"%s\",\"detected_at\":%d," +
                "\"event_version\":%d,\"replay_id\":\"%s\"}",
                escapeJson(anomalyId), escapeJson(tenantId), escapeJson(userId), escapeJson(username),
                escapeJson(detectorType), escapeJson(severity), score,
                escapeJson(description), detailJson != null ? detailJson : "{}",
                escapeJson(sourceIp1), escapeJson(sourceIp2),
                detectedAt, eventVersion, escapeJson(replayId));
    }

    /**
     * JSON 字符串值转义：覆盖引号、反斜杠、控制字符与常见空白。
     * 检测器拼接 detailJson 时必须用它转义用户可控字段（IP、角色名等），
     * 否则非法 JSON 会写入 ClickHouse detail_json 列。
     */
    public static String escapeJson(String s) {
        if (s == null || s.isEmpty()) return s == null ? "" : s;
        StringBuilder sb = new StringBuilder(s.length() + 16);
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '\\': sb.append("\\\\"); break;
                case '"': sb.append("\\\""); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                case '\b': sb.append("\\b"); break;
                case '\f': sb.append("\\f"); break;
                default:
                    if (c < 0x20) {
                        sb.append(String.format("\\u%04x", (int) c));
                    } else {
                        sb.append(c);
                    }
            }
        }
        return sb.toString();
    }
}
