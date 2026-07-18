package com.traffic.flink.behavior.sink;

import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** Emits durable data-plane application acknowledgements keyed by event_id. */
public final class ModelUpdateAckKafkaSinkFactory {
    private ModelUpdateAckKafkaSinkFactory() {}

    public static KafkaSink<ModelUpdateAppliedAck> createSink(String brokers, String topic) {
        Properties properties = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.RETRIES_CONFIG, String.valueOf(Integer.MAX_VALUE));
        properties.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        return KafkaSink.<ModelUpdateAppliedAck>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AckSerializationSchema(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(properties)
                .build();
    }

    private static class AckSerializationSchema
            implements KafkaRecordSerializationSchema<ModelUpdateAppliedAck> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        private AckSerializationSchema(String topic) {
            this.topic = topic;
        }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(ModelUpdateAppliedAck ack,
                                                        KafkaSinkContext context,
                                                        Long timestamp) {
            byte[] key = ack.eventId.getBytes(StandardCharsets.UTF_8);
            ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                    topic, null, timestamp, key, ack.toJson());
            record.headers().add("event_id", key);
            record.headers().add("tenant_id", ack.tenantId.getBytes(StandardCharsets.UTF_8));
            record.headers().add("content_type", "application/json".getBytes(StandardCharsets.UTF_8));
            return record;
        }
    }
}
