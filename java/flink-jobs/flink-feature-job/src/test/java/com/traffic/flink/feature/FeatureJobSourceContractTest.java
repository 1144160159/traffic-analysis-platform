package com.traffic.flink.feature;

import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.Properties;

import static org.junit.jupiter.api.Assertions.assertEquals;

class FeatureJobSourceContractTest {

    @Test
    void sourceOffsetsCanOnlyCommitWithCompletedCheckpoint() {
        ParameterTool params = ParameterTool.fromMap(Map.of(
                "enable.auto.commit", "true",
                "commit.offsets.on.checkpoint", "false"));

        Properties properties = FeatureJob.featureConsumerProperties(params);

        assertEquals("false", properties.getProperty("enable.auto.commit"));
        assertEquals("true", properties.getProperty("commit.offsets.on.checkpoint"));
        assertEquals("30000", properties.getProperty("partition.discovery.interval.ms"));
    }
}
