package com.traffic.flink.feature.calculator;

import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureCategory;
import com.traffic.proto.traffic.v1.FeatureSeq;
import com.traffic.proto.traffic.v1.SessionEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;

import java.nio.ByteBuffer;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.List;

/** Projects deterministic packet-length/direction and IAT sequence features. */
public final class FeatureSeqCalculator {
    public static final String SCHEMA_VERSION = "feature-seq/v1";
    public static final String ALGORITHM_VERSION = "signed-sequence-sha256-wavelet-haar/v1";

    private FeatureSeqCalculator() {}

    public static FeatureSeq calculate(SessionEvent session) {
        TrafficFeatureObservation observation = session.hasFeatureObservation()
                ? session.getFeatureObservation() : TrafficFeatureObservation.getDefaultInstance();
        int count = observation.getSignedPacketLengthsCount();
        List<String> missing = new ArrayList<>();
        if (count == 0) missing.add("signed_packet_lengths");
        if (observation.getPacketEventTimeUsCount() != count) missing.add("packet_event_time_us");
        if (count < 2) missing.add("wavelet.minimum_two_samples");
        if (observation.getSequenceTruncated()) missing.add("sequence_truncated");
        if (observation.getMissingFieldsList().contains("sequence_truncated")) {
            missing.add("sequence_truncated");
        }
        if (observation.getRawTrafficRef().isEmpty()) {
            missing.add("seq_blob_ref");
        }
        missing = FeatureLineage.sorted(missing);

        List<Integer> fwd = new ArrayList<>();
        List<Integer> bwd = new ArrayList<>();
        for (int value : observation.getSignedPacketLengthsList()) {
            if (value > 0) fwd.add(value);
            if (value < 0) bwd.add(Math.abs(value));
        }
        if (count > 0 && fwd.size() < 2) missing.add("wavelet_fwd.minimum_two_samples");
        if (count > 0 && bwd.size() < 2) missing.add("wavelet_bwd.minimum_two_samples");
        missing = FeatureLineage.sorted(missing);
        WaveletFeatures forward = wavelet(fwd);
        WaveletFeatures backward = wavelet(bwd);

        FeatureAvailability availability;
        String missingReason;
        if (count == 0) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_MISSING_INPUT;
            missingReason = "packet sequence was not carried by the source session";
        } else if (!missing.isEmpty()) {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE;
            if (observation.getSequenceTruncated()) {
                missingReason = "packet_sequence_truncated";
            } else if (missing.contains("packet_event_time_us")) {
                missingReason = "packet_timestamps_not_available";
            } else if (missing.stream().anyMatch(value -> value.startsWith("wavelet"))) {
                missingReason = "directional_wavelet_not_calculable";
            } else {
                missingReason = "raw_sequence_reference_not_available";
            }
        } else {
            availability = FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE;
            missingReason = "";
        }

