package com.traffic.flink.feature.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.sql.SQLException;
import java.sql.Statement;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CheckpointAwareClickHouseSinkTest {

    @Test
    void failedBatchRemainsPendingForCheckpointReplay() throws Exception {
        CheckpointAwareClickHouseSink<FeatureStat> sink = sinkThatFails();
        sink.invoke(feature("event-1"), null);

        assertThrows(SQLException.class, sink::flushBuffer);
        assertEquals(1, sink.pendingCount());
    }

    @Test
    void fullExternalAcknowledgementClearsPendingBatch() throws Exception {
        CheckpointAwareClickHouseSink<FeatureStat> sink = new TestSink(false);
        sink.invoke(feature("event-1"), null);
        sink.invoke(feature("event-2"), null);

        assertEquals(2, sink.pendingCount());
        sink.flushBuffer();
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void partialAndFailedReceiptsFailClosed() {
        assertThrows(SQLException.class,
                () -> CheckpointAwareClickHouseSink.validateBatchReceipt(2, new int[]{1}));
        assertThrows(SQLException.class,
                () -> CheckpointAwareClickHouseSink.validateBatchReceipt(
                        2, new int[]{1, Statement.EXECUTE_FAILED}));
    }

    @Test
    void deduplicationTokenIsStableForSameOrderedBatch() {
        CheckpointAwareClickHouseSink<FeatureStat> sink = newSink();
        List<FeatureStat> original = List.of(feature("event-1"), feature("event-2"));
        List<FeatureStat> replay = List.of(feature("event-1"), feature("event-2"));
        List<FeatureStat> reordered = List.of(feature("event-2"), feature("event-1"));

        assertEquals(sink.deduplicationToken(original), sink.deduplicationToken(replay));
        assertNotEquals(sink.deduplicationToken(original), sink.deduplicationToken(reordered));
        assertTrue(CheckpointAwareClickHouseSink.withDeduplicationToken(
                "jdbc:clickhouse://clickhouse:8123/traffic", sink.deduplicationToken(original))
                .contains("insert_deduplication_token=feature-batch-v1-"));
    }

    @Test
    void missingStableEventIdentityFailsBeforeBuffering() {
        CheckpointAwareClickHouseSink<FeatureStat> sink = newSink();
        assertThrows(IllegalArgumentException.class,
                () -> sink.invoke(feature(""), null));
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void shadowActivationSuppressesNewWritesAndDoesNotFlushRestoredPendingRows() throws Exception {
        DeploymentActivation shadow = DeploymentActivation.from(
                ParameterTool.fromMap(Map.of(
                        "deployment.activation.mode", "shadow",
                        "deployment.candidate.sha256", "a".repeat(64))),
                "flink-feature-job", "flink-feature-job-shadow-aaaaaaaaaaaa");
        TestSink sink = new TestSink(false, shadow);
        sink.invoke(feature("new-shadow-event"), null);
        assertEquals(0, sink.pendingCount());

        java.lang.reflect.Field buffer = CheckpointAwareClickHouseSink.class.getDeclaredField("buffer");
        buffer.setAccessible(true);
        @SuppressWarnings("unchecked")
        List<FeatureStat> pending = (List<FeatureStat>) buffer.get(sink);
        pending.add(feature("restored-event"));
        sink.flushBuffer();
        assertEquals(1, sink.pendingCount());
        assertEquals(0, sink.writeCalls);
    }

    private static CheckpointAwareClickHouseSink<FeatureStat> sinkThatFails() {
        return new TestSink(true);
    }

    private static CheckpointAwareClickHouseSink<FeatureStat> newSink() {
        return new CheckpointAwareClickHouseSink<>(
                "jdbc:unused", "user", "password", "feature_stat",
                "INSERT INTO feature_stat (event_id) VALUES (?)",
                (statement, feature) -> statement.setString(1, feature.getHeader().getEventId()),
                feature -> feature.getHeader().getEventId(),
                FeatureStat.class,
                "feature-test-pending",
                100,
                0);
    }

    private static FeatureStat feature(String eventId) {
        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder().setTenantId("tenant-1").setEventId(eventId))
                .setObjectId("session-1")
                .build();
    }

    private static final class TestSink extends CheckpointAwareClickHouseSink<FeatureStat> {
        private final boolean fail;
        private int writeCalls;

        private TestSink(boolean fail) {
            this(fail, DeploymentActivation.legacy("flink-feature-job"));
        }

        private TestSink(boolean fail, DeploymentActivation activation) {
            super(
                    "jdbc:unused", "user", "password", "feature_stat",
                    "INSERT INTO feature_stat (event_id) VALUES (?)",
                    (statement, feature) -> statement.setString(1, feature.getHeader().getEventId()),
                    feature -> feature.getHeader().getEventId(),
                    FeatureStat.class,
                    "feature-test-pending",
                    100,
                    0,
                    activation);
            this.fail = fail;
        }

        @Override
        protected void writeBatch(List<FeatureStat> batch) throws SQLException {
            writeCalls++;
            if (fail) throw new SQLException("forced batch failure");
            // Otherwise simulate a complete external executeBatch acknowledgement.
        }
    }
}
