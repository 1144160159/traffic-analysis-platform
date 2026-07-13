////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/config/BehaviorJobConfigTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.config;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.HashSet;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

/**
 * 配置类单元测试
 */
@DisplayName("BehaviorJobConfig Tests")
class BehaviorJobConfigTest {

    @Test
    @DisplayName("默认配置正确")
    void testDefaultConfig() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder().build();

        // Kafka 默认配置 (K8s 集群端点)
        assertEquals("kafka-bootstrap.middleware.svc:9092", config.getKafkaBrokers());
        assertEquals("feature.stat.v1", config.getInputTopic());
        assertEquals("detections.behavior.v1", config.getOutputTopic());
        assertEquals("flink-behavior-job", config.getConsumerGroupId());

        // ClickHouse 默认配置 (K8s 集群端点)
        assertEquals("clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123", config.getClickhouseUrl());
        assertEquals("traffic", config.getClickhouseDatabase());
        assertEquals("detections_behavior", config.getClickhouseTable());

        // 性能默认配置
        assertEquals(4, config.getParallelism());
        assertEquals(128, config.getMaxParallelism());

        // 检测默认配置
        assertEquals(0.5f, config.getMinConfidenceThreshold());
        assertEquals(0.8f, config.getHighConfidenceThreshold());
        assertTrue(config.isAsyncInferenceEnabled());
    }

    @Test
    @DisplayName("Builder 模式配置正确")
    void testBuilderConfig() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .kafkaBrokers("kafka1:9092,kafka2:9092")
                .inputTopic("custom.input.topic")
                .outputTopic("custom.output.topic")
                .parallelism(8)
                .minConfidenceThreshold(0.6f)
                .scanThreshold(0.8f)
                .asyncInferenceEnabled(false)
                .debugPrintEnabled(true)
                .build();

        assertEquals("kafka1:9092,kafka2:9092", config.getKafkaBrokers());
        assertEquals("custom.input.topic", config.getInputTopic());
        assertEquals("custom.output.topic", config.getOutputTopic());
        assertEquals(8, config.getParallelism());
        assertEquals(0.6f, config.getMinConfidenceThreshold());
        assertEquals(0.8f, config.getScanThreshold());
        assertFalse(config.isAsyncInferenceEnabled());
        assertTrue(config.isDebugPrintEnabled());
    }

    @Test
    @DisplayName("所有模型阈值配置")
    void testAllModelThresholds() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .scanThreshold(0.71f)
                .tunnelThreshold(0.72f)
                .dgaThreshold(0.73f)
                .encryptedTrafficThreshold(0.74f)
                .anomalyThreshold(0.75f)
                .c2Threshold(0.76f)
                .dataExfilThreshold(0.77f)
                .botnetThreshold(0.78f)
                .malwareThreshold(0.79f)
                .phishingThreshold(0.80f)
                .build();

        assertEquals(0.71f, config.getScanThreshold());
        assertEquals(0.72f, config.getTunnelThreshold());
        assertEquals(0.73f, config.getDgaThreshold());
        assertEquals(0.74f, config.getEncryptedTrafficThreshold());
        assertEquals(0.75f, config.getAnomalyThreshold());
        assertEquals(0.76f, config.getC2Threshold());
        assertEquals(0.77f, config.getDataExfilThreshold());
        assertEquals(0.78f, config.getBotnetThreshold());
        assertEquals(0.79f, config.getMalwareThreshold());
        assertEquals(0.80f, config.getPhishingThreshold());
    }

    @Test
    @DisplayName("getModelThreshold 方法正确返回阈值")
    void testGetModelThreshold() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .scanThreshold(0.71f)
                .tunnelThreshold(0.72f)
                .minConfidenceThreshold(0.5f)
                .build();

        assertEquals(0.71f, config.getModelThreshold("scan"));
        assertEquals(0.71f, config.getModelThreshold("SCAN"));  // 大小写不敏感
        assertEquals(0.72f, config.getModelThreshold("tunnel"));
        assertEquals(0.5f, config.getModelThreshold("unknown"));  // 未知模型返回默认阈值
    }

    @Test
    @DisplayName("isModelEnabled 方法正确判断")
    void testIsModelEnabled() {
        Set<String> enabledModels = new HashSet<>(Arrays.asList("scan", "tunnel"));
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .enabledModels(enabledModels)
                .build();

        assertTrue(config.isModelEnabled("scan"));
        assertTrue(config.isModelEnabled("SCAN"));  // 大小写不敏感
        assertTrue(config.isModelEnabled("tunnel"));
        assertFalse(config.isModelEnabled("dga"));
        assertFalse(config.isModelEnabled("botnet"));
    }

    @Test
    @DisplayName("默认启用所有模型")
    void testDefaultEnabledModels() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder().build();

        Set<String> enabledModels = config.getEnabledModels();
        assertEquals(10, enabledModels.size());
        assertTrue(enabledModels.contains("scan"));
        assertTrue(enabledModels.contains("tunnel"));
        assertTrue(enabledModels.contains("dga"));
        assertTrue(enabledModels.contains("encrypted"));
        assertTrue(enabledModels.contains("anomaly"));
        assertTrue(enabledModels.contains("c2"));
        assertTrue(enabledModels.contains("data_exfil"));
        assertTrue(enabledModels.contains("botnet"));
        assertTrue(enabledModels.contains("malware"));
        assertTrue(enabledModels.contains("phishing"));
    }

    @Test
    @DisplayName("从命令行参数构建配置")
    void testFromArgs() {
        String[] args = {
                "--kafka.brokers", "kafka:9092",
                "--parallelism", "16",
                "--threshold.scan", "0.85"
        };

        BehaviorJobConfig config = BehaviorJobConfig.fromArgs(args);

        assertEquals("kafka:9092", config.getKafkaBrokers());
        assertEquals(16, config.getParallelism());
        assertEquals(0.85f, config.getScanThreshold());
    }

    @Test
    @DisplayName("Duration 类型 getter")
    void testDurationGetter() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .watermarkDelayMs(15000L)
                .build();

        assertEquals(15000L, config.getWatermarkDelayMs());
        assertEquals(15000L, config.getWatermarkDelayDuration().toMillis());
    }

    @Test
    @DisplayName("toString 不抛出异常")
    void testToString() {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder().build();
        String str = config.toString();

        assertNotNull(str);
        assertTrue(str.contains("BehaviorJobConfig"));
        assertTrue(str.contains("kafkaBrokers"));
    }
}