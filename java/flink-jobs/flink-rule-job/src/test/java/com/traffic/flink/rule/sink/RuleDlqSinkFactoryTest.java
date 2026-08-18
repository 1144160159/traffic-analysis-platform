package com.traffic.flink.rule.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

class RuleDlqSinkFactoryTest {

    @Test
    void onlyCanonicalTopicIsAccepted() {
        assertThrows(IllegalArgumentException.class,
                () -> RuleDlqSinkFactory.create("broker:9092", "dlq.rule-job"));
    }

    @Test
    void serializerKeysByImmutableSourceCoordinate() {
        RawKafkaRecord source = new RawKafkaRecord(
                "feature.stat.v1", 1, 27L, 2_200L, null, new byte[]{1}, Map.of());
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source, "BAD_SCHEMA", "parse_error", "bad protobuf",
                "tenant-1", "", "", "", "",
                "flink-rule-job", "traffic.v1.FeatureStat", "v1");

        ProducerRecord<byte[], byte[]> record = new RuleDlqSinkFactory.Serializer("dlq.v1")
                .serialize(failure, null, 2_300L);

        assertNotNull(record);
        assertEquals("dlq.v1", record.topic());
        assertEquals("tenant-1:feature.stat.v1:1:27",
                new String(record.key(), StandardCharsets.UTF_8));
    }
}
