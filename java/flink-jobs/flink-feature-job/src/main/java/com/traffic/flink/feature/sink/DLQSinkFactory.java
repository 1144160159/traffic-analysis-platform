package com.traffic.flink.feature.sink;

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
 * DLQ (Dead Letter Queue) Sink 工厂
 */
public class DLQSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(DLQSinkFactory.class);

    /**
     * 创建 DLQ Kafka Sink
     *
     * @param brokers Kafka Brokers
     * @param topic   DLQ Topic（建议: dlq.feature-job）
     * @return KafkaSink
     */
    public static KafkaSink<String> createDLQSink(String brokers, String topic) {
        LOG.info("Creating DLQ Kafka sink: {} -> {}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "1"); // DLQ 降低一致性要求
        producerProps.setProperty("retries", "2");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("batch.size", "16384");
        producerProps.setProperty("linger.ms", "100");

        return KafkaSink.<String>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DLQKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * DLQ Kafka 序列化器
     */
    private static class DLQKafkaSerializer implements KafkaRecordSerializationSchema<String> {

        private static final long serialVersionUID = 1L;
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
            if (element == null || element.isEmpty()) {
                return null;
            }

            byte[] valueBytes = element.getBytes(StandardCharsets.UTF_8);
            Long recordTimestamp = timestamp != null ? timestamp : System.currentTimeMillis();

            return new ProducerRecord<>(topic, null, recordTimestamp, null, valueBytes);
        }
    }
}
