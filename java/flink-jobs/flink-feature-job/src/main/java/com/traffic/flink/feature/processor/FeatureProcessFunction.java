package com.traffic.flink.feature.processor;

import com.traffic.flink.feature.calculator.FeatureCalculator;
import com.traffic.flink.feature.config.FeatureSetConfig;
import com.traffic.flink.feature.config.TenantConfig;
import com.traffic.flink.feature.metrics.FeatureMetrics;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;
import org.apache.flink.metrics.Histogram;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.util.Base64;

/**
 * Feature 处理函数 v3（完整增强版）
 * 
 * 增强内容（P2）：
 * 1. ✅ 候选触发机制（L2 侧输出）
 * 2. ✅ Backpressure 检测与自动降级
 * 3. ✅ Feature Set 动态加载（BroadcastState）
 * 4. ✅ 租户级配置支持
 * 5. ✅ 租户优先级管理
 */
public class FeatureProcessFunction extends BroadcastProcessFunction<SessionEvent, Object, FeatureStat> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FeatureProcessFunction.class);

    // 侧输出标签
    public static final OutputTag<String> DLQ_TAG =
            new OutputTag<String>("dlq-errors", TypeInformation.of(String.class)) {};
    public static final OutputTag<SessionEvent> L2_TRIGGER_TAG =
            new OutputTag<SessionEvent>("l2-trigger", TypeInformation.of(SessionEvent.class)) {};

    // BroadcastState 描述符
    public static final MapStateDescriptor<String, FeatureSetConfig> FEATURE_SET_STATE_DESC =
            new MapStateDescriptor<>("feature-set-config", String.class, FeatureSetConfig.class);
    
    public static final MapStateDescriptor<String, TenantConfig> TENANT_CONFIG_STATE_DESC =
            new MapStateDescriptor<>("tenant-config", String.class, TenantConfig.class);

    // 配置
    private final boolean enableSampling;
    private final float defaultSamplingRate;

    // 降级相关
    private static final long E2E_LATENCY_WARN_MS = 60000;
    private static final int BACKPRESSURE_CHECK_INTERVAL = 1000; // 每处理 1000 条检测一次

    // Metrics
    private transient FeatureMetrics metrics;
    private transient long lastLogTime;
    private transient long processedCount;

    // 默认配置（当 BroadcastState 未加载时使用）
    private transient FeatureSetConfig defaultFeatureSetConfig;
    private transient TenantConfig defaultTenantConfig;

    public FeatureProcessFunction() {
        this(false, 1.0f);
    }

    public FeatureProcessFunction(boolean enableSampling, float defaultSamplingRate) {
        this.enableSampling = enableSampling;
        this.defaultSamplingRate = defaultSamplingRate;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 初始化 Metrics
        this.metrics = new FeatureMetrics(getRuntimeContext().getMetricGroup());
        this.lastLogTime = System.currentTimeMillis();
        this.processedCount = 0;

        // 初始化默认配置
        this.defaultFeatureSetConfig = createDefaultFeatureSetConfig();
        this.defaultTenantConfig = createDefaultTenantConfig();

        LOG.info("FeatureProcessFunctionV3 initialized with sampling: enabled={}, rate={}",
                enableSampling, defaultSamplingRate);
    }

    @Override
    public void processElement(
            SessionEvent session,
            ReadOnlyContext ctx,
            Collector<FeatureStat> out
    ) throws Exception {

        try {
            // ==================== 获取配置 ====================
            ReadOnlyBroadcastState<String, FeatureSetConfig> featureSetState =
                    ctx.getBroadcastState(FEATURE_SET_STATE_DESC);
            ReadOnlyBroadcastState<String, TenantConfig> tenantConfigState =
                    ctx.getBroadcastState(TENANT_CONFIG_STATE_DESC);

            String featureSetId = session.getHeader().getFeatureSetId();
            String tenantId = session.getHeader().getTenantId();

            FeatureSetConfig featureSetConfig = featureSetState.get(featureSetId);
            if (featureSetConfig == null) {
                featureSetConfig = defaultFeatureSetConfig;
            }

            TenantConfig tenantConfig = tenantConfigState.get(tenantId);
            if (tenantConfig == null) {
                tenantConfig = defaultTenantConfig;
            }

            // ==================== 降级逻辑 ====================
            if (shouldSkip(session, tenantConfig)) {
                metrics.incSkipped();
                return;
            }

            // ==================== 端到端延迟监控 ====================
            long sessionIngestTs = session.getHeader().getIngestTs();
            long now = System.currentTimeMillis();
            long e2eLatencyMs = now - sessionIngestTs;
            
            metrics.recordE2ELatency(e2eLatencyMs);
            
            if (e2eLatencyMs > E2E_LATENCY_WARN_MS) {
                LOG.warn("High E2E latency: {}ms for session {} (tenant={})",
                        e2eLatencyMs, session.getSessionId(), tenantId);
            }

            // ==================== 计算特征 ====================
            long startTime = System.nanoTime();
            FeatureStat feature = FeatureCalculator.calculate(session);
            long endTime = System.nanoTime();

            metrics.recordFeatureDuration(endTime - startTime);

            // ==================== 业务指标监控 ====================
            recordBusinessMetrics(session, feature);

            // ==================== L2 候选触发（✅ 新增）====================
            if (featureSetConfig.isEnableL2Trigger() && tenantConfig.isEnableL2()) {
                if (shouldTriggerL2(session, feature, featureSetConfig)) {
                    ctx.output(L2_TRIGGER_TAG, session);
                    metrics.incL2Triggered();
                }
            }

            // ==================== 输出特征 ====================
            metrics.incProcessed();
            out.collect(feature);

            // ==================== 定期检测 Backpressure ====================
            processedCount++;
            if (processedCount % BACKPRESSURE_CHECK_INTERVAL == 0) {
                logStatsIfNeeded();
            }

        } catch (Exception e) {
            metrics.incError();
            handleError(session, e, ctx);
        }
    }

    @Override
    public void processBroadcastElement(
            Object value,
            Context ctx,
            Collector<FeatureStat> out
    ) throws Exception {

        if (value instanceof FeatureSetConfig) {
            // 更新 Feature Set 配置
            FeatureSetConfig config = (FeatureSetConfig) value;
            BroadcastState<String, FeatureSetConfig> state = ctx.getBroadcastState(FEATURE_SET_STATE_DESC);
            state.put(config.getFeatureSetId(), config);
            LOG.info("Feature Set config updated: {}", config);

        } else if (value instanceof TenantConfig) {
            // 更新 Tenant 配置
            TenantConfig config = (TenantConfig) value;
            BroadcastState<String, TenantConfig> state = ctx.getBroadcastState(TENANT_CONFIG_STATE_DESC);
            state.put(config.getTenantId(), config);
            LOG.info("Tenant config updated: {}", config);
        }
    }

    /**
     * 判断是否应跳过处理（降级逻辑）
     */
    private boolean shouldSkip(SessionEvent session, TenantConfig tenantConfig) {
        // 1. 全局采样降级
        if (enableSampling && Math.random() > defaultSamplingRate) {
            return true;
        }

        // 2. 租户级采样
        if (tenantConfig.isEnableDegradation() && Math.random() > tenantConfig.getSamplingRate()) {
            return true;
        }

        // 3. 租户优先级过滤（Backpressure 时仅保留高优先级租户）
        if (isBackpressured() && tenantConfig.getPriority() < 7) {
            return true;
        }

        // 4. 跳过无效数据
        if (session.getPacketsTotal() == 0 && session.getBytesTotal() == 0) {
            return true;
        }

        return false;
    }

    /**
     * 判断是否触发 L2 特征提取
     */
    private boolean shouldTriggerL2(
            SessionEvent session,
            FeatureStat feature,
            FeatureSetConfig config
    ) {
        FeatureSetConfig.L2TriggerThresholds thresholds = config.getL2Thresholds();

        // 1. 高 PPS/BPS 流
        if (feature.getPps() > thresholds.getHighPpsThreshold() ||
            feature.getBps() > thresholds.getHighBpsThreshold()) {
            return true;
        }

        // 2. 特定协议（TLS/HTTP）
        int protocol = session.getProtocol();
        int dstPort = session.getTuple() != null ? session.getTuple().getDstPort() : 0;
        if (protocol == 6 && (dstPort == thresholds.getTlsPort() || dstPort == thresholds.getHttpPort())) {
            return true;
        }

        // 3. 疑似加密流量（载荷标准差高）
        if (session.getStdPayload() > thresholds.getEncryptedStdPayloadThreshold()) {
            return true;
        }

        return false;
    }

    /**
     * 检测 Backpressure（简化实现）
     * 生产环境应通过 Flink Metrics API 获取实际 backpressure 状态
     */
    private boolean isBackpressured() {
        // 基于端到端延迟的 backpressure 检测:
        // 如果最近的处理延迟超过阈值，判定为 backpressure
        Histogram e2eHistogram = metrics.getE2eLatencyHistogram();
        if (e2eHistogram != null && e2eHistogram.getCount() > 10) {
            // 使用 histogram 的统计信息: 均值超过 30 秒即认为有 backpressure
            double mean = e2eHistogram.getStatistics().getMean();
            if (mean > 30_000.0) { // 30 seconds in ms
                LOG.warn("Backpressure detected: mean e2e latency {:.0f}ms", mean);
                return true;
            }
        }
        // Fallback: if we have very few data points, cannot reliably determine backpressure
        if (e2eHistogram != null && e2eHistogram.getCount() < 10) {
            return false; // Not enough data yet, cannot determine backpressure
        }
        return false;
    }

    /**
     * 记录业务指标
     */
    private void recordBusinessMetrics(SessionEvent session, FeatureStat feature) {
        if (session.getPacketsTotal() == 0) {
            metrics.incZeroPackets();
        }

        if (feature.getPps() > 10000.0f) {
            metrics.incHighPps();
        }

        if (feature.getBps() > 1e9f) {
            metrics.incHighBps();
        }

        if (session.getStdPayload() > 100.0f) {
            metrics.incEncrypted();
        }

        metrics.recordPPS(feature.getPps());
        metrics.recordBPS(feature.getBps());
        metrics.recordUpDownRatio(feature.getUpDownRatio());
    }

    /**
     * 错误处理
     */
    private void handleError(SessionEvent session, Exception e, ReadOnlyContext ctx) {
        LOG.error("Failed to calculate features for session {} (tenant={}, run_id={}): {}",
                session.getSessionId(),
                session.getHeader().getTenantId(),
                session.getHeader().getRunId(),
                e.getMessage(),
                e);

        String dlqMessage = buildEnhancedDLQMessage(session, e);
        ctx.output(DLQ_TAG, dlqMessage);
        metrics.incDlqWrite();
    }

    /**
     * 构造增强版 DLQ 消息
     */
    private String buildEnhancedDLQMessage(SessionEvent session, Exception e) {
        String stackTrace = getStackTraceString(e);
        String rawEventBase64 = Base64.getEncoder().encodeToString(session.toByteArray());

        return String.format(
                "{\"tenant_id\":\"%s\"," +
                        "\"run_id\":\"%s\"," +
                        "\"session_id\":\"%s\"," +
                        "\"event_id\":\"%s\"," +
                        "\"community_id\":\"%s\"," +
                        "\"probe_id\":\"%s\"," +
                        "\"error_type\":\"%s\"," +
                        "\"error_message\":\"%s\"," +
                        "\"stack_trace\":\"%s\"," +
                        "\"timestamp\":%d," +
                        "\"raw_event_base64\":\"%s\"}",
                escapeJson(session.getHeader().getTenantId()),
                escapeJson(session.getHeader().getRunId()),
                escapeJson(session.getSessionId()),
                escapeJson(session.getHeader().getEventId()),
                escapeJson(session.getCommunityId()),
                escapeJson(session.getHeader().getProbeId()),
                escapeJson(e.getClass().getSimpleName()),
                escapeJson(e.getMessage()),
                escapeJson(stackTrace),
                System.currentTimeMillis(),
                rawEventBase64
        );
    }

    private String getStackTraceString(Exception e) {
        StringWriter sw = new StringWriter();
        PrintWriter pw = new PrintWriter(sw);
        e.printStackTrace(pw);
        return sw.toString();
    }

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
     * 定期打印统计信息
     */
    private void logStatsIfNeeded() {
        long now = System.currentTimeMillis();
        if (now - lastLogTime > 10_000) {
            LOG.info("Feature Stats: " +
                            "processed={}, error={}, skipped={}, " +
                            "zero_packets={}, high_pps={}, high_bps={}, encrypted={}, " +
                            "l2_triggered={}, dlq_write={}, " +
                            "processing_rate={}/s, error_rate={}/s",
                    metrics.getProcessedCounter().getCount(),
                    metrics.getErrorCounter().getCount(),
                    metrics.getSkippedCounter().getCount(),
                    metrics.getZeroPacketsCounter().getCount(),
                    metrics.getHighPpsCounter().getCount(),
                    metrics.getHighBpsCounter().getCount(),
                    metrics.getEncryptedCounter().getCount(),
                    metrics.getL2TriggeredCounter().getCount(),
                    metrics.getDlqWriteCounter().getCount(),
                    String.format("%.2f", metrics.getProcessingRate().getRate()),
                    String.format("%.2f", metrics.getErrorRate().getRate())
            );
            lastLogTime = now;
        }
    }

    /**
     * 创建默认 Feature Set 配置
     */
    private FeatureSetConfig createDefaultFeatureSetConfig() {
        FeatureSetConfig config = new FeatureSetConfig("default", "v2.0");
        config.setIatThresholdMs(1000.0f);
        config.setEnableL2Trigger(true);

        FeatureSetConfig.L2TriggerThresholds thresholds = new FeatureSetConfig.L2TriggerThresholds();
        thresholds.setHighPpsThreshold(10000.0f);
        thresholds.setHighBpsThreshold(1e9f);
        thresholds.setEncryptedStdPayloadThreshold(100.0f);
        thresholds.setTlsPort(443);
        thresholds.setHttpPort(80);
        config.setL2Thresholds(thresholds);

        return config;
    }

    /**
     * 创建默认租户配置
     */
    private TenantConfig createDefaultTenantConfig() {
        TenantConfig config = new TenantConfig("default");
        config.setPriority(5);
        config.setEnableL2(true);
        config.setSamplingRate(1.0f);
        config.setMaxEventsPerSecond(-1);
        config.setEnableDegradation(true);
        return config;
    }
}
