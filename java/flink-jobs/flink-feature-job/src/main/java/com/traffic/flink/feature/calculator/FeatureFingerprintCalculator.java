package com.traffic.flink.feature.calculator;

import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureCategory;
import com.traffic.proto.traffic.v1.FeatureFingerprint;
import com.traffic.proto.traffic.v1.SessionEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.List;

/** Projects wire-observed TLS/QUIC identity and payload randomness statistics. */
public final class FeatureFingerprintCalculator {
    public static final String SCHEMA_VERSION = "feature-fingerprint/v1";
    public static final String ALGORITHM_VERSION =
            "ja3-md5-ja4-sha256-sni-sha256-nibble-shannon-chi-square/v1";

    private FeatureFingerprintCalculator() {}

    public static FeatureFingerprint calculate(SessionEvent session) {
        TrafficFeatureObservation observation = session.hasFeatureObservation()
                ? session.getFeatureObservation() : TrafficFeatureObservation.getDefaultInstance();
        TransportSecurityProtocol protocol = observation.getTransportSecurity();
        boolean encrypted = protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS
                || protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC;
        List<String> missing = new ArrayList<>();
        for (String field : observation.getMissingFieldsList()) {
            if (field.equals("security_handshake_truncated")
                    || field.equals("security_observation_conflict")
                    || field.equals("payload_bytes")
                    || field.equals("raw_traffic_ref")) {
                missing.add(field);
            }
        }
        if (!session.hasFeatureObservation()) missing.add("feature_observation");
        if (observation.getPayloadObservedBytes() == 0) missing.add("payload_randomness_statistics");
        if (observation.getRawTrafficRef().isEmpty()) missing.add("raw_traffic_ref");
        if (protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS) {
            if (observation.getJa3().isEmpty()) missing.add("ja3");
            if (observation.getJa4().isEmpty()) missing.add("ja4");
            if (observation.getSni().isEmpty()) missing.add("sni");
            if (observation.getCertSha256().isEmpty()) missing.add("certificate");
        } else if (protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC) {
            missing.add("quic_client_hello");
            missing.add("ja3_not_applicable_to_quic");
        } else {
            missing.add("transport_security");
        }
        missing = FeatureLineage.sorted(missing);

        List<Float> frequencies = nibbleFrequencies(observation);
        List<Float> ratios = nibbleRatios(frequencies);
        float entropy = shannonEntropy(frequencies);
        float chiSquare = chiSquare(observation);

        FeatureAvailability availability;
        String missingReason;
        if (!session.hasFeatureObservation()) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_MISSING_INPUT;
            missingReason = "source session did not carry packet feature observations";
        } else if (observation.getMissingFieldsList().contains("security_handshake_truncated")) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_INVALID;
            missingReason = "security handshake was truncated and cannot be fingerprinted completely";
        } else if (protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE;
            missingReason = "QUIC version is available; Initial decryption and ClientHello extraction are unsupported";
        } else if (!missing.isEmpty()) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE;
            missingReason = missingReason(protocol, missing);
        } else {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE;
            missingReason = "";
        }

        String sni = observation.getSni();
        return FeatureFingerprint.newBuilder()
                .setHeader(FeatureLineage.header(session, "fingerprint", SCHEMA_VERSION))
                .setCommunityId(session.getCommunityId())
                .setSessionId(session.getSessionId())
                .setTs(session.getTsEnd())
                .setIsEncrypted(encrypted ? 1 : 0)
                .setTlsVersion(observation.getTlsVersion())
                .setJa3(observation.getJa3())
                .setJa4(observation.getJa4())
                .setSni(sni)
                .setSniHash(sni.isEmpty() ? "" : sha256(sni.getBytes(StandardCharsets.UTF_8)))
                .setCertSha256(observation.getCertSha256())
                .setCertIsSelfSigned(observation.getCertIsSelfSignedKnown()
                        && observation.getCertIsSelfSigned() ? 1 : 0)
                .setPubkeyLen(observation.getPubkeyLenKnown() ? observation.getPubkeyLen() : 0)
                .addAllHexFreq(frequencies)
                .addAllHexRatio(ratios)
                .setEntropyPayload(entropy)
                .setChiSquareBfd(chiSquare)
                .setFeatureCategory(encrypted
                        ? FeatureCategory.FEATURE_CATEGORY_PLAINTEXT_VISIBLE
                        : FeatureCategory.FEATURE_CATEGORY_RANDOMNESS_STATISTICS)
                .setAvailability(availability)
                .setSchemaVersion(SCHEMA_VERSION)
                .setAlgorithmVersion(ALGORITHM_VERSION)
                .setWindowId(session.getSessionId())
                .setEventTimeStartMs(session.getEventTimeStartMs() > 0
                        ? session.getEventTimeStartMs() : session.getTsStart())
                .setEventTimeEndMs(session.getEventTimeEndMs() > 0
                        ? session.getEventTimeEndMs() : session.getTsEnd())
                .setValueUnit("entropy_bits_per_nibble,chi_square_nibble,count_ratio")
                .addAllSourceEventIds(FeatureLineage.sourceEventIds(session))
                .addAllEvidenceIds(FeatureLineage.evidenceIds(session))
                .addAllMissingFields(missing)
                .setMissingReason(missingReason)
                .setQuicVersion(observation.getQuicVersion())
                .setTransportSecurity(protocol)
                .setRawTrafficRef(observation.getRawTrafficRef())
                .build();
    }

    private static String missingReason(TransportSecurityProtocol protocol, List<String> missing) {
        if (protocol == TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS) {
            if (missing.contains("certificate")) return "certificate_not_observed";
            if (missing.contains("sni")) return "no_sni_observed";
            return "TLS handshake was only partially observable";
        }
        if (missing.contains("payload_randomness_statistics")) return "payload_not_observed";
        return "transport_security_not_observed";
    }

    private static List<Float> nibbleFrequencies(TrafficFeatureObservation observation) {
        List<Float> result = new ArrayList<>(16);
        for (int i = 0; i < 16; i++) {
            result.add(i < observation.getPayloadNibbleCountsCount()
                    ? (float) observation.getPayloadNibbleCounts(i) : 0.0f);
        }
        return result;
    }

    private static List<Float> nibbleRatios(List<Float> frequencies) {
        double total = frequencies.stream().mapToDouble(Float::doubleValue).sum();
        List<Float> result = new ArrayList<>(frequencies.size());
        for (float value : frequencies) result.add(total > 0.0d ? (float) (value / total) : 0.0f);
        return result;
    }

    private static float shannonEntropy(List<Float> frequencies) {
        double total = frequencies.stream().mapToDouble(Float::doubleValue).sum();
        if (total <= 0.0d) return 0.0f;
        double entropy = 0.0d;
        for (float count : frequencies) {
            if (count <= 0.0f) continue;
            double p = count / total;
            entropy -= p * (Math.log(p) / Math.log(2.0d));
        }
        return (float) entropy;
    }

    private static float chiSquare(TrafficFeatureObservation observation) {
        if (observation.getPayloadNibbleCountsCount() != 16) return 0.0f;
        double total = 0.0d;
        for (long count : observation.getPayloadNibbleCountsList()) total += count;
        if (total <= 0.0d) return 0.0f;
        double expected = total / 16.0d;
        double value = 0.0d;
        for (long count : observation.getPayloadNibbleCountsList()) {
            double difference = count - expected;
            value += difference * difference / expected;
        }
        return Double.isFinite(value) && value <= Float.MAX_VALUE ? (float) value : 0.0f;
    }

    private static String sha256(byte[] value) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(value);
            StringBuilder result = new StringBuilder(64);
            for (byte item : digest) result.append(String.format("%02x", item & 0xff));
            return result.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 is required", e);
        }
    }
}
