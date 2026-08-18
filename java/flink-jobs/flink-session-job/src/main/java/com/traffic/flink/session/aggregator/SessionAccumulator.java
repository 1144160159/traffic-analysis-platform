package com.traffic.flink.session.aggregator;

import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.TreeSet;
import java.util.Comparator;

/**
 * Session 聚合累加器（修复版）
 * 
 * 修复要点：
 * 1. 统计字段语义明确（sum/sumSquares 代表总体二阶矩）
 * 2. 新增 up/down 方向映射字段
 * 3. 修复默认值语义（pktlenMin/iatMin）
 * 4. 新增 lastSeenFlowTs 用于 event_id 确定性生成
 */
public class SessionAccumulator implements Serializable {

    private static final long serialVersionUID = 1L;

    // ==================== 时间范围 ====================
    public long tsStart = Long.MAX_VALUE;
    public long tsEnd = 0;
    public long lastSeenFlowTs = 0; // ✅ 新增：用于确定性 event_id 生成
    public long sourceIngestTs = 0;
    public long kafkaTs = 0;
    public long flinkOutTs = 0;

    // ==================== 流量统计（原始方向：fwd/bwd） ====================
    public long packetsFwd = 0;
    public long packetsBwd = 0;
    public long bytesFwd = 0;
    public long bytesBwd = 0;

    // ==================== 流量统计（定向后：up/down = client→server / server→client） ====================
    // ✅ 新增：用于最终映射到 bytes_up/bytes_down
    public long packetsUp = 0;
    public long packetsDown = 0;
    public long bytesUp = 0;
    public long bytesDown = 0;

    // ==================== TCP 标志位 ====================
    public int tcpFlagsFwd = 0;
    public int tcpFlagsBwd = 0;
    public int tcpFlagsAck = 0; // ACK 计数

    // ==================== 包长统计（总体二阶矩） ====================
    // ✅ 修复：明确语义为 Σx 和 Σx²
    public long pktlenSum = 0;          // Σ(pktlen)
    public long pktlenSumSquares = 0;   // Σ(pktlen²)
    public int pktlenMin = Integer.MAX_VALUE;
    public int pktlenMax = 0;
    public long pktlenCount = 0;

    // ==================== IAT 统计（总体二阶矩） ====================
    // ✅ 修复：统一使用 long 避免精度丢失
    public long iatSumMs = 0;           // Σ(iat) in milliseconds
    public long iatSumSquaresMs = 0;    // Σ(iat²) in milliseconds²
    public long iatMinMs = Long.MAX_VALUE;
    public long iatMaxMs = 0;
    public long iatCount = 0;
    public long lastPacketTs = 0;

    // ==================== 协议统计 ====================
    public int dnsPktCnt = 0;
    public int tcpPktCnt = 0;
    public int udpPktCnt = 0;
    public int icmpPktCnt = 0;

    // ==================== Flow 关联 ====================
    public List<String> flowIds = new ArrayList<>();
    public List<String> sourceEventIds = new ArrayList<>();
    public boolean flowIdsTruncated = false;
    public boolean sourceEventIdsTruncated = false;

    // ==================== Feature observation carrier ====================
    private static final int MAX_FEATURE_SEQUENCE_POINTS = 256;
    public List<FeatureSequencePoint> featureSequence = new ArrayList<>();
    public long[] payloadNibbleCounts = new long[16];
    public long payloadObservedBytes = 0L;
    public boolean featureSequenceTruncated = false;
    public int transportSecurityValue = 0;
    public String tlsVersion;
    public String ja3;
    public String ja4;
    public String sni;
    public String certSha256;
    public boolean certIsSelfSigned;
    public boolean certIsSelfSignedKnown;
    public int pubkeyLen;
    public boolean pubkeyLenKnown;
    public String quicVersion;
    public String rawTrafficRef;
    public boolean securityObservationConflict = false;
    public TreeSet<String> featureMissingFields = new TreeSet<>();
    public boolean hasFeatureObservation = false;

    public static final class FeatureSequencePoint implements Serializable {
        private static final long serialVersionUID = 1L;
        final long eventTimeUs;
        final int signedLength;

        FeatureSequencePoint(long eventTimeUs, int signedLength) {
            this.eventTimeUs = eventTimeUs;
            this.signedLength = signedLength;
        }
    }

    private static final Comparator<FeatureSequencePoint> FEATURE_SEQUENCE_ORDER =
            Comparator.comparingLong((FeatureSequencePoint point) -> point.eventTimeUs)
                    .thenComparingInt(point -> point.signedLength);

