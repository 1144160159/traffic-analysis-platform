package com.traffic.flink.behavior.model;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

@DisplayName("AnomalyDetectionModel Tests")
class AnomalyDetectionModelTest {

    private AnomalyDetectionModel model;

    @BeforeEach
    void setUp() throws Exception {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .anomalyThreshold(0.6f)
                .modelVersion("v1.0-test")
                .build();
        model = new AnomalyDetectionModel(config);
        model.initialize();
    }

    @Test
    @DisplayName("高 Z-Score 流量应被判定为异常")
    void testHighZScoreAnomaly() {
        FeatureStat feature = baseFeature()
                .setPps(10000.0f)      // 远高于基线
                .setBps(50_000_000.0f) // 远高于基线
                .setDurationMs(300_000)
                .build();

        ModelInferenceResult result = model.infer(feature);
        assertNotNull(result);
        assertFalse(result.hasError());
        assertTrue(result.isDetected());
        assertNotEquals("normal", result.getTopLabel());
        assertTrue(result.getTopScore() >= 0.6f);
    }

    @Test
    @DisplayName("基线附近的流量应为 normal")
    void testBaselineNormal() {
        FeatureStat feature = baseFeature().build();
        ModelInferenceResult result = model.infer(feature);
        assertNotNull(result);
        assertFalse(result.hasError());
        // 允许 topScore < threshold 时被归为 normal
        assertEquals("normal", result.getTopLabel());
    }

    private FeatureStat.Builder baseFeature() {
        long now = System.currentTimeMillis();
        return FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("test-tenant")
                        .setEventId("event-1")
                        .build())
                .setObjectType("flow")
                .setObjectId("obj-1")
                .setCommunityId("1:abc")
                .setTs(now)
                .setProtocol(6)
                .setDurationMs(30_000)
                .setPps(100.0f)
                .setBps(1_000_000.0f)
                .setUpDownRatio(1.0f)
                .setPktlenMean(200.0f)
                .setPktlenStd(20.0f)
                .setIatMeanMs(10.0f)
                .setIatStdMs(2.0f)
                .setActiveMeanMs(100.0f)
                .setIdleMeanMs(100.0f)
                .setTcpFlagSynCnt(1)
                .setTcpFlagAckCnt(10)
                .setTcpInitWinBytesFwd(65_535)
                .setTcpInitWinBytesBwd(65_535);
    }
}