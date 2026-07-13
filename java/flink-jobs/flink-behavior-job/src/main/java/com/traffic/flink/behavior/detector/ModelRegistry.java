////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelRegistry.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.*;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
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
    private static final Map<String, String> activeModelVersions = new ConcurrentHashMap<>();

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
        String activeVersion = activeModelVersions.get(name);
        if (activeVersion != null && !activeVersion.isEmpty()) {
            return activeVersion;
        }
        return modelVersions.get(name);
    }

    /**
     * 应用 JVM 级模型热更新，供 broadcast operator 与 detector operator 共享。
     */
    public static void applyGlobalModelUpdate(String modelName, String modelType, String version, String artifactUri) {
        if (modelName == null || modelName.isEmpty()) {
            LOG.warn("Ignoring global model update with empty model name: version={}, artifact={}", version, artifactUri);
            return;
        }
        if (version == null || version.isEmpty()) {
            LOG.warn("Ignoring global model update for {} with empty version", modelName);
            return;
        }

        String previousVersion = activeModelVersions.put(modelName, version);
        LOG.info("Applied hot model update: model={}, type={}, version {} -> {}, artifact={}",
                modelName, modelType, previousVersion, version, artifactUri);
    }

    /**
     * 应用来自 model-updates topic 的模型热更新事件。
     *
     * 当前内置行为模型以规则/统计模型为主，外部 artifact 由注册中心追踪；
     * 这里将活跃版本写入运行时 registry，保证所有 TaskManager 对新版本可见。
     */
    public void applyModelUpdate(String modelName, String modelType, String version, String artifactUri) {
        if (modelName == null || modelName.isEmpty()) {
            LOG.warn("Ignoring model update with empty model name: version={}, artifact={}", version, artifactUri);
            return;
        }
        if (version == null || version.isEmpty()) {
            LOG.warn("Ignoring model update for {} with empty version", modelName);
            return;
        }

        String previousVersion = modelVersions.put(modelName, version);
        activeModelVersions.put(modelName, version);
        if (models.containsKey(modelName)) {
            LOG.info("Applied hot model update: model={}, type={}, version {} -> {}, artifact={}",
                    modelName, modelType, previousVersion, version, artifactUri);
        } else {
            LOG.warn("Applied version marker for unknown model: model={}, type={}, version={}, artifact={}",
                    modelName, modelType, version, artifactUri);
        }
    }

    /**
     * 记录模型调用
     */
    public void recordInvocation(String modelName) {
        AtomicLong counter = modelInvocations.get(modelName);
        if (counter != null) {
            counter.incrementAndGet();
        }
    }

    /**
     * 记录模型错误
     */
    public void recordError(String modelName) {
        AtomicLong counter = modelErrors.get(modelName);
        if (counter != null) {
            counter.incrementAndGet();
        }
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
