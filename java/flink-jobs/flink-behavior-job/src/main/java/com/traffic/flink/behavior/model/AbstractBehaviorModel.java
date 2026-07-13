package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.time.Time;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * 行为检测模型抽象基类
 *
 * 提供公共功能：
 * 1. 生命周期管理
 * 2. 批量推理的默认实现
 * 3. 指标统计
 * 4. 特征预处理
 * 5. 统一的 StateTTL 配置 (防止生产环境状态无限增长)
 */
public abstract class AbstractBehaviorModel implements BehaviorModel {

    private static final long serialVersionUID = 1L;
    protected final Logger LOG = LoggerFactory.getLogger(getClass());

    /** 默认 State TTL: 2 小时 */
    protected static final long DEFAULT_STATE_TTL_HOURS = 2;

    protected final BehaviorJobConfig config;
    protected final String name;
    protected final String version;
    protected final float threshold;
    protected final List<String> supportedLabels;

    protected final AtomicBoolean initialized = new AtomicBoolean(false);
    protected final AtomicBoolean closed = new AtomicBoolean(false);

    /**
     * 推理统计
     */
    protected transient long totalInferences = 0;
    protected transient long totalInferenceTimeMs = 0;
    protected transient long errorCount = 0;

    protected AbstractBehaviorModel(BehaviorJobConfig config, String name, String version,
                                    float threshold, String... supportedLabels) {
        this.config = config;
        this.name = name;
        this.version = version;
        this.threshold = threshold;
        this.supportedLabels = Arrays.asList(supportedLabels);
    }

    /**
     * 创建带 TTL 的 ValueStateDescriptor，防止状态无限增长。
     * 子类应使用此方法创建所有有状态描述符。
     */
    protected static <T> ValueStateDescriptor<T> createStateDescriptor(String stateName, Class<T> typeClass) {
        return createStateDescriptor(stateName, typeClass, DEFAULT_STATE_TTL_HOURS);
    }

