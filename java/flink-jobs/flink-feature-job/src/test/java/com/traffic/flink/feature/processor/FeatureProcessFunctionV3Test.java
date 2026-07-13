package com.traffic.flink.feature.processor;

import com.traffic.flink.feature.config.TenantConfig;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class FeatureProcessFunctionV3Test {

    @Test
    @DisplayName("默认租户配置不应在无显式配置时触发降级丢弃")
    void defaultTenantConfigKeepsLiveTrafficEnabled() {
        TenantConfig config = new FeatureProcessFunctionV3().createDefaultTenantConfig();

        assertEquals("default", config.getTenantId());
        assertEquals(10, config.getPriority());
        assertFalse(config.isEnableDegradation());
        assertTrue(config.isEnableL2());
        assertEquals(1.0f, config.getSamplingRate(), 0.0001f);
        assertEquals(-1, config.getMaxEventsPerSecond());
    }
}
