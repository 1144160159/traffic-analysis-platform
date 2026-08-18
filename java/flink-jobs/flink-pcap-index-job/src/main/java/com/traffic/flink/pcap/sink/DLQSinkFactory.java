package com.traffic.flink.pcap.sink;

import com.traffic.flink.pcap.source.PcapDeadLetter;

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
    private static final String CANONICAL_DLQ_TOPIC = "dlq.v1";
    private static final int MAX_DLQ_MESSAGE_BYTES = 1024 * 1024;

    /**
     * Creates the consumer-first canonical PCAP DLQ sink. This overload is
     * intentionally separate from the legacy String sink used by the old main
     * graph so N009 can compile and test without prematurely cutting N010/N011.
     */
    public static KafkaSink<PcapDeadLetter> createDLQSink(
            String brokers, String topic, Properties trustedProperties) {
        if (brokers == null || brokers.trim().isEmpty()) {
            throw new IllegalArgumentException("Kafka brokers are required for PCAP DLQ");
        }
        if (!CANONICAL_DLQ_TOPIC.equals(topic)) {
            throw new IllegalArgumentException("PCAP consumer DLQ must be canonical dlq.v1");
        }
        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        if (trustedProperties != null) producerProps.putAll(trustedProperties);
        String requestedAcks = producerProps.getProperty("acks", "all").trim();
        if (!"all".equalsIgnoreCase(requestedAcks) && !"-1".equals(requestedAcks)) {
            throw new IllegalArgumentException("PCAP DLQ acknowledgements cannot be weaker than all");
        }
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("enable.idempotence", "true");
        producerProps.setProperty("retries", String.valueOf(Integer.MAX_VALUE));
        producerProps.setProperty("max.in.flight.requests.per.connection", "5");
        producerProps.setProperty("delivery.timeout.ms", "120000");
        producerProps.setProperty("request.timeout.ms", "30000");
        producerProps.setProperty("max.request.size", String.valueOf(MAX_DLQ_MESSAGE_BYTES));
        return KafkaSink.<PcapDeadLetter>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new PcapDeadLetterSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

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
        producerProps.setProperty("acks", "all"); // DLQ 是证据通道，Leader 确认即可不足以保证不丢
        producerProps.setProperty("enable.idempotence", "true"); // 幂等，配合 acks=all
        producerProps.setProperty("retries", "10");
        producerProps.setProperty("max.in.flight.requests.per.connection", "5");
        
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

            // ==================== 2. JSON 序列化 ====================
            byte[] valueBytes = element.getBytes(StandardCharsets.UTF_8);
            
            // ==================== 3. 使用事件时间或当前时间 ====================
            Long recordTimestamp = timestamp != null ? timestamp : System.currentTimeMillis();

            // ==================== 4. 构造 ProducerRecord ====================
            // Key 为 null（DLQ 通常不需要按 Key 分区）
            // Timestamp 为事件时间（便于追踪）
            return new ProducerRecord<>(topic, null, recordTimestamp, null, valueBytes);
        }
    }

    static final class PcapDeadLetterSerializer
            implements KafkaRecordSerializationSchema<PcapDeadLetter> {
        private static final long serialVersionUID = 1L;
        private final String topic;
        PcapDeadLetterSerializer(String topic) {
            if (!CANONICAL_DLQ_TOPIC.equals(topic)) throw new IllegalArgumentException("invalid canonical DLQ topic");
            this.topic = topic;
        }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                PcapDeadLetter element, KafkaSinkContext context, Long timestamp) {
            if (element == null) throw new IllegalArgumentException("PCAP dead letter cannot be null");
            try {
                byte[] value = element.toCanonicalJson();
                if (value.length == 0 || value.length > MAX_DLQ_MESSAGE_BYTES) {
                    throw new IllegalArgumentException("PCAP dead letter exceeds canonical size bounds");
                }
                byte[] key = element.projectionIdentity().getBytes(StandardCharsets.UTF_8);
                org.apache.kafka.common.header.internals.RecordHeaders headers =
                        new org.apache.kafka.common.header.internals.RecordHeaders();
                headers.add("source_topic", element.getSource().getTopic().getBytes(StandardCharsets.UTF_8));
                headers.add("source_partition", String.valueOf(element.getSource().getPartition()).getBytes(StandardCharsets.UTF_8));
                headers.add("source_offset", String.valueOf(element.getSource().getOffset()).getBytes(StandardCharsets.UTF_8));
                headers.add("error_code", element.getErrorCode().getBytes(StandardCharsets.UTF_8));
                headers.add("raw_sha256", element.getSource().getRawSha256().getBytes(StandardCharsets.UTF_8));
                Long recordTimestamp = element.getSource().getTimestamp() > 0 ? element.getSource().getTimestamp() : timestamp;
                return new ProducerRecord<>(topic, null, recordTimestamp, key, value, headers);
            } catch (RuntimeException error) {
                throw error;
            } catch (Exception error) {
                throw new IllegalStateException("serialize canonical PCAP dead letter", error);
            }
        }
    }
}
