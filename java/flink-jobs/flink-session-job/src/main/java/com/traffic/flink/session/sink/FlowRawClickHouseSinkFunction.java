package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.ActiveIdleStats;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.InterArrivalStats;
import com.traffic.proto.traffic.v1.PacketLengthStats;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Batched ClickHouse sink for raw FlowEvent rows.
 */
public class FlowRawClickHouseSinkFunction extends RichSinkFunction<FlowEvent> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FlowRawClickHouseSinkFunction.class);

    private final String jdbcUrl;
    private final String table;
    private final String user;
    private final String password;
    private final int batchSize;
    private final long batchIntervalMs;
    private final int maxRetries;

    private transient List<FlowEvent> buffer;
    private transient ScheduledExecutorService flushScheduler;
    private transient AtomicBoolean flushing;
    private transient volatile boolean closing;

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
        this.jdbcUrl = jdbcUrl;
        this.table = table;
        this.user = user;
        this.password = password;
        this.batchSize = batchSize;
        this.batchIntervalMs = batchIntervalMs;
        this.maxRetries = maxRetries;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        this.buffer = new ArrayList<>(Math.min(batchSize, 4096));
        this.flushing = new AtomicBoolean(false);
        this.closing = false;
        this.flushScheduler = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread thread = new Thread(r, "clickhouse-flow-raw-flusher");
            thread.setDaemon(true);
            return thread;
        });
        this.flushScheduler.scheduleWithFixedDelay(
                this::flushSafely,
                batchIntervalMs,
                batchIntervalMs,
                TimeUnit.MILLISECONDS);

        MetricGroup metricGroup = getRuntimeContext().getMetricGroup()
                .addGroup("clickhouse_flow_raw_sink");
        this.insertSuccessCounter = metricGroup.counter("insert_success_total");
        this.insertFailCounter = metricGroup.counter("insert_fail_total");
        this.insertRetryCounter = metricGroup.counter("insert_retry_total");
        this.batchFlushCounter = metricGroup.counter("batch_flush_total");

        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");
        LOG.info("FlowRawClickHouseSinkFunction initialized: url={}, table={}, batchSize={}, batchIntervalMs={}",
                jdbcUrl, table, batchSize, batchIntervalMs);
    }

    @Override
    public void invoke(FlowEvent flow, Context context) {
        if (flow == null || closing) {
            return;
        }

        boolean shouldFlush;
        synchronized (buffer) {
            buffer.add(flow);
            shouldFlush = buffer.size() >= batchSize;
        }

        if (shouldFlush) {
            flushSafely();
        }
    }

    @Override
    public void close() throws Exception {
        closing = true;
        if (flushScheduler != null) {
            flushScheduler.shutdownNow();
        }
        flushSafely();
        super.close();
    }

    private void flushSafely() {
        if (flushing == null || !flushing.compareAndSet(false, true)) {
            return;
        }

        List<FlowEvent> batch;
        synchronized (buffer) {
            if (buffer.isEmpty()) {
                flushing.set(false);
                return;
            }
            batch = new ArrayList<>(buffer);
            buffer.clear();
        }

        try {
            boolean success = writeWithRetry(batch);
            if (success) {
                if (insertSuccessCounter != null) {
                    insertSuccessCounter.inc(batch.size());
                }
                if (batchFlushCounter != null) {
                    batchFlushCounter.inc();
                }
            } else if (insertFailCounter != null) {
                insertFailCounter.inc(batch.size());
            }
        } finally {
            flushing.set(false);
        }
    }

    private boolean writeWithRetry(List<FlowEvent> batch) {
        int attempts = 0;
        Exception lastException = null;

        while (attempts < maxRetries) {
            try {
                writeBatch(batch);
                return true;
            } catch (Exception e) {
                attempts++;
                lastException = e;
                if (insertRetryCounter != null) {
                    insertRetryCounter.inc();
                }
                LOG.warn("ClickHouse raw-flow insert failed (attempt {}/{}): {}",
                        attempts, maxRetries, e.getMessage());
                if (attempts < maxRetries) {
                    try {
                        Thread.sleep((long) Math.pow(2, attempts) * 100L);
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }
            }
        }

        LOG.error("ClickHouse raw-flow insert failed after {} attempts: {}",
                maxRetries, lastException != null ? lastException.getMessage() : "unknown");
        return false;
    }

    private void writeBatch(List<FlowEvent> batch) throws SQLException {
        try (Connection conn = DriverManager.getConnection(jdbcUrl, user, password);
             PreparedStatement ps = conn.prepareStatement(buildInsertSql())) {
            for (FlowEvent flow : batch) {
                setStatementParameters(ps, flow);
                ps.addBatch();
            }
            ps.executeBatch();
        }
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
        long flinkOutTs = header.getFlinkOutTs() > 0
                ? header.getFlinkOutTs()
                : System.currentTimeMillis();

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
}
