package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.streaming.api.functions.async.ResultFuture;
import org.apache.flink.streaming.api.functions.async.RichAsyncFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.RejectedExecutionException;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * ClickHouse 异步写入函数
 * 
 * 核心功能：
 * 1. 异步写入 ClickHouse，不阻塞主流
 * 2. 批量写入优化（内部缓冲）
 * 3. 写入失败时返回原始数据，由外层处理（写 DLQ）
 * 4. 自定义 Prometheus Metrics
 * 
 * 输出：
 * - 成功：返回空集合
 * - 失败：返回包含原始 SessionEvent 的集合（用于 DLQ）
 */
public class ClickHouseAsyncSinkFunction extends RichAsyncFunction<SessionEvent, SessionEvent> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseAsyncSinkFunction.class);

    // ==================== 配置 ====================
    private final String jdbcUrl;
    private final String table;
    private final String user;
    private final String password;
    private final int batchSize;
    private final long batchIntervalMs;
    private final int maxRetries;
    private final int threadPoolSize;

    // ==================== 运行时资源 ====================
    private transient ExecutorService executorService;
    private transient ScheduledExecutorService flushScheduler;
    private transient List<PendingWrite> buffer;
    private transient long lastFlushTime;
    private transient AtomicBoolean flushing;
    private transient volatile boolean closing;

    // ==================== Metrics ====================
    private transient Counter insertSuccessCounter;
    private transient Counter insertFailCounter;
    private transient Counter insertRetryCounter;
    private transient Counter batchFlushCounter;

    public ClickHouseAsyncSinkFunction(
            String jdbcUrl,
            String table,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs,
            int maxRetries,
            int threadPoolSize) {
        this.jdbcUrl = jdbcUrl;
        this.table = table;
        this.user = user;
        this.password = password;
        this.batchSize = batchSize;
        this.batchIntervalMs = batchIntervalMs;
        this.maxRetries = maxRetries;
        this.threadPoolSize = threadPoolSize;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 初始化线程池
        this.executorService = new ThreadPoolExecutor(
                threadPoolSize,
                threadPoolSize * 2,
                60L, TimeUnit.SECONDS,
                new LinkedBlockingQueue<>(1000),
                new ThreadPoolExecutor.CallerRunsPolicy()
        );

        this.buffer = Collections.synchronizedList(new ArrayList<>());
        this.lastFlushTime = System.currentTimeMillis();
        this.flushing = new AtomicBoolean(false);
        this.closing = false;
        this.flushScheduler = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread thread = new Thread(r, "clickhouse-session-flusher");
            thread.setDaemon(true);
            return thread;
        });
        this.flushScheduler.scheduleWithFixedDelay(
                this::triggerFlush,
                batchIntervalMs,
                batchIntervalMs,
                TimeUnit.MILLISECONDS);

        // 初始化 Metrics
        MetricGroup metricGroup = getRuntimeContext().getMetricGroup()
                .addGroup("clickhouse_sink");

        this.insertSuccessCounter = metricGroup.counter("insert_success_total");
        this.insertFailCounter = metricGroup.counter("insert_fail_total");
        this.insertRetryCounter = metricGroup.counter("insert_retry_total");
        this.batchFlushCounter = metricGroup.counter("batch_flush_total");

        Class.forName("com.clickhouse.jdbc.ClickHouseDriver");

        LOG.info("ClickHouseAsyncSinkFunction initialized: url={}, table={}, batchSize={}, batchIntervalMs={}",
                jdbcUrl, table, batchSize, batchIntervalMs);
    }

    @Override
    public void close() throws Exception {
        closing = true;
        if (flushScheduler != null) {
            flushScheduler.shutdownNow();
        }
        waitForCurrentFlush();
        flushRemainingSynchronously();

        if (executorService != null) {
            executorService.shutdown();
            try {
                if (!executorService.awaitTermination(30, TimeUnit.SECONDS)) {
                    executorService.shutdownNow();
                }
            } catch (InterruptedException e) {
                executorService.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }
        super.close();
    }

    @Override
    public void asyncInvoke(SessionEvent session, ResultFuture<SessionEvent> resultFuture) throws Exception {
        if (closing) {
            resultFuture.complete(Collections.singletonList(session));
            return;
        }

        PendingWrite pendingWrite = new PendingWrite(session, resultFuture);
        boolean shouldFlush;
        long now = System.currentTimeMillis();

        synchronized (buffer) {
            buffer.add(pendingWrite);
            shouldFlush = buffer.size() >= batchSize || (now - lastFlushTime) >= batchIntervalMs;
        }

        if (shouldFlush) {
            triggerFlush();
        }
    }

    @Override
    public void timeout(SessionEvent session, ResultFuture<SessionEvent> resultFuture) throws Exception {
        LOG.warn("ClickHouse write timeout for session: {}", session.getSessionId());
        if (insertFailCounter != null) {
            insertFailCounter.inc();
        }
        // 超时也返回原始数据，由外层写入 DLQ
        resultFuture.complete(Collections.singletonList(session));
    }

    /**
     * 带重试的写入
     */
    private boolean writeWithRetry(List<PendingWrite> batch) {
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

                LOG.warn("ClickHouse insert failed (attempt {}/{}): {}",
                        attempts, maxRetries, e.getMessage());

                if (attempts < maxRetries) {
                    try {
                        // 指数退避
                        Thread.sleep((long) Math.pow(2, attempts) * 100);
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }
            }
        }

        LOG.error("ClickHouse insert failed after {} attempts: {}",
                maxRetries, lastException != null ? lastException.getMessage() : "unknown");
        return false;
    }

    /**
     * 写入一批记录
     */
    private void writeBatch(List<PendingWrite> batch) throws SQLException {
        String insertSql = buildInsertSql();

        try (Connection conn = DriverManager.getConnection(jdbcUrl, user, password);
             PreparedStatement ps = conn.prepareStatement(insertSql)) {

            for (PendingWrite pendingWrite : batch) {
                setStatementParameters(ps, pendingWrite.session);
                ps.addBatch();
            }

            ps.executeBatch();

        } catch (SQLException e) {
            throw e;
        }
    }

    private void triggerFlush() {
        if (closing || !flushing.compareAndSet(false, true)) {
            return;
        }

        List<PendingWrite> batch = drainBuffer();
        if (batch.isEmpty()) {
            flushing.set(false);
            return;
        }

        try {
            CompletableFuture.runAsync(() -> flushBatch(batch), executorService);
        } catch (RejectedExecutionException e) {
            flushing.set(false);
            LOG.error("ClickHouse flush task rejected: {}", e.getMessage(), e);
            completeBatch(batch, false);
        }
    }

    private void flushBatch(List<PendingWrite> batch) {
        try {
            boolean success = writeWithRetry(batch);
            completeBatch(batch, success);
        } catch (Exception e) {
            LOG.error("Unexpected error flushing ClickHouse batch: {}", e.getMessage(), e);
            completeBatch(batch, false);
        } finally {
            flushing.set(false);
            if (!closing && shouldFlushPending()) {
                triggerFlush();
            }
        }
    }

    private List<PendingWrite> drainBuffer() {
        synchronized (buffer) {
            if (buffer.isEmpty()) {
                return Collections.emptyList();
            }

            List<PendingWrite> batch = new ArrayList<>(buffer);
            buffer.clear();
            lastFlushTime = System.currentTimeMillis();
            return batch;
        }
    }

    private boolean shouldFlushPending() {
        long now = System.currentTimeMillis();
        synchronized (buffer) {
            return !buffer.isEmpty()
                    && (buffer.size() >= batchSize || (now - lastFlushTime) >= batchIntervalMs);
        }
    }

    private void flushRemainingSynchronously() {
        List<PendingWrite> batch = drainBuffer();
        if (batch.isEmpty()) {
            return;
        }

        try {
            completeBatch(batch, writeWithRetry(batch));
        } catch (Exception e) {
            LOG.error("Failed to flush remaining ClickHouse batch during close: {}", e.getMessage(), e);
            completeBatch(batch, false);
        }
    }

    private void waitForCurrentFlush() {
        long deadline = System.currentTimeMillis() + TimeUnit.SECONDS.toMillis(30);
        while (flushing != null && flushing.get() && System.currentTimeMillis() < deadline) {
            try {
                Thread.sleep(50L);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
        }
    }

    private void completeBatch(List<PendingWrite> batch, boolean success) {
        if (success) {
            if (insertSuccessCounter != null) {
                insertSuccessCounter.inc(batch.size());
            }
            if (batchFlushCounter != null) {
                batchFlushCounter.inc();
            }

            for (PendingWrite pendingWrite : batch) {
                pendingWrite.completeSuccess();
            }
            return;
        }

        if (insertFailCounter != null) {
            insertFailCounter.inc(batch.size());
        }

        for (PendingWrite pendingWrite : batch) {
            pendingWrite.completeFailure();
        }
    }

    /**
     * 构建 INSERT SQL
     */
    private String buildInsertSql() {
        return String.format(
            "INSERT INTO %s (" +
                "session_id, tenant_id, community_id, " +
                "ts_start, ts_end, duration_ms, " +
                "packets_fwd, packets_bwd, bytes_fwd, bytes_bwd, " +
                "event_id, run_id, feature_set_id, event_ts, ingest_ts, kafka_ts, flink_out_ts, probe_id, " +
                "src_ip, dst_ip, src_port, dst_port, protocol, " +
                "bytes_total, up_down_ratio, num_pkts, avg_payload, min_payload, max_payload, std_payload, " +
                "mean_iat_ms, min_iat_ms, max_iat_ms, std_iat_ms, " +
                "flags_syn, flags_ack, flags_fin, flags_psh, flags_rst, " +
                "dns_pkt_cnt, tcp_pkt_cnt, udp_pkt_cnt, icmp_pkt_cnt, " +
                "has_syn, has_fin, has_rst, is_established, " +
                "evidence_count, flow_ids, end_reason" +
            ") VALUES (" +
                "?, ?, ?, " +
                "?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, ?, ?, ?, ?, " +
                "?, ?, ?, ?, ?, ?, ?, " +
                "?, ?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?, ?, " +
                "?, ?, ?" +
            ")",
            table
        );
    }

    /**
     * 设置 PreparedStatement 参数
     */
    private void setStatementParameters(PreparedStatement ps, SessionEvent session) throws SQLException {
        int idx = 1;
        EventHeader header = session.hasHeader()
                ? session.getHeader()
                : EventHeader.getDefaultInstance();
        FiveTuple tuple = session.hasTuple()
                ? session.getTuple()
                : FiveTuple.getDefaultInstance();

        // 租户与标识
        ps.setString(idx++, session.getSessionId());
        ps.setString(idx++, header.getTenantId());
        ps.setString(idx++, session.getCommunityId());

        // 时间范围
        ps.setLong(idx++, session.getTsStart());
        ps.setLong(idx++, session.getTsEnd());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getDurationMs()));

        // SessionEvent proto has packets_total only; keep total in packets_fwd and num_pkts.
        ps.setLong(idx++, session.getPacketsTotal());
        ps.setLong(idx++, 0L);
        ps.setLong(idx++, session.getBytesFwd());
        ps.setLong(idx++, session.getBytesBwd());

        ps.setString(idx++, header.getEventId());
        ps.setString(idx++, header.getRunId());
        ps.setString(idx++, header.getFeatureSetId());
        ps.setLong(idx++, header.getEventTs());
        ps.setLong(idx++, header.getIngestTs());
        ps.setLong(idx++, header.getKafkaTs());
        ps.setLong(idx++, header.getFlinkOutTs() > 0 ? header.getFlinkOutTs() : System.currentTimeMillis());
        ps.setString(idx++, header.getProbeId());

        ps.setString(idx++, tuple.getSrcIp());
        ps.setString(idx++, tuple.getDstIp());
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getSrcPort()));
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getDstPort()));
        ps.setInt(idx++, session.getProtocol());

        // 流量统计
        ps.setLong(idx++, session.getBytesTotal());
        ps.setFloat(idx++, session.getUpDownRatio());

        // 包长统计
        ps.setLong(idx++, Integer.toUnsignedLong(session.getNumPkts()));
        ps.setFloat(idx++, session.getAvgPayload());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getMinPayload()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getMaxPayload()));
        ps.setFloat(idx++, session.getStdPayload());

        // IAT 统计
        ps.setFloat(idx++, session.getMeanIatMs());
        ps.setFloat(idx++, session.getMinIatMs());
        ps.setFloat(idx++, session.getMaxIatMs());
        ps.setFloat(idx++, session.getStdIatMs());

        // TCP 标志
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsSyn()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsAck()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsFin()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsPsh()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsRst()));

        // 协议统计
        ps.setLong(idx++, Integer.toUnsignedLong(session.getDnsPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getTcpPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getUdpPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getIcmpPktCnt()));

        ps.setInt(idx++, session.getHasSyn() ? 1 : 0);
        ps.setInt(idx++, session.getHasFin() ? 1 : 0);
        ps.setInt(idx++, session.getHasRst() ? 1 : 0);
        ps.setInt(idx++, session.getIsEstablished() ? 1 : 0);

        // 其他
        ps.setLong(idx++, Integer.toUnsignedLong(session.getEvidenceCount()));
        ps.setObject(idx++, session.getFlowIdsList().toArray(new String[0]));
        ps.setString(idx++, session.getEndReason());
    }

    private static final class PendingWrite {
        private final SessionEvent session;
        private final ResultFuture<SessionEvent> resultFuture;
        private final AtomicBoolean completed;

        private PendingWrite(SessionEvent session, ResultFuture<SessionEvent> resultFuture) {
            this.session = session;
            this.resultFuture = resultFuture;
            this.completed = new AtomicBoolean(false);
        }

        private void completeSuccess() {
            if (completed.compareAndSet(false, true)) {
                resultFuture.complete(Collections.emptyList());
            }
        }

        private void completeFailure() {
            if (completed.compareAndSet(false, true)) {
                resultFuture.complete(Collections.singletonList(session));
            }
        }
    }
}
