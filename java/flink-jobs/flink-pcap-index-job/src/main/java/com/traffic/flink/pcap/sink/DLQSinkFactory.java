package com.traffic.flink.pcap.sink;

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
 * DLQ (Dead Letter Queue) Sink 工厂（优化版 v2）
 * 
 * 优化内容：
 * 1. ✅ 序列化优化（使用 JSON 格式，便于人工排查）
 * 2. ✅ 错误处理增强（记录序列化失败）
 * 3. ✅ 配置优化（调整 Kafka Producer 参数）
 * 4. ✅ 增加详细注释与日志
 * 
 * 用于写入无效或异常的 PCAP 索引数据
 */
public class DLQSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(DLQSinkFactory.class);

    /**
     * 创建 DLQ Kafka Sink
     *
     * @param brokers Kafka Brokers 地址
     * @param topic   DLQ Topic 名称（建议: dlq.pcap-index-job）
     * @return KafkaSink<String>
     */
    public static KafkaSink<String> createDLQSink(String brokers, String topic) {
        LOG.info("Creating DLQ Kafka sink v2: {} -> {}", brokers, topic);

        // ==================== Kafka Producer 配置 ====================
        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        
        // DLQ 降低一致性要求（优先吞吐）
        producerProps.setProperty("acks", "1"); // Leader 确认即可
        
        // 重试次数（降低，避免阻塞主流程）
        producerProps.setProperty("retries", "2");
        
        // 压缩类型（LZ4 速度快，适合日志）
        producerProps.setProperty("compression.type", "lz4");
        
        // 批量大小（16KB，快速刷新）
        producerProps.setProperty("batch.size", "16384");
        
        // 延迟时间（100ms，避免过多小批次）
        producerProps.setProperty("linger.ms", "100");
        
        // 最大请求大小（1MB，防止单条消息过大）
        producerProps.setProperty("max.request.size", "1048576");
        
        // 超时时间（30s，快速失败）
        producerProps.setProperty("request.timeout.ms", "30000");

        return KafkaSink.<String>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DLQKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * DLQ Kafka 序列化器（内部类，优化版 v2）
     */
    private static class DLQKafkaSerializer implements KafkaRecordSerializationSchema<String> {

        private static final long serialVersionUID = 1L;
        private static final Logger LOG = LoggerFactory.getLogger(DLQKafkaSerializer.class);
        
        private final String topic;

        public DLQKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                String element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            // ==================== 1. 空值检查 ====================
            if (element == null || element.isEmpty()) {
                LOG.warn("DLQ message is null or empty, skipping");
                return null;
            }

            try {
                // ==================== 2. JSON 序列化 ====================
                byte[] valueBytes = element.getBytes(StandardCharsets.UTF_8);
                
                // ==================== 3. 使用事件时间或当前时间 ====================
                Long recordTimestamp = timestamp != null ? timestamp : System.currentTimeMillis();

                // ==================== 4. 构造 ProducerRecord ====================
                // Key 为 null（DLQ 通常不需要按 Key 分区）
                // Timestamp 为事件时间（便于追踪）
                return new ProducerRecord<>(topic, null, recordTimestamp, null, valueBytes);

            } catch (Exception e) {
                LOG.error("Failed to serialize DLQ message: {}, error={}",
                        element.substring(0, Math.min(element.length(), 200)), // 只打印前 200 字符
                        e.getMessage(), e);
                
                // 序列化失败，返回 null（跳过该消息）
                return null;
            }
        }
    }
}
