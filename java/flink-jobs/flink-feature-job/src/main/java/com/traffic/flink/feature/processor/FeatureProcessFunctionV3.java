package com.traffic.flink.feature.processor;

import com.traffic.flink.common.DeterministicId;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.feature.calculator.FeatureCalculator;
import com.traffic.flink.feature.calculator.FeatureFingerprintCalculator;
import com.traffic.flink.feature.calculator.FeatureSeqCalculator;
import com.traffic.flink.feature.config.FeatureSetConfig;
import com.traffic.flink.feature.config.TenantConfig;
import com.traffic.flink.feature.metrics.FeatureMetrics;
import com.traffic.flink.feature.source.ValidatedSessionInput;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FeatureFingerprint;
import com.traffic.proto.traffic.v1.FeatureSeq;
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
public class FeatureProcessFunctionV3 extends BroadcastProcessFunction<ValidatedSessionInput, Object, FeatureStat> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FeatureProcessFunctionV3.class);

    // 侧输出标签
    public static final OutputTag<CanonicalDlqMessage> DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("dlq-errors", TypeInformation.of(CanonicalDlqMessage.class)) {};
    public static final OutputTag<SessionEvent> L2_TRIGGER_TAG =
            new OutputTag<SessionEvent>("l2-trigger", TypeInformation.of(SessionEvent.class)) {};
    public static final OutputTag<FeatureSeq> FEATURE_SEQ_TAG =
            new OutputTag<FeatureSeq>("feature-sequence", TypeInformation.of(FeatureSeq.class)) {};
    public static final OutputTag<FeatureFingerprint> FEATURE_FINGERPRINT_TAG =
            new OutputTag<FeatureFingerprint>(
                    "feature-fingerprint", TypeInformation.of(FeatureFingerprint.class)) {};

    // BroadcastState 描述符
    public static final MapStateDescriptor<String, FeatureSetConfig> FEATURE_SET_STATE_DESC =
            new MapStateDescriptor<>("feature-set-config", String.class, FeatureSetConfig.class);
    
    public static final MapStateDescriptor<String, TenantConfig> TENANT_CONFIG_STATE_DESC =
            new MapStateDescriptor<>("tenant-config", String.class, TenantConfig.class);

    // 配置
    private final boolean enableSampling;
    private final float defaultSamplingRate;
    private final long allowedLatenessMs;

    // 降级相关
    private static final long E2E_LATENCY_WARN_MS = 60000;
    private static final int BACKPRESSURE_CHECK_INTERVAL = 1000; // 每处理 1000 条检测一次

    // Metrics
    private transient FeatureMetrics metrics;
    private transient long lastLogTime;
    private transient long processedCount;

    // 租户级 EPS 配额滑动窗口（子任务内存态，重启后重置）
    private transient java.util.Map<String, TenantEpsWindow> tenantEpsWindows;

    // 默认配置（当 BroadcastState 未加载时使用）
    private transient FeatureSetConfig defaultFeatureSetConfig;
    private transient TenantConfig defaultTenantConfig;

    public FeatureProcessFunctionV3() {
        this(false, 1.0f, 0L);
    }

    public FeatureProcessFunctionV3(boolean enableSampling, float defaultSamplingRate) {
        this(enableSampling, defaultSamplingRate, 0L);
    }

    public FeatureProcessFunctionV3(
            boolean enableSampling,
            float defaultSamplingRate,
            long allowedLatenessMs) {
        if (defaultSamplingRate < 0.0f || defaultSamplingRate > 1.0f) {
            throw new IllegalArgumentException("default sampling rate must be between 0 and 1");
        }
        if (allowedLatenessMs < 0L) {
            throw new IllegalArgumentException("allowed lateness must not be negative");
        }
        this.enableSampling = enableSampling;
        this.defaultSamplingRate = defaultSamplingRate;
        this.allowedLatenessMs = allowedLatenessMs;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 初始化 Metrics
        this.metrics = new FeatureMetrics(getRuntimeContext().getMetricGroup());
        this.lastLogTime = System.currentTimeMillis();
        this.processedCount = 0;
        this.tenantEpsWindows = new java.util.HashMap<>();

        // 初始化默认配置
        this.defaultFeatureSetConfig = createDefaultFeatureSetConfig();
        this.defaultTenantConfig = createDefaultTenantConfig();

        LOG.info("FeatureProcessFunctionV3 initialized with sampling: enabled={}, rate={}",
            enableSampling, defaultSamplingRate);
    }

    @Override
    public void processElement(
            ValidatedSessionInput input,
            ReadOnlyContext ctx,
            Collector<FeatureStat> out
    ) throws Exception {

        SessionEvent session = input.getSession();

        long eventTimestamp = session.getEventTimeEndMs() > 0
                ? session.getEventTimeEndMs() : session.getTsEnd();
        long watermark = ctx.currentWatermark();
        if (isTooLate(eventTimestamp, watermark, allowedLatenessMs)) {
            metrics.incSkipped();
            metrics.incDlqWrite();
            ctx.output(DLQ_TAG, lateDataFailure(input, watermark, allowedLatenessMs));
            return;
        }

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
            FeatureSeq sequence = FeatureSeqCalculator.calculate(session);
            FeatureFingerprint fingerprint = FeatureFingerprintCalculator.calculate(session);
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
            ctx.output(FEATURE_SEQ_TAG, sequence);
            ctx.output(FEATURE_FINGERPRINT_TAG, fingerprint);
            metrics.incProcessed();
            out.collect(feature);

            // ==================== 定期检测 Backpressure ====================
            processedCount++;
            if (processedCount % BACKPRESSURE_CHECK_INTERVAL == 0) {
                logStatsIfNeeded();
            }

        } catch (Exception e) {
            metrics.incError();
            handleError(input, e, ctx);
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
            if (config.isActive()) {
                state.put(config.getFeatureSetId(), config);
                LOG.info("Feature Set config updated: {}", config);
            } else {
                state.remove(config.getFeatureSetId());
                LOG.info("Feature Set config removed: featureSetId={}", config.getFeatureSetId());
            }

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
        if (enableSampling && !DeterministicId.sample(
                defaultSamplingRate,
                "flink-feature-global-sampling/v1",
                session.getHeader().getTenantId(),
                session.getHeader().getEventId(),
                session.getSessionId())) {
            return true;
        }

        // 2. 租户级采样
        if (tenantConfig.isEnableDegradation() && !DeterministicId.sample(
                tenantConfig.getSamplingRate(),
                "flink-feature-tenant-sampling/v1",
                session.getHeader().getTenantId(),
                session.getHeader().getEventId(),
                session.getSessionId())) {
            return true;
        }

        // 3. 租户优先级过滤（Backpressure 时仅保留高优先级租户）
        if (isBackpressured() && tenantConfig.getPriority() < 7) {
            return true;
        }

        // 4. 租户级 EPS 配额（maxEventsPerSecond > 0 时生效）。
        // 配额在子任务内尽力执行（内存滑动窗口，重启后重置）；超限输入被跳过。
        // 此前配额字段被写入广播状态但从未校验，租户级限流形同虚设。
        if (tenantConfig.getMaxEventsPerSecond() > 0
                && !tryAcquireEps(session.getHeader().getTenantId(),
                        tenantConfig.getMaxEventsPerSecond())) {
            return true;
        }

        return false;
    }

    /**
     * 租户级 EPS 配额滑动窗口：每租户每秒配额。
     */
    private boolean tryAcquireEps(String tenantId, int maxEventsPerSecond) {
        long nowSecond = System.currentTimeMillis() / 1000L;
        TenantEpsWindow window = tenantEpsWindows.get(tenantId);
        if (window == null) {
            window = new TenantEpsWindow(nowSecond, 0);
            tenantEpsWindows.put(tenantId, window);
        } else if (window.second != nowSecond) {
            window.second = nowSecond;
            window.count = 0;
        }
        if (window.count >= maxEventsPerSecond) {
            return false;
        }
        window.count++;
        return true;
    }

    /** 租户 EPS 滑动窗口（1 秒粒度，子任务内存态） */
    private static final class TenantEpsWindow {
        long second;
        int count;

        TenantEpsWindow(long second, int count) {
            this.second = second;
            this.count = count;
        }
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

        // 3. 高载荷方差只触发深度特征，不推断加密或恶意。
        if (session.getStdPayload() > thresholds.getEncryptedStdPayloadThreshold()) {
            return true;
        }

        return false;
    }

    /**
     * 检测 Backpressure — Flink MetricGroup 不提供内置指标读取接口,
     * 这里使用端到端延迟 P95 作为启发式降级信号。
     */
    private boolean isBackpressured() {
        Histogram e2eHistogram = metrics.getE2eLatencyHistogram();
        if (e2eHistogram != null && e2eHistogram.getCount() > 100) {
            long p95 = (long) e2eHistogram.getStatistics().getQuantile(0.95);
            if (p95 > 30_000L) {
                return true;
            }
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
            metrics.incHighPayloadVariance();
        }

        metrics.recordPPS(feature.getPps());
        metrics.recordBPS(feature.getBps());
        metrics.recordUpDownRatio(feature.getUpDownRatio());
    }

    /**
     * 错误处理
     */
    private void handleError(ValidatedSessionInput input, Exception e, ReadOnlyContext ctx) {
        SessionEvent session = input.getSession();
        LOG.error("Failed to calculate features for session {} (tenant={}, run_id={}): {}",
                session.getSessionId(),
                session.getHeader().getTenantId(),
                session.getHeader().getRunId(),
                e.getMessage(),
                e);

        CanonicalDlqMessage dlqMessage = CanonicalDlqMessage.failure(
                input.getSource(),
                "FEATURE_CALCULATION_FAILED",
                "processing_error",
                e.getMessage(),
                session.getHeader().getTenantId(),
                session.getHeader().getEventId(),
                session.getHeader().getTraceId(),
                session.getHeader().getRunId(),
                session.getHeader().getProbeId());
        ctx.output(DLQ_TAG, dlqMessage);
        metrics.incDlqWrite();
    }

    static boolean isTooLate(long eventTimestamp, long watermark, long allowedLatenessMs) {
        return EventTimePolicy.isLate(eventTimestamp, watermark, allowedLatenessMs);
    }

    static CanonicalDlqMessage lateDataFailure(
            ValidatedSessionInput input,
            long watermark,
            long allowedLatenessMs) {
        SessionEvent session = input.getSession();
        long eventTimestamp = session.getEventTimeEndMs() > 0
                ? session.getEventTimeEndMs() : session.getTsEnd();
        return CanonicalDlqMessage.failure(
                input.getSource(),
                "SUPER_LATE_EVENT",
                "event_time_lateness",
                "event_time_exceeded_feature_allowed_lateness: event_time_ms=" + eventTimestamp
                        + ", watermark_ms=" + watermark
                        + ", allowed_lateness_ms=" + allowedLatenessMs,
                session.getHeader().getTenantId(),
                session.getHeader().getEventId(),
                session.getHeader().getTraceId(),
                session.getHeader().getRunId(),
                session.getHeader().getProbeId());
    }

    private static long safeAdd(long left, long right) {
        if (right > 0L && left > Long.MAX_VALUE - right) return Long.MAX_VALUE;
        if (right < 0L && left < Long.MIN_VALUE - right) return Long.MIN_VALUE;
        return left + right;
    }

    /**
     * 定期打印统计信息
     */
    private void logStatsIfNeeded() {
        long now = System.currentTimeMillis();
        if (now - lastLogTime > 10_000) {
            LOG.info("Feature Stats: " +
                            "processed={}, error={}, skipped={}, " +
                            "zero_packets={}, high_pps={}, high_bps={}, high_payload_variance={}, " +
                            "l2_triggered={}, dlq_write={}, " +
                            "processing_rate={}/s, error_rate={}/s",
                    metrics.getProcessedCounter().getCount(),
                    metrics.getErrorCounter().getCount(),
                    metrics.getSkippedCounter().getCount(),
                    metrics.getZeroPacketsCounter().getCount(),
                            metrics.getHighPpsCounter().getCount(),
                            metrics.getHighBpsCounter().getCount(),
                            metrics.getHighPayloadVarianceCounter().getCount(),
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
        config.setEnableL2Trigger(false);

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
    TenantConfig createDefaultTenantConfig() {
        TenantConfig config = new TenantConfig("default");
        config.setPriority(10);
        config.setEnableL2(true);
        config.setSamplingRate(1.0f);
        config.setMaxEventsPerSecond(-1);
        config.setEnableDegradation(false);
        return config;
    }
}
