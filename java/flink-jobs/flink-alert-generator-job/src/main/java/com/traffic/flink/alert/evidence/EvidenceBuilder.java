package com.traffic.flink.alert.evidence;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.traffic.proto.traffic.v1.Evidence;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.UUID;

/**
 * Evidence 构建器 (修复版)
 * 
 * 帮助构建结构化的证据对象
 * 
 * 修复内容：
 * - 确保 Arkime 链接正确设置
 * - 使用 LinkedHashMap 保持字段顺序
 * - 添加更多验证和默认值处理
 * - 支持可视化 URL
 */
public class EvidenceBuilder {

    private static final Logger LOG = LoggerFactory.getLogger(EvidenceBuilder.class);
    
    private static final ObjectMapper MAPPER = new ObjectMapper()
            .configure(SerializationFeature.ORDER_MAP_ENTRIES_BY_KEYS, true);

    // 必填字段
    private final String tenantId;
    private final String evidenceId;
    private final String alertId;

    // 可选字段
    private String type = "generic";
    private String summary = "";
    private Map<String, Object> metrics = new LinkedHashMap<>();
    private Map<String, String> snippets = new LinkedHashMap<>();
    private String arkimeLink = "";
    private String visualizationUrl = "";
    private float confidence = 1.0f;

    /**
     * 构造函数
     * 
     * @param tenantId 租户 ID
     * @param evidenceId 证据 ID
     * @param alertId 关联的告警 ID
     */
    public EvidenceBuilder(String tenantId, String evidenceId, String alertId) {
        if (tenantId == null || tenantId.isEmpty()) {
            throw new IllegalArgumentException("tenantId cannot be null or empty");
        }
        if (evidenceId == null || evidenceId.isEmpty()) {
            throw new IllegalArgumentException("evidenceId cannot be null or empty");
        }
        if (alertId == null || alertId.isEmpty()) {
            throw new IllegalArgumentException("alertId cannot be null or empty");
        }
        
        this.tenantId = tenantId;
        this.evidenceId = evidenceId;
        this.alertId = alertId;
    }

    /**
     * 设置证据类型
     * 
     * @param type 证据类型（如 "behavior_detection", "rule_match", "payload_analysis"）
     */
    public EvidenceBuilder setType(String type) {
        this.type = type != null ? type : "generic";
        return this;
    }

    /**
     * 设置证据摘要
     * 
     * @param summary 人类可读的摘要描述
     */
    public EvidenceBuilder setSummary(String summary) {
        this.summary = summary != null ? summary : "";
        return this;
    }

    /**
     * 设置置信度
     * 
     * @param confidence 置信度 (0.0 - 1.0)
     */
    public EvidenceBuilder setConfidence(float confidence) {
        this.confidence = Math.max(0.0f, Math.min(1.0f, confidence));
        return this;
    }

    /**
     * 设置 Arkime 链接
     * 
     * @param arkimeLink Arkime 查询链接
     */
    public EvidenceBuilder setArkimeLink(String arkimeLink) {
        this.arkimeLink = arkimeLink != null ? arkimeLink : "";
        return this;
    }

    /**
     * 设置可视化 URL
     * 
     * @param visualizationUrl 可视化面板 URL（如 Grafana 仪表盘）
     */
    public EvidenceBuilder setVisualizationUrl(String visualizationUrl) {
        this.visualizationUrl = visualizationUrl != null ? visualizationUrl : "";
        return this;
    }

    /**
     * 添加指标
     * 
     * @param key 指标名称
     * @param value 指标值（支持数值、字符串、布尔值）
     */
    public EvidenceBuilder addMetric(String key, Object value) {
        if (key != null && !key.isEmpty() && value != null) {
            metrics.put(key, value);
        }
        return this;
    }

    /**
     * 添加多个指标
     * 
     * @param metricsToAdd 指标 Map
     */
    public EvidenceBuilder addMetrics(Map<String, Object> metricsToAdd) {
        if (metricsToAdd != null) {
            for (Map.Entry<String, Object> entry : metricsToAdd.entrySet()) {
                addMetric(entry.getKey(), entry.getValue());
            }
        }
        return this;
    }

    /**
     * 添加代码片段引用
     * 
     * @param key 片段名称
     * @param value 片段内容或引用
     */
    public EvidenceBuilder addSnippet(String key, String value) {
        if (key != null && !key.isEmpty() && value != null) {
            snippets.put(key, value);
        }
        return this;
    }

