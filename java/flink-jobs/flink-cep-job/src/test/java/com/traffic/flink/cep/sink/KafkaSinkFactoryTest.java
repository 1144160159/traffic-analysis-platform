////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/test/java/com/traffic/flink/cep/sink/KafkaSinkFactoryTest.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;
import com.traffic.proto.traffic.v1.EventHeader;

import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/**
 * KafkaSinkFactory 单元测试
 */
class KafkaSinkFactoryTest {

    @Test
    @DisplayName("创建 Campaign Kafka Sink")
    void testCreateCampaignSink() {
        KafkaSink<Campaign> sink = KafkaSinkFactory.createCampaignSink(
                "localhost:9092",
                "campaigns.v1"
        );

        assertThat(sink).isNotNull();
    }

    @Test
    @DisplayName("验证空 Campaign 处理")
    void testNullCampaignHandling() {
		assertThatThrownBy(() -> KafkaSinkFactory.buildCampaignKey(null))
				.isInstanceOf(IllegalArgumentException.class);
    }

	@Test
	@DisplayName("publisher 拒绝 unknown 或 header 租户不一致")
	void testPublisherIdentityFailsClosed() {
		Campaign valid = Campaign.newBuilder()
				.setTenantId("tenant-a").setCampaignId("campaign-1")
				.setHeader(EventHeader.newBuilder().setTenantId("tenant-a").build()).build();
		assertThat(KafkaSinkFactory.buildCampaignKey(valid)).isEqualTo("tenant-a:campaign-1");
		for (Campaign invalid : new Campaign[]{
				Campaign.newBuilder().setCampaignId("campaign-1").build(),
				Campaign.newBuilder().setTenantId("unknown").setCampaignId("campaign-1").build(),
				Campaign.newBuilder().setTenantId("tenant-a").setCampaignId("campaign-1")
						.setHeader(EventHeader.newBuilder().setTenantId("tenant-b").build()).build()}) {
			assertThatThrownBy(() -> KafkaSinkFactory.buildCampaignKey(invalid))
					.isInstanceOf(IllegalArgumentException.class);
		}
	}

	@Test
	@DisplayName("campaigns.v1 record 冻结 Protobuf envelope")
	void testProtobufEnvelope() throws Exception {
		Campaign campaign = Campaign.newBuilder()
				.setTenantId("tenant-a").setCampaignId("campaign-1").setEventId("event-1")
				.setTsEnd(1700000000000L)
				.setHeader(EventHeader.newBuilder().setTenantId("tenant-a").setEventId("event-1").build())
				.build();
		ProducerRecord<byte[], byte[]> record = KafkaSinkFactory.buildCampaignRecord("campaigns.v1", campaign);
		assertThat(new String(record.key())).isEqualTo("tenant-a:campaign-1");
		assertThat(Campaign.parseFrom(record.value())).isEqualTo(campaign);
		assertThat(new String(record.headers().lastHeader("content_type").value()))
				.isEqualTo("application/x-protobuf");
		assertThat(new String(record.headers().lastHeader("proto_message_type").value()))
				.isEqualTo("traffic.v1.Campaign");
		assertThat(new String(record.headers().lastHeader("tenant_id").value())).isEqualTo("tenant-a");
		assertThat(new String(record.headers().lastHeader("event_id").value())).isEqualTo("event-1");
	}
}
