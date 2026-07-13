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
 * ClickHouse Detection Sink 工厂
 */
public class ClickHouseDetectionSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseDetectionSinkFactory.class);

    /**
     * 创建 DetectionBehavior Sink
     */
    public static SinkFunction<DetectionBehavior> createDetectionSink(
            String host,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = String.format("jdbc:clickhouse://%s/%s", host, database);
        
        LOG.info("Creating ClickHouse detection sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildInsertSql(table);

        return JdbcSink.sink(
                insertSql,
                (ps, detection) -> {
                    int idx = 1;
                    
                    // header
                    ps.setString(idx++, detection.getHeader().getTenantId());
                    ps.setString(idx++, detection.getHeader().getRunId());
                    ps.setString(idx++, detection.getHeader().getFeatureSetId());
                    ps.setString(idx++, detection.getModelVersion());
                    ps.setString(idx++, detection.getHeader().getEventId());
                    
                    // identifiers
                    ps.setString(idx++, detection.getCommunityId());
                    ps.setString(idx++, detection.getObjectType());
                    ps.setString(idx++, detection.getObjectId());
                    
                    // timestamp
                    ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(detection.getTs())));
                    
                    // labels (Array<String>)
                    if (detection.getLabelsCount() > 0) {
                        ps.setObject(idx++, detection.getLabelsList().toArray(new String[0]));
                    } else {
                        ps.setObject(idx++, new String[0]);
                    }
                    
                    // scores (Array<Float32>)
                    if (detection.getScoresCount() > 0) {
                        Float[] scores = new Float[detection.getScoresCount()];
                        for (int i = 0; i < detection.getScoresCount(); i++) {
                            scores[i] = detection.getScores(i);
                        }
                        ps.setObject(idx++, scores);
                    } else {
                        ps.setObject(idx++, new Float[0]);
                    }
                    
                    // top result
                    ps.setString(idx++, detection.getTopLabel());
                    ps.setFloat(idx++, detection.getTopScore());
                    
                    // ingest_ts
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
                        .build()
        );
    }

    private static String buildInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        "tenant_id, run_id, feature_set_id, model_version, event_id, " +
                        "community_id, object_type, object_id, " +
                        "ts, " +
                        "labels, scores, top_label, top_score, " +
                        "ingest_ts" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }
}