    // Deterministically selected lineage fields for the derived event envelope.
    public String traceId;
    public String correlationId;

    /**
     * Maintain a deterministic bounded evidence set. Arrival order, window
     * merge order and replay partitioning must not select different IDs.
     */
    public void addFlowId(String flowId) {
        flowIdsTruncated |= addBoundedOrderedId(flowIds, flowId);
    }

    public void addFlowIds(Collection<String> values) {
        for (String value : values) {
            addFlowId(value);
        }
    }

    public void addSourceEventId(String eventId) {
        sourceEventIdsTruncated |= addBoundedOrderedId(sourceEventIds, eventId);
    }

    public void addSourceEventIds(Collection<String> values) {
        for (String value : values) {
            addSourceEventId(value);
        }
    }

    private static boolean addBoundedOrderedId(List<String> target, String value) {
        if (value == null || value.isEmpty()) {
            return false;
        }
        TreeSet<String> ordered = new TreeSet<>(target);
        ordered.add(value);
        boolean truncated = ordered.size() > 100;
        while (ordered.size() > 100) {
            ordered.pollLast();
        }
        target.clear();
        target.addAll(ordered);
        return truncated;
    }

    /**
     * Preserve explicit upstream trace/correlation values without making the
     * result depend on arrival or window-merge order.
     */
    public void observeHeader(EventHeader header) {
        if (header == null) {
            return;
        }
        traceId = minNonEmpty(traceId, header.getTraceId());
        correlationId = minNonEmpty(correlationId, header.getCorrelationId());
    }

    public static String minNonEmpty(String left, String right) {
        if (left == null || left.isEmpty()) return right;
        if (right == null || right.isEmpty()) return left;
        return left.compareTo(right) <= 0 ? left : right;
    }

    public void observeFeatureObservation(TrafficFeatureObservation observation) {
        if (observation == null) return;
        hasFeatureObservation = true;
        for (int i = 0; i < observation.getSignedPacketLengthsCount(); i++) {
            featureSequence.add(new FeatureSequencePoint(
                    observation.getPacketEventTimeUs(i),
                    observation.getSignedPacketLengths(i)));
        }
        featureSequence.sort(FEATURE_SEQUENCE_ORDER);
        if (featureSequence.size() > MAX_FEATURE_SEQUENCE_POINTS) {
            featureSequence.subList(MAX_FEATURE_SEQUENCE_POINTS, featureSequence.size()).clear();
            featureSequenceTruncated = true;
        }
        for (int i = 0; i < observation.getPayloadNibbleCountsCount(); i++) {
            payloadNibbleCounts[i] = saturatingAdd(
                    payloadNibbleCounts[i], observation.getPayloadNibbleCounts(i));
        }
        payloadObservedBytes = saturatingAdd(
                payloadObservedBytes, observation.getPayloadObservedBytes());
        featureSequenceTruncated |= observation.getSequenceTruncated();
        securityObservationConflict |= mergeSecurityInt(observation.getTransportSecurityValue());
        securityObservationConflict |= mergeSecurityString("tls_version", observation.getTlsVersion());
        securityObservationConflict |= mergeSecurityString("ja3", observation.getJa3());
        securityObservationConflict |= mergeSecurityString("ja4", observation.getJa4());
        securityObservationConflict |= mergeSecurityString("sni", observation.getSni());
        securityObservationConflict |= mergeSecurityString("cert_sha256", observation.getCertSha256());
        securityObservationConflict |= mergeSecurityString("quic_version", observation.getQuicVersion());
        securityObservationConflict |= mergeSecurityString("raw_traffic_ref", observation.getRawTrafficRef());
        if (observation.getCertIsSelfSignedKnown()) {
            if (certIsSelfSignedKnown && certIsSelfSigned != observation.getCertIsSelfSigned()) {
                securityObservationConflict = true;
                certIsSelfSigned = certIsSelfSigned && observation.getCertIsSelfSigned();
            } else {
                certIsSelfSigned = observation.getCertIsSelfSigned();
            }
            certIsSelfSignedKnown = true;
        }
        if (observation.getPubkeyLenKnown()) {
            if (pubkeyLenKnown && pubkeyLen != observation.getPubkeyLen()) {
                securityObservationConflict = true;
                pubkeyLen = Math.min(pubkeyLen, observation.getPubkeyLen());
            } else {
                pubkeyLen = observation.getPubkeyLen();
            }
            pubkeyLenKnown = true;
        }
        featureMissingFields.addAll(observation.getMissingFieldsList());
        if (featureSequenceTruncated) featureMissingFields.add("sequence_truncated");
        if (securityObservationConflict) featureMissingFields.add("security_observation_conflict");
    }

