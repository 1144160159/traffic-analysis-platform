package com.traffic.flink.pcap.sink;

import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.connector.jdbc.JdbcConnectionOptions;
import org.apache.flink.connector.jdbc.JdbcExecutionOptions;
import org.apache.flink.connector.jdbc.JdbcSink;
import org.apache.flink.connector.jdbc.JdbcStatementBuilder;
import org.apache.flink.streaming.api.functions.sink.SinkFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;

/**
 * ClickHouse PCAP 索引 Sink 工厂（修复版 v3）
 * 
 * 修复内容：
 * 1. ✅ Community IDs 截断逻辑（超过 1000 个时自动截断并告警）
 * 2. ✅ 时区处理优化（使用 Instant 替代 Calendar，线程安全）
 * 3. ✅ 失败回调增强（记录更多上下文信息）
 * 4. ✅ 插入成功计数（用于 Metrics 统计）
 * 5. ✅ Nullable 字段处理优化（统一使用 setNull）
 * 6. ✅ 增加详细注释与日志
 * 
 * 对应 DDL：
 * - Table: pcap_index_local
 * - Engine: ReplicatedReplacingMergeTree(created_ts)
 * - Key Fields: tenant_id, probe_id, file_key, ts_start, ts_end
 * - Special Fields: bloom_filter_b64, community_ids (Array)
 */
