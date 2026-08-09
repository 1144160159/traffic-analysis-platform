package com.traffic.flink.behavior.user.sink;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import org.junit.jupiter.api.Test;

import java.sql.SQLException;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ClickHouseAnomalySinkTest {

    @Test
    void failedBatchRemainsBufferedForCheckpointReplay() throws Exception {
        ClickHouseAnomalySink sink = sinkThatFails();
        sink.invoke(anomaly("anomaly-1"), null);

        assertThrows(SQLException.class, sink::flushBuffer);
        assertEquals(1, sink.pendingCount());
    }

    @Test
    void acknowledgedBatchClearsOnlyAfterExternalAck() throws Exception {
        ClickHouseAnomalySink sink = new ClickHouseAnomalySink(
                "jdbc:unused", "user", "password", "traffic.user_anomalies_v2",
                100, Long.MAX_VALUE, 0) {
            @Override
            protected void writeBatch(List<AnomalyEvent> batch) {
                // External batch receipt accepted every row.
            }
        };

        sink.invoke(anomaly("anomaly-1"), null);
        sink.invoke(anomaly("anomaly-2"), null);
        assertEquals(2, sink.pendingCount());
        sink.flushBuffer();
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void unstableBusinessKeyFailsClosed() {
        ClickHouseAnomalySink sink = new ClickHouseAnomalySink(
                "jdbc:unused", "user", "password", "traffic.user_anomalies_v2",
                100, Long.MAX_VALUE, 0);
        AnomalyEvent anomaly = anomaly("");

        assertThrows(IllegalArgumentException.class, () -> sink.invoke(anomaly, null));
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void insertContractContainsStableKeyVersionAndReplayMarker() {
        ClickHouseAnomalySink sink = new ClickHouseAnomalySink(
                "jdbc:unused", "user", "password", "traffic.user_anomalies_v2",
                100, Long.MAX_VALUE, 0);

        String sql = sink.buildInsertSql();
        org.junit.jupiter.api.Assertions.assertTrue(sql.contains("anomaly_id"));
        org.junit.jupiter.api.Assertions.assertTrue(sql.contains("event_version"));
        org.junit.jupiter.api.Assertions.assertTrue(sql.contains("replay_id"));
    }

    private static ClickHouseAnomalySink sinkThatFails() {
        return new ClickHouseAnomalySink(
                "jdbc:unused", "user", "password", "traffic.user_anomalies_v2",
                100, Long.MAX_VALUE, 0) {
            @Override
            protected void writeBatch(List<AnomalyEvent> batch) throws SQLException {
                throw new SQLException("forced failure");
            }
        };
    }

    private static AnomalyEvent anomaly(String anomalyId) {
        AnomalyEvent anomaly = new AnomalyEvent();
        anomaly.anomalyId = anomalyId;
        anomaly.tenantId = "tenant-a";
        anomaly.userId = "user-a";
        anomaly.detectorType = "BRUTE_FORCE";
        anomaly.severity = "high";
        anomaly.detectedAt = 1_700_000_000_000L;
        anomaly.eventVersion = 1L;
        anomaly.replayId = "replay-1";
        return anomaly;
    }
}
