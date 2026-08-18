package com.traffic.flink.feature.calculator;

import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureCategory;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;
import java.util.TreeSet;

/**
 * L1 统计特征计算器（增强版 v2）
 * 
 * 修复内容（P0）：
 * 1. ✅ 使用 IAT min/max 字段
 * 2. ✅ 使用 is_established/end_reason/evidence_count
 * 3. ✅ TCP 初始窗口使用 UNKNOWN 值（Integer.MAX_VALUE）
 * 4. ✅ Extra 字段扩展到 20 个槽位
 * 5. ✅ 包长 min/max 作为独立特征
 * 
 * Extra 字段映射表（v2.0）：
 * - extra[0]:  dns_pkt_ratio          (DNS 包比例)
 * - extra[1]:  tcp_pkt_ratio          (TCP 包比例)
 * - extra[2]:  udp_pkt_ratio          (UDP 包比例)
 * - extra[3]:  icmp_pkt_ratio         (ICMP 包比例)
 * - extra[4]:  std_payload            (载荷标准差)
 * - extra[5]:  min_payload            (最小包长)
 * - extra[6]:  max_payload            (最大包长)
 * - extra[7]:  avg_payload            (平均载荷，用于验证)
 * - extra[8]:  min_iat_ms             (最小 IAT)
 * - extra[9]:  max_iat_ms             (最大 IAT)
 * - extra[10]: iat_range_ms           (IAT 范围 = max - min)
 * - extra[11]: is_established         (TCP 是否建立连接)
 * - extra[12]: end_reason_code        (会话结束原因编码)
 * - extra[13]: evidence_count         (证据数量)
 * - extra[14]: flags_fin_cnt          (FIN 标志计数)
 * - extra[15]: flags_psh_cnt          (PSH 标志计数)
 * - extra[16]: flags_rst_cnt          (RST 标志计数)
 * - extra[17]: has_syn                (是否包含 SYN)
 * - extra[18]: has_fin                (是否包含 FIN)
 * - extra[19]: has_rst                (是否包含 RST)
 */
public class FeatureCalculator {

    private static final Logger LOG = LoggerFactory.getLogger(FeatureCalculator.class);

    // 特征版本（升级到 v2.0）
    private static final String SCHEMA_VERSION = "v2.0";

    private static final String ALGORITHM_VERSION = "feature-stat-flow-metadata-v1";

    private FeatureCalculator() {
        // Utility class
    }

