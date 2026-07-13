package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.io.InputStream;
import java.util.Properties;
import java.util.HashMap;
import java.util.Map;

/**
 * 统一配置工具类
 * 优先级：命令行参数 > 环境变量 > 配置文件 > 默认值
 */
public final class ConfigUtils {

    private static final Logger LOG = LoggerFactory.getLogger(ConfigUtils.class);

    private ConfigUtils() {}

    /**
     * 加载配置
     * @param args 命令行参数
     * @param propertiesFile 配置文件名（从 classpath 加载）
     * @return ParameterTool
     */
    public static ParameterTool loadConfig(String[] args, String propertiesFile) throws Exception {
        // 1. 从配置文件加载
        ParameterTool fileParams = loadFromFile(propertiesFile);
        
        // 2. 从环境变量加载
        ParameterTool envParams = ParameterTool.fromSystemProperties();
        
        // 3. 从命令行参数加载
        ParameterTool cliParams = ParameterTool.fromArgs(args);
        
        // 合并（优先级：CLI > ENV > File）
        ParameterTool merged = fileParams
                .mergeWith(envParams)
                .mergeWith(cliParams);
        
        LOG.info("Configuration loaded from: {}", propertiesFile);
        return merged;
    }

    private static ParameterTool loadFromFile(String filename) throws IOException {
        Properties props = new Properties();
        try (InputStream input = ConfigUtils.class.getClassLoader().getResourceAsStream(filename)) {
            if (input == null) {
                LOG.warn("Configuration file not found: {}, using defaults", filename);
                return ParameterTool.fromMap(System.getenv());
            }
            props.load(input);
        }
        Map<String, String> map = new HashMap<>();
        for (String name : props.stringPropertyNames()) {
            map.put(name, props.getProperty(name));
        }
        return ParameterTool.fromMap(map);
    }

