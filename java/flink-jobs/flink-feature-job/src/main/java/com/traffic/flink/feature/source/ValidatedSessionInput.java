package com.traffic.flink.feature.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.SessionEvent;

import java.io.Serializable;

/** Validated SessionEvent paired with the immutable Kafka source tuple. */
public final class ValidatedSessionInput implements Serializable {

    private static final long serialVersionUID = 1L;

    private final RawKafkaRecord source;
    private final SessionEvent session;

    public ValidatedSessionInput(RawKafkaRecord source, SessionEvent session) {
        this.source = source;
        this.session = session;
    }

    public RawKafkaRecord getSource() { return source; }
    public SessionEvent getSession() { return session; }
}
