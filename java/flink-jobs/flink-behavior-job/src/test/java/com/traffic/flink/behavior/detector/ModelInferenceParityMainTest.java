package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.model.FeatureStatVectorizer;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ModelInferenceParityMainTest {

    @Test
    void parityRouteUsesTheProductionFeatureStatOrder() {
        Map<String, Float> values = Map.of(
                "bps", 8192.0f,
                "iat_mean_ms", 12.5f,
                "pktlen_mean", 512.0f,
                "pps", 42.0f);
        FeatureStat feature = ModelInferenceParityMain.featureStat(values);

        assertArrayEquals(
                new float[]{8192.0f, 12.5f, 512.0f, 42.0f},
                FeatureStatVectorizer.vectorize(
                        feature, List.of("bps", "iat_mean_ms", "pktlen_mean", "pps")));
    }

    @Test
    void percentileUsesBoundedNearestRank() {
        List<Double> values = new ArrayList<>(List.of(1.0, 2.0, 3.0, 4.0, 5.0));
        assertEquals(3.0, ModelInferenceParityMain.percentile(values, 0.50));
        assertEquals(5.0, ModelInferenceParityMain.percentile(values, 0.99));
        assertThrows(IllegalArgumentException.class,
                () -> ModelInferenceParityMain.percentile(List.of(), 0.95));
    }

    @Test
    void unsupportedFeatureCannotBeSilentlyZeroFilled() {
        assertThrows(IllegalArgumentException.class,
                () -> ModelInferenceParityMain.featureStat(Map.of("unknown_feature", 1.0f)));
    }
}