    /**
     * 获取字符串配置
     */
    public static String get(ParameterTool params, String key, String defaultValue) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            LOG.debug("Config key '{}' not set, using default: {}", key, defaultValue);
            return defaultValue;
        }
        return value;
    }

    /**
     * 获取整数配置
     */
    public static int getInt(ParameterTool params, String key, int defaultValue) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            return defaultValue;
        }
        try {
            return Integer.parseInt(value);
        } catch (NumberFormatException e) {
            LOG.warn("Invalid integer for key '{}': {}, using default: {}", key, value, defaultValue);
            return defaultValue;
        }
    }

    /**
     * 获取长整型配置
     */
    public static long getLong(ParameterTool params, String key, long defaultValue) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            return defaultValue;
        }
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException e) {
            LOG.warn("Invalid long for key '{}': {}, using default: {}", key, value, defaultValue);
            return defaultValue;
        }
    }

    /**
     * 获取浮点数配置
     */
    public static float getFloat(ParameterTool params, String key, float defaultValue) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            return defaultValue;
        }
        try {
            return Float.parseFloat(value);
        } catch (NumberFormatException e) {
            LOG.warn("Invalid float for key '{}': {}, using default: {}", key, value, defaultValue);
            return defaultValue;
        }
    }

    /**
     * 获取布尔配置
     */
    public static boolean getBoolean(ParameterTool params, String key, boolean defaultValue) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            return defaultValue;
        }
        return Boolean.parseBoolean(value);
    }

    /**
     * 获取配置并验证非空
     */
    public static String getRequired(ParameterTool params, String key) {
        String value = params.get(key);
        if (value == null || value.isEmpty()) {
            throw new IllegalArgumentException("Required configuration missing: " + key);
        }
        return value;
    }

    /**
     * 构建 ClickHouse JDBC URL
     */
    public static String buildClickHouseUrl(String host, String database) {
        return String.format("jdbc:clickhouse://%s/%s", host, database);
    }

    public static Properties kafkaClientProperties() {
        return kafkaClientProperties(ParameterTool.fromMap(System.getenv()));
    }

    public static Properties kafkaClientProperties(ParameterTool params) {
        Properties props = new Properties();
        String securityProtocol = getKafkaValue(params, "kafka.security.protocol", "KAFKA_SECURITY_PROTOCOL", "");
        if (securityProtocol.isEmpty()) {
            return props;
        }

        props.setProperty("security.protocol", securityProtocol);
        String mechanism = getKafkaValue(params, "kafka.sasl.mechanism", "KAFKA_SASL_MECHANISM", "SCRAM-SHA-512");
        String username = getKafkaValue(params, "kafka.sasl.username", "KAFKA_SASL_USERNAME", "");
        String password = getKafkaValue(params, "kafka.sasl.password", "KAFKA_SASL_PASSWORD", "");
        String jaasConfig = getKafkaValue(params, "kafka.sasl.jaas.config", "KAFKA_SASL_JAAS_CONFIG", "");

        if (!mechanism.isEmpty()) {
            props.setProperty("sasl.mechanism", mechanism);
        }
        if (jaasConfig.isEmpty() && !username.isEmpty() && !password.isEmpty()) {
            String loginModule = "PLAIN".equalsIgnoreCase(mechanism)
                    ? "org.apache.kafka.common.security.plain.PlainLoginModule"
                    : "org.apache.kafka.common.security.scram.ScramLoginModule";
            jaasConfig = String.format("%s required username=\"%s\" password=\"%s\";", loginModule, username, password);
        }
        if (!jaasConfig.isEmpty()) {
            props.setProperty("sasl.jaas.config", jaasConfig);
        }

        putKafkaProperty(props, params, "ssl.truststore.location", "kafka.ssl.truststore.location", "KAFKA_SSL_TRUSTSTORE_LOCATION");
        putKafkaProperty(props, params, "ssl.truststore.password", "kafka.ssl.truststore.password", "KAFKA_SSL_TRUSTSTORE_PASSWORD");
        putKafkaProperty(props, params, "ssl.truststore.type", "kafka.ssl.truststore.type", "KAFKA_SSL_TRUSTSTORE_TYPE");
        putKafkaProperty(props, params, "ssl.keystore.location", "kafka.ssl.keystore.location", "KAFKA_SSL_KEYSTORE_LOCATION");
        putKafkaProperty(props, params, "ssl.keystore.password", "kafka.ssl.keystore.password", "KAFKA_SSL_KEYSTORE_PASSWORD");
        putKafkaProperty(props, params, "ssl.key.password", "kafka.ssl.key.password", "KAFKA_SSL_KEY_PASSWORD");
        putKafkaProperty(props, params, "ssl.endpoint.identification.algorithm", "kafka.ssl.endpoint.identification.algorithm", "KAFKA_SSL_ENDPOINT_IDENTIFICATION_ALGORITHM");

        return props;
    }

    private static void putKafkaProperty(Properties props, ParameterTool params, String targetKey, String configKey, String envKey) {
        String value = getKafkaValue(params, configKey, envKey, "");
        if (!value.isEmpty()) {
            props.setProperty(targetKey, value);
        }
    }

    private static String getKafkaValue(ParameterTool params, String configKey, String envKey, String defaultValue) {
        String value = params.get(configKey);
        if (value == null || value.isEmpty()) {
            value = params.get(envKey);
        }
        if (value == null || value.isEmpty()) {
            value = System.getenv(envKey);
        }
        return value == null || value.isEmpty() ? defaultValue : value;
    }

    /**
     * 打印配置摘要（脱敏）
     */
    public static void logConfigSummary(ParameterTool params, String jobName) {
        LOG.info("========== {} Configuration ==========", jobName);
        LOG.info("Kafka Brokers: {}", params.get("kafka.brokers", "N/A"));
        LOG.info("Input Topic: {}", params.get("kafka.input.topic", "N/A"));
        LOG.info("Output Topic: {}", params.get("kafka.output.topic", "N/A"));
        LOG.info("ClickHouse URL: {}", params.get("clickhouse.url", "N/A"));
        LOG.info("Parallelism: {}", params.get("parallelism", "N/A"));
        LOG.info("Checkpoint Interval: {}ms", params.get("checkpoint.interval.ms", "N/A"));
        LOG.info("=========================================");
    }
}
