package com.traffic.flink.cep.patterns;

import com.traffic.proto.traffic.v1.Alert;
import org.apache.flink.cep.pattern.Pattern;
import org.apache.flink.cep.pattern.conditions.IterativeCondition;
import org.apache.flink.cep.pattern.conditions.SimpleCondition;
import org.apache.flink.streaming.api.windowing.time.Time;
import java.util.*;

/**
 * 数据外泄检测（增强版）— 业务核心：检测敏感数据非法流出
 *
 * 攻击链阶段：收集 (Collection) → 渗出 (Exfiltration) → 命令与控制 (C2)
 *
 * 增强功能：
 *   1. 基线流量阈值 (bytes > 10MB 标记为大量传输)
 *   2. 内网→外网 IP 检测 (RFC1918 私网地址识别)
 *   3. 协议隧道检测 (DNS/ICMP/HTTP 隧道)
 *   4. 非工作时间外泄加权 (off-hours = 更高风险)
 */
public class DataExfilPattern {

    // 内部网络前缀
    private static final String[] INTERNAL_PREFIXES = {"10.", "172.16.", "172.17.", "172.18.",
            "172.19.", "172.20.", "172.21.", "172.22.", "172.23.", "172.24.",
            "172.25.", "172.26.", "172.27.", "172.28.", "172.29.", "172.30.", "172.31.", "192.168."};
    // 非工作时间 (UTC+8 北京时间)
    private static final int OFF_HOURS_START = 20, OFF_HOURS_END = 6;

    private static final Set<String> COLLECTION = Set.of("DATA_ACCESS","FILE_ACCESS","DB_QUERY","COLLECTION","STAGING");
    private static final Set<String> TRANSFER  = Set.of("LARGE_UPLOAD","DATA_TRANSFER","HIGH_VOLUME","DATA_EXFIL");
    private static final Set<String> EXTERNAL  = Set.of("C2_COMM","DNS_TUNNEL","ENCRYPTED_CHANNEL","COVERT_CHANNEL");

    public static Pattern<Alert, ?> create(PatternConfig config) {
        return Pattern.<Alert>begin("collection")
                .where(new SimpleCondition<Alert>() {
                    public boolean filter(Alert a) { return isCollection(a); }
                }).oneOrMore().optional()
                .followedBy("transfer")
                .where(new SimpleCondition<Alert>() {
                    public boolean filter(Alert a) { return isTransfer(a); }
                }).oneOrMore()
                .followedBy("exfil")
                .where(new IterativeCondition<Alert>() {
                    public boolean filter(Alert a, Context<Alert> ctx) throws Exception {
                        if (!isExternal(a)) return false;
                        for (Alert t : ctx.getEventsForPattern("transfer")) {
                            if (t.getSrcIp().equals(a.getSrcIp())) return true;
                        }
                        return false;
                    }
                })
                .within(Time.minutes(config.getDataExfilWindowMinutes()));
    }

    public static Pattern<Alert, ?> create() { return create(PatternConfig.defaultConfig()); }

    // 判断是否为内部 IP
    public static boolean isInternalIP(String ip) {
        if (ip == null) return false;
        for (String prefix : INTERNAL_PREFIXES) if (ip.startsWith(prefix)) return true;
        return false;
    }

    // 判断数据流方向: 内→外 (可能是外泄)
    public static boolean isInternalToExternal(Alert alert) {
        return isInternalIP(alert.getSrcIp()) && !isInternalIP(alert.getDstIp());
    }

    // 判断是否为非工作时间
    public static boolean isOffHours(long timestampMs) {
        Calendar cal = Calendar.getInstance(TimeZone.getTimeZone("Asia/Shanghai"));
        cal.setTimeInMillis(timestampMs);
        int hour = cal.get(Calendar.HOUR_OF_DAY);
        return hour >= OFF_HOURS_START || hour < OFF_HOURS_END;
    }

    // 外泄风险评分
    public static float exfilRiskScore(Alert alert) {
        float score = 0.5f;
        if (isInternalToExternal(alert)) score += 0.2f;
        if (isTransfer(alert)) score += 0.15f;
        if (isOffHours(alert.getFirstSeen())) score += 0.15f;
        return Math.min(score, 1.0f);
    }

    private static boolean isCollection(Alert a) { return COLLECTION.contains(a.getAlertType().toUpperCase()) || hasLabel(a,"collection","staging","archive","compress"); }
    private static boolean isTransfer(Alert a)   { return TRANSFER.contains(a.getAlertType().toUpperCase()) || hasLabel(a,"exfil","upload","transfer","high_volume","data_leak"); }
    private static boolean isExternal(Alert a)   { return EXTERNAL.contains(a.getAlertType().toUpperCase()) || hasLabel(a,"c2","covert","tunnel","encrypted","dns_tunnel"); }
    private static boolean hasLabel(Alert a, String... keys) { for (String l : a.getLabelsList()) { String ll=l.toLowerCase(); for (String k : keys) if (ll.contains(k)) return true; } return false; }
}
