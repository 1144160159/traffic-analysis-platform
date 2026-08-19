package com.traffic.flink.rule.sink;

import com.traffic.flink.common.sink.AckSinkFactory;
import com.traffic.flink.rule.model.RuleUpdateAppliedAck;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * Produces durable per-subtask rule application receipts keyed by command event_id.
 * 实现 flink-common 的 AckSinkFactory 产品族契约;静态 create() 仅为
 * 既有调用方保留,新代码使用 DEFAULT 工厂实例。
 */
public final class RuleUpdateAckKafkaSinkFactory implements AckSinkFactory<RuleUpdateAppliedAck> {

    private static final long serialVersionUID = 1L;

    public static final RuleUpdateAckKafkaSinkFactory DEFAULT = new RuleUpdateAckKafkaSinkFactory();

    public RuleUpdateAckKafkaSinkFactory() {
    }

    public static KafkaSink<RuleUpdateAppliedAck> create(String brokers, String topic) {
        return DEFAULT.createSink(brokers, topic);
    }

    @Override
    public KafkaSink<RuleUpdateAppliedAck> createSink(String brokers, String topic) {
        if (!"rule-update-applied.v1".equals(topic)) {
            throw new IllegalArgumentException(
                    "rule update acknowledgements require rule-update-applied.v1");
        }
        Properties properties = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.RETRIES_CONFIG, String.valueOf(Integer.MAX_VALUE));
        properties.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        return KafkaSink.<RuleUpdateAppliedAck>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AckSerializationSchema(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(properties)
                .build();
    }

    static final class AckSerializationSchema
            implements KafkaRecordSerializationSchema<RuleUpdateAppliedAck> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        AckSerializationSchema(String topic) {
            this.topic = topic;
        }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                RuleUpdateAppliedAck ack, KafkaSinkContext context, Long timestamp) {
            if (ack == null || ack.eventId == null || ack.eventId.isBlank()
                    || ack.tenantId == null || ack.tenantId.isBlank()) {
                throw new IllegalArgumentException(
                        "rule update acknowledgement requires event and tenant identity");
            }
            byte[] key = ack.eventId.getBytes(StandardCharsets.UTF_8);
            ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                    topic, null, timestamp, key, ack.toJson());
            record.headers().add("event_id", key);
            record.headers().add("tenant_id", ack.tenantId.getBytes(StandardCharsets.UTF_8));
            record.headers().add("content_type", "application/json".getBytes(StandardCharsets.UTF_8));
            record.headers().add("schema_version", "1".getBytes(StandardCharsets.UTF_8));
            return record;
        }
    }
}