        return FeatureSeq.newBuilder()
                .setHeader(FeatureLineage.header(session, "seq", SCHEMA_VERSION))
                .setObjectType("session")
                .setObjectId(session.getSessionId())
                .setCommunityId(session.getCommunityId())
                .setWindowId(session.getSessionId())
                .setTsStart(session.getEventTimeStartMs() > 0
                        ? session.getEventTimeStartMs() : session.getTsStart())
                .setTsEnd(session.getEventTimeEndMs() > 0
                        ? session.getEventTimeEndMs() : session.getTsEnd())
                .setPktlenSeqHash(count == 0 ? "" : hashSignedInts(observation.getSignedPacketLengthsList()))
                .setIatSeqHash(count < 2 ? "" : hashIat(observation.getPacketEventTimeUsList()))
                .setWaveletRelengFwd(forward.relativeEnergy)
                .setWaveletRelengBwd(backward.relativeEnergy)
                .setWaveletEntropyFwd(forward.entropy)
                .setWaveletEntropyBwd(backward.entropy)
                .setWaveletDetailMeanFwd(forward.detailMean)
                .setWaveletDetailMeanBwd(backward.detailMean)
                .setWaveletDetailStdFwd(forward.detailStd)
                .setWaveletDetailStdBwd(backward.detailStd)
                .setSeqBlobRef(observation.getRawTrafficRef())
                .setFeatureCategory(FeatureCategory.FEATURE_CATEGORY_SIDE_CHANNEL)
                .setAvailability(availability)
                .setSchemaVersion(SCHEMA_VERSION)
                .setAlgorithmVersion(ALGORITHM_VERSION)
                .setValueUnit("packet_bytes_signed,time_microseconds,wavelet_ratio")
                .addAllSourceEventIds(FeatureLineage.sourceEventIds(session))
                .addAllEvidenceIds(FeatureLineage.evidenceIds(session))
                .addAllMissingFields(missing)
                .setMissingReason(missingReason)
                .build();
    }

    private static WaveletFeatures wavelet(List<Integer> values) {
        if (values.size() < 2) return new WaveletFeatures();
        List<Double> details = new ArrayList<>();
        double totalEnergy = 0.0d;
        double detailEnergy = 0.0d;
        for (int value : values) totalEnergy += (double) value * value;
        for (int i = 0; i + 1 < values.size(); i += 2) {
            double detail = (values.get(i) - values.get(i + 1)) / Math.sqrt(2.0d);
            details.add(detail);
            detailEnergy += detail * detail;
        }
        if (details.isEmpty()) return new WaveletFeatures();
        double mean = details.stream().mapToDouble(Double::doubleValue).average().orElse(0.0d);
        double variance = details.stream()
                .mapToDouble(value -> (value - mean) * (value - mean))
                .average().orElse(0.0d);
        double entropy = 0.0d;
        if (detailEnergy > 0.0d) {
            for (double detail : details) {
                double probability = detail * detail / detailEnergy;
                if (probability > 0.0d) entropy -= probability * (Math.log(probability) / Math.log(2.0d));
            }
        }
        return new WaveletFeatures(
                totalEnergy == 0.0d ? 0.0f : finiteFloat(detailEnergy / totalEnergy),
                finiteFloat(entropy), finiteFloat(mean), finiteFloat(Math.sqrt(variance)));
    }

    private static String hashSignedInts(List<Integer> values) {
        ByteBuffer buffer = ByteBuffer.allocate(values.size() * Integer.BYTES);
        for (int value : values) buffer.putInt(value);
        return sha256(buffer.array());
    }

    private static String hashIat(List<Long> timestamps) {
        ByteBuffer buffer = ByteBuffer.allocate(Math.max(0, timestamps.size() - 1) * Long.BYTES);
        for (int i = 1; i < timestamps.size(); i++) {
            buffer.putLong(Math.max(0L, timestamps.get(i) - timestamps.get(i - 1)));
        }
        return sha256(buffer.array());
    }

    private static String sha256(byte[] bytes) {
        try {
            byte[] digest = MessageDigest.getInstance("SHA-256").digest(bytes);
            StringBuilder value = new StringBuilder(64);
            for (byte item : digest) value.append(String.format("%02x", item & 0xff));
            return value.toString();
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 is required", e);
        }
    }

    private static float finiteFloat(double value) {
        return Double.isFinite(value) && value <= Float.MAX_VALUE && value >= -Float.MAX_VALUE
                ? (float) value : 0.0f;
    }

    private static final class WaveletFeatures {
        final float relativeEnergy;
        final float entropy;
        final float detailMean;
        final float detailStd;

        WaveletFeatures() { this(0, 0, 0, 0); }
        WaveletFeatures(float relativeEnergy, float entropy, float detailMean, float detailStd) {
            this.relativeEnergy = relativeEnergy;
            this.entropy = entropy;
            this.detailMean = detailMean;
            this.detailStd = detailStd;
        }
    }
}
