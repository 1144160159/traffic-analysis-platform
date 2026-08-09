package com.traffic.flink.alert;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class AlertGeneratorJobTest {
    @Test
    void v2TargetRequiresExplicitEnable() {
        assertEquals("alerts", AlertGeneratorJob.resolveOpenSearchWriteTarget(
                false, "alerts", "alerts-v2-write"));
        assertEquals("alerts-v2-write", AlertGeneratorJob.resolveOpenSearchWriteTarget(
                true, "alerts", "alerts-v2-write"));
    }

    @Test
    void blankSelectedTargetFailsClosed() {
        assertThrows(IllegalArgumentException.class,
                () -> AlertGeneratorJob.resolveOpenSearchWriteTarget(true, "alerts", " "));
    }
}
