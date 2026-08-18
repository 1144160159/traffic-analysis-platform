package com.traffic.flink.pcap.source;

import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Properties;
import java.util.Set;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.*;

class PcapConsumerPipelineTest {
    @Test
    void refusesActivationWithoutCanonicalAclAndReviewedDownstream() {
        assertThrows(IllegalArgumentException.class, () -> config(false, true, "N010_N011_REVIEWED_CARRIER_SINK"));
        assertThrows(IllegalArgumentException.class, () -> config(true, false, "N010_N011_REVIEWED_CARRIER_SINK"));
        assertThrows(IllegalArgumentException.class, () -> config(true, true, "MISSING"));
        Properties weak = new Properties(); weak.setProperty("acks", "1");
        assertThrows(IllegalArgumentException.class, () -> new PcapConsumerConfig(
                "broker:9092", "pcap.index.v1", "flink-pcap-index-job", "dlq.v1", weak,
                OffsetsInitializer.committedOffsets(), true, true, "N010_N011_REVIEWED_CARRIER_SINK"));
    }

    @Test
    void buildsDormantTopologyWithStableUids() {
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        PcapConsumerPipelineResult result = PcapConsumerPipeline.build(
                env, config(true, true, "N010_N011_REVIEWED_CARRIER_SINK"));
        assertEquals(List.of("pcap-raw-kafka-source-v1", "pcap-raw-parse-v1", "pcap-raw-canonical-dlq-v1"),
                result.getOperatorUids());
        assertNotNull(result.getIndexedRecords());
        Set<String> transformationUids = env.getStreamGraph().getStreamNodes().stream()
                .map(node -> node.getTransformationUID())
                .filter(uid -> uid != null)
                .collect(Collectors.toSet());
        assertTrue(transformationUids.containsAll(result.getOperatorUids()));
    }

    private static PcapConsumerConfig config(boolean dlqAcl, boolean idempotentAcl, String capability) {
        return new PcapConsumerConfig("broker:9092", "pcap.index.v1", "flink-pcap-index-job", "dlq.v1",
                new Properties(), OffsetsInitializer.committedOffsets(), dlqAcl, idempotentAcl, capability);
    }
}
