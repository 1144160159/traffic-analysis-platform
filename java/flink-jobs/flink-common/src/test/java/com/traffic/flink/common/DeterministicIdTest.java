package com.traffic.flink.common;

import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DeterministicIdTest {
    @Test
    void uuidIsStableAndUsesVersionEight() {
        String first = DeterministicId.uuid("feature-event/v1", "tenant-a", "session-7", 42L);
        String second = DeterministicId.uuid("feature-event/v1", "tenant-a", "session-7", 42L);

        assertEquals(first, second);
        assertEquals(8, UUID.fromString(first).version());
        assertEquals(2, UUID.fromString(first).variant());
    }

    @Test
    void lengthPrefixesPreventConcatenationAndNullCollisions() {
        assertNotEquals(
                DeterministicId.uuid("test/v1", "ab", "c"),
                DeterministicId.uuid("test/v1", "a", "bc"));
        assertNotEquals(
                DeterministicId.uuid("test/v1", (Object) null),
                DeterministicId.uuid("test/v1", ""));
    }

    @Test
    void sortedEventSetIsOrderIndependentButDuplicateSensitive() {
        String first = DeterministicId.uuidFromSorted(
                "campaign/v1", Arrays.asList("event-b", "event-a"), "tenant-a", "rule-v3");
        String reordered = DeterministicId.uuidFromSorted(
                "campaign/v1", Arrays.asList("event-a", "event-b"), "tenant-a", "rule-v3");
        String duplicate = DeterministicId.uuidFromSorted(
                "campaign/v1", Arrays.asList("event-a", "event-b", "event-b"), "tenant-a", "rule-v3");

        assertEquals(first, reordered);
        assertNotEquals(first, duplicate);
    }

    @Test
    void samplingIsStableAndHonorsBoundaries() {
        boolean first = DeterministicId.sample(0.37d, "feature-sampling/v1", "tenant-a", "session-7");
        boolean second = DeterministicId.sample(0.37d, "feature-sampling/v1", "tenant-a", "session-7");

        assertEquals(first, second);
        assertFalse(DeterministicId.sample(0.0d, "feature-sampling/v1", "x"));
        assertTrue(DeterministicId.sample(1.0d, "feature-sampling/v1", "x"));
        assertThrows(IllegalArgumentException.class,
                () -> DeterministicId.sample(1.01d, "feature-sampling/v1", "x"));
    }

    @Test
    void fixedVectorDetectsAccidentalContractChanges() {
        assertEquals("8202c851-ab0c-89da-b641-a60c261a90cf",
                DeterministicId.uuid("feature-event/v1", "tenant-a", "session-7", 42L));
        assertEquals("8202c851ab0c",
                DeterministicId.shortId("feature-event/v1", 12, "tenant-a", "session-7", 42L));
    }
}
