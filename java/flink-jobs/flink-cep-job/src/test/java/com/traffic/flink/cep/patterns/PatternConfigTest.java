package com.traffic.flink.cep.patterns;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * PatternConfig 单元测试
 */
class PatternConfigTest {

    @Test
    @DisplayName("默认配置值正确")
    void testDefaultValues() {
        PatternConfig config = PatternConfig.defaultConfig();

        assertThat(config.getScanExploitWindowMinutes()).isEqualTo(30);
        assertThat(config.getMinScanCount()).isEqualTo(1);
        assertThat(config.getBruteForceWindowMinutes()).isEqualTo(10);
        assertThat(config.getMinFailedAttempts()).isEqualTo(5);
        assertThat(config.getLateralMovementWindowMinutes()).isEqualTo(60);
        assertThat(config.getMinHops()).isEqualTo(2);
        assertThat(config.getDataExfilWindowMinutes()).isEqualTo(30);
        assertThat(config.getMinExfilBytes()).isEqualTo(10 * 1024 * 1024);
        assertThat(config.getC2WindowMinutes()).isEqualTo(120);
        assertThat(config.getMinBeaconCount()).isEqualTo(5);
        assertThat(config.getBeaconIntervalToleranceRatio()).isEqualTo(0.1f);
    }

    @Test
    @DisplayName("配置值可正确设置和获取")
    void testSettersAndGetters() {
        PatternConfig config = new PatternConfig();

        config.setScanExploitWindowMinutes(60);
        config.setMinScanCount(3);
        config.setBruteForceWindowMinutes(15);
        config.setMinFailedAttempts(10);
        config.setLateralMovementWindowMinutes(120);
        config.setMinHops(3);
        config.setDataExfilWindowMinutes(45);
        config.setMinExfilBytes(50 * 1024 * 1024);
        config.setC2WindowMinutes(180);
        config.setMinBeaconCount(10);
        config.setBeaconIntervalToleranceRatio(0.2f);

        assertThat(config.getScanExploitWindowMinutes()).isEqualTo(60);
        assertThat(config.getMinScanCount()).isEqualTo(3);
        assertThat(config.getBruteForceWindowMinutes()).isEqualTo(15);
        assertThat(config.getMinFailedAttempts()).isEqualTo(10);
        assertThat(config.getLateralMovementWindowMinutes()).isEqualTo(120);
        assertThat(config.getMinHops()).isEqualTo(3);
        assertThat(config.getDataExfilWindowMinutes()).isEqualTo(45);
        assertThat(config.getMinExfilBytes()).isEqualTo(50 * 1024 * 1024);
        assertThat(config.getC2WindowMinutes()).isEqualTo(180);
        assertThat(config.getMinBeaconCount()).isEqualTo(10);
        assertThat(config.getBeaconIntervalToleranceRatio()).isEqualTo(0.2f);
    }

    @Test
    @DisplayName("PatternConfig 可序列化")
    void testSerializable() throws Exception {
        PatternConfig config = new PatternConfig();
        config.setScanExploitWindowMinutes(45);

        // 序列化
        java.io.ByteArrayOutputStream baos = new java.io.ByteArrayOutputStream();
        java.io.ObjectOutputStream oos = new java.io.ObjectOutputStream(baos);
        oos.writeObject(config);
        oos.close();

        // 反序列化
        java.io.ByteArrayInputStream bais = new java.io.ByteArrayInputStream(baos.toByteArray());
        java.io.ObjectInputStream ois = new java.io.ObjectInputStream(bais);
        PatternConfig deserialized = (PatternConfig) ois.readObject();
        ois.close();

        assertThat(deserialized.getScanExploitWindowMinutes()).isEqualTo(45);
    }
}