package com.traffic.flink.session.source;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Deduplicates FlowEvent identity and classifies lateness without losing source coordinates. */
public final class FlowLatenessFunction
        extends KeyedProcessFunction<String, ValidatedFlowInput, FlowEvent> {
    private static final long serialVersionUID = 1L;

    private final EventTimePolicy policy;
    private final String consumerGroup;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final OutputTag<SourceQualityReceipt> qualityTag;
    private final OutputTag<ValidatedFlowInput> acceptedFactTag;
    private transient MapState<String, String> eventHashes;

    public FlowLatenessFunction(
            long allowedLatenessMs,
            OutputTag<CanonicalDlqMessage> dlqTag) {
        this(new EventTimePolicy(0L, 1L, allowedLatenessMs, Long.MAX_VALUE, 0L),
                "", dlqTag, null, null);
    }

    public FlowLatenessFunction(
            EventTimePolicy policy,
            String consumerGroup,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag) {
        this(policy, consumerGroup, dlqTag, qualityTag, null);
    }

    public FlowLatenessFunction(
            EventTimePolicy policy,
            String consumerGroup,
            OutputTag<CanonicalDlqMessage> dlqTag,
            OutputTag<SourceQualityReceipt> qualityTag,
            OutputTag<ValidatedFlowInput> acceptedFactTag) {
        if (policy == null) throw new IllegalArgumentException("event-time policy is required");
        if (dlqTag == null) throw new IllegalArgumentException("late-data DLQ tag is required");
        this.policy = policy;
        this.consumerGroup = consumerGroup == null ? "" : consumerGroup;
        this.dlqTag = dlqTag;
        this.qualityTag = qualityTag;
        this.acceptedFactTag = acceptedFactTag;
    }

    @Override
    public void open(Configuration parameters) {
        MapStateDescriptor<String, String> descriptor = new MapStateDescriptor<>(
                "flow-event-source-hash-v1", String.class, String.class);
        descriptor.enableTimeToLive(StateTtlConfig.newBuilder(Time.days(7))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .build());
        eventHashes = getRuntimeContext().getMapState(descriptor);
    }

    @Override
    public void processElement(
            ValidatedFlowInput input,
            Context context,
            Collector<FlowEvent> out) throws Exception {
        long watermark = context.timerService().currentWatermark();
        FlowEvent flow = input.getFlow();
        String sourceHash = SourceQualityReceipt.hashSource(input.getSource().getValue());
        String previousHash = eventHashes.get(flow.getHeader().getEventId());
        if (previousHash != null) {
            if (previousHash.equals(sourceHash)) {
                emitReceipt(context, input, sourceHash, "duplicate", "DUPLICATE_EVENT", watermark);
                return;
            }
            context.output(dlqTag, conflictFailure(input));
            emitReceipt(context, input, sourceHash, "conflict", "EVENT_ID_CONFLICT", watermark);
            return;
        }
        if (isTooLate(flow.getTsEnd(), watermark, policy.getAllowedLatenessMs())) {
            context.output(dlqTag, lateFailure(input, watermark, policy.getAllowedLatenessMs()));
            emitReceipt(context, input, sourceHash, "late", "SUPER_LATE_EVENT", watermark);
            return;
        }
        eventHashes.put(flow.getHeader().getEventId(), sourceHash);
        emitReceipt(context, input, sourceHash, "accepted", "", watermark);
        if (acceptedFactTag != null) context.output(acceptedFactTag, input);
        out.collect(flow);
    }

    static boolean isTooLate(long eventTimestamp, long watermark, long allowedLatenessMs) {
        return EventTimePolicy.isLate(eventTimestamp, watermark, allowedLatenessMs);
    }

    public static CanonicalDlqMessage lateFailure(
            ValidatedFlowInput input,
            long watermark,
            long allowedLatenessMs) {
        FlowEvent flow = input.getFlow();
        EventHeader header = flow.hasHeader()
                ? flow.getHeader() : EventHeader.getDefaultInstance();
        return CanonicalDlqMessage.failure(
                input.getSource(),
                "SUPER_LATE_EVENT",
                "event_time_lateness",
                "event_time_exceeded_session_allowed_lateness: event_time_ms=" + flow.getTsEnd()
                        + ", watermark_ms=" + watermark
                        + ", allowed_lateness_ms=" + allowedLatenessMs,
                header.getTenantId(), header.getEventId(), header.getTraceId(),
                header.getRunId(), header.getProbeId(),
                "flink-session-job", "traffic.v1.FlowEvent", "v1");
    }

    static CanonicalDlqMessage conflictFailure(ValidatedFlowInput input) {
        FlowEvent flow = input.getFlow();
        EventHeader header = flow.getHeader();
        return CanonicalDlqMessage.failure(
                input.getSource(), "EVENT_ID_CONFLICT", "flow_event_quality",
                "same FlowEvent event_id has another payload",
                header.getTenantId(), header.getEventId(), header.getTraceId(),
                header.getRunId(), header.getProbeId(),
                "flink-session-job", "traffic.v1.FlowEvent", "v1");
    }

    private void emitReceipt(
            Context context,
            ValidatedFlowInput input,
            String sourceHash,
            String category,
            String reasonCode,
            long watermark) {
        if (qualityTag == null) return;
        FlowEvent flow = input.getFlow();
        context.output(qualityTag, new SourceQualityReceipt(
                flow.getHeader().getTenantId(), "flow", consumerGroup,
                input.getSource().getTopic(), input.getSource().getPartition(),
                input.getSource().getOffset(), category, flow.getHeader().getEventId(),
                sourceHash, watermark, input.getSource().getTimestamp(), reasonCode));
    }

}
