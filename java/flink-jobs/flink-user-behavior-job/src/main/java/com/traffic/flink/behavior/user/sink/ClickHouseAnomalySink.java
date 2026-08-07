package com.traffic.flink.behavior.user.sink;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.common.ConfigUtil;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.sql.Statement;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.regex.Pattern;

/**
 * Checkpoint-aware ClickHouse sink for user behavior anomalies.
 *
 * <p>The operator keeps every row in state until ClickHouse acknowledges the
 * corresponding JDBC batch. Stable {@code anomaly_id} plus
 * {@code event_version} makes a checkpoint replay idempotent in the
 * ReplacingMergeTree projection.</p>
 */
public class ClickHouseAnomalySink extends RichSinkFunction<AnomalyEvent>
        implements CheckpointedFunction {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseAnomalySink.class);
    private static final Pattern TABLE_PATTERN = Pattern.compile("[A-Za-z0-9_.]+");

    private final String url;
    private final String user;
    private final String password;
    private final String table;
    private final int batchSize;
    private final long batchIntervalMs;
    private final int maxRetries;

    private transient List<AnomalyEvent> buffer;
    private transient ListState<AnomalyEvent> pendingState;
    private transient long lastFlushTime;

    private transient Counter inputCounter;
    private transient Counter acceptedCounter;
    private transient Counter failedCounter;
    private transient Counter sinkSuccessCounter;
    private transient Counter retryCounter;
    private transient Counter batchFlushCounter;

    public ClickHouseAnomalySink() {
        this(
                System.getenv().getOrDefault("CLICKHOUSE_URL", "jdbc:clickhouse://" +
                        System.getenv().getOrDefault("CLICKHOUSE_HOST", "clickhouse-1.middleware.svc") + ":8123/" +
                        System.getenv().getOrDefault("CLICKHOUSE_DATABASE", "traffic")),
                ConfigUtil.CLICKHOUSE_USER,
                ConfigUtil.CLICKHOUSE_PASSWORD,
                System.getenv().getOrDefault("CLICKHOUSE_USER_ANOMALY_TABLE", "traffic.user_anomalies_v2"),
                500,
                2_000L,
                3);
    }

    public ClickHouseAnomalySink(String url, String user, String password) {
        this(url, user, password, "traffic.user_anomalies_v2", 500, 2_000L, 3);
    }

    public ClickHouseAnomalySink(
            String url,
            String user,
            String password,
            String table,
            int batchSize,
            long batchIntervalMs,
            int maxRetries) {
        if (url == null || url.isBlank()) {
            throw new IllegalArgumentException("ClickHouse URL must not be blank");
        }
        if (table == null || !TABLE_PATTERN.matcher(table).matches()) {
            throw new IllegalArgumentException("Invalid ClickHouse table name: " + table);
        }
        if (batchSize <= 0 || batchIntervalMs <= 0 || maxRetries < 0) {
            throw new IllegalArgumentException("Invalid ClickHouse sink batch/retry configuration");
        }
        this.url = url;
        this.user = user;
        this.password = password;
        this.table = table;
        this.batchSize = batchSize;
        this.batchIntervalMs = batchIntervalMs;
        this.maxRetries = maxRetries;
        this.buffer = new ArrayList<>(initialBufferCapacity());
        this.lastFlushTime = System.currentTimeMillis();
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        if (buffer == null) {
            buffer = new ArrayList<>(initialBufferCapacity());
        }
        lastFlushTime = System.currentTimeMillis();
        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");

        MetricGroup metrics = getRuntimeContext().getMetricGroup()
                .addGroup("clickhouse_user_anomaly_sink");
        inputCounter = metrics.counter("input_total");
        acceptedCounter = metrics.counter("accepted_total");
        failedCounter = metrics.counter("failed_total");
        sinkSuccessCounter = metrics.counter("sink_success_total");
        retryCounter = metrics.counter("retry_total");
        batchFlushCounter = metrics.counter("batch_flush_total");
        metrics.gauge("pending_records", this::pendingCount);

        LOG.info("ClickHouse user anomaly sink initialized: url={}, table={}, batchSize={}, intervalMs={}",
                url, table, batchSize, batchIntervalMs);
    }

    @Override
    public void invoke(AnomalyEvent anomaly, Context context) throws Exception {
        if (inputCounter != null) {
            inputCounter.inc();
        }
        validate(anomaly);
        buffer.add(anomaly);
        if (acceptedCounter != null) {
            acceptedCounter.inc();
        }

        long now = System.currentTimeMillis();
        if (buffer.size() >= batchSize || now - lastFlushTime >= batchIntervalMs) {
            flushBuffer();
        }
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        // A checkpoint must not advance past a batch ClickHouse has not ACKed.
        flushBuffer();
        pendingState.clear();
        for (AnomalyEvent anomaly : buffer) {
            pendingState.add(anomaly);
        }
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>("clickhouse-user-anomaly-pending-v2", AnomalyEvent.class));
        buffer = new ArrayList<>(initialBufferCapacity());
        if (context.isRestored()) {
            for (AnomalyEvent anomaly : pendingState.get()) {
                buffer.add(anomaly);
            }
        }
    }

    @Override
    public void close() throws Exception {
        flushBuffer();
        super.close();
    }

    void flushBuffer() throws Exception {
        if (buffer == null || buffer.isEmpty()) {
            return;
        }
        List<AnomalyEvent> batch = new ArrayList<>(buffer);
        try {
            writeWithRetry(batch);
        } catch (Exception error) {
            if (failedCounter != null) {
                failedCounter.inc(batch.size());
            }
            LOG.error("ClickHouse user anomaly batch failed; retaining {} rows for checkpoint replay",
                    batch.size(), error);
            throw error;
        }

        // Clear only the prefix acknowledged by executeBatch.
        buffer.subList(0, batch.size()).clear();
        lastFlushTime = System.currentTimeMillis();
        if (sinkSuccessCounter != null) {
            sinkSuccessCounter.inc(batch.size());
        }
        if (batchFlushCounter != null) {
            batchFlushCounter.inc();
        }
    }

    private void writeWithRetry(List<AnomalyEvent> batch) throws Exception {
        int maxAttempts = maxRetries + 1;
        Exception lastError = null;
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                writeBatch(batch);
                return;
            } catch (Exception error) {
                lastError = error;
                if (attempt < maxAttempts) {
                    if (retryCounter != null) {
                        retryCounter.inc();
                    }
                    try {
                        Thread.sleep(Math.min(2_000L, 100L << Math.min(attempt, 4)));
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw new SQLException("Interrupted while retrying ClickHouse anomaly batch", interrupted);
                    }
                }
            }
        }
        throw new SQLException(
                "ClickHouse user anomaly insert failed after " + maxAttempts + " attempts",
                lastError);
    }

    protected void writeBatch(List<AnomalyEvent> batch) throws SQLException {
        try (Connection connection = DriverManager.getConnection(url, user, password);
             PreparedStatement statement = connection.prepareStatement(buildInsertSql())) {
            for (AnomalyEvent anomaly : batch) {
                bindParameters(statement, anomaly);
                statement.addBatch();
            }
            int[] results = statement.executeBatch();
            if (results == null || results.length != batch.size()) {
                throw new SQLException("ClickHouse returned an incomplete batch receipt: expected=" +
                        batch.size() + ", actual=" + (results == null ? "null" : results.length));
            }
            for (int result : results) {
                if (result == Statement.EXECUTE_FAILED) {
                    throw new SQLException("ClickHouse reported EXECUTE_FAILED for a batch item");
                }
            }
        }
    }

    String buildInsertSql() {
        return "INSERT INTO " + table + " (" +
                "anomaly_id, tenant_id, user_id, username, detector_type, severity, score, " +
                "description, detail_json, source_ip1, source_ip2, detected_at, event_version, replay_id" +
                ") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)";
    }

    static void bindParameters(PreparedStatement statement, AnomalyEvent anomaly) throws SQLException {
        statement.setString(1, anomaly.anomalyId);
        statement.setString(2, anomaly.tenantId);
        statement.setString(3, nonNull(anomaly.userId));
        statement.setString(4, nonNull(anomaly.username));
        statement.setString(5, nonNull(anomaly.detectorType));
        statement.setString(6, nonNull(anomaly.severity));
        statement.setFloat(7, anomaly.score);
        statement.setString(8, nonNull(anomaly.description));
        statement.setString(9, anomaly.detailJson == null || anomaly.detailJson.isBlank()
                ? "{}" : anomaly.detailJson);
        statement.setString(10, nonNull(anomaly.sourceIp1));
        statement.setString(11, nonNull(anomaly.sourceIp2));
        statement.setTimestamp(12, Timestamp.from(Instant.ofEpochMilli(anomaly.detectedAt)));
        statement.setLong(13, anomaly.eventVersion);
        statement.setString(14, nonNull(anomaly.replayId));
    }

    int pendingCount() {
        return buffer == null ? 0 : buffer.size();
    }

    private int initialBufferCapacity() {
        return Math.max(1, Math.min(batchSize, 4096));
    }

    private void validate(AnomalyEvent anomaly) {
        if (anomaly == null || anomaly.anomalyId == null || anomaly.anomalyId.isBlank()) {
            failValidation("anomaly_id must not be blank");
        }
        if (anomaly.tenantId == null || anomaly.tenantId.isBlank()) {
            failValidation("tenant_id must not be blank");
        }
        if (anomaly.detectedAt <= 0) {
            failValidation("detected_at must be stable and positive");
        }
        if (anomaly.eventVersion <= 0) {
            failValidation("event_version must be positive");
        }
    }

    private void failValidation(String message) {
        if (failedCounter != null) {
            failedCounter.inc();
        }
        throw new IllegalArgumentException(message);
    }

    private static String nonNull(String value) {
        return value == null ? "" : value;
    }
}
