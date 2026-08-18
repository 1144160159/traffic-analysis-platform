package com.traffic.flink.common.eventtime;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class EventTimePolicyTest {
    private final EventTimePolicy policy = new EventTimePolicy(10L, 1_000L, 100L, 500L, 50L);

    @Test
    void strictLateBoundaryAndUninitializedWatermarkAreStable() {
        assertFalse(EventTimePolicy.isLate(900L, Long.MIN_VALUE, 100L));
        assertFalse(EventTimePolicy.isLate(900L, 1_000L, 100L));
        assertTrue(EventTimePolicy.isLate(899L, 1_000L, 100L));
        assertFalse(EventTimePolicy.isLate(Long.MIN_VALUE, Long.MIN_VALUE, Long.MAX_VALUE));
    }

    @Test
    void classificationOrderIsDeterministic() {
        assertEquals(EventTimePolicy.Status.FUTURE_EVENT,
                policy.classify(1_501L, 1_000L, 2_000L, Long.MIN_VALUE, null).getStatus());
        assertEquals(EventTimePolicy.Status.CLOCK_ROLLBACK,
                policy.classify(949L, 1_500L, 2_000L, 1_200L, 1_000L).getStatus());
        assertEquals(EventTimePolicy.Status.LATE_EVENT,
                policy.classify(1_099L, 1_500L, 2_000L, 1_200L, null).getStatus());
        assertEquals(EventTimePolicy.Status.ACCEPT,
                policy.classify(1_100L, 1_500L, 2_000L, 1_200L, null).getStatus());
    }

    @Test
    void asOfUsesWatermarkButNeverMovesIntoProcessingFuture() {
        assertEquals(2_000L, EventTimePolicy.effectiveAsOf(Long.MIN_VALUE, 2_000L));
        assertEquals(1_900L, EventTimePolicy.effectiveAsOf(1_900L, 2_000L));
        assertEquals(2_000L, EventTimePolicy.effectiveAsOf(2_100L, 2_000L));
        assertThrows(IllegalArgumentException.class,
                () -> EventTimePolicy.effectiveAsOf(1L, 0L));
    }

    @Test
    void overflowAndPolicyConstructionAreSafe() {
        assertFalse(EventTimePolicy.isFuture(Long.MAX_VALUE, Long.MAX_VALUE, Long.MAX_VALUE));
        assertFalse(EventTimePolicy.isClockRollback(
                Long.MIN_VALUE, Long.MIN_VALUE, Long.MAX_VALUE));
        assertThrows(IllegalArgumentException.class,
                () -> new EventTimePolicy(0L, 0L, 0L, 0L, 0L));
        WatermarkStrategy<Long> strategy = policy.watermarkStrategy(value -> value);
        assertTrue(strategy != null);
    }
}
