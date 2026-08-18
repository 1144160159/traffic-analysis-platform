package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.security.KeyPairGenerator;
import java.util.Base64;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ModelConsumerProfileTest {
    @Test
    void profileBindsRuntimeSchemasFormatsAndTrustRoot() throws Exception {
        byte[] der = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().getPublic().getEncoded();
        String pem = "-----BEGIN PUBLIC KEY-----\n"
                + Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(der)
                + "\n-----END PUBLIC KEY-----\n";
        String inline = Base64.getEncoder().encodeToString(pem.getBytes(StandardCharsets.US_ASCII));
        BehaviorJobConfig provisional = config(inline, "a".repeat(64), 1);
        String profile = ModelConsumerProfile.calculateSha256(provisional);

        assertDoesNotThrow(() -> ModelConsumerProfile.verifyConfiguredProfile(
                config(inline, profile, 1)));
        assertThrows(IllegalArgumentException.class, () ->
                ModelConsumerProfile.verifyConfiguredProfile(config(inline, profile, 2)));
    }

    private static BehaviorJobConfig config(String inline, String profile, int graphSchemaVersion) {
        return new BehaviorJobConfig.Builder()
                .modelUpdateConsumerEnabled(true)
                .modelConsumerDeploymentId("behavior-shadow-r1")
                .modelConsumerProfileSha256(profile)
                .modelRuntimeContract("traffic.behavior.inference.v1")
                .modelRuntimeVersion("1.0.0")
                .modelFeatureSchemaVersion(1)
                .modelGraphSchemaVersion(graphSchemaVersion)
                .modelSigningPublicKeyPemBase64(inline)
                .build();
    }
}
