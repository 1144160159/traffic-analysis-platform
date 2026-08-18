package com.traffic.flink.behavior.receipt;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.PreparedStatement;
import java.sql.SQLException;

/**
 * RunDetectionResultClickHouseSinkFactory —— 每输入×detector 结果行落库
 * traffic.analysis_detections(列契约:tenant_id/run_id/execution_spec_sha256/
 * input_identity/detector_id/disposition/score/labels/evidence_refs/ts)。
 * 批写;失败显式抛出让作业 fail-closed,不静默丢结果。
 */
public final class RunDetectionResultClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(RunDetectionResultClickHouseSinkFactory.class);

    private static final String INSERT_SQL =
            "INSERT INTO traffic.analysis_detections (" +
            "tenant_id, run_id, execution_spec_sha256, input_identity, detector_id, " +
            "disposition, score, labels, evidence_refs, ts" +
            ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)";

    private RunDetectionResultClickHouseSinkFactory() {}

    public static SinkFunction<RunDetectionResultRow> createSink(
            String url, String user, String password, int batchSize, long batchIntervalMs) {
        LOG.info("analysis_detections sink: url={} batch={}/{}ms", url, batchSize, batchIntervalMs);
        return JdbcSink.sink(
                INSERT_SQL,
                RunDetectionResultClickHouseSinkFactory::bind,
                JdbcExecutionOptions.builder()
                        .withBatchSize(batchSize)
                        .withBatchIntervalMs(batchIntervalMs)
                        .withMaxRetries(3)
                        .build(),
                new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                        .withUrl("jdbc:clickhouse://" + url + "/traffic")
                        .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                        .withUsername(user)
                        .withPassword(password)
                        .build());
    }

    private static void bind(PreparedStatement ps, RunDetectionResultRow row) throws SQLException {
        ps.setString(1, row.tenantId);
        ps.setString(2, row.runId);
        ps.setString(3, row.executionSpecSha256);
        ps.setString(4, row.inputIdentity);
        ps.setString(5, row.detectorId);
        ps.setString(6, row.disposition);
        ps.setDouble(7, row.score);
        ps.setString(8, row.labels == null ? "" : row.labels);
        ps.setString(9, row.evidenceRefs == null ? "" : row.evidenceRefs);
        ps.setObject(10, new java.sql.Timestamp(row.tsMs));
    }
}
