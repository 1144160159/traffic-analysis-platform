package com.traffic.flink.rule.model;

/**
 * 严重程度枚举
 */
public enum Severity {
    INFO("info", 1),
    LOW("low", 2),
    MEDIUM("medium", 3),
    HIGH("high", 4),
    CRITICAL("critical", 5);

    private final String value;
    private final int level;

    Severity(String value, int level) {
        this.value = value;
        this.level = level;
    }

    public String getValue() {
        return value;
    }

    public int getLevel() {
        return level;
    }

    public static Severity fromString(String value) {
        for (Severity severity : values()) {
            if (severity.value.equalsIgnoreCase(value)) {
                return severity;
            }
        }
        return MEDIUM;
    }

    public boolean isHigherThan(Severity other) {
        return this.level > other.level;
    }
}