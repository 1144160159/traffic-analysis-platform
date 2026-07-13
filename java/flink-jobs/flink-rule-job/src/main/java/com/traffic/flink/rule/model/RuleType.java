package com.traffic.flink.rule.model;

/**
 * 规则类型枚举
 */
public enum RuleType {
    /**
     * 阈值规则（如 PPS > 1000）
     */
    THRESHOLD("threshold"),

    /**
     * IP 黑名单
     */
    BLACKLIST("blacklist"),

    /**
     * 端口扫描检测
     */
    PORT_SCAN("port_scan"),

    /**
     * 暴力破解检测
     */
    BRUTE_FORCE("brute_force"),

    /**
     * 数据外泄检测
     */
    DATA_EXFIL("data_exfil"),

    /**
     * DGA 域名检测
     */
    DGA("dga"),

    /**
     * 隧道检测
     */
    TUNNEL("tunnel"),

    /**
     * C2 通信检测
     */
    C2("c2"),

    /**
     * 异常流量检测
     */
    ANOMALY("anomaly"),

    /**
     * 协议异常检测 (ProtocolAnomalyMatcher)
     */
    PROTOCOL_ANOMALY("protocol_anomaly"),

    /**
     * TLS/JA3 指纹检测 (TlsFingerprintMatcher)
     */
    TLS_FINGERPRINT("tls_fingerprint"),

    /**
     * 自定义规则
     */
    CUSTOM("custom");

    private final String value;

    RuleType(String value) {
        this.value = value;
    }

    public String getValue() {
        return value;
    }

    public static RuleType fromString(String value) {
        for (RuleType type : values()) {
            if (type.value.equalsIgnoreCase(value)) {
                return type;
            }
        }
        return THRESHOLD;
    }
}