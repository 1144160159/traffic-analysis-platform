package com.traffic.flink.behavior.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.FeatureStat;

import java.io.Serializable;

/** A validated feature plus its immutable Kafka source coordinates. */
public final class ValidatedBehaviorFeature implements Serializable {
    private static final long serialVersionUID = 1L;
    private final RawKafkaRecord source;
    private final byte[] featureBytes;
    private transient FeatureStat feature;

    public ValidatedBehaviorFeature(RawKafkaRecord source, FeatureStat feature) {
        this.source = source;
        this.feature = feature;
        this.featureBytes = feature.toByteArray();
    }

    public RawKafkaRecord getSource() { return source; }
    public FeatureStat getFeature() {
        if (feature == null) {
            try {
                feature = FeatureStat.parseFrom(featureBytes);
            } catch (java.io.IOException error) {
                throw new IllegalStateException("validated FeatureStat bytes are invalid", error);
            }
        }
        return feature;
    }
}
