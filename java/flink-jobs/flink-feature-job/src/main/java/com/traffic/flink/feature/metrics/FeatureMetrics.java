package com.traffic.flink.feature.metrics;

import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Histogram;
import org.apache.flink.metrics.Meter;
import org.apache.flink.metrics.MetricGroup;
import org.apache.flink.runtime.metrics.DescriptiveStatisticsHistogram;

/**
 * Feature Job 统一 Metrics 管理（增强版 v2）
 * 
 * 增强内容（P1）：
 * 1. ✅ 业务 Metrics（zero_packets/high_pps/encrypted/l2_triggered）
 * 2. ✅ 端到端延迟 Histogram
 * 3. ✅ ClickHouse/Kafka 写入成功/失败计数
 * 4. ✅ DLQ 写入计数
 */
public class FeatureMetrics {

    // ==================== Counters ====================
    
    // 处理计数
    private final Counter processedCounter;
    private final Counter errorCounter;
    private final Counter skippedCounter;

    // 业务特征计数（✅ 新增）
    private final Counter zeroPacketsCounter;
    private final Counter highPpsCounter;
    private final Counter highBpsCounter;
    private final Counter encryptedCounter;
    private final Counter l2TriggeredCounter;

    // Sink 计数
    private final Counter clickhouseSuccessCounter;
    private final Counter clickhouseFailureCounter;
    private final Counter kafkaSuccessCounter;
    private final Counter kafkaFailureCounter;
    private final Counter dlqWriteCounter;

    // ==================== Histograms ====================
    
    // 特征分布
    private final Histogram featureDurationHistogram;
    private final Histogram ppsHistogram;
    private final Histogram bpsHistogram;
    private final Histogram upDownRatioHistogram;

    // 延迟分布（✅ 新增）
    private final Histogram e2eLatencyHistogram;
    private final Histogram clickhouseWriteLatencyHistogram;
    private final Histogram kafkaWriteLatencyHistogram;

    // ==================== Meters ====================
    
    private final Meter processingRate;
    private final Meter errorRate;

    public FeatureMetrics(MetricGroup metricGroup) {
        // ==================== 初始化 Counters ====================
        
        // 处理计数
        this.processedCounter = metricGroup.counter("feature_processed_total");
        this.errorCounter = metricGroup.counter("feature_error_total");
        this.skippedCounter = metricGroup.counter("feature_skipped_total");

        // 业务特征计数
        this.zeroPacketsCounter = metricGroup.counter("feature_zero_packets_total");
        this.highPpsCounter = metricGroup.counter("feature_high_pps_total");
        this.highBpsCounter = metricGroup.counter("feature_high_bps_total");
        this.encryptedCounter = metricGroup.counter("feature_encrypted_total");
        this.l2TriggeredCounter = metricGroup.counter("feature_l2_triggered_total");

        // Sink 计数
        this.clickhouseSuccessCounter = metricGroup.counter("clickhouse_write_success_total");
        this.clickhouseFailureCounter = metricGroup.counter("clickhouse_write_failure_total");
        this.kafkaSuccessCounter = metricGroup.counter("kafka_write_success_total");
        this.kafkaFailureCounter = metricGroup.counter("kafka_write_failure_total");
        this.dlqWriteCounter = metricGroup.counter("dlq_write_total");

        // ==================== 初始化 Histograms ====================
        
        // 特征分布
        this.featureDurationHistogram = metricGroup.histogram(
                "feature_duration_ms",
                new DescriptiveStatisticsHistogram(1000)
        );
        this.ppsHistogram = metricGroup.histogram(
                "feature_pps_distribution",
                new DescriptiveStatisticsHistogram(1000)
        );
        this.bpsHistogram = metricGroup.histogram(
                "feature_bps_distribution",
                new DescriptiveStatisticsHistogram(1000)
        );
        this.upDownRatioHistogram = metricGroup.histogram(
                "feature_up_down_ratio_distribution",
                new DescriptiveStatisticsHistogram(1000)
        );

        // 延迟分布
        this.e2eLatencyHistogram = metricGroup.histogram(
                "e2e_latency_ms",
                new DescriptiveStatisticsHistogram(1000)
        );
        this.clickhouseWriteLatencyHistogram = metricGroup.histogram(
                "clickhouse_write_latency_ms",
                new DescriptiveStatisticsHistogram(500)
        );
        this.kafkaWriteLatencyHistogram = metricGroup.histogram(
                "kafka_write_latency_ms",
                new DescriptiveStatisticsHistogram(500)
        );

        // ==================== 初始化 Meters ====================
        
        this.processingRate = metricGroup.meter(
                "processing_rate",
                new org.apache.flink.metrics.MeterView(processedCounter, 60)
        );
        this.errorRate = metricGroup.meter(
                "error_rate",
                new org.apache.flink.metrics.MeterView(errorCounter, 60)
        );
    }

