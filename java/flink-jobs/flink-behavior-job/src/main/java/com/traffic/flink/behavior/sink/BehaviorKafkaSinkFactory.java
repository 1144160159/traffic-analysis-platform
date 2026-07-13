////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/sink/BehaviorKafkaSinkFactory.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.sink;

import com.traffic.proto.traffic.v1.DetectionBehavior;

import org.apache.flink.api.common.serialization.SerializationSchema;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * Kafka Sink 工厂类
 * 
 * 创建用于写入 detections.behavior.v1 Topic 的 Sink。
 * 
 * 特性：
 * 1. 幂等生产者（enable.idempotence=true）
 * 2. 使用 tenant_id + community_id 作为分区键
 * 3. Protobuf 序列化
 * 4. At-least-once 语义
 */
public class BehaviorKafkaSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(BehaviorKafkaSinkFactory.class);

    private BehaviorKafkaSinkFactory() {
        // Utility class
    }

    /**
     * 创建 Kafka Sink
     *
     * @param brokers    Kafka Broker 列表
     * @param topic      目标 Topic
     * @return KafkaSink
     */
    public static KafkaSink<DetectionBehavior> createSink(String brokers, String topic) {
        return createSink(brokers, topic, DeliveryGuarantee.AT_LEAST_ONCE);
    }

    /**
     * 创建 Kafka Sink（可配置投递语义）
     *
     * @param brokers           Kafka Broker 列表
     * @param topic             目标 Topic
     * @param deliveryGuarantee 投递保证级别
     * @return KafkaSink
     */
    public static KafkaSink<DetectionBehavior> createSink(
            String brokers,
            String topic,
            DeliveryGuarantee deliveryGuarantee) {

        LOG.info("Creating Kafka sink: brokers={}, topic={}, deliveryGuarantee={}",
                brokers, topic, deliveryGuarantee);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        
        // 基础配置
        producerProps.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        
        // 幂等生产者配置
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.RETRIES_CONFIG, String.valueOf(Integer.MAX_VALUE));
        producerProps.setProperty(ProducerConfig.MAX_IN_FLIGHT_REQUESTS_PER_CONNECTION, "5");
        
        // 性能优化配置
        producerProps.setProperty(ProducerConfig.LINGER_MS_CONFIG, "5");
        producerProps.setProperty(ProducerConfig.BATCH_SIZE_CONFIG, "65536"); // 64KB
        producerProps.setProperty(ProducerConfig.BUFFER_MEMORY_CONFIG, "67108864"); // 64MB
        producerProps.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        
        // 超时配置
        producerProps.setProperty(ProducerConfig.REQUEST_TIMEOUT_MS_CONFIG, "30000");
        producerProps.setProperty(ProducerConfig.DELIVERY_TIMEOUT_MS_CONFIG, "120000");

        return KafkaSink.<DetectionBehavior>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DetectionBehaviorSerializationSchema(topic))
                .setDeliveryGuarantee(deliveryGuarantee)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * 创建带有自定义配置的 Kafka Sink
     */
    public static KafkaSink<DetectionBehavior> createSink(
            String brokers,
            String topic,
            Properties additionalProps) {

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        
        // 基础配置
        producerProps.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        
        // 合并自定义配置
        if (additionalProps != null) {
            producerProps.putAll(additionalProps);
        }

        LOG.info("Creating Kafka sink with custom config: brokers={}, topic={}", brokers, topic);

        return KafkaSink.<DetectionBehavior>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DetectionBehaviorSerializationSchema(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * DetectionBehavior 序列化 Schema
     * 
     * 实现：
     * 1. 使用 Protobuf 二进制序列化
     * 2. 使用 tenant_id:community_id 作为分区键
     * 3. 在 Header 中添加 event_id 用于幂等
     */
    private static class DetectionBehaviorSerializationSchema 
            implements KafkaRecordSerializationSchema<DetectionBehavior> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public DetectionBehaviorSerializationSchema(String topic) {
            this.topic = topic;
        }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                DetectionBehavior detection,
                KafkaSinkContext context,
                Long timestamp) {

            // 构建分区键：tenant_id:community_id
            String partitionKey = buildPartitionKey(detection);
            byte[] keyBytes = partitionKey.getBytes(StandardCharsets.UTF_8);

            // Protobuf 序列化
            byte[] valueBytes = detection.toByteArray();

            // 创建 ProducerRecord
            ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                    topic,
                    null, // 让 Kafka 根据 key 分区
                    timestamp != null ? timestamp : System.currentTimeMillis(),
                    keyBytes,
                    valueBytes
            );

            // 添加 Header：event_id（用于幂等去重）
            if (detection.hasHeader() && detection.getHeader().getEventId() != null) {
                record.headers().add("event_id", 
                        detection.getHeader().getEventId().getBytes(StandardCharsets.UTF_8));
            }

            // 添加 Header：model_version
            if (detection.getModelVersion() != null) {
                record.headers().add("model_version",
                        detection.getModelVersion().getBytes(StandardCharsets.UTF_8));
            }

            // 添加 Header：top_label
            if (detection.getTopLabel() != null) {
                record.headers().add("top_label",
                        detection.getTopLabel().getBytes(StandardCharsets.UTF_8));
            }

            return record;
        }

        /**
         * 构建分区键
         * 格式：tenant_id:community_id
         */
        private String buildPartitionKey(DetectionBehavior detection) {
            StringBuilder sb = new StringBuilder();

            if (detection.hasHeader() && detection.getHeader().getTenantId() != null) {
                sb.append(detection.getHeader().getTenantId());
            } else {
                sb.append("default");
            }

            sb.append(":");

            if (detection.getCommunityId() != null && !detection.getCommunityId().isEmpty()) {
                sb.append(detection.getCommunityId());
            } else {
                sb.append(detection.getObjectId());
            }

            return sb.toString();
        }
    }

    /**
     * 简单的 Protobuf 序列化器（用于兼容旧 API）
     */
    public static class DetectionBehaviorSerializer implements SerializationSchema<DetectionBehavior> {

        private static final long serialVersionUID = 1L;

        @Override
        public byte[] serialize(DetectionBehavior detection) {
            if (detection == null) {
                return new byte[0];
            }
            return detection.toByteArray();
        }
    }
}
