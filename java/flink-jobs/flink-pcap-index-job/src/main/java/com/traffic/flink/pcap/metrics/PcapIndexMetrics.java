package com.traffic.flink.pcap.metrics;

import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;
import org.apache.flink.metrics.Histogram;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.runtime.metrics.DescriptiveStatisticsHistogram;

/**
 * PCAP Index Job 统一 Metrics 管理（增强版 v2）
 * 
 * 增强内容：
 * 1. ✅ 增加缺失字段 Metrics（missing_community_ids、missing_bloom_filter）
 * 2. ✅ 增加截断计数 Metrics（community_ids_truncated）
 * 3. ✅ 增加 Getter 方法（供外部访问）
 * 4. ✅ 优化 Metrics 命名（符合 Prometheus 规范）
 * 5. ✅ 增加详细注释
 */
public class PcapIndexMetrics {

    // ==================== Counters ====================
    
    // 处理计数
    private final Counter processedCounter;
    private final Counter errorCounter;
    private final Counter invalidCounter;
    
    // 业务计数
    private final Counter bytesProcessedCounter;
    private final Counter totalFilesCounter;
    private final Counter invalidTimeRangeCounter;
    private final Counter missingSha256Counter;
    private final Counter largeFileCounter;
    
    // ✅ 新增：缺失字段计数
    private final Counter missingCommunityIdsCounter;
    private final Counter missingBloomFilterCounter;
    
    // ✅ 新增：截断计数
    private final Counter communityIdsTruncatedCounter;
    
    // Sink 计数
    private final Counter clickhouseSuccessCounter;
    private final Counter clickhouseFailureCounter;
    private final Counter dlqWriteCounter;
    
    // ==================== Histograms ====================
    
    private final Histogram fileSizeHistogram;
    private final Histogram timeRangeHistogram;
    
    // ==================== Gauges ====================
    
    private volatile long lastProcessTime;

    /**
     * 构造函数
     *
     * @param metricGroup Flink MetricGroup
     */
    public PcapIndexMetrics(MetricGroup metricGroup) {
        // ==================== 处理计数 ====================
        this.processedCounter = metricGroup.counter("pcap_index_processed_total");
        this.errorCounter = metricGroup.counter("pcap_index_error_total");
        this.invalidCounter = metricGroup.counter("pcap_index_invalid_total");
        
        // ==================== 业务计数 ====================
        this.bytesProcessedCounter = metricGroup.counter("pcap_bytes_indexed_total");
        this.totalFilesCounter = metricGroup.counter("pcap_total_files_indexed");
        this.invalidTimeRangeCounter = metricGroup.counter("pcap_invalid_time_range_total");
        this.missingSha256Counter = metricGroup.counter("pcap_missing_sha256_total");
        this.largeFileCounter = metricGroup.counter("pcap_large_file_total");
        
        // ==================== 新增：缺失字段计数 ====================
        this.missingCommunityIdsCounter = metricGroup.counter("pcap_missing_community_ids_total");
        this.missingBloomFilterCounter = metricGroup.counter("pcap_missing_bloom_filter_total");
        
        // ==================== 新增：截断计数 ====================
        this.communityIdsTruncatedCounter = metricGroup.counter("pcap_community_ids_truncated_total");
        
        // ==================== Sink 计数 ====================
        this.clickhouseSuccessCounter = metricGroup.counter("clickhouse_write_success_total");
        this.clickhouseFailureCounter = metricGroup.counter("clickhouse_write_failure_total");
        this.dlqWriteCounter = metricGroup.counter("dlq_write_total");
        
        // ==================== Histograms ====================
        this.fileSizeHistogram = metricGroup.histogram(
                "pcap_file_size_bytes_distribution",
                new DescriptiveStatisticsHistogram(1000)
        );
        this.timeRangeHistogram = metricGroup.histogram(
                "pcap_time_range_seconds_distribution",
                new DescriptiveStatisticsHistogram(1000)
        );
        
        // ==================== Gauges ====================
        metricGroup.gauge("pcap_last_process_time", (Gauge<Long>) () -> lastProcessTime);
    }

    // ==================== Counter Methods ====================

    public void incProcessed() {
        processedCounter.inc();
    }

    public void incError() {
        errorCounter.inc();
    }

    public void incInvalid() {
        invalidCounter.inc();
    }

    public void incBytesProcessed(long bytes) {
        bytesProcessedCounter.inc(bytes);
    }

    public void incTotalFiles() {
        totalFilesCounter.inc();
    }

    public void incInvalidTimeRange() {
        invalidTimeRangeCounter.inc();
    }

    public void incMissingSha256() {
        missingSha256Counter.inc();
    }

    public void incLargeFile() {
        largeFileCounter.inc();
    }

    // ✅ 新增方法
    public void incMissingCommunityIds() {
        missingCommunityIdsCounter.inc();
    }

    // ✅ 新增方法
    public void incMissingBloomFilter() {
        missingBloomFilterCounter.inc();
    }

    // ✅ 新增方法
    public void incCommunityIdsTruncated() {
        communityIdsTruncatedCounter.inc();
    }

    public void incClickHouseSuccess() {
        clickhouseSuccessCounter.inc();
    }

    public void incClickHouseFailure() {
        clickhouseFailureCounter.inc();
    }

    public void incDlqWrite() {
        dlqWriteCounter.inc();
    }

    // ==================== Histogram Methods ====================

    public void recordFileSize(long bytes) {
        fileSizeHistogram.update(bytes);
    }

    public void recordTimeRange(long durationMs) {
        timeRangeHistogram.update(durationMs / 1000); // ms -> seconds
    }

    // ==================== Gauge Methods ====================

    public void updateLastProcessTime() {
        this.lastProcessTime = System.currentTimeMillis();
    }

    // ==================== Getters（新增）====================

    public Counter getProcessedCounter() {
        return processedCounter;
    }

    public Counter getErrorCounter() {
        return errorCounter;
    }

    public Counter getInvalidCounter() {
        return invalidCounter;
    }

    public Counter getTotalFilesCounter() {
        return totalFilesCounter;
    }

    public Counter getInvalidTimeRangeCounter() {
        return invalidTimeRangeCounter;
    }

    public Counter getMissingSha256Counter() {
        return missingSha256Counter;
    }

    public Counter getLargeFileCounter() {
        return largeFileCounter;
    }

    // ✅ 新增 Getter
    public Counter getMissingCommunityIdsCounter() {
        return missingCommunityIdsCounter;
    }

    // ✅ 新增 Getter
    public Counter getMissingBloomFilterCounter() {
        return missingBloomFilterCounter;
    }

    // ✅ 新增 Getter
    public Counter getCommunityIdsTruncatedCounter() {
        return communityIdsTruncatedCounter;
    }

    public Counter getClickhouseSuccessCounter() {
        return clickhouseSuccessCounter;
    }

    public Counter getClickhouseFailureCounter() {
        return clickhouseFailureCounter;
    }

    public Counter getDlqWriteCounter() {
        return dlqWriteCounter;
    }
}