    /**
     * 创建带自定义 TTL 的 ValueStateDescriptor
     */
    protected static <T> ValueStateDescriptor<T> createStateDescriptor(String stateName, Class<T> typeClass, long ttlHours) {
        ValueStateDescriptor<T> descriptor = new ValueStateDescriptor<>(stateName, typeClass);
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.hours(ttlHours))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();
        descriptor.enableTimeToLive(ttlConfig);
        return descriptor;
    }

    @Override
    public String getName() {
        return name;
    }

    @Override
    public String getVersion() {
        return version;
    }

    @Override
    public float getThreshold() {
        return threshold;
    }

    @Override
    public List<String> getSupportedLabels() {
        return supportedLabels;
    }

    @Override
    public boolean isReady() {
        return initialized.get() && !closed.get();
    }

    @Override
    public void initialize() throws Exception {
        if (initialized.compareAndSet(false, true)) {
            LOG.info("Initializing model: {} (version: {})", name, version);
            long startTime = System.currentTimeMillis();
            
            doInitialize();
            
            long elapsed = System.currentTimeMillis() - startTime;
            LOG.info("Model {} initialized in {}ms", name, elapsed);
        }
    }

    /**
     * 子类实现具体的初始化逻辑
     */
    protected abstract void doInitialize() throws Exception;

    @Override
    public ModelInferenceResult infer(FeatureStat feature) {
        if (!isReady()) {
            return ModelInferenceResult.failure(name, version, "Model not ready");
        }

        long startTime = System.currentTimeMillis();
        try {
            // 特征预处理
            float[] processedFeatures = preprocessFeatures(feature);
            
            // 执行推理
            ModelInferenceResult result = doInfer(feature, processedFeatures);
            
            // 统计
            long elapsed = System.currentTimeMillis() - startTime;
            totalInferences++;
            totalInferenceTimeMs += elapsed;
            
            return result;
            
        } catch (Exception e) {
            errorCount++;
            LOG.error("Inference error in model {}: {}", name, e.getMessage(), e);
            return ModelInferenceResult.failure(name, version, e.getMessage());
        }
    }

    /**
     * 子类实现具体的推理逻辑
     */
    protected abstract ModelInferenceResult doInfer(FeatureStat feature, float[] processedFeatures);

    @Override
    public List<ModelInferenceResult> inferBatch(List<FeatureStat> features) {
        List<ModelInferenceResult> results = new ArrayList<>(features.size());
        
        // 默认实现：逐个推理
        // 子类可以覆盖此方法以支持真正的批量推理
        for (FeatureStat feature : features) {
            results.add(infer(feature));
        }
        
        return results;
    }

    /**
     * 特征预处理
     * 将 FeatureStat 转换为模型输入格式
     */
    protected float[] preprocessFeatures(FeatureStat feature) {
        // 提取 17 个核心特征
        float[] features = new float[17];
        
        features[0] = feature.getProtocol();
        features[1] = feature.getDurationMs();
        features[2] = feature.getPps();
        features[3] = feature.getBps();
        features[4] = feature.getUpDownRatio();
        features[5] = feature.getPktlenMean();
        features[6] = feature.getPktlenStd();
        features[7] = feature.getIatMeanMs();
        features[8] = feature.getIatStdMs();
        features[9] = feature.getActiveMeanMs();
        features[10] = feature.getIdleMeanMs();
        features[11] = feature.getTcpFlagSynCnt();
        features[12] = feature.getTcpFlagAckCnt();
        features[13] = feature.getTcpInitWinBytesFwd();
        features[14] = feature.getTcpInitWinBytesBwd();
        
        // 添加 extra 特征（如果有）
        List<Float> extra = feature.getExtraList();
        if (extra != null && extra.size() >= 2) {
            features[15] = extra.get(0);
            features[16] = extra.get(1);
        }
        
        return features;
    }

    /**
     * 特征归一化
     */
    protected float[] normalizeFeatures(float[] features, float[] means, float[] stds) {
        float[] normalized = new float[features.length];
        for (int i = 0; i < features.length; i++) {
            if (stds[i] > 0) {
                normalized[i] = (features[i] - means[i]) / stds[i];
            } else {
                normalized[i] = 0;
            }
        }
        return normalized;
    }

    /**
     * 计算 softmax
     */
    protected float[] softmax(float[] logits) {
        float max = Float.NEGATIVE_INFINITY;
        for (float logit : logits) {
            max = Math.max(max, logit);
        }
        
        float sum = 0;
        float[] exp = new float[logits.length];
        for (int i = 0; i < logits.length; i++) {
            exp[i] = (float) Math.exp(logits[i] - max);
            sum += exp[i];
        }
        
        float[] probs = new float[logits.length];
        for (int i = 0; i < logits.length; i++) {
            probs[i] = exp[i] / sum;
        }
        
        return probs;
    }

    /**
     * 计算 sigmoid
     */
    protected float sigmoid(float x) {
        return (float) (1.0 / (1.0 + Math.exp(-x)));
    }

    @Override
    public String checkForUpdate() {
        // 默认实现：不检查更新
        return null;
    }

    @Override
    public BehaviorModel reload() throws Exception {
        // 默认实现：返回自身
        return this;
    }

    @Override
    public void close() throws Exception {
        if (closed.compareAndSet(false, true)) {
            LOG.info("Closing model: {} (version: {})", name, version);
            LOG.info("Model stats - Total inferences: {}, Avg time: {}ms, Errors: {}",
                    totalInferences,
                    totalInferences > 0 ? totalInferenceTimeMs / totalInferences : 0,
                    errorCount);
            
            doClose();
            
            LOG.info("Model {} closed", name);
        }
    }

    /**
     * 子类实现具体的关闭逻辑
     */
    protected void doClose() throws Exception {
        // 默认空实现
    }

    /**
     * 获取平均推理时间
     */
    public long getAverageInferenceTimeMs() {
        return totalInferences > 0 ? totalInferenceTimeMs / totalInferences : 0;
    }

    /**
     * 获取总推理次数
     */
    public long getTotalInferences() {
        return totalInferences;
    }

    /**
     * 获取错误次数
     */
    public long getErrorCount() {
        return errorCount;
    }
}