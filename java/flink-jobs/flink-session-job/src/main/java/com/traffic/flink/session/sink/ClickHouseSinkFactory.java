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
import java.sql.Timestamp;

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
            (PreparedStatement ps, SessionEvent session) -> {
                try {
                    int idx = 1;
                    
                    // 租户与标识
                    ps.setString(idx++, session.getHeader().getTenantId());
                    ps.setString(idx++, session.getHeader().getRunId());
                    ps.setString(idx++, session.getHeader().getFeatureSetId());
                    ps.setString(idx++, session.getHeader().getEventId());
                    
                    // Session 标识
                    ps.setString(idx++, session.getSessionId());
                    ps.setString(idx++, session.getCommunityId());
                    
                    // 时间范围
                    ps.setTimestamp(idx++, new Timestamp(session.getTsStart()));
                    ps.setTimestamp(idx++, new Timestamp(session.getTsEnd()));
                    ps.setInt(idx++, session.getDurationMs());
                    
                    // 网络信息
                    ps.setInt(idx++, session.getProtocol());
                    ps.setString(idx++, session.getClientIp());
                    ps.setString(idx++, session.getServerIp());
                    ps.setInt(idx++, session.getClientPort());
                    ps.setInt(idx++, session.getServerPort());
                    
                    // 流量统计
                    ps.setLong(idx++, session.getPacketsTotal());
                    ps.setLong(idx++, session.getBytesTotal());
                    
                    // ✅ 修复：bytes_up = bytesFwd（已在 SessionAggregator 中映射为 client→server）
                    // ✅ 修复：bytes_down = bytesBwd（已在 SessionAggregator 中映射为 server→client）
                    ps.setLong(idx++, session.getBytesFwd());   // bytes_up
                    ps.setLong(idx++, session.getBytesBwd());   // bytes_down
                    ps.setFloat(idx++, session.getUpDownRatio());
                    
                    // 包长统计
                    ps.setInt(idx++, session.getNumPkts());
                    ps.setFloat(idx++, session.getAvgPayload());
                    ps.setInt(idx++, session.getMinPayload());
                    ps.setInt(idx++, session.getMaxPayload());
                    ps.setFloat(idx++, session.getStdPayload());
                    
                    // IAT 统计
                    ps.setFloat(idx++, session.getMeanIatMs());
                    ps.setFloat(idx++, session.getMinIatMs());
                    ps.setFloat(idx++, session.getMaxIatMs());
                    ps.setFloat(idx++, session.getStdIatMs());
                    
                    // TCP 标志（修复后输出为 0/1）
                    ps.setInt(idx++, session.getFlagsSyn());
                    ps.setInt(idx++, session.getFlagsAck());
                    ps.setInt(idx++, session.getFlagsFin());
                    ps.setInt(idx++, session.getFlagsPsh());
                    ps.setInt(idx++, session.getFlagsRst());
                    
                    // 协议统计
                    ps.setInt(idx++, session.getDnsPktCnt());
                    ps.setInt(idx++, session.getTcpPktCnt());
                    ps.setInt(idx++, session.getUdpPktCnt());
                    ps.setInt(idx++, session.getIcmpPktCnt());
                    
                    // 证据数量
                    ps.setInt(idx++, session.getEvidenceCount());
                    
                    // 摄入时间戳
                    ps.setTimestamp(idx++, new Timestamp(session.getHeader().getIngestTs()));
                    
                } catch (SQLException e) {
                    LOG.error("Error setting parameters for session_id={}, event_id={}: {}", 
                            session.getSessionId(), 
                            session.getHeader().getEventId(), 
                            e.getMessage(), e);
                    throw e;
                }
            },
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
    private static String buildInsertSql(String table) {
        return String.format(
            "INSERT INTO %s (" +
                // 租户与标识
                "tenant_id, run_id, feature_set_id, event_id, " +
                // Session 标识
                "session_id, community_id, " +
                // 时间范围
                "ts_start, ts_end, duration_ms, " +
                // 网络信息
                "protocol, client_ip, server_ip, client_port, server_port, " +
                // 流量统计（bytes_up = client→server, bytes_down = server→client）
                "packets_total, bytes_total, bytes_up, bytes_down, up_down_ratio, " +
                // 包长统计
                "num_pkts, avg_payload, min_payload, max_payload, std_payload, " +
                // IAT 统计
                "mean_iat_ms, min_iat_ms, max_iat_ms, std_iat_ms, " +
                // TCP 标志（0/1 presence）
                "flags_syn, flags_ack, flags_fin, flags_psh, flags_rst, " +
                // 协议统计
                "dns_pkt_cnt, tcp_pkt_cnt, udp_pkt_cnt, icmp_pkt_cnt, " +
                // 证据数量
                "evidence_count, " +
                // 摄入时间戳
                "ingest_ts" +
            ") VALUES (" +
                "?, ?, ?, ?, " +  // tenant_id, run_id, feature_set_id, event_id
                "?, ?, " +         // session_id, community_id
                "?, ?, ?, " +      // ts_start, ts_end, duration_ms
                "?, ?, ?, ?, ?, " + // protocol, client_ip, server_ip, client_port, server_port
                "?, ?, ?, ?, ?, " + // packets_total, bytes_total, bytes_up, bytes_down, up_down_ratio
                "?, ?, ?, ?, ?, " + // num_pkts, avg_payload, min_payload, max_payload, std_payload
                "?, ?, ?, ?, " +    // mean_iat_ms, min_iat_ms, max_iat_ms, std_iat_ms
                "?, ?, ?, ?, ?, " + // flags_syn, flags_ack, flags_fin, flags_psh, flags_rst
                "?, ?, ?, ?, " +    // dns_pkt_cnt, tcp_pkt_cnt, udp_pkt_cnt, icmp_pkt_cnt
                "?, " +             // evidence_count
                "?" +               // ingest_ts
            ")",
            table
        );
    }
}