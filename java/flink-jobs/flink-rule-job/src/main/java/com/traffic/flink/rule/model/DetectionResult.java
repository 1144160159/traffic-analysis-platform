package com.traffic.flink.rule.model;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * 检测结果模型
 * 
 * 由 RuleMatcher 返回
 */
public class DetectionResult implements Serializable {

    private static final long serialVersionUID = 1L;

    private String ruleId;
    private String ruleName;
    private RuleType ruleType;
    private Severity severity;
    private List<String> labels;
    private float score;
    private Map<String, String> evidence;

    private DetectionResult(Builder builder) {
        this.ruleId = builder.ruleId;
        this.ruleName = builder.ruleName;
        this.ruleType = builder.ruleType;
        this.severity = builder.severity;
        this.labels = builder.labels;
        this.score = builder.score;
        this.evidence = builder.evidence;
    }

    // ==================== Getters ====================

    public String getRuleId() {
        return ruleId;
    }

    public String getRuleName() {
        return ruleName;
    }

    public RuleType getRuleType() {
        return ruleType;
    }

    public Severity getSeverity() {
        return severity;
    }

    public List<String> getLabels() {
        return labels;
    }

    public float getScore() {
        return score;
    }

    public Map<String, String> getEvidence() {
        return evidence;
    }

    // ==================== Builder ====================

    public static Builder builder() {
        return new Builder();
    }

    public static class Builder {
        private String ruleId;
        private String ruleName;
        private RuleType ruleType;
        private Severity severity = Severity.MEDIUM;
        private List<String> labels = new ArrayList<>();
        private float score = 1.0f;
        private Map<String, String> evidence = new HashMap<>();

        public Builder ruleId(String ruleId) {
            this.ruleId = ruleId;
            return this;
        }

        public Builder ruleName(String ruleName) {
            this.ruleName = ruleName;
            return this;
        }

        public Builder ruleType(RuleType ruleType) {
            this.ruleType = ruleType;
            return this;
        }

        public Builder severity(Severity severity) {
            this.severity = severity;
            return this;
        }

        public Builder labels(List<String> labels) {
            this.labels = labels != null ? new ArrayList<>(labels) : new ArrayList<>();
            return this;
        }

        public Builder addLabel(String label) {
            this.labels.add(label);
            return this;
        }

        public Builder score(float score) {
            this.score = score;
            return this;
        }

        public Builder evidence(Map<String, String> evidence) {
            this.evidence = evidence != null ? new HashMap<>(evidence) : new HashMap<>();
            return this;
        }

        public Builder addEvidence(String key, String value) {
            this.evidence.put(key, value);
            return this;
        }

        public DetectionResult build() {
            return new DetectionResult(this);
        }
    }

    // ---- 便捷工厂方法 ----

    /** 创建非匹配结果 */
    public static DetectionResult noMatch(String ruleId, String reason) {
        return builder()
                .ruleId(ruleId)
                .ruleName(reason)
                .ruleType(RuleType.THRESHOLD)
                .score(0.0f)
                .build();
    }

    /** 创建匹配结果 */
    public static DetectionResult matched(String ruleId, String ruleName,
                                           String detectionType, String label,
                                           double score, String evidenceSummary,
                                           Map<String, String> evidence) {
        return builder()
                .ruleId(ruleId)
                .ruleName(ruleName)
                .ruleType(RuleType.THRESHOLD)
                .severity(scoreToSeverity((float) score))
                .addLabel(detectionType)
                .addLabel(label)
                .score((float) score)
                .evidence(evidence)
                .addEvidence("summary", evidenceSummary)
                .build();
    }

    private static Severity scoreToSeverity(float score) {
        if (score >= 0.9f) return Severity.CRITICAL;
        if (score >= 0.7f) return Severity.HIGH;
        if (score >= 0.5f) return Severity.MEDIUM;
        if (score >= 0.3f) return Severity.LOW;
        return Severity.LOW;
    }

    @Override
    public String toString() {
        return "DetectionResult{" +
                "ruleId='" + ruleId + '\'' +
                ", ruleName='" + ruleName + '\'' +
                ", ruleType=" + ruleType +
                ", severity=" + severity +
                ", score=" + score +
                ", evidenceCount=" + evidence.size() +
                '}';
    }
}