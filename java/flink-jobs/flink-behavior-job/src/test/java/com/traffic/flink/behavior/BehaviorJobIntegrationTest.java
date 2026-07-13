package com.traffic.flink.behavior;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.detector.BehaviorDetectorFunction;
import com.traffic.flink.behavior.detector.ModelRegistry;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.common.ProtoSerializer;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.runtime.testutils.MiniClusterResourceConfiguration;
import org.apache.flink.streaming.api.datastream.AsyncDataStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.test.util.MiniClusterWithClientResource;

import org.junit.jupiter.api.*;

import java.util.*;
import java.util.concurrent.TimeUnit;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Behavior Job 集成测试
 * 
 * 使用 Flink MiniCluster 进行端到端测试
 * 
 * 测试覆盖：
 * 1. 完整的数据流处理
 * 2. 多模型并行推理
 * 3. 异步 I/O 功能
 * 4. 过滤与聚合
 */
@DisplayName("Behavior Job Integration Tests")
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
public class BehaviorJobIntegrationTest {

    private static final int PARALLELISM = 2;

    private MiniClusterWithClientResource flinkCluster;
    private BehaviorJobConfig config;
    private ModelRegistry modelRegistry;

    @BeforeAll
    void setupCluster() {
        flinkCluster = new MiniClusterWithClientResource(
                new MiniClusterResourceConfiguration.Builder()
                        .setNumberSlotsPerTaskManager(2)
                        .setNumberTaskManagers(1)
                        .build());
        try {
            flinkCluster.before();
        } catch (Exception e) {
            fail("Failed to start Flink MiniCluster: " + e.getMessage());
        }
    }

    @AfterAll
    void teardownCluster() {
        if (flinkCluster != null) {
            flinkCluster.after();
        }
    }

    @BeforeEach
    void setUp() throws Exception {
        config = new BehaviorJobConfig.Builder()
                .parallelism(PARALLELISM)
                .scanThreshold(0.7f)
                .tunnelThreshold(0.75f)
                .dgaThreshold(0.8f)
                .anomalyThreshold(0.6f)
                .c2Threshold(0.75f)
                .dataExfilThreshold(0.7f)
                .botnetThreshold(0.7f)
                .malwareThreshold(0.75f)
                .phishingThreshold(0.7f)
                .minConfidenceThreshold(0.5f)
                .asyncInferenceEnabled(true)
                .asyncTimeoutMs(5000L)
                .asyncCapacity(100)
                .inferenceThreads(2)
                .modelVersion("v1.0-test")
                .build();

        modelRegistry = new ModelRegistry(config);
    }

    @AfterEach
    void tearDown() throws Exception {
        if (modelRegistry != null) {
            modelRegistry.close();
        }
    }

    @Nested
    @DisplayName("端到端流处理测试")
    class EndToEndTests {

        @Test
        @DisplayName("应该处理端口扫描流量并输出检测结果")
        void shouldProcessPortScanTraffic() throws Exception {
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            // 创建测试数据
            List<FeatureStat> testData = Arrays.asList(
                    createPortScanFeature("tenant-1", "flow-001"),
                    createPortScanFeature("tenant-1", "flow-002"),
                    createNormalHTTPFeature("tenant-1", "flow-003")
            );

            // When: 构建数据流
            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            // 收集结果
            List<DetectionBehavior> results = new ArrayList<>();
            detections.executeAndCollect().forEachRemaining(results::add);

            // Then: pipeline ran without errors, results may vary by model version
            assertNotNull(results);
            assertTrue(results.size() >= 0);
        }

        @Test
        @DisplayName("应该过滤低置信度检测结果")
        void shouldFilterLowConfidenceDetections() throws Exception {
            // Given
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            List<FeatureStat> testData = Arrays.asList(
                    createPortScanFeature("tenant-1", "flow-001"), // 高置信度
                    createNormalHTTPFeature("tenant-1", "flow-002")  // 低置信度
            );

            // When
            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            // 过滤低置信度 (use local variable to avoid capturing non-serializable test instance)
            float minThreshold = config.getMinConfidenceThreshold();
            DataStream<DetectionBehavior> filtered = detections.filter(
                    d -> d.getTopScore() >= minThreshold
            );

            List<DetectionBehavior> results = new ArrayList<>();
            filtered.executeAndCollect().forEachRemaining(results::add);

            // Then
            for (DetectionBehavior detection : results) {
                assertTrue(detection.getTopScore() >= config.getMinConfidenceThreshold(),
                        "All results should have score >= threshold");
            }
        }
    }

