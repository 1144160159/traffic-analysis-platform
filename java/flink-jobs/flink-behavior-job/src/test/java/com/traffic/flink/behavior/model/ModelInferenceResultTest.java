////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/model/ModelInferenceResultTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * ModelInferenceResult 单元测试
 * 
 * 测试模型推理结果的 Builder 模式和工具方法
 */
@DisplayName("ModelInferenceResult Tests")
public class ModelInferenceResultTest {

    @Nested
    @DisplayName("Builder 模式测试")
    class BuilderTests {

        @Test
        @DisplayName("应该使用 Builder 创建成功结果")
        void shouldCreateSuccessResultWithBuilder() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .topLabel("test-label")
                    .topScore(0.85f)
                    .detected(true)
                    .inferenceTimeMs(10L)
                    .build();

            assertNotNull(result);
            assertEquals("test-model", result.getModelName());
            assertEquals("v1.0", result.getModelVersion());
            assertEquals("test-label", result.getTopLabel());
            assertEquals(0.85f, result.getTopScore());
            assertTrue(result.isDetected());
            assertEquals(10L, result.getInferenceTimeMs());
        }

        @Test
        @DisplayName("应该能添加多个标签")
        void shouldAddMultipleLabels() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.8f)
                    .addLabel("label2", 0.6f)
                    .addLabel("label3", 0.4f)
                    .build();

            assertEquals(3, result.getLabels().size());
            assertEquals(3, result.getScores().size());
            assertEquals("label1", result.getTopLabel());
            assertEquals(0.8f, result.getTopScore());
        }

        @Test
        @DisplayName("自动选择最高分的标签为 topLabel")
        void shouldAutoSelectHighestScoreAsTop() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.3f)
                    .addLabel("label2", 0.9f)
                    .addLabel("label3", 0.5f)
                    .build();

            assertEquals("label2", result.getTopLabel());
            assertEquals(0.9f, result.getTopScore());
        }

        @Test
        @DisplayName("应该能添加特征重要性")
        void shouldAddFeatureImportance() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .topLabel("test-label")
                    .topScore(0.7f)
                    .addFeatureImportance("feature1", 0.5f)
                    .addFeatureImportance("feature2", 0.3f)
                    .build();

            assertEquals(2, result.getFeatureImportance().size());
            assertTrue(result.getFeatureImportance().containsKey("feature1"));
        }

        @Test
        @DisplayName("未设置标签时应自动添加 normal")
        void shouldAutoAddNormalWhenNoLabels() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .build();

            assertEquals(1, result.getLabels().size());
            assertEquals("normal", result.getTopLabel());
            assertEquals(1.0f, result.getTopScore());
        }

        @Test
        @DisplayName("未检测时应设置 normal 为 topLabel")
        void shouldSetNormalWhenNotDetected() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .detected(false)
                    .build();

            assertEquals("normal", result.getTopLabel());
            assertFalse(result.isDetected());
        }
    }

    @Nested
    @DisplayName("失败结果测试")
    class FailureTests {

        @Test
        @DisplayName("应该创建失败结果")
        void shouldCreateFailureResult() {
            ModelInferenceResult result = ModelInferenceResult
                    .failure("test-model", "v1.0", "Something went wrong");

            assertNotNull(result);
            assertTrue(result.hasError());
            assertEquals("test-model", result.getModelName());
            assertEquals("v1.0", result.getModelVersion());
            assertEquals("Something went wrong", result.getErrorMessage());
        }

        @Test
        @DisplayName("失败结果应该有错误标志")
        void failureResultShouldHaveErrorFlag() {
            ModelInferenceResult result = ModelInferenceResult
                    .failure("test-model", "v1.0", "Error");

            assertTrue(result.hasError());
            assertNotNull(result.getErrorMessage());
        }
    }

    @Nested
    @DisplayName("空结果测试")
    class EmptyResultTests {

        @Test
        @DisplayName("应该创建空结果")
        void shouldCreateEmptyResult() {
            ModelInferenceResult result = ModelInferenceResult
                    .empty("test-model", "v1.0");

            assertNotNull(result);
            assertEquals("test-model", result.getModelName());
            assertEquals("v1.0", result.getModelVersion());
            assertEquals("normal", result.getTopLabel());
            assertEquals(1.0f, result.getTopScore());
            assertFalse(result.isDetected());
            assertFalse(result.hasError());
        }
    }

    @Nested
    @DisplayName("工具方法测试")
    class UtilityMethodTests {

        @Test
        @DisplayName("应该能获取指定标签的分数")
        void shouldGetScoreForLabel() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.8f)
                    .addLabel("label2", 0.6f)
                    .build();

            assertEquals(0.8f, result.getScoreForLabel("label1"));
            assertEquals(0.6f, result.getScoreForLabel("label2"));
            assertEquals(0.0f, result.getScoreForLabel("nonexistent"));
        }

        @Test
        @DisplayName("应该能获取超过阈值的标签")
        void shouldGetLabelsAboveThreshold() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.9f)
                    .addLabel("label2", 0.7f)
                    .addLabel("label3", 0.5f)
                    .build();

            List<String> aboveThreshold = result.getLabelsAboveThreshold(0.6f);
            
            assertEquals(2, aboveThreshold.size());
            assertTrue(aboveThreshold.contains("label1"));
            assertTrue(aboveThreshold.contains("label2"));
            assertFalse(aboveThreshold.contains("label3"));
        }

        @Test
        @DisplayName("高阈值应该返回少量标签")
        void highThresholdShouldReturnFewLabels() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.9f)
                    .addLabel("label2", 0.7f)
                    .addLabel("label3", 0.5f)
                    .addLabel("label4", 0.3f)
                    .build();

            List<String> aboveThreshold = result.getLabelsAboveThreshold(0.8f);
            
            assertEquals(1, aboveThreshold.size());
            assertTrue(aboveThreshold.contains("label1"));
        }
    }

    @Nested
    @DisplayName("边界条件测试")
    class BoundaryTests {

        @Test
        @DisplayName("分数应该在 [0, 1] 范围内")
        void scoresShouldBeInZeroToOneRange() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 1.0f)
                    .addLabel("label2", 0.0f)
                    .addLabel("label3", 0.5f)
                    .build();

            for (float score : result.getScores()) {
                assertTrue(score >= 0.0f && score <= 1.0f,
                        "Score " + score + " should be in [0, 1]");
            }
        }

        @Test
        @DisplayName("空标签列表应该不崩溃")
        void emptyLabelsShouldNotCrash() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .build();

            assertNotNull(result.getLabels());
            assertEquals(1, result.getLabels().size()); // 自动添加了 normal
        }

        @Test
        @DisplayName("特征重要性应该是不可变的")
        void featureImportanceShouldBeImmutable() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addFeatureImportance("f1", 0.5f)
                    .build();

            assertThrows(UnsupportedOperationException.class, 
                    () -> result.getFeatureImportance().put("f2", 0.3f));
        }

        @Test
        @DisplayName("标签列表应该是不可变的")
        void labelsShouldBeImmutable() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .addLabel("label1", 0.5f)
                    .build();

            assertThrows(UnsupportedOperationException.class, 
                    () -> result.getLabels().add("label2"));
        }
    }

    @Nested
    @DisplayName("toString 测试")
    class ToStringTests {

        @Test
        @DisplayName("应该包含关键信息")
        void shouldContainKeyInfo() {
            ModelInferenceResult result = ModelInferenceResult
                    .success("test-model", "v1.0")
                    .topLabel("test-label")
                    .topScore(0.85f)
                    .detected(true)
                    .inferenceTimeMs(10L)
                    .build();

            String str = result.toString();
            
            assertTrue(str.contains("test-model"));
            assertTrue(str.contains("v1.0"));
            assertTrue(str.contains("test-label"));
            assertTrue(str.contains("0.85"));
            assertTrue(str.contains("true"));
        }

        @Test
        @DisplayName("失败结果应该包含错误信息")
        void failureResultShouldContainErrorInfo() {
            ModelInferenceResult result = ModelInferenceResult
                    .failure("test-model", "v1.0", "Test error");

            String str = result.toString();
            
            assertTrue(str.contains("error"));
            assertTrue(str.contains("Test error"));
        }
    }
}