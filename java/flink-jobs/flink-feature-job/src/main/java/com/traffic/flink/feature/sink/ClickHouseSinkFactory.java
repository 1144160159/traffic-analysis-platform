package com.traffic.flink.feature.sink;

import com.traffic.flink.common.DeploymentActivation;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FeatureFingerprint;
import com.traffic.proto.traffic.v1.FeatureSeq;

import org.apache.flink.connector.jdbc.JdbcStatementBuilder;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.sql.Timestamp;
import java.sql.Types;
import java.time.Instant;
import java.util.Calendar;
import java.util.List;
import java.util.TimeZone;

/**
 * ClickHouse sink factory for the three M03 feature projections.
 *
 * <p>Every returned sink batches records, verifies the full executeBatch
 * receipt, retains unacknowledged rows in operator state and fails the Flink
 * checkpoint after bounded retries.</p>
 */
public class ClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseSinkFactory.class);

    // UTC 时区（全局单例）
    private static final Calendar UTC_CALENDAR = Calendar.getInstance(TimeZone.getTimeZone("UTC"));

    private static final int BATCH_SIZE = 5000;
    private static final int MAX_RETRIES = 3;

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
        return createFeatureSink(host, database, table, user, password,
                DeploymentActivation.legacy("flink-feature-job"));
    }

    public static SinkFunction<FeatureStat> createFeatureSink(
            String host, String database, String table, String user, String password,
            DeploymentActivation activation) {
        String jdbcUrl = jdbcUrl(host, database);
        LOG.info("Creating checkpoint-aware ClickHouse sink: {} -> {}.{}", jdbcUrl, database, table);
        return new CheckpointAwareClickHouseSink<>(
                jdbcUrl, user, password, table, buildInsertSql(table),
                new FeatureStatementBuilder(),
                feature -> feature.getHeader().getEventId(),
                FeatureStat.class,
                "clickhouse-feature-stat-pending-v1",
                BATCH_SIZE, MAX_RETRIES, activation);
    }

    public static SinkFunction<FeatureSeq> createFeatureSequenceSink(
            String host, String database, String table, String user, String password) {
        return createFeatureSequenceSink(host, database, table, user, password,
                DeploymentActivation.legacy("flink-feature-job"));
    }

    public static SinkFunction<FeatureSeq> createFeatureSequenceSink(
            String host, String database, String table, String user, String password,
            DeploymentActivation activation) {
        String jdbcUrl = jdbcUrl(host, database);
        return new CheckpointAwareClickHouseSink<>(
                jdbcUrl, user, password, table, buildSequenceInsertSql(table),
                new FeatureSequenceStatementBuilder(),
                feature -> feature.getHeader().getEventId(),
                FeatureSeq.class,
                "clickhouse-feature-sequence-pending-v1",
                BATCH_SIZE, MAX_RETRIES, activation);
    }

    public static SinkFunction<FeatureFingerprint> createFeatureFingerprintSink(
            String host, String database, String table, String user, String password) {
        return createFeatureFingerprintSink(host, database, table, user, password,
                DeploymentActivation.legacy("flink-feature-job"));
    }

    public static SinkFunction<FeatureFingerprint> createFeatureFingerprintSink(
            String host, String database, String table, String user, String password,
            DeploymentActivation activation) {
        String jdbcUrl = jdbcUrl(host, database);
        return new CheckpointAwareClickHouseSink<>(
                jdbcUrl, user, password, table, buildFingerprintInsertSql(table),
                new FeatureFingerprintStatementBuilder(),
                feature -> feature.getHeader().getEventId(),
                FeatureFingerprint.class,
                "clickhouse-feature-fingerprint-pending-v1",
                BATCH_SIZE, MAX_RETRIES, activation);
    }

    private static String jdbcUrl(String host, String database) {
        return String.format("jdbc:clickhouse://%s/%s", host, database);
    }

    /**
     * 构建 INSERT SQL
     */
    static String buildInsertSql(String table) {
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
                        // 扩展字段与 M03 来源/完整性合同
                        "extra, event_schema_version, aggregate_version, " +
                        "event_time_start_ms, event_time_end_ms, source_watermark_ms, " +
                        "source_event_ids, evidence_ids, feature_category, availability, " +
                        "algorithm_version, window_id, value_unit, is_partial, missing_fields, missing_reason, " +
                        // 摄入时间
                        "ingest_ts" +
                        ") VALUES (" + String.join(", ", java.util.Collections.nCopies(41, "?")) + ")",
                table
        );
    }

    static String buildSequenceInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (tenant_id, run_id, feature_set_id, event_id, "
                        + "object_type, object_id, community_id, window_id, ts_start, ts_end, "
                        + "pktlen_seq_hash, iat_seq_hash, wavelet_releng_fwd, wavelet_releng_bwd, "
                        + "wavelet_entropy_fwd, wavelet_entropy_bwd, wavelet_detail_mean_fwd, "
                        + "wavelet_detail_mean_bwd, wavelet_detail_std_fwd, wavelet_detail_std_bwd, "
                        + "seq_blob_ref, feature_category, availability, schema_version, "
                        + "algorithm_version, value_unit, source_event_ids, evidence_ids, "
                        + "missing_fields, missing_reason, ingest_ts) VALUES ("
                        + String.join(", ", java.util.Collections.nCopies(31, "?")) + ")",
                table);
    }

    static String buildFingerprintInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (tenant_id, run_id, feature_set_id, event_id, community_id, "
                        + "session_id, ts, is_encrypted, tls_version, ja3, ja4, sni, sni_hash, "
                        + "cert_sha256, cert_is_self_signed, pubkey_len, quic_version, "
                        + "transport_security, raw_traffic_ref, hex_freq, hex_ratio, entropy_payload, "
                        + "chi_square_bfd, feature_category, availability, schema_version, "
                        + "algorithm_version, window_id, event_time_start_ms, event_time_end_ms, "
                        + "source_event_ids, evidence_ids, missing_fields, missing_reason, ingest_ts) VALUES ("
                        + String.join(", ", java.util.Collections.nCopies(35, "?")) + ")",
                table);
    }

    /**
     * JDBC Statement Builder（内部类）
     */
    static class FeatureStatementBuilder implements JdbcStatementBuilder<FeatureStat> {

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

                ps.setString(idx++, feature.getHeader().getSchemaVersion());
                ps.setLong(idx++, feature.getHeader().getAggregateVersion());
                ps.setLong(idx++, feature.getEventTimeStartMs());
                ps.setLong(idx++, feature.getEventTimeEndMs());
                // FeatureStat v1 has no operator-watermark field. Preserve
                // unknown as NULL instead of presenting event time as progress.
                ps.setNull(idx++, Types.BIGINT);
                ps.setObject(idx++, feature.getSourceEventIdsList().toArray(new String[0]));
                ps.setObject(idx++, feature.getEvidenceIdsList().toArray(new String[0]));
                ps.setString(idx++, feature.getFeatureCategory().name());
                ps.setString(idx++, feature.getAvailability().name());
                ps.setString(idx++, feature.getAlgorithmVersion());
                ps.setString(idx++, feature.getWindowId());
                ps.setString(idx++, feature.getValueUnit());
                ps.setInt(idx++, feature.getAvailability()
                        == com.traffic.proto.traffic.v1.FeatureAvailability.FEATURE_AVAILABILITY_AVAILABLE ? 0 : 1);
                ps.setObject(idx++, feature.getMissingFieldsList().toArray(new String[0]));
                ps.setString(idx++, feature.getMissingReason());

                // ==================== 摄入时间（UTC 时区）====================
                ps.setTimestamp(idx++, 
                        Timestamp.from(Instant.ofEpochMilli(feature.getHeader().getIngestTs())), 
                        UTC_CALENDAR);

            } catch (Exception e) {
                LOG.error("Failed to bind parameters for feature {}: {}", 
                        feature.getObjectId(), e.getMessage(), e);
                throw new SQLException("Parameter binding failed", e);
            }
        }
    }

    static class FeatureSequenceStatementBuilder implements JdbcStatementBuilder<FeatureSeq> {
        private static final long serialVersionUID = 1L;

        @Override
        public void accept(PreparedStatement ps, FeatureSeq feature) throws SQLException {
            int idx = 1;
            ps.setString(idx++, feature.getHeader().getTenantId());
            ps.setString(idx++, feature.getHeader().getRunId());
            ps.setString(idx++, feature.getHeader().getFeatureSetId());
            ps.setString(idx++, feature.getHeader().getEventId());
            ps.setString(idx++, feature.getObjectType());
            ps.setString(idx++, feature.getObjectId());
            ps.setString(idx++, feature.getCommunityId());
            ps.setString(idx++, feature.getWindowId());
            ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(feature.getTsStart())), UTC_CALENDAR);
            ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(feature.getTsEnd())), UTC_CALENDAR);
            ps.setString(idx++, feature.getPktlenSeqHash());
            ps.setString(idx++, feature.getIatSeqHash());
            ps.setFloat(idx++, feature.getWaveletRelengFwd());
            ps.setFloat(idx++, feature.getWaveletRelengBwd());
            ps.setFloat(idx++, feature.getWaveletEntropyFwd());
            ps.setFloat(idx++, feature.getWaveletEntropyBwd());
            ps.setFloat(idx++, feature.getWaveletDetailMeanFwd());
            ps.setFloat(idx++, feature.getWaveletDetailMeanBwd());
            ps.setFloat(idx++, feature.getWaveletDetailStdFwd());
            ps.setFloat(idx++, feature.getWaveletDetailStdBwd());
            ps.setString(idx++, feature.getSeqBlobRef());
            ps.setString(idx++, feature.getFeatureCategory().name());
            ps.setString(idx++, feature.getAvailability().name());
            ps.setString(idx++, feature.getSchemaVersion());
            ps.setString(idx++, feature.getAlgorithmVersion());
            ps.setString(idx++, feature.getValueUnit());
            ps.setObject(idx++, feature.getSourceEventIdsList().toArray(new String[0]));
            ps.setObject(idx++, feature.getEvidenceIdsList().toArray(new String[0]));
            ps.setObject(idx++, feature.getMissingFieldsList().toArray(new String[0]));
            ps.setString(idx++, feature.getMissingReason());
            ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(feature.getHeader().getIngestTs())), UTC_CALENDAR);
        }
    }

    static class FeatureFingerprintStatementBuilder
            implements JdbcStatementBuilder<FeatureFingerprint> {
        private static final long serialVersionUID = 1L;

        @Override
        public void accept(PreparedStatement ps, FeatureFingerprint feature) throws SQLException {
            int idx = 1;
            ps.setString(idx++, feature.getHeader().getTenantId());
            ps.setString(idx++, feature.getHeader().getRunId());
            ps.setString(idx++, feature.getHeader().getFeatureSetId());
            ps.setString(idx++, feature.getHeader().getEventId());
            ps.setString(idx++, feature.getCommunityId());
            ps.setString(idx++, feature.getSessionId());
            ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(feature.getTs())), UTC_CALENDAR);
            ps.setInt(idx++, feature.getIsEncrypted());
            ps.setString(idx++, feature.getTlsVersion());
            ps.setString(idx++, feature.getJa3());
            ps.setString(idx++, feature.getJa4());
            ps.setString(idx++, feature.getSni());
            ps.setString(idx++, feature.getSniHash());
            ps.setString(idx++, feature.getCertSha256());
            ps.setInt(idx++, feature.getCertIsSelfSigned());
            ps.setInt(idx++, feature.getPubkeyLen());
            ps.setString(idx++, feature.getQuicVersion());
            ps.setString(idx++, feature.getTransportSecurity().name());
            ps.setString(idx++, feature.getRawTrafficRef());
            ps.setObject(idx++, feature.getHexFreqList().toArray(new Float[0]));
            ps.setObject(idx++, feature.getHexRatioList().toArray(new Float[0]));
            ps.setFloat(idx++, feature.getEntropyPayload());
            ps.setFloat(idx++, feature.getChiSquareBfd());
            ps.setString(idx++, feature.getFeatureCategory().name());
            ps.setString(idx++, feature.getAvailability().name());
            ps.setString(idx++, feature.getSchemaVersion());
            ps.setString(idx++, feature.getAlgorithmVersion());
            ps.setString(idx++, feature.getWindowId());
            ps.setLong(idx++, feature.getEventTimeStartMs());
            ps.setLong(idx++, feature.getEventTimeEndMs());
            ps.setObject(idx++, feature.getSourceEventIdsList().toArray(new String[0]));
            ps.setObject(idx++, feature.getEvidenceIdsList().toArray(new String[0]));
            ps.setObject(idx++, feature.getMissingFieldsList().toArray(new String[0]));
            ps.setString(idx++, feature.getMissingReason());
            ps.setTimestamp(idx++, Timestamp.from(Instant.ofEpochMilli(feature.getHeader().getIngestTs())), UTC_CALENDAR);
        }
    }
}
