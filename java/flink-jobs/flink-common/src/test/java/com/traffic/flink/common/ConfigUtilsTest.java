package com.traffic.flink.common;

import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class ConfigUtilsTest {

    @Test
    void environmentVariablesExposeCanonicalDottedKeys() {
        ParameterTool parameters = ConfigUtils.environmentParameters(Map.of(
                "POSTGRES_PASSWORD", "pg-secret",
                "KAFKA_SASL_JAAS_CONFIG", "jaas-secret"
        ));

        assertEquals("pg-secret", parameters.get("POSTGRES_PASSWORD"));
        assertEquals("pg-secret", parameters.get("postgres.password"));
        assertEquals("jaas-secret", parameters.get("kafka.sasl.jaas.config"));
    }

    @Test
    void laterLayersOverrideEnvironmentValues() {
        ParameterTool environment = ConfigUtils.environmentParameters(
                Map.of("CLICKHOUSE_PASSWORD", "env-secret"));
        ParameterTool cli = ParameterTool.fromArgs(
                new String[]{"--clickhouse.password", "cli-secret"});

        assertEquals(
                "cli-secret",
                environment.mergeWith(cli).get("clickhouse.password"));
    }
}
