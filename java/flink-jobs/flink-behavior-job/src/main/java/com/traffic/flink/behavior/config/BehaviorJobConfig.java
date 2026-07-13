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
    private final String modelUpdateTopic;
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
    private final long modelReloadIntervalMs;
    private final boolean modelCacheEnabled;
    private final int modelCacheSize;

    // ==================== 推理配置 ====================
    private final boolean asyncInferenceEnabled;
    private final long asyncTimeoutMs;
    private final int asyncCapacity;
    private final int inferenceThreads;
    private final int batchInferenceSize;

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
        this.modelUpdateTopic = builder.modelUpdateTopic;
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
        this.modelReloadIntervalMs = builder.modelReloadIntervalMs;
        this.modelCacheEnabled = builder.modelCacheEnabled;
        this.modelCacheSize = builder.modelCacheSize;
        this.asyncInferenceEnabled = builder.asyncInferenceEnabled;
        this.asyncTimeoutMs = builder.asyncTimeoutMs;
        this.asyncCapacity = builder.asyncCapacity;
        this.inferenceThreads = builder.inferenceThreads;
        this.batchInferenceSize = builder.batchInferenceSize;
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
                        getEnv("KAFKA_OUTPUT_TOPIC", "detections.behavior.v1")))
                .modelUpdateTopic(getConfig(params, fileProps, "kafka.model.update.topic",
                        getEnv("KAFKA_MODEL_UPDATE_TOPIC",
                                getEnv("MODEL_UPDATE_TOPIC",
                                        getEnv("KAFKA_MODEL_TOPIC", "model-updates")))))
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
                // 修复：添加所有 10 个模型到默认启用列表
                .enabledModels(parseModels(getConfig(params, fileProps, "model.enabled", 
                        "scan,tunnel,dga,encrypted,anomaly,c2,data_exfil,botnet,malware,phishing")))
                .modelReloadIntervalMs(getConfigLong(params, fileProps, "model.reload.interval.ms", 300000L))
                .modelCacheEnabled(getConfigBoolean(params, fileProps, "model.cache.enabled", true))
                .modelCacheSize(getConfigInt(params, fileProps, "model.cache.size", 1000))
                
                // 推理
                .asyncInferenceEnabled(getConfigBoolean(params, fileProps, "inference.async.enabled", true))
                .asyncTimeoutMs(getConfigLong(params, fileProps, "inference.async.timeout.ms", 5000L))
                .asyncCapacity(getConfigInt(params, fileProps, "inference.async.capacity", 100))
                .inferenceThreads(getConfigInt(params, fileProps, "inference.threads", 4))
                .batchInferenceSize(getConfigInt(params, fileProps, "inference.batch.size", 32))
                
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

    /**
     * 获取配置值（优先级：命令行 > 配置文件 > 默认值）
     */
    private static String getConfig(ParameterTool params, Properties props, String key, String defaultValue) {
        if (params.has(key)) {
            return params.get(key);
        }
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
    public String getModelUpdateTopic() { return modelUpdateTopic; }
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
    public long getModelReloadIntervalMs() { return modelReloadIntervalMs; }
    public boolean isModelCacheEnabled() { return modelCacheEnabled; }
    public int getModelCacheSize() { return modelCacheSize; }
    public boolean isAsyncInferenceEnabled() { return asyncInferenceEnabled; }
    public long getAsyncTimeoutMs() { return asyncTimeoutMs; }
    public int getAsyncCapacity() { return asyncCapacity; }
    public int getInferenceThreads() { return inferenceThreads; }
    public int getBatchInferenceSize() { return batchInferenceSize; }
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
        private String outputTopic = "detections.behavior.v1";
        private String modelUpdateTopic = "model-updates";
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
        // 修复：默认启用所有 10 个模型
        private Set<String> enabledModels = new HashSet<>(Arrays.asList(
                "scan", "tunnel", "dga", "encrypted", "anomaly", "c2", "data_exfil",
                "botnet", "malware", "phishing"));
        private long modelReloadIntervalMs = 300000L;
        private boolean modelCacheEnabled = true;
        private int modelCacheSize = 1000;
        private boolean asyncInferenceEnabled = true;
        private long asyncTimeoutMs = 5000L;
        private int asyncCapacity = 100;
        private int inferenceThreads = 4;
        private int batchInferenceSize = 32;
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
        public Builder modelUpdateTopic(String val) { modelUpdateTopic = val; return this; }
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
        public Builder modelReloadIntervalMs(long val) { modelReloadIntervalMs = val; return this; }
        public Builder modelCacheEnabled(boolean val) { modelCacheEnabled = val; return this; }
        public Builder modelCacheSize(int val) { modelCacheSize = val; return this; }
        public Builder asyncInferenceEnabled(boolean val) { asyncInferenceEnabled = val; return this; }
        public Builder asyncTimeoutMs(long val) { asyncTimeoutMs = val; return this; }
        public Builder asyncCapacity(int val) { asyncCapacity = val; return this; }
        public Builder inferenceThreads(int val) { inferenceThreads = val; return this; }
        public Builder batchInferenceSize(int val) { batchInferenceSize = val; return this; }
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
