package com.traffic.flink.session.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.FlowEvent;

import java.io.Serializable;

/** Validated FlowEvent paired with its immutable broker source coordinates. */
public final class ValidatedFlowInput implements Serializable {
    private static final long serialVersionUID = 1L;

    private final RawKafkaRecord source;
    private final FlowEvent flow;

    public ValidatedFlowInput(RawKafkaRecord source, FlowEvent flow) {
        if (source == null || flow == null) {
            throw new IllegalArgumentException("validated flow input requires source and FlowEvent");
        }
        this.source = source;
        this.flow = flow;
    }

    public RawKafkaRecord getSource() { return source; }
    public FlowEvent getFlow() { return flow; }

    /** Stable keyed-state scope; Kafka is partitioned by this same identity. */
    public String identityKey() {
        return flow.getHeader().getTenantId() + "|" + flow.getCommunityId();
    }
}
