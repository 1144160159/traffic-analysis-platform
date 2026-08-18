package com.traffic.flink.log.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;

class LogSourceQualitySinkFactoryTest {
    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void serializerUsesTenantKeyAndReceiptOwnedObservationTime() throws Exception {
        SourceQualityReceipt receipt = new SourceQualityReceipt(
                "tenant-a", "device_log", "flink-log-job-shadow-candidate",
                "device.logs.v1", 2, 41L, "late", "log-1",
                SourceQualityReceipt.hashSource(new byte[]{1, 2, 3}),
                1_700_000_000_000L, 1_700_000_001_000L, "LATE_EVENT");
        ProducerRecord<byte[], byte[]> record =
                new LogSourceQualitySinkFactory.Serializer("audit.logs")
                        .serialize(receipt, null, 9_999L);

        assertEquals("audit.logs", record.topic());
        assertEquals("tenant-a", new String(record.key(), StandardCharsets.UTF_8));
        assertNull(record.timestamp());
        JsonNode value = JSON.readTree(record.value());
        assertEquals("SOURCE_QUALITY_late", value.get("action").asText());
        assertEquals(41L, value.get("detail").get("source").get("offset").asLong());
    }
}
