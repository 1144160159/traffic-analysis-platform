package com.traffic.flink.session.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.SessionEvent;
import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class CheckpointAwareSessionClickHouseSinkTest {

    @Test
    void failedBatchRemainsPending() throws Exception {
        CheckpointAwareSessionClickHouseSink sink = new TestSink(true);
        sink.invoke(session("event-1"), null);
        assertThrows(SQLException.class, sink::flushBuffer);
        assertEquals(1, sink.pendingCount());
    }

    @Test
    void completeAcknowledgementClearsBatch() throws Exception {
        CheckpointAwareSessionClickHouseSink sink = new TestSink(false);
        sink.invoke(session("event-1"), null);
        sink.invoke(session("event-2"), null);
        sink.flushBuffer();
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void partialReceiptAndFailedItemAreRejected() {
        assertThrows(SQLException.class,
                () -> CheckpointAwareSessionClickHouseSink.validateBatchReceipt(2, new int[]{1}));
        assertThrows(SQLException.class,
                () -> CheckpointAwareSessionClickHouseSink.validateBatchReceipt(
                        2, new int[]{1, Statement.EXECUTE_FAILED}));
    }

    @Test
    void tokenIsStableForSameOrderedBatch() {
        CheckpointAwareSessionClickHouseSink sink = new TestSink(false);
        List<SessionEvent> original = List.of(session("event-1"), session("event-2"));
        List<SessionEvent> replay = List.of(session("event-1"), session("event-2"));
        List<SessionEvent> reordered = List.of(session("event-2"), session("event-1"));
        assertEquals(sink.deduplicationToken(original), sink.deduplicationToken(replay));
        assertNotEquals(sink.deduplicationToken(original), sink.deduplicationToken(reordered));
    }

    @Test
    void shadowActivationSuppressesNewAndRestoredPendingWrites() throws Exception {
        DeploymentActivation shadow = DeploymentActivation.from(
                ParameterTool.fromMap(Map.of(
                        "deployment.activation.mode", "shadow",
                        "deployment.candidate.sha256", "b".repeat(64))),
                "flink-session-job", "flink-session-job-shadow-bbbbbbbbbbbb");
        TestSink sink = new TestSink(false, shadow);
        sink.invoke(session("new-shadow-event"), null);
        assertEquals(0, sink.pendingCount());

        java.lang.reflect.Field buffer =
                CheckpointAwareSessionClickHouseSink.class.getDeclaredField("buffer");
        buffer.setAccessible(true);
        @SuppressWarnings("unchecked")
        List<SessionEvent> pending = (List<SessionEvent>) buffer.get(sink);
        pending.add(session("restored-event"));
        sink.flushBuffer();
        assertEquals(1, sink.pendingCount());
        assertEquals(0, sink.writeCalls);
    }

    private static SessionEvent session(String eventId) {
        return SessionEvent.newBuilder()
                .setHeader(EventHeader.newBuilder().setTenantId("tenant-1").setEventId(eventId))
                .setSessionId("session-1")
                .setCommunityId("1:abc")
                .build();
    }

    private static final class TestSink extends CheckpointAwareSessionClickHouseSink {
        private final boolean fail;
        private int writeCalls;

        private TestSink(boolean fail) {
            this(fail, DeploymentActivation.legacy("flink-session-job"));
        }

        private TestSink(boolean fail, DeploymentActivation activation) {
            super("jdbc:unused", "sessions", "user", "password", 100, 0, activation);
            this.fail = fail;
        }

        @Override
        protected void writeBatch(List<SessionEvent> batch) throws SQLException {
            writeCalls++;
            if (fail) throw new SQLException("forced failure");
        }
    }
}
