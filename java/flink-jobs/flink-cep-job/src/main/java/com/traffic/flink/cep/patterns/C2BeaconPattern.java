package com.traffic.flink.cep.patterns;

import com.traffic.proto.traffic.v1.Alert;
import org.apache.flink.cep.pattern.Pattern;
import org.apache.flink.cep.pattern.conditions.IterativeCondition;
import org.apache.flink.cep.pattern.conditions.SimpleCondition;
import org.apache.flink.streaming.api.windowing.time.Time;
import java.util.*;

/**
 * C2 信标检测（增强版）— 业务核心：检测周期性命令控制通信
 *
 * 攻击链阶段：命令与控制 (Command & Control)
 *
 * 增强功能：
 *   1. 周期间隔检测 + 抖动 (jitter) 容忍
 *   2. 协议隧道识别 (DNS/HTTP/HTTPS/ICMP beacon)
 *   3. Beacon 评分 (间隔规律性 + 目标固定性 + 时间持续性)
 *   4. DGA 域名检测标签
 */
public class C2BeaconPattern {

    private static final Set<String> C2_TYPES = Set.of("C2_BEACON","C2_COMM","SUSPICIOUS_DNS","PERIODIC_COMM","BEACON");
    private static final Set<String> TUNNEL_TYPES = Set.of("DNS_TUNNEL","HTTP_TUNNEL","HTTPS_TUNNEL","ICMP_TUNNEL");

    public static Pattern<Alert, ?> create(PatternConfig config) {
        return Pattern.<Alert>begin("beacon")
                .where(new SimpleCondition<Alert>() {
                    public boolean filter(Alert a) { return isC2Alert(a); }
                })
                .timesOrMore(config.getMinBeaconCount())
                .consecutive()
                .where(new IterativeCondition<Alert>() {
                    public boolean filter(Alert a, Context<Alert> ctx) throws Exception {
                        List<Alert> beacons = new ArrayList<>();
                        for (Alert beacon : ctx.getEventsForPattern("beacon")) {
                            beacons.add(beacon);
                        }
                        if (beacons == null || beacons.isEmpty()) return true;
                        Alert last = beacons.get(beacons.size()-1);
                        if (!last.getDstIp().equals(a.getDstIp())) return false;
                        if (beacons.size() >= 2)
                            return checkJitter(beacons, a, config.getBeaconIntervalToleranceRatio());
                        return true;
                    }
                })
                .within(Time.minutes(config.getC2WindowMinutes()));
    }

    public static Pattern<Alert, ?> create() { return create(PatternConfig.defaultConfig()); }

    // ---- 信标评分 ----
    public static float beaconScore(List<Alert> beacons) {
        if (beacons.size() < 3) return 0.5f;
        float score = 0.3f;
        if (beacons.size() >= 5) score += 0.2f;           // 5+ 次通信
        if (hasTunnelProtocol(beacons)) score += 0.2f;    // 使用了隧道技术
        if (isLongDuration(beacons, 30)) score += 0.2f;    // 持续 >30min
        if (hasDGAIndicator(beacons)) score += 0.1f;      // DGA 域名特征
        return Math.min(score, 1.0f);
    }

    // ---- 抖动检测 ----
    private static boolean checkJitter(List<Alert> beacons, Alert newAlert, float tolerance) {
        long total=0; for (int i=1; i<beacons.size(); i++) total += beacons.get(i).getLastSeen()-beacons.get(i-1).getLastSeen();
        long avg = total/(beacons.size()-1);
        Alert last = beacons.get(beacons.size()-1);
        long cur = newAlert.getLastSeen()-last.getLastSeen();
        // 允许抖动 + 固定 5s 容差
        return Math.abs(cur-avg) <= avg*tolerance+5000;
    }

    // ---- 协议隧道检测 ----
    private static boolean hasTunnelProtocol(List<Alert> beacons) {
        for (Alert b : beacons)
            if (b.getDstPort()==53||b.getDstPort()==443||b.getDstPort()==80||TUNNEL_TYPES.contains(b.getAlertType().toUpperCase()))
                return true;
        return false;
    }

    // ---- 持续时间 ----
    private static boolean isLongDuration(List<Alert> beacons, int minutes) {
        if (beacons.size()<2) return false;
        return (beacons.get(beacons.size()-1).getLastSeen()-beacons.get(0).getLastSeen()) > minutes*60000L;
    }

    // ---- DGA 域名检测 (简化: 高熵域名) ----
    private static boolean hasDGAIndicator(List<Alert> beacons) {
        for (Alert b : beacons) for (String l : b.getLabelsList())
            if (l.toLowerCase().contains("dga")) return true;
        return false;
    }

    private static boolean isC2Alert(Alert a) { return C2_TYPES.contains(a.getAlertType().toUpperCase()) || hasLabel(a,"c2","beacon","callback","periodic","heartbeat"); }
    private static boolean hasLabel(Alert a, String... keys) { for (String l : a.getLabelsList()) { String ll=l.toLowerCase(); for (String k : keys) if (ll.contains(k)) return true; } return false; }
}
