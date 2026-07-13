////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/sink/BehaviorClickHouseSinkFactory.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.sink;

import com.traffic.proto.traffic.v1.DetectionBehavior;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Timestamp;
import java.util.List;
import java.util.stream.Collectors;

/**
 * ClickHouse Sink 工厂类
 * 
 * 创建用于写入 detections_behavior_local 表的 Sink。
 * 
 * 表结构匹配（来自 DDL）：
 * - tenant_id String
 * - run_id String
 * - feature_set_id String
 * - model_version String
 * - event_id String
 * - community_id String
 * - object_type LowCardinality(String)
 * - object_id String
 * - ts DateTime64(3)
 * - labels Array(LowCardinality(String))
 * - scores Array(Float32)
 * - top_label LowCardinality(String)
 * - top_score Float32
 * - ingest_ts DateTime64(3) DEFAULT now64(3)
 */
public class BehaviorClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(BehaviorClickHouseSinkFactory.class);

    /**
     * 插入 SQL
     */
    private static final String INSERT_SQL = 
            "INSERT INTO %s.%s (" +
            "tenant_id, run_id, feature_set_id, model_version, event_id, " +
            "community_id, object_type, object_id, ts, " +
            "labels, scores, top_label, top_score, ingest_ts" +
            ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)";

    private BehaviorClickHouseSinkFactory() {
        // Utility class
    }

    /**
     * 创建 ClickHouse Sink
     *
     * @param url            ClickHouse URL (host:port)
     * @param database       数据库名
     * @param table          表名
     * @param user           用户名
     * @param password       密码
     * @param batchSize      批量大小
     * @param batchIntervalMs 批量间隔（毫秒）
     * @return SinkFunction
     */
    public static SinkFunction<DetectionBehavior> createSink(
            String url,
            String database,
            String table,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs) {

        String jdbcUrl = buildJdbcUrl(url, database);
        String sql = String.format(INSERT_SQL, database, table);

        LOG.info("Creating ClickHouse sink: url={}, table={}.{}, batchSize={}, batchIntervalMs={}",
                url, database, table, batchSize, batchIntervalMs);

        return JdbcSink.sink(
                sql,
                (statement, detection) -> {
                    try {
                        // 1. tenant_id
                        statement.setString(1, getStringOrDefault(
                                detection.hasHeader() ? detection.getHeader().getTenantId() : null, ""));
                        
                        // 2. run_id
                        statement.setString(2, getStringOrDefault(
                                detection.hasHeader() ? detection.getHeader().getRunId() : null, ""));
                        
                        // 3. feature_set_id
                        statement.setString(3, getStringOrDefault(
                                detection.hasHeader() ? detection.getHeader().getFeatureSetId() : null, ""));
                        
                        // 4. model_version
                        statement.setString(4, getStringOrDefault(detection.getModelVersion(), "unknown"));
                        
                        // 5. event_id
                        statement.setString(5, getStringOrDefault(
                                detection.hasHeader() ? detection.getHeader().getEventId() : null, ""));
                        
                        // 6. community_id
                        statement.setString(6, getStringOrDefault(detection.getCommunityId(), ""));
                        
                        // 7. object_type
                        statement.setString(7, getStringOrDefault(detection.getObjectType(), ""));
                        
                        // 8. object_id
                        statement.setString(8, getStringOrDefault(detection.getObjectId(), ""));
                        
                        // 9. ts (DateTime64(3))
                        statement.setTimestamp(9, new Timestamp(detection.getTs()));
                        
                        // 10. labels (Array)
                        List<String> labels = detection.getLabelsList();
                        statement.setArray(10, statement.getConnection().createArrayOf(
                                "String", labels.toArray(new String[0])));
                        
                        // 11. scores (Array)
                        List<Float> scores = detection.getScoresList();
                        Float[] scoresArray = scores.toArray(new Float[0]);
                        statement.setArray(11, statement.getConnection().createArrayOf(
                                "Float32", scoresArray));
                        
                        // 12. top_label
                        statement.setString(12, getStringOrDefault(detection.getTopLabel(), "unknown"));
                        
                        // 13. top_score
                        statement.setFloat(13, detection.getTopScore());
                        
                        // 14. ingest_ts
                        statement.setTimestamp(14, new Timestamp(System.currentTimeMillis()));

                    } catch (Exception e) {
                        LOG.error("Failed to set statement parameters: {}", e.getMessage(), e);
                        throw e;
                    }
                },
                JdbcExecutionOptions.builder()
                        .withBatchSize(batchSize)
                        .withBatchIntervalMs(batchIntervalMs)
                        .withMaxRetries(3)
                        .build(),
                new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                        .withUrl(jdbcUrl)
                        .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                        .withUsername(user)
                        .withPassword(password)
                        .build()
        );
    }

    /**
     * 创建简化配置的 ClickHouse Sink
     */
    public static SinkFunction<DetectionBehavior> createSink(
            String url,
            String database,
            String table,
            String user,
            String password) {
        return createSink(url, database, table, user, password, 5000, 2000);
    }

    /**
     * 构建 JDBC URL
     */
    private static String buildJdbcUrl(String hostPort, String database) {
        // 处理可能已经包含 jdbc:clickhouse:// 前缀的情况
        if (hostPort.startsWith("jdbc:clickhouse://")) {
            return hostPort;
        }
        
        // 处理可能包含 http:// 或 https:// 的情况
        if (hostPort.startsWith("http://") || hostPort.startsWith("https://")) {
            hostPort = hostPort.replaceFirst("https?://", "");
        }

        return String.format("jdbc:clickhouse://%s/%s?socket_timeout=300000&connection_timeout=60000",
                hostPort, database);
    }

    /**
     * 获取字符串或默认值
     */
    private static String getStringOrDefault(String value, String defaultValue) {
        return (value != null && !value.isEmpty()) ? value : defaultValue;
    }
}