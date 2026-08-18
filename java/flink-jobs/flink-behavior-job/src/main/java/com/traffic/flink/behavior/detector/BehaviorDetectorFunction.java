package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.BehaviorModel;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.functions.RuntimeContext;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;
import org.apache.flink.streaming.api.functions.async.ResultFuture;
import org.apache.flink.streaming.api.functions.async.RichAsyncFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 异步行为检测函数
 *
 * 使用 Flink AsyncIO 进行异步模型推理，提高吞吐量。
 *
 * 功能：
 * 1. 接收 FeatureStat 输入
 * 2. 并行调用多个行为检测模型
 * 3. 聚合推理结果
 * 4. 输出带 typed outcome 的 BehaviorInferenceOutcome（DETECTED /
 *    LOW_CONFIDENCE / NO_DETECTION / MODEL_ERROR / TIMEOUT）
 *
 * 修复：推理异常/超时不再 complete(emptyList()) 静默吞掉——
 * 无输出≠阴性；异常与超时以显式 outcome 上抛，由下游接入 DLQ。
 */
public class BehaviorDetectorFunction extends RichAsyncFunction<FeatureStat, BehaviorInferenceOutcome> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BehaviorDetectorFunction.class);

    /**
     * 配置
     */
    private final BehaviorJobConfig config;

    /**
     * 模型注册表
     */
    private final ModelRegistry modelRegistry;

    /**
     * 推理线程池
     */
    private transient ExecutorService inferenceExecutor;

    /**
     * 统计指标
     */
    private transient AtomicLong processedCount;
    private transient AtomicLong detectedCount;
    private transient AtomicLong errorCount;
    private transient AtomicLong timeoutCount;
    private transient AtomicLong totalInferenceTimeMs;
    private transient Counter modelErrorCounter;

    public BehaviorDetectorFunction(BehaviorJobConfig config, ModelRegistry modelRegistry) {
        this.config = config;
        this.modelRegistry = modelRegistry;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 创建推理线程池
        int threads = config.getInferenceThreads();
        inferenceExecutor = Executors.newFixedThreadPool(threads, r -> {
            Thread t = new Thread(r, "behavior-inference-" + System.currentTimeMillis());
            t.setDaemon(true);
            return t;
        });

        // 初始化统计指标
        processedCount = new AtomicLong(0);
        detectedCount = new AtomicLong(0);
        errorCount = new AtomicLong(0);
        timeoutCount = new AtomicLong(0);
        totalInferenceTimeMs = new AtomicLong(0);

        RuntimeContext runtimeContext = getRuntimeContext();
        runtimeContext.getMetricGroup().gauge("behavior_processed_total", (Gauge<Long>) processedCount::get);
        runtimeContext.getMetricGroup().gauge("behavior_detected_total", (Gauge<Long>) detectedCount::get);
        runtimeContext.getMetricGroup().gauge("behavior_model_errors_total", (Gauge<Long>) errorCount::get);
        runtimeContext.getMetricGroup().gauge("behavior_inference_timeouts_total", (Gauge<Long>) timeoutCount::get);
        modelErrorCounter = runtimeContext.getMetricGroup().counter("behavior_model_error_events_total");

        LOG.info("BehaviorDetectorFunction opened with {} inference threads", threads);
    }

    @Override
    public void asyncInvoke(FeatureStat input, ResultFuture<BehaviorInferenceOutcome> resultFuture) {
        // 异步执行推理
        CompletableFuture.supplyAsync(() -> {
            long startTime = System.currentTimeMillis();
            try {
                // 执行多模型推理
                List<ModelInferenceResult> results = runAllModels(input);

                // 选择最佳结果
                ModelInferenceResult bestResult = selectBestResult(results);

                // 更新统计
                processedCount.incrementAndGet();
                if (bestResult != null && bestResult.isDetected()) {
                    detectedCount.incrementAndGet();
                }
                totalInferenceTimeMs.addAndGet(System.currentTimeMillis() - startTime);

                if (bestResult == null) {
                    // 模型正常推理但无命中：阴性也是合法 typed outcome
                    return BehaviorInferenceOutcome.noDetection();
                }
                DetectionBehavior detection = toDetectionBehavior(input, bestResult);
                if (bestResult.isDetected()
                        && detection.getTopScore() >= config.getMinConfidenceThreshold()) {
                    return BehaviorInferenceOutcome.detected(detection);
                }
                if (config.isDebugPrintEnabled()) {
                    return BehaviorInferenceOutcome.detected(detection);
                }
                return BehaviorInferenceOutcome.lowConfidence(detection);

            } catch (Exception e) {
                errorCount.incrementAndGet();
                if (modelErrorCounter != null) {
                    modelErrorCounter.inc();
                }
                LOG.error("Inference error for feature {}: {}",
                        input.getObjectId(), e.getMessage(), e);
                return BehaviorInferenceOutcome.modelError(e.getMessage());
            }
        }, inferenceExecutor).thenAccept(outcome -> {
            resultFuture.complete(Collections.singletonList(outcome));
        }).exceptionally(throwable -> {
            LOG.error("Async invoke failed: {}", throwable.getMessage(), throwable);
            errorCount.incrementAndGet();
            if (modelErrorCounter != null) {
                modelErrorCounter.inc();
            }
            resultFuture.complete(Collections.singletonList(
                    BehaviorInferenceOutcome.modelError(throwable.getMessage())));
            return null;
        });
    }

    @Override
    public void timeout(FeatureStat input, ResultFuture<BehaviorInferenceOutcome> resultFuture) {
        LOG.warn("Inference timeout for feature: {}", input.getObjectId());
        timeoutCount.incrementAndGet();
        errorCount.incrementAndGet();
        if (modelErrorCounter != null) {
            modelErrorCounter.inc();
        }
        // 超时显式输出 TIMEOUT outcome，由下游接入 DLQ；不静默丢弃
        resultFuture.complete(Collections.singletonList(
                BehaviorInferenceOutcome.timeout("inference timeout for feature " + input.getObjectId())));
    }

    /**
     * 运行所有启用的模型
     */
    private List<ModelInferenceResult> runAllModels(FeatureStat feature) {
        List<ModelInferenceResult> results = new java.util.ArrayList<>();
        String tenantId = feature.hasHeader() ? feature.getHeader().getTenantId() : "";
        Map<String, BehaviorModel> models = modelRegistry.getModelsForTenant(tenantId);

        for (Map.Entry<String, BehaviorModel> entry : models.entrySet()) {
            String modelName = entry.getKey();
            BehaviorModel model = entry.getValue();

            try {
                if (model.isReady()) {
                    ModelInferenceResult result = inferWithRetry(modelName, model, feature);
                    if (result != null && !result.hasError()) {
                        results.add(result);
                        modelRegistry.recordInvocation(modelName);
                    }
                }
            } catch (Exception e) {
                LOG.warn("Model {} inference failed: {}", modelName, e.getMessage());
                modelRegistry.recordError(modelName);
            }
        }

        return results;
    }

    private ModelInferenceResult inferWithRetry(
            String modelName, BehaviorModel model, FeatureStat feature) throws Exception {
        int allowedAttempts = Math.max(1, config.getAsyncMaxRetries() + 1);
        Exception lastFailure = null;
        for (int attempt = 1; attempt <= allowedAttempts; attempt++) {
            try {
                return model.infer(feature);
            } catch (Exception e) {
                lastFailure = e;
                LOG.warn("Model {} inference failed (attempt {}/{}): {}",
                        modelName, attempt, allowedAttempts, e.getMessage());
                if (attempt < allowedAttempts) {
                    try {
                        Thread.sleep(Math.min(500L, 50L << (attempt - 1)));
                    } catch (InterruptedException interrupted) {
                        Thread.currentThread().interrupt();
                        throw interrupted;
                    }
                }
            }
        }
        throw lastFailure;
    }

    /**
     * 选择最佳推理结果
     * 策略：选择置信度最高且超过阈值的结果
     */
    private ModelInferenceResult selectBestResult(List<ModelInferenceResult> results) {
        if (results == null || results.isEmpty()) {
            return null;
        }

        ModelInferenceResult bestResult = null;
        float bestScore = 0.0f;

        for (ModelInferenceResult result : results) {
            if (result.isDetected() && result.getTopScore() > bestScore) {
                bestScore = result.getTopScore();
                bestResult = result;
            }
        }

        // 如果没有检测到的结果，返回置信度最高的
        if (bestResult == null && !results.isEmpty()) {
            for (ModelInferenceResult result : results) {
                if (result.getTopScore() > bestScore) {
                    bestScore = result.getTopScore();
                    bestResult = result;
                }
            }
        }

        return bestResult;
    }

    private DetectionBehavior toDetectionBehavior(FeatureStat input, ModelInferenceResult result) {
        String tenantId = input.hasHeader() ? input.getHeader().getTenantId() : "";
        String modelVersion = modelRegistry.getModelVersion(tenantId, result.getModelName());
        if (modelVersion == null || modelVersion.isEmpty()) {
            modelVersion = result.getModelVersion();
        }
        return BehaviorDetectionEventFactory.build(
                input, result, modelVersion, System.currentTimeMillis());
    }

    @Override
    public void close() throws Exception {
        // 关闭线程池
        if (inferenceExecutor != null) {
            inferenceExecutor.shutdown();
            try {
                if (!inferenceExecutor.awaitTermination(10, TimeUnit.SECONDS)) {
                    inferenceExecutor.shutdownNow();
                }
            } catch (InterruptedException e) {
                inferenceExecutor.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }

        // 打印统计信息
        LOG.info("BehaviorDetectorFunction closed. Stats: processed={}, detected={}, errors={}, timeouts={}, avgLatencyMs={}",
                processedCount.get(),
                detectedCount.get(),
                errorCount.get(),
                timeoutCount.get(),
                processedCount.get() > 0 ? totalInferenceTimeMs.get() / processedCount.get() : 0);

        super.close();
    }

    /**
     * 获取处理计数
     */
    public long getProcessedCount() {
        return processedCount != null ? processedCount.get() : 0;
    }

    /**
     * 获取检测计数
     */
    public long getDetectedCount() {
        return detectedCount != null ? detectedCount.get() : 0;
    }

    /**
     * 获取错误计数
     */
    public long getErrorCount() {
        return errorCount != null ? errorCount.get() : 0;
    }
}
