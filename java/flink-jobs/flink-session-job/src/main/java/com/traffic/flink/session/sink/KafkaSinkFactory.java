package com.traffic.flink.session.sink;

import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.SessionEvent;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.header.Headers;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * Kafka Sink 工厂类（扩展版）
 * 
 * 新增功能：
 * 1. createSessionDlqSink：用于 ClickHouse/OpenSearch 写入失败时的 DLQ
 * 2. 增加 error_code/error_message header
 */
public class KafkaSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(KafkaSinkFactory.class);

    private KafkaSinkFactory() {}

    /**
     * 创建 Session Event Kafka Sink
     */
    public static KafkaSink<SessionEvent> createSink(String brokers, String topic) {
        LOG.info("Creating Kafka sink for SessionEvent: brokers={}, topic={}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("retry.backoff.ms", "1000");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("batch.size", "65536");
        producerProps.setProperty("linger.ms", "10");
        producerProps.setProperty("buffer.memory", "67108864");

        return KafkaSink.<SessionEvent>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new SessionEventSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * 创建 Late Data (FlowEvent) Kafka Sink
     */
    public static KafkaSink<FlowEvent> createLateDataSink(String brokers, String topic) {
        LOG.info("Creating Kafka sink for Late Data: brokers={}, topic={}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("compression.type", "lz4");

        return KafkaSink.<FlowEvent>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new FlowEventSerializer(topic, true))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * 创建 Session DLQ Sink（用于 CH/OS 写入失败）
     */
    public static KafkaSink<SessionEvent> createSessionDlqSink(String brokers, String topic) {
        LOG.info("Creating Kafka sink for Session DLQ: brokers={}, topic={}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("compression.type", "lz4");

        return KafkaSink.<SessionEvent>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new SessionDlqSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * 创建统一 DeadLetter Sink（用于输入解析/字段质量失败）
     */
    public static KafkaSink<DeadLetter> createDeadLetterSink(String brokers, String topic) {
        LOG.info("Creating Kafka sink for DeadLetter: brokers={}, topic={}", brokers, topic);

        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("retries", "3");
        producerProps.setProperty("compression.type", "lz4");

        return KafkaSink.<DeadLetter>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DeadLetterSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    /**
     * SessionEvent 序列化器
     */
    private static class SessionEventSerializer implements KafkaRecordSerializationSchema<SessionEvent> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        public SessionEventSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                SessionEvent element, KafkaSinkContext context, Long timestamp) {
            if (element == null) return null;

            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.toByteArray();

            Headers headers = new RecordHeaders();
            headers.add("run_id", element.getHeader().getRunId().getBytes(StandardCharsets.UTF_8));
            headers.add("feature_set_id", element.getHeader().getFeatureSetId().getBytes(StandardCharsets.UTF_8));
            headers.add("event_id", element.getHeader().getEventId().getBytes(StandardCharsets.UTF_8));
            headers.add("tenant_id", element.getHeader().getTenantId().getBytes(StandardCharsets.UTF_8));
            headers.add("event_ts", String.valueOf(element.getHeader().getEventTs()).getBytes(StandardCharsets.UTF_8));
            headers.add("ingest_ts", String.valueOf(element.getHeader().getIngestTs()).getBytes(StandardCharsets.UTF_8));
            headers.add("kafka_ts", String.valueOf(element.getHeader().getKafkaTs()).getBytes(StandardCharsets.UTF_8));
            headers.add("flink_out_ts", String.valueOf(element.getHeader().getFlinkOutTs()).getBytes(StandardCharsets.UTF_8));

            Long recordTimestamp = element.getHeader().getFlinkOutTs() > 0 ? element.getHeader().getFlinkOutTs() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes, headers);
        }

        private String buildKey(SessionEvent session) {
            String tenantId = session.getHeader() != null ? session.getHeader().getTenantId() : "unknown";
            String communityId = session.getCommunityId();
            if (communityId == null || communityId.isEmpty()) {
                communityId = "unknown";
            }
            return tenantId + ":" + communityId;
        }
    }

    /**
     * FlowEvent 序列化器（Late Data）
     */
    private static class FlowEventSerializer implements KafkaRecordSerializationSchema<FlowEvent> {
        private static final long serialVersionUID = 1L;
        private final String topic;
        private final boolean isLateData;

        public FlowEventSerializer(String topic, boolean isLateData) {
            this.topic = topic;
            this.isLateData = isLateData;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                FlowEvent element, KafkaSinkContext context, Long timestamp) {
            if (element == null) return null;

            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.toByteArray();

            Headers headers = new RecordHeaders();
            headers.add("run_id", element.getHeader().getRunId().getBytes(StandardCharsets.UTF_8));
            headers.add("event_id", element.getHeader().getEventId().getBytes(StandardCharsets.UTF_8));
            headers.add("tenant_id", element.getHeader().getTenantId().getBytes(StandardCharsets.UTF_8));
            headers.add("kafka_ts", String.valueOf(element.getHeader().getKafkaTs()).getBytes(StandardCharsets.UTF_8));
            if (isLateData) {
                headers.add("late_data", "true".getBytes(StandardCharsets.UTF_8));
            }

            Long recordTimestamp = element.getHeader().getKafkaTs() > 0 ? element.getHeader().getKafkaTs() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes, headers);
        }

        private String buildKey(FlowEvent flow) {
            String tenantId = flow.getHeader() != null ? flow.getHeader().getTenantId() : "unknown";
            String communityId = flow.getCommunityId();
            if (communityId == null || communityId.isEmpty()) {
                communityId = "unknown";
            }
            return tenantId + ":" + communityId;
        }
    }

    /**
     * Session DLQ 序列化器
     */
    private static class SessionDlqSerializer implements KafkaRecordSerializationSchema<SessionEvent> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        public SessionDlqSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                SessionEvent element, KafkaSinkContext context, Long timestamp) {
            if (element == null) return null;

            String key = buildKey(element);
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.toByteArray();

            Headers headers = new RecordHeaders();
            headers.add("run_id", element.getHeader().getRunId().getBytes(StandardCharsets.UTF_8));
            headers.add("event_id", element.getHeader().getEventId().getBytes(StandardCharsets.UTF_8));
            headers.add("tenant_id", element.getHeader().getTenantId().getBytes(StandardCharsets.UTF_8));
            headers.add("kafka_ts", String.valueOf(element.getHeader().getKafkaTs()).getBytes(StandardCharsets.UTF_8));
            headers.add("flink_out_ts", String.valueOf(element.getHeader().getFlinkOutTs()).getBytes(StandardCharsets.UTF_8));
            headers.add("dlq_reason", "sink_write_failed".getBytes(StandardCharsets.UTF_8));
            headers.add("dlq_timestamp", String.valueOf(System.currentTimeMillis()).getBytes(StandardCharsets.UTF_8));

            Long recordTimestamp = element.getHeader().getFlinkOutTs() > 0 ? element.getHeader().getFlinkOutTs() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes, headers);
        }

        private String buildKey(SessionEvent session) {
            String tenantId = session.getHeader() != null ? session.getHeader().getTenantId() : "unknown";
            String communityId = session.getCommunityId();
            if (communityId == null || communityId.isEmpty()) {
                communityId = "unknown";
            }
            return tenantId + ":" + communityId;
        }
    }

    /**
     * DeadLetter 序列化器
     */
    private static class DeadLetterSerializer implements KafkaRecordSerializationSchema<DeadLetter> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        public DeadLetterSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                DeadLetter element, KafkaSinkContext context, Long timestamp) {
            if (element == null) return null;

            String key = element.getTenantId() + ":" + element.getSourceTopic() + ":" + element.getSourceKey();
            byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.toByteArray();

            Headers headers = new RecordHeaders();
            headers.add("event_id", element.getEventId().getBytes(StandardCharsets.UTF_8));
            headers.add("tenant_id", element.getTenantId().getBytes(StandardCharsets.UTF_8));
            headers.add("source_topic", element.getSourceTopic().getBytes(StandardCharsets.UTF_8));
            headers.add("source_key", element.getSourceKey().getBytes(StandardCharsets.UTF_8));
            headers.add("error_msg", element.getErrorMsg().getBytes(StandardCharsets.UTF_8));

            Long recordTimestamp = element.getCreatedAt() > 0 ? element.getCreatedAt() : null;

            return new ProducerRecord<>(topic, null, recordTimestamp, keyBytes, valueBytes, headers);
        }
    }
}