    public void mergeFeatureObservation(SessionAccumulator other) {
        if (other == null || !other.hasFeatureObservation) return;
        TrafficFeatureObservation.Builder builder = other.buildFeatureObservation().toBuilder();
        observeFeatureObservation(builder.build());
    }

    public TrafficFeatureObservation buildFeatureObservation() {
        TrafficFeatureObservation.Builder builder = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("session-feature-merge/v1")
                .setPayloadObservedBytes(payloadObservedBytes)
                .setSequenceTruncated(featureSequenceTruncated)
                .setTransportSecurityValue(transportSecurityValue)
                .setTlsVersion(emptyIfNull(tlsVersion))
                .setJa3(emptyIfNull(ja3))
                .setJa4(emptyIfNull(ja4))
                .setSni(emptyIfNull(sni))
                .setCertSha256(emptyIfNull(certSha256))
                .setCertIsSelfSigned(certIsSelfSigned)
                .setCertIsSelfSignedKnown(certIsSelfSignedKnown)
                .setPubkeyLen(pubkeyLen)
                .setPubkeyLenKnown(pubkeyLenKnown)
                .setQuicVersion(emptyIfNull(quicVersion))
                .setRawTrafficRef(emptyIfNull(rawTrafficRef))
                .addAllMissingFields(featureMissingFields);
        featureSequence.sort(FEATURE_SEQUENCE_ORDER);
        for (FeatureSequencePoint point : featureSequence) {
            builder.addSignedPacketLengths(point.signedLength);
            builder.addPacketEventTimeUs(point.eventTimeUs);
        }
        for (long count : payloadNibbleCounts) builder.addPayloadNibbleCounts(count);
        return builder.build();
    }

    private boolean mergeSecurityInt(int incoming) {
        if (incoming == 0) return false;
        if (transportSecurityValue == 0) {
            transportSecurityValue = incoming;
            return false;
        }
        if (transportSecurityValue != incoming) {
            transportSecurityValue = Math.min(transportSecurityValue, incoming);
            return true;
        }
        return false;
    }

    private boolean mergeSecurityString(String field, String incoming) {
        if (incoming == null || incoming.isEmpty()) return false;
        String current;
        switch (field) {
            case "tls_version": current = tlsVersion; break;
            case "ja3": current = ja3; break;
            case "ja4": current = ja4; break;
            case "sni": current = sni; break;
            case "cert_sha256": current = certSha256; break;
            case "quic_version": current = quicVersion; break;
            case "raw_traffic_ref": current = rawTrafficRef; break;
            default: throw new IllegalArgumentException("unknown security field " + field);
        }
        String selected = minNonEmpty(current, incoming);
        switch (field) {
            case "tls_version": tlsVersion = selected; break;
            case "ja3": ja3 = selected; break;
            case "ja4": ja4 = selected; break;
            case "sni": sni = selected; break;
            case "cert_sha256": certSha256 = selected; break;
            case "quic_version": quicVersion = selected; break;
            case "raw_traffic_ref": rawTrafficRef = selected; break;
            default: break;
        }
        return current != null && !current.isEmpty() && !current.equals(incoming);
    }

    private static long saturatingAdd(long left, long right) {
        try {
            return Math.addExact(left, right);
        } catch (ArithmeticException e) {
            return Long.MAX_VALUE;
        }
    }

    private static String emptyIfNull(String value) {
        return value == null ? "" : value;
    }

    // ==================== 五元组和元数据 ====================
    // 五元组必须参与窗口状态序列化（window 模式累加器入 checkpoint）。
    // 曾为 transient：恢复后 tuple==null 导致会话退化为 error session。
    // FiveTuple 为 protobuf 消息，需在作业侧注册 Kryo JavaSerializer
    // （见 SessionJob.main 中的 registerTypeWithKryoSerializer）。
    public FiveTuple tuple;
    public String srcIp;
    public String dstIp;
    public int srcPort;
    public int dstPort;
    public int protocol;
    public String communityId;
    public String tenantId;
    public String runId;
    public String featureSetId;
    public String probeId;

    // ==================== 流数量 ====================
    public int flowCount = 0;

    // ==================== Client/Server 识别标记 ====================
    // ✅ 新增：记录是否已确定方向
    public boolean clientServerDetermined = false;
    public String determinedClientIp;
    public String determinedServerIp;
    public int determinedClientPort;
    public int determinedServerPort;

