package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;

import java.util.Map;

/**
 * 兼容旧名 `ConfigUtil` 的包装类，内部委托给 `ConfigUtils` 实现。
 */
public final class ConfigUtil {

    private ConfigUtil() {}

    // Backward-compatible default constants
    public static final String KAFKA_BROKERS = System.getenv().getOrDefault("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092");
    public static final String TOPIC_FLOW_EVENTS = "flow.events.v1";
    public static final String TOPIC_SESSION_EVENTS = "session.events.v1";
    public static final String CLICKHOUSE_DATABASE = System.getenv().getOrDefault("CLICKHOUSE_DATABASE", "traffic");
    public static final String CLICKHOUSE_USER = System.getenv().getOrDefault("CLICKHOUSE_USER", "default");
    public static final String CLICKHOUSE_PASSWORD = System.getenv().getOrDefault("CLICKHOUSE_PASSWORD", "");
    // 批量写入：5000 条或 2s（匹配 agent.md 性能规范）
    public static final int CLICKHOUSE_BATCH_SIZE = 5000;
    public static final long CLICKHOUSE_BATCH_INTERVAL_MS = 2000L;
    // Checkpoint：30s 间隔, 10min 超时（匹配 rules/java.md 规范）
    public static final String CHECKPOINT_PATH = System.getenv().getOrDefault(
            "CHECKPOINT_PATH",
            "s3://flink-checkpoints/checkpoints/flink-traffic");
    public static final long CHECKPOINT_INTERVAL_MS = 30_000L;
    public static final long CHECKPOINT_TIMEOUT_MS = 600_000L;
    public static final long CHECKPOINT_MIN_PAUSE_MS = 5_000L;
    // Low-latency acceptance profile: probe inactive timeout=5s + session gap=5s + watermark delay=5s.
    public static final long SESSION_GAP_MS = Long.parseLong(System.getenv().getOrDefault("SESSION_GAP_MS", "5000"));
    public static final long WATERMARK_DELAY_MS = 5_000L;

    public static ParameterTool loadConfig(String[] args, String propertiesFile) throws Exception {
        return ConfigUtils.loadConfig(args, propertiesFile);
    }

    public static String get(ParameterTool params, String key, String defaultValue) {
        return ConfigUtils.get(params, key, defaultValue);
    }

    public static int getInt(ParameterTool params, String key, int defaultValue) {
        return ConfigUtils.getInt(params, key, defaultValue);
    }

    public static long getLong(ParameterTool params, String key, long defaultValue) {
        return ConfigUtils.getLong(params, key, defaultValue);
    }

    public static float getFloat(ParameterTool params, String key, float defaultValue) {
        return ConfigUtils.getFloat(params, key, defaultValue);
    }

    public static boolean getBoolean(ParameterTool params, String key, boolean defaultValue) {
        return ConfigUtils.getBoolean(params, key, defaultValue);
    }

    public static String getRequired(ParameterTool params, String key) {
        return ConfigUtils.getRequired(params, key);
    }

    public static String buildClickHouseUrl(String host, String database) {
        return ConfigUtils.buildClickHouseUrl(host, database);
    }

    public static java.util.Properties kafkaClientProperties() {
        return ConfigUtils.kafkaClientProperties();
    }

    public static java.util.Properties kafkaClientProperties(ParameterTool params) {
        return ConfigUtils.kafkaClientProperties(params);
    }

    // No-arg helper for legacy callers
    public static String buildClickHouseUrl() {
        String host = System.getenv().getOrDefault("CLICKHOUSE_HOST", "clickhouse-1.middleware.svc");
        return buildClickHouseUrl(host, CLICKHOUSE_DATABASE);
    }

    public static void logConfigSummary(ParameterTool params, String jobName) {
        ConfigUtils.logConfigSummary(params, jobName);
    }
}
