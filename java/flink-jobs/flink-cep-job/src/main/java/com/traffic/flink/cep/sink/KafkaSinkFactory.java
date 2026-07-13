package com.traffic.flink.cep.sink;

import com.traffic.proto.traffic.v1.Campaign;

import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;

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
                return null;
            }

            // Key: tenant_id:campaign_id
            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            
            // Value: Protobuf 二进制
            byte[] valueBytes = element.toByteArray();

            // 使用战役结束时间作为 Kafka 记录时间戳
            Long recordTimestamp = element.getTsEnd() > 0 ? element.getTsEnd() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes);
        }

        private String buildKey(Campaign campaign) {
            String tenantId = campaign.getTenantId();
            String campaignId = campaign.getCampaignId();
            
            if (tenantId == null) tenantId = "unknown";
            if (campaignId == null) campaignId = "unknown";
            
            return tenantId + ":" + campaignId;
        }
    }
}
