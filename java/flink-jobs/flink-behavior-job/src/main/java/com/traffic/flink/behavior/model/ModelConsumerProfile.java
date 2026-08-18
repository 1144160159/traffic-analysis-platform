package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

/** Exact, replay-stable compatibility profile announced by the shadow consumer. */
public final class ModelConsumerProfile {
    public static final String ENTRY_CLASS = "com.traffic.flink.behavior.ModelUpdateConsumerJob";
    public static final String SUPPORTED_MODEL_FORMATS = "onnx,numpy_npz_v1";

    private ModelConsumerProfile() {}

    public static String calculateSha256(BehaviorJobConfig config) {
        TrustedModelSigningKey signingKey = TrustedModelSigningKey.fromConfig(config);
        String canonical = String.join("\n",
                "schema_version=1",
                "entry_class=" + ENTRY_CLASS,
                "runtime_contract=" + config.getModelRuntimeContract(),
                "runtime_version=" + config.getModelRuntimeVersion(),
                "feature_schema_version=" + config.getModelFeatureSchemaVersion(),
                "graph_schema_version=" + config.getModelGraphSchemaVersion(),
                "supported_model_formats=" + SUPPORTED_MODEL_FORMATS,
                "signing_public_key_sha256=" + signingKey.sha256(),
                "activation_enabled=false",
                "serving_outputs_enabled=false") + "\n";
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256")
                    .digest(canonical.getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(64);
            for (byte item : digest) {
                result.append(String.format("%02x", item));
            }
            return result.toString();
        } catch (Exception error) {
            throw new IllegalStateException("SHA-256 is required by the JVM", error);
        }
    }

    public static void verifyConfiguredProfile(BehaviorJobConfig config) {
        String calculated = calculateSha256(config);
        if (!calculated.equals(config.getModelConsumerProfileSha256())) {
            throw new IllegalArgumentException(
                    "MODEL_CONSUMER_PROFILE_SHA256 does not match the runtime compatibility profile");
        }
    }
}
