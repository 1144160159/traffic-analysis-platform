package com.traffic.flink.behavior.user;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Deduplicates UserEvent identity and applies the shared strict late boundary. */
public final class UserEventTimeFunction
        extends KeyedProcessFunction<String, ValidatedUserEvent, UserEvent> {
    private static final long serialVersionUID = 1L;

    private final EventTimePolicy policy;
    private final String consumerGroup;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final OutputTag<SourceQualityReceipt> qualityTag;
    private final OutputTag<ValidatedUserEvent> acceptedFactTag;
    private transient MapState<String, String> eventHashes;

    public UserEventTimeFunction(
            EventTimePolicy policy,
            String consumerGroup,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag) {
        this(policy, consumerGroup, dlqTag, qualityTag, null);
    }

    public UserEventTimeFunction(
            EventTimePolicy policy,
            String consumerGroup,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag,
            OutputTag<ValidatedUserEvent> acceptedFactTag) {
        this.policy = policy;
        this.consumerGroup = consumerGroup;
        this.dlqTag = dlqTag;
        this.qualityTag = qualityTag;
        this.acceptedFactTag = acceptedFactTag;
    }

    @Override
    public void open(Configuration parameters) {
        MapStateDescriptor<String, String> descriptor =
                new MapStateDescriptor<>("user-event-source-hash-v1", String.class, String.class);
        descriptor.enableTimeToLive(StateTtlConfig.newBuilder(Time.days(7))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .build());
        eventHashes = getRuntimeContext().getMapState(descriptor);
    }

    @Override
    public void processElement(
            ValidatedUserEvent input,
            Context context,
            Collector<UserEvent> out) throws Exception {
        UserEvent event = input.getEvent();
        String sourceHash = SourceQualityReceipt.hashSource(input.getSource().getValue());
        String previousHash = eventHashes.get(event.getEventId());
        long watermark = context.timerService().currentWatermark();
        if (previousHash != null) {
            if (previousHash.equals(sourceHash)) {
                emitReceipt(context, input, sourceHash, "duplicate", "DUPLICATE_EVENT", watermark);
                return;
            }
            context.output(dlqTag, failure(input, "EVENT_ID_CONFLICT", "same event_id has another payload"));
            emitReceipt(context, input, sourceHash, "conflict", "EVENT_ID_CONFLICT", watermark);
            return;
        }
        if (EventTimePolicy.isLate(event.getTimestamp(), watermark, policy.getAllowedLatenessMs())) {
            context.output(dlqTag, failure(input, "LATE_EVENT", "event_time is before allowed boundary"));
            emitReceipt(context, input, sourceHash, "late", "LATE_EVENT", watermark);
            return;
        }
        eventHashes.put(event.getEventId(), sourceHash);
        emitReceipt(context, input, sourceHash, "accepted", "", watermark);
        if (acceptedFactTag != null) context.output(acceptedFactTag, input);
        out.collect(event);
    }

    private void emitReceipt(
            Context context,
            ValidatedUserEvent input,
            String sourceHash,
            String category,
            String reason,
            long watermark) {
        UserEvent event = input.getEvent();
        context.output(qualityTag, new SourceQualityReceipt(
                event.getTenantId(), "user_behavior", consumerGroup,
                input.getSource().getTopic(), input.getSource().getPartition(),
                input.getSource().getOffset(), category, event.getEventId(), sourceHash,
                watermark, input.getSource().getTimestamp(), reason));
    }

    private static CanonicalDlqMessage failure(
            ValidatedUserEvent input, String code, String message) {
        UserEvent event = input.getEvent();
        return CanonicalDlqMessage.failure(
                input.getSource(), code, "user_event_quality", message,
                event.getTenantId(), event.getEventId(),
                input.getSource().header("trace_id"), input.getSource().header("run_id"), "",
                "flink-user-behavior-job", "traffic.v1.UserEvent", "v1");
    }
}