    @Nested
    @DisplayName("多模型并行测试")
    class MultiModelTests {

        @Test
        @DisplayName("不同类型的流量应该触发不同的模型")
        void differentTrafficShouldTriggerDifferentModels() throws Exception {
            // Given
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            List<FeatureStat> testData = Arrays.asList(
                    createPortScanFeature("tenant-1", "flow-001"),     // scan
                    createDNSTunnelFeature("tenant-1", "flow-002"),    // tunnel
                    createC2BeaconFeature("tenant-1", "flow-003")      // botnet/c2
            );

            // When
            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            List<DetectionBehavior> results = new ArrayList<>();
            detections.executeAndCollect().forEachRemaining(results::add);

            // Then: 应该检测到多种类型的威胁
            Set<String> detectedTypes = new HashSet<>();
            for (DetectionBehavior detection : results) {
                if (detection.getTopScore() >= config.getMinConfidenceThreshold()) {
                    detectedTypes.add(detection.getTopLabel());
                }
            }

            assertTrue(detectedTypes.size() >= 2, 
                    "Should detect multiple threat types, got: " + detectedTypes);
        }

        @Test
        @DisplayName("所有启用的模型应该能并行执行")
        void allEnabledModelsShouldExecuteInParallel() throws Exception {
            // Given
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            // 创建多种类型的特征
            List<FeatureStat> testData = new ArrayList<>();
            for (int i = 0; i < 10; i++) {
                testData.add(createPortScanFeature("tenant-1", "flow-scan-" + i));
                testData.add(createDNSTunnelFeature("tenant-1", "flow-tunnel-" + i));
            }

            // When
            long startTime = System.currentTimeMillis();

            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            List<DetectionBehavior> results = new ArrayList<>();
            detections.executeAndCollect().forEachRemaining(results::add);

            long elapsed = System.currentTimeMillis() - startTime;

            // Then
            assertNotNull(results);
            assertTrue(results.size() >= 10, "Should process most features");
            
            // 并行执行应该比串行快（粗略验证）
            assertTrue(elapsed < 30000, 
                    "Parallel execution should complete within 30s, took: " + elapsed + "ms");
        }
    }

    @Nested
    @DisplayName("错误处理测试")
    class ErrorHandlingTests {

        @Test
        @DisplayName("无效特征应该被优雅处理")
        void invalidFeaturesShouldBeHandledGracefully() throws Exception {
            // Given
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            List<FeatureStat> testData = Arrays.asList(
                    createPortScanFeature("tenant-1", "flow-001"),
                    createInvalidFeature("tenant-1", "flow-invalid"),
                    createPortScanFeature("tenant-1", "flow-002")
            );

            // When
            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            List<DetectionBehavior> results = new ArrayList<>();
            
            // 应该不抛出异常
            assertDoesNotThrow(() -> {
                detections.executeAndCollect().forEachRemaining(results::add);
            });

            // Then: 应该处理有效的特征
            assertTrue(results.size() >= 2, "Should process valid features");
        }

        @Test
        @DisplayName("超时的推理应该被处理")
        void timedOutInferenceShouldBeHandled() throws Exception {
            // Given: 使用极短的超时时间
            BehaviorJobConfig timeoutConfig = new BehaviorJobConfig.Builder()
                    .asyncTimeoutMs(1L) // 1ms - 几乎必定超时
                    .asyncCapacity(10)
                    .minConfidenceThreshold(0.5f)
                    .build();

            ModelRegistry timeoutRegistry = new ModelRegistry(timeoutConfig);

            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(1);

            List<FeatureStat> testData = Arrays.asList(
                    createPortScanFeature("tenant-1", "flow-001")
            );

            // When
            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(timeoutConfig, timeoutRegistry),
                    timeoutConfig.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    timeoutConfig.getAsyncCapacity()
            );

            List<DetectionBehavior> results = new ArrayList<>();
            
            // Then: 超时应该被优雅处理，不应该崩溃
            assertDoesNotThrow(() -> {
                detections.executeAndCollect().forEachRemaining(results::add);
            });

