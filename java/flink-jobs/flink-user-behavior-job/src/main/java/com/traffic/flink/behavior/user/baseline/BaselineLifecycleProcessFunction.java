package com.traffic.flink.behavior.user.baseline;

import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Map;

/**
 * Stages approved immutable snapshots, emits a checkpoint-coupled ACK, and
 * switches online inference only after baseline.version.activated.v1.
 */
public final class BaselineLifecycleProcessFunction
        extends BroadcastProcessFunction<UserEvent, BaselineLifecycleEvent, BaselineAwareUserEvent> {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BaselineLifecycleProcessFunction.class);

    /** PROCESSED_EVENTS 重放守卫上限：超出后清空守卫（弱化冲突检测，不影响激活语义） */
    static final int MAX_PROCESSED_EVENTS = 10_000;

    public static final MapStateDescriptor<String, BaselineSnapshot> STAGED_BASELINES =
            new MapStateDescriptor<>("behavior-baseline-staged-v1",
                    TypeInformation.of(String.class), TypeInformation.of(BaselineSnapshot.class));
    public static final MapStateDescriptor<String, BaselineSnapshot> ACTIVE_BASELINES =
            new MapStateDescriptor<>("behavior-baseline-active-v1",
                    TypeInformation.of(String.class), TypeInformation.of(BaselineSnapshot.class));
    public static final MapStateDescriptor<String, String> PROCESSED_EVENTS =
            new MapStateDescriptor<>("behavior-baseline-events-v1", String.class, String.class);
    public static final OutputTag<BaselineActivationAck> ACTIVATION_ACKS =
            new OutputTag<BaselineActivationAck>("behavior-baseline-activation-acks-v1") {};

    private final String consumerId;

    public BaselineLifecycleProcessFunction(String consumerId) {
        if (consumerId == null || consumerId.isBlank()) {
            throw new IllegalArgumentException("behavior baseline consumer ID is required");
        }
        this.consumerId = consumerId;
    }

    @Override
    public void processElement(UserEvent event, ReadOnlyContext context, Collector<BaselineAwareUserEvent> out)
            throws Exception {
        ReadOnlyBroadcastState<String, BaselineSnapshot> active =
                context.getBroadcastState(ACTIVE_BASELINES);
        BaselineSnapshot snapshot = active.get(stateKey(event.getTenantId(), "user:" + event.getUserId()));
        if (snapshot == null) {
            snapshot = active.get(stateKey(event.getTenantId(), "tenant:" + event.getTenantId()));
        }
        out.collect(new BaselineAwareUserEvent(event, snapshot));
    }

    @Override
    public void processBroadcastElement(
            BaselineLifecycleEvent event, Context context, Collector<BaselineAwareUserEvent> out) throws Exception {
        BroadcastState<String, BaselineSnapshot> staged = context.getBroadcastState(STAGED_BASELINES);
        BroadcastState<String, BaselineSnapshot> active = context.getBroadcastState(ACTIVE_BASELINES);
        BroadcastState<String, String> processed = context.getBroadcastState(PROCESSED_EVENTS);
        String storedPayload = processed.get(event.eventId);
        if (storedPayload != null && !storedPayload.equals(event.payloadSha256)) {
            throw new IllegalStateException("behavior baseline event replay changed payload bytes");
        }
        if (event.isActivationRequested()) {
            if (!event.addressedTo(consumerId)) return;
            BaselineSnapshot snapshot = new BaselineSnapshot(event);
            BaselineSnapshot existing = staged.get(event.stateKey());
            if (existing != null && !existing.matchesActivated(event)) {
                throw new IllegalStateException("behavior baseline staged version changed without activation");
            }
            staged.put(event.stateKey(), snapshot);
            processed.put(event.eventId, event.payloadSha256);
            boundProcessedEvents(processed);
            context.output(ACTIVATION_ACKS, BaselineActivationAck.staged(event, consumerId));
            return;
        }
        if (event.isActivated()) {
            BaselineSnapshot snapshot = staged.get(event.stateKey());
            validateActivation(snapshot, event);
            active.put(event.stateKey(), snapshot);
            // 已激活的基线不再需要 staged 条目，避免广播 MapState 只增不减
            staged.remove(event.stateKey());
            processed.put(event.eventId, event.payloadSha256);
            boundProcessedEvents(processed);
            return;
        }
        if (event.isRetired()) {
            // 退役分支：从 active/staged 中移除，已激活基线可正常退役
            BaselineSnapshot snapshot = active.get(event.stateKey());
            if (snapshot == null) {
                snapshot = staged.get(event.stateKey());
            }
            if (snapshot != null) {
                active.remove(event.stateKey());
                staged.remove(event.stateKey());
                LOG.info("Behavior baseline retired: stateKey={}, version={}",
                        event.stateKey(), event.retiredByVersion);
            } else {
                LOG.warn("Retire event for unknown baseline ignored: stateKey={}", event.stateKey());
            }
            processed.put(event.eventId, event.payloadSha256);
            boundProcessedEvents(processed);
            return;
        }
    }

    /**
     * 约束 PROCESSED_EVENTS 重放守卫的大小：仅用于检测"同 eventId 重放但
     * payload 变化"，保留最近一批即可，超出上限后整体清空重放守卫。
     * 清空只弱化冲突检测，不影响激活/退役语义。
     */
    private static void boundProcessedEvents(BroadcastState<String, String> processed) throws Exception {
        int count = 0;
        for (Map.Entry<String, String> ignored : processed.entries()) {
            if (++count > MAX_PROCESSED_EVENTS) {
                processed.clear();
                LOG.warn("Behavior baseline replay guard exceeded {} events; guard reset",
                        MAX_PROCESSED_EVENTS);
                return;
            }
        }
    }

    static void validateActivation(BaselineSnapshot staged, BaselineLifecycleEvent activated) {
        if (staged == null || !staged.matchesActivated(activated)) {
            throw new IllegalStateException("activated behavior baseline does not match a staged immutable snapshot");
        }
    }

    private static String stateKey(String tenantId, String baselineId) {
        return tenantId + '\u001f' + baselineId;
    }
}
