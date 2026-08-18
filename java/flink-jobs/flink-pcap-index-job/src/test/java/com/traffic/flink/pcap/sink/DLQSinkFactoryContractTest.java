package com.traffic.flink.pcap.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapRawKafkaRecord;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

import static org.junit.jupiter.api.Assertions.*;

class DLQSinkFactoryContractTest {
    @Test
    void rejectsPrivateTopicAndWeakAcknowledgements() {
        assertThrows(IllegalArgumentException.class,
                () -> DLQSinkFactory.createDLQSink("broker:9092", "dlq.pcap-index-job", new Properties()));
        Properties weak = new Properties(); weak.setProperty("acks", "1");
        assertThrows(IllegalArgumentException.class,
                () -> DLQSinkFactory.createDLQSink("broker:9092", "dlq.v1", weak));
        assertNotNull(DLQSinkFactory.createDLQSink("broker:9092", "dlq.v1", new Properties()));
    }

    @Test
    void typedSerializerNeverReturnsNullAndUsesSourceCoordinateIdentity() throws Exception {
        PcapRawKafkaRecord raw = PcapRawKafkaRecord.fromConsumerRecord(new ConsumerRecord<>(
                "pcap.index.v1", 1, 9, "tenant-a:probe-a".getBytes(), new byte[]{1, 2, 3}));
        PcapDeadLetter letter = new PcapDeadLetter(raw, "INVALID_PROTOBUF", "bad bytes", "tenant-a", "probe-a");
        DLQSinkFactory.PcapDeadLetterSerializer serializer =
                new DLQSinkFactory.PcapDeadLetterSerializer("dlq.v1");
        ProducerRecord<byte[], byte[]> produced = serializer.serialize(letter, null, 100L);
        assertNotNull(produced);
        assertEquals("dlq.v1", produced.topic());
        assertEquals(letter.projectionIdentity(), new String(produced.key(), StandardCharsets.UTF_8));
        JsonNode json = new ObjectMapper().readTree(produced.value());
        assertEquals(9, json.get("original_offset").asLong());
        assertEquals("INVALID_PROTOBUF", json.get("error_code").asText());
        assertThrows(IllegalArgumentException.class, () -> serializer.serialize(null, null, null));
    }
}
