package com.traffic.flink.session.processor;

import com.traffic.flink.common.CommunityIdUtil;
import com.traffic.flink.session.SessionJobConfig;
import com.traffic.flink.session.aggregator.SessionAccumulator;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Base64;

/**
 * SessionizeProcessFunction - 基于 KeyedProcessFunction 的会话聚合
 * 
 * 核心功能：
 * 1. Active Timeout：会话持续时间超过阈值（如 30 分钟）强制切分
 * 2. Idle Timeout：会话空闲时间超过阈值（如 2 分钟）自动结束
 * 3. 自定义 Prometheus Metrics：session_emitted_total, late_flow_total 等
 * 4. Late Data Side Output：延迟数据输出到侧流
 * 
 * 状态管理：
 * - ValueState<SessionAccumulator>：会话累加器
 * - ValueState<Long>：Idle Timer 时间戳
 * - ValueState<Long>：Active Timer 时间戳
 * 
 * Timer 策略：
 * - Idle Timer：每次收到新 Flow 时重置为 tsEnd + idleTimeoutMs
 * - Active Timer：会话开始时设置为 tsStart + activeTimeoutMs，不再更新
 */
public class SessionizeProcessFunction 
        extends KeyedProcessFunction<String, FlowEvent, SessionEvent> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(SessionizeProcessFunction.class);

    // ==================== 配置 ====================
    private final long idleTimeoutMs;
    private final long activeTimeoutMs;
    private final long stateTtlMs;
    private final OutputTag<FlowEvent> lateDataOutputTag;

    // ==================== 状态 ====================
    private transient ValueState<SessionAccumulator> accumulatorState;
    private transient ValueState<Long> idleTimerState;
    private transient ValueState<Long> activeTimerState;

    // ==================== Metrics ====================
    private transient Counter sessionEmittedCounter;
    private transient Counter sessionBytesCounter;
    private transient Counter lateFlowCounter;
    private transient Counter idleTimeoutCounter;
    private transient Counter activeTimeoutCounter;
    private transient Counter flowProcessedCounter;

    // 协议常量
    private static final int PROTOCOL_TCP = 6;
    private static final int PROTOCOL_UDP = 17;
    private static final int PROTOCOL_ICMP = 1;
    private static final int DNS_PORT = 53;

    // TCP 标志位常量
    private static final int TCP_FLAG_FIN = 0x01;
    private static final int TCP_FLAG_SYN = 0x02;
    private static final int TCP_FLAG_RST = 0x04;
    private static final int TCP_FLAG_PSH = 0x08;
    private static final int TCP_FLAG_ACK = 0x10;

    /**
     * 构造函数
     */
    public SessionizeProcessFunction(SessionJobConfig config, OutputTag<FlowEvent> lateDataOutputTag) {
        this.idleTimeoutMs = config.getSessionGapMs();
        this.activeTimeoutMs = config.getActiveTimeoutMs();
        this.stateTtlMs = config.getStateTtlMs();
        this.lateDataOutputTag = lateDataOutputTag;
    }

    /**
     * 简化构造函数（使用默认 OutputTag）
     */
    public SessionizeProcessFunction(long idleTimeoutMs, long activeTimeoutMs, long stateTtlMs,
                                      OutputTag<FlowEvent> lateDataOutputTag) {
        this.idleTimeoutMs = idleTimeoutMs;
        this.activeTimeoutMs = activeTimeoutMs;
        this.stateTtlMs = stateTtlMs;
        this.lateDataOutputTag = lateDataOutputTag;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // ==================== 初始化状态 ====================

        // 配置 State TTL
        StateTtlConfig ttlConfig = StateTtlConfig.newBuilder(Time.milliseconds(stateTtlMs))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupInRocksdbCompactFilter(1000)
                .build();

        // SessionAccumulator 状态
        ValueStateDescriptor<SessionAccumulator> accDescriptor = new ValueStateDescriptor<>(
                "session-accumulator",
                TypeInformation.of(SessionAccumulator.class)
        );
        accDescriptor.enableTimeToLive(ttlConfig);
        this.accumulatorState = getRuntimeContext().getState(accDescriptor);

        // Idle Timer 状态
        ValueStateDescriptor<Long> idleTimerDescriptor = new ValueStateDescriptor<>(
                "idle-timer-ts",
                Long.class
        );
        idleTimerDescriptor.enableTimeToLive(ttlConfig);
        this.idleTimerState = getRuntimeContext().getState(idleTimerDescriptor);

        // Active Timer 状态
        ValueStateDescriptor<Long> activeTimerDescriptor = new ValueStateDescriptor<>(
                "active-timer-ts",
                Long.class
        );
        activeTimerDescriptor.enableTimeToLive(ttlConfig);
        this.activeTimerState = getRuntimeContext().getState(activeTimerDescriptor);

        // ==================== 初始化 Metrics ====================
        MetricGroup metricGroup = getRuntimeContext().getMetricGroup()
                .addGroup("session_job");

        this.sessionEmittedCounter = metricGroup.counter("session_emitted_total");
        this.sessionBytesCounter = metricGroup.counter("session_bytes_total");
        this.lateFlowCounter = metricGroup.counter("late_flow_total");
        this.idleTimeoutCounter = metricGroup.counter("idle_timeout_total");
        this.activeTimeoutCounter = metricGroup.counter("active_timeout_total");
        this.flowProcessedCounter = metricGroup.counter("flow_processed_total");

        LOG.info("SessionizeProcessFunction initialized: idleTimeout={}ms, activeTimeout={}ms, stateTTL={}ms",
                idleTimeoutMs, activeTimeoutMs, stateTtlMs);
    }

    @Override
    public void processElement(FlowEvent flow, Context ctx, Collector<SessionEvent> out) throws Exception {
        // 检查是否为 Late Data
        long currentWatermark = ctx.timerService().currentWatermark();
        if (flow.getTsEnd() <= currentWatermark && currentWatermark != Long.MIN_VALUE) {
            // Late Data：输出到侧流
            if (lateDataOutputTag != null) {
                ctx.output(lateDataOutputTag, flow);
            }
            lateFlowCounter.inc();
            LOG.debug("Late flow detected: flow_id={}, tsEnd={}, watermark={}",
                    flow.getFlowId(), flow.getTsEnd(), currentWatermark);
            return;
        }

        flowProcessedCounter.inc();

        // 获取当前累加器状态
        SessionAccumulator acc = accumulatorState.value();
        boolean isNewSession = (acc == null);

        if (isNewSession) {
            acc = new SessionAccumulator();
        }

        // 累加 Flow 到 Session
        accumulateFlow(flow, acc);

        // 更新状态
        accumulatorState.update(acc);

        // ==================== Timer 管理 ====================

        long flowTsEnd = flow.getTsEnd();

        if (isNewSession) {
            // 新会话：注册 Active Timer（基于会话开始时间）
            long activeTimerTs = acc.tsStart + activeTimeoutMs;
            ctx.timerService().registerEventTimeTimer(activeTimerTs);
            activeTimerState.update(activeTimerTs);

            // 注册 Idle Timer
            long idleTimerTs = flowTsEnd + idleTimeoutMs;
            ctx.timerService().registerEventTimeTimer(idleTimerTs);
            idleTimerState.update(idleTimerTs);

            LOG.debug("New session started: key={}, activeTimer={}, idleTimer={}",
                    ctx.getCurrentKey(), activeTimerTs, idleTimerTs);
        } else {
            // 已有会话：更新 Idle Timer
            Long oldIdleTimer = idleTimerState.value();
            long newIdleTimer = flowTsEnd + idleTimeoutMs;

            if (oldIdleTimer != null && newIdleTimer > oldIdleTimer) {
                // 删除旧的 Idle Timer，注册新的
                ctx.timerService().deleteEventTimeTimer(oldIdleTimer);
                ctx.timerService().registerEventTimeTimer(newIdleTimer);
                idleTimerState.update(newIdleTimer);

                LOG.debug("Idle timer updated: key={}, oldTimer={}, newTimer={}",
                        ctx.getCurrentKey(), oldIdleTimer, newIdleTimer);
            }
            // Active Timer 保持不变
        }
    }

    @Override
    public void onTimer(long timestamp, OnTimerContext ctx, Collector<SessionEvent> out) throws Exception {
        SessionAccumulator acc = accumulatorState.value();

        if (acc == null || acc.flowCount == 0) {
            // 状态已被清空或无效，直接返回
            LOG.debug("Timer fired but no valid state: key={}, timestamp={}", ctx.getCurrentKey(), timestamp);
            clearAllState(ctx);
            return;
        }

        Long activeTimerTs = activeTimerState.value();
        Long idleTimerTs = idleTimerState.value();

        boolean isActiveTimeout = (activeTimerTs != null && timestamp == activeTimerTs);
        boolean isIdleTimeout = (idleTimerTs != null && timestamp == idleTimerTs);

        if (isActiveTimeout) {
            // Active Timeout：强制输出会话
            LOG.debug("Active timeout triggered: key={}, timestamp={}", ctx.getCurrentKey(), timestamp);
            emitSession(acc, "ACTIVE_TIMEOUT", out);
            activeTimeoutCounter.inc();
            clearAllState(ctx);
        } else if (isIdleTimeout) {
            // Idle Timeout：输出会话
            LOG.debug("Idle timeout triggered: key={}, timestamp={}", ctx.getCurrentKey(), timestamp);
            emitSession(acc, "IDLE_TIMEOUT", out);
            idleTimeoutCounter.inc();
            clearAllState(ctx);
        } else {
            // 未知 Timer（可能是已被取消但仍触发的旧 Timer）
            LOG.debug("Unknown timer fired: key={}, timestamp={}, activeTs={}, idleTs={}",
                    ctx.getCurrentKey(), timestamp, activeTimerTs, idleTimerTs);
        }
    }

    /**
     * 累加 Flow 到 Session
     */
    private void accumulateFlow(FlowEvent flow, SessionAccumulator acc) {
        // 第一个 Flow：初始化基础信息
        if (acc.flowCount == 0) {
            if (flow.getTuple() != null) {
                acc.srcIp = flow.getTuple().getSrcIp();
                acc.dstIp = flow.getTuple().getDstIp();
                acc.srcPort = flow.getTuple().getSrcPort();
                acc.dstPort = flow.getTuple().getDstPort();
                acc.protocol = flow.getTuple().getProtocol();
            }
            acc.communityId = flow.getCommunityId();
            if (flow.getHeader() != null) {
                acc.tenantId = flow.getHeader().getTenantId();
                acc.runId = flow.getHeader().getRunId();
                acc.featureSetId = flow.getHeader().getFeatureSetId();
                acc.probeId = flow.getHeader().getProbeId();
            }
        }

        // 更新时间范围
        acc.tsStart = Math.min(acc.tsStart, flow.getTsStart());
        acc.tsEnd = Math.max(acc.tsEnd, flow.getTsEnd());
        acc.lastSeenFlowTs = Math.max(acc.lastSeenFlowTs, flow.getTsEnd());
        if (flow.getHeader() != null) {
            acc.sourceIngestTs = Math.max(acc.sourceIngestTs, flow.getHeader().getIngestTs());
            acc.kafkaTs = Math.max(acc.kafkaTs, flow.getHeader().getKafkaTs());
        }

        // 累加流量统计
        acc.packetsFwd += flow.getPacketsFwd();
        acc.packetsBwd += flow.getPacketsBwd();
        acc.bytesFwd += flow.getBytesFwd();
        acc.bytesBwd += flow.getBytesBwd();

        // 累加 TCP 标志位
        if (flow.getTuple().getProtocol() == PROTOCOL_TCP) {
            acc.tcpFlagsFwd |= flow.getTcpFlagsFwd();
            acc.tcpFlagsBwd |= flow.getTcpFlagsBwd();
            if ((flow.getTcpFlagsFwd() & TCP_FLAG_ACK) > 0 || (flow.getTcpFlagsBwd() & TCP_FLAG_ACK) > 0) {
                acc.tcpFlagsAck++;
            }
        }

        // 累加包长统计
        if (flow.hasPktlenStats()) {
            long packetsInFlow = flow.getPacketsFwd() + flow.getPacketsBwd();
            long bytesInFlow = flow.getBytesFwd() + flow.getBytesBwd();

            if (packetsInFlow > 0) {
                int min = flow.getPktlenStats().getMin();
                int max = flow.getPktlenStats().getMax();
                float mean = flow.getPktlenStats().getMean();
                float std = flow.getPktlenStats().getStd();

                acc.pktlenCount += packetsInFlow;
                acc.pktlenSum += bytesInFlow;
                long sumSquaresFromFlow = (long) (packetsInFlow * (std * std + mean * mean));
                acc.pktlenSumSquares += sumSquaresFromFlow;

                if (min > 0) {
                    acc.pktlenMin = Math.min(acc.pktlenMin, min);
                }
                if (max > 0) {
                    acc.pktlenMax = Math.max(acc.pktlenMax, max);
                }
            }
        }

        // 累加 IAT 统计
        if (flow.hasIatStats()) {
            long packetsInFlow = flow.getPacketsFwd() + flow.getPacketsBwd();

            if (packetsInFlow > 1) {
                float minMs = flow.getIatStats().getMinMs();
                float maxMs = flow.getIatStats().getMaxMs();
                float meanMs = flow.getIatStats().getMeanMs();
                float stdMs = flow.getIatStats().getStdMs();

                long iatCountInFlow = packetsInFlow - 1;
                acc.iatCount += iatCountInFlow;
                acc.iatSumMs += (long) (meanMs * iatCountInFlow);
                long sumSquaresFromFlow = (long) (iatCountInFlow * (stdMs * stdMs + meanMs * meanMs));
                acc.iatSumSquaresMs += sumSquaresFromFlow;

                if (minMs > 0) {
                    acc.iatMinMs = Math.min(acc.iatMinMs, (long) minMs);
                }
                if (maxMs > 0) {
                    acc.iatMaxMs = Math.max(acc.iatMaxMs, (long) maxMs);
                }
            }
        }

        // 累加协议统计
        int protocol = flow.getTuple().getProtocol();
        long packets = flow.getPacketsFwd() + flow.getPacketsBwd();

        if (protocol == PROTOCOL_TCP) {
            acc.tcpPktCnt += packets;
        } else if (protocol == PROTOCOL_UDP) {
            acc.udpPktCnt += packets;
            if (flow.getTuple().getDstPort() == DNS_PORT || flow.getTuple().getSrcPort() == DNS_PORT) {
                acc.dnsPktCnt += packets;
            }
        } else if (protocol == PROTOCOL_ICMP) {
            acc.icmpPktCnt += packets;
        }

        // 记录 Flow ID
        if (acc.flowIds.size() < 100) {
            acc.flowIds.add(flow.getFlowId());
        }

        acc.flowCount++;
    }

    /**
     * 输出 Session
     */
    private void emitSession(SessionAccumulator acc, String endReason, Collector<SessionEvent> out) {
        try {
            // 确定 client/server 并映射方向
            determineClientServerAndMapDirection(acc);

            // 计算 Community ID
            String communityId = calculateCommunityId(acc);

            // 生成确定性 ID
            String sessionId = generateDeterministicSessionId(acc);
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

            // 计算统计值
            long durationMs = acc.getDurationMs();
            float avgPayload = acc.getPktlenMean();
            int minPayload = acc.pktlenMin == Integer.MAX_VALUE ? 0 : acc.pktlenMin;
            int maxPayload = acc.pktlenMax;
            float stdPayload = acc.getPktlenStd();
            float meanIatMs = acc.getIatMeanMs();
            float minIatMs = acc.iatMinMs == Long.MAX_VALUE ? 0 : (float) acc.iatMinMs;
            float maxIatMs = (float) acc.iatMaxMs;
            float stdIatMs = acc.getIatStdMs();

            // TCP 标志位（0/1 presence）
            int flagsSyn = (acc.tcpFlagsFwd & TCP_FLAG_SYN) > 0 ? 1 : 0;
            int flagsAck = acc.tcpFlagsAck > 0 ? 1 : 0;
            int flagsFin = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_FIN) > 0 ? 1 : 0;
            int flagsPsh = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_PSH) > 0 ? 1 : 0;
            int flagsRst = ((acc.tcpFlagsFwd | acc.tcpFlagsBwd) & TCP_FLAG_RST) > 0 ? 1 : 0;

            boolean hasSyn = flagsSyn > 0;
            boolean hasFin = flagsFin > 0;
            boolean hasRst = flagsRst > 0;
            boolean isEstablished = hasSyn && flagsAck > 0;

            // 确定结束原因
            String finalEndReason = determineFinalEndReason(acc, endReason);

            // 构建 SessionEvent
            SessionEvent session = SessionEvent.newBuilder()
                    .setHeader(header)
                    .setSessionId(sessionId)
                    .setCommunityId(communityId)
                    .setTuple(FiveTuple.newBuilder()
                        .setSrcIp(acc.srcIp != null ? acc.srcIp : "")
                        .setDstIp(acc.dstIp != null ? acc.dstIp : "")
                        .setSrcPort(acc.srcPort)
                        .setDstPort(acc.dstPort)
                        .setProtocol(acc.protocol)
                        .build())
                    .setTsStart(acc.tsStart)
                    .setTsEnd(acc.tsEnd)
                    .setDurationMs((int) Math.min(durationMs, Integer.MAX_VALUE))
                    .setProtocol(acc.protocol)
                    .setClientIp(acc.determinedClientIp)
                    .setServerIp(acc.determinedServerIp)
                    .setClientPort(acc.determinedClientPort)
                    .setServerPort(acc.determinedServerPort)
                    .setPacketsTotal(acc.getPacketsTotal())
                    .setBytesTotal(acc.getBytesTotal())
                    .setBytesFwd(acc.bytesUp)
                    .setBytesBwd(acc.bytesDown)
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
                    .setEndReason(finalEndReason)
                    .build();

            out.collect(session);

            // 更新 Metrics
            sessionEmittedCounter.inc();
            sessionBytesCounter.inc(acc.getBytesTotal());

            LOG.debug("Session emitted: session_id={}, duration={}ms, packets={}, bytes={}, endReason={}",
                    sessionId, durationMs, acc.getPacketsTotal(), acc.getBytesTotal(), finalEndReason);

        } catch (Exception e) {
            LOG.error("Error emitting session: {}", e.getMessage(), e);
        }
    }

    /**
     * 清空所有状态
     */
    private void clearAllState(OnTimerContext ctx) throws Exception {
        Long activeTs = activeTimerState.value();
        Long idleTs = idleTimerState.value();

        if (activeTs != null) {
            ctx.timerService().deleteEventTimeTimer(activeTs);
        }
        if (idleTs != null) {
            ctx.timerService().deleteEventTimeTimer(idleTs);
        }

        accumulatorState.clear();
        idleTimerState.clear();
        activeTimerState.clear();
    }

    /**
     * 确定 client/server 并映射方向
     */
    private void determineClientServerAndMapDirection(SessionAccumulator acc) {
        if (acc.clientServerDetermined) {
            return;
        }

        String srcIp = acc.srcIp;
        String dstIp = acc.dstIp;
        int srcPort = acc.srcPort;
        int dstPort = acc.dstPort;

        boolean isSrcClient;
        if (isWellKnownPort(dstPort)) {
            isSrcClient = true;
        } else if (isWellKnownPort(srcPort)) {
            isSrcClient = false;
        } else {
            isSrcClient = srcPort > dstPort;
        }

        if (isSrcClient) {
            acc.determinedClientIp = srcIp;
            acc.determinedServerIp = dstIp;
            acc.determinedClientPort = srcPort;
            acc.determinedServerPort = dstPort;
            acc.bytesUp = acc.bytesFwd;
            acc.bytesDown = acc.bytesBwd;
            acc.packetsUp = acc.packetsFwd;
            acc.packetsDown = acc.packetsBwd;
        } else {
            acc.determinedClientIp = dstIp;
            acc.determinedServerIp = srcIp;
            acc.determinedClientPort = dstPort;
            acc.determinedServerPort = srcPort;
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
                acc.srcIp,
                acc.dstIp,
                acc.srcPort,
                acc.dstPort,
                acc.protocol
        );
    }

    /**
     * 生成确定性 Session ID
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
     * SHA256 哈希
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
     * 确定最终结束原因
     */
    private String determineFinalEndReason(SessionAccumulator acc, String timerReason) {
        // 优先检查 TCP 标志位
        if ((acc.tcpFlagsFwd & TCP_FLAG_FIN) > 0 || (acc.tcpFlagsBwd & TCP_FLAG_FIN) > 0) {
            return "FIN";
        } else if ((acc.tcpFlagsFwd & TCP_FLAG_RST) > 0 || (acc.tcpFlagsBwd & TCP_FLAG_RST) > 0) {
            return "RST";
        }
        // 使用 Timer 原因
        return timerReason;
    }
}
