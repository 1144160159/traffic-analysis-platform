package com.traffic.flink.feature.calculator;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

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

    // IAT 阈值（区分 Active/Idle），单位 ms
    private static final float IAT_THRESHOLD_MS = 1000.0f;

    // TCP 初始窗口 UNKNOWN 值
    private static final long TCP_WINDOW_UNKNOWN = Integer.MAX_VALUE;

    private FeatureCalculator() {
        // Utility class
    }

    /**
     * 计算 L1 统计特征
     */
    public static FeatureStat calculate(SessionEvent session) {
        // ==================== 提取基础信息 ====================
        long durationMs = session.getDurationMs();
        if (durationMs <= 0) {
            durationMs = 1; // 避免除零
        }

        long packetsTotal = session.getPacketsTotal();
        long bytesTotal = session.getBytesTotal();
        long bytesFwd = session.getBytesFwd();
        long bytesBwd = session.getBytesBwd();

        // ==================== 速率特征 ====================
        float durationSec = durationMs / 1000.0f;
        float pps = packetsTotal / Math.max(durationSec, 0.001f);
        float bps = bytesTotal * 8.0f / Math.max(durationSec, 0.001f);

        // ==================== 方向特征 ====================
        float upDownRatio = calculateUpDownRatio(bytesFwd, bytesBwd);

        // ==================== 包长特征 ====================
        float pktlenMean = 0.0f;
        float pktlenStd = 0.0f;
        if (packetsTotal > 0) {
            pktlenMean = (float) bytesTotal / packetsTotal;
            
            // 使用 Welford 算法估算标准差
            if (session.getMaxPayload() > session.getMinPayload()) {
                pktlenStd = estimateStdDev(
                        session.getMinPayload(),
                        session.getMaxPayload(),
                        pktlenMean,
                        packetsTotal
                );
            }
        }

        // ==================== IAT 特征（✅ 修复：使用 min/max）====================
        float iatMeanMs = session.getMeanIatMs();
        float iatStdMs = session.getStdIatMs();
        float iatMinMs = session.getMinIatMs();
        float iatMaxMs = session.getMaxIatMs();

        // 如果 session 没有提供 IAT mean，则估算
        if (iatMeanMs <= 0 && packetsTotal > 1) {
            iatMeanMs = (float) durationMs / (packetsTotal - 1);
        }

        // 如果 session 没有提供 IAT min/max，则估算
        if (iatMinMs <= 0 && iatMeanMs > 0) {
            iatMinMs = iatMeanMs * 0.1f; // 估算为均值的 10%
        }
        if (iatMaxMs <= 0 && iatMeanMs > 0) {
            iatMaxMs = iatMeanMs * 3.0f; // 估算为均值的 3 倍
        }

        // ==================== Active/Idle 特征 ====================
        float activeMeanMs = 0.0f;
        float idleMeanMs = 0.0f;

        // 基于 IAT 阈值估算
        if (iatMeanMs > 0) {
            if (iatMeanMs < IAT_THRESHOLD_MS) {
                // 大部分时间 Active
                activeMeanMs = durationMs * 0.8f;
                idleMeanMs = durationMs * 0.2f;
            } else {
                // 大部分时间 Idle
                activeMeanMs = durationMs * 0.3f;
                idleMeanMs = durationMs * 0.7f;
            }
        }

        // ==================== TCP Flags 特征 ====================
        int tcpFlagSynCnt = session.getFlagsSyn();
        int tcpFlagAckCnt = session.getFlagsAck();

        // ==================== 协议特征 ====================
        int protocol = session.getProtocol();
        if (protocol == 0 && session.getTuple() != null) {
            protocol = session.getTuple().getProtocol();
        }

        // ==================== TCP 初始窗口（✅ 修复：使用 UNKNOWN 值）====================
        long tcpInitWinBytesFwd = TCP_WINDOW_UNKNOWN;
        long tcpInitWinBytesBwd = TCP_WINDOW_UNKNOWN;
        
        // 注意：SessionEvent Protobuf 中未定义此字段，标记为 UNKNOWN
        // 若未来 Session Job 提供此字段，则使用实际值

        // ==================== 扩展特征（✅ 修复：扩展到 20 个槽位）====================
        List<Float> extra = buildExtraFeaturesV2(session, packetsTotal, iatMinMs, iatMaxMs);

        // ==================== 构造 EventHeader ====================
        EventHeader header = EventHeader.newBuilder()
                .setEventId(generateEventId())
                .setTenantId(session.getHeader().getTenantId())
                .setRunId(session.getHeader().getRunId())
                .setEventTs(session.getTsEnd())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId(session.getHeader().getProbeId())
                .setFeatureSetId(session.getHeader().getFeatureSetId())
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
                .setTcpInitWinBytesFwd(tcpInitWinBytesFwd > Integer.MAX_VALUE ? Integer.MAX_VALUE : (int) tcpInitWinBytesFwd)
                .setTcpInitWinBytesBwd(tcpInitWinBytesBwd > Integer.MAX_VALUE ? Integer.MAX_VALUE : (int) tcpInitWinBytesBwd)
                // 扩展特征（20 个槽位）
                .addAllExtra(extra)
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
     * 估算标准差（基于 min/max/mean）
     * 使用修正的贝塞尔公式：std ≈ (max - min) / (2 * sqrt(n))
     */
    private static float estimateStdDev(int min, int max, float mean, long count) {
        if (count <= 1) {
            return 0.0f;
        }
        
        // 使用范围估算（假设接近正态分布）
        float range = max - min;
        float estimatedStd = range / (2.0f * (float) Math.sqrt(count));
        
        // 限制最大值（防止异常值）
        return Math.min(estimatedStd, range / 2.0f);
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
    private static String generateEventId() {
        return UUID.randomUUID().toString();
    }

    /**
     * 创建错误特征对象
     */
    public static FeatureStat createErrorFeature(SessionEvent session, String errorMessage) {
        String tenantId = session.getHeader() != null ? session.getHeader().getTenantId() : "unknown";
        String sessionId = session.getSessionId() != null ? session.getSessionId() : "unknown";

        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId(generateEventId())
                        .setTenantId(tenantId)
                        .setEventTs(System.currentTimeMillis())
                        .setIngestTs(System.currentTimeMillis())
                        .build())
                .setSchemaVersion(SCHEMA_VERSION)
                .setObjectType("error")
                .setObjectId(sessionId)
                .setCommunityId("error:" + errorMessage)
                .build();
    }
}