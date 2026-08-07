package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.kafka.clients.consumer.OffsetResetStrategy;

import java.util.Locale;

/** Contracted Kafka source startup policy shared by every Flink data job. */
public final class KafkaStartingOffsets {
    public static final String CONFIG_KEY = "kafka.starting.offsets";
    public static final String DEFAULT_MODE = "committed-or-earliest";

    private KafkaStartingOffsets() {
    }

    public static OffsetsInitializer from(ParameterTool params) {
        return parse(ConfigUtils.get(params, CONFIG_KEY, DEFAULT_MODE));
    }

    static OffsetsInitializer parse(String rawMode) {
        String mode = rawMode == null ? DEFAULT_MODE : rawMode.trim().toLowerCase(Locale.ROOT);
        switch (mode) {
            case "committed-or-earliest":
                return OffsetsInitializer.committedOffsets(OffsetResetStrategy.EARLIEST);
            case "earliest":
                return OffsetsInitializer.earliest();
            case "latest":
                // Latest remains an explicit migration-only compatibility option. It is never
                // the default because a new or expired consumer group would skip retained facts.
                return OffsetsInitializer.latest();
            default:
                throw new IllegalArgumentException(
                        "Unsupported " + CONFIG_KEY + " mode: " + rawMode
                                + "; expected committed-or-earliest, earliest or latest");
        }
    }
}
