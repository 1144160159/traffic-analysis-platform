////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelRegistry.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.*;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;

/**
 * 模型注册表
 * 
 * 功能：
 * 1. 管理所有行为检测模型的生命周期
 * 2. 支持模型热更新
 * 3. 支持模型版本管理
 * 4. 提供模型健康检查
 * 
 * 支持的模型列表：
 * - scan: 扫描检测
 * - tunnel: 隧道检测
 * - dga: DGA 检测
 * - encrypted: 加密流量检测
 * - anomaly: 异常检测
 * - c2: C2 通信检测
 * - data_exfil: 数据外泄检测
 * - botnet: 僵尸网络检测
 * - malware: 恶意软件检测
 * - phishing: 钓鱼检测
 */
public class ModelRegistry implements Serializable, AutoCloseable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ModelRegistry.class);

    /**
     * 模型实例映射：模型名称 -> 模型实例
     */
    private final Map<String, BehaviorModel> models = new ConcurrentHashMap<>();

    /**
     * 模型版本映射：模型名称 -> 版本号
     */
    private final Map<String, String> modelVersions = new ConcurrentHashMap<>();

    /**
     * TaskManager JVM 级活跃版本映射。
     *
     * Flink 会为 broadcast operator 和 detector operator 分别反序列化函数实例；
     * 使用 JVM 级版本表可让同一 TaskManager 内的检测函数立即看到广播热更新。
     */
    private static final Map<String, BehaviorModel> activeTenantModels = new ConcurrentHashMap<>();
    private static final Map<String, String> activeTenantVersions = new ConcurrentHashMap<>();
    private static final Map<String, String> activeTenantArtifactSha256 = new ConcurrentHashMap<>();
    private static final Map<String, Object> modelSwapLocks = new ConcurrentHashMap<>();
    private static final ScheduledExecutorService retiredModelCloser =
            Executors.newSingleThreadScheduledExecutor(runnable -> {
                Thread thread = new Thread(runnable, "retired-model-closer");
                thread.setDaemon(true);
                return thread;
            });

    /**
     * 模型调用统计：模型名称 -> 调用次数
     */
    private final Map<String, AtomicLong> modelInvocations = new ConcurrentHashMap<>();

    /**
     * 模型错误统计：模型名称 -> 错误次数
     */
    private final Map<String, AtomicLong> modelErrors = new ConcurrentHashMap<>();

    /**
     * 配置
     */
    private final BehaviorJobConfig config;

    /**
     * 模型重加载调度器
     */
    private transient ScheduledExecutorService reloadScheduler;
    private transient MinioModelLoader artifactLoader;

    /**
     * 初始化时间
     */
    private final long initTime = System.currentTimeMillis();

    public ModelRegistry(BehaviorJobConfig config) {
        this.config = config;
        initializeModels();
        startReloadScheduler();
    }

    /**
     * 初始化所有模型
     */
    private void initializeModels() {
        LOG.info("Initializing model registry...");
        long startTime = System.currentTimeMillis();

        Set<String> enabledModels = config.getEnabledModels();
        LOG.info("Enabled models: {}", enabledModels);

        // 扫描检测模型
        if (enabledModels.contains("scan")) {
            registerModel("scan", new ScanDetectionModel(config));
        }

        // 隧道检测模型
        if (enabledModels.contains("tunnel")) {
            registerModel("tunnel", new TunnelDetectionModel(config));
        }

        // DGA 检测模型
        if (enabledModels.contains("dga")) {
            registerModel("dga", new DGADetectionModel(config));
        }

        // 加密流量分析模型
        if (enabledModels.contains("encrypted")) {
            registerModel("encrypted", new EncryptedTrafficModel(config));
        }

        // 异常检测模型
        if (enabledModels.contains("anomaly")) {
            registerModel("anomaly", new AnomalyDetectionModel(config));
        }

        // C2 通信检测模型
        if (enabledModels.contains("c2")) {
            registerModel("c2", new C2DetectionModel(config));
        }

        // 数据外泄检测模型
        if (enabledModels.contains("data_exfil")) {
            registerModel("data_exfil", new DataExfilDetectionModel(config));
        }

        // ========== 新增模型 ==========

        // 僵尸网络检测模型
        if (enabledModels.contains("botnet")) {
            registerModel("botnet", new BotnetDetectionModel(config));
        }

        // 恶意软件检测模型
        if (enabledModels.contains("malware")) {
            registerModel("malware", new MalwareDetectionModel(config));
        }

        // 钓鱼检测模型
        if (enabledModels.contains("phishing")) {
            registerModel("phishing", new PhishingDetectionModel(config));
        }

        long elapsed = System.currentTimeMillis() - startTime;
        LOG.info("Model registry initialized with {} models in {}ms", models.size(), elapsed);
        
        // 打印已注册的模型列表
        for (String modelName : models.keySet()) {
            BehaviorModel model = models.get(modelName);
            LOG.info("  - {} (version: {}, threshold: {})", 
                    modelName, model.getVersion(), model.getThreshold());
        }
    }

    /**
     * 注册模型
     */
    public void registerModel(String name, BehaviorModel model) {
        try {
            model.initialize();
            models.put(name, model);
            modelVersions.put(name, model.getVersion());
            modelInvocations.put(name, new AtomicLong(0));
            modelErrors.put(name, new AtomicLong(0));
            LOG.info("Registered model: {} (version: {}, description: {})", 
                    name, model.getVersion(), model.getDescription());
        } catch (Exception e) {
            LOG.error("Failed to register model {}: {}", name, e.getMessage(), e);
        }
    }

    /**
     * 注销模型
     */
    public void unregisterModel(String name) {
        BehaviorModel model = models.remove(name);
        if (model != null) {
            try {
                model.close();
                modelVersions.remove(name);
                modelInvocations.remove(name);
                modelErrors.remove(name);
                LOG.info("Unregistered model: {}", name);
            } catch (Exception e) {
                LOG.error("Failed to close model {}: {}", name, e.getMessage(), e);
            }
        }
    }

    /**
     * 获取模型
     */
    public BehaviorModel getModel(String name) {
        return models.get(name);
    }

    /**
     * 获取所有模型
     */
    public Map<String, BehaviorModel> getAllModels() {
        return models;
    }

    /** Built-in models plus only the calling tenant's dynamically activated models. */
    public Map<String, BehaviorModel> getModelsForTenant(String tenantId) {
        Map<String, BehaviorModel> scoped = new HashMap<>(models);
        String prefix = scopePrefix(tenantId);
        activeTenantModels.forEach((key, model) -> {
            if (key.startsWith(prefix)) {
                scoped.put(key.substring(prefix.length()), model);
            }
        });
        return scoped;
    }

    /**
     * 获取模型数量
     */
    public int getModelCount() {
        return models.size();
    }

    /**
     * 检查模型是否存在
     */
    public boolean hasModel(String name) {
        return models.containsKey(name);
    }

    /**
     * 获取模型版本
     */
    public String getModelVersion(String name) {
        return modelVersions.get(name);
    }

    public String getModelVersion(String tenantId, String modelId) {
        String active = activeTenantVersions.get(scopeKey(tenantId, modelId));
        return active == null || active.isBlank() ? getModelVersion(modelId) : active;
    }

    /**
     * 应用 JVM 级模型热更新，供 broadcast operator 与 detector operator 共享。
     */
    /**
     * Downloads, verifies, initializes and warms a real inference model before
     * atomically exposing it to detector operators in the same TaskManager JVM.
     */
    public ApplyReceipt applyModelUpdate(String tenantId, String modelId, String modelName,
                                         String modelType, String version, String artifactUri,
                                         String expectedSha256, float threshold) throws Exception {
        requireValue(tenantId, "tenant_id");
        requireValue(modelId, "model_id");
        requireValue(version, "version");
        requireValue(artifactUri, "artifact_uri");
        if (!"xgboost".equalsIgnoreCase(modelType)) {
            throw new IllegalArgumentException("Unsupported production model_type: " + modelType);
        }

        String scopedKey = scopeKey(tenantId, modelId);
        Object lock = modelSwapLocks.computeIfAbsent(scopedKey, ignored -> new Object());
        synchronized (lock) {
            String currentVersion = activeTenantVersions.get(scopedKey);
            String currentSha = activeTenantArtifactSha256.get(scopedKey);
            if (version.equals(currentVersion)
                    && expectedSha256 != null && !expectedSha256.isBlank()
                    && expectedSha256.equalsIgnoreCase(currentSha)) {
                BehaviorModel current = activeTenantModels.get(scopedKey);
                float score = current instanceof XGBoostModelWrapper
                        ? ((XGBoostModelWrapper) current).getWarmupScore() : Float.NaN;
                return new ApplyReceipt(currentVersion, currentSha, score, false);
            }

            MinioModelLoader loader = artifactLoader();
            Path localArtifact = loader.downloadModel(artifactUri);
            String actualSha256 = loader.verifySha256(localArtifact, expectedSha256);
            String[] columns = new com.fasterxml.jackson.databind.ObjectMapper().readValue(
                    loader.getFeatureColumnsPath(localArtifact).toFile(), String[].class);
            XGBoostModelWrapper candidate = new XGBoostModelWrapper(
                    modelId, version, artifactUri, localArtifact, List.of(columns), threshold);
            candidate.initialize();

            BehaviorModel previous = activeTenantModels.put(scopedKey, candidate);
            String previousVersion = activeTenantVersions.put(scopedKey, version);
            activeTenantArtifactSha256.put(scopedKey, actualSha256);
            modelInvocations.computeIfAbsent(modelId, ignored -> new AtomicLong());
            modelErrors.computeIfAbsent(modelId, ignored -> new AtomicLong());
            if (previous != null && previous != candidate) {
                // Readers can hold a snapshot reference while the map is swapped.
                // Close after a grace period longer than the configured inference
                // timeout so no in-flight inference sees a disposed native booster.
                retiredModelCloser.schedule(() -> {
                    try {
                        previous.close();
                    } catch (Exception closeError) {
                        LOG.warn("Retired model close failed: {}", closeError.getMessage());
                    }
                }, 30, TimeUnit.SECONDS);
            }
            LOG.info("Applied real model artifact: tenant={}, modelId={}, modelName={}, type={}, "
                            + "version {} -> {}, sha256={}, warmupScore={}",
                    tenantId, modelId, modelName, modelType, previousVersion, version,
                    actualSha256, candidate.getWarmupScore());
            return new ApplyReceipt(previousVersion, actualSha256, candidate.getWarmupScore(), true);
        }
    }

    public void removeTenantModel(String tenantId, String modelId) {
        String key = scopeKey(tenantId, modelId);
        BehaviorModel removed = activeTenantModels.remove(key);
        activeTenantVersions.remove(key);
        activeTenantArtifactSha256.remove(key);
        if (removed != null) {
            retiredModelCloser.schedule(() -> {
                try {
                    removed.close();
                } catch (Exception e) {
                    LOG.warn("Failed to close deprecated model {}: {}", key, e.getMessage());
                }
            }, 30, TimeUnit.SECONDS);
        }
    }

    private MinioModelLoader artifactLoader() {
        if (artifactLoader == null) {
            artifactLoader = new MinioModelLoader(config.getModelPath(), config.getModelCacheSize());
            artifactLoader.initialize();
        }
        return artifactLoader;
    }

    private static void requireValue(String value, String name) {
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException(name + " is required");
        }
    }

    private static String scopePrefix(String tenantId) {
        return (tenantId == null ? "" : tenantId) + '\u001f';
    }

    private static String scopeKey(String tenantId, String modelId) {
        return scopePrefix(tenantId) + (modelId == null ? "" : modelId);
    }

    public static class ApplyReceipt implements Serializable {
        private static final long serialVersionUID = 1L;
        private final String previousVersion;
        private final String artifactSha256;
        private final float warmupScore;
        private final boolean switched;

        public ApplyReceipt(String previousVersion, String artifactSha256,
                            float warmupScore, boolean switched) {
            this.previousVersion = previousVersion;
            this.artifactSha256 = artifactSha256;
            this.warmupScore = warmupScore;
            this.switched = switched;
        }

        public String getPreviousVersion() { return previousVersion; }
        public String getArtifactSha256() { return artifactSha256; }
        public float getWarmupScore() { return warmupScore; }
        public boolean isSwitched() { return switched; }
    }

    /**
     * 记录模型调用
     */
    public void recordInvocation(String modelName) {
        modelInvocations.computeIfAbsent(modelName, ignored -> new AtomicLong()).incrementAndGet();
    }

    /**
     * 记录模型错误
     */
    public void recordError(String modelName) {
        modelErrors.computeIfAbsent(modelName, ignored -> new AtomicLong()).incrementAndGet();
    }

    /**
     * 获取模型调用次数
     */
    public long getInvocationCount(String modelName) {
        AtomicLong counter = modelInvocations.get(modelName);
        return counter != null ? counter.get() : 0;
    }

    /**
     * 获取模型错误次数
     */
    public long getErrorCount(String modelName) {
        AtomicLong counter = modelErrors.get(modelName);
        return counter != null ? counter.get() : 0;
    }

    /**
     * 获取模型健康状态
     */
    public ModelHealth getModelHealth(String modelName) {
        BehaviorModel model = models.get(modelName);
        if (model == null) {
            return ModelHealth.NOT_FOUND;
        }

        long invocations = getInvocationCount(modelName);
        long errors = getErrorCount(modelName);

        // 错误率超过 10% 视为不健康
        if (invocations > 100 && (double) errors / invocations > 0.1) {
            return ModelHealth.UNHEALTHY;
        }

        return model.isReady() ? ModelHealth.HEALTHY : ModelHealth.INITIALIZING;
    }

    /**
     * 获取所有模型的健康报告
     */
    public Map<String, ModelHealthReport> getHealthReport() {
        Map<String, ModelHealthReport> report = new ConcurrentHashMap<>();
        
        for (String modelName : models.keySet()) {
            BehaviorModel model = models.get(modelName);
            ModelHealthReport healthReport = new ModelHealthReport(
                    modelName,
                    getModelVersion(modelName),
                    model.getDescription(),
                    getModelHealth(modelName),
                    getInvocationCount(modelName),
                    getErrorCount(modelName),
                    System.currentTimeMillis() - initTime
            );
            report.put(modelName, healthReport);
        }
        
        return report;
    }

    /**
     * 打印健康报告
     */
    public void printHealthReport() {
        LOG.info("========== Model Health Report ==========");
        for (ModelHealthReport report : getHealthReport().values()) {
            LOG.info("  {}: health={}, invocations={}, errors={}, errorRate={:.4f}%",
                    report.getModelName(),
                    report.getHealth(),
                    report.getInvocations(),
                    report.getErrors(),
                    report.getErrorRate() * 100);
        }
        LOG.info("==========================================");
    }

    /**
     * 启动模型重加载调度器
     */
    private void startReloadScheduler() {
        if (config.getModelReloadIntervalMs() > 0) {
            reloadScheduler = Executors.newSingleThreadScheduledExecutor(r -> {
                Thread t = new Thread(r, "model-reload-scheduler");
                t.setDaemon(true);
                return t;
            });

            reloadScheduler.scheduleAtFixedRate(
                    this::checkAndReloadModels,
                    config.getModelReloadIntervalMs(),
                    config.getModelReloadIntervalMs(),
                    TimeUnit.MILLISECONDS
            );

            LOG.info("Model reload scheduler started with interval: {}ms", 
                    config.getModelReloadIntervalMs());
        }
    }

    /**
     * 检查并重加载模型
     */
    private void checkAndReloadModels() {
        LOG.debug("Checking for model updates...");
        
        for (Map.Entry<String, BehaviorModel> entry : models.entrySet()) {
            String name = entry.getKey();
            BehaviorModel model = entry.getValue();
            
            try {
                String currentVersion = modelVersions.get(name);
                String latestVersion = model.checkForUpdate();
                
                if (latestVersion != null && !latestVersion.equals(currentVersion)) {
                    LOG.info("Reloading model {} from version {} to {}", 
                            name, currentVersion, latestVersion);
                    
                    // 创建新模型实例
                    BehaviorModel newModel = model.reload();
                    
                    // 替换旧模型
                    models.put(name, newModel);
                    modelVersions.put(name, latestVersion);
                    
                    // 关闭旧模型
                    model.close();
                    
                    LOG.info("Model {} reloaded successfully", name);
                }
            } catch (Exception e) {
                LOG.error("Failed to reload model {}: {}", name, e.getMessage(), e);
            }
        }
    }

    @Override
    public void close() {
        LOG.info("Closing model registry...");

        // 打印最终健康报告
        printHealthReport();

        // 停止重加载调度器
        if (reloadScheduler != null) {
            reloadScheduler.shutdown();
            try {
                if (!reloadScheduler.awaitTermination(5, TimeUnit.SECONDS)) {
                    reloadScheduler.shutdownNow();
                }
            } catch (InterruptedException e) {
                reloadScheduler.shutdownNow();
                Thread.currentThread().interrupt();
            }
        }

        // 关闭所有模型
        for (Map.Entry<String, BehaviorModel> entry : models.entrySet()) {
            try {
                entry.getValue().close();
                LOG.info("Closed model: {}", entry.getKey());
            } catch (Exception e) {
                LOG.error("Failed to close model {}: {}", entry.getKey(), e.getMessage(), e);
            }
        }

        models.clear();
        if (artifactLoader != null) {
            artifactLoader.close();
        }
        LOG.info("Model registry closed");
    }

    /**
     * 模型健康状态枚举
     */
    public enum ModelHealth {
        HEALTHY,
        UNHEALTHY,
        INITIALIZING,
        NOT_FOUND
    }

    /**
     * 模型健康报告
     */
    public static class ModelHealthReport implements Serializable {
        private static final long serialVersionUID = 1L;

        private final String modelName;
        private final String version;
        private final String description;
        private final ModelHealth health;
        private final long invocations;
        private final long errors;
        private final long uptimeMs;

        public ModelHealthReport(String modelName, String version, String description,
                                 ModelHealth health, long invocations, long errors, long uptimeMs) {
            this.modelName = modelName;
            this.version = version;
            this.description = description;
            this.health = health;
            this.invocations = invocations;
            this.errors = errors;
            this.uptimeMs = uptimeMs;
        }

        public String getModelName() { return modelName; }
        public String getVersion() { return version; }
        public String getDescription() { return description; }
        public ModelHealth getHealth() { return health; }
        public long getInvocations() { return invocations; }
        public long getErrors() { return errors; }
        public long getUptimeMs() { return uptimeMs; }
        
        public double getErrorRate() {
            return invocations > 0 ? (double) errors / invocations : 0.0;
        }

        @Override
        public String toString() {
            return String.format("ModelHealthReport{model=%s, version=%s, health=%s, " +
                    "invocations=%d, errors=%d, errorRate=%.4f, uptimeMs=%d}",
                    modelName, version, health, invocations, errors, getErrorRate(), uptimeMs);
        }
    }
}
