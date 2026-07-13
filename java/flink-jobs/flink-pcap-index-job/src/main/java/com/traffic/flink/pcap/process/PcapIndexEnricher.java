package com.traffic.flink.pcap.process;

import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.api.common.functions.RichMapFunction;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * PCAP 索引增强处理（已废弃，使用 PcapIndexProcessFunction 替代）
 * 
 * @deprecated 使用 {@link PcapIndexProcessFunction} 替代，该类提供更完整的业务验证逻辑和 DLQ 支持
 * 
 * 废弃原因：
 * 1. 缺少完整的业务验证（如 Community IDs 检查、BloomFilter 检查）
 * 2. 缺少 DLQ 侧输出支持（无法隔离坏数据）
 * 3. Metrics 不完整（缺少新增的 missing_community_ids、missing_bloom_filter 等）
 * 
 * 迁移指南：
 * 替换代码：
 *   // 旧代码
 *   dataStream.map(new PcapIndexEnricher())
 * 
 *   // 新代码
 *   dataStream.process(new PcapIndexProcessFunction(maxFileSizeGB, maxTimeRangeHours))
 * 
 * 并获取 DLQ 侧输出流：
 *   DataStream<String> dlqStream = processedStream.getSideOutput(PcapIndexProcessFunction.DLQ_TAG);
 *   dlqStream.sinkTo(DLQSinkFactory.createDLQSink(brokers, dlqTopic));
 */
@Deprecated
public class PcapIndexEnricher extends RichMapFunction<PcapIndexMeta, PcapIndexMeta> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexEnricher.class);

    // Metrics
    private transient Counter processedCounter;
    private transient Counter bytesProcessed;
    private transient Counter errorCounter;
    private transient volatile long lastProcessTime;
    private transient volatile long totalFiles;

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        LOG.warn("PcapIndexEnricher is DEPRECATED. Please use PcapIndexProcessFunction instead.");

        // 注册 Metrics
        processedCounter = getRuntimeContext()
                .getMetricGroup()
                .counter("pcap_index_processed_total");

        bytesProcessed = getRuntimeContext()
                .getMetricGroup()
                .counter("pcap_bytes_indexed_total");

        errorCounter = getRuntimeContext()
                .getMetricGroup()
                .counter("pcap_index_error_total");

        getRuntimeContext()
                .getMetricGroup()
                .gauge("pcap_last_process_time", (Gauge<Long>) () -> lastProcessTime);

        getRuntimeContext()
                .getMetricGroup()
                .gauge("pcap_total_files_indexed", (Gauge<Long>) () -> totalFiles);

        LOG.info("PcapIndexEnricher initialized (DEPRECATED)");
    }

    @Override
    public PcapIndexMeta map(PcapIndexMeta meta) throws Exception {
        try {
            // 验证必要字段
            validateMeta(meta);

            // 更新指标
            processedCounter.inc();
            bytesProcessed.inc(meta.getByteSize());
            lastProcessTime = System.currentTimeMillis();
            totalFiles++;

            // 日志记录（每 1000 条记录一次）
            if (totalFiles % 1000 == 0) {
                LOG.info("Processed {} PCAP index records, latest: tenant={}, probe={}, file={}, size={}",
                        totalFiles,
                        meta.getTenantId(),
                        meta.getProbeId(),
                        meta.getFileKey(),
                        formatBytes(meta.getByteSize()));
            }

            return meta;

        } catch (Exception e) {
            errorCounter.inc();
            LOG.error("Error processing PCAP index: {}", e.getMessage(), e);
            throw e;
        }
    }

    /**
     * 验证索引元数据（简化版，仅基础检查）
     */
    private void validateMeta(PcapIndexMeta meta) {
        if (meta.getTsEnd() < meta.getTsStart()) {
            LOG.warn("Invalid time range: ts_start={}, ts_end={}, file={}",
                    meta.getTsStart(), meta.getTsEnd(), meta.getFileKey());
        }

        if (meta.getByteSize() <= 0) {
            LOG.warn("Invalid byte size: {}, file={}", 
                    meta.getByteSize(), meta.getFileKey());
        }

        if (meta.getSha256() == null || meta.getSha256().isEmpty()) {
            LOG.debug("Missing SHA256 for file: {}", meta.getFileKey());
        }
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
}