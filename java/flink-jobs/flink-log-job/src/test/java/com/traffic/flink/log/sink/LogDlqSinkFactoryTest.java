package com.traffic.flink.log.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class LogDlqSinkFactoryTest {
    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void serializerUsesCanonicalSourceTupleIdentity() throws Exception {
        RawKafkaRecord source = new RawKafkaRecord(
                "device.logs.v1", 3, 99L, 1_700_000_000_000L,
                "tenant-a:192.0.2.8".getBytes(StandardCharsets.UTF_8),
                new byte[]{1, 2, 3}, Map.of("tenant_id", "tenant-a"));
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, "BAD_SCHEMA", "parse_error", "bad protobuf",
                "tenant-a", "log-1", "trace-1", "run-1", "probe-1",
                "flink-log-job", "traffic.v1.DeviceLog", "v1");

        ProducerRecord<byte[], byte[]> record = new LogDlqSinkFactory.Serializer("dlq.v1")
                .serialize(failure, null, 1_700_000_000_100L);

        assertEquals("dlq.v1", record.topic());
        assertEquals("tenant-a:device.logs.v1:3:99",
                new String(record.key(), StandardCharsets.UTF_8));
        JsonNode value = JSON.readTree(record.value());
        assertEquals("flink-log-job", value.get("service_name").asText());
        assertEquals("device.logs.v1:3:99",
                value.get("metadata").get("source_tuple").asText());
    }
}
