// Protocol Anomaly Matcher — TCP/UDP/ICMP 协议异常检测
//
// 业务价值: 检测违反协议规范的异常流量
// 支持: TCP标志位异常, TCP状态机异常, UDP异常, ICMP隧道, IP分片异常, 流异常
package com.traffic.flink.rule.matcher;

import com.traffic.flink.rule.model.DetectionResult;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.model.RuleType;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.*;

public class ProtocolAnomalyMatcher implements RuleMatcher {

    private static final Logger LOG = LoggerFactory.getLogger(ProtocolAnomalyMatcher.class);
    private static final long serialVersionUID = 1L;

    private static final Map<Integer, String> KNOWN_TCP_FLAG_ATTACKS = new LinkedHashMap<>();
    static {
        KNOWN_TCP_FLAG_ATTACKS.put(0x00, "NULL scan");
        KNOWN_TCP_FLAG_ATTACKS.put(0x01, "FIN scan");
        KNOWN_TCP_FLAG_ATTACKS.put(0x02, "SYN scan");
        KNOWN_TCP_FLAG_ATTACKS.put(0x29, "XMAS scan");
        KNOWN_TCP_FLAG_ATTACKS.put(0x03, "SYN+FIN impossible");
        KNOWN_TCP_FLAG_ATTACKS.put(0x37, "All-flags set");
        KNOWN_TCP_FLAG_ATTACKS.put(0x0B, "FIN+PSH+URG scan");
    }

    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        Map<String, Object> conditions = rule.getConditions();
        if (conditions == null) return Optional.empty();

        String anomalyType = (String) conditions.getOrDefault("anomaly_type", "tcp_flag_anomaly");

