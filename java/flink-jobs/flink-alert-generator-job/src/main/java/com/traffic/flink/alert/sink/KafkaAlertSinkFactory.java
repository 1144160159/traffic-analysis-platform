package com.traffic.flink.alert.sink;

import com.traffic.proto.traffic.v1.Alert;

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
 * Kafka Alert Sink 工厂 (修复版)
 * 
 * 修复内容：
 * - 添加幂等 Producer 配置 (enable.idempotence=true)
 * - 修复 Key 策略：使用 tenant_id + community_id（与设计文档一致）
 * - 优化 Producer 参数配置
 */
public class KafkaAlertSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(KafkaAlertSinkFactory.class);

    /**
     * 创建 Alert Kafka Sink
     * 
     * @param brokers Kafka broker 地址
     * @param topic 目标 Topic
     * @return KafkaSink 实例
     */
    public static KafkaSink<Alert> createAlertSink(String brokers, String topic) {
        LOG.info("Creating Kafka alert sink: {} -> {}", brokers, topic);

        Properties producerProps = buildProducerProperties();

        return KafkaSink.<Alert>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AlertKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * 构建 Producer 配置
     * 
     * 关键配置：
     * - 幂等 Producer：避免重复写入
     * - 压缩：LZ4 高效压缩
     * - 批量优化：减少网络开销
     */
    private static Properties buildProducerProperties() {
        Properties props = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();

        // ==================== 可靠性配置 ====================
        
        // 幂等 Producer（关键！避免重复写入）
        props.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        
        // 等待所有副本确认
        props.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        
        // 重试次数（幂等 Producer 要求 >= 1）
        props.setProperty(ProducerConfig.RETRIES_CONFIG, "3");
        
        // 重试间隔
        props.setProperty(ProducerConfig.RETRY_BACKOFF_MS_CONFIG, "1000");
        
        // 请求超时
        props.setProperty(ProducerConfig.REQUEST_TIMEOUT_MS_CONFIG, "30000");
        
        // 传输超时
        props.setProperty(ProducerConfig.DELIVERY_TIMEOUT_MS_CONFIG, "120000");

        // ==================== 性能配置 ====================
        
        // 压缩类型（LZ4 高效）
        props.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        
        // 批量大小（64KB）
        props.setProperty(ProducerConfig.BATCH_SIZE_CONFIG, "65536");
        
        // 发送延迟（10ms 等待更多消息）
        props.setProperty(ProducerConfig.LINGER_MS_CONFIG, "10");
        
        // 缓冲区大小（64MB）
        props.setProperty(ProducerConfig.BUFFER_MEMORY_CONFIG, "67108864");
        
        // 最大请求大小（1MB）
        props.setProperty(ProducerConfig.MAX_REQUEST_SIZE_CONFIG, "1048576");
        
        // 最大阻塞时间
        props.setProperty(ProducerConfig.MAX_BLOCK_MS_CONFIG, "60000");

        // ==================== 幂等 Producer 要求 ====================
        
        // 最大未确认请求数（幂等 Producer 要求 <= 5）
        props.setProperty(ProducerConfig.MAX_IN_FLIGHT_REQUESTS_PER_CONNECTION, "5");

        return props;
    }

    /**
     * Alert Kafka 序列化器
     * 
     * Key 策略：tenant_id:community_id
     * - 确保同一会话的告警在同一分区
     * - 便于下游按 Key 聚合处理
     */
    private static class AlertKafkaSerializer implements KafkaRecordSerializationSchema<Alert> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public AlertKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                Alert element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            // Key: tenant_id:community_id（按设计文档）
            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);

            // Value: Protobuf 二进制
            byte[] valueBytes = element.toByteArray();

            // 使用告警最后更新时间作为 Kafka 记录时间戳
            Long recordTimestamp = element.getLastSeen() > 0 ? element.getLastSeen() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes);
        }

        /**
         * 构建 Kafka Key
         * 
         * 格式: tenant_id:community_id
         * 
         * 使用 community_id 而非 alert_id 的原因：
         * - 同一会话的多个告警应在同一分区
         * - 便于下游 Consumer 按会话聚合
         * - 与 Kafka Topic 分区策略一致
         */
        private String buildKey(Alert alert) {
            String tenantId = alert.getTenantId();
            String communityId = alert.getCommunityId();

            // 空值保护
            if (tenantId == null || tenantId.isEmpty()) {
                tenantId = "unknown";
            }
            if (communityId == null || communityId.isEmpty()) {
                communityId = alert.getAlertId(); // 降级使用 alertId
            }

            return tenantId + ":" + communityId;
        }
    }
}
