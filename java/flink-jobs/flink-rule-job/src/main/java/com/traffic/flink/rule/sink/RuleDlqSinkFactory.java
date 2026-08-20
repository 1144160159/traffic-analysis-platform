package com.traffic.flink.rule.sink;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ConfigUtil;
import com.traffic.flink.common.sink.DlqSinkFactory;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * Canonical DLQ sink for rule feature and update input failures.
 * 实现 flink-common 的 DlqSinkFactory 产品族契约;静态 create() 仅为
 * 既有调用方保留,新代码使用 DEFAULT 工厂实例。
 */
public final class RuleDlqSinkFactory implements DlqSinkFactory<CanonicalDlqMessage> {

    private static final long serialVersionUID = 1L;

    public static final RuleDlqSinkFactory DEFAULT = new RuleDlqSinkFactory();
    /** 反序列化时保持单例(避免多实例破坏 DEFAULT 契约)。 */
    private Object readResolve() {
        return DEFAULT;
    }


    public RuleDlqSinkFactory() {}

    public static KafkaSink<CanonicalDlqMessage> create(String brokers, String topic) {
        return DEFAULT.createSink(brokers, topic);
    }

    @Override
    public KafkaSink<CanonicalDlqMessage> createSink(String brokers, String topic) {
        if (!"dlq.v1".equals(topic)) {
            throw new IllegalArgumentException("Rule job failures must use canonical dlq.v1");
        }
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty("acks", "all");
        properties.setProperty("retries", "3");
        properties.setProperty("enable.idempotence", "true");
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
