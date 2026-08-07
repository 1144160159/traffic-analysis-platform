package com.traffic.flink.behavior.user;

import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.util.Base64;

/** Routes events outside the declared business-time allowance to a durable DLQ side output. */
public class LateUserEventRouter extends ProcessFunction<UserEvent, UserEvent> {
    private static final long serialVersionUID = 1L;

    public static final OutputTag<DeadLetter> LATE_EVENTS = new OutputTag<DeadLetter>(
            "late-user-events-v1", TypeInformation.of(DeadLetter.class)) { };

    private final long allowedLatenessMs;
    private final String sourceTopic;

    public LateUserEventRouter(long allowedLatenessMs, String sourceTopic) {
        if (allowedLatenessMs < 0) {
            throw new IllegalArgumentException("allowedLatenessMs must not be negative");
        }
        this.allowedLatenessMs = allowedLatenessMs;
        this.sourceTopic = sourceTopic;
    }

    @Override
    public void processElement(UserEvent event, Context context, Collector<UserEvent> out) {
        long watermark = context.timerService().currentWatermark();
        if (isTooLate(event.getTimestamp(), watermark, allowedLatenessMs)) {
            context.output(LATE_EVENTS, toDeadLetter(
                    event, sourceTopic, watermark, allowedLatenessMs));
            return;
        }
        out.collect(event);
    }

    static boolean isTooLate(long eventTimestamp, long watermark, long allowedLatenessMs) {
        return watermark != Long.MIN_VALUE
                && eventTimestamp <= cutoff(watermark, allowedLatenessMs);
    }

    static DeadLetter toDeadLetter(
            UserEvent event, String sourceTopic, long watermark, long allowedLatenessMs) {
        long cutoff = cutoff(watermark, allowedLatenessMs);
        String sourceKey = event.getTenantId() + "|" + event.getUserId();
        return DeadLetter.newBuilder()
                .setEventId(DeterministicId.uuid(
                        "flink-user-event-late/v1",
                        event.getEventId(), sourceTopic, sourceKey, event.getTimestamp(), cutoff))
                .setTenantId(event.getTenantId())
                .setSourceTopic(sourceTopic)
                .setSourceKey(sourceKey)
                .setErrorMsg("event_time_older_than_allowed_lateness; watermark="
                        + watermark + "; cutoff=" + cutoff)
                .setRawPayload(Base64.getEncoder().encodeToString(event.toByteArray()))
                .setRetryCount(0)
                .setCreatedAt(event.getTimestamp())
                .build();
    }

    private static long cutoff(long watermark, long allowedLatenessMs) {
        if (watermark == Long.MIN_VALUE || watermark < Long.MIN_VALUE + allowedLatenessMs) {
            return Long.MIN_VALUE;
        }
        return watermark - allowedLatenessMs;
    }
}
