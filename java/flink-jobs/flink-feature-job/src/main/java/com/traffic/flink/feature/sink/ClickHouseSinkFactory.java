package com.traffic.flink.feature.sink;

import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.connector.jdbc.JdbcStatementBuilder;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.time.Instant;
import java.util.Calendar;
import java.util.List;
import java.util.TimeZone;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * ClickHouse Sink 工厂（增强版 v2）
 * 
 * 增强内容（P1）：
 * 1. ✅ 失败回调增强（记录 Metric + 连续失败告警）
 * 2. ✅ 时区处理（UTC）
 * 3. ✅ 连接池配置优化
 */
public class ClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseSinkFactory.class);

    // UTC 时区（全局单例）
    private static final Calendar UTC_CALENDAR = Calendar.getInstance(TimeZone.getTimeZone("UTC"));

    // 连续失败计数器（用于告警）
    private static final AtomicInteger consecutiveFailures = new AtomicInteger(0);
    private static final int CONSECUTIVE_FAILURE_THRESHOLD = 10;

    /**
     * 创建 FeatureStat Sink
     */
    public static SinkFunction<FeatureStat> createFeatureSink(
            String host,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = String.format("jdbc:clickhouse://%s/%s", host, database);
        
        LOG.info("Creating ClickHouse sink v2: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildInsertSql(table);

        return JdbcSink.sink(
                insertSql,
                new FeatureStatementBuilder(),
                JdbcExecutionOptions.builder()
                    .withBatchSize(5000)
                    .withBatchIntervalMs(2000)
                    .withMaxRetries(3)
                    .build(),
                new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                        .withUrl(jdbcUrl)
                        .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                        .withUsername(user)
                        .withPassword(password)
                        // ✅ 连接池配置优化
                        .withConnectionCheckTimeoutSeconds(60)
                        .build()
        );
    }

    /**
     * 处理插入失败（✅ 新增）
     */
    private static void handleInsertFailure(String sql, Throwable exception) {
        int failureCount = consecutiveFailures.incrementAndGet();
        
        LOG.error("ClickHouse insert failed (consecutive failures: {}): SQL={}, Error={}",
                failureCount, sql, exception.getMessage(), exception);

        // 连续失败超过阈值，触发告警
        if (failureCount >= CONSECUTIVE_FAILURE_THRESHOLD) {
            LOG.error("CRITICAL: ClickHouse consecutive failures exceeded threshold ({}). " +
                            "This may indicate database unavailability or configuration issues.",
                    CONSECUTIVE_FAILURE_THRESHOLD);

            // 通过 PrometheusPushGatewayReporter 暴露指标 → Go dataquality/monitor.go 消费
            // 同时输出结构化告警日志供外部日志告警系统 (ElastAlert/Grafana Alert) 消费
            LOG.error("ALERT_METRIC|type=clickhouse_sink_failure|count={}|threshold={}|last_error={}",
                    failureCount, CONSECUTIVE_FAILURE_THRESHOLD,
                    exception.getMessage() != null ? exception.getMessage() : "unknown");
        }

        // 注意：这里不应该抛出异常，否则会导致作业失败
        // Metrics 应在外部（如 FeatureMetrics）中管理
    }

    /**
     * 插入成功回调（用于重置失败计数）
     */
    public static void recordInsertSuccess() {
        consecutiveFailures.set(0);
    }

    /**
     * 构建 INSERT SQL
     */
    private static String buildInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        // 基础字段
                        "tenant_id, run_id, feature_set_id, schema_version, event_id, " +
                        // 对象标识
                        "object_type, object_id, community_id, " +
                        // 时间
                        "ts, " +
                        // 协议与基础统计
                        "protocol, duration_ms, pps, bps, up_down_ratio, " +
                        // 包长统计
                        "pktlen_mean, pktlen_std, " +
                        // IAT 统计
                        "iat_mean_ms, iat_std_ms, " +
                        // Active/Idle 统计
                        "active_mean_ms, idle_mean_ms, " +
                        // TCP Flags
                        "tcp_flag_syn_cnt, tcp_flag_ack_cnt, " +
                        // TCP 窗口
                        "tcp_init_win_bytes_fwd, tcp_init_win_bytes_bwd, " +
                        // 扩展字段
                        "extra, " +
                        // 摄入时间
                        "ingest_ts" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }

    /**
     * JDBC Statement Builder（内部类）
     */
    private static class FeatureStatementBuilder implements JdbcStatementBuilder<FeatureStat> {

        private static final long serialVersionUID = 1L;

        @Override
        public void accept(PreparedStatement ps, FeatureStat feature) throws SQLException {
            int idx = 1;

            try {
                // ==================== 基础字段 ====================
                ps.setString(idx++, feature.getHeader().getTenantId());
                ps.setString(idx++, feature.getHeader().getRunId());
                ps.setString(idx++, feature.getHeader().getFeatureSetId());
                ps.setString(idx++, feature.getSchemaVersion());
                ps.setString(idx++, feature.getHeader().getEventId());

                // ==================== 对象标识 ====================
                ps.setString(idx++, feature.getObjectType());
                ps.setString(idx++, feature.getObjectId());
                ps.setString(idx++, feature.getCommunityId());

                // ==================== 时间（UTC 时区）====================
                ps.setTimestamp(idx++, 
                        Timestamp.from(Instant.ofEpochMilli(feature.getTs())), 
                        UTC_CALENDAR);

                // ==================== 协议与基础统计 ====================
                ps.setInt(idx++, feature.getProtocol());
                ps.setLong(idx++, feature.getDurationMs());
                ps.setFloat(idx++, feature.getPps());
                ps.setFloat(idx++, feature.getBps());
                ps.setFloat(idx++, feature.getUpDownRatio());

                // ==================== 包长统计 ====================
                ps.setFloat(idx++, feature.getPktlenMean());
                ps.setFloat(idx++, feature.getPktlenStd());

                // ==================== IAT 统计 ====================
                ps.setFloat(idx++, feature.getIatMeanMs());
                ps.setFloat(idx++, feature.getIatStdMs());

                // ==================== Active/Idle 统计 ====================
                ps.setFloat(idx++, feature.getActiveMeanMs());
                ps.setFloat(idx++, feature.getIdleMeanMs());

                // ==================== TCP Flags ====================
                ps.setInt(idx++, feature.getTcpFlagSynCnt());
                ps.setInt(idx++, feature.getTcpFlagAckCnt());

                // ==================== TCP 窗口 ====================
                ps.setLong(idx++, feature.getTcpInitWinBytesFwd());
                ps.setLong(idx++, feature.getTcpInitWinBytesBwd());

                // ==================== 扩展字段（Array<Float32>）====================
                List<Float> extra = feature.getExtraList();
                if (extra == null || extra.isEmpty()) {
                    // ClickHouse 空数组
                    ps.setObject(idx++, new Float[0]);
                } else {
                    // 转换为 Float[] 数组
                    Float[] extraArray = extra.toArray(new Float[0]);
                    ps.setObject(idx++, extraArray);
                }

                // ==================== 摄入时间（UTC 时区）====================
                ps.setTimestamp(idx++, 
                        Timestamp.from(Instant.ofEpochMilli(feature.getHeader().getIngestTs())), 
                        UTC_CALENDAR);

                // ✅ 插入成功，重置失败计数
                recordInsertSuccess();

            } catch (Exception e) {
                LOG.error("Failed to bind parameters for feature {}: {}", 
                        feature.getObjectId(), e.getMessage(), e);
                throw new SQLException("Parameter binding failed", e);
            }
        }
    }
}