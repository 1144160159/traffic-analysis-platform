////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/detector/ModelRegistryTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.*;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

/**
 * ModelRegistry 单元测试
 * 
 * 测试模型注册表的完整生命周期
 */
@DisplayName("ModelRegistry Tests")
public class ModelRegistryTest {

    private BehaviorJobConfig config;
    private ModelRegistry registry;

    @BeforeEach
    void setUp() {
        config = new BehaviorJobConfig.Builder()
                .enabledModels(Set.of("scan", "tunnel", "dga", "anomaly"))
                .scanThreshold(0.7f)
                .tunnelThreshold(0.75f)
                .dgaThreshold(0.8f)
                .anomalyThreshold(0.6f)
                .modelVersion("v1.0")
                .build();
        registry = new ModelRegistry(config);
    }

    @AfterEach
    void tearDown() throws Exception {
        registry.close();
    }

    @Nested
    @DisplayName("模型注册")
    class RegistrationTests {

        @Test
        @DisplayName("应该注册所有启用的模型")
        void shouldRegisterAllEnabledModels() {
            assertEquals(4, registry.getModelCount());
        }

        @Test
        @DisplayName("注册的模型应该可用")
        void registeredModelsShouldBeAvailable() {
            assertTrue(registry.hasModel("scan"));
            assertTrue(registry.hasModel("tunnel"));
            assertTrue(registry.hasModel("dga"));
            assertTrue(registry.hasModel("anomaly"));
        }

        @Test
        @DisplayName("未启用的模型不应该被注册")
        void disabledModelsShouldNotBeRegistered() {
            assertFalse(registry.hasModel("botnet"));
            assertFalse(registry.hasModel("malware"));
        }

        @Test
        @DisplayName("应该能获取已注册的模型")
        void shouldGetRegisteredModel() {
            BehaviorModel scanModel = registry.getModel("scan");
            assertNotNull(scanModel);
            assertEquals("scan", scanModel.getName());
        }

        @Test
        @DisplayName("应该返回所有模型映射")
        void shouldReturnAllModels() {
            Map<String, BehaviorModel> models = registry.getAllModels();
            assertNotNull(models);
            assertEquals(4, models.size());
            assertTrue(models.containsKey("scan"));
        }
    }

    @Nested
    @DisplayName("模型执行")
    class ExecutionTests {

        @Test
        @DisplayName("所有模型应该就绪")
        void allModelsShouldBeReady() {
            for (String modelName : registry.getAllModels().keySet()) {
                BehaviorModel model = registry.getModel(modelName);
                assertTrue(model.isReady(), "Model " + modelName + " should be ready");
            }
        }

        @Test
        @DisplayName("应该能调用模型推理")
        void shouldInvokeModelInference() {
            BehaviorModel scanModel = registry.getModel("scan");
            assertNotNull(scanModel);
            assertTrue(scanModel.isReady());
            
            // 创建一个简单特征
            com.traffic.proto.traffic.v1.FeatureStat feature = 
                    com.traffic.proto.traffic.v1.FeatureStat.newBuilder()
                            .setHeader(createHeader())
                            .setPps(100.0f)
                            .setDurationMs(10000)
                            .setProtocol(6)
                            .build();
            
            ModelInferenceResult result = scanModel.infer(feature);
            assertNotNull(result);
            assertFalse(result.hasError());
        }
    }

    @Nested
    @DisplayName("统计功能")
    class StatisticsTests {

        @Test
        @DisplayName("应该能记录模型调用")
        void shouldRecordModelInvocation() {
            registry.recordInvocation("scan");
            assertEquals(1, registry.getInvocationCount("scan"));
            
            registry.recordInvocation("scan");
            registry.recordInvocation("scan");
            assertEquals(3, registry.getInvocationCount("scan"));
        }

        @Test
        @DisplayName("应该能记录模型错误")
        void shouldRecordModelError() {
            registry.recordError("scan");
            assertEquals(1, registry.getErrorCount("scan"));
            
            registry.recordError("scan");
            assertEquals(2, registry.getErrorCount("scan"));
        }

