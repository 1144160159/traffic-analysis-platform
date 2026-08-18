package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/** Checkpoint-safe carrier from broadcast candidate state to the async observer. */
public final class ShadowEvaluationRequest implements Serializable {
    private static final long serialVersionUID = 1L;

    private final byte[] featureBytes;
    private transient FeatureStat feature;
    private final List<ModelUpdateEvent> candidates;

    public ShadowEvaluationRequest(FeatureStat feature, List<ModelUpdateEvent> candidates) {
        if (feature == null) throw new IllegalArgumentException("feature is required");
        if (candidates == null || candidates.isEmpty()) {
            throw new IllegalArgumentException("at least one shadow candidate is required");
        }
        this.feature = feature;
        this.featureBytes = feature.toByteArray();
        // Keep the serialized field mutable: Flink's Kryo collection serializer
        // reconstructs lists by adding elements. The getter remains read-only.
        this.candidates = new ArrayList<>(candidates);
    }

    public FeatureStat getFeature() {
        if (feature == null) {
            try {
                feature = FeatureStat.parseFrom(featureBytes);
            } catch (java.io.IOException error) {
                throw new IllegalStateException("shadow FeatureStat bytes are invalid", error);
            }
        }
        return feature;
    }
    public List<ModelUpdateEvent> getCandidates() {
        return Collections.unmodifiableList(candidates);
    }
}
