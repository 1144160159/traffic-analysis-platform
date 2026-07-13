package com.traffic.flink.session.aggregator;

import com.traffic.proto.traffic.v1.FiveTuple;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.List;

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

    // ==================== 五元组和元数据 ====================
    public transient FiveTuple tuple;
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
