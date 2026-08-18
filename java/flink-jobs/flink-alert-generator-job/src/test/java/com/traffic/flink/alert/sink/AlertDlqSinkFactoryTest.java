package com.traffic.flink.alert.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

class AlertDlqSinkFactoryTest {

    @Test
    void rejectsNonCanonicalTopic() {
        assertThrows(IllegalArgumentException.class,
                () -> AlertDlqSinkFactory.create("broker:9092", "dlq.alert-generator"));
    }

    @Test
    void serializerUsesStableSourceCoordinateKeyAndCanonicalJson() throws Exception {
        RawKafkaRecord source = new RawKafkaRecord(
                "detections.behavior.v1", 4, 99L, 2_300L,
                null, new byte[]{1, 2}, Map.of());
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, "BAD_SCHEMA", "parse_error", "bad protobuf",
                "tenant-1", "", "", "", "",
                "flink-alert-generator-job", "traffic.v1.DetectionBehavior", "v1");

        ProducerRecord<byte[], byte[]> record =
                new AlertDlqSinkFactory.Serializer("dlq.v1")
                        .serialize(failure, null, 2_400L);

        assertNotNull(record);
        assertEquals("dlq.v1", record.topic());
        assertEquals("tenant-1:detections.behavior.v1:4:99",
                new String(record.key(), StandardCharsets.UTF_8));
        JsonNode json = new ObjectMapper().readTree(record.value());
        assertEquals("BAD_SCHEMA", json.get("error_code").asText());
    }
}
