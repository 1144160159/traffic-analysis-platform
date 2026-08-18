package com.traffic.flink.behavior.config;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Iterator;
import java.util.Map;
import java.util.TreeMap;

/** Fail-closed activation gate for optional known-behavior detection. */
public final class BehaviorActivationPolicy {

    private static final String PROFILE_RESOURCE = "known-detection-profile.v1.json";

    public enum Mode {
        OFF,
        KNOWN_FROZEN,
        DYNAMIC
    }

    private final Mode mode;
    private final String profileId;
    private final String profileSha256;

    private BehaviorActivationPolicy(Mode mode, String profileId, String profileSha256) {
        this.mode = mode;
        this.profileId = profileId;
        this.profileSha256 = profileSha256;
    }

    public static BehaviorActivationPolicy validate(BehaviorJobConfig config) {
        if (!"dlq.v1".equals(config.getDlqTopic())) {
            throw new IllegalArgumentException("Behavior job failures must use canonical dlq.v1");
        }
        Mode mode;
        try {
            mode = Mode.valueOf(config.getDetectionMode().trim().toUpperCase().replace('-', '_'));
        } catch (Exception error) {
            throw new IllegalArgumentException(
                    "detection.mode must be one of off, known_frozen, dynamic", error);
        }
        if (mode == Mode.OFF) {
            return new BehaviorActivationPolicy(mode, "", "");
        }
        validateThresholdRange(config);
        if (mode == Mode.DYNAMIC) {
            return new BehaviorActivationPolicy(mode, "dynamic", "");
        }
        if (config.isModelHotUpdateEnabled() || config.getModelReloadIntervalMs() != 0L) {
            throw new IllegalArgumentException(
                    "known_frozen detection forbids model hot update and reload polling");
        }

        try {
            byte[] bytes = readResource();
            String actualSha256 = sha256(bytes);
            if (!actualSha256.equals(config.getKnownProfileSha256())) {
                throw new IllegalArgumentException("known detection profile sha256 mismatch");
            }
            JsonNode profile = new ObjectMapper().readTree(bytes);
            if (profile.path("schema_version").asInt() != 1) {
                throw new IllegalArgumentException("unsupported known detection profile schema");
            }
            String profileId = profile.path("profile_id").asText();
            if (!profileId.equals(config.getKnownProfileId())) {
                throw new IllegalArgumentException("known detection profile_id mismatch");
            }
            if (!profile.path("algorithm_version").asText().equals(config.getModelVersion())) {
                throw new IllegalArgumentException("known detection algorithm_version mismatch");
            }

            Map<String, Float> expected = new TreeMap<>();
            Iterator<Map.Entry<String, JsonNode>> fields = profile.path("models").fields();
            while (fields.hasNext()) {
                Map.Entry<String, JsonNode> field = fields.next();
                expected.put(field.getKey(), (float) field.getValue().asDouble());
            }
            if (!expected.keySet().equals(config.getEnabledModels())) {
                throw new IllegalArgumentException("enabled models differ from frozen profile");
            }
            for (Map.Entry<String, Float> entry : expected.entrySet()) {
                if (Float.compare(entry.getValue(), config.getModelThreshold(entry.getKey())) != 0) {
                    throw new IllegalArgumentException(
                            "threshold differs from frozen profile for model " + entry.getKey());
                }
            }
            return new BehaviorActivationPolicy(mode, profileId, actualSha256);
        } catch (IllegalArgumentException error) {
            throw error;
        } catch (Exception error) {
            throw new IllegalStateException("failed to validate frozen detection profile", error);
        }
    }

    private static void validateThresholdRange(BehaviorJobConfig config) {
        if (config.getEnabledModels().isEmpty()) {
            throw new IllegalArgumentException("enabled model set must not be empty");
        }
        for (String model : config.getEnabledModels()) {
            float threshold = config.getModelThreshold(model);
            if (!Float.isFinite(threshold) || threshold <= 0.0f || threshold > 1.0f) {
                throw new IllegalArgumentException("invalid threshold for model " + model);
            }
        }
    }

    private static byte[] readResource() throws Exception {
        try (InputStream input = BehaviorActivationPolicy.class.getClassLoader()
                .getResourceAsStream(PROFILE_RESOURCE)) {
            if (input == null) throw new IllegalStateException("known detection profile is missing");
            ByteArrayOutputStream output = new ByteArrayOutputStream();
            byte[] buffer = new byte[4096];
            int read;
            while ((read = input.read(buffer)) >= 0) output.write(buffer, 0, read);
            return output.toByteArray();
        }
    }

    private static String sha256(byte[] bytes) throws Exception {
        byte[] digest = MessageDigest.getInstance("SHA-256").digest(bytes);
        StringBuilder value = new StringBuilder(digest.length * 2);
        for (byte item : digest) value.append(String.format("%02x", item));
        return value.toString();
    }

    public boolean shouldRun() { return mode != Mode.OFF; }
    public boolean allowsHotUpdates() { return mode == Mode.DYNAMIC; }
    public Mode getMode() { return mode; }
    public String getProfileId() { return profileId; }
    public String getProfileSha256() { return profileSha256; }
}
