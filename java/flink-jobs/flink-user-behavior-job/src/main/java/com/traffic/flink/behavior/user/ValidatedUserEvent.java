package com.traffic.flink.behavior.user;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.UserEvent;

import java.io.Serializable;

/** UserEvent paired with the Kafka tuple that owns offset and replay identity. */
public final class ValidatedUserEvent implements Serializable {
    private static final long serialVersionUID = 1L;
    private final RawKafkaRecord source;
    private final UserEvent event;

    public ValidatedUserEvent(RawKafkaRecord source, UserEvent event) {
        if (source == null || event == null) throw new IllegalArgumentException("source and event are required");
        this.source = source;
        this.event = event;
    }

    public RawKafkaRecord getSource() { return source; }
    public UserEvent getEvent() { return event; }
    public String identityKey() { return event.getTenantId() + "|" + event.getUserId(); }
}
