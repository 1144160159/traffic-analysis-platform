////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/test/java/com/traffic/flink/behavior/model/BotnetDetectionModelTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Nested;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * BotnetDetectionModel 单元测试
 * 
 * 测试僵尸网络检测模型的各种场景
 */
@DisplayName("BotnetDetectionModel Tests")
public class BotnetDetectionModelTest {

    private BehaviorJobConfig config;
    private BotnetDetectionModel model;

    @BeforeEach
    void setUp() throws Exception {
        config = new BehaviorJobConfig.Builder()
                .botnetThreshold(0.7f)
                .modelVersion("v1.0")
                .build();
        model = new BotnetDetectionModel(config);
        model.initialize();
    }

    @Nested
    @DisplayName("C2 信标检测")
    class C2BeaconTests {

        @Test
        @DisplayName("应该检测到 C2 信标 - 规律性心跳")
        void shouldDetectC2Beacon() throws Exception {
            // Given: 周期性心跳特征
            FeatureStat feature = createC2BeaconFeature();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            assertNotNull(result);
            assertTrue(result.getLabels().contains("c2_beacon"));
            assertTrue(result.getScoreForLabel("c2_beacon") > 0.5f);
        }

        @Test
        @DisplayName("高规律性应该有高置信度")
        void highRegularityShouldHaveHighConfidence() throws Exception {
            // Given: 非常规律的心跳（低 IAT CV）
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(createHeader())
                    .setDurationMs(600000) // 10分钟
                    .setIatMeanMs(60000.0f) // 1分钟间隔
                    .setIatStdMs(5000.0f) // CV = 0.083
                    .setPktlenMean(100.0f)
                    .setPps(0.5f)
                    .setBps(5000.0f)
                    .setProtocol(6)
                    .build();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            float beaconScore = result.getScoreForLabel("c2_beacon");
            assertTrue(beaconScore > 0.6f);
        }

        @Test
        @DisplayName("短连接不应该检测为 C2 信标")
        void shortConnectionShouldNotBeC2Beacon() throws Exception {
            // Given: 短连接
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(createHeader())
                    .setDurationMs(5000) // 5秒
                    .setPps(10.0f)
                    .setProtocol(6)
                    .build();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            float beaconScore = result.getScoreForLabel("c2_beacon");
            assertTrue(beaconScore < 0.5f, "Short connection should have low beacon score");
        }
    }

    @Nested
    @DisplayName("DDoS 参与检测")
    class DdosParticipantTests {

        @Test
        @DisplayName("应该检测到 DDoS 参与 - 高PPS + 高上下行比")
        void shouldDetectDdosParticipation() throws Exception {
            // Given: DDoS 攻击特征
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(createHeader())
                    .setPps(5000.0f)
                    .setBps(10000000) // 10 Mbps
                    .setUpDownRatio(50.0f)
                    .setPktlenMean(60.0f)
                    .setProtocol(6)
                    .build();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            assertNotNull(result);
            assertTrue(result.getLabels().contains("ddos_participant"));
            assertTrue(result.getScoreForLabel("ddos_participant") > 0.7f);
        }

        @Test
        @DisplayName("极高 PPS 应该有极高置信度")
        void veryHighPPSShouldHaveVeryHighConfidence() throws Exception {
            // Given: 极高的 PPS
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(createHeader())
                    .setPps(20000.0f)
                    .setUpDownRatio(100.0f)
                    .setProtocol(6)
                    .build();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            float ddosScore = result.getScoreForLabel("ddos_participant");
            assertTrue(ddosScore > 0.8f, "Very high PPS should have very high confidence");
        }
    }

    @Nested
    @DisplayName("挖矿行为检测")
    class CryptoMiningTests {

        @Test
        @DisplayName("应该检测到挖矿行为 - 长连接 + 规律性")
        void shouldDetectCryptoMining() throws Exception {
            // Given: 挖矿特征
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(createHeader())
                    .setDurationMs(3600000) // 1小时
                    .setIatMeanMs(10000.0f)
                    .setIatStdMs(2000.0f) // CV = 0.2
                    .setUpDownRatio(1.0f)
                    .setPps(5.0f)
                    .setBps(10000.0f)
                    .setPktlenMean(300.0f)
                    .setProtocol(6)
                    .build();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            assertNotNull(result);
            assertTrue(result.getLabels().contains("crypto_mining"));
            assertTrue(result.getScoreForLabel("crypto_mining") > 0.3f);
        }
    }

    @Nested
    @DisplayName("正常流量")
    class NormalTrafficTests {

        @Test
        @DisplayName("正常 Web 浏览不应该被标记")
        void normalWebBrowsingShouldNotBeDetected() throws Exception {
            // Given: 正常 Web 流量
            FeatureStat feature = createNormalWebFeature();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            assertNotNull(result);
            assertFalse(result.isDetected(), "Normal web browsing should not be detected");
        }

        @Test
        @DisplayName("正常视频流不应该被标记")
        void normalVideoStreamingShouldNotBeDetected() throws Exception {
            // Given: 视频流（高上下行比但不规律）
            FeatureStat feature = createNormalVideoFeature();

            // When
            ModelInferenceResult result = model.infer(feature);

            // Then
            assertFalse(result.isDetected());
        }
    }

    // ==================== 辅助方法 ====================

    private com.traffic.proto.traffic.v1.EventHeader createHeader() {
        return com.traffic.proto.traffic.v1.EventHeader.newBuilder()
                .setTenantId("default")
                .setRunId("test-run-001")
                .setEventId("test-event-001")
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId("probe-001")
                .setFeatureSetId("feature-set-v1")
                .build();
    }

    private FeatureStat createC2BeaconFeature() {
        return FeatureStat.newBuilder()
                .setHeader(createHeader())
                .setDurationMs(600000) // 10分钟
                .setIatMeanMs(60000.0f) // 1分钟间隔
                .setIatStdMs(10000.0f) // CV = 0.167
                .setPktlenMean(150.0f)
                .setPps(0.1f)
                .setBps(2000.0f)
                .setProtocol(6)
                .build();
    }

    private FeatureStat createNormalWebFeature() {
        return FeatureStat.newBuilder()
                .setHeader(createHeader())
                .setDurationMs(30000) // 30秒
                .setIatMeanMs(100.0f)
                .setIatStdMs(200.0f)
                .setPktlenMean(800.0f)
                .setPps(5.0f)
                .setBps(500000.0f)
                .setProtocol(6)
                .setUpDownRatio(0.3f)
                .build();
    }

    private FeatureStat createNormalVideoFeature() {
        return FeatureStat.newBuilder()
                .setHeader(createHeader())
                .setDurationMs(1800000) // 30分钟
                .setIatMeanMs(50.0f)
                .setIatStdMs(100.0f)
                .setPktlenMean(1200.0f)
                .setPps(100.0f)
                .setBps(5000000.0f)
                .setProtocol(6)
                .setUpDownRatio(1.2f)
                .build();
    }
}