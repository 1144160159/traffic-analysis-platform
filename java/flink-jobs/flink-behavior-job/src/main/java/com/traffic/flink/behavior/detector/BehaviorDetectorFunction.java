package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.BehaviorModel;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.async.ResultFuture;
import org.apache.flink.streaming.api.functions.async.RichAsyncFunction;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.UUID;
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
 * 4. 输出 DetectionBehavior 事件
 * 
 * 架构特点：
 * - 异步非阻塞推理
 * - 多模型并行执行
 * - 支持模型热更新
 * - 完善的错误处理
 */
public class BehaviorDetectorFunction extends RichAsyncFunction<FeatureStat, DetectionBehavior> {

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
    private transient AtomicLong totalInferenceTimeMs;

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
        totalInferenceTimeMs = new AtomicLong(0);

        LOG.info("BehaviorDetectorFunction opened with {} inference threads", threads);
    }

    @Override
    public void asyncInvoke(FeatureStat input, ResultFuture<DetectionBehavior> resultFuture) {
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

                return bestResult;

            } catch (Exception e) {
                errorCount.incrementAndGet();
                LOG.error("Inference error for feature {}: {}", 
                        input.getObjectId(), e.getMessage(), e);
                return null;
            }
        }, inferenceExecutor).thenAccept(result -> {
            if (result != null && result.isDetected()) {
                // 转换为 DetectionBehavior 并输出
                DetectionBehavior detection = toDetectionBehavior(input, result);
                resultFuture.complete(Collections.singletonList(detection));
            } else if (result != null && config.isDebugPrintEnabled()) {
                // 调试模式下也输出非检测结果
                DetectionBehavior detection = toDetectionBehavior(input, result);
                resultFuture.complete(Collections.singletonList(detection));
            } else {
                // 无检测结果
                resultFuture.complete(Collections.emptyList());
            }
        }).exceptionally(throwable -> {
            LOG.error("Async invoke failed: {}", throwable.getMessage(), throwable);
            errorCount.incrementAndGet();
            resultFuture.complete(Collections.emptyList());
            return null;
        });
    }

    @Override
    public void timeout(FeatureStat input, ResultFuture<DetectionBehavior> resultFuture) {
        LOG.warn("Inference timeout for feature: {}", input.getObjectId());
        errorCount.incrementAndGet();
        resultFuture.complete(Collections.emptyList());
    }

    /**
     * 运行所有启用的模型
     */
    private List<ModelInferenceResult> runAllModels(FeatureStat feature) {
        List<ModelInferenceResult> results = new ArrayList<>();
        String tenantId = feature.hasHeader() ? feature.getHeader().getTenantId() : "";
        Map<String, BehaviorModel> models = modelRegistry.getModelsForTenant(tenantId);

        for (Map.Entry<String, BehaviorModel> entry : models.entrySet()) {
            String modelName = entry.getKey();
            BehaviorModel model = entry.getValue();

            try {
                if (model.isReady()) {
                    ModelInferenceResult result = model.infer(feature);
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

    /**
     * 将推理结果转换为 DetectionBehavior Protobuf 消息
     */
    private DetectionBehavior toDetectionBehavior(FeatureStat input, ModelInferenceResult result) {
        // 构建 EventHeader
        EventHeader.Builder headerBuilder = EventHeader.newBuilder()
                .setEventId(generateEventId(input, result))
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis());

        // 复制输入的 Header 字段
        if (input.hasHeader()) {
            EventHeader inputHeader = input.getHeader();
            headerBuilder.setTenantId(inputHeader.getTenantId());
            headerBuilder.setRunId(inputHeader.getRunId());
            headerBuilder.setProbeId(inputHeader.getProbeId());
            headerBuilder.setFeatureSetId(inputHeader.getFeatureSetId());
        }

        // 构建 DetectionBehavior
        String tenantId = input.hasHeader() ? input.getHeader().getTenantId() : "";
        String modelVersion = modelRegistry.getModelVersion(tenantId, result.getModelName());
        if (modelVersion == null || modelVersion.isEmpty()) {
            modelVersion = result.getModelVersion();
        }

        DetectionBehavior.Builder builder = DetectionBehavior.newBuilder()
                .setHeader(headerBuilder.build())
                .setModelVersion(modelVersion)
                .setCommunityId(input.getCommunityId())
                .setObjectType(input.getObjectType())
                .setObjectId(input.getObjectId())
                .setTs(input.getTs())
                .setTopLabel(result.getTopLabel())
                .setTopScore(result.getTopScore());

        // 添加所有标签和分数
        List<String> labels = result.getLabels();
        List<Float> scores = result.getScores();
        if (labels != null && scores != null) {
            builder.addAllLabels(labels);
            builder.addAllScores(scores);
        }

        return builder.build();
    }

    /**
     * 生成事件ID
     * 格式：hash(tenant_id + run_id + object_id + ts + model_name)
     */
    private String generateEventId(FeatureStat input, ModelInferenceResult result) {
        StringBuilder sb = new StringBuilder();
        
        if (input.hasHeader()) {
            sb.append(input.getHeader().getTenantId());
            sb.append(input.getHeader().getRunId());
        }
        sb.append(input.getObjectId());
        sb.append(input.getTs());
        sb.append(result.getModelName());

        // 使用 UUID v5 风格的确定性 ID
        return UUID.nameUUIDFromBytes(sb.toString().getBytes()).toString();
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
        LOG.info("BehaviorDetectorFunction closed. Stats: processed={}, detected={}, errors={}, avgLatencyMs={}",
                processedCount.get(),
                detectedCount.get(),
                errorCount.get(),
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