    // ==================== Counter Methods ====================

    public void incProcessed() {
        processedCounter.inc();
    }

    public void incError() {
        errorCounter.inc();
    }

    public void incSkipped() {
        skippedCounter.inc();
    }

    // 业务特征计数（✅ 新增）
    public void incZeroPackets() {
        zeroPacketsCounter.inc();
    }

    public void incHighPps() {
        highPpsCounter.inc();
    }

    public void incHighBps() {
        highBpsCounter.inc();
    }

    public void incEncrypted() {
        encryptedCounter.inc();
    }

    public void incL2Triggered() {
        l2TriggeredCounter.inc();
    }

    // Sink 计数
    public void incClickHouseSuccess() {
        clickhouseSuccessCounter.inc();
    }

    public void incClickHouseFailure() {
        clickhouseFailureCounter.inc();
    }

    public void incKafkaSuccess() {
        kafkaSuccessCounter.inc();
    }

    public void incKafkaFailure() {
        kafkaFailureCounter.inc();
    }

    public void incDlqWrite() {
        dlqWriteCounter.inc();
    }

    // ==================== Histogram Methods ====================

    public void recordFeatureDuration(long durationNs) {
        featureDurationHistogram.update(durationNs / 1_000_000); // ns -> ms
    }

    public void recordPPS(float pps) {
        if (Float.isFinite(pps)) {
            ppsHistogram.update((long) pps);
        }
    }

    public void recordBPS(float bps) {
        if (Float.isFinite(bps)) {
            bpsHistogram.update((long) bps);
        }
    }

    public void recordUpDownRatio(float ratio) {
        if (Float.isFinite(ratio) && ratio < Float.MAX_VALUE) {
            upDownRatioHistogram.update((long) (ratio * 100)); // 放大 100 倍记录
        }
    }

    // 延迟 Histogram（✅ 新增）
    public void recordE2ELatency(long latencyMs) {
        e2eLatencyHistogram.update(latencyMs);
    }

    public void recordClickHouseWriteLatency(long latencyMs) {
        clickhouseWriteLatencyHistogram.update(latencyMs);
    }

    public void recordKafkaWriteLatency(long latencyMs) {
        kafkaWriteLatencyHistogram.update(latencyMs);
    }

    // ==================== Getters ====================

    public Counter getProcessedCounter() {
        return processedCounter;
    }

    public Counter getErrorCounter() {
        return errorCounter;
    }

    public Counter getSkippedCounter() {
        return skippedCounter;
    }

    public Counter getZeroPacketsCounter() {
        return zeroPacketsCounter;
    }

    public Counter getHighPpsCounter() {
        return highPpsCounter;
    }

    public Counter getHighBpsCounter() {
        return highBpsCounter;
    }

    public Counter getEncryptedCounter() {
        return encryptedCounter;
    }

    public Counter getL2TriggeredCounter() {
        return l2TriggeredCounter;
    }

    public Counter getClickhouseSuccessCounter() {
        return clickhouseSuccessCounter;
    }

    public Counter getClickhouseFailureCounter() {
        return clickhouseFailureCounter;
    }

    public Counter getKafkaSuccessCounter() {
        return kafkaSuccessCounter;
    }

    public Counter getKafkaFailureCounter() {
        return kafkaFailureCounter;
    }

    public Counter getDlqWriteCounter() {
        return dlqWriteCounter;
    }

    public Histogram getE2eLatencyHistogram() {
        return e2eLatencyHistogram;
    }

    public Meter getProcessingRate() {
        return processingRate;
    }

    public Meter getErrorRate() {
        return errorRate;
    }
}