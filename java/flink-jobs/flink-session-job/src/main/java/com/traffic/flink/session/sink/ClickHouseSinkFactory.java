package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.SessionEvent;
import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.sql.Types;

/**
 * ClickHouse Sink 工厂类（修复版）
 * 
 * 修复要点：
 * 1. ✅ 修复字段映射：bytes_up/bytes_down 对应 SessionEvent.bytesFwd/bytesBwd
 *    （在修复后的 SessionAggregator 中，bytesFwd 已经映射为 client→server）
 * 2. ✅ 增加异常处理与详细日志
 * 3. ✅ 增加字段注释
 */
public class ClickHouseSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHouseSinkFactory.class);

    private ClickHouseSinkFactory() {}

    /**
     * 创建 ClickHouse Sink
     *
     * @param url      JDBC URL (如 jdbc:clickhouse://host:8123/database)
     * @param table    目标表名
     * @param user     用户名
     * @param password 密码
     * @param batchSize 批量大小
     * @param batchIntervalMs 批量间隔
     * @return ClickHouse Sink
     */
    public static SinkFunction<SessionEvent> createSink(
            String url,
            String table,
            String user,
            String password,
            int batchSize,
            long batchIntervalMs) {

        LOG.info("Creating ClickHouse sink for table: {}, url: {}, batchSize: {}, batchIntervalMs: {}ms", 
                table, url, batchSize, batchIntervalMs);

        String insertSql = buildInsertSql(table);
        LOG.debug("ClickHouse INSERT SQL: {}", insertSql);

        return JdbcSink.sink(
            insertSql,
            ClickHouseSinkFactory::bindStatement,
            JdbcExecutionOptions.builder()
                .withBatchSize(batchSize)
                .withBatchIntervalMs(batchIntervalMs)
                .withMaxRetries(3)
                .build(),
            new JdbcConnectionOptions.JdbcConnectionOptionsBuilder()
                .withUrl(url)
                .withDriverName("com.clickhouse.jdbc.ClickHouseDriver")
                .withUsername(user)
                .withPassword(password)
                .build()
        );
    }

    /**
     * 构建 INSERT SQL 语句
     * ✅ 增加字段注释
     */
    static String buildInsertSql(String table) {
        return String.format(
            "INSERT INTO %s (" +
                "session_id, tenant_id, community_id, ts_start, ts_end, duration_ms, " +
                "packets_fwd, packets_bwd, bytes_fwd, bytes_bwd, " +
                "event_id, run_id, feature_set_id, event_ts, ingest_ts, kafka_ts, flink_out_ts, probe_id, " +
                "src_ip, dst_ip, src_port, dst_port, protocol, bytes_total, up_down_ratio, " +
                "num_pkts, avg_payload, min_payload, max_payload, std_payload, " +
                "mean_iat_ms, min_iat_ms, max_iat_ms, std_iat_ms, " +
                "flags_syn, flags_ack, flags_fin, flags_psh, flags_rst, " +
                "dns_pkt_cnt, tcp_pkt_cnt, udp_pkt_cnt, icmp_pkt_cnt, " +
                "has_syn, has_fin, has_rst, is_established, evidence_count, flow_ids, end_reason, " +
                "event_schema_version, aggregate_version, identity_version, session_version, " +
                "event_time_start_ms, event_time_end_ms, source_watermark_ms, source_event_ids, " +
                "evidence_ids, completeness, is_partial, missing_fields" +
            ") VALUES (" + String.join(", ", java.util.Collections.nCopies(62, "?")) + ")",
            table);
    }

    static void bindStatement(PreparedStatement ps, SessionEvent session) throws SQLException {
        int idx = 1;
        com.traffic.proto.traffic.v1.EventHeader header = session.hasHeader()
                ? session.getHeader()
                : com.traffic.proto.traffic.v1.EventHeader.getDefaultInstance();
        com.traffic.proto.traffic.v1.FiveTuple tuple = session.hasTuple()
                ? session.getTuple()
                : com.traffic.proto.traffic.v1.FiveTuple.getDefaultInstance();

        ps.setString(idx++, session.getSessionId());
        ps.setString(idx++, header.getTenantId());
        ps.setString(idx++, session.getCommunityId());
        ps.setLong(idx++, session.getTsStart());
        ps.setLong(idx++, session.getTsEnd());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getDurationMs()));
        // DDL 语义：packets_fwd = 正向包数、packets_bwd = 反向包数。
        // SessionEvent 契约已补齐 packets_fwd/packets_bwd(v1 加法式字段),直接绑定;
        // 旧生产者缺省为 0(proto3 zero default),保持兼容。
        ps.setLong(idx++, session.getPacketsFwd());
        ps.setLong(idx++, session.getPacketsBwd());
        ps.setLong(idx++, session.getBytesFwd());
        ps.setLong(idx++, session.getBytesBwd());
        ps.setString(idx++, header.getEventId());
        ps.setString(idx++, header.getRunId());
        ps.setString(idx++, header.getFeatureSetId());
        ps.setLong(idx++, header.getEventTs());
        ps.setLong(idx++, header.getIngestTs());
        ps.setLong(idx++, header.getKafkaTs());
        ps.setLong(idx++, header.getFlinkOutTs());
        ps.setString(idx++, header.getProbeId());
        ps.setString(idx++, tuple.getSrcIp());
        ps.setString(idx++, tuple.getDstIp());
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getSrcPort()));
        ps.setLong(idx++, Integer.toUnsignedLong(tuple.getDstPort()));
        ps.setInt(idx++, session.getProtocol());
        ps.setLong(idx++, session.getBytesTotal());
        ps.setFloat(idx++, session.getUpDownRatio());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getNumPkts()));
        ps.setFloat(idx++, session.getAvgPayload());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getMinPayload()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getMaxPayload()));
        ps.setFloat(idx++, session.getStdPayload());
        ps.setFloat(idx++, session.getMeanIatMs());
        ps.setFloat(idx++, session.getMinIatMs());
        ps.setFloat(idx++, session.getMaxIatMs());
        ps.setFloat(idx++, session.getStdIatMs());
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsSyn()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsAck()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsFin()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsPsh()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getFlagsRst()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getDnsPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getTcpPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getUdpPktCnt()));
        ps.setLong(idx++, Integer.toUnsignedLong(session.getIcmpPktCnt()));
        ps.setInt(idx++, session.getHasSyn() ? 1 : 0);
        ps.setInt(idx++, session.getHasFin() ? 1 : 0);
        ps.setInt(idx++, session.getHasRst() ? 1 : 0);
        ps.setInt(idx++, session.getIsEstablished() ? 1 : 0);
        ps.setLong(idx++, Integer.toUnsignedLong(session.getEvidenceCount()));
        ps.setObject(idx++, session.getFlowIdsList().toArray(new String[0]));
        ps.setString(idx++, session.getEndReason());
        ps.setString(idx++, header.getSchemaVersion());
        ps.setLong(idx++, header.getAggregateVersion());
        ps.setString(idx++, session.getIdentityVersion());
        ps.setLong(idx++, session.getSessionVersion());
        ps.setLong(idx++, session.getEventTimeStartMs());
        ps.setLong(idx++, session.getEventTimeEndMs());
        // SessionEvent v1 does not carry an operator watermark. Persist unknown,
        // never a fabricated event-end timestamp.
        ps.setNull(idx++, Types.BIGINT);
        ps.setObject(idx++, session.getSourceEventIdsList().toArray(new String[0]));
        ps.setObject(idx++, session.getEvidenceIdsList().toArray(new String[0]));
        ps.setString(idx++, session.getCompleteness().name());
        ps.setInt(idx++, session.getCompleteness()
                == com.traffic.proto.traffic.v1.SessionCompleteness.SESSION_COMPLETENESS_COMPLETE ? 0 : 1);
        ps.setObject(idx++, session.getMissingFieldsList().toArray(new String[0]));
    }
}
