////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/select/CampaignBuilderUtilsTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.select;

import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.Severity;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * CampaignBuilderUtils 单元测试
 */
class CampaignBuilderUtilsTest {

    @Test
    @DisplayName("提取实体添加 ip: 前缀")
    void testExtractEntities() {
        List<Alert> alerts = Arrays.asList(
                createAlert("192.168.1.100", "10.0.0.1"),
                createAlert("192.168.1.100", "10.0.0.2"),
                createAlert("192.168.1.101", "10.0.0.1")
        );

        Set<String> entities = CampaignBuilderUtils.extractEntities(alerts);

        assertThat(entities).hasSize(4);
        assertThat(entities).contains(
                "ip:192.168.1.100",
                "ip:192.168.1.101",
                "ip:10.0.0.1",
                "ip:10.0.0.2"
        );
    }

    @Test
    @DisplayName("提取实体处理空值")
    void testExtractEntitiesWithNulls() {
        Alert alertWithNullSrc = Alert.newBuilder()
                .setAlertId("alert-1")
                .setDstIp("10.0.0.1")
                .build();

        Alert alertWithEmptySrc = Alert.newBuilder()
                .setAlertId("alert-2")
                .setSrcIp("")
                .setDstIp("10.0.0.2")
                .build();

        List<Alert> alerts = Arrays.asList(alertWithNullSrc, alertWithEmptySrc);
        Set<String> entities = CampaignBuilderUtils.extractEntities(alerts);

        assertThat(entities).hasSize(2);
        assertThat(entities).contains("ip:10.0.0.1", "ip:10.0.0.2");
    }

    @Test
    @DisplayName("提取告警 ID 列表")
    void testExtractAlertIds() {
        List<Alert> alerts = Arrays.asList(
                Alert.newBuilder().setAlertId("alert-1").build(),
                Alert.newBuilder().setAlertId("alert-2").build(),
                Alert.newBuilder().setAlertId("alert-1").build() // 重复
        );

        List<String> alertIds = CampaignBuilderUtils.extractAlertIds(alerts);

        assertThat(alertIds).hasSize(2);
        assertThat(alertIds).contains("alert-1", "alert-2");
    }

    @Test
    @DisplayName("提取规则 ID 列表")
    void testExtractRuleIds() {
        List<Alert> alerts = Arrays.asList(
                Alert.newBuilder().setAlertId("a1").setRuleVersion("rule-v1").build(),
                Alert.newBuilder().setAlertId("a2").setRuleVersion("rule-v2").build(),
                Alert.newBuilder().setAlertId("a3").setRuleVersion("").build(),
                Alert.newBuilder().setAlertId("a4").build()
        );

        List<String> ruleIds = CampaignBuilderUtils.extractRuleIds(alerts);

        assertThat(ruleIds).hasSize(2);
        assertThat(ruleIds).contains("rule-v1", "rule-v2");
    }

    @Test
    @DisplayName("提取模型 ID 列表")
    void testExtractModelIds() {
        List<Alert> alerts = Arrays.asList(
                Alert.newBuilder().setAlertId("a1").setModelVersion("model-v1").build(),
                Alert.newBuilder().setAlertId("a2").setModelVersion("model-v2").build(),
                Alert.newBuilder().setAlertId("a3").setModelVersion("model-v1").build()
        );

        List<String> modelIds = CampaignBuilderUtils.extractModelIds(alerts);

        assertThat(modelIds).hasSize(2);
        assertThat(modelIds).contains("model-v1", "model-v2");
    }

    @Test
    @DisplayName("构建 EventHeader")
    void testBuildEventHeader() {
        long eventTs = System.currentTimeMillis();
        EventHeader header = CampaignBuilderUtils.buildEventHeader("tenant-1", eventTs);

        assertThat(header).isNotNull();
        assertThat(header.getTenantId()).isEqualTo("tenant-1");
        assertThat(header.getEventId()).isNotEmpty();
        assertThat(header.getRunId()).isEqualTo("realtime");
        assertThat(header.getProbeId()).isEqualTo("cep-engine");
        assertThat(header.getFeatureSetId()).isEqualTo("campaign");
        assertThat(header.getEventTs()).isEqualTo(eventTs);
        assertThat(header.getIngestTs()).isGreaterThan(0);
    }

    @Test
    @DisplayName("构建 EventHeader 处理空租户")
    void testBuildEventHeaderWithNullTenant() {
        EventHeader header = CampaignBuilderUtils.buildEventHeader(null, System.currentTimeMillis());

        assertThat(header.getTenantId()).isEqualTo("unknown");
    }

    @Test
    @DisplayName("获取时间范围")
    void testGetTimeRange() {
        long t1 = 1000L;
        long t2 = 2000L;
        long t3 = 3000L;

        List<Alert> alerts = Arrays.asList(
                Alert.newBuilder().setFirstSeen(t2).setLastSeen(t2).build(),
                Alert.newBuilder().setFirstSeen(t1).setLastSeen(t3).build(),
                Alert.newBuilder().setFirstSeen(t3).setLastSeen(t3).build()
        );

        long startTime = CampaignBuilderUtils.getStartTime(alerts);
        long endTime = CampaignBuilderUtils.getEndTime(alerts);

        assertThat(startTime).isEqualTo(t1);
        assertThat(endTime).isEqualTo(t3);
    }

    @Test
    @DisplayName("获取租户 ID")
    void testGetTenantId() {
        List<Alert> alerts = Arrays.asList(
                Alert.newBuilder().setTenantId("tenant-1").build(),
                Alert.newBuilder().setTenantId("tenant-2").build()
        );

        String tenantId = CampaignBuilderUtils.getTenantId(alerts);
        assertThat(tenantId).isEqualTo("tenant-1");
    }

    @Test
    @DisplayName("获取租户 ID 处理空列表")
    void testGetTenantIdWithEmptyList() {
        String tenantId = CampaignBuilderUtils.getTenantId(Arrays.asList());
        assertThat(tenantId).isEqualTo("unknown");
    }

    @Test
    @DisplayName("格式化时间跨度")
    void testFormatDuration() {
        assertThat(CampaignBuilderUtils.formatDuration(30000)).isEqualTo("30秒");
        assertThat(CampaignBuilderUtils.formatDuration(120000)).isEqualTo("2分钟");
        assertThat(CampaignBuilderUtils.formatDuration(5400000)).isEqualTo("1.5小时");
    }

    @Test
    @DisplayName("生成 Campaign ID")
    void testGenerateCampaignId() {
        String campaignId = CampaignBuilderUtils.generateCampaignId("se", "tenant-1", 1234567890L);

        assertThat(campaignId).startsWith("campaign-se-tenant-1-1234567890-");
        assertThat(campaignId).hasSize("campaign-se-tenant-1-1234567890-".length() + 8);
    }

    private Alert createAlert(String srcIp, String dstIp) {
        return Alert.newBuilder()
                .setAlertId("alert-" + System.nanoTime())
                .setTenantId("tenant-1")
                .setSrcIp(srcIp)
                .setDstIp(dstIp)
                .setAlertType("TEST")
                .setSeverity(Severity.SEVERITY_MEDIUM)
                .setFirstSeen(System.currentTimeMillis())
                .setLastSeen(System.currentTimeMillis())
                .build();
    }
}