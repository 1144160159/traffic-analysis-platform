package com.traffic.flink.rule.sink;

import com.traffic.proto.traffic.v1.DetectionBehavior;

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
 * Kafka Detection Sink 工厂
 */
public class KafkaDetectionSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(KafkaDetectionSinkFactory.class);

    /**
     * 创建 DetectionBehavior Kafka Sink
     */
    public static KafkaSink<DetectionBehavior> createDetectionSink(String brokers, String topic) {
        LOG.info("Creating Kafka detection sink: {} -> {}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("retry.backoff.ms", "1000");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("batch.size", "65536");
        producerProps.setProperty("linger.ms", "10");
        producerProps.setProperty("buffer.memory", "67108864");

        return KafkaSink.<DetectionBehavior>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DetectionKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * DetectionBehavior Kafka 序列化器
     */
    private static class DetectionKafkaSerializer implements KafkaRecordSerializationSchema<DetectionBehavior> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public DetectionKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                DetectionBehavior element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            // Key: tenant_id:community_id
            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            
            // Value: Protobuf 二进制
            byte[] valueBytes = element.toByteArray();

            // 使用检测时间作为 Kafka 记录时间戳
            Long recordTimestamp = element.getTs() > 0 ? element.getTs() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes);
        }

        private String buildKey(DetectionBehavior detection) {
            String tenantId = detection.getHeader().getTenantId();
            String communityId = detection.getCommunityId();
            
            if (tenantId == null) tenantId = "unknown";
            if (communityId == null) communityId = "unknown";
            
            return tenantId + ":" + communityId;
        }
    }
}
