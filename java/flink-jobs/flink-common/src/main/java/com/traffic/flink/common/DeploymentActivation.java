package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;

import java.io.Serializable;
import java.util.Locale;
import java.util.Objects;
import java.util.regex.Pattern;

/**
 * Candidate-bound external-write contract for consumer-first Flink rollout.
 *
 * <p>Legacy keeps existing deployments compatible. A rollout candidate must
 * explicitly choose shadow or production and bind that choice to an immutable
 * source digest. Shadow uses a separate consumer group and never writes an
 * external sink; production uses the canonical group.</p>
 */
public final class DeploymentActivation implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final Pattern SHA256 = Pattern.compile("[0-9a-f]{64}");
    private static final Pattern GROUP = Pattern.compile("[A-Za-z0-9._-]{1,249}");

    public enum Mode {
        LEGACY,
        SHADOW,
        PRODUCTION
    }

    private final Mode mode;
    private final String candidateSha256;
    private final String consumerGroup;
    private final String canonicalConsumerGroup;

    private DeploymentActivation(
            Mode mode,
            String candidateSha256,
            String consumerGroup,
            String canonicalConsumerGroup) {
        this.mode = Objects.requireNonNull(mode, "mode");
        this.candidateSha256 = candidateSha256 == null ? "" : candidateSha256;
        this.consumerGroup = requireGroup(consumerGroup, "consumer group");
        this.canonicalConsumerGroup = requireGroup(canonicalConsumerGroup, "canonical consumer group");
        validate();
    }

    public static DeploymentActivation from(
            ParameterTool params,
            String canonicalConsumerGroup,
            String configuredConsumerGroup) {
        String rawMode = ConfigUtils.get(params, "deployment.activation.mode", "legacy")
                .trim().toUpperCase(Locale.ROOT);
        final Mode mode;
        try {
            mode = Mode.valueOf(rawMode);
        } catch (IllegalArgumentException error) {
            throw new IllegalArgumentException(
                    "deployment.activation.mode must be legacy, shadow or production", error);
        }
        String candidate = ConfigUtils.get(params, "deployment.candidate.sha256", "").trim();
        return new DeploymentActivation(mode, candidate, configuredConsumerGroup, canonicalConsumerGroup);
    }

    public static DeploymentActivation legacy(String consumerGroup) {
        return new DeploymentActivation(Mode.LEGACY, "", consumerGroup, consumerGroup);
    }

    private void validate() {
        if (mode == Mode.LEGACY) {
            if (!candidateSha256.isEmpty()) {
                throw new IllegalArgumentException("legacy activation must not claim a candidate sha256");
            }
            return;
        }
        if (!SHA256.matcher(candidateSha256).matches()) {
            throw new IllegalArgumentException(
                    "candidate rollout requires deployment.candidate.sha256 as lowercase SHA-256");
        }
        if (mode == Mode.PRODUCTION && !consumerGroup.equals(canonicalConsumerGroup)) {
            throw new IllegalArgumentException("production activation must use the canonical consumer group");
        }
        if (mode == Mode.SHADOW && !consumerGroup.equals(expectedShadowConsumerGroup())) {
            throw new IllegalArgumentException(
                    "shadow activation must use candidate-bound consumer group "
                            + expectedShadowConsumerGroup());
        }
    }

    private static String requireGroup(String value, String label) {
        String normalized = value == null ? "" : value.trim();
        if (!GROUP.matcher(normalized).matches()) {
            throw new IllegalArgumentException(label + " is blank or invalid");
        }
        return normalized;
    }

    public Mode getMode() {
        return mode;
    }

    public String getCandidateSha256() {
        return candidateSha256;
    }

    public String getConsumerGroup() {
        return consumerGroup;
    }

    public String getCanonicalConsumerGroup() {
        return canonicalConsumerGroup;
    }

    public boolean externalWritesEnabled() {
        return mode != Mode.SHADOW;
    }

    public boolean isCandidateBound() {
        return mode != Mode.LEGACY;
    }

    public String expectedShadowConsumerGroup() {
        if (candidateSha256.length() < 12) {
            return canonicalConsumerGroup + "-shadow-invalid";
        }
        return canonicalConsumerGroup + "-shadow-" + candidateSha256.substring(0, 12);
    }

    @Override
    public String toString() {
        return "DeploymentActivation{" +
                "mode=" + mode +
                ", candidate=" + (candidateSha256.isEmpty() ? "legacy" : candidateSha256.substring(0, 12)) +
                ", consumerGroup='" + consumerGroup + '\'' +
                '}';
    }
}