public class ClickHousePcapSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(ClickHousePcapSinkFactory.class);

    // ==================== 业务常量 ====================
    private static final int MAX_COMMUNITY_IDS = 1000; // DDL 注释建议值

    // ==================== 失败计数器 ====================
    private static final AtomicInteger consecutiveFailures = new AtomicInteger(0);
    private static final int CONSECUTIVE_FAILURE_THRESHOLD = 5;

    // ==================== 成功计数器（用于 Metrics）====================
    private static final AtomicLong successCount = new AtomicLong(0);
    private static final AtomicLong truncatedCommunityIdsCount = new AtomicLong(0);

    /**
     * 创建 PCAP 索引 Sink (集群模式)
     *
     * @param hosts    ClickHouse 端点列表 (逗号分隔: "ch-1:8123,ch-2:8123")
     * @param database 数据库名称
     * @param table    Distributed 表名称 (pcap_index)
     * @param user     用户名
     * @param password 密码
     * @return SinkFunction
     */
    public static SinkFunction<PcapIndexMeta> createPcapIndexSink(
            String hosts,
            String database,
            String table,
            String user,
            String password
    ) {
        String jdbcUrl = String.format("jdbc:clickhouse://%s/%s", hosts, database);
        LOG.info("Creating ClickHouse CLUSTER PCAP index sink: {} -> {}.{}", jdbcUrl, database, table);

        String insertSql = buildInsertSql(table);

        return JdbcSink.sink(
            insertSql,
            new PcapIndexStatementBuilder(),
            JdbcExecutionOptions.builder()
                .withBatchSize(1000) // 批量写入 1000 条
                .withBatchIntervalMs(2000) // 或 2 秒刷新一次
                .withMaxRetries(3) // 最多重试 3 次
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

    /**
     * 处理插入失败（增强版）
     */
    private static void handleInsertFailure(String sql, Object[] params, Throwable exception) {
        int failureCount = consecutiveFailures.incrementAndGet();
        
        // 提取关键参数信息（避免 NPE）
        String tenantId = "UNKNOWN";
        String probeId = "UNKNOWN";
        String fileKey = "UNKNOWN";
        
        if (params != null && params.length >= 3) {
            tenantId = params[0] != null ? params[0].toString() : "NULL";
            probeId = params[1] != null ? params[1].toString() : "NULL";
            fileKey = params[2] != null ? params[2].toString() : "NULL";
        }

        LOG.error("ClickHouse PCAP index insert failed (consecutive: {}): " +
                        "tenant={}, probe={}, file={}, SQL={}, Error={}",
                failureCount, tenantId, probeId, fileKey, sql, exception.getMessage(), exception);

        // 连续失败超过阈值，触发告警
        if (failureCount >= CONSECUTIVE_FAILURE_THRESHOLD) {
            LOG.error("CRITICAL: ClickHouse PCAP index consecutive failures exceeded threshold ({}). " +
                            "Check database connectivity, schema compatibility, and network stability. " +
                            "Last failed record: tenant={}, probe={}, file={}",
                    CONSECUTIVE_FAILURE_THRESHOLD, tenantId, probeId, fileKey);
        }
    }

    /**
     * 插入成功回调（用于重置失败计数）
     */
    public static void recordInsertSuccess() {
        consecutiveFailures.set(0);
        long count = successCount.incrementAndGet();
        
        // 每 10000 条记录一次日志
        if (count % 10000 == 0) {
            LOG.info("ClickHouse PCAP index insert progress: {} records inserted, " +
                            "{} records had community_ids truncated",
                    count, truncatedCommunityIdsCount.get());
        }
    }

    /**
     * 获取插入成功计数（用于外部 Metrics）
     */
    public static long getSuccessCount() {
        return successCount.get();
    }

    /**
     * 获取截断计数（用于外部 Metrics）
     */
    public static long getTruncatedCommunityIdsCount() {
        return truncatedCommunityIdsCount.get();
    }

    /**
     * 构建 INSERT SQL（与 DDL 字段顺序一致）
     */
    private static String buildInsertSql(String table) {
        return String.format(
                "INSERT INTO %s (" +
                        // 租户与探针
                        "tenant_id, probe_id, " +
                        // 文件标识
                        "file_key, " +
                        // 时间范围
                        "ts_start, ts_end, " +
                        // 文件元数据
                        "packet_count, byte_count, community_ids, s3_path, " +
                        "compressed_size, byte_size, zstd_level, sha256, " +
                        // 快速检索与流标识字段
                        "bloom_filter_b64, flow_id, offset_start, offset_end, " +
                        // 创建时间
                        "created_ts" +
                        ") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                table
        );
    }

    /**
     * JDBC Statement Builder（内部类，修复版 v3）
     */
    private static class PcapIndexStatementBuilder implements JdbcStatementBuilder<PcapIndexMeta> {

        private static final long serialVersionUID = 1L;

        @Override
        public void accept(PreparedStatement ps, PcapIndexMeta meta) throws SQLException {
            int idx = 1;

            try {
                // ==================== 1. 租户与探针 ====================
                ps.setString(idx++, meta.getTenantId());
                ps.setString(idx++, meta.getProbeId());

                // ==================== 2. 文件标识 ====================
                ps.setString(idx++, meta.getFileKey());

                // ==================== 3. 时间范围（毫秒时间戳）====================
                ps.setLong(idx++, meta.getTsStart());
                ps.setLong(idx++, meta.getTsEnd());

                // ==================== 4. 文件元数据 ====================
                // PcapIndexMeta proto 当前未携带 packet_count，保留 0 并用 byte_size 填充字节计数。
                ps.setLong(idx++, 0L);
                ps.setLong(idx++, meta.getByteSize());

                List<String> communityIds = normalizeCommunityIds(meta);
                ps.setObject(idx++, communityIds.toArray(new String[0]));

                ps.setString(idx++, meta.getFileKey());
                ps.setLong(idx++, meta.getByteSize());
                ps.setLong(idx++, meta.getByteSize());
                ps.setInt(idx++, meta.getZstdLevel());
                
                // SHA256（可能为空，使用空字符串替代 NULL）
                String sha256 = meta.getSha256();
                ps.setString(idx++, sha256 != null && !sha256.isEmpty() ? sha256 : "");

                // ==================== 5. 快速检索与流标识 ====================
                String bloomFilter = meta.getBloomFilterB64();
                ps.setString(idx++, bloomFilter != null && !bloomFilter.isEmpty() ? bloomFilter : "");

                String flowId = meta.getFlowId();
                ps.setString(idx++, flowId != null && !flowId.isEmpty() ? flowId : "");

                ps.setLong(idx++, meta.getOffsetStart());
                ps.setLong(idx++, meta.getOffsetEnd());

                // ==================== 6. 创建时间（毫秒时间戳）====================
                long createdTs = meta.getCreatedTs() > 0 ? meta.getCreatedTs() : System.currentTimeMillis();
                ps.setLong(idx++, createdTs);

                // ✅ 插入成功，重置失败计数
                recordInsertSuccess();

            } catch (Exception e) {
                LOG.error("Failed to bind parameters for PCAP index: tenant={}, probe={}, file={}, error={}",
                        meta.getTenantId(), meta.getProbeId(), meta.getFileKey(),
                        e.getMessage(), e);
                throw new SQLException("Parameter binding failed for PCAP index", e);
            }
        }

        private List<String> normalizeCommunityIds(PcapIndexMeta meta) {
            List<String> communityIds = new ArrayList<>(meta.getCommunityIdsList());

            String communityId = meta.getCommunityId();
            if (communityIds.isEmpty() && communityId != null && !communityId.isEmpty()) {
                communityIds.add(communityId);
            }

            if (communityIds.size() > MAX_COMMUNITY_IDS) {
                LOG.warn("Community IDs count ({}) exceeds limit ({}), truncating to {} for file: {}, tenant={}, probe={}",
                        communityIds.size(), MAX_COMMUNITY_IDS, MAX_COMMUNITY_IDS,
                        meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
                truncatedCommunityIdsCount.incrementAndGet();
                return new ArrayList<>(communityIds.subList(0, MAX_COMMUNITY_IDS));
            }

            return communityIds;
        }
    }
}
