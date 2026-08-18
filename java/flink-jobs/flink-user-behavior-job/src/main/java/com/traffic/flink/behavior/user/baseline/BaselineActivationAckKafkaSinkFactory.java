package com.traffic.flink.behavior.user.baseline;

import com.traffic.flink.common.ConfigUtils;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Checkpoint-coupled producer for baseline.activation-acks.v1. */
public final class BaselineActivationAckKafkaSinkFactory {
    private BaselineActivationAckKafkaSinkFactory() {}

    public static KafkaSink<BaselineActivationAck> create(
            String brokers,
            String topic,
            String transactionalPrefix,
            long transactionTimeoutMs,
            ParameterTool params) {
        if (!"baseline.activation-acks.v1".equals(topic)
                || transactionalPrefix == null || transactionalPrefix.isBlank()) {
            throw new IllegalArgumentException("exact behavior baseline ACK topic and transaction prefix are required");
        }
        Properties properties = ConfigUtils.kafkaClientProperties(params);
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.RETRIES_CONFIG, String.valueOf(Integer.MAX_VALUE));
        properties.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        properties.setProperty(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG, Long.toString(transactionTimeoutMs));
        return KafkaSink.<BaselineActivationAck>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AckSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(transactionalPrefix)
                .setKafkaProducerConfig(properties)
                .build();
    }

    static final class AckSerializer implements KafkaRecordSerializationSchema<BaselineActivationAck> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        AckSerializer(String topic) { this.topic = topic; }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                BaselineActivationAck ack, KafkaSinkContext context, Long timestamp) {
            byte[] key = ack.partitionKey.getBytes(StandardCharsets.UTF_8);
            ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                    topic, null, timestamp, key, ack.toJson());
            header(record, "event_id", ack.eventId);
            header(record, "event_type", ack.eventType);
            header(record, "schema_version", Integer.toString(ack.schemaVersion));
            header(record, "tenant_id", ack.tenantId);
            header(record, "baseline_id", ack.baselineId);
            header(record, "baseline_version", Long.toString(ack.baselineVersion));
            header(record, "consumer_id", ack.consumerId);
            header(record, "candidate_sha256", ack.candidateSha256);
            header(record, "snapshot_sha256", ack.snapshotSha256);
            header(record, "trace_id", ack.traceId);
            header(record, "target_topic", topic);
            return record;
        }

        private static void header(ProducerRecord<byte[], byte[]> record, String name, String value) {
            record.headers().add(name, value.getBytes(StandardCharsets.UTF_8));
        }
    }
}