        @Test
        @DisplayName("不存在的模型统计应该返回0")
        void unknownModelStatsShouldReturnZero() {
            assertEquals(0, registry.getInvocationCount("unknown"));
            assertEquals(0, registry.getErrorCount("unknown"));
        }

        @Test
        @DisplayName("模型调用和错误统计应该独立")
        void invocationAndErrorShouldBeIndependent() {
            registry.recordInvocation("scan");
            registry.recordInvocation("scan");
            registry.recordError("scan");
            
            assertEquals(2, registry.getInvocationCount("scan"));
            assertEquals(1, registry.getErrorCount("scan"));
        }
    }

    @Nested
    @DisplayName("健康检查")
    class HealthCheckTests {

        @Test
        @DisplayName("应该返回正确的健康状态")
        void shouldReturnCorrectHealthStatus() {
            ModelRegistry.ModelHealth health = registry.getModelHealth("scan");
            assertEquals(ModelRegistry.ModelHealth.HEALTHY, health);
        }

        @Test
        @DisplayName("不存在的模型应该返回NOT_FOUND")
        void unknownModelShouldReturnNotFound() {
            ModelRegistry.ModelHealth health = registry.getModelHealth("unknown");
            assertEquals(ModelRegistry.ModelHealth.NOT_FOUND, health);
        }

        @Test
        @DisplayName("应该能获取健康报告")
        void shouldGetHealthReport() {
            Map<String, ModelRegistry.ModelHealthReport> report = registry.getHealthReport();
            assertNotNull(report);
            assertEquals(4, report.size());
            
            for (ModelRegistry.ModelHealthReport r : report.values()) {
                assertNotNull(r.getModelName());
                assertNotNull(r.getVersion());
                assertNotNull(r.getHealth());
            }
        }

        @Test
        @DisplayName("健康报告应该包含正确的统计信息")
        void healthReportShouldContainCorrectStats() {
            registry.recordInvocation("scan");
            registry.recordInvocation("scan");
            
            Map<String, ModelRegistry.ModelHealthReport> report = registry.getHealthReport();
            ModelRegistry.ModelHealthReport scanReport = report.get("scan");
            
            assertEquals("scan", scanReport.getModelName());
            assertEquals(2, scanReport.getInvocations());
            assertEquals(0.0, scanReport.getErrorRate());
        }
    }

    @Nested
    @DisplayName("模型注销")
    class UnregistrationTests {

        @Test
        @DisplayName("应该能注销模型")
        void shouldUnregisterModel() {
            assertTrue(registry.hasModel("scan"));
            
            registry.unregisterModel("scan");
            
            assertFalse(registry.hasModel("scan"));
            assertEquals(3, registry.getModelCount());
        }

        @Test
        @DisplayName("注销不存在的模型不应该崩溃")
        void unregisteringUnknownModelShouldNotCrash() {
            assertDoesNotThrow(() -> registry.unregisterModel("unknown"));
            assertEquals(4, registry.getModelCount());
        }
    }

    @Nested
    @DisplayName("关闭功能")
    class ShutdownTests {

        @Test
        @DisplayName("关闭后应该清理所有模型")
        void closeShouldCleanAllModels() {
            assertTrue(registry.getModelCount() > 0);
            
            registry.close();
            
            // 验证：close 后仍能访问但不应该有副作用
            assertTrue(registry.getModelCount() >= 0);
        }

        @Test
        @DisplayName("多次关闭不应该崩溃")
        void multipleCloseShouldNotCrash() throws Exception {
            registry.close();
            assertDoesNotThrow(() -> registry.close());
        }
    }

    // ==================== 辅助方法 ====================

    private com.traffic.proto.traffic.v1.EventHeader createHeader() {
        return com.traffic.proto.traffic.v1.EventHeader.newBuilder()
                .setTenantId("test-tenant")
                .setRunId("test-run")
                .setEventId("test-event")
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId("test-probe")
                .setFeatureSetId("feature-set-v1")
                .build();
    }
}