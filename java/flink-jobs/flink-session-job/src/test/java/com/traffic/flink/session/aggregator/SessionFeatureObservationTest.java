package com.traffic.flink.session.aggregator;

import com.traffic.proto.traffic.v1.TrafficFeatureObservation;
import com.traffic.proto.traffic.v1.TransportSecurityProtocol;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class SessionFeatureObservationTest {

    @Test
    void mergeIsArrivalOrderIndependent() {
        TrafficFeatureObservation early = observation(1_000L, 120, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb");
        TrafficFeatureObservation late = observation(2_000L, -80, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");

        SessionAccumulator first = new SessionAccumulator();
        first.observeFeatureObservation(late);
        first.observeFeatureObservation(early);

        SessionAccumulator second = new SessionAccumulator();
        second.observeFeatureObservation(early);
        second.observeFeatureObservation(late);

        TrafficFeatureObservation left = first.buildFeatureObservation();
        TrafficFeatureObservation right = second.buildFeatureObservation();
        assertEquals(left, right);
        assertEquals(List.of(120, -80), left.getSignedPacketLengthsList());
        assertEquals(List.of(1_000L, 2_000L), left.getPacketEventTimeUsList());
        assertEquals("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", left.getJa3());
        assertTrue(left.getMissingFieldsList().contains("security_observation_conflict"));
    }

    @Test
    void sequenceUsesDeterministicBoundAndMarksTruncation() {
        SessionAccumulator accumulator = new SessionAccumulator();
        for (int i = 300; i >= 1; i--) {
            accumulator.observeFeatureObservation(observation(i, i, ""));
        }

        TrafficFeatureObservation result = accumulator.buildFeatureObservation();
        assertEquals(256, result.getSignedPacketLengthsCount());
        assertEquals(1L, result.getPacketEventTimeUs(0));
        assertEquals(256L, result.getPacketEventTimeUs(255));
        assertTrue(result.getSequenceTruncated());
        assertTrue(result.getMissingFieldsList().contains("sequence_truncated"));
    }

    private static TrafficFeatureObservation observation(long eventTimeUs, int length, String ja3) {
        TrafficFeatureObservation.Builder builder = TrafficFeatureObservation.newBuilder()
                .setSchemaVersion("traffic-feature-observation/v1")
                .setAlgorithmVersion("probe-packet-feature/v1")
                .addSignedPacketLengths(length)
                .addPacketEventTimeUs(eventTimeUs)
                .setTransportSecurity(TransportSecurityProtocol.TRANSPORT_SECURITY_PROTOCOL_TLS)
                .setTlsVersion("TLS1.3")
                .setJa3(ja3)
                .setPayloadObservedBytes(1)
                .addMissingFields("certificate");
        for (int i = 0; i < 16; i++) builder.addPayloadNibbleCounts(i == 0 ? 2 : 0);
        return builder.build();
    }
}
