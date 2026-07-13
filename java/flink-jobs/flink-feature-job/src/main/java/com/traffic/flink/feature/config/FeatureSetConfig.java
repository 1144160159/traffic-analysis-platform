package com.traffic.flink.feature.config;

import java.io.Serializable;
import java.util.HashMap;
import java.util.Map;

/**
 * Feature Set 配置
 * 
 * 从 PostgreSQL feature_sets 表加载，支持 BroadcastState 热更新
 */
public class FeatureSetConfig implements Serializable {

    private static final long serialVersionUID = 1L;

    // 特征集 ID
    private String featureSetId;

    // Schema 版本
    private String schemaVersion;

    // IAT 阈值（区分 Active/Idle）
    private float iatThresholdMs;

    // 是否启用 L2 触发
    private boolean enableL2Trigger;

    // L2 触发阈值配置
    private L2TriggerThresholds l2Thresholds;

    // 特征权重（用于未来扩展）
    private Map<String, Float> featureWeights;

    // 元数据
    private String description;
    private long updatedAt;

    public FeatureSetConfig() {
        this.featureWeights = new HashMap<>();
        this.l2Thresholds = new L2TriggerThresholds();
    }

    public FeatureSetConfig(String featureSetId, String schemaVersion) {
        this();
        this.featureSetId = featureSetId;
        this.schemaVersion = schemaVersion;
    }

    // ==================== Getters & Setters ====================

    public String getFeatureSetId() {
        return featureSetId;
    }

    public void setFeatureSetId(String featureSetId) {
        this.featureSetId = featureSetId;
    }

    public String getSchemaVersion() {
        return schemaVersion;
    }

    public void setSchemaVersion(String schemaVersion) {
        this.schemaVersion = schemaVersion;
    }

    public float getIatThresholdMs() {
        return iatThresholdMs;
    }

    public void setIatThresholdMs(float iatThresholdMs) {
        this.iatThresholdMs = iatThresholdMs;
    }

    public boolean isEnableL2Trigger() {
        return enableL2Trigger;
    }

    public void setEnableL2Trigger(boolean enableL2Trigger) {
        this.enableL2Trigger = enableL2Trigger;
    }

    public L2TriggerThresholds getL2Thresholds() {
        return l2Thresholds;
    }

    public void setL2Thresholds(L2TriggerThresholds l2Thresholds) {
        this.l2Thresholds = l2Thresholds;
    }

    public Map<String, Float> getFeatureWeights() {
        return featureWeights;
    }

    public void setFeatureWeights(Map<String, Float> featureWeights) {
        this.featureWeights = featureWeights;
    }

    public String getDescription() {
        return description;
    }

    public void setDescription(String description) {
        this.description = description;
    }

    public long getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(long updatedAt) {
        this.updatedAt = updatedAt;
    }

    @Override
    public String toString() {
        return "FeatureSetConfig{" +
                "featureSetId='" + featureSetId + '\'' +
                ", schemaVersion='" + schemaVersion + '\'' +
                ", iatThresholdMs=" + iatThresholdMs +
                ", enableL2Trigger=" + enableL2Trigger +
                ", l2Thresholds=" + l2Thresholds +
                ", updatedAt=" + updatedAt +
                '}';
    }

    /**
     * L2 触发阈值配置
     */
    public static class L2TriggerThresholds implements Serializable {
        private static final long serialVersionUID = 1L;

        private float highPpsThreshold = 10000.0f;
        private float highBpsThreshold = 1e9f;
        private float encryptedStdPayloadThreshold = 100.0f;
        private int tlsPort = 443;
        private int httpPort = 80;

        public float getHighPpsThreshold() {
            return highPpsThreshold;
        }

        public void setHighPpsThreshold(float highPpsThreshold) {
            this.highPpsThreshold = highPpsThreshold;
        }

        public float getHighBpsThreshold() {
            return highBpsThreshold;
        }

        public void setHighBpsThreshold(float highBpsThreshold) {
            this.highBpsThreshold = highBpsThreshold;
        }

        public float getEncryptedStdPayloadThreshold() {
            return encryptedStdPayloadThreshold;
        }

        public void setEncryptedStdPayloadThreshold(float encryptedStdPayloadThreshold) {
            this.encryptedStdPayloadThreshold = encryptedStdPayloadThreshold;
        }

        public int getTlsPort() {
            return tlsPort;
        }

        public void setTlsPort(int tlsPort) {
            this.tlsPort = tlsPort;
        }

        public int getHttpPort() {
            return httpPort;
        }

        public void setHttpPort(int httpPort) {
            this.httpPort = httpPort;
        }

        @Override
        public String toString() {
            return "L2TriggerThresholds{" +
                    "highPpsThreshold=" + highPpsThreshold +
                    ", highBpsThreshold=" + highBpsThreshold +
                    ", encryptedStdPayloadThreshold=" + encryptedStdPayloadThreshold +
                    ", tlsPort=" + tlsPort +
                    ", httpPort=" + httpPort +
                    '}';
        }
    }
}
