package com.traffic.flink.alert.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ConfigUtil;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Kafka sink for the platform-wide canonical dlq.v1 contract. */
public final class AlertDlqSinkFactory {

    private AlertDlqSinkFactory() {}

    public static KafkaSink<CanonicalDlqMessage> create(String brokers, String topic) {
        if (!"dlq.v1".equals(topic)) {
            throw new IllegalArgumentException(
                    "Alert generator failures must use canonical dlq.v1");
        }
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty("acks", "all");
        properties.setProperty("retries", "3");
        properties.setProperty("enable.idempotence", "true");
        properties.setProperty("compression.type", "lz4");

        return KafkaSink.<CanonicalDlqMessage>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new Serializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(properties)
                .build();
    }

    static final class Serializer
            implements KafkaRecordSerializationSchema<CanonicalDlqMessage> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        Serializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                CanonicalDlqMessage element, KafkaSinkContext context, Long timestamp) {
            if (element == null) return null;
            byte[] key = (element.tenantId() + ":" + element.originalTopic() + ":"
                    + element.originalPartition() + ":" + element.originalOffset())
                    .getBytes(StandardCharsets.UTF_8);
            long recordTimestamp = timestamp != null ? timestamp : System.currentTimeMillis();
            return new ProducerRecord<>(topic, null, recordTimestamp, key,
                    element.toJson().getBytes(StandardCharsets.UTF_8));
        }
    }
}
