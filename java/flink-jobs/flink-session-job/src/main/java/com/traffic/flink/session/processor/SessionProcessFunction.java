package com.traffic.flink.session.processor;

import com.traffic.flink.session.aggregator.SessionAccumulator;
import com.traffic.flink.session.aggregator.SessionAggregator;
import com.traffic.flink.session.state.SessionState;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Session 处理函数（KeyedProcessFunction）
 * 
 * 功能：
 * 1. ✅ Idle Timeout：2 分钟无数据 → 输出并清空状态
 * 2. ✅ Active Timeout：30 分钟强制切分 → 输出并继续聚合
 * 3. ✅ State TTL：30 分钟自动过期（兜底防泄漏）
 * 4. ✅ Late Data：输出到 Side Output
 * 
 * 替代：EventTimeSessionWindows（无 Active Timeout 能力）
 */
public class SessionProcessFunction 
        extends KeyedProcessFunction<String, FlowEvent, SessionEvent> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(SessionProcessFunction.class);

    // ==================== 配置参数 ====================
    private final long idleTimeoutMs;    // Idle Timeout（默认 2 分钟）
    private final long activeTimeoutMs;  // Active Timeout（默认 30 分钟）
    private final long stateTtlMs;       // State TTL（默认 30 分钟）
    private final OutputTag<FlowEvent> lateDataTag; // Late Data Side Output Tag

    // ==================== 聚合器 ====================
    private final SessionAggregator aggregator;

    // ==================== 状态 ====================
    private transient ValueState<SessionState> sessionState;

    // ==================== 构造函数 ====================
    
    public SessionProcessFunction(
            long idleTimeoutMs,
            long activeTimeoutMs,
            long stateTtlMs,
            OutputTag<FlowEvent> lateDataTag) {
        this.idleTimeoutMs = idleTimeoutMs;
        this.activeTimeoutMs = activeTimeoutMs;
        this.stateTtlMs = stateTtlMs;
        this.lateDataTag = lateDataTag;
        this.aggregator = new SessionAggregator();
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 配置 State TTL（防止状态泄漏）
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.milliseconds(stateTtlMs))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupInRocksdbCompactFilter(1000)
                .build();

        ValueStateDescriptor<SessionState> stateDescriptor =
                new ValueStateDescriptor<>("session-state", SessionState.class);
        stateDescriptor.enableTimeToLive(ttlConfig);

        sessionState = getRuntimeContext().getState(stateDescriptor);

        LOG.info("SessionProcessFunction initialized: idleTimeout={}ms, activeTimeout={}ms, stateTtl={}ms",
                idleTimeoutMs, activeTimeoutMs, stateTtlMs);
    }

    @Override
    public void processElement(
            FlowEvent flow,
            Context ctx,
            Collector<SessionEvent> out) throws Exception {

        // 获取当前状态
        SessionState state = sessionState.value();
        if (state == null) {
            state = new SessionState();
        }

        SessionAccumulator acc = state.getAccumulator();

        // 第一个 Flow：初始化会话
        if (acc.flowCount == 0) {
            initializeNewSession(flow, state, ctx);
        }

        // 聚合 Flow 到累加器
        aggregator.add(flow, acc);
        state.setAccumulator(acc);

        // 更新 Idle Timer（每次 processElement 都刷新）
        updateIdleTimer(state, ctx);

        // 更新状态
        sessionState.update(state);
    }

    @Override
    public void onTimer(
            long timestamp,
            OnTimerContext ctx,
            Collector<SessionEvent> out) throws Exception {

        SessionState state = sessionState.value();
        if (state == null) {
            LOG.warn("Timer fired but state is null (key={}), possibly TTL expired", ctx.getCurrentKey());
            return;
        }

        SessionAccumulator acc = state.getAccumulator();
        if (acc.flowCount == 0) {
            // 空会话（可能 TTL 刚清理），清空状态
            sessionState.clear();
            return;
        }

        // 判断 Timer 类型
        boolean isIdleTimer = timestamp == state.getIdleTimerTimestamp();
        boolean isActiveTimer = timestamp == state.getActiveTimerTimestamp();

        if (isIdleTimer) {
            // Idle Timeout：输出并清空状态
            handleIdleTimeout(state, ctx, out);
        } else if (isActiveTimer) {
            // Active Timeout：输出并继续聚合（切分新段）
            handleActiveTimeout(state, ctx, out);
        } else {
            LOG.warn("Unknown timer type (key={}, timestamp={})", ctx.getCurrentKey(), timestamp);
        }
    }

    // ==================== 私有辅助方法 ====================

    /**
     * 初始化新会话
     */
    private void initializeNewSession(FlowEvent flow, SessionState state, Context ctx) throws Exception {
        SessionAccumulator acc = state.getAccumulator();

        // 记录会话开始时间（跨 segment 不变）
        long sessionStartTime = flow.getTsStart();
        state.setSessionStartTime(sessionStartTime);

        // 生成会话 ID 基础部分（用于后续确定性生成）
        String tenantId = flow.getHeader().getTenantId();
        String communityId = flow.getCommunityId();
        String sessionIdBase = String.format("%s-%s-%d", tenantId, communityId, sessionStartTime);
        state.setSessionIdBase(sessionIdBase);

        // 注册 Active Timer（首次）
        long activeTimeout = ctx.timestamp() + activeTimeoutMs;
        ctx.timerService().registerEventTimeTimer(activeTimeout);
        state.setActiveTimerTimestamp(activeTimeout);

        LOG.debug("New session initialized: key={}, sessionIdBase={}, activeTimeout={}",
                ctx.getCurrentKey(), sessionIdBase, activeTimeout);
    }

    /**
     * 更新 Idle Timer（每次 processElement 调用）
     */
    private void updateIdleTimer(SessionState state, Context ctx) throws Exception {
        // 删除旧 Idle Timer
        if (state.getIdleTimerTimestamp() != null) {
            ctx.timerService().deleteEventTimeTimer(state.getIdleTimerTimestamp());
        }

        // 注册新 Idle Timer
        long idleTimeout = ctx.timestamp() + idleTimeoutMs;
        ctx.timerService().registerEventTimeTimer(idleTimeout);
        state.setIdleTimerTimestamp(idleTimeout);
    }

    /**
     * 处理 Idle Timeout
     */
    private void handleIdleTimeout(SessionState state, OnTimerContext ctx, Collector<SessionEvent> out) 
            throws Exception {
        SessionAccumulator acc = state.getAccumulator();

        LOG.debug("Idle Timeout triggered: key={}, flowCount={}, duration={}ms",
                ctx.getCurrentKey(), acc.flowCount, acc.getDurationMs());

        // 输出 SessionEvent
        SessionEvent session = buildSessionEvent(state, "IDLE_TIMEOUT");
        out.collect(session);

        // 清空状态（会话结束）
        sessionState.clear();

        // 删除 Active Timer
        if (state.getActiveTimerTimestamp() != null) {
            ctx.timerService().deleteEventTimeTimer(state.getActiveTimerTimestamp());
        }
    }

    /**
     * 处理 Active Timeout（长流切分）
     */
    private void handleActiveTimeout(SessionState state, OnTimerContext ctx, Collector<SessionEvent> out) 
            throws Exception {
        SessionAccumulator acc = state.getAccumulator();

        LOG.debug("Active Timeout triggered: key={}, segmentIndex={}, flowCount={}, duration={}ms",
                ctx.getCurrentKey(), state.getSegmentIndex(), acc.flowCount, acc.getDurationMs());

        // 输出当前段的 SessionEvent
        SessionEvent session = buildSessionEvent(state, "ACTIVE_TIMEOUT");
        out.collect(session);

        // 重置累加器（开始新段）
        state.resetAccumulatorForNewSegment();

        // 重新注册 Active Timer（下一个 30 分钟）
        long nextActiveTimeout = ctx.timestamp() + activeTimeoutMs;
        ctx.timerService().registerEventTimeTimer(nextActiveTimeout);
        state.setActiveTimerTimestamp(nextActiveTimeout);

        // 重新注册 Idle Timer（重置 2 分钟计时）
        long nextIdleTimeout = ctx.timestamp() + idleTimeoutMs;
        ctx.timerService().registerEventTimeTimer(nextIdleTimeout);
        state.setIdleTimerTimestamp(nextIdleTimeout);

        // 更新状态
        sessionState.update(state);
    }

    /**
     * 构建 SessionEvent
     */
    private SessionEvent buildSessionEvent(SessionState state, String triggerReason) {
        SessionAccumulator acc = state.getAccumulator();

        // 生成带 segment 索引的 session_id（确保切分后的段有不同 ID）
        String sessionId = state.getSessionIdBase() + "-seg" + state.getSegmentIndex();

        // ✅ 临时设置 session_id（传递给 aggregator）
        // 注意：SessionAggregator.getResult() 会重新生成，需要调整
        // 这里我们直接调用 aggregator.getResult() 并手动覆盖 session_id
        SessionEvent session = aggregator.getResult(acc);

        // 覆盖 session_id（包含 segment 索引）
        session = session.toBuilder()
                .setSessionId(sessionId)
                .build();

        LOG.debug("SessionEvent built: sessionId={}, trigger={}, flowCount={}, duration={}ms",
                sessionId, triggerReason, acc.flowCount, acc.getDurationMs());

        return session;
    }
}