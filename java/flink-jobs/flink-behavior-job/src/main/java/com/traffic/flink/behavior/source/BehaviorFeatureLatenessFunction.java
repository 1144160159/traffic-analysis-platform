package com.traffic.flink.behavior.source;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Routes features beyond the explicit event-time lateness budget to canonical DLQ. */
public final class BehaviorFeatureLatenessFunction
        extends ProcessFunction<ValidatedBehaviorFeature, FeatureStat> {

    private static final long serialVersionUID = 1L;
    private final long allowedLatenessMs;
    private final OutputTag<CanonicalDlqMessage> dlqTag;

    public BehaviorFeatureLatenessFunction(
            long allowedLatenessMs, OutputTag<CanonicalDlqMessage> dlqTag) {
        if (allowedLatenessMs < 0) throw new IllegalArgumentException("allowed lateness is negative");
        this.allowedLatenessMs = allowedLatenessMs;
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(
            ValidatedBehaviorFeature input, Context context, Collector<FeatureStat> out) {
        long watermark = context.timerService().currentWatermark();
        if (isLate(input.getFeature().getTs(), watermark, allowedLatenessMs)) {
            context.output(dlqTag, lateFailure(input, watermark, allowedLatenessMs));
        } else {
            out.collect(input.getFeature());
        }
    }

    static boolean isLate(long eventTime, long watermark, long allowedLatenessMs) {
        return watermark != Long.MIN_VALUE && eventTime < watermark - allowedLatenessMs;
    }

    static CanonicalDlqMessage lateFailure(
            ValidatedBehaviorFeature input, long watermark, long allowedLatenessMs) {
        EventHeader header = input.getFeature().getHeader();
        String message = "FeatureStat event_time=" + input.getFeature().getTs()
                + " is older than watermark=" + watermark
                + " with allowed_lateness_ms=" + allowedLatenessMs;
        return CanonicalDlqMessage.failure(
                input.getSource(), "LATE_EVENT", "event_time_lateness", message,
                header.getTenantId(), header.getEventId(), header.getTraceId(),
                header.getRunId(), header.getProbeId(),
                "flink-behavior-job", "traffic.v1.FeatureStat", "v1");
    }
}
