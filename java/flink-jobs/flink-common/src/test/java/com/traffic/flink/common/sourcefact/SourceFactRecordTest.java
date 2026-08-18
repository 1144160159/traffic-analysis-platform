package com.traffic.flink.common.sourcefact;

import com.traffic.flink.common.RawKafkaRecord;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class SourceFactRecordTest {
    @Test
    void replayKeepsProjectionAndReceiptIdentity() {
        RawKafkaRecord source = new RawKafkaRecord(
                "device.logs.v1", 2, 41L, 1_786_000_000_000L,
                new byte[]{1}, new byte[]{2, 3, 4}, Map.of());
        SourceFactRecord first = SourceFactRecord.fromAccepted(
                "device_log", "tenant-a", "10.0.0.7", "log-7",
                1_786_000_000_000L, 1_786_000_000_001L, "v1",
                source, "flink-log-job", 42L);
        SourceFactRecord replay = SourceFactRecord.fromAccepted(
                "device_log", "tenant-a", "10.0.0.7", "log-7",
                1_786_000_000_000L, 1_786_000_000_001L, "v1",
                source, "flink-log-job", 42L);

        assertEquals(first.getProjectionIdentity(), replay.getProjectionIdentity());
        assertEquals(first.getProjectionHash(), replay.getProjectionHash());
        assertEquals(first.getSourceQualityReceiptId(), replay.getSourceQualityReceiptId());
        assertEquals("AgME", first.getPayloadBase64());
    }

    @Test
    void tenantAndEventIdentityArePartOfTargetId() {
        RawKafkaRecord source = new RawKafkaRecord(
                "user.events.v1", 0, 0L, 1_786_000_000_000L,
                null, new byte[]{9}, Map.of());
        SourceFactRecord first = SourceFactRecord.fromAccepted(
                "user_behavior", "tenant-a", "user-1", "event-1",
                1L, 2L, "v1", source, "flink-user-behavior-job", 1L);
        SourceFactRecord otherTenant = SourceFactRecord.fromAccepted(
                "user_behavior", "tenant-b", "user-1", "event-1",
                1L, 2L, "v1", source, "flink-user-behavior-job", 1L);
        assertNotEquals(first.getProjectionIdentity(), otherTenant.getProjectionIdentity());
    }

    @Test
    void rejectsInventedOrIncompleteCoordinates() {
        RawKafkaRecord invalid = new RawKafkaRecord(
                "flow.events.v1", 0, -1L, 1L, null, new byte[]{1}, Map.of());
        assertThrows(IllegalArgumentException.class, () -> SourceFactRecord.fromAccepted(
                "flow", "tenant-a", "flow-1", "event-1",
                1L, 2L, "v1", invalid, "flink-session-job", 1L));
    }
}
