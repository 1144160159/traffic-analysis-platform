package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.Collections;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;

class KafkaStartingOffsetsTest {
    @Test
    void defaultIsCommittedWithEarliestFallback() {
        ParameterTool params = ParameterTool.fromMap(Collections.emptyMap());
        assertDoesNotThrow(() -> KafkaStartingOffsets.from(params));
    }

    @Test
    void supportedModesAreExplicitAndUnknownModesFailClosed() {
        assertDoesNotThrow(() -> KafkaStartingOffsets.parse("committed-or-earliest"));
        assertDoesNotThrow(() -> KafkaStartingOffsets.parse("earliest"));
        assertDoesNotThrow(() -> KafkaStartingOffsets.parse("latest"));
        assertThrows(IllegalArgumentException.class, () -> KafkaStartingOffsets.parse("magic"));
    }
}
