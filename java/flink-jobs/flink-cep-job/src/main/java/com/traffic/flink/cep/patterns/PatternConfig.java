package com.traffic.flink.cep.patterns;

import java.io.Serializable;

/**
 * 模式配置
 */
public class PatternConfig implements Serializable {
    
    private static final long serialVersionUID = 1L;

    // 扫描-利用模式
    private int scanExploitWindowMinutes = 30;
    private int minScanCount = 1;

    // 暴力破解模式
    private int bruteForceWindowMinutes = 10;
    private int minFailedAttempts = 5;

    // 横向移动模式
    private int lateralMovementWindowMinutes = 60;
    private int minHops = 2;

    // 数据外泄模式
    private int dataExfilWindowMinutes = 30;
    private long minExfilBytes = 10 * 1024 * 1024; // 10 MB

    // C2 通信模式
    private int c2WindowMinutes = 120;
    private int minBeaconCount = 5;
    private float beaconIntervalToleranceRatio = 0.1f;

    // Getters and Setters
    public int getScanExploitWindowMinutes() {
        return scanExploitWindowMinutes;
    }

    public void setScanExploitWindowMinutes(int scanExploitWindowMinutes) {
        this.scanExploitWindowMinutes = scanExploitWindowMinutes;
    }

    public int getMinScanCount() {
        return minScanCount;
    }

    public void setMinScanCount(int minScanCount) {
        this.minScanCount = minScanCount;
    }

    public int getBruteForceWindowMinutes() {
        return bruteForceWindowMinutes;
    }

    public void setBruteForceWindowMinutes(int bruteForceWindowMinutes) {
        this.bruteForceWindowMinutes = bruteForceWindowMinutes;
    }

    public int getMinFailedAttempts() {
        return minFailedAttempts;
    }

    public void setMinFailedAttempts(int minFailedAttempts) {
        this.minFailedAttempts = minFailedAttempts;
    }

    public int getLateralMovementWindowMinutes() {
        return lateralMovementWindowMinutes;
    }

    public void setLateralMovementWindowMinutes(int lateralMovementWindowMinutes) {
        this.lateralMovementWindowMinutes = lateralMovementWindowMinutes;
    }

    public int getMinHops() {
        return minHops;
    }

    public void setMinHops(int minHops) {
        this.minHops = minHops;
    }

    public int getDataExfilWindowMinutes() {
        return dataExfilWindowMinutes;
    }

    public void setDataExfilWindowMinutes(int dataExfilWindowMinutes) {
        this.dataExfilWindowMinutes = dataExfilWindowMinutes;
    }

    public long getMinExfilBytes() {
        return minExfilBytes;
    }

    public void setMinExfilBytes(long minExfilBytes) {
        this.minExfilBytes = minExfilBytes;
    }

    public int getC2WindowMinutes() {
        return c2WindowMinutes;
    }

    public void setC2WindowMinutes(int c2WindowMinutes) {
        this.c2WindowMinutes = c2WindowMinutes;
    }

    public int getMinBeaconCount() {
        return minBeaconCount;
    }

    public void setMinBeaconCount(int minBeaconCount) {
        this.minBeaconCount = minBeaconCount;
    }

    public float getBeaconIntervalToleranceRatio() {
        return beaconIntervalToleranceRatio;
    }

    public void setBeaconIntervalToleranceRatio(float beaconIntervalToleranceRatio) {
        this.beaconIntervalToleranceRatio = beaconIntervalToleranceRatio;
    }

    public static PatternConfig defaultConfig() {
        return new PatternConfig();
    }
}