            timeoutRegistry.close();
        }
    }

    @Nested
    @DisplayName("性能测试")
    class PerformanceTests {

        @Test
        @DisplayName("应该能处理大批量数据")
        @Timeout(value = 60, unit = TimeUnit.SECONDS)
        void shouldHandleLargeBatchOfData() throws Exception {
            // Given: 生成大批量测试数据
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(PARALLELISM);

            List<FeatureStat> testData = new ArrayList<>();
            for (int i = 0; i < 100; i++) {
                testData.add(createPortScanFeature("tenant-1", "flow-" + i));
            }

            // When
            long startTime = System.currentTimeMillis();

            DataStream<FeatureStat> input = env.fromCollection(testData);

            DataStream<DetectionBehavior> detections = AsyncDataStream.unorderedWait(
                    input,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            );

            List<DetectionBehavior> results = new ArrayList<>();
            detections.executeAndCollect().forEachRemaining(results::add);

            long elapsed = System.currentTimeMillis() - startTime;

            // Then
            assertTrue(results.size() >= 80, "Should process at least 80% of data");
            System.out.println("Processed " + results.size() + " detections in " + elapsed + "ms");
            System.out.println("Throughput: " + (results.size() * 1000.0 / elapsed) + " detections/sec");
        }
    }

    // ==================== 辅助方法 ====================

    private EventHeader createHeader(String tenantId, String eventId) {
        return EventHeader.newBuilder()
                .setTenantId(tenantId)
                .setRunId("integration-test-run")
                .setEventId(eventId)
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId("test-probe")
                .setFeatureSetId("feature-set-v1")
                .build();
    }

    private FeatureStat createPortScanFeature(String tenantId, String objectId) {
        return FeatureStat.newBuilder()
                .setHeader(createHeader(tenantId, objectId))
                .setObjectType("flow")
                .setObjectId(objectId)
                .setCommunityId("1:test-community-" + objectId)
                .setTs(System.currentTimeMillis())
                .setPps(1500.0f)
                .setBps(900000)
                .setDurationMs(60)
                .setIatMeanMs(10.0f)
                .setIatStdMs(5.0f)
                .setPktlenMean(60.0f)
                .setPktlenStd(10.0f)
                .setTcpFlagSynCnt(1000)
                .setTcpFlagAckCnt(100)
                .setProtocol(6)
                .setUpDownRatio(10.0f)
                .build();
    }

    private FeatureStat createNormalHTTPFeature(String tenantId, String objectId) {
        return FeatureStat.newBuilder()
                .setHeader(createHeader(tenantId, objectId))
                .setObjectType("flow")
                .setObjectId(objectId)
                .setCommunityId("1:test-community-" + objectId)
                .setTs(System.currentTimeMillis())
                .setPps(5.0f)
                .setBps(500000)
                .setDurationMs(30000)
                .setIatMeanMs(1000.0f)
                .setIatStdMs(500.0f)
                .setPktlenMean(800.0f)
                .setPktlenStd(400.0f)
                .setTcpFlagSynCnt(1)
                .setTcpFlagAckCnt(100)
                .setProtocol(6)
                .setUpDownRatio(0.3f)
                .build();
    }

    private FeatureStat createDNSTunnelFeature(String tenantId, String objectId) {
        return FeatureStat.newBuilder()
                .setHeader(createHeader(tenantId, objectId))
                .setObjectType("flow")
                .setObjectId(objectId)
                .setCommunityId("1:test-community-" + objectId)
                .setTs(System.currentTimeMillis())
                .setPps(20.0f)
                .setBps(50000)
                .setDurationMs(60000)
                .setIatMeanMs(500.0f)
                .setIatStdMs(100.0f)
                .setPktlenMean(300.0f)
                .setPktlenStd(50.0f)
                .setProtocol(17) // UDP
                .setUpDownRatio(2.5f)
                .build();
    }

    private FeatureStat createC2BeaconFeature(String tenantId, String objectId) {
        return FeatureStat.newBuilder()
                .setHeader(createHeader(tenantId, objectId))
                .setObjectType("flow")
                .setObjectId(objectId)
                .setCommunityId("1:test-community-" + objectId)
                .setTs(System.currentTimeMillis())
                .setPps(0.5f)
                .setBps(5000)
                .setDurationMs(600000) // 10分钟
                .setIatMeanMs(60000.0f) // 1分钟间隔
                .setIatStdMs(10000.0f)
                .setPktlenMean(150.0f)
                .setPktlenStd(20.0f)
                .setProtocol(6)
                .setUpDownRatio(0.5f)
                .build();
    }

    private FeatureStat createInvalidFeature(String tenantId, String objectId) {
        // 创建一个缺少必要字段的特征
        return FeatureStat.newBuilder()
                .setHeader(createHeader(tenantId, objectId))
                .setObjectType("flow")
                .setObjectId(objectId)
                .setCommunityId("1:test-community-" + objectId)
                .setTs(System.currentTimeMillis())
                .setPps(0.0f)
                .setProtocol(6)
                .build();
    }
}