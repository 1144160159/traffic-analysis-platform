package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;

import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.header.internals.RecordHeader;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * Kafka Campaign Sink 工厂
 */
public class KafkaSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(KafkaSinkFactory.class);

    /**
     * 创建 Campaign Kafka Sink
     */
    public static KafkaSink<Campaign> createCampaignSink(String brokers, String topic) {
        LOG.info("Creating Kafka campaign sink: {} -> {}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("retry.backoff.ms", "1000");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("batch.size", "65536");
        producerProps.setProperty("linger.ms", "10");
        producerProps.setProperty("buffer.memory", "67108864");

        return KafkaSink.<Campaign>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new CampaignKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * Campaign Kafka 序列化器
     */
    private static class CampaignKafkaSerializer implements KafkaRecordSerializationSchema<Campaign> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public CampaignKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                Campaign element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
				throw new IllegalArgumentException("campaign Kafka publisher rejects null records");
            }

			return buildCampaignRecord(topic, element);
        }

    }

	static String buildCampaignKey(Campaign campaign) {
		if (campaign == null) {
			throw new IllegalArgumentException("campaign Kafka publisher rejects null records");
		}
		String tenantId = campaign.getTenantId().trim();
		String campaignId = campaign.getCampaignId().trim();
		if (tenantId.isEmpty() || "unknown".equalsIgnoreCase(tenantId) || campaignId.isEmpty() || !campaign.hasHeader() ||
				!tenantId.equals(campaign.getHeader().getTenantId())) {
			throw new IllegalArgumentException("campaign Kafka key requires a consistent tenant and campaign identity");
		}
		return tenantId + ":" + campaignId;
	}

	static ProducerRecord<byte[], byte[]> buildCampaignRecord(String topic, Campaign campaign) {
		if (topic == null || topic.trim().isEmpty()) {
			throw new IllegalArgumentException("campaign Kafka topic is required");
		}
		String key = buildCampaignKey(campaign);
		Long recordTimestamp = campaign.getTsEnd() > 0 ? campaign.getTsEnd() : null;
		ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
				topic, null, recordTimestamp, key.getBytes(StandardCharsets.UTF_8), campaign.toByteArray());
		addHeader(record, "content_type", "application/x-protobuf");
		addHeader(record, "proto_message_type", "traffic.v1.Campaign");
		addHeader(record, "schema_version", "1");
		addHeader(record, "source_service", "flink-cep-job");
		addHeader(record, "target_topic", topic);
		addHeader(record, "tenant_id", campaign.getTenantId());
		addHeader(record, "campaign_id", campaign.getCampaignId());
		addHeader(record, "event_id", campaign.getEventId());
		return record;
	}

	private static void addHeader(ProducerRecord<byte[], byte[]> record, String name, String value) {
		record.headers().add(new RecordHeader(name, value.getBytes(StandardCharsets.UTF_8)));
	}
}
