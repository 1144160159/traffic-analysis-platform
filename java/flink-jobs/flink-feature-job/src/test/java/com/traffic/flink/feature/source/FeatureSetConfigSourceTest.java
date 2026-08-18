package com.traffic.flink.feature.source;

import com.traffic.flink.feature.config.FeatureSetConfig;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FeatureSetConfigSourceTest {

    private final FeatureSetConfigSource source = new FeatureSetConfigSource(
            "jdbc:postgresql://unused/test", "unused", "unused", 1000L);

    @Test
    void parsesEverySupportedFieldAndIgnoresUnknownFields() {
        FeatureSetConfig config = source.parseConfig(
                "fs-1",
                "v3",
                "runtime config",
                "{"
                        + "\"iat_threshold_ms\":250.5,"
                        + "\"enable_l2_trigger\":true,"
                        + "\"l2_thresholds\":{"
                        + "\"high_pps_threshold\":12.5,"
                        + "\"high_bps_threshold\":9000,"
                        + "\"encrypted_std_payload_threshold\":7.25,"
                        + "\"tls_port\":8443,"
                        + "\"http_port\":8080},"
                        + "\"feature_weights\":{\"z\":0.25,\"a\":1.5},"
                        + "\"future_field\":{\"compatible\":true}"
                        + "}",
                1234L);

        assertEquals("fs-1", config.getFeatureSetId());
        assertEquals("v3", config.getSchemaVersion());
        assertEquals("runtime config", config.getDescription());
        assertEquals(1234L, config.getUpdatedAt());
        assertTrue(config.isActive());
        assertTrue(config.isEnableL2Trigger());
        assertEquals(250.5f, config.getIatThresholdMs());
        assertEquals(12.5f, config.getL2Thresholds().getHighPpsThreshold());
        assertEquals(9000.0f, config.getL2Thresholds().getHighBpsThreshold());
        assertEquals(7.25f, config.getL2Thresholds().getEncryptedStdPayloadThreshold());
        assertEquals(8443, config.getL2Thresholds().getTlsPort());
        assertEquals(8080, config.getL2Thresholds().getHttpPort());
        assertEquals(2, config.getFeatureWeights().size());
        assertEquals(new ArrayList<>(config.getFeatureWeights().keySet()), java.util.Arrays.asList("a", "z"));
    }

    @Test
    void absentFieldsUseContractDefaultsWithL2Disabled() {
        FeatureSetConfig config = source.parseConfig("fs-2", "v1", null, "{}", 1L);

        assertEquals(1000.0f, config.getIatThresholdMs());
        assertFalse(config.isEnableL2Trigger());
        assertEquals(10000.0f, config.getL2Thresholds().getHighPpsThreshold());
        assertEquals(1.0e9f, config.getL2Thresholds().getHighBpsThreshold());
        assertEquals(443, config.getL2Thresholds().getTlsPort());
        assertTrue(config.getFeatureWeights().isEmpty());
    }

    @Test
    void rejectsMalformedOrUnsafeValues() {
        assertThrows(IllegalArgumentException.class,
                () -> source.parseConfig("fs", "v1", null, "[]", 1L));
        assertThrows(IllegalArgumentException.class,
                () -> source.parseConfig("fs", "v1", null, "{\"iat_threshold_ms\":0}", 1L));
        assertThrows(IllegalArgumentException.class,
                () -> source.parseConfig("fs", "v1", null, "{\"enable_l2_trigger\":\"true\"}", 1L));
        assertThrows(IllegalArgumentException.class,
                () -> source.parseConfig("fs", "v1", null, "{\"l2_thresholds\":{\"tls_port\":70000}}", 1L));
        assertThrows(IllegalArgumentException.class,
                () -> source.parseConfig("fs", "v1", null, "{\"feature_weights\":{\"bad\":-1}}", 1L));
    }

    @Test
    void rejectsNonPositivePollingInterval() {
        assertThrows(IllegalArgumentException.class,
                () -> new FeatureSetConfigSource("url", "user", "password", 0L));
    }
}
