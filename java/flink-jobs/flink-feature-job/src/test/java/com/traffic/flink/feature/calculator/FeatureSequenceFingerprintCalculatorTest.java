package com.traffic.flink.feature.calculator;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureAvailability;
import com.traffic.proto.traffic.v1.FeatureFingerprint;
import com.traffic.proto.traffic.v1.FeatureSeq;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;
import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.junit.jupiter.api.Test;

import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FeatureSequenceFingerprintCalculatorTest {

    @Test
    void sameObservationProducesStableGoldenSequenceAndFingerprint() {
        SessionEvent session = session(tlsObservation());

        FeatureSeq firstSequence = FeatureSeqCalculator.calculate(session);
        FeatureSeq replaySequence = FeatureSeqCalculator.calculate(session);
        FeatureFingerprint firstFingerprint = FeatureFingerprintCalculator.calculate(session);
        FeatureFingerprint replayFingerprint = FeatureFingerprintCalculator.calculate(session);

        assertEquals(firstSequence.getHeader().getEventId(), replaySequence.getHeader().getEventId());
        assertEquals(firstSequence.getPktlenSeqHash(), replaySequence.getPktlenSeqHash());
        assertEquals(firstSequence.getIatSeqHash(), replaySequence.getIatSeqHash());
        assertEquals(64, firstSequence.getPktlenSeqHash().length());
        assertEquals(64, firstSequence.getIatSeqHash().length());
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE,
                firstSequence.getAvailability());
        assertTrue(firstSequence.getMissingFieldsList().contains("seq_blob_ref"));

        assertEquals(firstFingerprint.getHeader().getEventId(), replayFingerprint.getHeader().getEventId());
        assertEquals(firstFingerprint.getSniHash(), replayFingerprint.getSniHash());
        assertEquals(64, firstFingerprint.getSniHash().length());
        assertEquals(1, firstFingerprint.getIsEncrypted());
        assertEquals("example.com", firstFingerprint.getSni());
        assertEquals("ja4-golden", firstFingerprint.getJa4());
        assertEquals(16, firstFingerprint.getHexFreqCount());
        assertEquals(16, firstFingerprint.getHexRatioCount());
        assertEquals(4.0f, firstFingerprint.getEntropyPayload(), 0.0001f);
        assertEquals(0.0f, firstFingerprint.getChiSquareBfd(), 0.0001f);
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE,
                firstFingerprint.getAvailability());
        assertEquals("certificate_not_observed", firstFingerprint.getMissingReason());
        assertFalse(firstFingerprint.getMissingFieldsList().contains("sni"));
    }

    @Test
    void absentObservationIsMissingInputNotZeroMeasurement() {
        SessionEvent session = baseSessionBuilder().build();

        FeatureSeq sequence = FeatureSeqCalculator.calculate(session);
        FeatureFingerprint fingerprint = FeatureFingerprintCalculator.calculate(session);

        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_MISSING_INPUT,
                sequence.getAvailability());
        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_MISSING_INPUT,
                fingerprint.getAvailability());
        assertTrue(fingerprint.getMissingFieldsList().contains("feature_observation"));
        assertEquals(0, fingerprint.getIsEncrypted());
    }

    @Test
    void quicIsEncryptedButDoesNotFabricateJa3() {
        TrafficFeatureObservation observation = baseObservationBuilder()
                .setTransportSecurity(TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_QUIC)
                .setQuicVersion("0x00000001")
                .build();

        FeatureFingerprint fingerprint = FeatureFingerprintCalculator.calculate(session(observation));

        assertEquals(1, fingerprint.getIsEncrypted());
        assertEquals("", fingerprint.getJa3());
        assertEquals("0x00000001", fingerprint.getQuicVersion());
        assertTrue(fingerprint.getMissingFieldsList().contains("ja3_not_applicable_to_quic"));
        assertEquals("QUIC version is available; Initial decryption and ClientHello extraction are unsupported",
                fingerprint.getMissingReason());
    }

    @Test
    void onePacketDirectionIsExplicitlyNotCalculable() {
        TrafficFeatureObservation observation = baseObservationBuilder()
                .clearSignedPacketLengths()
                .clearPacketEventTimeUs()
                .addAllSignedPacketLengths(Arrays.asList(100, 120, -60))
                .addAllPacketEventTimeUs(Arrays.asList(1_000_000L, 1_010_000L, 1_020_000L))
                .setRawTrafficRef("pcap://tenant-1/object-1")
                .build();

        FeatureSeq sequence = FeatureSeqCalculator.calculate(session(observation));

        assertEquals(FeatureAvailability.FEATURE_AVAILABILITY_PARTIALLY_AVAILABLE,
                sequence.getAvailability());
        assertTrue(sequence.getMissingFieldsList().contains("wavelet_bwd.minimum_two_samples"));
        assertFalse(sequence.getMissingFieldsList().contains("wavelet_fwd.minimum_two_samples"));
        assertEquals("directional_wavelet_not_calculable", sequence.getMissingReason());
    }

    private static SessionEvent session(TrafficFeatureObservation observation) {
        return baseSessionBuilder().setFeatureObservation(observation).build();
    }

    private static SessionEvent.Builder baseSessionBuilder() {
        return SessionEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId("session-event-1")
                        .setTenantId("tenant-1")
                        .setRunId("run-1")
                        .setProbeId("probe-1")
                        .setFeatureSetId("feature-set-1"))
                .setSessionId("session-1")
                .setCommunityId("1:abc")
                .setTuple(FiveTuple.newBuilder()
                        .setSrcIp("192.0.2.1").setDstIp("198.51.100.1")
                        .setSrcPort(50000).setDstPort(443).setProtocol(6))
                .setTsStart(1_000)
                .setTsEnd(2_000)
                .setEventTimeStartMs(1_000)
                .setEventTimeEndMs(2_000)
                .addSourceEventIds("flow-event-1")
                .addEvidenceIds("flow-1");
    }

    private static TrafficFeatureObservation tlsObservation() {
        return baseObservationBuilder()
                .setTransportSecurity(TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS)
                .setTlsVersion("TLS1.3")
                .setJa3("0123456789abcdef0123456789abcdef")
                .setJa4("ja4-golden")
                .setSni("example.com")
                .addMissingFields("certificate")
                .addMissingFields("raw_traffic_ref")
                .build();
    }

    private static TrafficFeatureObservation.Builder baseObservationBuilder() {
        TrafficFeatureObservation.Builder builder = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("session-feature-merge/v1")
                .addAllSignedPacketLengths(Arrays.asList(100, -80, 120, -60))
                .addAllPacketEventTimeUs(Arrays.asList(1_000_000L, 1_010_000L, 1_025_000L, 1_040_000L))
                .setPayloadObservedBytes(128);
        for (int i = 0; i < 16; i++) builder.addPayloadNibbleCounts(16);
        return builder;
    }
}
