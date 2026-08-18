package com.traffic.flink.log.sink;

import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.log.LogJobConfig;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Publishes source-quality receipts in the checkpoint that advances source offsets. */
public final class LogSourceQualitySinkFactory {
    private LogSourceQualitySinkFactory() {}

    public static KafkaSink<SourceQualityReceipt> create(LogJobConfig config) {
        if (!LogJobConfig.AUDIT_TOPIC.equals(config.getAuditTopic())) {
            throw new IllegalArgumentException("quality receipts must use canonical audit.logs");
        }
        Properties properties = config.getKafkaProducerProperties();
        properties.setProperty("acks", "all");
        properties.setProperty("enable.idempotence", "true");
        properties.setProperty("max.in.flight.requests.per.connection", "5");
        properties.setProperty("transaction.timeout.ms",
                String.valueOf(config.getKafkaTransactionTimeoutMs()));
        return KafkaSink.<SourceQualityReceipt>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setRecordSerializer(new Serializer(config.getAuditTopic()))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(config.getQualityTransactionalIdPrefix())
                .setKafkaProducerConfig(properties)
                .build();
    }

    static final class Serializer
            implements KafkaRecordSerializationSchema<SourceQualityReceipt> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        Serializer(String topic) { this.topic = topic; }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                SourceQualityReceipt receipt,
                KafkaSinkContext context,
                Long ignoredTimestamp) {
            if (receipt == null) return null;
            byte[] key = receipt.getTenantId().getBytes(StandardCharsets.UTF_8);
            return new ProducerRecord<>(
                    topic, null, null, key, receipt.toAuditEventJson());
        }
    }
}
