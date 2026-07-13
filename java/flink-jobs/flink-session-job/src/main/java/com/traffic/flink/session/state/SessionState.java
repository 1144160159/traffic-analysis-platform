package com.traffic.flink.session.state;

import com.traffic.flink.session.aggregator.SessionAccumulator;

import java.io.Serializable;

/**
 * Session 状态封装（用于 KeyedProcessFunction）
 * 
 * 包含：
 * 1. 累加器（聚合数据）
 * 2. Timer 时间戳（Idle/Active）
 * 3. 段索引（Active Timeout 切分计数）
 */
public class SessionState implements Serializable {

    private static final long serialVersionUID = 1L;

    // ==================== 聚合数据 ====================
    private SessionAccumulator accumulator;

    // ==================== Timer 时间戳 ====================
    private Long idleTimerTimestamp;   // Idle Timeout Timer
    private Long activeTimerTimestamp; // Active Timeout Timer

    // ==================== 段索引（Active Timeout 切分计数） ====================
    private int segmentIndex;          // 当前会话被切分的段序号（从 0 开始）

    // ==================== 会话元数据 ====================
    private long sessionStartTime;     // 会话首次开始时间（跨 segment 不变）
    private String sessionIdBase;      // 会话 ID 基础部分（用于生成确定性 session_id）

    public SessionState() {
        this.accumulator = new SessionAccumulator();
        this.segmentIndex = 0;
        this.sessionStartTime = 0;
    }

    /**
     * 重置累加器（Active Timeout 触发后调用）
     * 注意：保留 sessionIdBase 和 sessionStartTime
     */
    public void resetAccumulatorForNewSegment() {
        this.accumulator.reset();
        this.segmentIndex++;
        this.idleTimerTimestamp = null;
        this.activeTimerTimestamp = null;
    }

    /**
     * 完全清空状态（Idle Timeout 或会话结束）
     */
    public void clear() {
        this.accumulator.reset();
        this.segmentIndex = 0;
        this.sessionStartTime = 0;
        this.sessionIdBase = null;
        this.idleTimerTimestamp = null;
        this.activeTimerTimestamp = null;
    }

    // ==================== Getters & Setters ====================

    public SessionAccumulator getAccumulator() {
        return accumulator;
    }

    public void setAccumulator(SessionAccumulator accumulator) {
        this.accumulator = accumulator;
    }

    public Long getIdleTimerTimestamp() {
        return idleTimerTimestamp;
    }

    public void setIdleTimerTimestamp(Long idleTimerTimestamp) {
        this.idleTimerTimestamp = idleTimerTimestamp;
    }

    public Long getActiveTimerTimestamp() {
        return activeTimerTimestamp;
    }

    public void setActiveTimerTimestamp(Long activeTimerTimestamp) {
        this.activeTimerTimestamp = activeTimerTimestamp;
    }

    public int getSegmentIndex() {
        return segmentIndex;
    }

    public void setSegmentIndex(int segmentIndex) {
        this.segmentIndex = segmentIndex;
    }

    public long getSessionStartTime() {
        return sessionStartTime;
    }

    public void setSessionStartTime(long sessionStartTime) {
        this.sessionStartTime = sessionStartTime;
    }

    public String getSessionIdBase() {
        return sessionIdBase;
    }

    public void setSessionIdBase(String sessionIdBase) {
        this.sessionIdBase = sessionIdBase;
    }

    @Override
    public String toString() {
        return "SessionState{" +
                "segmentIndex=" + segmentIndex +
                ", sessionStartTime=" + sessionStartTime +
                ", accumulator=" + accumulator +
                '}';
    }
}