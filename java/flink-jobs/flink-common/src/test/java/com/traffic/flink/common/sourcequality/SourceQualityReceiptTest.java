package com.traffic.flink.common.sourcequality;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class SourceQualityReceiptTest {
    private static final ObjectMapper JSON = new ObjectMapper();

    @Test
    void replayIsStableAndUsesExistingAuditEventEnvelope() throws Exception {
        SourceQualityReceipt first = receipt("tenant-a", "accepted", "");
        SourceQualityReceipt replay = receipt("tenant-a", "accepted", "");
        assertEquals(first.getReceiptId(), replay.getReceiptId());
        assertEquals(new String(first.toAuditEventJson()), new String(replay.toAuditEventJson()));

        JsonNode event = JSON.readTree(first.toAuditEventJson());
        assertEquals(first.getReceiptId(), event.get("event_id").asText());
        assertEquals("source_quality_receipt", event.get("resource_type").asText());
        assertEquals("source_quality_receipt", event.get("object_type").asText());
        assertEquals(41L, event.get("detail").get("source").get("offset").asLong());
        assertEquals("accepted", event.get("detail").get("category").asText());
    }

    @Test
    void tenantAndSourceTupleAreBoundIntoIdentity() {
        assertNotEquals(
                receipt("tenant-a", "accepted", "").getReceiptId(),
                receipt("tenant-b", "accepted", "").getReceiptId());
        assertThrows(IllegalArgumentException.class,
                () -> receipt("tenant-a", "conflict", ""));
    }

    private static SourceQualityReceipt receipt(String tenant, String category, String reason) {
        return new SourceQualityReceipt(
                tenant,
                "device_log",
                "flink-log-job-shadow-candidate",
                "device.logs.v1",
                2,
                41L,
                category,
                "log-001",
                SourceQualityReceipt.hashSource("payload".getBytes()),
                1_700_000_000_000L,
                1_700_000_001_000L,
                reason);
    }
}
