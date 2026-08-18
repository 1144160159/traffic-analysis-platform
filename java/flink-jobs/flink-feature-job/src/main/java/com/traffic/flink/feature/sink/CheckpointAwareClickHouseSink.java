package com.traffic.flink.feature.sink;

import com.traffic.flink.common.DeploymentActivation;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.connector.jdbc.JdbcStatementBuilder;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
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

/**
 * Bounded, checkpoint-aware ClickHouse batch sink.
 *
 * <p>Rows remain buffered until {@link PreparedStatement#executeBatch()} returns
 * a complete non-failed receipt. An exception is propagated to Flink so a
 * checkpoint cannot commit source offsets past an unacknowledged batch. The
 * per-batch ClickHouse deduplication token is derived from the target table and
 * ordered stable event identities; this covers ambiguous retries of the same
 * batch. Replays with a different batch boundary remain visible by event_id and
 * must be reconciled by the projection query.</p>
 */
public class CheckpointAwareClickHouseSink<T> extends RichSinkFunction<T>
        implements CheckpointedFunction {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(CheckpointAwareClickHouseSink.class);
    private static final Pattern TABLE_PATTERN = Pattern.compile("[A-Za-z0-9_.]+");

    @FunctionalInterface
    public interface StableIdentity<T> extends Serializable {
        String get(T record);
    }

    private final String jdbcUrl;
    private final String user;
    private final String password;
    private final String table;
    private final String insertSql;
    private final JdbcStatementBuilder<T> binder;
    private final StableIdentity<T> identity;
    private final Class<T> recordClass;
    private final String stateName;
    private final int batchSize;
    private final int maxRetries;
    private final DeploymentActivation activation;

    private transient List<T> buffer;
    private transient ListState<T> pendingState;
    private transient Counter inputCounter;
    private transient Counter acknowledgedCounter;
    private transient Counter failedCounter;
    private transient Counter retryCounter;
    private transient Counter batchCounter;

    public CheckpointAwareClickHouseSink(
            String jdbcUrl,
            String user,
            String password,
            String table,
            String insertSql,
            JdbcStatementBuilder<T> binder,
            StableIdentity<T> identity,
            Class<T> recordClass,
            String stateName,
            int batchSize,
            int maxRetries) {
        this(jdbcUrl, user, password, table, insertSql, binder, identity, recordClass,
                stateName, batchSize, maxRetries,
                DeploymentActivation.legacy("flink-feature-job"));
    }

    public CheckpointAwareClickHouseSink(
            String jdbcUrl,
            String user,
            String password,
            String table,
            String insertSql,
            JdbcStatementBuilder<T> binder,
            StableIdentity<T> identity,
            Class<T> recordClass,
            String stateName,
            int batchSize,
            int maxRetries,
            DeploymentActivation activation) {
        if (jdbcUrl == null || jdbcUrl.trim().isEmpty()) {
            throw new IllegalArgumentException("ClickHouse JDBC URL must not be blank");
        }
        if (table == null || !TABLE_PATTERN.matcher(table).matches()) {
            throw new IllegalArgumentException("Invalid ClickHouse table name: " + table);
        }
        if (insertSql == null || insertSql.trim().isEmpty() || binder == null
                || identity == null || recordClass == null || stateName == null
                || stateName.trim().isEmpty()) {
            throw new IllegalArgumentException("ClickHouse sink contract is incomplete");
        }
        if (batchSize <= 0 || maxRetries < 0) {
            throw new IllegalArgumentException("Invalid ClickHouse batch/retry configuration");
        }
        this.jdbcUrl = jdbcUrl;
        this.user = user;
        this.password = password;
        this.table = table;
        this.insertSql = insertSql;
        this.binder = binder;
        this.identity = identity;
        this.recordClass = recordClass;
        this.stateName = stateName;
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
                .addGroup("clickhouse_feature_sink")
                .addGroup("table", table);
        inputCounter = metrics.counter("input_total");
        acknowledgedCounter = metrics.counter("acknowledged_total");
        failedCounter = metrics.counter("failed_total");
        retryCounter = metrics.counter("retry_total");
        batchCounter = metrics.counter("acknowledged_batches_total");
        metrics.gauge("pending_records", this::pendingCount);
        LOG.info("Checkpoint-aware ClickHouse sink initialized: url={}, table={}, batchSize={}",
                jdbcUrl, table, batchSize);
    }

    @Override
    public void invoke(T value, Context context) throws Exception {
        if (!activation.externalWritesEnabled()) return;
        if (inputCounter != null) inputCounter.inc();
        validate(value);
        buffer.add(value);
        if (buffer.size() >= batchSize) {
            flushBuffer();
        }
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        // executeBatch must ACK before this operator lets the checkpoint advance.
        if (activation.externalWritesEnabled()) flushBuffer();
        pendingState.clear();
        for (T value : buffer) pendingState.add(value);
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>(stateName, recordClass));
        buffer = new ArrayList<>(initialBufferCapacity());
        if (context.isRestored()) {
            for (T value : pendingState.get()) buffer.add(value);
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
        List<T> batch = new ArrayList<>(buffer);
        try {
            writeWithRetry(batch);
        } catch (Exception error) {
            if (failedCounter != null) failedCounter.inc(batch.size());
            LOG.error("ClickHouse feature batch failed; table={}, retained_records={}",
                    table, batch.size(), error);
            throw error;
        }
        buffer.subList(0, batch.size()).clear();
        if (acknowledgedCounter != null) acknowledgedCounter.inc(batch.size());
        if (batchCounter != null) batchCounter.inc();
    }

    private void writeWithRetry(List<T> batch) throws Exception {
        int maxAttempts = maxRetries + 1;
        Exception lastError = null;
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
                        throw new SQLException("Interrupted while retrying ClickHouse feature batch", interrupted);
                    }
                }
            }
        }
        throw new SQLException("ClickHouse feature insert failed after " + maxAttempts + " attempts", lastError);
    }

    protected void writeBatch(List<T> batch) throws SQLException {
        String token = deduplicationToken(batch);
        try (Connection connection = DriverManager.getConnection(withDeduplicationToken(jdbcUrl, token), user, password);
             PreparedStatement statement = connection.prepareStatement(insertSql)) {
            for (T value : batch) {
                binder.accept(statement, value);
                statement.addBatch();
            }
            validateBatchReceipt(batch.size(), statement.executeBatch());
        }
    }

    static void validateBatchReceipt(int expected, int[] receipt) throws SQLException {
        if (receipt == null || receipt.length != expected) {
            throw new SQLException("ClickHouse returned an incomplete batch receipt: expected="
                    + expected + ", actual=" + (receipt == null ? "null" : receipt.length));
        }
        for (int item : receipt) {
            if (item == Statement.EXECUTE_FAILED) {
                throw new SQLException("ClickHouse reported EXECUTE_FAILED for a feature batch item");
            }
        }
    }

    String deduplicationToken(List<T> batch) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update((table + "\n").getBytes(StandardCharsets.UTF_8));
            for (T value : batch) {
                String recordIdentity = requireIdentity(value);
                digest.update(recordIdentity.getBytes(StandardCharsets.UTF_8));
                digest.update((byte) '\n');
            }
            StringBuilder token = new StringBuilder("feature-batch-v1-");
            for (byte item : digest.digest()) token.append(String.format("%02x", item & 0xff));
            return token.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is required", error);
        }
    }

    static String withDeduplicationToken(String url, String token) {
        String separator = url.contains("?") ? "&" : "?";
        return url + separator + "insert_deduplication_token="
                + URLEncoder.encode(token, StandardCharsets.UTF_8);
    }

    int pendingCount() {
        return buffer == null ? 0 : buffer.size();
    }

    private void validate(T value) {
        if (value == null) throw new IllegalArgumentException("Feature record must not be null");
        requireIdentity(value);
    }

    private String requireIdentity(T value) {
        String recordIdentity = identity.get(value);
        if (recordIdentity == null || recordIdentity.trim().isEmpty()) {
            throw new IllegalArgumentException("Feature event_id must not be blank");
        }
        return recordIdentity;
    }

    private int initialBufferCapacity() {
        return Math.max(1, Math.min(batchSize, 4096));
    }
}
