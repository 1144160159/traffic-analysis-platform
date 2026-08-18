package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DeploymentActivationTest {
    private static final String DIGEST = "a".repeat(64);

    @Test
    void legacyRemainsCompatibleAndWritable() throws Exception {
        DeploymentActivation activation = DeploymentActivation.from(
                ParameterTool.fromMap(Map.of()), "flink-session-job", "custom-legacy-group");
        assertEquals(DeploymentActivation.Mode.LEGACY, activation.getMode());
        assertTrue(activation.externalWritesEnabled());
        assertTrue(new ExternalWriteGate<>(activation).filter("value"));
    }

    @Test
    void shadowRequiresCandidateBoundGroupAndSuppressesWrites() throws Exception {
        DeploymentActivation activation = DeploymentActivation.from(
                ParameterTool.fromMap(Map.of(
                        "deployment.activation.mode", "shadow",
                        "deployment.candidate.sha256", DIGEST)),
                "flink-session-job", "flink-session-job-shadow-aaaaaaaaaaaa");
        assertEquals(DeploymentActivation.Mode.SHADOW, activation.getMode());
        assertFalse(activation.externalWritesEnabled());
        assertFalse(new ExternalWriteGate<>(activation).filter("value"));
    }

    @Test
    void productionRequiresCanonicalGroupAndImmutableCandidate() {
        assertThrows(IllegalArgumentException.class, () -> DeploymentActivation.from(
                ParameterTool.fromMap(Map.of(
                        "deployment.activation.mode", "production",
                        "deployment.candidate.sha256", DIGEST)),
                "flink-feature-job", "flink-feature-job-other"));
        assertThrows(IllegalArgumentException.class, () -> DeploymentActivation.from(
                ParameterTool.fromMap(Map.of(
                        "deployment.activation.mode", "shadow",
                        "deployment.candidate.sha256", "mutable-tag")),
                "flink-feature-job", "flink-feature-job-shadow-mutable"));
    }
}
