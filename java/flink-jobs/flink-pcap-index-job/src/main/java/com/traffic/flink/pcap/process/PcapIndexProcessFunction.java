package com.traffic.flink.pcap.process;

import com.traffic.flink.pcap.metrics.PcapIndexMetrics;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Base64;

/**
 * PCAP Index 处理函数（完整版 v3）
 * 
 * 修复内容：
 * 1. ✅ 调用新增 Metrics（missing_community_ids、missing_bloom_filter）
 * 2. ✅ 完善统计日志（使用 Metrics Getter 输出完整统计）
 * 3. ✅ 优化日志输出频率（每 10 秒一次）
 * 4. ✅ 增加 Community IDs 空值处理
 * 5. ✅ 增加 BloomFilter 空值处理
 * 
 * 功能：
 * 1. 验证索引元数据完整性
 * 2. 增强业务 Metrics
 * 3. 侧输出无效数据到 DLQ
 */
public class PcapIndexProcessFunction extends ProcessFunction<PcapIndexMeta, PcapIndexMeta> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexProcessFunction.class);

    // ==================== 侧输出标签 ====================
    public static final OutputTag<String> DLQ_TAG = new OutputTag<String>("dlq-errors") {};

    // ==================== 可配置阈值 ====================
    private final long largeFileThresholdBytes;
    private final long maxTimeRangeMs;
    
    // ==================== 业务常量 ====================
    private static final int MAX_COMMUNITY_IDS = 1000; // DDL 注释建议值

    // ==================== Metrics ====================
    private transient PcapIndexMetrics metrics;
    private transient long lastLogTime;
    
    // ==================== 统计日志间隔（毫秒）====================
    private static final long STATS_LOG_INTERVAL_MS = 10_000; // 10 秒

    /**
     * 构造函数（支持外部化配置）
     *
     * @param largeFileThresholdBytes 大文件阈值（字节）
     * @param maxTimeRangeMs 最大时间范围（毫秒）
     */
    public PcapIndexProcessFunction(long largeFileThresholdBytes, long maxTimeRangeMs) {
        this.largeFileThresholdBytes = largeFileThresholdBytes;
        this.maxTimeRangeMs = maxTimeRangeMs;
    }

    /**
     * 默认构造函数（使用默认阈值）
     */
    public PcapIndexProcessFunction() {
        this(10L * 1024 * 1024 * 1024, 3600_000L); // 10GB, 1 hour
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        this.metrics = new PcapIndexMetrics(getRuntimeContext().getMetricGroup());
        this.lastLogTime = System.currentTimeMillis();
        
        LOG.info("PcapIndexProcessFunction initialized: " +
                "largeFileThreshold={} GB, maxTimeRange={} hours",
                largeFileThresholdBytes / (1024 * 1024 * 1024),
                maxTimeRangeMs / (3600 * 1000));
    }

    @Override
    public void processElement(
            PcapIndexMeta meta,
            Context ctx,
            Collector<PcapIndexMeta> out
    ) throws Exception {

        try {
            // ==================== 1. 验证数据完整性 ====================
            ValidationResult result = validateMeta(meta);
            
            if (!result.isValid()) {
                // 无效数据写入 DLQ
                metrics.incInvalid();
                String dlqMessage = buildDLQMessage(meta, result.getReason());
                ctx.output(DLQ_TAG, dlqMessage);
                metrics.incDlqWrite();
                
                LOG.warn("Invalid PCAP index: tenant={}, probe={}, file={}, reason={}",
                        meta.getTenantId(), meta.getProbeId(), meta.getFileKey(),
                        result.getReason());
                return;
            }

            // ==================== 2. 记录业务 Metrics ====================
            metrics.incProcessed();
            metrics.incBytesProcessed(meta.getByteSize());
            metrics.incTotalFiles();
            metrics.recordFileSize(meta.getByteSize());
            
            long timeRangeMs = meta.getTsEnd() - meta.getTsStart();
            metrics.recordTimeRange(timeRangeMs);

            // ==================== 3. 业务告警检查 ====================
            
            // 3.1 超大文件告警
            if (meta.getByteSize() > largeFileThresholdBytes) {
                metrics.incLargeFile();
                LOG.warn("Large PCAP file detected: tenant={}, probe={}, file={}, size={}",
                        meta.getTenantId(), meta.getProbeId(), meta.getFileKey(),
                        formatBytes(meta.getByteSize()));
            }

            // 3.2 时间范围过长告警（可能的异常数据）
            if (timeRangeMs > maxTimeRangeMs) {
                LOG.warn("Suspicious long time range: {} ms for file {}, tenant={}, probe={}",
                        timeRangeMs, meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
            }

            // 3.3 Community IDs 数量检查（业务告警）
            if (meta.getCommunityIdsCount() > MAX_COMMUNITY_IDS) {
                LOG.warn("Suspicious large community_ids count: {} for file {}, tenant={}, probe={} " +
                                "(will be truncated in Sink)",
                        meta.getCommunityIdsCount(), meta.getFileKey(),
                        meta.getTenantId(), meta.getProbeId());
                // 注意：实际截断在 ClickHousePcapSinkFactory 中处理
            }

            // ==================== 4. 更新处理时间 ====================
            metrics.updateLastProcessTime();

            // ==================== 5. 输出有效数据 ====================
            out.collect(meta);

            // ==================== 6. 定期打印统计 ====================
            logStatsIfNeeded();

        } catch (Exception e) {
            metrics.incError();
            LOG.error("Error processing PCAP index: tenant={}, probe={}, file={}, error={}",
                    meta.getTenantId(), meta.getProbeId(), meta.getFileKey(),
                    e.getMessage(), e);
            
            // 异常也写入 DLQ
            String dlqMessage = buildDLQMessage(meta, "Exception: " + e.getMessage());
            ctx.output(DLQ_TAG, dlqMessage);
            metrics.incDlqWrite();
        }
    }

    /**
     * 验证索引元数据（增强版 v3）
     */
    private ValidationResult validateMeta(PcapIndexMeta meta) {
        // ==================== 1. 必要字段检查 ====================
        if (meta.getTenantId() == null || meta.getTenantId().isEmpty()) {
            return ValidationResult.invalid("Missing tenant_id");
        }
        if (meta.getProbeId() == null || meta.getProbeId().isEmpty()) {
            return ValidationResult.invalid("Missing probe_id");
        }
        if (meta.getFileKey() == null || meta.getFileKey().isEmpty()) {
            return ValidationResult.invalid("Missing file_key");
        }

        // ==================== 2. 时间范围检查 ====================
        if (meta.getTsStart() <= 0) {
            return ValidationResult.invalid("Invalid ts_start: " + meta.getTsStart());
        }
        if (meta.getTsEnd() < meta.getTsStart()) {
            metrics.incInvalidTimeRange();
            return ValidationResult.invalid(
                    String.format("Invalid time range: ts_start=%d, ts_end=%d",
                            meta.getTsStart(), meta.getTsEnd())
            );
        }

        // ==================== 3. 字节大小检查 ====================
        if (meta.getByteSize() <= 0) {
            return ValidationResult.invalid("Invalid byte_size: " + meta.getByteSize());
        }

        // ==================== 4. SHA256 检查（警告级别，不拒绝）====================
        if (meta.getSha256() == null || meta.getSha256().isEmpty()) {
            metrics.incMissingSha256();
            LOG.debug("Missing SHA256 for file: {}, tenant={}, probe={}",
                    meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
        }

        // ==================== 5. Community IDs 检查（新增，调用新 Metrics）====================
        if (meta.getCommunityIdsCount() == 0) {
            // ✅ 调用新增 Metrics
            metrics.incMissingCommunityIds();
            
            // 警告级别：允许通过，但记录日志
            LOG.warn("Missing community_ids for file: {}, tenant={}, probe={}, " +
                            "this may impact forensics capability",
                    meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
        } else if (meta.getCommunityIdsCount() > MAX_COMMUNITY_IDS) {
            // 超过限制：仅警告（截断在 Sink 层处理）
            LOG.debug("Community IDs count ({}) exceeds limit ({}), will be truncated in Sink: " +
                            "file={}, tenant={}, probe={}",
                    meta.getCommunityIdsCount(), MAX_COMMUNITY_IDS,
                    meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
        }

        // ==================== 6. BloomFilter 检查（新增，调用新 Metrics）====================
        if (meta.getBloomFilterB64() == null || meta.getBloomFilterB64().isEmpty()) {
            // ✅ 调用新增 Metrics
            metrics.incMissingBloomFilter();
            
            // 警告级别：允许通过，但记录日志
            LOG.warn("Missing bloom_filter_b64 for file: {}, tenant={}, probe={}, " +
                            "IP fast lookup will be unavailable",
                    meta.getFileKey(), meta.getTenantId(), meta.getProbeId());
        }

        // ==================== 7. 所有检查通过 ====================
        return ValidationResult.valid();
    }

    /**
     * 构造 DLQ 消息（JSON 格式，增强版）
     */
    private String buildDLQMessage(PcapIndexMeta meta, String reason) {
        String rawEventBase64 = Base64.getEncoder().encodeToString(meta.toByteArray());
        
        return String.format(
                "{\"tenant_id\":\"%s\"," +
                        "\"probe_id\":\"%s\"," +
                        "\"file_key\":\"%s\"," +
                        "\"ts_start\":%d," +
                        "\"ts_end\":%d," +
                        "\"byte_size\":%d," +
                        "\"community_ids_count\":%d," +
                        "\"has_bloom_filter\":%b," +
                        "\"has_sha256\":%b," +
                        "\"reason\":\"%s\"," +
                        "\"timestamp\":%d," +
                        "\"raw_event_base64\":\"%s\"}",
                escapeJson(meta.getTenantId()),
                escapeJson(meta.getProbeId()),
                escapeJson(meta.getFileKey()),
                meta.getTsStart(),
                meta.getTsEnd(),
                meta.getByteSize(),
                meta.getCommunityIdsCount(),
                meta.getBloomFilterB64() != null && !meta.getBloomFilterB64().isEmpty(),
                meta.getSha256() != null && !meta.getSha256().isEmpty(),
                escapeJson(reason),
                System.currentTimeMillis(),
                rawEventBase64
        );
    }

    /**
     * 转义 JSON 字符串
     */
    private String escapeJson(String input) {
        if (input == null) {
            return "";
        }
        return input.replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                .replace("\t", "\\t");
    }

    /**
     * 格式化字节大小
     */
    private String formatBytes(long bytes) {
        if (bytes < 1024) {
            return bytes + " B";
        } else if (bytes < 1024 * 1024) {
            return String.format("%.2f KB", bytes / 1024.0);
        } else if (bytes < 1024 * 1024 * 1024) {
            return String.format("%.2f MB", bytes / (1024.0 * 1024));
        } else {
            return String.format("%.2f GB", bytes / (1024.0 * 1024 * 1024));
        }
    }

    /**
     * 定期打印统计信息（完整版，使用 Metrics Getter）
     */
    private void logStatsIfNeeded() {
        long now = System.currentTimeMillis();
        if (now - lastLogTime > STATS_LOG_INTERVAL_MS) {
            LOG.info("========== PCAP Index Processing Stats ==========");
            LOG.info("  Processed:               {}", metrics.getProcessedCounter().getCount());
            LOG.info("  Invalid:                 {}", metrics.getInvalidCounter().getCount());
            LOG.info("  Error:                   {}", metrics.getErrorCounter().getCount());
            LOG.info("  Total Files:             {}", metrics.getTotalFilesCounter().getCount());
            LOG.info("  Invalid Time Range:      {}", metrics.getInvalidTimeRangeCounter().getCount());
            LOG.info("  Missing SHA256:          {}", metrics.getMissingSha256Counter().getCount());
            LOG.info("  Missing Community IDs:   {}", metrics.getMissingCommunityIdsCounter().getCount());
            LOG.info("  Missing BloomFilter:     {}", metrics.getMissingBloomFilterCounter().getCount());
            LOG.info("  Large File:              {}", metrics.getLargeFileCounter().getCount());
            LOG.info("  DLQ Write:               {}", metrics.getDlqWriteCounter().getCount());
            LOG.info("  ClickHouse Success:      {}", metrics.getClickhouseSuccessCounter().getCount());
            LOG.info("  ClickHouse Failure:      {}", metrics.getClickhouseFailureCounter().getCount());
            LOG.info("==================================================");
            
            lastLogTime = now;
        }
    }

    /**
     * 验证结果（内部类）
     */
    private static class ValidationResult {
        private final boolean valid;
        private final String reason;

        private ValidationResult(boolean valid, String reason) {
            this.valid = valid;
            this.reason = reason;
        }

        public static ValidationResult valid() {
            return new ValidationResult(true, null);
        }

        public static ValidationResult invalid(String reason) {
            return new ValidationResult(false, reason);
        }

        public boolean isValid() {
            return valid;
        }

        public String getReason() {
            return reason;
        }
    }
}