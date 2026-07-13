////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/sink/ClickHouseSinkFactoryTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;
import com.traffic.proto.traffic.v1.EventHeader;

import org.apache.flink.streaming.api.functions.sink.SinkFunction;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * ClickHouseSinkFactory 单元测试
 */
class ClickHouseSinkFactoryTest {

    @Test
    @DisplayName("创建 Campaign Sink")
    void testCreateCampaignSink() {
        SinkFunction<Campaign> sink = ClickHouseSinkFactory.createCampaignSink(
                "localhost:8123",
                "traffic",
                "campaigns_local",
                "default",
                ""
        );

        assertThat(sink).isNotNull();
    }

    @Test
    @DisplayName("验证 Campaign 对象构建")
    void testCampaignBuilding() {
        long now = System.currentTimeMillis();
        String eventId = UUID.randomUUID().toString();

        EventHeader header = EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId("tenant-1")
                .setRunId("realtime")
                .setEventTs(now)
                .setIngestTs(now)
                .setProbeId("cep-engine")
                .setFeatureSetId("campaign")
                .build();

        Campaign campaign = Campaign.newBuilder()
                .setHeader(header)
                .setTenantId("tenant-1")
                .setCampaignId("campaign-test-123")
                .setTsStart(now - 60000)
                .setTsEnd(now)
                .addAllEntities(Arrays.asList("ip:192.168.1.100", "ip:10.0.0.1"))
                .addAllAlerts(Arrays.asList("alert-1", "alert-2"))
                .setScore(0.85f)
                .setSummary("测试战役")
                .setEventId(eventId)
                .setIngestTs(now)
                .setCampaignType("scan_exploit")
                .addAllAttackPhases(Arrays.asList("reconnaissance", "initial_access"))
                .addAllRuleIds(Arrays.asList("rule-v1"))
                .addAllModelIds(Arrays.asList("model-v1"))
                .build();

        assertThat(campaign.getTenantId()).isEqualTo("tenant-1");
        assertThat(campaign.getCampaignId()).isEqualTo("campaign-test-123");
        assertThat(campaign.getEntitiesCount()).isEqualTo(2);
        assertThat(campaign.getAlertsCount()).isEqualTo(2);
        assertThat(campaign.getScore()).isEqualTo(0.85f);
        assertThat(campaign.getCampaignType()).isEqualTo("scan_exploit");
        assertThat(campaign.getAttackPhasesCount()).isEqualTo(2);
    }
}