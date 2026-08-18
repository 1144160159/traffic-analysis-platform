package com.traffic.flink.behavior.config;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class BehaviorActivationPolicyTest {

    @Test
    void defaultIsOffAndStartsNoProducer() {
        BehaviorActivationPolicy policy = BehaviorActivationPolicy.validate(
                new BehaviorJobConfig.Builder().build());

        assertEquals(BehaviorActivationPolicy.Mode.OFF, policy.getMode());
        assertFalse(policy.shouldRun());
    }

    @Test
    void frozenKnownProfilePinsArtifactModelSetAndThresholds() {
        BehaviorActivationPolicy policy = BehaviorActivationPolicy.validate(
                new BehaviorJobConfig.Builder().detectionMode("known_frozen").build());

        assertTrue(policy.shouldRun());
        assertFalse(policy.allowsHotUpdates());
        assertEquals("m04-known-behavior-v1", policy.getProfileId());
        assertEquals(
                "3308cd498548716c68b79c8f665f5a1cb6d7b1d95769234853bbf1e9f7a03cdb",
                policy.getProfileSha256());
    }

    @Test
    void profileHashDriftFailsClosed() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .detectionMode("known_frozen")
                .knownProfileSha256("0000000000000000000000000000000000000000000000000000000000000000")
                .build();

        assertThrows(IllegalArgumentException.class,
                () -> BehaviorActivationPolicy.validate(config));
    }

    @Test
    void thresholdDriftFailsClosed() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .detectionMode("known_frozen")
                .scanThreshold(0.71f)
                .build();

        assertThrows(IllegalArgumentException.class,
                () -> BehaviorActivationPolicy.validate(config));
    }

    @Test
    void frozenModeRejectsHotUpdates() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .detectionMode("known_frozen")
                .modelHotUpdateEnabled(true)
                .build();

        assertThrows(IllegalArgumentException.class,
                () -> BehaviorActivationPolicy.validate(config));
    }
}
