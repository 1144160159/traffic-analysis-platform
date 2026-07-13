package com.traffic.flink.behavior.user.sink;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.common.ConfigUtil;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import java.sql.*;
import java.time.Instant;

/** ClickHouse Sink for user behavior anomalies */
public class ClickHouseAnomalySink extends RichSinkFunction<AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseAnomalySink.class);
    private final String url;
    private final String user;
    private final String password;
    private Connection conn;
    private PreparedStatement stmt;
    private int batchCount;

    public ClickHouseAnomalySink() {
        this(
                System.getenv().getOrDefault("CLICKHOUSE_URL", "jdbc:clickhouse://" +
                        System.getenv().getOrDefault("CLICKHOUSE_HOST", "clickhouse-1.middleware.svc") + ":8123/" +
                        System.getenv().getOrDefault("CLICKHOUSE_DATABASE", "traffic")),
                ConfigUtil.CLICKHOUSE_USER,
                ConfigUtil.CLICKHOUSE_PASSWORD
        );
    }

    public ClickHouseAnomalySink(String url, String user, String password) {
        this.url = url;
        this.user = user;
        this.password = password;
    }

    @Override public void open(Configuration params) {
        try {
            Class.forName("com.clickhouse.jdbc.ClickHouseDriver");
            conn = DriverManager.getConnection(url, user, password);
            conn.createStatement().execute(
                "CREATE TABLE IF NOT EXISTS traffic.user_anomalies (" +
                "anomaly_id String, tenant_id String, user_id String, username String," +
                "detector_type LowCardinality(String), severity LowCardinality(String), score Float32," +
                "description String, detail_json String, source_ip1 String, source_ip2 String," +
                "detected_at DateTime) ENGINE = MergeTree() ORDER BY (tenant_id, detected_at)" +
                "TTL detected_at + INTERVAL 180 DAY");
            stmt = conn.prepareStatement(
                    "INSERT INTO traffic.user_anomalies " +
                    "(anomaly_id, tenant_id, user_id, username, detector_type, severity, score, " +
                    "description, detail_json, source_ip1, source_ip2, detected_at) " +
                    "VALUES (?,?,?,?,?,?,?,?,?,?,?,?)");
            LOG.info("ClickHouse user anomaly sink initialized: {}", url);
        } catch (Exception e) {
            LOG.error("CH init error: {}", e.getMessage(), e);
            throw new RuntimeException("Failed to initialize ClickHouse user anomaly sink", e);
        }
    }

    @Override public void invoke(AnomalyEvent a, Context ctx) {
        try {
            stmt.setString(1, nonNull(a.anomalyId));
            stmt.setString(2, nonNull(a.tenantId));
            stmt.setString(3, nonNull(a.userId));
            stmt.setString(4, nonNull(a.username));
            stmt.setString(5, nonNull(a.detectorType));
            stmt.setString(6, nonNull(a.severity));
            stmt.setFloat(7, a.score);
            stmt.setString(8, nonNull(a.description));
            stmt.setString(9, a.detailJson == null || a.detailJson.isBlank() ? "{}" : a.detailJson);
            stmt.setString(10, nonNull(a.sourceIp1));
            stmt.setString(11, nonNull(a.sourceIp2));
            long detectedAt = a.detectedAt > 0 ? a.detectedAt : System.currentTimeMillis();
            stmt.setTimestamp(12, Timestamp.from(Instant.ofEpochMilli(detectedAt)));
            stmt.addBatch();
            batchCount++;
            flush();
        } catch (SQLException e) {
            LOG.error("CH insert error: {}", e.getMessage(), e);
            throw new RuntimeException("Failed to write user anomaly to ClickHouse", e);
        }
    }

    @Override public void close() {
        try {
            flush();
            if (stmt != null) stmt.close();
            if (conn != null) conn.close();
        } catch (SQLException e) {
            LOG.error("CH close error: {}", e.getMessage());
        }
    }

    private void flush() throws SQLException {
        if (stmt != null && batchCount > 0) {
            stmt.executeBatch();
            batchCount = 0;
        }
    }

    private static String nonNull(String value) {
        return value == null ? "" : value;
    }
}
