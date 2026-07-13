package com.traffic.flink.session.aggregator;

import com.traffic.flink.common.CommunityIdUtil;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.functions.AggregateFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Base64;

/**
 * Session 聚合函数（修复版）
 * 
 * 修复要点（P0 阻断项）：
 * 1. ✅ 移除 Proto3 标量字段的 hasXxx() 调用（编译阻断）
 * 2. ✅ event_id 改为确定性生成（幂等性）
 * 3. ✅ session_id 改为确定性生成（稳定性）
 * 4. ✅ client/server 判定后同步映射 bytes_up/down（语义一致）
 * 5. ✅ 流量累加改为始终累加 fwd/bwd（不依赖 direction 字符串）
 * 
 * 修复要点（P1 一致性项）：
 * 6. ✅ pktlen/iat 统计改为二阶矩合并（统计质量）
 * 7. ✅ flags_* 输出语义统一为 0/1（presence）
 * 8. ✅ end_reason 大小写统一
 */
public class SessionAggregator implements AggregateFunction<FlowEvent, SessionAccumulator, SessionEvent> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(SessionAggregator.class);

    // 协议常量
    private static final int PROTOCOL_TCP = 6;
    private static final int PROTOCOL_UDP = 17;
    private static final int PROTOCOL_ICMP = 1;

    // DNS 端口
    private static final int DNS_PORT = 53;

    // TCP 标志位常量
    private static final int TCP_FLAG_FIN = 0x01;
    private static final int TCP_FLAG_SYN = 0x02;
    private static final int TCP_FLAG_RST = 0x04;
    private static final int TCP_FLAG_PSH = 0x08;
    private static final int TCP_FLAG_ACK = 0x10;

    @Override
    public SessionAccumulator createAccumulator() {
        return new SessionAccumulator();
    }

    @Override
    public SessionAccumulator add(FlowEvent flow, SessionAccumulator acc) {
        try {
            // 第一个 Flow：初始化基础信息
            if (acc.flowCount == 0) {
                initializeAccumulator(flow, acc);
            }

            // 更新时间范围
            updateTimeRange(flow, acc);
            updateLatencyTimestamps(flow, acc);

            // 累加流量统计（✅ 修复：始终累加 fwd/bwd，不依赖 direction）
            accumulateTraffic(flow, acc);

            // 累加 TCP 标志位
            accumulateTcpFlags(flow, acc);

            // 累加包长统计（✅ 修复：二阶矩合并）
            accumulatePacketLength(flow, acc);

            // 累加 IAT 统计（✅ 修复：二阶矩合并）
            accumulateInterArrivalTime(flow, acc);

            // 累加协议统计
            accumulateProtocolStats(flow, acc);

            // 记录 Flow ID
            if (acc.flowIds.size() < 100) {
                acc.flowIds.add(flow.getFlowId());
            }

            // 更新最后见到的 Flow 时间戳（用于确定性 event_id）
            acc.lastSeenFlowTs = Math.max(acc.lastSeenFlowTs, flow.getTsEnd());

            acc.flowCount++;

            return acc;

        } catch (Exception e) {
            LOG.error("Error adding flow {} to accumulator: {}", 
                    flow.getFlowId(), e.getMessage(), e);
            return acc;
        }
    }

    @Override
    public SessionEvent getResult(SessionAccumulator acc) {
        try {
            // 验证累加器有效性
            if (acc.flowCount == 0 || acc.tuple == null) {
                LOG.warn("Invalid accumulator: flowCount={}, tuple={}", acc.flowCount, acc.tuple);
                return createErrorSession(acc);
            }

            // 计算 Community ID
            String communityId = calculateCommunityId(acc);

            // ✅ 修复：确定 client/server 并同步映射 bytes_up/down
            determineClientServerAndMapDirection(acc);

            // ✅ 修复：生成确定性 Session ID
            String sessionId = generateDeterministicSessionId(acc);

            // ✅ 修复：生成确定性 Event ID
            String eventId = generateDeterministicEventId(acc);

            // 构建 EventHeader
            EventHeader header = EventHeader.newBuilder()
                    .setEventId(eventId)
                    .setTenantId(acc.tenantId != null ? acc.tenantId : "unknown")
                    .setRunId(acc.runId != null ? acc.runId : "unknown")
                    .setEventTs(acc.tsEnd)
                    .setIngestTs(acc.sourceIngestTs > 0 ? acc.sourceIngestTs : System.currentTimeMillis())
                    .setKafkaTs(acc.kafkaTs)
                    .setFlinkOutTs(System.currentTimeMillis())
                    .setProbeId(acc.probeId != null ? acc.probeId : "unknown")
                    .setFeatureSetId(acc.featureSetId != null ? acc.featureSetId : "default")
                    .build();

            // 计算持续时间
            long durationMs = acc.getDurationMs();

            // 计算包长统计（✅ 修复后的统计）
            float avgPayload = acc.getPktlenMean();
            int minPayload = acc.pktlenMin == Integer.MAX_VALUE ? 0 : acc.pktlenMin;
            int maxPayload = acc.pktlenMax;
            float stdPayload = acc.getPktlenStd();

            // 计算 IAT 统计（✅ 修复后的统计）
            float meanIatMs = acc.getIatMeanMs();
            float minIatMs = acc.iatMinMs == Long.MAX_VALUE ? 0 : (float) acc.iatMinMs;
            float maxIatMs = (float) acc.iatMaxMs;
            float stdIatMs = acc.getIatStdMs();

            // ✅ 修复：flags_* 输出为 0/1（presence）
            int flagsSyn = (acc.tcpFlagsFwd & TCP_FLAG_SYN) > 0 ? 1 : 0;
            int flagsAck = acc.tcpFlagsAck > 0 ? 1 : 0;
            int flagsFin = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_FIN) > 0 ? 1 : 0;
            int flagsPsh = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_PSH) > 0 ? 1 : 0;
            int flagsRst = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_RST) > 0 ? 1 : 0;

            boolean hasSyn = flagsSyn > 0;
            boolean hasFin = flagsFin > 0;
            boolean hasRst = flagsRst > 0;
            boolean isEstablished = hasSyn && flagsAck > 0;

            // ✅ 修复：end_reason 大写统一
            String endReason = determineEndReason(acc);

            // 构建 SessionEvent
            SessionEvent.Builder sessionBuilder = SessionEvent.newBuilder()
                    .setHeader(header)
                    .setSessionId(sessionId)
                    .setCommunityId(communityId)
                    .setTuple(acc.tuple)
                    .setTsStart(acc.tsStart)
                    .setTsEnd(acc.tsEnd)
                    .setDurationMs((int) Math.min(durationMs, Integer.MAX_VALUE))
                    .setProtocol(acc.tuple.getProtocol())
                    .setClientIp(acc.determinedClientIp)
                    .setServerIp(acc.determinedServerIp)
                    .setClientPort(acc.determinedClientPort)
                    .setServerPort(acc.determinedServerPort)
                    .setPacketsTotal(acc.getPacketsTotal())
                    .setBytesTotal(acc.getBytesTotal())
                    .setBytesFwd(acc.bytesUp)      // ✅ 修复：fwd 映射为 up（client→server）
                    .setBytesBwd(acc.bytesDown)    // ✅ 修复：bwd 映射为 down（server→client）
                    .setUpDownRatio(acc.getUpDownRatio())
                    .setNumPkts((int) Math.min(acc.pktlenCount, Integer.MAX_VALUE))
                    .setAvgPayload(avgPayload)
                    .setMinPayload(minPayload)
                    .setMaxPayload(maxPayload)
                    .setStdPayload(stdPayload)
                    .setMeanIatMs(meanIatMs)
                    .setMinIatMs(minIatMs)
                    .setMaxIatMs(maxIatMs)
                    .setStdIatMs(stdIatMs)
                    .setFlagsSyn(flagsSyn)
                    .setFlagsAck(flagsAck)
                    .setFlagsFin(flagsFin)
                    .setFlagsPsh(flagsPsh)
                    .setFlagsRst(flagsRst)
                    .setDnsPktCnt(acc.dnsPktCnt)
                    .setTcpPktCnt(acc.tcpPktCnt)
                    .setUdpPktCnt(acc.udpPktCnt)
                    .setIcmpPktCnt(acc.icmpPktCnt)
                    .setHasSyn(hasSyn)
                    .setHasFin(hasFin)
                    .setHasRst(hasRst)
                    .setIsEstablished(isEstablished)
                    .setEvidenceCount(0)
                    .addAllFlowIds(acc.flowIds)
                    .setEndReason(endReason);

            return sessionBuilder.build();

        } catch (Exception e) {
            LOG.error("Error building SessionEvent from accumulator: {}", e.getMessage(), e);
            return createErrorSession(acc);
        }
    }

    // ==================== 下一部分继续 ====================
        // ==================== 继续 SessionAggregator.java ====================

    @Override
    public SessionAccumulator merge(SessionAccumulator a, SessionAccumulator b) {
        // 合并两个累加器（用于窗口合并）
        if (a.flowCount == 0) return b;
        if (b.flowCount == 0) return a;

        SessionAccumulator merged = new SessionAccumulator();

        // 合并元数据（使用第一个累加器的）
        merged.tuple = a.tuple;
        merged.communityId = a.communityId;
        merged.tenantId = a.tenantId;
        merged.runId = a.runId;
        merged.featureSetId = a.featureSetId;
        merged.probeId = a.probeId;

        // 合并时间范围
        merged.tsStart = Math.min(a.tsStart, b.tsStart);
        merged.tsEnd = Math.max(a.tsEnd, b.tsEnd);
        merged.lastSeenFlowTs = Math.max(a.lastSeenFlowTs, b.lastSeenFlowTs);
        merged.sourceIngestTs = Math.max(a.sourceIngestTs, b.sourceIngestTs);
        merged.kafkaTs = Math.max(a.kafkaTs, b.kafkaTs);

        // 合并流量统计（原始 fwd/bwd）
        merged.packetsFwd = a.packetsFwd + b.packetsFwd;
        merged.packetsBwd = a.packetsBwd + b.packetsBwd;
        merged.bytesFwd = a.bytesFwd + b.bytesFwd;
        merged.bytesBwd = a.bytesBwd + b.bytesBwd;

        // 合并定向流量（up/down）
        merged.packetsUp = a.packetsUp + b.packetsUp;
        merged.packetsDown = a.packetsDown + b.packetsDown;
        merged.bytesUp = a.bytesUp + b.bytesUp;
        merged.bytesDown = a.bytesDown + b.bytesDown;

        // 合并 TCP 标志位（位或）
        merged.tcpFlagsFwd = a.tcpFlagsFwd | b.tcpFlagsFwd;
        merged.tcpFlagsBwd = a.tcpFlagsBwd | b.tcpFlagsBwd;
        merged.tcpFlagsAck = a.tcpFlagsAck + b.tcpFlagsAck;

        // ✅ 修复：合并包长统计（二阶矩）
        merged.pktlenSum = a.pktlenSum + b.pktlenSum;
        merged.pktlenSumSquares = a.pktlenSumSquares + b.pktlenSumSquares;
        merged.pktlenMin = Math.min(a.pktlenMin, b.pktlenMin);
        merged.pktlenMax = Math.max(a.pktlenMax, b.pktlenMax);
        merged.pktlenCount = a.pktlenCount + b.pktlenCount;

        // ✅ 修复：合并 IAT 统计（二阶矩）
        merged.iatSumMs = a.iatSumMs + b.iatSumMs;
        merged.iatSumSquaresMs = a.iatSumSquaresMs + b.iatSumSquaresMs;
        merged.iatMinMs = Math.min(a.iatMinMs, b.iatMinMs);
        merged.iatMaxMs = Math.max(a.iatMaxMs, b.iatMaxMs);
        merged.iatCount = a.iatCount + b.iatCount;
        merged.lastPacketTs = Math.max(a.lastPacketTs, b.lastPacketTs);

        // 合并协议统计
        merged.dnsPktCnt = a.dnsPktCnt + b.dnsPktCnt;
        merged.tcpPktCnt = a.tcpPktCnt + b.tcpPktCnt;
        merged.udpPktCnt = a.udpPktCnt + b.udpPktCnt;
        merged.icmpPktCnt = a.icmpPktCnt + b.icmpPktCnt;

        // 合并 Flow IDs
        merged.flowIds.addAll(a.flowIds);
        if (merged.flowIds.size() < 100) {
            int remaining = 100 - merged.flowIds.size();
            merged.flowIds.addAll(b.flowIds.subList(0, Math.min(remaining, b.flowIds.size())));
        }

        merged.flowCount = a.flowCount + b.flowCount;

        // 合并 client/server 判定（使用第一个的结果）
        merged.clientServerDetermined = a.clientServerDetermined;
        merged.determinedClientIp = a.determinedClientIp;
        merged.determinedServerIp = a.determinedServerIp;
        merged.determinedClientPort = a.determinedClientPort;
        merged.determinedServerPort = a.determinedServerPort;

        return merged;
    }

    // ==================== 私有辅助方法 ====================

    /**
     * 初始化累加器
     */
    private void initializeAccumulator(FlowEvent flow, SessionAccumulator acc) {
        acc.tuple = flow.getTuple();
        acc.communityId = flow.getCommunityId();
        acc.tenantId = flow.getHeader().getTenantId();
        acc.runId = flow.getHeader().getRunId();
        acc.featureSetId = flow.getHeader().getFeatureSetId();
        acc.probeId = flow.getHeader().getProbeId();
    }

    /**
     * 更新时间范围
     */
    private void updateTimeRange(FlowEvent flow, SessionAccumulator acc) {
        acc.tsStart = Math.min(acc.tsStart, flow.getTsStart());
        acc.tsEnd = Math.max(acc.tsEnd, flow.getTsEnd());
    }

    /**
     * 累加端到端链路时间戳。
     */
    private void updateLatencyTimestamps(FlowEvent flow, SessionAccumulator acc) {
        if (flow.getHeader() == null) {
            return;
        }
        acc.sourceIngestTs = Math.max(acc.sourceIngestTs, flow.getHeader().getIngestTs());
        acc.kafkaTs = Math.max(acc.kafkaTs, flow.getHeader().getKafkaTs());
    }

    /**
     * 累加流量统计
     * ✅ 修复：始终累加 fwd/bwd，不依赖 direction 字符串
     */
    private void accumulateTraffic(FlowEvent flow, SessionAccumulator acc) {
        // 始终累加双向统计（FlowEvent 已包含 fwd/bwd）
        acc.packetsFwd += flow.getPacketsFwd();
        acc.packetsBwd += flow.getPacketsBwd();
        acc.bytesFwd += flow.getBytesFwd();
        acc.bytesBwd += flow.getBytesBwd();
    }

    /**
     * 累加 TCP 标志位
     */
    private void accumulateTcpFlags(FlowEvent flow, SessionAccumulator acc) {
        if (flow.getTuple().getProtocol() == PROTOCOL_TCP) {
            acc.tcpFlagsFwd |= flow.getTcpFlagsFwd();
            acc.tcpFlagsBwd |= flow.getTcpFlagsBwd();
            
            // 统计 ACK 数量
            if ((flow.getTcpFlagsFwd() & TCP_FLAG_ACK) > 0 || (flow.getTcpFlagsBwd() & TCP_FLAG_ACK) > 0) {
                acc.tcpFlagsAck++;
            }
        }
    }

    /**
     * 累加包长统计
     * ✅ 修复：使用二阶矩合并（基于 Proto3 实际可用字段）
     */
    private void accumulatePacketLength(FlowEvent flow, SessionAccumulator acc) {
        if (flow.hasPktlenStats()) {
            long packetsInFlow = flow.getPacketsFwd() + flow.getPacketsBwd();
            long bytesInFlow = flow.getBytesFwd() + flow.getBytesBwd();
            
            if (packetsInFlow > 0) {
                // ✅ 修复：移除 hasMin/hasMax 调用（Proto3 标量字段无此方法）
                // 直接使用 getMin/getMax，默认值 0 表示"未提供"
                int min = flow.getPktlenStats().getMin();
                int max = flow.getPktlenStats().getMax();
                float mean = flow.getPktlenStats().getMean();
                float std = flow.getPktlenStats().getStd();
                
                // 更新 count
                acc.pktlenCount += packetsInFlow;
                
                // 更新 sum
                acc.pktlenSum += bytesInFlow;
                
                // 更新 sumSquares（使用二阶矩公式：Σx² = n*(σ² + μ²)）
                long sumSquaresFromFlow = (long) (packetsInFlow * (std * std + mean * mean));
                acc.pktlenSumSquares += sumSquaresFromFlow;
                
                // 更新 min/max（仅在非零时更新）
                if (min > 0) {
                    acc.pktlenMin = Math.min(acc.pktlenMin, min);
                }
                if (max > 0) {
                    acc.pktlenMax = Math.max(acc.pktlenMax, max);
                }
            }
        }
    }

    /**
     * 累加到达间隔统计
     * ✅ 修复：使用二阶矩合并
     */
    private void accumulateInterArrivalTime(FlowEvent flow, SessionAccumulator acc) {
        if (flow.hasIatStats()) {
            long packetsInFlow = flow.getPacketsFwd() + flow.getPacketsBwd();
            
            if (packetsInFlow > 1) {
                // ✅ 修复：移除 hasMinMs/hasMaxMs 调用
                float minMs = flow.getIatStats().getMinMs();
                float maxMs = flow.getIatStats().getMaxMs();
                float meanMs = flow.getIatStats().getMeanMs();
                float stdMs = flow.getIatStats().getStdMs();
                
                long iatCountInFlow = packetsInFlow - 1;
                
                // 更新 count
                acc.iatCount += iatCountInFlow;
                
                // 更新 sum
                acc.iatSumMs += (long) (meanMs * iatCountInFlow);
                
                // 更新 sumSquares（使用二阶矩公式）
                long sumSquaresFromFlow = (long) (iatCountInFlow * (stdMs * stdMs + meanMs * meanMs));
                acc.iatSumSquaresMs += sumSquaresFromFlow;
                
                // 更新 min/max（仅在非零时更新）
                if (minMs > 0) {
                    acc.iatMinMs = Math.min(acc.iatMinMs, (long) minMs);
                }
                if (maxMs > 0) {
                    acc.iatMaxMs = Math.max(acc.iatMaxMs, (long) maxMs);
                }
            }
        }
        
        // 更新最后一个包时间戳
        if (flow.getTsEnd() > acc.lastPacketTs) {
            acc.lastPacketTs = flow.getTsEnd();
        }
    }

    /**
     * 累加协议统计
     */
    private void accumulateProtocolStats(FlowEvent flow, SessionAccumulator acc) {
        int protocol = flow.getTuple().getProtocol();
        long packets = flow.getPacketsFwd() + flow.getPacketsBwd();
        
        if (protocol == PROTOCOL_TCP) {
            acc.tcpPktCnt += packets;
        } else if (protocol == PROTOCOL_UDP) {
            acc.udpPktCnt += packets;
            
            // 检查是否是 DNS
            if (flow.getTuple().getDstPort() == DNS_PORT || flow.getTuple().getSrcPort() == DNS_PORT) {
                acc.dnsPktCnt += packets;
            }
        } else if (protocol == PROTOCOL_ICMP) {
            acc.icmpPktCnt += packets;
        }
    }

    // ==================== 下一部分继续 ====================
        // ==================== 继续 SessionAggregator.java ====================

    /**
     * 确定 client/server 并映射方向
     * ✅ 修复：定义"前向=client→server"并在确定后同步映射 bytes_up/down
     */
    private void determineClientServerAndMapDirection(SessionAccumulator acc) {
        if (acc.clientServerDetermined) {
            return; // 已经确定过，直接返回
        }

        String srcIp = acc.tuple.getSrcIp();
        String dstIp = acc.tuple.getDstIp();
        int srcPort = acc.tuple.getSrcPort();
        int dstPort = acc.tuple.getDstPort();

        // 判定逻辑：
        // 1. 如果 dstPort 是知名端口，则 dst 为 server
        // 2. 如果 srcPort 是知名端口，则 src 为 server
        // 3. 否则，端口小的为 server
        boolean isSrcClient;
        if (isWellKnownPort(dstPort)) {
            isSrcClient = true; // dst 是 server，src 是 client
        } else if (isWellKnownPort(srcPort)) {
            isSrcClient = false; // src 是 server，dst 是 client
        } else {
            // 都不是知名端口，端口小的为 server
            isSrcClient = srcPort > dstPort;
        }

        if (isSrcClient) {
            // src = client, dst = server
            acc.determinedClientIp = srcIp;
            acc.determinedServerIp = dstIp;
            acc.determinedClientPort = srcPort;
            acc.determinedServerPort = dstPort;
            // fwd = client→server = up
            acc.bytesUp = acc.bytesFwd;
            acc.bytesDown = acc.bytesBwd;
            acc.packetsUp = acc.packetsFwd;
            acc.packetsDown = acc.packetsBwd;
        } else {
            // dst = client, src = server（需要反转）
            acc.determinedClientIp = dstIp;
            acc.determinedServerIp = srcIp;
            acc.determinedClientPort = dstPort;
            acc.determinedServerPort = srcPort;
            // fwd = server→client = down
            acc.bytesUp = acc.bytesBwd;
            acc.bytesDown = acc.bytesFwd;
            acc.packetsUp = acc.packetsBwd;
            acc.packetsDown = acc.packetsFwd;
        }

        acc.clientServerDetermined = true;
    }

    /**
     * 计算 Community ID
     */
    private String calculateCommunityId(SessionAccumulator acc) {
        if (acc.communityId != null && !acc.communityId.isEmpty()) {
            return acc.communityId;
        }
        
        return CommunityIdUtil.compute(
                acc.tuple.getSrcIp(),
                acc.tuple.getDstIp(),
                acc.tuple.getSrcPort(),
                acc.tuple.getDstPort(),
                acc.tuple.getProtocol()
        );
    }

    /**
     * 生成确定性 Session ID
     * ✅ 修复：使用确定性 hash（全量 community_id + window_start + window_end）
     */
    private String generateDeterministicSessionId(SessionAccumulator acc) {
        String tenantId = acc.tenantId != null ? acc.tenantId : "unknown";
        String communityId = acc.communityId != null ? acc.communityId : "unknown";
        long tsStart = acc.tsStart;
        long tsEnd = acc.tsEnd;

        String input = String.format("%s|%s|%d|%d", tenantId, communityId, tsStart, tsEnd);
        String hash = sha256Hash(input);
        
        return String.format("session-%s-%s", tenantId, hash.substring(0, 16));
    }

    /**
     * 生成确定性 Event ID
     * ✅ 修复：hash(tenant_id, run_id, community_id, window_start, window_end, lastSeenFlowTs)
     */
    private String generateDeterministicEventId(SessionAccumulator acc) {
        String tenantId = acc.tenantId != null ? acc.tenantId : "unknown";
        String runId = acc.runId != null ? acc.runId : "unknown";
        String communityId = acc.communityId != null ? acc.communityId : "unknown";
        long tsStart = acc.tsStart;
        long tsEnd = acc.tsEnd;
        long lastSeenFlowTs = acc.lastSeenFlowTs;

        String input = String.format("%s|%s|%s|%d|%d|%d|session", 
                tenantId, runId, communityId, tsStart, tsEnd, lastSeenFlowTs);
        String hash = sha256Hash(input);
        
        return String.format("evt-%s", hash.substring(0, 24));
    }

    /**
     * SHA256 哈希工具（用于确定性 ID 生成）
     */
    private String sha256Hash(String input) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hashBytes = digest.digest(input.getBytes(StandardCharsets.UTF_8));
            return Base64.getUrlEncoder().withoutPadding().encodeToString(hashBytes);
        } catch (Exception e) {
            LOG.error("Error generating SHA256 hash: {}", e.getMessage());
            return "error-hash";
        }
    }

    /**
     * 判断是否是知名端口
     */
    private boolean isWellKnownPort(int port) {
        return port > 0 && port < 1024;
    }

    /**
     * 确定会话结束原因
     * ✅ 修复：大写统一（FIN/RST/TIMEOUT/ERROR）
     */
    private String determineEndReason(SessionAccumulator acc) {
        if ((acc.tcpFlagsFwd & TCP_FLAG_FIN) > 0 || (acc.tcpFlagsBwd & TCP_FLAG_FIN) > 0) {
            return "FIN";
        } else if ((acc.tcpFlagsFwd & TCP_FLAG_RST) > 0 || (acc.tcpFlagsBwd & TCP_FLAG_RST) > 0) {
            return "RST";
        } else {
            return "TIMEOUT";
        }
    }

    /**
     * 创建错误 Session（降级处理）
     */
    private SessionEvent createErrorSession(SessionAccumulator acc) {
        EventHeader header = EventHeader.newBuilder()
                .setEventId("error-event-" + System.currentTimeMillis())
                .setTenantId(acc.tenantId != null ? acc.tenantId : "unknown")
                .setRunId(acc.runId != null ? acc.runId : "unknown")
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(acc.sourceIngestTs > 0 ? acc.sourceIngestTs : System.currentTimeMillis())
                .setKafkaTs(acc.kafkaTs)
                .setFlinkOutTs(System.currentTimeMillis())
                .setProbeId(acc.probeId != null ? acc.probeId : "unknown")
                .setFeatureSetId(acc.featureSetId != null ? acc.featureSetId : "default")
                .build();

        return SessionEvent.newBuilder()
                .setHeader(header)
                .setSessionId("error-session-" + System.currentTimeMillis())
                .setCommunityId("error")
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("0.0.0.0")
                        .setDstIp("0.0.0.0")
                        .setSrcPort(0)
                        .setDstPort(0)
                        .setProtocol(0)
                        .build())
                .setTsStart(System.currentTimeMillis())
                .setTsEnd(System.currentTimeMillis())
                .setDurationMs(0)
                .setProtocol(0)
                .setClientIp("0.0.0.0")
                .setServerIp("0.0.0.0")
                .setClientPort(0)
                .setServerPort(0)
                .setPacketsTotal(0)
                .setBytesTotal(0)
                .setBytesFwd(0)
                .setBytesBwd(0)
                .setUpDownRatio(0)
                .setEndReason("ERROR")
                .build();
    }
}