    /**
     * 添加 Payload 十六进制摘要
     * 
     * @param payloadBytes 原始 Payload 字节
     * @param maxBytes 最大截取字节数
     */
    public EvidenceBuilder addPayloadHex(byte[] payloadBytes, int maxBytes) {
        if (payloadBytes != null && payloadBytes.length > 0) {
            int length = Math.min(payloadBytes.length, maxBytes);
            StringBuilder hex = new StringBuilder();
            for (int i = 0; i < length; i++) {
                hex.append(String.format("%02x", payloadBytes[i] & 0xFF));
                if (i < length - 1) {
                    hex.append(" ");
                }
            }
            if (payloadBytes.length > maxBytes) {
                hex.append(" ...(truncated)");
            }
            addSnippet("payload_hex", hex.toString());
            addMetric("payload_length", payloadBytes.length);
        }
        return this;
    }

    /**
     * 添加规则匹配信息
     * 
     * @param ruleId 规则 ID
     * @param ruleName 规则名称
     * @param matchOffset 匹配偏移量
     * @param matchLength 匹配长度
     */
    public EvidenceBuilder addRuleMatch(String ruleId, String ruleName, int matchOffset, int matchLength) {
        addMetric("rule_id", ruleId);
        addMetric("rule_name", ruleName);
        addMetric("match_offset", matchOffset);
        addMetric("match_length", matchLength);
        return this;
    }

    /**
     * 构建 Evidence 对象
     * 
     * @param timestamp 证据时间戳（毫秒）
     * @return Evidence Protobuf 对象
     */
    public Evidence build(long timestamp) {
        // 序列化 metrics 和 snippets 为 JSON
        String metricsJson = toJson(metrics);
        String snippetsJson = toJson(snippets);

        // 生成确定性 event_id（基于输入参数，确保幂等）
        String eventId = generateDeterministicUUID(
                tenantId, evidenceId, alertId, String.valueOf(timestamp));

        // 当前摄入时间
        long ingestTs = System.currentTimeMillis();

        // 构建 Evidence
        Evidence evidence = Evidence.newBuilder()
                .setTenantId(tenantId)
                .setEvidenceId(evidenceId)
                .setAlertId(alertId)
                .setTs(timestamp)
                .setType(type)
                .setSummary(summary)
                .setMetricsJson(metricsJson)
                .setSnippetRefJson(snippetsJson)
                .setArkimeLink(arkimeLink)
                .setVisualizationUrl(visualizationUrl)
                .setConfidence(confidence)
                .setEventId(eventId)
                .setIngestTs(ingestTs)
                .build();

        LOG.debug("Evidence built: evidenceId={}, alertId={}, type={}, confidence={}",
                evidenceId, alertId, type, confidence);

        return evidence;
    }

    /**
     * 生成确定性 UUID（基于输入参数，确保幂等）
     * 使用 MD5 哈希生成 UUID v3 风格的确定性 UUID
     */
    private static String generateDeterministicUUID(String... components) {
        try {
            String combined = String.join("::", components);
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(combined.getBytes(StandardCharsets.UTF_8));
            // 将 MD5 哈希格式化为 UUID (设置 version 3 variant)
            long msb = 0;
            long lsb = 0;
            for (int i = 0; i < 8; i++) {
                msb = (msb << 8) | (hash[i] & 0xff);
            }
            for (int i = 8; i < 16; i++) {
                lsb = (lsb << 8) | (hash[i] & 0xff);
            }
            // UUID v3: version=3 (0x3000), variant=10 (0x8000)
            msb = (msb & 0xffffffffffff0fffL) | 0x0000000000003000L;
            lsb = (lsb & 0x3fffffffffffffffL) | 0x8000000000000000L;
            return new UUID(msb, lsb).toString();
        } catch (Exception e) {
            LOG.error("Failed to generate deterministic UUID: {}", e.getMessage());
            return UUID.randomUUID().toString();
        }
    }

    /**
     * 将 Map 转换为 JSON 字符串
     */
    private String toJson(Map<String, ?> map) {
        if (map == null || map.isEmpty()) {
            return "{}";
        }
        
        try {
            return MAPPER.writeValueAsString(map);
        } catch (JsonProcessingException e) {
            LOG.error("Failed to serialize map to JSON: {}", e.getMessage());
            return "{}";
        }
    }

    /**
     * 获取当前构建器状态的摘要（用于调试）
     */
    @Override
    public String toString() {
        return String.format(
                "EvidenceBuilder{tenantId='%s', evidenceId='%s', alertId='%s', " +
                        "type='%s', confidence=%.2f, metricsCount=%d, snippetsCount=%d, " +
                        "hasArkimeLink=%b, hasVisualizationUrl=%b}",
                tenantId, evidenceId, alertId,
                type, confidence, metrics.size(), snippets.size(),
                !arkimeLink.isEmpty(), !visualizationUrl.isEmpty()
        );
    }
}