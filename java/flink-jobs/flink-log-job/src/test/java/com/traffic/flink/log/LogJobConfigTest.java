package com.traffic.flink.log;

import com.traffic.flink.common.DeploymentActivation;
import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class LogJobConfigTest {
    private static final String DIGEST = "b".repeat(64);

    @Test
    void legacyDefaultsPreserveExistingProjectionBehavior() {
        LogJobConfig config = LogJobConfig.from(ParameterTool.fromMap(Map.of()));

        assertEquals(LogJobConfig.INPUT_TOPIC, config.getInputTopic());
        assertEquals(LogJobConfig.CANONICAL_GROUP, config.getConsumerGroup());
        assertEquals(DeploymentActivation.Mode.LEGACY, config.getActivation().getMode());
        assertTrue(config.projectionWritesEnabled());
        assertFalse(config.sourceFactWritesEnabled());
        assertEquals(LogJobConfig.AUDIT_TOPIC, config.getAuditTopic());
        assertEquals("false", config.getKafkaConsumerProperties()
                .getProperty("enable.auto.commit"));
        assertEquals("true", config.getKafkaConsumerProperties()
                .getProperty("commit.offsets.on.checkpoint"));
        assertEquals("read_committed", config.getKafkaConsumerProperties()
                .getProperty("isolation.level"));
    }

    @Test
    void candidateBoundShadowIsConsumerReadyAndUsesIsolatedOffsets() {
        Map<String, String> values = new HashMap<>();
        values.put("deployment.activation.mode", "shadow");
        values.put("deployment.candidate.sha256", DIGEST);
        values.put("kafka.group.id", "flink-log-job-shadow-bbbbbbbbbbbb");
        values.put("projection.writes.enabled", "false");

        LogJobConfig config = LogJobConfig.from(ParameterTool.fromMap(values));

        assertEquals(DeploymentActivation.Mode.SHADOW, config.getActivation().getMode());
        assertFalse(config.projectionWritesEnabled());
        assertEquals("flink-log-job-dlq-v1-bbbbbbbbbbbb",
                config.getDlqTransactionalIdPrefix());
        assertEquals("flink-log-job-quality-v1-bbbbbbbbbbbb",
                config.getQualityTransactionalIdPrefix());
    }

    @Test
    void shadowCannotEnableExternalProjections() {
        Map<String, String> values = new HashMap<>();
        values.put("deployment.activation.mode", "shadow");
        values.put("deployment.candidate.sha256", DIGEST);
        values.put("kafka.group.id", "flink-log-job-shadow-bbbbbbbbbbbb");
        values.put("projection.writes.enabled", "true");

        assertThrows(IllegalArgumentException.class,
                () -> LogJobConfig.from(ParameterTool.fromMap(values)));
    }

    @Test
    void shadowCannotEnableSourceFactWrites() {
        Map<String, String> values = new HashMap<>();
        values.put("deployment.activation.mode", "shadow");
        values.put("deployment.candidate.sha256", DIGEST);
        values.put("kafka.group.id", "flink-log-job-shadow-bbbbbbbbbbbb");
        values.put("projection.writes.enabled", "false");
        values.put("source.fact.writes.enabled", "true");

        assertThrows(IllegalArgumentException.class,
                () -> LogJobConfig.from(ParameterTool.fromMap(values)));
    }

    @Test
    void canonicalTopicsAndCheckpointTransactionBudgetAreFailClosed() {
        assertThrows(IllegalArgumentException.class, () -> LogJobConfig.from(
                ParameterTool.fromMap(Map.of("kafka.input.topic", "device.logs.legacy"))));
        assertThrows(IllegalArgumentException.class, () -> LogJobConfig.from(
                ParameterTool.fromMap(Map.of("kafka.dlq.topic", "dlq.log-job"))));
        assertThrows(IllegalArgumentException.class, () -> LogJobConfig.from(
                ParameterTool.fromMap(Map.of("kafka.audit.topic", "quality.events"))));
        assertThrows(IllegalArgumentException.class, () -> LogJobConfig.from(
                ParameterTool.fromMap(Map.of(
                        "kafka.dlq.transactional.id.prefix", "same",
                        "kafka.quality.transactional.id.prefix", "same"))));
        assertThrows(IllegalArgumentException.class, () -> LogJobConfig.from(
                ParameterTool.fromMap(Map.of(
                        "checkpoint.interval.ms", "30000",
                        "checkpoint.timeout.ms", "300000",
                        "kafka.transaction.timeout.ms", "330000"))));
    }
}
