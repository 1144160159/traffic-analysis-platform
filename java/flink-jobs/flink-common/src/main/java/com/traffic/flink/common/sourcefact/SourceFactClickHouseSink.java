package com.traffic.flink.common.sourcefact;

import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Pattern;

/** Checkpoint-coupled, replay-safe writer for the four source-fact tables. */
public final class SourceFactClickHouseSink extends RichSinkFunction<SourceFactRecord>
        implements CheckpointedFunction {
    private static final long serialVersionUID = 1L;
    private static final Pattern TABLE = Pattern.compile("[A-Za-z0-9_.]+");

    private final String jdbcUrl;
    private final String table;
    private final String user;
    private final String password;
    private final int batchSize;
    private final int maxRetries;
    private transient List<SourceFactRecord> buffer;
    private transient ListState<SourceFactRecord> pendingState;

    public SourceFactClickHouseSink(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            int maxRetries) {
        this.jdbcUrl = requireText(jdbcUrl, "jdbcUrl");
        this.table = requireText(table, "table");
        if (!TABLE.matcher(this.table).matches()) {
            throw new IllegalArgumentException("invalid ClickHouse table: " + table);
        }
        this.user = user == null ? "" : user;
        this.password = password == null ? "" : password;
        if (batchSize <= 0 || maxRetries < 0) {
            throw new IllegalArgumentException("invalid source-fact batch/retry configuration");
        }
        this.batchSize = batchSize;
        this.maxRetries = maxRetries;
        this.buffer = new ArrayList<>(Math.min(batchSize, 4096));
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        if (buffer == null) buffer = new ArrayList<>(Math.min(batchSize, 4096));
        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");
    }

    @Override
    public void invoke(SourceFactRecord record, Context context) throws Exception {
        if (record == null) throw new IllegalArgumentException("source fact is required");
        buffer.add(record);
        if (buffer.size() >= batchSize) flush();
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        flush();
        pendingState.clear();
        for (SourceFactRecord record : buffer) pendingState.add(record);
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>("source-fact-clickhouse-pending-v1-" + table,
                        SourceFactRecord.class));
        buffer = new ArrayList<>(Math.min(batchSize, 4096));
        if (context.isRestored()) {
            for (SourceFactRecord record : pendingState.get()) buffer.add(record);
        }
    }

    @Override
    public void close() throws Exception {
        flush();
        super.close();
    }

    void flush() throws Exception {
        if (buffer == null || buffer.isEmpty()) return;
        List<SourceFactRecord> batch = new ArrayList<>(buffer);
        writeWithRetry(batch);
        buffer.subList(0, batch.size()).clear();
    }

    private void writeWithRetry(List<SourceFactRecord> batch) throws Exception {
        Exception last = null;
        for (int attempt = 0; attempt <= maxRetries; attempt++) {
            try {
                writeBatch(batch);
                return;
            } catch (Exception error) {
                last = error;
                if (error instanceof ProjectionVersionException || attempt == maxRetries) break;
                try {
                    Thread.sleep(100L << Math.min(attempt, 8));
                } catch (InterruptedException interrupted) {
                    Thread.currentThread().interrupt();
                    throw new SQLException("interrupted while retrying source-fact batch", interrupted);
                }
            }
        }
        throw new SQLException("source-fact insert failed after " + (maxRetries + 1) + " attempts", last);
    }

    private void writeBatch(List<SourceFactRecord> batch) throws Exception {
        List<SourceFactRecord> normalized = normalizeBatch(batch);
        String token = batchToken(normalized);
        try (Connection connection = DriverManager.getConnection(
                withDeduplicationToken(jdbcUrl, token), user, password)) {
            List<SourceFactRecord> pending = removeAlreadyApplied(connection, normalized);
            if (pending.isEmpty()) return;
            try (PreparedStatement statement = connection.prepareStatement(insertSql())) {
                for (SourceFactRecord record : pending) {
                    bind(statement, record);
                    statement.addBatch();
                }
                validateBatchReceipt(pending.size(), statement.executeBatch());
            }
        }
    }

    static List<SourceFactRecord> normalizeBatch(List<SourceFactRecord> batch)
            throws ProjectionVersionException {
        Map<String, SourceFactRecord> unique = new LinkedHashMap<>();
        for (SourceFactRecord record : batch) {
            SourceFactRecord previous = unique.get(record.getProjectionIdentity());
            if (previous == null) {
                unique.put(record.getProjectionIdentity(), record);
                continue;
            }
            if (previous.getSourceVersion() == record.getSourceVersion()
                    && previous.getProjectionHash().equals(record.getProjectionHash())) {
                continue;
            }
            if (record.getSourceVersion() < previous.getSourceVersion()) {
                throw new ProjectionVersionException("stale source-fact version in batch");
            }
            if (record.getSourceVersion() == previous.getSourceVersion()) {
                throw new ProjectionVersionException("same source-fact version has another hash");
            }
            unique.put(record.getProjectionIdentity(), record);
        }
        return new ArrayList<>(unique.values());
    }

    private List<SourceFactRecord> removeAlreadyApplied(
            Connection connection, List<SourceFactRecord> batch) throws SQLException {
        StringBuilder placeholders = new StringBuilder();
        for (int i = 0; i < batch.size(); i++) {
            if (i > 0) placeholders.append(',');
            placeholders.append('?');
        }
        String sql = "SELECT projection_identity, argMax(source_version, source_version), "
                + "argMax(projection_hash, source_version) FROM " + table
                + " WHERE projection_identity IN (" + placeholders + ") GROUP BY projection_identity";
        Map<String, ExistingProjection> existing = new HashMap<>();
        try (PreparedStatement statement = connection.prepareStatement(sql)) {
            int index = 1;
            for (SourceFactRecord record : batch) {
                statement.setString(index++, record.getProjectionIdentity());
            }
            try (ResultSet rows = statement.executeQuery()) {
                while (rows.next()) {
                    existing.put(rows.getString(1),
                            new ExistingProjection(rows.getLong(2), rows.getString(3)));
                }
            }
        }
        List<SourceFactRecord> pending = new ArrayList<>();
        for (SourceFactRecord record : batch) {
            ExistingProjection current = existing.get(record.getProjectionIdentity());
            if (current == null) {
                pending.add(record);
            } else if (current.version > record.getSourceVersion()) {
                throw new ProjectionVersionException("stale source-fact version already projected");
            } else if (current.version == record.getSourceVersion()) {
                if (!current.hash.equals(record.getProjectionHash())) {
                    throw new ProjectionVersionException("same source-fact version already has another hash");
                }
            } else {
                pending.add(record);
            }
        }
        return pending;
    }

    private String insertSql() {
        return "INSERT INTO " + table + " (rail,tenant_id,aggregate_id,event_id,event_time_ms,"
                + "ingest_time_ms,schema_version,source_topic,source_partition,source_offset,"
                + "source_timestamp_ms,source_payload_sha256,source_version,projection_identity,"
                + "source_quality_receipt_id,payload_base64,projection_hash) "
                + "VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)";
    }

    static void bind(PreparedStatement statement, SourceFactRecord record) throws SQLException {
        int index = 1;
        statement.setString(index++, record.getRail());
        statement.setString(index++, record.getTenantId());
        statement.setString(index++, record.getAggregateId());
        statement.setString(index++, record.getEventId());
        statement.setLong(index++, record.getEventTimeMs());
        statement.setLong(index++, record.getIngestTimeMs());
        statement.setString(index++, record.getSchemaVersion());
        statement.setString(index++, record.getSourceTopic());
        statement.setInt(index++, record.getSourcePartition());
        statement.setLong(index++, record.getSourceOffset());
        statement.setLong(index++, record.getSourceTimestampMs());
        statement.setString(index++, record.getSourcePayloadSha256());
        statement.setLong(index++, record.getSourceVersion());
        statement.setString(index++, record.getProjectionIdentity());
        statement.setString(index++, record.getSourceQualityReceiptId());
        statement.setString(index++, record.getPayloadBase64());
        statement.setString(index, record.getProjectionHash());
    }

    static void validateBatchReceipt(int expected, int[] receipt) throws SQLException {
        if (receipt == null || receipt.length != expected) {
            throw new SQLException("incomplete ClickHouse source-fact receipt");
        }
        for (int item : receipt) {
            if (item == Statement.EXECUTE_FAILED) {
                throw new SQLException("ClickHouse source-fact item failed");
            }
        }
    }

    String batchToken(List<SourceFactRecord> batch) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update((table + "\n").getBytes(StandardCharsets.UTF_8));
            for (SourceFactRecord record : batch) {
                digest.update(record.getProjectionHash().getBytes(StandardCharsets.UTF_8));
                digest.update((byte) '\n');
            }
            StringBuilder result = new StringBuilder("source-fact-v1-");
            for (byte item : digest.digest()) result.append(String.format("%02x", item & 0xff));
            return result.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }

    static String withDeduplicationToken(String url, String token) {
        return url + (url.contains("?") ? "&" : "?") + "insert_deduplication_token="
                + URLEncoder.encode(token, StandardCharsets.UTF_8);
    }

    private static String requireText(String value, String field) {
        String normalized = value == null ? "" : value.trim();
        if (normalized.isEmpty()) throw new IllegalArgumentException(field + " is required");
        return normalized;
    }

    private static final class ExistingProjection {
        private final long version;
        private final String hash;
        private ExistingProjection(long version, String hash) {
            this.version = version;
            this.hash = hash == null ? "" : hash;
        }
    }

    public static final class ProjectionVersionException extends SQLException {
        private static final long serialVersionUID = 1L;
        ProjectionVersionException(String message) { super(message); }
    }
}
