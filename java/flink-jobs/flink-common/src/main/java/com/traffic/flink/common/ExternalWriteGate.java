package com.traffic.flink.common;

import org.apache.flink.api.common.functions.FilterFunction;

import java.util.Objects;

/** Stateless defense-in-depth gate immediately before external sinks. */
public final class ExternalWriteGate<T> implements FilterFunction<T> {
    private static final long serialVersionUID = 1L;
    private final DeploymentActivation activation;

    public ExternalWriteGate(DeploymentActivation activation) {
        this.activation = Objects.requireNonNull(activation, "activation");
    }

    @Override
    public boolean filter(T value) {
        return activation.externalWritesEnabled();
    }
}
