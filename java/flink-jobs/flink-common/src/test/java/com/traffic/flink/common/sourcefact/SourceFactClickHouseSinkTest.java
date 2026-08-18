package com.traffic.flink.common.sourcefact;

import com.traffic.flink.common.RawKafkaRecord;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class SourceFactClickHouseSinkTest {
    @Test
    void sameVersionAndHashIsIdempotent() throws Exception {
        SourceFactRecord record = record(4L, new byte[]{1});
        assertEquals(1, SourceFactClickHouseSink.normalizeBatch(List.of(record, record)).size());
    }

    @Test
    void sameVersionWithAnotherHashFailsClosed() {
        assertThrows(
                SourceFactClickHouseSink.ProjectionVersionException.class,
                () -> SourceFactClickHouseSink.normalizeBatch(List.of(
                        record(4L, new byte[]{1}), record(4L, new byte[]{2}))));
    }

    @Test
    void olderVersionAfterNewerVersionIsRejected() {
        assertThrows(
                SourceFactClickHouseSink.ProjectionVersionException.class,
                () -> SourceFactClickHouseSink.normalizeBatch(List.of(
                        record(5L, new byte[]{1}), record(4L, new byte[]{2}))));
    }

    private static SourceFactRecord record(long version, byte[] payload) {
        return SourceFactRecord.fromAccepted(
                "flow", "tenant-a", "flow-1", "event-1", 1L, 2L, "v1",
                new RawKafkaRecord("flow.events.v1", 0, 3L, 4L, null, payload, Map.of()),
                "flink-session-job", version);
    }
}
