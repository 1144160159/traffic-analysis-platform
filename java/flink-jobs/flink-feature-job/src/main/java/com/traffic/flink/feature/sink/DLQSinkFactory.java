package com.traffic.flink.feature.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
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
     * @param topic   canonical DLQ Topic (must be dlq.v1)
     * @return KafkaSink
     */
    public static KafkaSink<CanonicalDlqMessage> createDLQSink(String brokers, String topic) {
        if (!"dlq.v1".equals(topic)) {
            throw new IllegalArgumentException("Feature job failures must use canonical dlq.v1");
        }
        LOG.info("Creating DLQ Kafka sink: {} -> {}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("enable.idempotence", "true");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("batch.size", "16384");
        producerProps.setProperty("linger.ms", "100");

        return KafkaSink.<CanonicalDlqMessage>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DLQKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * DLQ Kafka 序列化器
     */
    static class DLQKafkaSerializer implements KafkaRecordSerializationSchema<CanonicalDlqMessage> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public DLQKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                CanonicalDlqMessage element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            byte[] keyBytes = (element.tenantId() + ":" + element.originalTopic()
                    + ":" + element.originalPartition() + ":" + element.originalOffset())
                    .getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.toJson().getBytes(StandardCharsets.UTF_8);
            Long recordTimestamp = timestamp != null ? timestamp : System.currentTimeMillis();

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes);
        }
    }
}
