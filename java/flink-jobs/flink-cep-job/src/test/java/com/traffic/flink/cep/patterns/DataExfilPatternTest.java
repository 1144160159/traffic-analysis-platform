package com.traffic.flink.cep.patterns;

import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.Severity;

import org.apache.flink.cep.pattern.Pattern;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * DataExfilPattern 单元测试
 */
class DataExfilPatternTest {

    @Test
    @DisplayName("创建数据外泄模式")
    void testCreatePattern() {
        Pattern<Alert, ?> pattern = DataExfilPattern.create();

        assertThat(pattern).isNotNull();
    }

    @Test
    @DisplayName("使用自定义配置创建模式")
    void testCreatePatternWithCustomConfig() {
        PatternConfig config = new PatternConfig();
        config.setDataExfilWindowMinutes(45);
        config.setMinExfilBytes(50 * 1024 * 1024);

        Pattern<Alert, ?> pattern = DataExfilPattern.create(config);

        assertThat(pattern).isNotNull();
    }

    @Test
    @DisplayName("验证数据收集告警")
    void testCollectionAlert() {
        Alert collectionAlert = Alert.newBuilder()
                .setAlertId("alert-collection-1")
                .setTenantId("tenant-1")
                .setSrcIp("192.168.1.100")
                .setDstIp("192.168.1.50")
                .setDstPort(445)
                .setAlertType("DATA_ACCESS")
                .addLabels("collection")
                .addLabels("smb")
                .setSeverity(Severity.SEVERITY_MEDIUM)
                .setFirstSeen(System.currentTimeMillis())
                .setLastSeen(System.currentTimeMillis())
                .build();

        assertThat(collectionAlert.getAlertType()).isEqualTo("DATA_ACCESS");
        assertThat(collectionAlert.getLabelsList()).contains("collection");
    }

    @Test
    @DisplayName("验证数据传输告警")
    void testTransferAlert() {
        Alert transferAlert = Alert.newBuilder()
                .setAlertId("alert-transfer-1")
                .setTenantId("tenant-1")
                .setSrcIp("192.168.1.100")
                .setDstIp("203.0.113.50")
                .setDstPort(443)
                .setAlertType("LARGE_UPLOAD")
                .addLabels("high_volume")
                .addLabels("exfil")
                .setSeverity(Severity.SEVERITY_HIGH)
                .setFirstSeen(System.currentTimeMillis())
                .setLastSeen(System.currentTimeMillis())
                .build();

        assertThat(transferAlert.getAlertType()).isEqualTo("LARGE_UPLOAD");
        assertThat(transferAlert.getLabelsList()).contains("high_volume");
    }
}