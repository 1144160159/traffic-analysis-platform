package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.junit.jupiter.api.Test;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class FlowRawClickHouseSinkFunctionTest {
    @Test
    void failedBatchRemainsBufferedForCheckpointReplay() throws Exception {
        FlowRawClickHouseSinkFunction sink = new FlowRawClickHouseSinkFunction(
                "jdbc:unused", "traffic.flow_raw", "user", "password",
                100, Long.MAX_VALUE, 1) {
            @Override
            protected void writeBatch(List<FlowEvent> batch) throws SQLException {
                throw new SQLException("forced failure");
            }
        };

        sink.invoke(flow("event-1"), null);
        assertThrows(SQLException.class, sink::flushBuffer);
        assertEquals(1, sink.pendingCount());
    }

    @Test
    void acknowledgedBatchClearsBufferedRows() throws Exception {
        FlowRawClickHouseSinkFunction sink = new FlowRawClickHouseSinkFunction(
                "jdbc:unused", "traffic.flow_raw", "user", "password",
                100, Long.MAX_VALUE, 1) {
            @Override
            protected void writeBatch(List<FlowEvent> batch) {
                // External system acknowledged every row.
            }
        };

        sink.invoke(flow("event-1"), null);
        sink.invoke(flow("event-2"), null);
        sink.flushBuffer();
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void incompleteAndFailedReceiptsAreRejected() {
        assertThrows(SQLException.class,
                () -> FlowRawClickHouseSinkFunction.validateBatchReceipt(2, new int[]{1}));
        assertThrows(SQLException.class,
                () -> FlowRawClickHouseSinkFunction.validateBatchReceipt(
                        2, new int[]{1, Statement.EXECUTE_FAILED}));
    }

    @Test
    void orderedBatchTokenAndFallbackTimestampAreDeterministic() {
        FlowRawClickHouseSinkFunction sink = new FlowRawClickHouseSinkFunction(
                "jdbc:unused", "traffic.flows_raw", "user", "password",
                100, Long.MAX_VALUE, 1);
        List<FlowEvent> original = List.of(flow("event-1"), flow("event-2"));
        List<FlowEvent> replay = List.of(flow("event-1"), flow("event-2"));
        List<FlowEvent> reordered = List.of(flow("event-2"), flow("event-1"));
        assertEquals(sink.deduplicationToken(original), sink.deduplicationToken(replay));
        assertNotEquals(sink.deduplicationToken(original), sink.deduplicationToken(reordered));

        FlowEvent value = flow("event-3");
        assertEquals(value.getHeader().getEventTs(),
                FlowRawClickHouseSinkFunction.resolvedFlinkOutTs(value, value.getHeader()));
    }

    private static FlowEvent flow(String eventId) {
        return FlowEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setEventId(eventId)
                        .setTenantId("tenant-a")
                        .setEventTs(1_700_000_000_000L)
                        .build())
                .setTsStart(1_700_000_000_000L)
                .setTsEnd(1_700_000_001_000L)
                .build();
    }
}
