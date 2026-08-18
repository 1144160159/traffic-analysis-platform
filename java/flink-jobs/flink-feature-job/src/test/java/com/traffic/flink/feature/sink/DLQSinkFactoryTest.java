package com.traffic.flink.feature.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class DLQSinkFactoryTest {

    @Test
    void refusesLegacyFeatureDlqTopic() {
        assertThrows(IllegalArgumentException.class,
                () -> DLQSinkFactory.createDLQSink("localhost:9092", "dlq.feature-job"));
    }

    @Test
    void serializerUsesTenantAndExactSourceTupleAsKey() {
        RawKafkaRecord source = new RawKafkaRecord(
                "session.events.v1", 2, 9, 1000,
                "source-key".getBytes(StandardCharsets.UTF_8), new byte[]{1}, Map.of());
        CanonicalDlqMessage message = CanonicalDlqMessage.failure(
                source, "BAD_SCHEMA", "parse_error", "bad", "tenant-1", "", "", "", "");

        ProducerRecord<byte[], byte[]> record =
                new DLQSinkFactory.DLQKafkaSerializer("dlq.v1").serialize(message, null, 2000L);

        assertEquals("dlq.v1", record.topic());
        assertEquals(2000L, record.timestamp());
        assertArrayEquals("tenant-1:session.events.v1:2:9".getBytes(StandardCharsets.UTF_8), record.key());
    }
}
