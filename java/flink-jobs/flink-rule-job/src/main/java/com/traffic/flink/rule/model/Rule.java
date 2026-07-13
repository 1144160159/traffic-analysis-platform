package com.traffic.flink.rule.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

import java.io.Serializable;
import java.util.*;

/**
 * 规则模型
 * 
 * 从 Kafka rule.updates topic 反序列化
 * 对应 PostgreSQL rules 表结构
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class Rule implements Serializable {

    private static final long serialVersionUID = 1L;

    @JsonProperty("rule_id")
    private String ruleId;

    @JsonProperty("tenant_id")
    private String tenantId;

    @JsonProperty("name")
    private String name;

    @JsonProperty("rule_type")
    private String ruleTypeStr;

    @JsonProperty("engine")
    private String engine = "internal";

    @JsonProperty("description")
    private String description;

    @JsonProperty("conditions")
    private Map<String, Object> conditions = new HashMap<>();

    @JsonProperty("labels")
    private List<String> labels = new ArrayList<>();

    @JsonProperty("severity")
    private String severityStr;

    @JsonProperty("enabled")
    private boolean enabled = false;

    @JsonProperty("priority")
    private int priority = 50;

    @JsonProperty("version")
    private long version = 1;

    @JsonProperty("status")
    private String status = "draft";

    @JsonProperty("action")
    private String actionStr;

    @JsonProperty("created_by")
    private String createdBy;

    @JsonProperty("updated_by")
    private String updatedBy;

    @JsonProperty("created_at")
    private Long createdAt;

    @JsonProperty("updated_at")
    private Long updatedAt;

    // ==================== Getters & Setters ====================

    public String getRuleId() {
        return ruleId;
    }

    public void setRuleId(String ruleId) {
        this.ruleId = ruleId;
    }

    public String getTenantId() {
        return tenantId;
    }

    public void setTenantId(String tenantId) {
        this.tenantId = tenantId;
    }

    public String getName() {
        return name;
    }

    public void setName(String name) {
        this.name = name;
    }

    public String getRuleTypeStr() {
        return ruleTypeStr;
    }

    public void setRuleTypeStr(String ruleTypeStr) {
        this.ruleTypeStr = ruleTypeStr;
    }

    public RuleType getType() {
        try {
            return RuleType.valueOf(ruleTypeStr.toUpperCase());
        } catch (Exception e) {
            return RuleType.THRESHOLD;
        }
    }

    public void setType(RuleType type) {
        if (type == null) return;
        this.ruleTypeStr = type.name().toLowerCase();
    }

    public String getEngine() {
        return engine;
    }

    public void setEngine(String engine) {
        this.engine = engine;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public Map<String, Object> getConditions() {
        return conditions;
    }

    public void setConditions(Map<String, Object> conditions) {
        this.conditions = conditions;
    }

    public List<String> getLabels() {
        return labels;
    }

    public void setLabels(List<String> labels) {
        this.labels = labels;
    }

    public String getSeverityStr() {
        return severityStr;
    }

    public void setSeverityStr(String severityStr) {
        this.severityStr = severityStr;
    }

    public Severity getSeverity() {
        try {
            return Severity.valueOf(severityStr.toUpperCase());
        } catch (Exception e) {
            return Severity.MEDIUM;
        }
    }

    public void setSeverity(String severity) {
        this.severityStr = severity;
    }

    public boolean isEnabled() {
        return enabled;
    }

    public void setEnabled(boolean enabled) {
        this.enabled = enabled;
    }

    public int getPriority() {
        return priority;
    }

    public void setPriority(int priority) {
        this.priority = priority;
    }

    public long getVersion() {
        return version;
    }

    public void setVersion(long version) {
        this.version = version;
    }

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public String getActionStr() {
        return actionStr;
    }

    public void setActionStr(String actionStr) {
        this.actionStr = actionStr;
    }

    public RuleAction getAction() {
        try {
            return RuleAction.valueOf(actionStr.toUpperCase());
        } catch (Exception e) {
            return RuleAction.UPDATE;
        }
    }

    public String getCreatedBy() {
        return createdBy;
    }

    public void setCreatedBy(String createdBy) {
        this.createdBy = createdBy;
    }

    public String getUpdatedBy() {
        return updatedBy;
    }

    public void setUpdatedBy(String updatedBy) {
        this.updatedBy = updatedBy;
    }

    public Long getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(Long createdAt) {
        this.createdAt = createdAt;
    }

    public Long getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(Long updatedAt) {
        this.updatedAt = updatedAt;
    }

    // ==================== Condition 辅助方法 ====================

    /**
     * 获取条件值（字符串）
     */
    public String getConditionAsString(String key, String defaultValue) {
        Object value = conditions.get(key);
        return value != null ? value.toString() : defaultValue;
    }

    /**
     * 获取条件值（整数）
     */
    public int getConditionAsInt(String key, int defaultValue) {
        Object value = conditions.get(key);
        if (value == null) return defaultValue;
        
        if (value instanceof Number) {
            return ((Number) value).intValue();
        }
        
        try {
            return Integer.parseInt(value.toString());
        } catch (NumberFormatException e) {
            return defaultValue;
        }
    }

    /**
     * 获取条件值（长整数）
     */
    public long getConditionAsLong(String key, long defaultValue) {
        Object value = conditions.get(key);
        if (value == null) return defaultValue;
        
        if (value instanceof Number) {
            return ((Number) value).longValue();
        }
        
        try {
            return Long.parseLong(value.toString());
        } catch (NumberFormatException e) {
            return defaultValue;
        }
    }

    /**
     * 获取条件值（浮点数）
     */
    public double getConditionAsDouble(String key, double defaultValue) {
        Object value = conditions.get(key);
        if (value == null) return defaultValue;
        
        if (value instanceof Number) {
            return ((Number) value).doubleValue();
        }
        
        try {
            return Double.parseDouble(value.toString());
        } catch (NumberFormatException e) {
            return defaultValue;
        }
    }

    /**
     * 获取条件值（布尔）
     */
    public boolean getConditionAsBoolean(String key, boolean defaultValue) {
        Object value = conditions.get(key);
        if (value == null) return defaultValue;
        
        if (value instanceof Boolean) {
            return (Boolean) value;
        }
        
        return Boolean.parseBoolean(value.toString());
    }

    /**
     * 获取条件值（列表）
     */
    @SuppressWarnings("unchecked")
    public List<String> getConditionAsList(String key) {
        Object value = conditions.get(key);
        if (value == null) return Collections.emptyList();
        
        if (value instanceof List) {
            return (List<String>) value;
        }
        
        if (value instanceof String) {
            String str = (String) value;
            if (str.startsWith("[") && str.endsWith("]")) {
                str = str.substring(1, str.length() - 1);
            }
            return Arrays.asList(str.split(","));
        }
        
        return Collections.emptyList();
    }

    @Override
    public String toString() {
        return "Rule{" +
                "ruleId='" + ruleId + '\'' +
                ", tenantId='" + tenantId + '\'' +
                ", name='" + name + '\'' +
                ", type=" + getType() +
                ", severity=" + getSeverity() +
                ", enabled=" + enabled +
                ", version=" + version +
                '}';
    }
}