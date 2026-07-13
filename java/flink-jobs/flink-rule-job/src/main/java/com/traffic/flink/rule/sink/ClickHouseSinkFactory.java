package com.traffic.flink.rule.sink;

import com.traffic.proto.traffic.v1.DetectionBehavior;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Timestamp;
import java.time.Instant;

/**
 * ClickHouse Sink 工厂 — 集群模式
 *
 * 集群架构: 2 Shard x 2 Replica + 3 Keeper
 * 写入 Distributed 表 (自动按 sharding key 分布)
 */
public class ClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseSinkFactory.class);

    /**
     * 创建 Detection Behavior Sink (集群模式)
     *
     * @param hosts    ClickHouse 端点列表 (逗号分隔: "ch-1:8123,ch-2:8123")
     * @param database 数据库名称
     * @param table    Distributed 表名 (detections_behavior)
     * @param user     用户名
     * @param password 密码
     */
    public static SinkFunction<DetectionBehavior> createDetectionSink(
            String hosts,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = String.format("jdbc:clickhouse://%s/%s", hosts, database);
        LOG.info("Creating ClickHouse CLUSTER detection sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = String.format(
                "INSERT INTO %s (" +
                        "tenant_id, run_id, feature_set_id, model_version, event_id, " +
                        "community_id, object_type, object_id, ts, " +
                        "labels, scores, top_label, top_score, " +
                        "ingest_ts" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );

        return JdbcSink.sink(
                insertSql,
                (ps, detection) -> {
                    int idx = 1;
                    ps.setString(idx++, detection.getHeader().getTenantId());
                    ps.setString(idx++, detection.getHeader().getRunId());
                    ps.setString(idx++, detection.getHeader().getFeatureSetId());
                    ps.setString(idx++, detection.getModelVersion());
                    ps.setString(idx++, detection.getHeader().getEventId());
                    ps.setString(idx++, detection.getCommunityId());
                    ps.setString(idx++, detection.getObjectType());
                    ps.setString(idx++, detection.getObjectId());
                    ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(detection.getTs())));
                    ps.setObject(idx++, detection.getLabelsList().toArray(new String[0]));
                    ps.setObject(idx++, detection.getScoresList().toArray(new Float[0]));
                    ps.setString(idx++, detection.getTopLabel());
                    ps.setFloat(idx++, detection.getTopScore());
                    ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(
                            detection.getHeader().getIngestTs())));
                },
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
                        .withConnectionCheckTimeoutSeconds(60)
                        .build()
        );
    }
}