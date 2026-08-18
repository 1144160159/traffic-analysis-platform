package com.traffic.flink.log.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.log.LogJobConfig;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Checkpoint-coupled canonical DLQ sink for device-log rejection records. */
public final class LogDlqSinkFactory {
    private LogDlqSinkFactory() {}

    public static KafkaSink<CanonicalDlqMessage> create(LogJobConfig config) {
        if (!LogJobConfig.DLQ_TOPIC.equals(config.getDlqTopic())) {
            throw new IllegalArgumentException("LogJob failures must use canonical dlq.v1");
        }
        Properties properties = config.getKafkaProducerProperties();
        properties.setProperty("acks", "all");
        properties.setProperty("enable.idempotence", "true");
        properties.setProperty("max.in.flight.requests.per.connection", "5");
        properties.setProperty("transaction.timeout.ms",
                String.valueOf(config.getKafkaTransactionTimeoutMs()));

        return KafkaSink.<CanonicalDlqMessage>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setRecordSerializer(new Serializer(config.getDlqTopic()))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(config.getDlqTransactionalIdPrefix())
                .setKafkaProducerConfig(properties)
                .build();
    }

    static final class Serializer
            implements KafkaRecordSerializationSchema<CanonicalDlqMessage> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        Serializer(String topic) { this.topic = topic; }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                CanonicalDlqMessage element,
                KafkaSinkContext context,
                Long timestamp) {
            if (element == null) return null;
            byte[] key = (element.tenantId() + ":" + element.originalTopic() + ":"
                    + element.originalPartition() + ":" + element.originalOffset())
                    .getBytes(StandardCharsets.UTF_8);
            long recordTimestamp = timestamp == null ? System.currentTimeMillis() : timestamp;
            return new ProducerRecord<>(
                    topic,
                    null,
                    recordTimestamp,
                    key,
                    element.toJson().getBytes(StandardCharsets.UTF_8));
        }
    }
}
