package com.traffic.flink.pcap;

import com.traffic.flink.pcap.process.PcapIndexParseFunction;
import com.traffic.flink.pcap.process.PcapManifestPolicy;
import com.traffic.flink.pcap.process.PcapManifestValidation;
import com.traffic.flink.pcap.process.PcapManifestValidator;
import com.traffic.flink.pcap.process.PcapManifestValidatorTest;
import com.traffic.flink.pcap.source.PcapConsumerConfig;
import com.traffic.flink.pcap.source.PcapConsumerPipeline;
import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.flink.pcap.source.PcapRawKafkaRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.operators.ProcessOperator;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.OneInputStreamOperatorTestHarness;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Optional;
import java.util.Properties;
import java.util.Queue;
import java.util.Set;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.*;

class M02ConsumerDlqMatrixTest {
    @Test
    void M02ConsumerDlqMatrixTest() throws Exception {
        Properties sourceProperties = new Properties();
        sourceProperties.setProperty("acks", "all");
        sourceProperties.setProperty("enable.auto.commit", "false");
        sourceProperties.setProperty("commit.offsets.on.checkpoint", "true");
        PcapConsumerConfig sourceConfig = new PcapConsumerConfig(
                "broker:9092", PcapConsumerConfig.INPUT_TOPIC, "m02-matrix",
                PcapConsumerConfig.DLQ_TOPIC, sourceProperties,
                OffsetsInitializer.committedOffsets(), true, true,
                "N010_N011_REVIEWED_CARRIER_SINK");

        StreamExecutionEnvironment environment = StreamExecutionEnvironment.getExecutionEnvironment();
        List<String> uids = PcapConsumerPipeline.build(environment, sourceConfig).getOperatorUids();
        assertEquals(List.of(PcapConsumerConfig.SOURCE_UID, PcapConsumerConfig.PARSE_UID,
                PcapConsumerConfig.DLQ_UID), uids);
        Set<String> graphUids = environment.getStreamGraph().getStreamNodes().stream()
                .map(node -> node.getTransformationUID())
                .filter(uid -> uid != null)
                .collect(Collectors.toSet());
        assertTrue(graphUids.containsAll(uids));

        Properties autoCommit = new Properties();
        autoCommit.putAll(sourceProperties);
        autoCommit.setProperty("enable.auto.commit", "true");
        assertThrows(IllegalArgumentException.class, () -> new PcapConsumerConfig(
                "broker:9092", PcapConsumerConfig.INPUT_TOPIC, "m02-matrix",
                PcapConsumerConfig.DLQ_TOPIC, autoCommit,
                OffsetsInitializer.committedOffsets(), true, true,
                "N010_N011_REVIEWED_CARRIER_SINK"));

        PcapRawKafkaRecord validRaw = raw(PcapManifestValidatorTest.v2Meta().build(), false);
        PcapIndexedRecord first = new PcapIndexedRecord(
                PcapManifestValidatorTest.v2Meta().build(), validRaw);
        PcapIndexedRecord replay = new PcapIndexedRecord(
                PcapManifestValidatorTest.v2Meta().build(), validRaw);
        assertEquals(first.getProjectionIdentity(), replay.getProjectionIdentity());
        assertEquals(PcapManifestValidation.Disposition.COMPLETE_V2,
                PcapManifestValidator.validate(first, PcapManifestPolicy.strictV2()).getDisposition());

        PcapRawKafkaRecord malformed = rawBytes(new byte[]{0x0a, (byte) 0xff}, false);
        PcapRawKafkaRecord conflicted = raw(PcapManifestValidatorTest.v2Meta().build(), true);
        ProcessOperator<PcapRawKafkaRecord, PcapIndexedRecord> operator =
                new ProcessOperator<>(new PcapIndexParseFunction());
        try (OneInputStreamOperatorTestHarness<PcapRawKafkaRecord, PcapIndexedRecord> harness =
                     new OneInputStreamOperatorTestHarness<>(operator)) {
            harness.open();
            harness.processElement(new StreamRecord<>(malformed, malformed.getTimestamp()));
            harness.processElement(new StreamRecord<>(conflicted, conflicted.getTimestamp()));
            assertTrue(harness.extractOutputValues().isEmpty());
            Queue<StreamRecord<PcapDeadLetter>> deadLetters =
                    harness.getSideOutput(PcapIndexParseFunction.DLQ_TAG);
            assertNotNull(deadLetters);
            assertEquals(2, deadLetters.size());
            assertEquals(List.of("INVALID_PROTOBUF", "IDENTITY_CONFLICT"), deadLetters.stream()
                    .map(record -> record.getValue().getErrorCode()).collect(Collectors.toList()));
            for (StreamRecord<PcapDeadLetter> record : deadLetters) {
                PcapDeadLetter letter = record.getValue();
                assertEquals(PcapConsumerConfig.INPUT_TOPIC, letter.getSource().getTopic());
                assertEquals(7L, letter.getSource().getOffset());
                assertEquals(64, letter.getSource().getRawSha256().length());
                assertTrue(letter.toCanonicalJson().length > 0);
            }
        }
    }

    private static PcapRawKafkaRecord raw(PcapIndexMeta meta, boolean conflictingKey) {
        return rawBytes(meta.toByteArray(), conflictingKey);
    }

    private static PcapRawKafkaRecord rawBytes(byte[] value, boolean conflictingKey) {
        PcapIndexMeta meta = PcapManifestValidatorTest.v2Meta().build();
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", bytes(meta.getTenantId()));
        headers.add("probe_id", bytes(meta.getProbeId()));
        headers.add("file_key", bytes(meta.getFileKey()));
        headers.add("sha256", bytes(meta.getSha256()));
        headers.add("content_type", bytes("application/x-protobuf"));
        headers.add("proto_message_type", bytes("traffic.v1.PcapIndexMeta"));
        headers.add("proto_schema_version", bytes("v1"));
        byte[] key = bytes(conflictingKey ? meta.getTenantId() + ":probe-other"
                : meta.getTenantId() + ":" + meta.getProbeId());
        return PcapRawKafkaRecord.fromConsumerRecord(new ConsumerRecord<>(
                PcapConsumerConfig.INPUT_TOPIC, 0, 7, 2_000L,
                org.apache.kafka.common.record.TimestampType.CREATE_TIME,
                key.length, value.length, key, value, headers, Optional.of(1)));
    }

    private static byte[] bytes(String value) {
        return value.getBytes(StandardCharsets.UTF_8);
    }
}