    /**
     * 计算 L1 统计特征
     */
    public static FeatureStat calculate(SessionEvent session) {
        // ==================== 提取基础信息 ====================
        long durationMs = Integer.toUnsignedLong(session.getDurationMs());
        boolean rateFeaturesAvailable = durationMs > 0;

        long packetsTotal = session.getPacketsTotal();
        long bytesTotal = session.getBytesTotal();
        long bytesFwd = session.getBytesFwd();
        long bytesBwd = session.getBytesBwd();

        // ==================== 速率特征 ====================
        float durationSec = durationMs / 1000.0f;
        float pps = rateFeaturesAvailable ? packetsTotal / durationSec : 0.0f;
        float bps = rateFeaturesAvailable ? bytesTotal * 8.0f / durationSec : 0.0f;

        // ==================== 方向特征 ====================
        float upDownRatio = calculateUpDownRatio(bytesFwd, bytesBwd);

        // ==================== 包长特征 ====================
        float pktlenMean = session.getAvgPayload();
        float pktlenStd = session.getStdPayload();

        // ==================== IAT 特征（✅ 修复：使用 min/max）====================
        float iatMeanMs = session.getMeanIatMs();
        float iatStdMs = session.getStdIatMs();
        float iatMinMs = session.getMinIatMs();
        float iatMaxMs = session.getMaxIatMs();

        // Missing IAT remains missing. Do not synthesize values from duration.
        boolean iatAvailable = packetsTotal > 1
                && iatMeanMs > 0 && iatMinMs > 0 && iatMaxMs >= iatMinMs;

        // SessionEvent does not provide measured active/idle segments. Do not
        // infer them from mean IAT: zero remains the wire default and the
        // missing contract below carries the absence explicitly.
        float activeMeanMs = 0.0f;
        float idleMeanMs = 0.0f;

        // ==================== TCP Flags 特征 ====================
        int tcpFlagSynCnt = session.getFlagsSyn();
        int tcpFlagAckCnt = session.getFlagsAck();

        // ==================== 协议特征 ====================
        int protocol = session.getProtocol();
        if (protocol == 0 && session.getTuple() != null) {
            protocol = session.getTuple().getProtocol();
        }

        // SessionEvent does not carry TCP initial windows. Numeric values stay
        // at the protobuf default and missing_fields carries the truth.
        int tcpInitWinBytesFwd = 0;
        int tcpInitWinBytesBwd = 0;

        // ==================== 扩展特征（✅ 修复：扩展到 20 个槽位）====================
        List<Float> extra = buildExtraFeaturesV2(session, packetsTotal, iatMinMs, iatMaxMs);

        // ==================== 构造 EventHeader ====================
        long producedAt = System.currentTimeMillis();
        String eventId = generateEventId(session, "feature");
        EventHeader sourceHeader = session.hasHeader()
                ? session.getHeader()
                : EventHeader.getDefaultInstance();
        String traceId = sourceHeader.getTraceId().isEmpty()
                ? sourceHeader.getEventId()
                : sourceHeader.getTraceId();
        String correlationId = sourceHeader.getCorrelationId().isEmpty()
                ? session.getCommunityId()
                : sourceHeader.getCorrelationId();
        List<String> sourceIdentityInputs = new ArrayList<>(session.getSourceEventIdsList());
        sourceIdentityInputs.add(sourceHeader.getEventId());
        List<String> sourceEventIds = stableNonEmptyIds(sourceIdentityInputs);
        List<String> evidenceIds = stableNonEmptyIds(
                session.getEvidenceIdsList().isEmpty()
                        ? session.getFlowIdsList()
                        : session.getEvidenceIdsList());
        List<String> missingFields = new ArrayList<>();
        for (String field : session.getMissingFieldsList()) {
            missingFields.add("source_session." + field);
        }
        if (!rateFeaturesAvailable) missingFields.add("rate_features.duration_ms");
        if (!iatAvailable) missingFields.add("inter_arrival_statistics");
        missingFields.add("active_mean_ms");
        missingFields.add("idle_mean_ms");
        missingFields.add("tcp_initial_window_fwd_bytes");
        missingFields.add("tcp_initial_window_bwd_bytes");
        missingFields = stableNonEmptyIds(missingFields);
        EventHeader header = EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId(session.getHeader().getTenantId())
                .setRunId(session.getHeader().getRunId())
                .setEventTs(session.getTsEnd())
                .setIngestTs(producedAt)
                .setProbeId(session.getHeader().getProbeId())
                .setFeatureSetId(session.getHeader().getFeatureSetId())
                .setEventType("traffic.feature.stat.v1")
                .setSchemaVersion(SCHEMA_VERSION)
                .setAggregateType("session")
                .setAggregateId(session.getSessionId())
                .setAggregateVersion(1)
                .setOccurredAt(session.getTsEnd())
                .setProducedAt(producedAt)
                .setTraceId(traceId)
                .setCausationId(sourceHeader.getEventId())
                .setCorrelationId(correlationId)
                .setIdempotencyKey(eventId)
                .setProducer("flink-feature-job")
                .build();

        // ==================== 构造 FeatureStat ====================
        return FeatureStat.newBuilder()
                .setHeader(header)
                .setSchemaVersion(SCHEMA_VERSION)
                .setObjectType("session")
                .setObjectId(session.getSessionId())
                .setCommunityId(session.getCommunityId())
                .setTs(session.getTsEnd())
                // 基础特征
                .setProtocol(protocol)
                .setDurationMs(durationMs > Integer.MAX_VALUE ? Integer.MAX_VALUE : (int) durationMs)
                .setPps(pps)
                .setBps(bps)
                .setUpDownRatio(upDownRatio)
                // 包长特征
                .setPktlenMean(pktlenMean)
                .setPktlenStd(pktlenStd)
                // IAT 特征
                .setIatMeanMs(iatMeanMs)
                .setIatStdMs(iatStdMs)
                // Active/Idle 特征
                .setActiveMeanMs(activeMeanMs)
                .setIdleMeanMs(idleMeanMs)
                // TCP Flags 特征
                .setTcpFlagSynCnt(tcpFlagSynCnt)
                .setTcpFlagAckCnt(tcpFlagAckCnt)
                // TCP 初始窗口（UNKNOWN 值）
                .setTcpInitWinBytesFwd(tcpInitWinBytesFwd)
                .setTcpInitWinBytesBwd(tcpInitWinBytesBwd)
                // 扩展特征（20 个槽位）
                .addAllExtra(extra)
                .setTuple(session.getTuple())
                .addAllEvidenceIds(evidenceIds)
                .setFeatureCategory(FeatureCategory.FEATURE_CATEGORY_FLOW_METADATA)
                .setAvailability(missingFields.isEmpty()
                        ? FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE
                        : FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE)
                .setAlgorithmVersion(ALGORITHM_VERSION)
                .setWindowId(session.getSessionId())
                .setEventTimeStartMs(session.getEventTimeStartMs() > 0
                        ? session.getEventTimeStartMs() : session.getTsStart())
                .setEventTimeEndMs(session.getEventTimeEndMs() > 0
                        ? session.getEventTimeEndMs() : session.getTsEnd())
                .setValueUnit("mixed:rate_bps,rate_pps,time_ms,bytes,count,ratio")
                .addAllSourceEventIds(sourceEventIds)
                .addAllMissingFields(missingFields)
                .setMissingReason(missingFields.isEmpty() ? "" : "source session did not measure listed fields")
                .build();
    }

