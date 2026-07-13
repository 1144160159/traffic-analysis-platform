package com.traffic.flink.alert.dedup;

import java.io.Serializable;
import java.util.Objects;

/**
 * 告警去重状态
 * 
 * 存储在 Flink ValueState 中，配合 State TTL 自动清理
 * 
 * 修复内容：
 * - 添加 stateVersion 字段支持乐观锁
 * - 添加 equals() 和 hashCode() 方法
 * - 添加 copy() 方法用于状态复制
 */
public class DedupState implements Serializable {

    private static final long serialVersionUID = 2L; // 版本升级

    /**
     * 告警去重指纹
     * 格式: MD5(tenant_id:alert_type:src_ip:dst_ip:dst_port)
     */
    private String fingerprint;

    /**
     * 首次生成的告警 ID
     */
    private String alertId;

    /**
     * 首次发现时间（毫秒时间戳）
     */
    private long firstSeen;

    /**
     * 最后发现时间（毫秒时间戳）
     */
    private long lastSeen;

    /**
     * 去重计数（同一指纹的告警出现次数）
     */
    private int count;

    /**
     * 状态版本号（用于乐观锁和 ReplacingMergeTree）
     */
    private long stateVersion;

    /**
     * 默认构造函数
     */
    public DedupState() {
        this.fingerprint = "";
        this.alertId = "";
        this.firstSeen = 0;
        this.lastSeen = 0;
        this.count = 0;
        this.stateVersion = 0;
    }

    /**
     * 完整构造函数
     */
    public DedupState(
            String fingerprint,
            String alertId,
            long firstSeen,
            long lastSeen,
            int count,
            long stateVersion
    ) {
        this.fingerprint = fingerprint;
        this.alertId = alertId;
        this.firstSeen = firstSeen;
        this.lastSeen = lastSeen;
        this.count = count;
        this.stateVersion = stateVersion;
    }

    /**
     * 复制构造函数
     */
    public DedupState(DedupState other) {
        this.fingerprint = other.fingerprint;
        this.alertId = other.alertId;
        this.firstSeen = other.firstSeen;
        this.lastSeen = other.lastSeen;
        this.count = other.count;
        this.stateVersion = other.stateVersion;
    }

    /**
     * 创建状态副本
     */
    public DedupState copy() {
        return new DedupState(this);
    }

    /**
     * 检查是否在指定的去重窗口内
     * 
     * @param currentTime 当前时间（毫秒）
     * @param windowMinutes 去重窗口（分钟）
     * @return 是否在窗口内
     */
    public boolean isWithinWindow(long currentTime, long windowMinutes) {
        long windowStartTime = currentTime - (windowMinutes * 60 * 1000);
        return lastSeen >= windowStartTime;
    }

    /**
     * 更新状态（去重命中时调用）
     * 
     * @param newLastSeen 新的最后发现时间
     */
    public void update(long newLastSeen) {
        this.lastSeen = newLastSeen;
        this.count++;
        this.stateVersion++;
    }

    // ==================== Getters and Setters ====================

    public String getFingerprint() {
        return fingerprint;
    }

    public void setFingerprint(String fingerprint) {
        this.fingerprint = fingerprint;
    }

    public String getAlertId() {
        return alertId;
    }

    public void setAlertId(String alertId) {
        this.alertId = alertId;
    }

    public long getFirstSeen() {
        return firstSeen;
    }

    public void setFirstSeen(long firstSeen) {
        this.firstSeen = firstSeen;
    }

    public long getLastSeen() {
        return lastSeen;
    }

    public void setLastSeen(long lastSeen) {
        this.lastSeen = lastSeen;
    }

    public int getCount() {
        return count;
    }

    public void setCount(int count) {
        this.count = count;
    }

    public long getStateVersion() {
        return stateVersion;
    }

    public void setStateVersion(long stateVersion) {
        this.stateVersion = stateVersion;
    }

    // ==================== Object Methods ====================

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (o == null || getClass() != o.getClass()) return false;
        DedupState that = (DedupState) o;
        return firstSeen == that.firstSeen &&
                lastSeen == that.lastSeen &&
                count == that.count &&
                stateVersion == that.stateVersion &&
                Objects.equals(fingerprint, that.fingerprint) &&
                Objects.equals(alertId, that.alertId);
    }

    @Override
    public int hashCode() {
        return Objects.hash(fingerprint, alertId, firstSeen, lastSeen, count, stateVersion);
    }

    @Override
    public String toString() {
        return "DedupState{" +
                "fingerprint='" + fingerprint + '\'' +
                ", alertId='" + alertId + '\'' +
                ", firstSeen=" + firstSeen +
                ", lastSeen=" + lastSeen +
                ", count=" + count +
                ", stateVersion=" + stateVersion +
                '}';
    }

    /**
     * 转换为简短日志格式
     */
    public String toShortString() {
        return String.format("DedupState[fp=%s, alert=%s, count=%d, ver=%d]",
                fingerprint.substring(0, Math.min(8, fingerprint.length())),
                alertId,
                count,
                stateVersion);
    }
}