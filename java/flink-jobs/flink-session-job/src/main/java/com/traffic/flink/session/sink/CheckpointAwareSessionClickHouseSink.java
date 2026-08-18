package com.traffic.flink.session.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.SessionEvent;
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

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;
import java.util.regex.Pattern;

/** Checkpoint barrier for batched writes to the distributed sessions table. */
public class CheckpointAwareSessionClickHouseSink extends RichSinkFunction<SessionEvent>
        implements CheckpointedFunction {
    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(CheckpointAwareSessionClickHouseSink.class);
    private static final Pattern TABLE_PATTERN = Pattern.compile("[A-Za-z0-9_.]+");

    private final String jdbcUrl;
    private final String table;
    private final String user;
    private final String password;
    private final int batchSize;
    private final int maxRetries;
    private final DeploymentActivation activation;

    private transient List<SessionEvent> buffer;
    private transient ListState<SessionEvent> pendingState;
    private transient Counter acknowledgedCounter;
    private transient Counter failedCounter;
    private transient Counter retryCounter;

    public CheckpointAwareSessionClickHouseSink(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            int maxRetries) {
        this(jdbcUrl, table, user, password, batchSize, maxRetries,
                DeploymentActivation.legacy("flink-session-job"));
    }

    public CheckpointAwareSessionClickHouseSink(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            int maxRetries,
            DeploymentActivation activation) {
        if (jdbcUrl == null || jdbcUrl.trim().isEmpty()) {
            throw new IllegalArgumentException("ClickHouse JDBC URL must not be blank");
        }
        if (table == null || !TABLE_PATTERN.matcher(table).matches()) {
            throw new IllegalArgumentException("Invalid ClickHouse table name: " + table);
        }
        if (batchSize <= 0 || maxRetries < 0) {
            throw new IllegalArgumentException("Invalid ClickHouse batch/retry configuration");
        }
        this.jdbcUrl = jdbcUrl;
        this.table = table;
        this.user = user;
        this.password = password;
        this.batchSize = batchSize;
        this.maxRetries = maxRetries;
        this.activation = java.util.Objects.requireNonNull(activation, "activation");
        this.buffer = new ArrayList<>(initialBufferCapacity());
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        if (buffer == null) buffer = new ArrayList<>(initialBufferCapacity());
        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");
        MetricGroup metrics = getRuntimeContext().getMetricGroup()
                .addGroup("clickhouse_session_sink")
                .addGroup("table", table);
        acknowledgedCounter = metrics.counter("acknowledged_total");
        failedCounter = metrics.counter("failed_total");
        retryCounter = metrics.counter("retry_total");
        metrics.gauge("pending_records", this::pendingCount);
    }

    @Override
    public void invoke(SessionEvent session, Context context) throws Exception {
        if (!activation.externalWritesEnabled()) return;
        validate(session);
        buffer.add(session);
        if (buffer.size() >= batchSize) flushBuffer();
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        // Failure propagates and prevents source offsets from advancing.
        if (activation.externalWritesEnabled()) flushBuffer();
        pendingState.clear();
        for (SessionEvent session : buffer) pendingState.add(session);
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>(
                        "clickhouse-session-pending-v2", SessionEvent.class));
        buffer = new ArrayList<>(initialBufferCapacity());
        if (context.isRestored()) {
            for (SessionEvent session : pendingState.get()) buffer.add(session);
        }
    }

    @Override
    public void close() throws Exception {
        if (activation.externalWritesEnabled()) flushBuffer();
        super.close();
    }

    void flushBuffer() throws Exception {
        if (!activation.externalWritesEnabled()) return;
        if (buffer == null || buffer.isEmpty()) return;
        List<SessionEvent> batch = new ArrayList<>(buffer);
        try {
            writeWithRetry(batch);
        } catch (Exception error) {
            if (failedCounter != null) failedCounter.inc(batch.size());
            LOG.error("ClickHouse session batch failed; retaining {} records", batch.size(), error);
            throw error;
        }
        buffer.subList(0, batch.size()).clear();
        if (acknowledgedCounter != null) acknowledgedCounter.inc(batch.size());
    }

    private void writeWithRetry(List<SessionEvent> batch) throws Exception {
        Exception lastError = null;
        int maxAttempts = maxRetries + 1;
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                writeBatch(batch);
                return;
            } catch (Exception error) {
                lastError = error;
                if (attempt < maxAttempts) {
                    if (retryCounter != null) retryCounter.inc();
                    try {
                        Thread.sleep(Math.min(2_000L, 100L << Math.min(attempt, 4)));
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw new SQLException("Interrupted while retrying ClickHouse session batch", interrupted);
                    }
                }
            }
        }
        throw new SQLException("ClickHouse session insert failed after " + maxAttempts + " attempts", lastError);
    }

    protected void writeBatch(List<SessionEvent> batch) throws SQLException {
        String token = deduplicationToken(batch);
        try (Connection connection = DriverManager.getConnection(withDeduplicationToken(jdbcUrl, token), user, password);
             PreparedStatement statement = connection.prepareStatement(
                     ClickHouseSinkFactory.buildInsertSql(table))) {
            for (SessionEvent session : batch) {
                ClickHouseSinkFactory.bindStatement(statement, session);
                statement.addBatch();
            }
            validateBatchReceipt(batch.size(), statement.executeBatch());
        }
    }

    static void validateBatchReceipt(int expected, int[] receipt) throws SQLException {
        if (receipt == null || receipt.length != expected) {
            throw new SQLException("ClickHouse returned an incomplete session batch receipt: expected="
                    + expected + ", actual=" + (receipt == null ? "null" : receipt.length));
        }
        for (int item : receipt) {
            if (item == Statement.EXECUTE_FAILED) {
                throw new SQLException("ClickHouse reported EXECUTE_FAILED for a session batch item");
            }
        }
    }

    String deduplicationToken(List<SessionEvent> batch) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update((table + "\n").getBytes(StandardCharsets.UTF_8));
            for (SessionEvent session : batch) {
                String eventId = stableEventId(session);
                digest.update(eventId.getBytes(StandardCharsets.UTF_8));
                digest.update((byte) '\n');
            }
            StringBuilder token = new StringBuilder("session-batch-v1-");
            for (byte item : digest.digest()) token.append(String.format("%02x", item & 0xff));
            return token.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is required", error);
        }
    }

    static String withDeduplicationToken(String url, String token) {
        return url + (url.contains("?") ? "&" : "?") + "insert_deduplication_token="
                + URLEncoder.encode(token, StandardCharsets.UTF_8);
    }

    int pendingCount() { return buffer == null ? 0 : buffer.size(); }

    private static void validate(SessionEvent session) {
        if (session == null) throw new IllegalArgumentException("SessionEvent must not be null");
        if (session.getSessionId().trim().isEmpty()) {
            throw new IllegalArgumentException("session_id must not be blank");
        }
        stableEventId(session);
    }

    private static String stableEventId(SessionEvent session) {
        if (!session.hasHeader() || session.getHeader().getEventId().trim().isEmpty()) {
            throw new IllegalArgumentException("SessionEvent event_id must not be blank");
        }
        return session.getHeader().getEventId();
    }

    private int initialBufferCapacity() {
        return Math.max(1, Math.min(batchSize, 4096));
    }
}