    /**
     * 计算上下行比
     */
    private static float calculateUpDownRatio(long bytesFwd, long bytesBwd) {
        if (bytesBwd > 0) {
            return (float) bytesFwd / bytesBwd;
        } else if (bytesFwd > 0) {
            return Float.MAX_VALUE;  // 单向上传
        } else {
            return 0.0f;  // 无数据
        }
    }

    /**
     * 构建扩展特征 v2.0（20 个槽位）
     * 
     * 槽位映射：
     * - [0-4]:   协议分布 + 载荷统计
     * - [5-10]:  包长与 IAT 扩展
     * - [11-13]: TCP 状态与证据
     * - [14-19]: TCP Flags 详细统计
     */
    private static List<Float> buildExtraFeaturesV2(
            SessionEvent session, 
            long packetsTotal,
            float iatMinMs,
            float iatMaxMs
    ) {
        List<Float> extra = new ArrayList<>(20);

        if (packetsTotal > 0) {
            // ==================== [0-4] 协议分布 + 载荷统计 ====================
            
            // extra[0]: DNS 包比例
            float dnsRatio = (float) session.getDnsPktCnt() / packetsTotal;
            extra.add(dnsRatio);

            // extra[1]: TCP 包比例
            float tcpRatio = (float) session.getTcpPktCnt() / packetsTotal;
            extra.add(tcpRatio);

            // extra[2]: UDP 包比例
            float udpRatio = (float) session.getUdpPktCnt() / packetsTotal;
            extra.add(udpRatio);

            // extra[3]: ICMP 包比例
            float icmpRatio = (float) session.getIcmpPktCnt() / packetsTotal;
            extra.add(icmpRatio);

            // extra[4]: 载荷标准差
            float stdPayload = session.getStdPayload();
            extra.add(stdPayload);

            // ==================== [5-7] 包长扩展（✅ 新增）====================
            
            // extra[5]: 最小包长
            extra.add((float) session.getMinPayload());

            // extra[6]: 最大包长
            extra.add((float) session.getMaxPayload());

            // extra[7]: 平均载荷（用于验证 pktlen_mean）
            extra.add(session.getAvgPayload());

            // ==================== [8-10] IAT 扩展（✅ 新增）====================
            
            // extra[8]: 最小 IAT
            extra.add(iatMinMs);

            // extra[9]: 最大 IAT
            extra.add(iatMaxMs);

            // extra[10]: IAT 范围（max - min）
            float iatRangeMs = iatMaxMs - iatMinMs;
            extra.add(iatRangeMs);

            // ==================== [11-13] TCP 状态与证据（✅ 新增）====================
            
            // extra[11]: is_established (0=未建立, 1=已建立)
            extra.add(session.getIsEstablished() ? 1.0f : 0.0f);

            // extra[12]: end_reason_code (0=UNKNOWN, 1=FIN, 2=RST, 3=TIMEOUT, 4=ERROR)
            float endReasonCode = encodeEndReason(session.getEndReason());
            extra.add(endReasonCode);

            // extra[13]: evidence_count
            extra.add((float) session.getEvidenceCount());

            // ==================== [14-19] TCP Flags 详细统计（✅ 新增）====================
            
            // extra[14]: FIN 标志计数
            extra.add((float) session.getFlagsFin());

            // extra[15]: PSH 标志计数
            extra.add((float) session.getFlagsPsh());

            // extra[16]: RST 标志计数
            extra.add((float) session.getFlagsRst());

            // extra[17]: has_syn (0/1)
            extra.add(session.getHasSyn() ? 1.0f : 0.0f);

            // extra[18]: has_fin (0/1)
            extra.add(session.getHasFin() ? 1.0f : 0.0f);

            // extra[19]: has_rst (0/1)
            extra.add(session.getHasRst() ? 1.0f : 0.0f);

        } else {
            // 空流：全部填充 0
            for (int i = 0; i < 20; i++) {
                extra.add(0.0f);
            }
        }

        return extra;
    }

