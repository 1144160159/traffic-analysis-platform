package com.traffic.flink.alert.sink;

import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.Evidence;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * ClickHouse Alert Sink 工厂 (修复版)
 * 
 * 修复内容：
 * - 验证 Array 类型处理兼容性
 * - 添加空值保护
 * - 优化批量写入配置
 * - 添加详细日志
 */
public class ClickHouseAlertSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseAlertSinkFactory.class);

    // ClickHouse JDBC 驱动类名
    private static final String CLICKHOUSE_DRIVER = "com.clickhouse.jdbc.ClickHouseDriver";

    /**
     * 创建 Alert Sink
     * 
     * @param host ClickHouse 主机地址
     * @param database 数据库名称
     * @param table 表名称
     * @param user 用户名
     * @param password 密码
     * @return SinkFunction 实例
     */
    public static SinkFunction<Alert> createAlertSink(
            String host,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = buildJdbcUrl(host, database);
        LOG.info("Creating ClickHouse alert sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildAlertInsertSql(table);
        LOG.debug("Alert INSERT SQL: {}", insertSql);

        return JdbcSink.sink(
                insertSql,
                (ps, alert) -> {
                    try {
                        setAlertParameters(ps, alert);
                    } catch (Exception e) {
                        LOG.error("Failed to set alert parameters: alertId={}, error={}",
                                alert.getAlertId(), e.getMessage(), e);
                        throw new RuntimeException(e);
                    }
                },
                buildExecutionOptions(),
                buildConnectionOptions(jdbcUrl, user, password)
        );
    }

    /**
     * 创建 Evidence Sink
     */
    public static SinkFunction<Evidence> createEvidenceSink(
            String host,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = buildJdbcUrl(host, database);
        LOG.info("Creating ClickHouse evidence sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildEvidenceInsertSql(table);
        LOG.debug("Evidence INSERT SQL: {}", insertSql);

        return JdbcSink.sink(
                insertSql,
                (ps, evidence) -> {
                    try {
                        setEvidenceParameters(ps, evidence);
                    } catch (Exception e) {
                        LOG.error("Failed to set evidence parameters: evidenceId={}, error={}",
                                evidence.getEvidenceId(), e.getMessage(), e);
                        throw new RuntimeException(e);
                    }
                },
                buildExecutionOptions(),
                buildConnectionOptions(jdbcUrl, user, password)
        );
    }

    /**
     * 设置 Alert PreparedStatement 参数
     */
    private static void setAlertParameters(java.sql.PreparedStatement ps, Alert alert) throws java.sql.SQLException {
        int idx = 1;
        long now = System.currentTimeMillis();

        // 1-2: 租户与标识
        ps.setString(idx++, nullToEmpty(alert.getTenantId()));
        ps.setString(idx++, nullToEmpty(alert.getAlertId()));

        // 3-7: 去重与关联字段
        ps.setString(idx++, nullToEmpty(alert.getDedupFingerprint()));
        ps.setString(idx++, nullToEmpty(alert.getCommunityId()));
        ps.setString(idx++, nullToEmpty(alert.getSessionId()));
        ps.setString(idx++, nullToEmpty(alert.getCampaignId()));
        ps.setString(idx++, nullToEmpty(alert.getFeatureSetId()));

        // 8-13: 网络五元组
        ps.setString(idx++, nullToEmpty(alert.getSrcIp()));
        ps.setString(idx++, nullToEmpty(alert.getDstIp()));
        ps.setInt(idx++, Math.max(0, alert.getSrcPort()));
        ps.setInt(idx++, Math.max(0, alert.getDstPort()));
        ps.setInt(idx++, Math.max(0, alert.getProtocol()));
        ps.setString(idx++, nullToEmpty(alert.getProtocolName()));

        // 14-18: 告警分类与状态
        ps.setString(idx++, nullToEmpty(alert.getAlertType()));
        String[] labels = alert.getLabelsList().toArray(new String[0]);
        ps.setObject(idx++, labels);
        ps.setString(idx++, alert.getSeverity() != null ? alert.getSeverity().name() : "SEVERITY_UNSPECIFIED");
        ps.setFloat(idx++, alert.getScore());
        ps.setString(idx++, alert.getStatus() != null ? alert.getStatus().name() : "ALERT_STATUS_NEW");
        ps.setString(idx++, nullToEmpty(alert.getAssignee()));

        // 19-23: 证据、Arkime 与反馈
        String[] evidenceIds = alert.getEvidenceIdsList().toArray(new String[0]);
        ps.setObject(idx++, evidenceIds);
        ps.setString(idx++, nullToEmpty(alert.getArkimeSessionLink()));
        ps.setString(idx++, nullToEmpty(alert.getFeedbackLabel()));
        ps.setInt(idx++, Math.max(0, alert.getFeedbackCount()));

        // 24-26: 时间窗口与次数，真实表为 Int64 毫秒时间戳。
        ps.setLong(idx++, toEpochMillis(alert.getFirstSeen(), now));
        ps.setLong(idx++, toEpochMillis(alert.getLastSeen(), now));
        ps.setInt(idx++, alert.getCount());

        // 27-30: 版本与事件
        ps.setString(idx++, nullToEmpty(alert.getModelVersion()));
        ps.setString(idx++, nullToEmpty(alert.getRuleVersion()));
        ps.setLong(idx++, Math.max(0L, alert.getStateVersion()));
        ps.setString(idx++, nullToEmpty(alert.getEventId()));

        // 31-32: 创建与更新时间，真实表字段名为 created_at / updated_at。
        ps.setLong(idx++, toEpochMillis(alert.getIngestTs(), now));
        ps.setLong(idx++, toEpochMillis(alert.getUpdatedTs(), now));
    }

    /**
     * 设置 Evidence PreparedStatement 参数
     */
    private static void setEvidenceParameters(java.sql.PreparedStatement ps, Evidence evidence) throws java.sql.SQLException {
        int idx = 1;

        // 1-3: 标识
        ps.setString(idx++, nullToEmpty(evidence.getTenantId()));
        ps.setString(idx++, nullToEmpty(evidence.getEvidenceId()));
        ps.setString(idx++, nullToEmpty(evidence.getAlertId()));

        // 4: 时间戳，真实表为 Int64 毫秒时间戳
        ps.setLong(idx++, toEpochMillis(evidence.getTs(), System.currentTimeMillis()));

        // 5-6: 类型与摘要
        ps.setString(idx++, nullToEmpty(evidence.getType()));
        ps.setString(idx++, nullToEmpty(evidence.getSummary()));

        // 7-8: JSON 字段
        ps.setString(idx++, nullToEmpty(evidence.getMetricsJson()));
        ps.setString(idx++, nullToEmpty(evidence.getSnippetRefJson()));

        // 9: Arkime 链接
        ps.setString(idx++, nullToEmpty(evidence.getArkimeLink()));

        // 10: 置信度
        ps.setFloat(idx++, evidence.getConfidence());

        // 11: 事件 ID
        ps.setString(idx++, nullToEmpty(evidence.getEventId()));

        // 12: 摄入时间戳，真实表为 Int64 毫秒时间戳
        ps.setLong(idx++, toEpochMillis(evidence.getIngestTs(), System.currentTimeMillis()));

        // 13: 可视化 URL
        ps.setString(idx++, nullToEmpty(evidence.getVisualizationUrl()));
    }

    /**
     * 构建 Alert INSERT SQL
     * 
     * 共 32 个字段
     */
    private static String buildAlertInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        // 1-2: 租户与标识
                        "tenant_id, alert_id, " +
                        // 3-7: 去重与关联
                        "dedup_fingerprint, community_id, session_id, campaign_id, feature_set_id, " +
                        // 8-13: 网络五元组
                        "src_ip, dst_ip, src_port, dst_port, protocol, protocol_name, " +
                        // 14-18: 告警分类与状态
                        "alert_type, labels, severity, score, status, assignee, " +
                        // 19-23: 证据、Arkime 与反馈
                        "evidence_ids, arkime_session_link, feedback_label, feedback_count, " +
                        // 24-26: 时间窗口与次数
                        "first_seen, last_seen, count, " +
                        // 27-30: 版本与事件
                        "model_version, rule_version, state_version, event_id, " +
                        // 31-32: 创建与更新时间
                        "created_at, updated_at" +
                        ") VALUES (" +
                        "?, ?, ?, ?, ?, ?, ?, ?, ?, ?, " +  // 1-10
                        "?, ?, ?, ?, ?, ?, ?, ?, ?, ?, " +  // 11-20
                        "?, ?, ?, ?, ?, ?, ?, ?, ?, ?, " +  // 21-30
                        "?, ?" +                            // 31-32
                        ")",
                table
        );
    }

    /**
     * 构建 Evidence INSERT SQL
     * 
     * 共 13 个字段
     */
    private static String buildEvidenceInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        "tenant_id, evidence_id, alert_id, ts, type, summary, " +
                        "metrics_json, snippet_ref_json, arkime_link, confidence, " +
                        "event_id, ingest_ts, visualization_url" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }

    /**
     * 构建 JDBC URL
     */
    private static String buildJdbcUrl(String host, String database) {
        // 添加 ClickHouse 特定参数
        return String.format(
                "jdbc:clickhouse://%s/%s?socket_timeout=300000&connection_timeout=30000",
                host, database
        );
    }

    /**
     * 构建执行选项
     */
    private static JdbcExecutionOptions buildExecutionOptions() {
        return JdbcExecutionOptions.builder()
                .withBatchSize(5000)           // 批量大小
                .withBatchIntervalMs(2000)     // 批量间隔
                .withMaxRetries(3)             // 最大重试次数
                .build();
    }

    /**
     * 构建连接选项
     */
    private static JdbcConnectionOptions buildConnectionOptions(
            String jdbcUrl,
            String user,
            String password
    ) {
        return new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                .withUrl(jdbcUrl)
                .withDriverName(CLICKHOUSE_DRIVER)
                .withUsername(user)
                .withPassword(password)
                .build();
    }

    /**
     * ClickHouse alert/evidence 表以 Int64 存储毫秒时间戳。
     */
    private static long toEpochMillis(long epochMillis, long fallbackMillis) {
        return epochMillis > 0 ? epochMillis : fallbackMillis;
    }

    /**
     * 空值保护
     */
    private static String nullToEmpty(String value) {
        return value != null ? value : "";
    }
}
