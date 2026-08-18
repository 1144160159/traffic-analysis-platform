package com.traffic.flink.behavior.model;

import com.traffic.proto.traffic.v1.FeatureStat;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class FeatureStatVectorizerTest {
    @Test
    void preservesSignedFeatureOrder() {
        FeatureStat feature = FeatureStat.newBuilder()
                .setPps(12.5f)
                .setProtocol(17)
                .setDurationMs(800)
                .build();

        assertThat(FeatureStatVectorizer.vectorize(
                feature, List.of("protocol", "pps", "duration_ms")))
                .containsExactly(17.0f, 12.5f, 800.0f);
    }

    @Test
    void incompatibleFeatureContractFailsInsteadOfSubstitutingZero() {
        assertThatThrownBy(() -> FeatureStatVectorizer.vectorize(
                FeatureStat.getDefaultInstance(), List.of("future_feature")))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("unsupported governed FeatureStat column");
    }
}