        switch (anomalyType) {
            case "tcp_flag_anomaly":   return checkTcpFlags(feature, rule, conditions, context);
            case "tcp_state_anomaly":  return checkTcpState(feature, rule, conditions, context);
            case "udp_anomaly":        return checkUdpAnomaly(feature, rule, conditions, context);
            case "icmp_anomaly":       return checkIcmpAnomaly(feature, rule, conditions, context);
            case "fragment_anomaly":   return checkFragmentAnomaly(feature, rule, conditions, context);
            case "flow_anomaly":       return checkFlowAnomaly(feature, rule, conditions, context);
            default:                   return Optional.empty();
        }
    }

    private Optional<DetectionResult> checkTcpFlags(FeatureStat feature, Rule rule,
                                                     Map<String, Object> cond, MatchContext ctx) {
        int flagsFwd = getIntField(feature, "tcp_flags_fwd", 0);
        int flagsBwd = getIntField(feature, "tcp_flags_bwd", 0);
        String direction = (String) cond.getOrDefault("direction", "any");
        int flags = 0;
        if ("fwd".equals(direction) || "any".equals(direction)) flags |= flagsFwd;
        if ("bwd".equals(direction) || "any".equals(direction)) flags |= flagsBwd;

        String attackType = ctx.getCachedAttackType(flags);
        if (attackType == null) {
            attackType = KNOWN_TCP_FLAG_ATTACKS.getOrDefault(flags, null);
            ctx.cacheAttackType(flags, attackType);
        }

        if (attackType != null) {
            return Optional.of(buildResult(rule, "protocol_anomaly.tcp_flag", attackType,
                    (float) calcTcpScore(flags),
                    String.format("TCP %s: flags=0x%02X dir=%s", attackType, flags, direction),
                    "tcp_flags", String.format("0x%02X", flags),
                    "attack_type", attackType, "direction", direction));
        }

        @SuppressWarnings("unchecked")
        List<Number> flagList = (List<Number>) cond.get("anomalous_flags");
        if (flagList != null) {
            for (Number n : flagList) {
                if (flags == n.intValue()) {
                    return Optional.of(buildResult(rule, "protocol_anomaly.tcp_flag",
                            "tcp_anomalous_flags", 0.85f,
                            "TCP anomalous flags: 0x" + Integer.toHexString(flags),
                            "tcp_flags", String.format("0x%02X", flags)));
                }
            }
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> checkTcpState(FeatureStat feature, Rule rule,
                                                     Map<String, Object> cond, MatchContext ctx) {
        long synCount = getLongField(feature, "syn_count", 0);
        long dataPackets = getLongField(feature, "data_packets", 0);

        if (synCount > 0 && dataPackets == 0) {
            long synThreshold = getLongCond(cond, "syn_threshold", 3);
            if (synCount >= synThreshold) {
                double score = Math.min(1.0, (double) synCount / synThreshold * 0.5 + 0.5);
                return Optional.of(buildResult(rule, "protocol_anomaly.tcp_state", "syn_flood",
                        (float) score,
                        String.format("SYN flood: %d SYN packets no data", synCount),
                        "syn_count", String.valueOf(synCount),
                        "data_packets", String.valueOf(dataPackets)));
            }
        }

        long pktsFwd = getLongField(feature, "packets_fwd", 0);
        long pktsBwd = getLongField(feature, "packets_bwd", 0);
        long total = pktsFwd + pktsBwd;
        if (total > 10) {
            double asym = total > 0 ? (double) pktsFwd / total : 0;
            double asymThresh = getDoubleCond(cond, "asymmetry_threshold", 0.95);
            if (asym > asymThresh) {
                double score = Math.min(1.0, (asym - asymThresh) / (1.0 - asymThresh) * 0.5 + 0.5);
                return Optional.of(buildResult(rule, "protocol_anomaly.tcp_state",
                        "asymmetric_flow", (float) score,
                        String.format("Asymmetric flow: fwd=%.1f%% (%d/%d)", asym * 100, pktsFwd, pktsBwd),
                        "asymmetry", String.format("%.2f", asym),
                        "pkts_fwd", String.valueOf(pktsFwd),
                        "pkts_bwd", String.valueOf(pktsBwd)));
            }
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> checkUdpAnomaly(FeatureStat feature, Rule rule,
                                                       Map<String, Object> cond, MatchContext ctx) {
        long maxPkt = getLongField(feature, "max_pkt_size", 0);
        long mtu = getLongCond(cond, "udp_mtu_threshold", 1500);
        if (maxPkt > mtu) {
            double score = Math.min(1.0, (double) (maxPkt - mtu) / 5000 * 0.5 + 0.5);
            return Optional.of(buildResult(rule, "protocol_anomaly.udp", "udp_oversize",
                    (float) score,
                    String.format("UDP oversize: max=%d (MTU=%d)", maxPkt, mtu),
                    "max_pkt_size", String.valueOf(maxPkt), "mtu_threshold", String.valueOf(mtu)));
        }

        int srcPort = getIntField(feature, "src_port", -1);
        int dstPort = getIntField(feature, "dst_port", -1);
        if (srcPort == 0 || dstPort == 0) {
            return Optional.of(buildResult(rule, "protocol_anomaly.udp", "udp_zero_port", 0.7f,
                    String.format("UDP zero-port: src=%d dst=%d", srcPort, dstPort),
                    "src_port", String.valueOf(srcPort), "dst_port", String.valueOf(dstPort)));
        }

        int[] scanPorts = {7, 9, 13, 19, 37, 69, 111, 137, 161, 162, 514, 520, 1434, 1900, 5353};
        for (int port : scanPorts) {
            if (dstPort == port) {
                long pktCount = getLongField(feature, "total_packets", 0);
                if (pktCount <= 3) {
                    return Optional.of(buildResult(rule, "protocol_anomaly.udp", "udp_scan_probe", 0.45f,
                            String.format("UDP scan probe: port=%d pkts=%d", dstPort, pktCount),
                            "dst_port", String.valueOf(dstPort), "packet_count", String.valueOf(pktCount)));
                }
            }
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> checkIcmpAnomaly(FeatureStat feature, Rule rule,
                                                        Map<String, Object> cond, MatchContext ctx) {
        long totalBytes = getLongField(feature, "total_bytes", 0);
        long totalPackets = getLongField(feature, "total_packets", 0);
        if (totalPackets > 0) {
            long avgPayload = totalBytes / totalPackets;
            long tunnelThresh = getLongCond(cond, "icmp_tunnel_threshold", 512);
            if (avgPayload > tunnelThresh) {
                double score = Math.min(1.0, (double) (avgPayload - tunnelThresh) / 1000 * 0.5 + 0.5);
                return Optional.of(buildResult(rule, "protocol_anomaly.icmp", "icmp_tunnel",
                        (float) score,
                        String.format("ICMP tunnel: avg=%d bytes (threshold=%d)", avgPayload, tunnelThresh),
                        "avg_payload", String.valueOf(avgPayload), "threshold", String.valueOf(tunnelThresh),
                        "total_packets", String.valueOf(totalPackets)));
            }
        }
        long pps = getLongField(feature, "pps", 0);
        long floodThresh = getLongCond(cond, "icmp_flood_threshold", 1000);
        if (pps > floodThresh) {
            return Optional.of(buildResult(rule, "protocol_anomaly.icmp", "icmp_flood",
                    (float) Math.min(1.0, pps / floodThresh * 0.5),
                    String.format("ICMP flood: %d pps (threshold=%d)", pps, floodThresh),
                    "pps", String.valueOf(pps), "threshold", String.valueOf(floodThresh)));
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> checkFragmentAnomaly(FeatureStat feature, Rule rule,
                                                            Map<String, Object> cond, MatchContext ctx) {
        boolean isFrag = getBoolField(feature, "is_fragment", false);
        boolean moreFrag = getBoolField(feature, "more_fragments", false);
        long fragOff = getLongField(feature, "fragment_offset", 0);
        long pktSize = getLongField(feature, "total_bytes", 0);

        if (isFrag && pktSize > 0 && pktSize < 68) {
            return Optional.of(buildResult(rule, "protocol_anomaly.fragment", "tiny_fragment", 0.85f,
                    String.format("Tiny fragment: size=%d offset=%d", pktSize, fragOff),
                    "fragment_size", String.valueOf(pktSize), "fragment_offset", String.valueOf(fragOff)));
        }
        long fragCount = getLongField(feature, "fragment_count", 0);
        long maxFrag = getLongCond(cond, "max_fragments", 10);
        if (fragCount > maxFrag && moreFrag) {
            return Optional.of(buildResult(rule, "protocol_anomaly.fragment", "excessive_fragments", 0.5f,
                    String.format("Excessive fragments: %d (max=%d)", fragCount, maxFrag),
                    "fragment_count", String.valueOf(fragCount)));
        }
        return Optional.empty();
    }

    private Optional<DetectionResult> checkFlowAnomaly(FeatureStat feature, Rule rule,
                                                        Map<String, Object> cond, MatchContext ctx) {
        long totalBytes = getLongField(feature, "total_bytes", 0);
        long totalPackets = getLongField(feature, "total_packets", 0);
        long durationMs = getLongField(feature, "duration_ms", 0);

        if (totalPackets > 0 && totalBytes == 0 && durationMs > 100) {
            return Optional.of(buildResult(rule, "protocol_anomaly.flow", "zero_payload", 0.6f,
                    String.format("Zero-payload flow: %d pkts %dms", totalPackets, durationMs),
                    "packets", String.valueOf(totalPackets), "duration_ms", String.valueOf(durationMs)));
        }
        if (totalPackets > 10 && totalBytes > 0 && totalBytes < totalPackets) {
            return Optional.of(buildResult(rule, "protocol_anomaly.flow", "micro_payload", 0.4f,
                    String.format("Micro-payload: %.2f bytes/pkt", (double) totalBytes / totalPackets),
                    "avg_bytes_per_pkt", String.format("%.2f", (double) totalBytes / totalPackets)));
        }
        return Optional.empty();
    }

    // ---- helpers ----

    private DetectionResult buildResult(Rule rule, String detectionType, String label,
                                         float score, String summary, String... evidenceKVs) {
        DetectionResult.Builder b = DetectionResult.builder()
                .ruleId(rule.getRuleId()).ruleName(rule.getName())
                .ruleType(RuleType.ANOMALY)
                .addLabel(detectionType).addLabel(label)
                .score(score)
                .addEvidence("summary", summary);
        for (int i = 0; i < evidenceKVs.length - 1; i += 2) {
            b.addEvidence(evidenceKVs[i], evidenceKVs[i + 1]);
        }
        return b.build();
    }

    private double calcTcpScore(int flags) {
        if (flags == 0x00) return 0.85;
        if (flags == 0x03) return 0.95;
        if (flags == 0x37) return 0.90;
        if ((flags & 0x01) != 0 && (flags & 0x02) == 0 && (flags & 0x10) == 0) return 0.65;
        if ((flags & 0x04) != 0 && (flags & 0x02) == 0) return 0.60;
        return 0.70;
    }

    private int getIntField(FeatureStat f, String name, int def) {
        Map<String, Object> extra = parseExtra(f);
        Object v = extra.get(name);
        return v instanceof Number ? ((Number) v).intValue() : def;
    }

    private long getLongField(FeatureStat f, String name, long def) {
        Map<String, Object> extra = parseExtra(f);
        Object v = extra.get(name);
        return v instanceof Number ? ((Number) v).longValue() : def;
    }

    private boolean getBoolField(FeatureStat f, String name, boolean def) {
        Map<String, Object> extra = parseExtra(f);
        Object v = extra.get(name);
        return v instanceof Boolean ? (Boolean) v : def;
    }

    private long getLongCond(Map<String, Object> cond, String key, long def) {
        Object v = cond.get(key);
        return v instanceof Number ? ((Number) v).longValue() : def;
    }

    private double getDoubleCond(Map<String, Object> cond, String key, double def) {
        Object v = cond.get(key);
        return v instanceof Number ? ((Number) v).doubleValue() : def;
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> parseExtra(FeatureStat feature) {
        Map<String, Object> result = new HashMap<>();
        try {
            // FeatureStat has no labels field; use getAllFields() for flexible field access
            Map<com.google.protobuf.Descriptors.FieldDescriptor, Object> allFields = feature.getAllFields();
            for (Map.Entry<com.google.protobuf.Descriptors.FieldDescriptor, Object> entry : allFields.entrySet()) {
                String fieldName = entry.getKey().getName();
                Object val = entry.getValue();
                if (fieldName != null && val != null) {
                    if (val instanceof Number) {
                        result.put(fieldName, String.valueOf(((Number) val).doubleValue()));
                    } else {
                        result.put(fieldName, val.toString());
                    }
                }
            }
        } catch (Exception e) {
            LOG.trace("parseExtra failed: {}", e.getMessage());
        }
        return result;
    }
}