    /**
     * 编码 end_reason 为数值代码
     */
    private static float encodeEndReason(String endReason) {
        if (endReason == null || endReason.isEmpty()) {
            return 0.0f; // UNKNOWN
        }
        
        switch (endReason.toUpperCase()) {
            case "FIN":
                return 1.0f;
            case "RST":
                return 2.0f;
            case "TIMEOUT":
                return 3.0f;
            case "ERROR":
                return 4.0f;
            default:
                return 0.0f; // UNKNOWN
        }
    }

    /**
     * 生成事件 ID
     */
    private static String generateEventId(SessionEvent session, String outputKind) {
        EventHeader inputHeader = session.getHeader();
        return DeterministicId.uuid(
                "flink-feature-event/v1",
                outputKind,
                inputHeader.getTenantId(),
                inputHeader.getEventId(),
                session.getSessionId(),
                session.getTsStart(),
                session.getTsEnd(),
                SCHEMA_VERSION);
    }

    static List<String> stableNonEmptyIds(List<String> values) {
        TreeSet<String> ordered = new TreeSet<>();
        for (String value : values) {
            if (value != null && !value.isEmpty()) {
                ordered.add(value);
            }
        }
        return new ArrayList<>(ordered);
    }

    /**
     * 创建错误特征对象
     */
    public static FeatureStat createErrorFeature(SessionEvent session, String errorMessage) {
        String tenantId = session.getHeader() != null ? session.getHeader().getTenantId() : "unknown";
        String sessionId = session.getSessionId() != null ? session.getSessionId() : "unknown";

        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId(generateEventId(session, "error"))
                        .setTenantId(tenantId)
                        .setEventTs(session.getTsEnd())
                        .setIngestTs(session.getHeader().getIngestTs())
                        .build())
                .setSchemaVersion(SCHEMA_VERSION)
                .setObjectType("error")
                .setObjectId(sessionId)
                .setCommunityId(session.getCommunityId())
                .setFeatureCategory(FeatureCategory.FEATURE_CATEGORY_FLOW_METADATA)
                .setAvailability(FeatureAvailability.FEATURE_AVAILABILITY_INVALID)
                .setAlgorithmVersion(ALGORITHM_VERSION)
                .setWindowId(sessionId)
                .setEventTimeStartMs(session.getTsStart())
                .setEventTimeEndMs(session.getTsEnd())
                .setValueUnit("none")
                .addMissingFields("feature_calculation")
                .setMissingReason(errorMessage == null || errorMessage.isEmpty()
                        ? "feature calculation failed" : errorMessage)
                .build();
    }
}
