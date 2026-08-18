package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import org.junit.jupiter.api.Test;

import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ModelConsumerReadinessTest {
    private static final String PROFILE = "a".repeat(64);

    @Test
    void readinessIdentityIsStableAndDeclaresBothModelFormats() {
        ModelUpdateAppliedAck first = ModelUpdateAppliedAck.consumerReady(
                "behavior-r1", PROFILE, "traffic.behavior.inference.v1", "1.0.0", 1, 1, 2, 4);
        ModelUpdateAppliedAck replay = ModelUpdateAppliedAck.consumerReady(
                "behavior-r1", PROFILE, "traffic.behavior.inference.v1", "1.0.0", 1, 1, 2, 4);
        assertEquals(first.eventId, replay.eventId);
        assertEquals(8, UUID.fromString(first.eventId).version());
        assertEquals("consumer_ready", first.ackType);
        assertEquals("consumer_ready", first.status);
        assertEquals(2, first.subtaskIndex);
        assertEquals(4, first.parallelism);
        assertTrue(first.supportedModelFormats.contains("onnx"));
        assertTrue(first.supportedModelFormats.contains("numpy_npz_v1"));
        assertFalse(first.toJson().length == 0);
    }

    @Test
    void readinessRequiresAnExactProfileAndRuntimeContract() {
        BehaviorJobConfig valid = new BehaviorJobConfig.Builder()
                .modelUpdateConsumerEnabled(true)
                .modelConsumerDeploymentId("behavior-r1")
                .modelConsumerProfileSha256(PROFILE)
                .modelRuntimeContract("traffic.behavior.inference.v1")
                .modelRuntimeVersion("1.0.0")
                .modelSigningPublicKeyFile("/tmp/model-signing-public.pem")
                .build();
        valid.validateModelUpdateConsumerConfig();

        BehaviorJobConfig invalid = new BehaviorJobConfig.Builder()
                .modelUpdateConsumerEnabled(true)
                .modelConsumerDeploymentId("behavior-r1")
                .modelConsumerProfileSha256("short")
                .build();
        assertThrows(IllegalArgumentException.class, invalid::validateModelUpdateConsumerConfig);
    }
}
