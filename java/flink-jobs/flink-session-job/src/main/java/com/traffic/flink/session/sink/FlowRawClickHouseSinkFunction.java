package com.traffic.flink.session.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.ActiveIdleStats;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.InterArrivalStats;
import com.traffic.proto.traffic.v1.PacketLengthStats;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
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
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.List;
import java.util.regex.Pattern;

/**
 * Batched ClickHouse sink for raw FlowEvent rows.
 */
public class FlowRawClickHouseSinkFunction extends RichSinkFunction<FlowEvent>
        implements CheckpointedFunction {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FlowRawClickHouseSinkFunction.class);
    private static final Pattern TABLE_PATTERN = Pattern.compile("[A-Za-z0-9_.]+");

    private final String jdbcUrl;
    private final String table;
    private final String user;
    private final String password;
    private final int batchSize;
    private final int maxRetries;
    private final DeploymentActivation activation;

    private transient List<FlowEvent> buffer;
    private transient ListState<FlowEvent> pendingState;

    private transient Counter insertSuccessCounter;
    private transient Counter insertFailCounter;
    private transient Counter insertRetryCounter;
    private transient Counter batchFlushCounter;

    public FlowRawClickHouseSinkFunction(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs,
            int maxRetries) {
        this(jdbcUrl, table, user, password, batchSize, batchIntervalMs, maxRetries,
                DeploymentActivation.legacy("flink-session-job"));
    }

    public FlowRawClickHouseSinkFunction(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs,
            int maxRetries,
            DeploymentActivation activation) {
        this.jdbcUrl = jdbcUrl;
        this.table = table;
        this.user = user;
        this.password = password;
        this.batchSize = batchSize;
        this.maxRetries = maxRetries;
        this.activation = java.util.Objects.requireNonNull(activation, "activation");
        if (jdbcUrl == null || jdbcUrl.trim().isEmpty()) {
            throw new IllegalArgumentException("ClickHouse JDBC URL must not be blank");
        }
        if (table == null || !TABLE_PATTERN.matcher(table).matches()) {
            throw new IllegalArgumentException("Invalid ClickHouse table name: " + table);
        }
        if (batchSize <= 0 || batchIntervalMs <= 0L || maxRetries < 0) {
            throw new IllegalArgumentException("Invalid ClickHouse raw-flow batch/retry configuration");
        }
        this.buffer = new ArrayList<>(initialBufferCapacity());
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        if (this.buffer == null) {
            this.buffer = new ArrayList<>(initialBufferCapacity());
        }

        MetricGroup metricGroup = getRuntimeContext().getMetricGroup()
                .addGroup("clickhouse_flow_raw_sink");
        this.insertSuccessCounter = metricGroup.counter("insert_success_total");
        this.insertFailCounter = metricGroup.counter("insert_fail_total");
        this.insertRetryCounter = metricGroup.counter("insert_retry_total");
        this.batchFlushCounter = metricGroup.counter("batch_flush_total");

        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");
        LOG.info("FlowRawClickHouseSinkFunction initialized: url={}, table={}, batchSize={}",
                jdbcUrl, table, batchSize);
    }

    @Override
    public void invoke(FlowEvent flow, Context context) throws Exception {
        if (!activation.externalWritesEnabled()) return;
        validate(flow);

        buffer.add(flow);
        if (buffer.size() >= batchSize) {
            flushBuffer();
        }
    }

    @Override
    public void close() throws Exception {
        if (activation.externalWritesEnabled()) flushBuffer();
        super.close();
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        // Do not let a checkpoint advance past rows that ClickHouse has not
        // acknowledged. Stable event_id plus a deterministic batch boundary
        // gives ambiguous retries the same ClickHouse deduplication token.
        if (activation.externalWritesEnabled()) flushBuffer();
        pendingState.clear();
        for (FlowEvent flow : buffer) {
            pendingState.add(flow);
        }
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        pendingState = context.getOperatorStateStore().getListState(
                new ListStateDescriptor<>("clickhouse-flow-raw-pending-v1", FlowEvent.class));
        buffer = new ArrayList<>(initialBufferCapacity());
        if (context.isRestored()) {
            for (FlowEvent flow : pendingState.get()) {
                buffer.add(flow);
            }
        }
    }

    void flushBuffer() throws Exception {
        if (!activation.externalWritesEnabled()) {
            return;
        }
        if (buffer == null || buffer.isEmpty()) {
            return;
        }

        List<FlowEvent> batch = new ArrayList<>(buffer);
        try {
            writeWithRetry(batch);
        } catch (Exception e) {
            if (insertFailCounter != null) {
                insertFailCounter.inc(batch.size());
            }
            LOG.error("ClickHouse raw-flow batch failed; retaining {} rows for replay",
                    batch.size(), e);
            throw e;
        }

        // Only remove the acknowledged prefix. No background thread mutates
        // this collection, so checkpoint and invoke share one ordered buffer.
        buffer.subList(0, batch.size()).clear();
        if (insertSuccessCounter != null) {
            insertSuccessCounter.inc(batch.size());
        }
        if (batchFlushCounter != null) {
            batchFlushCounter.inc();
        }
    }

    private void writeWithRetry(List<FlowEvent> batch) throws Exception {
        int attempts = 0;
        Exception lastException = null;

        int allowedAttempts = maxRetries + 1;
        while (attempts < allowedAttempts) {
            try {
                writeBatch(batch);
                return;
            } catch (Exception e) {
                attempts++;
                lastException = e;
                if (insertRetryCounter != null) {
                    insertRetryCounter.inc();
                }
                LOG.warn("ClickHouse raw-flow insert failed (attempt {}/{}): {}",
                        attempts, allowedAttempts, e.getMessage());
                if (attempts < allowedAttempts) {
                    try {
                        Thread.sleep((long) Math.pow(2, attempts) * 100L);
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }
            }
        }

        throw new SQLException(
                "ClickHouse raw-flow insert failed after " + allowedAttempts + " attempts",
                lastException);
    }

    protected void writeBatch(List<FlowEvent> batch) throws SQLException {
        String token = deduplicationToken(batch);
        try (Connection conn = DriverManager.getConnection(
                     withDeduplicationToken(jdbcUrl, token), user, password);
             PreparedStatement ps = conn.prepareStatement(buildInsertSql())) {
            for (FlowEvent flow : batch) {
                setStatementParameters(ps, flow);
                ps.addBatch();
            }
            validateBatchReceipt(batch.size(), ps.executeBatch());
        }
    }

    static void validateBatchReceipt(int expected, int[] receipt) throws SQLException {
        if (receipt == null || receipt.length != expected) {
            throw new SQLException("ClickHouse returned an incomplete raw-flow batch receipt: expected="
                    + expected + ", actual=" + (receipt == null ? "null" : receipt.length));
        }
        for (int item : receipt) {
            if (item == Statement.EXECUTE_FAILED) {
                throw new SQLException("ClickHouse reported EXECUTE_FAILED for a raw-flow batch item");
            }
        }
    }

    String deduplicationToken(List<FlowEvent> batch) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            digest.update((table + "\n").getBytes(StandardCharsets.UTF_8));
            for (FlowEvent flow : batch) {
                digest.update(stableEventId(flow).getBytes(StandardCharsets.UTF_8));
                digest.update((byte) '\n');
            }
            StringBuilder token = new StringBuilder("flow-raw-batch-v1-");
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

    int pendingCount() {
        return buffer == null ? 0 : buffer.size();
    }

    private int initialBufferCapacity() {
        return Math.max(1, Math.min(batchSize, 4096));
    }

    private String buildInsertSql() {
        return String.format(
            "INSERT INTO %s (" +
                "event_id, tenant_id, probe_id, community_id, " +
                "src_ip, dst_ip, src_port, dst_port, protocol, direction, " +
                "ts_start, ts_end, duration_ms, " +
                "packets_fwd, packets_bwd, bytes_fwd, bytes_bwd, pps, bps, " +
                "tcp_flags_fwd, tcp_flags_bwd, tos, " +
                "run_id, feature_set_id, event_ts, ingest_ts, kafka_ts, flink_out_ts, " +
                "pktlen_min, pktlen_max, pktlen_mean, pktlen_std, " +
                "iat_min_ms, iat_max_ms, iat_mean_ms, iat_std_ms, " +
                "active_min_ms, active_max_ms, active_mean_ms, active_std_ms, " +
                "idle_min_ms, idle_max_ms, idle_mean_ms, idle_std_ms, subflow_count" +
            ") VALUES (" +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, ?, ?, " +
                "?, ?, ?, " +
                "?, ?, ?, ?, ?, ?, " +
                "?, ?, ?, " +
                "?, ?, ?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, ?" +
            ")",
            table
        );
    }

    private void setStatementParameters(PreparedStatement ps, FlowEvent flow) throws SQLException {
        int idx = 1;
        EventHeader header = flow.hasHeader()
                ? flow.getHeader()
                : EventHeader.getDefaultInstance();
        FiveTuple tuple = flow.hasTuple()
                ? flow.getTuple()
                : FiveTuple.getDefaultInstance();
        PacketLengthStats pktlen = flow.hasPktlenStats()
                ? flow.getPktlenStats()
                : PacketLengthStats.getDefaultInstance();
        InterArrivalStats iat = flow.hasIatStats()
                ? flow.getIatStats()
                : InterArrivalStats.getDefaultInstance();
        ActiveIdleStats active = flow.hasActiveStats()
                ? flow.getActiveStats()
                : ActiveIdleStats.getDefaultInstance();
        ActiveIdleStats idle = flow.hasIdleStats()
                ? flow.getIdleStats()
                : ActiveIdleStats.getDefaultInstance();
        long flinkOutTs = resolvedFlinkOutTs(flow, header);

        ps.setString(idx++, header.getEventId());
        ps.setString(idx++, header.getTenantId());
        ps.setString(idx++, header.getProbeId());
        ps.setString(idx++, flow.getCommunityId());

        ps.setString(idx++, tuple.getSrcIp());
        ps.setString(idx++, tuple.getDstIp());
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getSrcPort()));
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getDstPort()));
        ps.setInt(idx++, tuple.getProtocol());
        ps.setString(idx++, flow.getDirection());

        ps.setLong(idx++, flow.getTsStart());
        ps.setLong(idx++, flow.getTsEnd());
        ps.setLong(idx++, Integer.toUnsignedLong(flow.getDurationMs()));

        ps.setLong(idx++, Integer.toUnsignedLong(flow.getPacketsFwd()));
        ps.setLong(idx++, Integer.toUnsignedLong(flow.getPacketsBwd()));
        ps.setLong(idx++, flow.getBytesFwd());
        ps.setLong(idx++, flow.getBytesBwd());
        ps.setFloat(idx++, flow.getPps());
        ps.setFloat(idx++, flow.getBps());

        ps.setLong(idx++, Integer.toUnsignedLong(flow.getTcpFlagsFwd()));
        ps.setLong(idx++, Integer.toUnsignedLong(flow.getTcpFlagsBwd()));
        ps.setLong(idx++, Integer.toUnsignedLong(flow.getTos()));

        ps.setString(idx++, header.getRunId());
        ps.setString(idx++, header.getFeatureSetId());
        ps.setLong(idx++, header.getEventTs());
        ps.setLong(idx++, header.getIngestTs());
        ps.setLong(idx++, header.getKafkaTs());
        ps.setLong(idx++, flinkOutTs);

        ps.setLong(idx++, Integer.toUnsignedLong(pktlen.getMin()));
        ps.setLong(idx++, Integer.toUnsignedLong(pktlen.getMax()));
        ps.setFloat(idx++, pktlen.getMean());
        ps.setFloat(idx++, pktlen.getStd());

        ps.setFloat(idx++, iat.getMinMs());
        ps.setFloat(idx++, iat.getMaxMs());
        ps.setFloat(idx++, iat.getMeanMs());
        ps.setFloat(idx++, iat.getStdMs());

        ps.setFloat(idx++, active.getMinMs());
        ps.setFloat(idx++, active.getMaxMs());
        ps.setFloat(idx++, active.getMeanMs());
        ps.setFloat(idx++, active.getStdMs());

        ps.setFloat(idx++, idle.getMinMs());
        ps.setFloat(idx++, idle.getMaxMs());
        ps.setFloat(idx++, idle.getMeanMs());
        ps.setFloat(idx++, idle.getStdMs());
        ps.setLong(idx++, Integer.toUnsignedLong(flow.getSubflowCount()));
    }

    static long resolvedFlinkOutTs(FlowEvent flow, EventHeader header) {
        if (header.getFlinkOutTs() > 0L) return header.getFlinkOutTs();
        if (header.getIngestTs() > 0L) return header.getIngestTs();
        if (header.getEventTs() > 0L) return header.getEventTs();
        return Math.max(0L, flow.getTsEnd());
    }

    private static void validate(FlowEvent flow) {
        if (flow == null) throw new IllegalArgumentException("FlowEvent must not be null");
        stableEventId(flow);
    }

    private static String stableEventId(FlowEvent flow) {
        if (!flow.hasHeader() || flow.getHeader().getEventId().trim().isEmpty()) {
            throw new IllegalArgumentException("FlowEvent event_id must not be blank");
        }
        return flow.getHeader().getEventId();
    }
}