    /**
     * 重置累加器
     */
    public void reset() {
        tsStart = Long.MAX_VALUE;
        tsEnd = 0;
        lastSeenFlowTs = 0;
        sourceIngestTs = 0;
        kafkaTs = 0;
        flinkOutTs = 0;
        packetsFwd = 0;
        packetsBwd = 0;
        bytesFwd = 0;
        bytesBwd = 0;
        packetsUp = 0;
        packetsDown = 0;
        bytesUp = 0;
        bytesDown = 0;
        tcpFlagsFwd = 0;
        tcpFlagsBwd = 0;
        tcpFlagsAck = 0;
        pktlenSum = 0;
        pktlenSumSquares = 0;
        pktlenMin = Integer.MAX_VALUE;
        pktlenMax = 0;
        pktlenCount = 0;
        iatSumMs = 0;
        iatSumSquaresMs = 0;
        iatMinMs = Long.MAX_VALUE;
        iatMaxMs = 0;
        iatCount = 0;
        lastPacketTs = 0;
        dnsPktCnt = 0;
        tcpPktCnt = 0;
        udpPktCnt = 0;
        icmpPktCnt = 0;
        flowIds.clear();
        sourceEventIds.clear();
        flowIdsTruncated = false;
        sourceEventIdsTruncated = false;
        featureSequence.clear();
        payloadNibbleCounts = new long[16];
        payloadObservedBytes = 0L;
        featureSequenceTruncated = false;
        transportSecurityValue = 0;
        tlsVersion = null;
        ja3 = null;
        ja4 = null;
        sni = null;
        certSha256 = null;
        certIsSelfSigned = false;
        certIsSelfSignedKnown = false;
        pubkeyLen = 0;
        pubkeyLenKnown = false;
        quicVersion = null;
        rawTrafficRef = null;
        securityObservationConflict = false;
        featureMissingFields.clear();
        hasFeatureObservation = false;
        traceId = null;
        correlationId = null;
        tuple = null;
        srcIp = null;
        dstIp = null;
        srcPort = 0;
        dstPort = 0;
        protocol = 0;
        communityId = null;
        tenantId = null;
        runId = null;
        featureSetId = null;
        probeId = null;
        flowCount = 0;
        clientServerDetermined = false;
        determinedClientIp = null;
        determinedServerIp = null;
        determinedClientPort = 0;
        determinedServerPort = 0;
    }

    /**
     * 计算包长均值（总体均值）
     */
    public float getPktlenMean() {
        return pktlenCount > 0 ? (float) pktlenSum / pktlenCount : 0;
    }

    /**
     * 计算包长标准差（总体标准差）
     * ✅ 修复：使用总体方差公式 σ² = E[X²] - E[X]²
     */
    public float getPktlenStd() {
        if (pktlenCount == 0) return 0;
        double mean = (double) pktlenSum / pktlenCount;
        double meanSquare = (double) pktlenSumSquares / pktlenCount;
        double variance = meanSquare - mean * mean;
        return (float) Math.sqrt(Math.max(0, variance));
    }

    /**
     * 计算 IAT 均值（毫秒）
     */
    public float getIatMeanMs() {
        return iatCount > 0 ? (float) iatSumMs / iatCount : 0;
    }

    /**
     * 计算 IAT 标准差（毫秒）
     * ✅ 修复：使用总体方差公式
     */
    public float getIatStdMs() {
        if (iatCount == 0) return 0;
        double mean = (double) iatSumMs / iatCount;
        double meanSquare = (double) iatSumSquaresMs / iatCount;
        double variance = meanSquare - mean * mean;
        return (float) Math.sqrt(Math.max(0, variance));
    }

    /**
     * 获取上下行比例（基于定向后的 up/down）
     * ✅ 修复：使用映射后的 bytesUp/bytesDown
     */
    public float getUpDownRatio() {
        if (bytesDown == 0) return bytesUp > 0 ? Float.MAX_VALUE : 1.0f;
        return (float) bytesUp / bytesDown;
    }

    /**
     * 获取总包数
     */
    public long getPacketsTotal() {
        return packetsFwd + packetsBwd;
    }

    /**
     * 获取总字节数
     */
    public long getBytesTotal() {
        return bytesFwd + bytesBwd;
    }

    /**
     * 获取持续时间（毫秒）
     */
    public long getDurationMs() {
        return tsEnd > tsStart ? tsEnd - tsStart : 0;
    }

    @Override
    public String toString() {
        return "SessionAccumulator{" +
                "communityId='" + communityId + '\'' +
                ", duration=" + getDurationMs() + "ms" +
                ", packets=" + getPacketsTotal() +
                ", bytes=" + getBytesTotal() +
                ", flows=" + flowCount +
                ", clientServerDetermined=" + clientServerDetermined +
                '}';
    }
}
