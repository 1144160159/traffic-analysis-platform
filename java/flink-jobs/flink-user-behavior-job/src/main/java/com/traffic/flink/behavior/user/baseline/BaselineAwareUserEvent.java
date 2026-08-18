package com.traffic.flink.behavior.user.baseline;

import com.traffic.proto.traffic.v1.UserEvent;

import java.io.Serializable;

/** User event paired with the exact active baseline observed by the operator. */
public final class BaselineAwareUserEvent implements Serializable {
    private static final long serialVersionUID = 1L;

    private final UserEvent event;
    private final BaselineSnapshot baseline;

    public BaselineAwareUserEvent(UserEvent event, BaselineSnapshot baseline) {
        if (event == null) throw new IllegalArgumentException("user event is required");
        this.event = event;
        this.baseline = baseline;
    }

    public UserEvent getEvent() { return event; }
    public BaselineSnapshot getBaseline() { return baseline; }

    public int intThreshold(String name, int fallback, int minimum, int maximum) {
        return baseline == null ? fallback : baseline.intThreshold(name, fallback, minimum, maximum);
    }

    public long longThreshold(String name, long fallback, long minimum, long maximum) {
        return baseline == null ? fallback : baseline.longThreshold(name, fallback, minimum, maximum);
    }
}
