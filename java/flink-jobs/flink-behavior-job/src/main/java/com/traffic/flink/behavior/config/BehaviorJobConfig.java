////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/config/BehaviorJobConfig.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.config;

import org.apache.flink.api.java.utils.ParameterTool;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.InputStream;
import java.io.Serializable;
import java.time.Duration;
import java.util.Arrays;
import java.util.HashSet;
import java.util.Properties;
import java.util.Set;

/**
 * Behavior Detection Job 配置类
 * 
 * 支持从以下来源加载配置（优先级从高到低）：
 * 1. 命令行参数
 * 2. 环境变量
 * 3. 配置文件 (behavior-job.properties)
 * 4. 默认值
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
public class BehaviorJobConfig implements Serializable {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BehaviorJobConfig.class);

    // ==================== Kafka 配置 ====================
    private final String kafkaBrokers;
    private final String inputTopic;
    private final String outputTopic;
    private final String dlqTopic;
    private final String modelUpdateTopic;
    private final String modelAppliedTopic;
    private final String modelShadowObservationTopic;
    private final String modelShadowFeatureConsumerGroupId;
    private final String modelShadowUpdateConsumerGroupId;
    private final String consumerGroupId;

    // ==================== ClickHouse 配置 ====================
    private final String clickhouseUrl;
    private final String clickhouseDatabase;
    private final String clickhouseTable;
    private final String clickhouseUser;
    private final String clickhousePassword;
    private final int clickhouseBatchSize;
    private final long clickhouseBatchIntervalMs;

    // ==================== Checkpoint 配置 ====================
    private final String checkpointPath;
    private final long checkpointIntervalMs;
    private final long checkpointTimeoutMs;
    private final long checkpointMinPauseMs;

    // ==================== 水印配置 ====================
    private final long watermarkDelayMs;
    private final long allowedLatenessMs;

    // ==================== 性能配置 ====================
    private final int parallelism;
    private final int maxParallelism;

    // ==================== 模型配置 ====================
    private final String modelPath;
    private final String modelVersion;
    private final Set<String> enabledModels;
    private final String detectionMode;
    private final String knownProfileId;
    private final String knownProfileSha256;
    private final boolean modelHotUpdateEnabled;
    private final boolean modelUpdateConsumerEnabled;
    private final String modelConsumerDeploymentId;
    private final String modelConsumerProfileSha256;
    private final String modelRuntimeContract;
    private final String modelRuntimeVersion;
    private final int modelFeatureSchemaVersion;
    private final int modelGraphSchemaVersion;
    private final String modelSigningPublicKeyFile;
    private final String modelSigningPublicKeyPemBase64;
    private final long modelReloadIntervalMs;
    private final boolean modelCacheEnabled;
    private final int modelCacheSize;

    // ==================== 推理配置 ====================
    private final boolean asyncInferenceEnabled;
    private final long asyncTimeoutMs;
    private final int asyncCapacity;
    private final int asyncMaxRetries;
    private final int inferenceThreads;
    private final int batchInferenceSize;
    private final boolean modelShadowEvaluationEnabled;
    private final boolean modelShadowObservationOnly;
    private final double modelShadowSampleRate;
    private final long modelShadowChallengerTimeoutMs;
    private final long modelShadowPackageLoadTimeoutMs;
    private final int modelShadowChallengerThreads;
    private final int modelShadowChallengerQueueCapacity;

    // ==================== 检测配置 ====================
    private final float minConfidenceThreshold;
    private final float highConfidenceThreshold;
    private final boolean multiLabelEnabled;
    private final int maxLabelsPerDetection;

    // ==================== 各模型阈值配置 ====================
    private final float scanThreshold;
    private final float tunnelThreshold;
    private final float dgaThreshold;
    private final float encryptedTrafficThreshold;
    private final float anomalyThreshold;
    private final float c2Threshold;
    private final float dataExfilThreshold;
    // ========== 新增阈值 ==========
    private final float botnetThreshold;
    private final float malwareThreshold;
    private final float phishingThreshold;

    // ==================== 调试配置 ====================
    private final boolean debugPrintEnabled;
    private final boolean metricsEnabled;
    private final int metricsIntervalSeconds;

    private BehaviorJobConfig(Builder builder) {
        this.kafkaBrokers = builder.kafkaBrokers;
        this.inputTopic = builder.inputTopic;
        this.outputTopic = builder.outputTopic;
        this.dlqTopic = builder.dlqTopic;
        this.modelUpdateTopic = builder.modelUpdateTopic;
        this.modelAppliedTopic = builder.modelAppliedTopic;
        this.modelShadowObservationTopic = builder.modelShadowObservationTopic;
        this.modelShadowFeatureConsumerGroupId = builder.modelShadowFeatureConsumerGroupId;
        this.modelShadowUpdateConsumerGroupId = builder.modelShadowUpdateConsumerGroupId;
        this.consumerGroupId = builder.consumerGroupId;
        this.clickhouseUrl = builder.clickhouseUrl;
        this.clickhouseDatabase = builder.clickhouseDatabase;
        this.clickhouseTable = builder.clickhouseTable;
        this.clickhouseUser = builder.clickhouseUser;
        this.clickhousePassword = builder.clickhousePassword;
        this.clickhouseBatchSize = builder.clickhouseBatchSize;
        this.clickhouseBatchIntervalMs = builder.clickhouseBatchIntervalMs;
        this.checkpointPath = builder.checkpointPath;
        this.checkpointIntervalMs = builder.checkpointIntervalMs;
        this.checkpointTimeoutMs = builder.checkpointTimeoutMs;
        this.checkpointMinPauseMs = builder.checkpointMinPauseMs;
        this.watermarkDelayMs = builder.watermarkDelayMs;
        this.allowedLatenessMs = builder.allowedLatenessMs;
        this.parallelism = builder.parallelism;
        this.maxParallelism = builder.maxParallelism;
        this.modelPath = builder.modelPath;
        this.modelVersion = builder.modelVersion;
        this.enabledModels = builder.enabledModels;
        this.detectionMode = builder.detectionMode;
        this.knownProfileId = builder.knownProfileId;
        this.knownProfileSha256 = builder.knownProfileSha256;
        this.modelHotUpdateEnabled = builder.modelHotUpdateEnabled;
        this.modelUpdateConsumerEnabled = builder.modelUpdateConsumerEnabled;
        this.modelConsumerDeploymentId = builder.modelConsumerDeploymentId;
        this.modelConsumerProfileSha256 = builder.modelConsumerProfileSha256;
        this.modelRuntimeContract = builder.modelRuntimeContract;
        this.modelRuntimeVersion = builder.modelRuntimeVersion;
        this.modelFeatureSchemaVersion = builder.modelFeatureSchemaVersion;
        this.modelGraphSchemaVersion = builder.modelGraphSchemaVersion;
        this.modelSigningPublicKeyFile = builder.modelSigningPublicKeyFile;
        this.modelSigningPublicKeyPemBase64 = builder.modelSigningPublicKeyPemBase64;
        this.modelReloadIntervalMs = builder.modelReloadIntervalMs;
        this.modelCacheEnabled = builder.modelCacheEnabled;
        this.modelCacheSize = builder.modelCacheSize;
        this.asyncInferenceEnabled = builder.asyncInferenceEnabled;
        this.asyncTimeoutMs = builder.asyncTimeoutMs;
        this.asyncCapacity = builder.asyncCapacity;
        this.asyncMaxRetries = builder.asyncMaxRetries;
        this.inferenceThreads = builder.inferenceThreads;
        this.batchInferenceSize = builder.batchInferenceSize;
        this.modelShadowEvaluationEnabled = builder.modelShadowEvaluationEnabled;
        this.modelShadowObservationOnly = builder.modelShadowObservationOnly;
        this.modelShadowSampleRate = builder.modelShadowSampleRate;
        this.modelShadowChallengerTimeoutMs = builder.modelShadowChallengerTimeoutMs;
        this.modelShadowPackageLoadTimeoutMs = builder.modelShadowPackageLoadTimeoutMs;
        this.modelShadowChallengerThreads = builder.modelShadowChallengerThreads;
        this.modelShadowChallengerQueueCapacity = builder.modelShadowChallengerQueueCapacity;
        this.minConfidenceThreshold = builder.minConfidenceThreshold;
        this.highConfidenceThreshold = builder.highConfidenceThreshold;
        this.multiLabelEnabled = builder.multiLabelEnabled;
        this.maxLabelsPerDetection = builder.maxLabelsPerDetection;
        this.scanThreshold = builder.scanThreshold;
        this.tunnelThreshold = builder.tunnelThreshold;
        this.dgaThreshold = builder.dgaThreshold;
        this.encryptedTrafficThreshold = builder.encryptedTrafficThreshold;
        this.anomalyThreshold = builder.anomalyThreshold;
        this.c2Threshold = builder.c2Threshold;
        this.dataExfilThreshold = builder.dataExfilThreshold;
        this.botnetThreshold = builder.botnetThreshold;
        this.malwareThreshold = builder.malwareThreshold;
        this.phishingThreshold = builder.phishingThreshold;
        this.debugPrintEnabled = builder.debugPrintEnabled;
        this.metricsEnabled = builder.metricsEnabled;
        this.metricsIntervalSeconds = builder.metricsIntervalSeconds;
    }

    /**
     * 从命令行参数构建配置
     */
    public static BehaviorJobConfig fromArgs(String[] args) {
        // 首先加载配置文件
        Properties fileProps = loadPropertiesFile("behavior-job.properties");
        if (fileProps.isEmpty()) {
            // Retained compatibility for the historically misnamed packaged resource.
            fileProps = loadPropertiesFile("feature-job.properties");
        }
        
        // 然后加载命令行参数
        ParameterTool params = ParameterTool.fromArgs(args);

        // 合并配置（命令行优先）
        return new Builder()
                // Kafka
                .kafkaBrokers(getConfig(params, fileProps, "kafka.brokers", 
                        getEnv("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092")))
                .inputTopic(getConfig(params, fileProps, "kafka.input.topic", 
                        getEnv("KAFKA_INPUT_TOPIC", "feature.stat.v1")))
                .outputTopic(getConfig(params, fileProps, "kafka.output.topic", 
                        getEnv("KAFKA_OUTPUT_TOPIC", "detections.v1")))
                .dlqTopic(getConfig(params, fileProps, "kafka.dlq.topic",
                        getEnv("KAFKA_DLQ_TOPIC", "dlq.v1")))
                .modelUpdateTopic(getConfig(params, fileProps, "kafka.model.update.topic",
                        getEnv("KAFKA_MODEL_UPDATE_TOPIC",
                                getEnv("MODEL_UPDATE_TOPIC",
                                        getEnv("KAFKA_MODEL_TOPIC", "model-updates")))))
                .modelAppliedTopic(getConfig(params, fileProps, "kafka.model.applied.topic",
                        getEnv("KAFKA_MODEL_APPLIED_TOPIC", "model-update-applied.v1")))
                .modelShadowObservationTopic(getConfigEnvironmentFirst(
                        params, fileProps, "kafka.model.shadow.observation.topic",
                        "KAFKA_MODEL_SHADOW_OBSERVATION_TOPIC",
                        "model-shadow-observations.v1"))
                .modelShadowFeatureConsumerGroupId(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.feature.consumer.group.id",
                        "MODEL_SHADOW_FEATURE_CONSUMER_GROUP_ID",
                        "flink-behavior-job-champion-challenger-features"))
                .modelShadowUpdateConsumerGroupId(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.update.consumer.group.id",
                        "MODEL_SHADOW_UPDATE_CONSUMER_GROUP_ID",
                        "flink-behavior-job-champion-challenger-model-updates"))
                .consumerGroupId(getConfig(params, fileProps, "kafka.group.id", 
                        getEnv("KAFKA_GROUP_ID", "flink-behavior-job")))
                
                // ClickHouse
                .clickhouseUrl(getConfig(params, fileProps, "clickhouse.url", 
                        getEnv("CLICKHOUSE_URL", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123")))
                .clickhouseDatabase(getConfig(params, fileProps, "clickhouse.database", 
                        getEnv("CLICKHOUSE_DATABASE", "traffic")))
                .clickhouseTable(getConfig(params, fileProps, "clickhouse.table", 
                        getEnv("CLICKHOUSE_TABLE", "detections_behavior")))
                .clickhouseUser(getConfig(params, fileProps, "clickhouse.user", 
                        getEnv("CLICKHOUSE_USER", "default")))
                .clickhousePassword(getConfig(params, fileProps, "clickhouse.password", 
                        getEnv("CLICKHOUSE_PASSWORD", "")))
                .clickhouseBatchSize(getConfigInt(params, fileProps, "clickhouse.batch.size", 5000))
                .clickhouseBatchIntervalMs(getConfigLong(params, fileProps, "clickhouse.batch.interval.ms", 2000L))
                
                // Checkpoint
                .checkpointPath(getConfig(params, fileProps, "checkpoint.path", 
                        getEnv("CHECKPOINT_PATH", "s3://flink-checkpoints/checkpoints/behavior-job")))
                .checkpointIntervalMs(getConfigLong(params, fileProps, "checkpoint.interval.ms", 60000L))
                .checkpointTimeoutMs(getConfigLong(params, fileProps, "checkpoint.timeout.ms", 180000L))
                .checkpointMinPauseMs(getConfigLong(params, fileProps, "checkpoint.min.pause.ms", 30000L))
                
                // 水印
                .watermarkDelayMs(getConfigLong(params, fileProps, "watermark.delay.ms", 10000L))
                .allowedLatenessMs(getConfigLong(params, fileProps, "allowed.lateness.ms", 60000L))
                
                // 性能
                .parallelism(getConfigInt(params, fileProps, "parallelism", 4))
                .maxParallelism(getConfigInt(params, fileProps, "max.parallelism", 128))
                
                // 模型
                .modelPath(getConfig(params, fileProps, "model.path", 
                        getEnv("MODEL_PATH", "/opt/flink/models")))
                .modelVersion(getConfig(params, fileProps, "model.version", 
                        getEnv("MODEL_VERSION", "v1.0")))
                .enabledModels(parseModels(getConfig(params, fileProps, "model.enabled", 
                        "scan,tunnel,dga,encrypted,anomaly,c2,data_exfil,botnet,malware,phishing")))
                .detectionMode(getConfig(params, fileProps, "detection.mode",
                        getEnv("DETECTION_MODE", "off")))
                .knownProfileId(getConfig(params, fileProps, "detection.known.profile.id",
                        "m04-known-behavior-v1"))
                .knownProfileSha256(getConfig(params, fileProps, "detection.known.profile.sha256",
                        "3308cd498548716c68b79c8f665f5a1cb6d7b1d95769234853bbf1e9f7a03cdb"))
                .modelHotUpdateEnabled(getConfigBoolean(
                        params, fileProps, "model.hot.update.enabled", false))
                .modelUpdateConsumerEnabled(getConfigBoolean(
                        params, fileProps, "model.update.consumer.v1.enabled",
                        Boolean.parseBoolean(getEnv("MODEL_UPDATE_CONSUMER_V1_ENABLED", "false"))))
                .modelConsumerDeploymentId(getConfig(params, fileProps,
                        "model.consumer.deployment.id", getEnv("MODEL_CONSUMER_DEPLOYMENT_ID", "")))
                .modelConsumerProfileSha256(getConfig(params, fileProps,
                        "model.consumer.profile.sha256", getEnv("MODEL_CONSUMER_PROFILE_SHA256", "")))
                .modelRuntimeContract(getConfig(params, fileProps,
                        "model.runtime.contract", getEnv("MODEL_RUNTIME_CONTRACT", "traffic.behavior.inference.v1")))
                .modelRuntimeVersion(getConfig(params, fileProps,
                        "model.runtime.version", getEnv("MODEL_RUNTIME_VERSION", "1.0.0")))
                .modelFeatureSchemaVersion(getConfigInt(params, fileProps,
                        "model.feature.schema.version", getEnvInt("MODEL_FEATURE_SCHEMA_VERSION", 1)))
                .modelGraphSchemaVersion(getConfigInt(params, fileProps,
                        "model.graph.schema.version", getEnvInt("MODEL_GRAPH_SCHEMA_VERSION", 1)))
                .modelSigningPublicKeyFile(getConfig(params, fileProps,
                        "model.signing.public.key.file", getEnv("MODEL_SIGNING_PUBLIC_KEY_FILE", "")))
                .modelSigningPublicKeyPemBase64(getConfig(params, fileProps,
                        "model.signing.public.key.pem.base64",
                        getEnv("MODEL_SIGNING_PUBLIC_KEY_PEM_BASE64", "")))
                .modelReloadIntervalMs(getConfigLong(params, fileProps, "model.reload.interval.ms", 0L))
                .modelCacheEnabled(getConfigBoolean(params, fileProps, "model.cache.enabled", true))
                .modelCacheSize(getConfigInt(params, fileProps, "model.cache.size", 1000))
                
                // 推理
                .asyncInferenceEnabled(getConfigBoolean(params, fileProps, "inference.async.enabled", true))
                .asyncTimeoutMs(getConfigLong(params, fileProps, "inference.async.timeout.ms", 5000L))
                .asyncCapacity(getConfigInt(params, fileProps, "inference.async.capacity", 100))
                .asyncMaxRetries(getConfigInt(params, fileProps, "inference.async.max.retries", 2))
                .inferenceThreads(getConfigInt(params, fileProps, "inference.threads", 4))
                .batchInferenceSize(getConfigInt(params, fileProps, "inference.batch.size", 32))
                .modelShadowEvaluationEnabled(Boolean.parseBoolean(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.evaluation.v1.enabled",
                        "MODEL_SHADOW_EVALUATION_V1_ENABLED", "false")))
                .modelShadowObservationOnly(Boolean.parseBoolean(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.observation.only",
                        "MODEL_SHADOW_OBSERVATION_ONLY", "false")))
                .modelShadowSampleRate(Double.parseDouble(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.sample.rate",
                        "MODEL_SHADOW_SAMPLE_RATE", "0.0")))
                .modelShadowChallengerTimeoutMs(Long.parseLong(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.challenger.timeout.ms",
                        "MODEL_SHADOW_CHALLENGER_TIMEOUT_MS", "250")))
                .modelShadowPackageLoadTimeoutMs(Long.parseLong(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.package.load.timeout.ms",
                        "MODEL_SHADOW_PACKAGE_LOAD_TIMEOUT_MS", "120000")))
                .modelShadowChallengerThreads(Integer.parseInt(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.challenger.threads",
                        "MODEL_SHADOW_CHALLENGER_THREADS", "2")))
                .modelShadowChallengerQueueCapacity(Integer.parseInt(getConfigEnvironmentFirst(
                        params, fileProps, "model.shadow.challenger.queue.capacity",
                        "MODEL_SHADOW_CHALLENGER_QUEUE_CAPACITY", "64")))
                
                // 检测
                .minConfidenceThreshold(getConfigFloat(params, fileProps, "detection.min.confidence", 0.5f))
                .highConfidenceThreshold(getConfigFloat(params, fileProps, "detection.high.confidence", 0.8f))
                .multiLabelEnabled(getConfigBoolean(params, fileProps, "detection.multi.label", true))
                .maxLabelsPerDetection(getConfigInt(params, fileProps, "detection.max.labels", 5))
                
                // 各模型阈值
                .scanThreshold(getConfigFloat(params, fileProps, "threshold.scan", 0.7f))
                .tunnelThreshold(getConfigFloat(params, fileProps, "threshold.tunnel", 0.75f))
                .dgaThreshold(getConfigFloat(params, fileProps, "threshold.dga", 0.8f))
                .encryptedTrafficThreshold(getConfigFloat(params, fileProps, "threshold.encrypted", 0.7f))
                .anomalyThreshold(getConfigFloat(params, fileProps, "threshold.anomaly", 0.6f))
                .c2Threshold(getConfigFloat(params, fileProps, "threshold.c2", 0.75f))
                .dataExfilThreshold(getConfigFloat(params, fileProps, "threshold.data_exfil", 0.7f))
                // 新增阈值
                .botnetThreshold(getConfigFloat(params, fileProps, "threshold.botnet", 0.7f))
                .malwareThreshold(getConfigFloat(params, fileProps, "threshold.malware", 0.75f))
                .phishingThreshold(getConfigFloat(params, fileProps, "threshold.phishing", 0.7f))
                
                // 调试
                .debugPrintEnabled(getConfigBoolean(params, fileProps, "debug.print", false))
                .metricsEnabled(getConfigBoolean(params, fileProps, "metrics.enabled", true))
                .metricsIntervalSeconds(getConfigInt(params, fileProps, "metrics.interval.seconds", 60))
                
                .build();
    }

    /**
     * 加载配置文件
     */
    private static Properties loadPropertiesFile(String filename) {
        Properties props = new Properties();
        try (InputStream is = BehaviorJobConfig.class.getClassLoader().getResourceAsStream(filename)) {
            if (is != null) {
                props.load(is);
                LOG.info("Loaded configuration from {}", filename);
            } else {
                LOG.warn("Configuration file {} not found, using defaults", filename);
            }
        } catch (Exception e) {
            LOG.warn("Failed to load configuration file {}: {}", filename, e.getMessage());
        }
        return props;
    }

    /**
     * 获取环境变量
     */
    private static String getEnv(String key, String defaultValue) {
        String value = System.getenv(key);
        return value != null && !value.isEmpty() ? value : defaultValue;
    }

    private static int getEnvInt(String key, int defaultValue) {
        String value = getEnv(key, "");
        if (value.isBlank()) {
            return defaultValue;
        }
        try {
            return Integer.parseInt(value);
        } catch (NumberFormatException error) {
            throw new IllegalArgumentException(key + " must be an integer", error);
        }
    }

    /**
     * 获取配置值（优先级：命令行 > 配置文件 > 默认值）
     */
    private static String getConfig(ParameterTool params, Properties props, String key, String defaultValue) {
        if (params.has(key)) {
            return params.get(key);
        }
        return props.getProperty(key, defaultValue);
    }

    private static String getConfigEnvironmentFirst(
            ParameterTool params, Properties props, String key, String environmentKey,
            String defaultValue) {
        if (params.has(key)) return params.get(key);
        String environmentValue = System.getenv(environmentKey);
        if (environmentValue != null && !environmentValue.isBlank()) return environmentValue;
        return props.getProperty(key, defaultValue);
    }

    private static int getConfigInt(ParameterTool params, Properties props, String key, int defaultValue) {
        String value = getConfig(params, props, key, null);
        if (value != null) {
            try {
                return Integer.parseInt(value);
            } catch (NumberFormatException e) {
                LOG.warn("Invalid integer value for {}: {}, using default: {}", key, value, defaultValue);
            }
        }
        return defaultValue;
    }

    private static long getConfigLong(ParameterTool params, Properties props, String key, long defaultValue) {
        String value = getConfig(params, props, key, null);
        if (value != null) {
            try {
                return Long.parseLong(value);
            } catch (NumberFormatException e) {
                LOG.warn("Invalid long value for {}: {}, using default: {}", key, value, defaultValue);
            }
        }
        return defaultValue;
    }

    private static float getConfigFloat(ParameterTool params, Properties props, String key, float defaultValue) {
        String value = getConfig(params, props, key, null);
        if (value != null) {
            try {
                return Float.parseFloat(value);
            } catch (NumberFormatException e) {
                LOG.warn("Invalid float value for {}: {}, using default: {}", key, value, defaultValue);
            }
        }
        return defaultValue;
    }

    private static boolean getConfigBoolean(ParameterTool params, Properties props, String key, boolean defaultValue) {
        String value = getConfig(params, props, key, null);
        if (value != null) {
            return Boolean.parseBoolean(value);
        }
        return defaultValue;
    }

    private static Set<String> parseModels(String value) {
        Set<String> models = new HashSet<>();
        if (value != null && !value.isEmpty()) {
            String[] parts = value.split(",");
            for (String part : parts) {
                String trimmed = part.trim().toLowerCase();
                if (!trimmed.isEmpty()) {
                    models.add(trimmed);
                }
            }
        }
        return models;
    }

    // ==================== Getters ====================

    public String getKafkaBrokers() { return kafkaBrokers; }
    public String getInputTopic() { return inputTopic; }
    public String getOutputTopic() { return outputTopic; }
    public String getDlqTopic() { return dlqTopic; }
    public String getModelUpdateTopic() { return modelUpdateTopic; }
    public String getModelAppliedTopic() { return modelAppliedTopic; }
    public String getModelShadowObservationTopic() { return modelShadowObservationTopic; }
    public String getModelShadowFeatureConsumerGroupId() { return modelShadowFeatureConsumerGroupId; }
    public String getModelShadowUpdateConsumerGroupId() { return modelShadowUpdateConsumerGroupId; }
    public String getConsumerGroupId() { return consumerGroupId; }
    public String getClickhouseUrl() { return clickhouseUrl; }
    public String getClickhouseDatabase() { return clickhouseDatabase; }
    public String getClickhouseTable() { return clickhouseTable; }
    public String getClickhouseUser() { return clickhouseUser; }
    public String getClickhousePassword() { return clickhousePassword; }
    public int getClickhouseBatchSize() { return clickhouseBatchSize; }
    public long getClickhouseBatchIntervalMs() { return clickhouseBatchIntervalMs; }
    public String getCheckpointPath() { return checkpointPath; }
    public long getCheckpointIntervalMs() { return checkpointIntervalMs; }
    public long getCheckpointTimeoutMs() { return checkpointTimeoutMs; }
    public long getCheckpointMinPauseMs() { return checkpointMinPauseMs; }
    public long getWatermarkDelayMs() { return watermarkDelayMs; }
    public Duration getWatermarkDelayDuration() { return Duration.ofMillis(watermarkDelayMs); }
    public long getAllowedLatenessMs() { return allowedLatenessMs; }
    public int getParallelism() { return parallelism; }
    public int getMaxParallelism() { return maxParallelism; }
    public String getModelPath() { return modelPath; }
    public String getModelVersion() { return modelVersion; }
    public Set<String> getEnabledModels() { return enabledModels; }
    public String getDetectionMode() { return detectionMode; }
    public String getKnownProfileId() { return knownProfileId; }
    public String getKnownProfileSha256() { return knownProfileSha256; }
    public boolean isModelHotUpdateEnabled() { return modelHotUpdateEnabled; }
    public boolean isModelUpdateConsumerEnabled() { return modelUpdateConsumerEnabled; }
    public boolean isModelUpdateConsumerConfigured() { return modelUpdateConsumerEnabled || modelHotUpdateEnabled; }
    public String getModelConsumerDeploymentId() { return modelConsumerDeploymentId; }
    public String getModelConsumerProfileSha256() { return modelConsumerProfileSha256; }
    public String getModelRuntimeContract() { return modelRuntimeContract; }
    public String getModelRuntimeVersion() { return modelRuntimeVersion; }
    public int getModelFeatureSchemaVersion() { return modelFeatureSchemaVersion; }
    public int getModelGraphSchemaVersion() { return modelGraphSchemaVersion; }
    public String getModelSigningPublicKeyFile() { return modelSigningPublicKeyFile; }
    public String getModelSigningPublicKeyPemBase64() { return modelSigningPublicKeyPemBase64; }

    public void validateModelUpdateConsumerConfig() {
        if (!isModelUpdateConsumerConfigured()) {
            return;
        }
        if (modelConsumerDeploymentId == null || modelConsumerDeploymentId.isBlank()) {
            throw new IllegalArgumentException("MODEL_CONSUMER_DEPLOYMENT_ID is required when the model-update consumer is enabled");
        }
        if (modelConsumerProfileSha256 == null || !modelConsumerProfileSha256.matches("^[0-9a-f]{64}$")) {
            throw new IllegalArgumentException("MODEL_CONSUMER_PROFILE_SHA256 must be a lowercase SHA-256 digest");
        }
        if (!"traffic.behavior.inference.v1".equals(modelRuntimeContract)) {
            throw new IllegalArgumentException("unsupported model runtime contract: " + modelRuntimeContract);
        }
        if (modelRuntimeVersion == null || !modelRuntimeVersion.matches("^[0-9]+\\.[0-9]+\\.[0-9]+$")) {
            throw new IllegalArgumentException("MODEL_RUNTIME_VERSION must be semantic major.minor.patch");
        }
        if (modelFeatureSchemaVersion <= 0 || modelGraphSchemaVersion <= 0) {
            throw new IllegalArgumentException("model feature and graph schema versions must be positive");
        }
        boolean hasFile = modelSigningPublicKeyFile != null && !modelSigningPublicKeyFile.isBlank();
        boolean hasInline = modelSigningPublicKeyPemBase64 != null && !modelSigningPublicKeyPemBase64.isBlank();
        if (hasFile == hasInline) {
            throw new IllegalArgumentException(
                    "exactly one trusted model signing public key file or base64 PEM is required");
        }
    }
    public long getModelReloadIntervalMs() { return modelReloadIntervalMs; }
    public boolean isModelCacheEnabled() { return modelCacheEnabled; }
    public int getModelCacheSize() { return modelCacheSize; }
    public boolean isAsyncInferenceEnabled() { return asyncInferenceEnabled; }
    public long getAsyncTimeoutMs() { return asyncTimeoutMs; }
    public int getAsyncCapacity() { return asyncCapacity; }
    public int getAsyncMaxRetries() { return asyncMaxRetries; }
    public int getInferenceThreads() { return inferenceThreads; }
    public int getBatchInferenceSize() { return batchInferenceSize; }
    public boolean isModelShadowEvaluationEnabled() { return modelShadowEvaluationEnabled; }
    public boolean isModelShadowObservationOnly() { return modelShadowObservationOnly; }
    public double getModelShadowSampleRate() { return modelShadowSampleRate; }
    public long getModelShadowChallengerTimeoutMs() { return modelShadowChallengerTimeoutMs; }
    public long getModelShadowPackageLoadTimeoutMs() { return modelShadowPackageLoadTimeoutMs; }
    public int getModelShadowChallengerThreads() { return modelShadowChallengerThreads; }
    public int getModelShadowChallengerQueueCapacity() { return modelShadowChallengerQueueCapacity; }

    public void validateChampionChallengerShadowConfig() {
        if (modelShadowObservationOnly && !modelShadowEvaluationEnabled) {
            throw new IllegalArgumentException(
                    "shadow observation-only mode requires champion/challenger evaluation");
        }
        if (!modelShadowEvaluationEnabled) return;
        if (!"known_frozen".equalsIgnoreCase(detectionMode)) {
            throw new IllegalArgumentException(
                    "champion/challenger shadow requires the frozen champion detection mode");
        }
        if (!modelUpdateConsumerEnabled) {
            throw new IllegalArgumentException(
                    "champion/challenger shadow requires the integrated model-update consumer");
        }
        if (modelHotUpdateEnabled || modelReloadIntervalMs != 0L) {
            throw new IllegalArgumentException(
                    "champion/challenger shadow forbids serving model mutation");
        }
        validateModelUpdateConsumerConfig();
        if (modelShadowObservationTopic == null
                || !modelShadowObservationTopic.matches("^[a-zA-Z0-9._-]+$")) {
            throw new IllegalArgumentException("model shadow observation topic is invalid");
        }
        if (!validKafkaGroup(modelShadowFeatureConsumerGroupId)
                || !validKafkaGroup(modelShadowUpdateConsumerGroupId)) {
            throw new IllegalArgumentException("model shadow consumer groups are invalid");
        }
        if (modelShadowObservationOnly
                && modelShadowFeatureConsumerGroupId.equals(consumerGroupId)) {
            throw new IllegalArgumentException(
                    "shadow observation-only feature group must not compete with serving");
        }
        if (!Double.isFinite(modelShadowSampleRate)
                || modelShadowSampleRate <= 0.0d || modelShadowSampleRate > 1.0d) {
            throw new IllegalArgumentException("model shadow sample rate must be in (0,1]");
        }
        if (modelShadowChallengerTimeoutMs <= 0L
                || modelShadowChallengerTimeoutMs >= asyncTimeoutMs) {
            throw new IllegalArgumentException(
                    "challenger timeout must be positive and below the serving async timeout");
        }
        if (modelShadowPackageLoadTimeoutMs <= 0L) {
            throw new IllegalArgumentException("shadow package load timeout must be positive");
        }
        if (modelShadowChallengerThreads <= 0
                || modelShadowChallengerQueueCapacity <= 0) {
            throw new IllegalArgumentException(
                    "challenger threads and queue capacity must be positive");
        }
    }

    private static boolean validKafkaGroup(String value) {
        return value != null && value.matches("^[a-zA-Z0-9._-]+$");
    }
    public float getMinConfidenceThreshold() { return minConfidenceThreshold; }
    public float getHighConfidenceThreshold() { return highConfidenceThreshold; }
    public boolean isMultiLabelEnabled() { return multiLabelEnabled; }
    public int getMaxLabelsPerDetection() { return maxLabelsPerDetection; }
    public float getScanThreshold() { return scanThreshold; }
    public float getTunnelThreshold() { return tunnelThreshold; }
    public float getDgaThreshold() { return dgaThreshold; }
    public float getEncryptedTrafficThreshold() { return encryptedTrafficThreshold; }
    public float getAnomalyThreshold() { return anomalyThreshold; }
    public float getC2Threshold() { return c2Threshold; }
    public float getDataExfilThreshold() { return dataExfilThreshold; }
    public float getBotnetThreshold() { return botnetThreshold; }
    public float getMalwareThreshold() { return malwareThreshold; }
    public float getPhishingThreshold() { return phishingThreshold; }
    public boolean isDebugPrintEnabled() { return debugPrintEnabled; }
    public boolean isMetricsEnabled() { return metricsEnabled; }
    public int getMetricsIntervalSeconds() { return metricsIntervalSeconds; }

    /**
     * 判断模型是否启用
     */
    public boolean isModelEnabled(String modelName) {
        return enabledModels.contains(modelName.toLowerCase());
    }

    /**
     * 获取模型阈值
     */
    public float getModelThreshold(String modelName) {
        switch (modelName.toLowerCase()) {
            case "scan": return scanThreshold;
            case "tunnel": return tunnelThreshold;
            case "dga": return dgaThreshold;
            case "encrypted": return encryptedTrafficThreshold;
            case "anomaly": return anomalyThreshold;
            case "c2": return c2Threshold;
            case "data_exfil": return dataExfilThreshold;
            case "botnet": return botnetThreshold;
            case "malware": return malwareThreshold;
            case "phishing": return phishingThreshold;
            default: return minConfidenceThreshold;
        }
    }

    @Override
    public String toString() {
        return "BehaviorJobConfig{" +
                "kafkaBrokers='" + kafkaBrokers + '\'' +
                ", inputTopic='" + inputTopic + '\'' +
                ", outputTopic='" + outputTopic + '\'' +
                ", modelUpdateTopic='" + modelUpdateTopic + '\'' +
                ", parallelism=" + parallelism +
                ", modelVersion='" + modelVersion + '\'' +
                ", detectionMode='" + detectionMode + '\'' +
                ", enabledModels=" + enabledModels +
                ", asyncInferenceEnabled=" + asyncInferenceEnabled +
                ", minConfidenceThreshold=" + minConfidenceThreshold +
                '}';
    }

    /**
     * Builder 模式
     */
    public static class Builder {
        private String kafkaBrokers = "kafka-bootstrap.middleware.svc:9092";
        private String inputTopic = "feature.stat.v1";
        private String outputTopic = "detections.v1";
        private String dlqTopic = "dlq.v1";
        private String modelUpdateTopic = "model-updates";
        private String modelAppliedTopic = "model-update-applied.v1";
        private String modelShadowObservationTopic = "model-shadow-observations.v1";
        private String modelShadowFeatureConsumerGroupId =
                "flink-behavior-job-champion-challenger-features";
        private String modelShadowUpdateConsumerGroupId =
                "flink-behavior-job-champion-challenger-model-updates";
        private String consumerGroupId = "flink-behavior-job";
        private String clickhouseUrl = "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123";
        private String clickhouseDatabase = "traffic";
        private String clickhouseTable = "detections_behavior";
        private String clickhouseUser = "default";
        private String clickhousePassword = "";
        private int clickhouseBatchSize = 5000;
        private long clickhouseBatchIntervalMs = 2000L;
        private String checkpointPath = "s3://flink-checkpoints/checkpoints/behavior-job";
        private long checkpointIntervalMs = 60000L;
        private long checkpointTimeoutMs = 180000L;
        private long checkpointMinPauseMs = 30000L;
        private long watermarkDelayMs = 10000L;
        private long allowedLatenessMs = 60000L;
        private int parallelism = 4;
        private int maxParallelism = 128;
        private String modelPath = "/opt/flink/models";
        private String modelVersion = "v1.0";
        private Set<String> enabledModels = new HashSet<>(Arrays.asList(
                "scan", "tunnel", "dga", "encrypted", "anomaly", "c2", "data_exfil",
                "botnet", "malware", "phishing"));
        private String detectionMode = "off";
        private String knownProfileId = "m04-known-behavior-v1";
        private String knownProfileSha256 =
                "3308cd498548716c68b79c8f665f5a1cb6d7b1d95769234853bbf1e9f7a03cdb";
        private boolean modelHotUpdateEnabled = false;
        private boolean modelUpdateConsumerEnabled = false;
        private String modelConsumerDeploymentId = "";
        private String modelConsumerProfileSha256 = "";
        private String modelRuntimeContract = "traffic.behavior.inference.v1";
        private String modelRuntimeVersion = "1.0.0";
        private int modelFeatureSchemaVersion = 1;
        private int modelGraphSchemaVersion = 1;
        private String modelSigningPublicKeyFile = "";
        private String modelSigningPublicKeyPemBase64 = "";
        private long modelReloadIntervalMs = 0L;
        private boolean modelCacheEnabled = true;
        private int modelCacheSize = 1000;
        private boolean asyncInferenceEnabled = true;
        private long asyncTimeoutMs = 5000L;
        private int asyncCapacity = 100;
        private int asyncMaxRetries = 2;
        private int inferenceThreads = 4;
        private int batchInferenceSize = 32;
        private boolean modelShadowEvaluationEnabled = false;
        private boolean modelShadowObservationOnly = false;
        private double modelShadowSampleRate = 0.0d;
        private long modelShadowChallengerTimeoutMs = 250L;
        private long modelShadowPackageLoadTimeoutMs = 120_000L;
        private int modelShadowChallengerThreads = 2;
        private int modelShadowChallengerQueueCapacity = 64;
        private float minConfidenceThreshold = 0.5f;
        private float highConfidenceThreshold = 0.8f;
        private boolean multiLabelEnabled = true;
        private int maxLabelsPerDetection = 5;
        private float scanThreshold = 0.7f;
        private float tunnelThreshold = 0.75f;
        private float dgaThreshold = 0.8f;
        private float encryptedTrafficThreshold = 0.7f;
        private float anomalyThreshold = 0.6f;
        private float c2Threshold = 0.75f;
        private float dataExfilThreshold = 0.7f;
        // 新增阈值默认值
        private float botnetThreshold = 0.7f;
        private float malwareThreshold = 0.75f;
        private float phishingThreshold = 0.7f;
        private boolean debugPrintEnabled = false;
        private boolean metricsEnabled = true;
        private int metricsIntervalSeconds = 60;

        public Builder kafkaBrokers(String val) { kafkaBrokers = val; return this; }
        public Builder inputTopic(String val) { inputTopic = val; return this; }
        public Builder outputTopic(String val) { outputTopic = val; return this; }
        public Builder dlqTopic(String val) { dlqTopic = val; return this; }
        public Builder modelUpdateTopic(String val) { modelUpdateTopic = val; return this; }
        public Builder modelAppliedTopic(String val) { modelAppliedTopic = val; return this; }
        public Builder modelShadowObservationTopic(String val) { modelShadowObservationTopic = val; return this; }
        public Builder modelShadowFeatureConsumerGroupId(String val) { modelShadowFeatureConsumerGroupId = val; return this; }
        public Builder modelShadowUpdateConsumerGroupId(String val) { modelShadowUpdateConsumerGroupId = val; return this; }
        public Builder consumerGroupId(String val) { consumerGroupId = val; return this; }
        public Builder clickhouseUrl(String val) { clickhouseUrl = val; return this; }
        public Builder clickhouseDatabase(String val) { clickhouseDatabase = val; return this; }
        public Builder clickhouseTable(String val) { clickhouseTable = val; return this; }
        public Builder clickhouseUser(String val) { clickhouseUser = val; return this; }
        public Builder clickhousePassword(String val) { clickhousePassword = val; return this; }
        public Builder clickhouseBatchSize(int val) { clickhouseBatchSize = val; return this; }
        public Builder clickhouseBatchIntervalMs(long val) { clickhouseBatchIntervalMs = val; return this; }
        public Builder checkpointPath(String val) { checkpointPath = val; return this; }
        public Builder checkpointIntervalMs(long val) { checkpointIntervalMs = val; return this; }
        public Builder checkpointTimeoutMs(long val) { checkpointTimeoutMs = val; return this; }
        public Builder checkpointMinPauseMs(long val) { checkpointMinPauseMs = val; return this; }
        public Builder watermarkDelayMs(long val) { watermarkDelayMs = val; return this; }
        public Builder allowedLatenessMs(long val) { allowedLatenessMs = val; return this; }
        public Builder parallelism(int val) { parallelism = val; return this; }
        public Builder maxParallelism(int val) { maxParallelism = val; return this; }
        public Builder modelPath(String val) { modelPath = val; return this; }
        public Builder modelVersion(String val) { modelVersion = val; return this; }
        public Builder enabledModels(Set<String> val) { enabledModels = val; return this; }
        public Builder detectionMode(String val) { detectionMode = val; return this; }
        public Builder knownProfileId(String val) { knownProfileId = val; return this; }
        public Builder knownProfileSha256(String val) { knownProfileSha256 = val; return this; }
        public Builder modelHotUpdateEnabled(boolean val) { modelHotUpdateEnabled = val; return this; }
        public Builder modelUpdateConsumerEnabled(boolean val) { modelUpdateConsumerEnabled = val; return this; }
        public Builder modelConsumerDeploymentId(String val) { modelConsumerDeploymentId = val; return this; }
        public Builder modelConsumerProfileSha256(String val) { modelConsumerProfileSha256 = val; return this; }
        public Builder modelRuntimeContract(String val) { modelRuntimeContract = val; return this; }
        public Builder modelRuntimeVersion(String val) { modelRuntimeVersion = val; return this; }
        public Builder modelFeatureSchemaVersion(int val) { modelFeatureSchemaVersion = val; return this; }
        public Builder modelGraphSchemaVersion(int val) { modelGraphSchemaVersion = val; return this; }
        public Builder modelSigningPublicKeyFile(String val) { modelSigningPublicKeyFile = val; return this; }
        public Builder modelSigningPublicKeyPemBase64(String val) { modelSigningPublicKeyPemBase64 = val; return this; }
        public Builder modelReloadIntervalMs(long val) { modelReloadIntervalMs = val; return this; }
        public Builder modelCacheEnabled(boolean val) { modelCacheEnabled = val; return this; }
        public Builder modelCacheSize(int val) { modelCacheSize = val; return this; }
        public Builder asyncInferenceEnabled(boolean val) { asyncInferenceEnabled = val; return this; }
        public Builder asyncTimeoutMs(long val) { asyncTimeoutMs = val; return this; }
        public Builder asyncCapacity(int val) { asyncCapacity = val; return this; }
        public Builder asyncMaxRetries(int val) { asyncMaxRetries = val; return this; }
        public Builder inferenceThreads(int val) { inferenceThreads = val; return this; }
        public Builder batchInferenceSize(int val) { batchInferenceSize = val; return this; }
        public Builder modelShadowEvaluationEnabled(boolean val) { modelShadowEvaluationEnabled = val; return this; }
        public Builder modelShadowObservationOnly(boolean val) { modelShadowObservationOnly = val; return this; }
        public Builder modelShadowSampleRate(double val) { modelShadowSampleRate = val; return this; }
        public Builder modelShadowChallengerTimeoutMs(long val) { modelShadowChallengerTimeoutMs = val; return this; }
        public Builder modelShadowPackageLoadTimeoutMs(long val) { modelShadowPackageLoadTimeoutMs = val; return this; }
        public Builder modelShadowChallengerThreads(int val) { modelShadowChallengerThreads = val; return this; }
        public Builder modelShadowChallengerQueueCapacity(int val) { modelShadowChallengerQueueCapacity = val; return this; }
        public Builder minConfidenceThreshold(float val) { minConfidenceThreshold = val; return this; }
        public Builder highConfidenceThreshold(float val) { highConfidenceThreshold = val; return this; }
        public Builder multiLabelEnabled(boolean val) { multiLabelEnabled = val; return this; }
        public Builder maxLabelsPerDetection(int val) { maxLabelsPerDetection = val; return this; }
        public Builder scanThreshold(float val) { scanThreshold = val; return this; }
        public Builder tunnelThreshold(float val) { tunnelThreshold = val; return this; }
        public Builder dgaThreshold(float val) { dgaThreshold = val; return this; }
        public Builder encryptedTrafficThreshold(float val) { encryptedTrafficThreshold = val; return this; }
        public Builder anomalyThreshold(float val) { anomalyThreshold = val; return this; }
        public Builder c2Threshold(float val) { c2Threshold = val; return this; }
        public Builder dataExfilThreshold(float val) { dataExfilThreshold = val; return this; }
        public Builder botnetThreshold(float val) { botnetThreshold = val; return this; }
        public Builder malwareThreshold(float val) { malwareThreshold = val; return this; }
        public Builder phishingThreshold(float val) { phishingThreshold = val; return this; }
        public Builder debugPrintEnabled(boolean val) { debugPrintEnabled = val; return this; }
        public Builder metricsEnabled(boolean val) { metricsEnabled = val; return this; }
        public Builder metricsIntervalSeconds(int val) { metricsIntervalSeconds = val; return this; }

        public BehaviorJobConfig build() {
            return new BehaviorJobConfig(this);
        }
    }
}
