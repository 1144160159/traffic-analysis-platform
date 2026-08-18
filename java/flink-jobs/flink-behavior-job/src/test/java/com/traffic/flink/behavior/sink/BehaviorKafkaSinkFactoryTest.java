package com.traffic.flink.behavior.sink;

import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class BehaviorKafkaSinkFactoryTest {

    @Test
    void canonicalKeyUsesRealTenantAndCommunityIdentity() {
        DetectionBehavior detection = DetectionBehavior.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-1")
                        .setEventId("event-1"))
                .setCommunityId("1:community")
                .setTs(2_000L)
                .build();

        ProducerRecord<byte[], byte[]> record =
                new BehaviorKafkaSinkFactory.DetectionBehaviorSerializationSchema("detections.v1")
                        .serialize(detection, null, 2_100L);

        assertEquals("tenant-1:1:community", new String(record.key(), StandardCharsets.UTF_8));
        assertEquals("detections.v1", record.topic());
    }

    @Test
    void missingTenantCannotFallBackToDefaultPartitionKey() {
        DetectionBehavior detection = DetectionBehavior.newBuilder()
                .setHeader(EventHeader.newBuilder().setEventId("event-1"))
                .setCommunityId("1:community")
                .build();

        assertThrows(IllegalArgumentException.class,
                () -> new BehaviorKafkaSinkFactory.DetectionBehaviorSerializationSchema("detections.v1")
                        .serialize(detection, null, 2_100L));
    }
}
