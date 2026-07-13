package com.traffic.flink.feature.config;

import java.io.Serializable;

/**
 * 租户级配置
 * 
 * 从 PostgreSQL tenant_config 表加载，支持差异化配置
 */
public class TenantConfig implements Serializable {

    private static final long serialVersionUID = 1L;

    // 租户 ID
    private String tenantId;

    // 租户优先级（1-10，数字越大优先级越高）
    private int priority;

    // 是否启用 L2 触发
    private boolean enableL2;

    // 采样率（0.0-1.0，降级时使用）
    private float samplingRate;

    // 配额限制（EPS - Events Per Second）
    private int maxEventsPerSecond;

    // 是否启用降级
    private boolean enableDegradation;

    // 元数据
    private long updatedAt;

    public TenantConfig() {
        this.priority = 5; // 默认中等优先级
        this.enableL2 = true;
        this.samplingRate = 1.0f;
        this.maxEventsPerSecond = -1; // -1 表示无限制
        this.enableDegradation = true;
    }

    public TenantConfig(String tenantId) {
        this();
        this.tenantId = tenantId;
    }

    // ==================== Getters & Setters ====================

    public String getTenantId() {
        return tenantId;
    }

    public void setTenantId(String tenantId) {
        this.tenantId = tenantId;
    }

    public int getPriority() {
        return priority;
    }

    public void setPriority(int priority) {
        this.priority = priority;
    }

    public boolean isEnableL2() {
        return enableL2;
    }

    public void setEnableL2(boolean enableL2) {
        this.enableL2 = enableL2;
    }

    public float getSamplingRate() {
        return samplingRate;
    }

    public void setSamplingRate(float samplingRate) {
        this.samplingRate = samplingRate;
    }

    public int getMaxEventsPerSecond() {
        return maxEventsPerSecond;
    }

    public void setMaxEventsPerSecond(int maxEventsPerSecond) {
        this.maxEventsPerSecond = maxEventsPerSecond;
    }

    public boolean isEnableDegradation() {
        return enableDegradation;
    }

    public void setEnableDegradation(boolean enableDegradation) {
        this.enableDegradation = enableDegradation;
    }

    public long getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(long updatedAt) {
        this.updatedAt = updatedAt;
    }

    @Override
    public String toString() {
        return "TenantConfig{" +
                "tenantId='" + tenantId + '\'' +
                ", priority=" + priority +
                ", enableL2=" + enableL2 +
                ", samplingRate=" + samplingRate +
                ", maxEventsPerSecond=" + maxEventsPerSecond +
                ", enableDegradation=" + enableDegradation +
                ", updatedAt=" + updatedAt +
                '}';
    }
}
