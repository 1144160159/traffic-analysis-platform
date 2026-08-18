package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;
import org.apache.flink.api.common.functions.MapFunction;

import java.util.Base64;

/** Converts session-job side outputs to the canonical dlq.v1 protobuf. */
public final class CanonicalDeadLetterMapper {
    private CanonicalDeadLetterMapper() { }

    public static final class LateFlow implements MapFunction<FlowEvent, DeadLetter> {
        private static final long serialVersionUID = 1L;

        @Override
        public DeadLetter map(FlowEvent event) {
            if (event == null || !event.hasHeader()) {
                throw new IllegalArgumentException("late FlowEvent requires an authoritative header");
            }
            return DeadLetter.newBuilder()
                    .setEventId("flink-session-late:" + event.getHeader().getEventId())
                    .setTenantId(event.getHeader().getTenantId())
                    .setSourceTopic("flow.events.v1")
                    .setSourceKey(event.getCommunityId())
                    .setErrorMsg("event_time_exceeded_session_allowed_lateness")
                    .setRawPayload(Base64.getEncoder().encodeToString(event.toByteArray()))
                    .setRetryCount(0)
                    .setCreatedAt(System.currentTimeMillis())
                    .build();
        }
    }

    public static final class SessionSinkFailure implements MapFunction<SessionEvent, DeadLetter> {
        private static final long serialVersionUID = 1L;
        private final String sink;

        public SessionSinkFailure(String sink) {
            if (sink == null || sink.trim().isEmpty()) {
                throw new IllegalArgumentException("failed sink identity is required");
            }
            this.sink = sink;
        }

        @Override
        public DeadLetter map(SessionEvent event) {
            if (event == null || !event.hasHeader()) {
                throw new IllegalArgumentException("failed SessionEvent requires an authoritative header");
            }
            return DeadLetter.newBuilder()
                    .setEventId("flink-session-sink-" + sink + ":" + event.getHeader().getEventId())
                    .setTenantId(event.getHeader().getTenantId())
                    .setSourceTopic("session.events.v1")
                    .setSourceKey(event.getCommunityId())
                    .setErrorMsg(sink + "_projection_failed")
                    .setRawPayload(Base64.getEncoder().encodeToString(event.toByteArray()))
                    .setRetryCount(0)
                    .setCreatedAt(System.currentTimeMillis())
                    .build();
        }
    }
}
