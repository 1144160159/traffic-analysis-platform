package com.traffic.flink.feature.sink;

import com.traffic.proto.traffic.v1.FeatureStat;
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
 * Kafka Sink 工厂（增强版 v2）
 * 
 * 增强内容（P1）：
 * 1. ✅ 幂等生产者配置
 * 2. ✅ 事务 ID 支持（可选）
 * 3. ✅ Kafka Header 增强（tenant_id/run_id/schema_version）
 * 4. ✅ 分区策略优化
 * 
 * 注意：Flink 1.17+ KafkaSink 不直接暴露成功/失败回调
 * 若需精确监控，建议使用自定义 SinkWriter 或旧版 FlinkKafkaProducer
 */
public class KafkaSinkFactory {

    private static final Logger LOG = LoggerFactory.getLogger(KafkaSinkFactory.class);

    /**
     * 创建 FeatureStat Kafka Sink
     * 
     * @param brokers Kafka Brokers
     * @param topic   输出 Topic
     * @return KafkaSink
     */
    public static KafkaSink<FeatureStat> createFeatureSink(String brokers, String topic) {
        return createFeatureSink(brokers, topic, false, null);
    }

    /**
     * 创建 SessionEvent Kafka Sink，用于 L2 触发侧输出。
     *
     * @param brokers Kafka Brokers
     * @param topic   输出 Topic
     * @return KafkaSink
     */
    public static KafkaSink<SessionEvent> createSessionEventSink(String brokers, String topic) {
        LOG.info("Creating SessionEvent Kafka sink: {} -> {}", brokers, topic);

        return KafkaSink.<SessionEvent>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new SessionEventKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(buildProducerProps(false, null))
                .build();
    }

    /**
     * 创建 FeatureStat Kafka Sink（完整配置）
     * 
     * @param brokers        Kafka Brokers
     * @param topic          输出 Topic
     * @param enableTxn      是否启用事务（Exactly-Once）
     * @param transactionalId 事务 ID 前缀（启用事务时必须）
     * @return KafkaSink
     */
    public static KafkaSink<FeatureStat> createFeatureSink(
            String brokers, 
            String topic, 
            boolean enableTxn,
            String transactionalId
    ) {
        LOG.info("Creating Kafka sink v2: {} -> {} (txn={})", brokers, topic, enableTxn);

        Properties producerProps = buildProducerProps(enableTxn, transactionalId);

        // 选择投递保证级别
        DeliveryGuarantee deliveryGuarantee = enableTxn 
                ? DeliveryGuarantee.EXACTLY_ONCE 
                : DeliveryGuarantee.AT_LEAST_ONCE;

        return KafkaSink.<FeatureStat>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new FeatureKafkaSerializer(topic))
                .setDeliveryGuarantee(deliveryGuarantee)
                .setKafkaProducerConfig(producerProps)
                // ✅ 事务前缀（仅在 EXACTLY_ONCE 时生效）
                .setTransactionalIdPrefix(transactionalId != null ? transactionalId : "feature-job")
                .build();
    }

    /**
     * 构建 Kafka Producer 配置
     */
    private static Properties buildProducerProps(boolean enableTxn, String transactionalId) {
        Properties props = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();

        // ==================== 基础配置 ====================
        props.setProperty("acks", "all");
        props.setProperty("retries", "3");
        props.setProperty("retry.backoff.ms", "1000");
        props.setProperty("max.in.flight.requests.per.connection", "5");

        // ==================== 性能优化 ====================
        props.setProperty("compression.type", "lz4");
        props.setProperty("batch.size", "65536");          // 64KB
        props.setProperty("linger.ms", "10");              // 10ms 批量延迟
        props.setProperty("buffer.memory", "67108864");    // 64MB

        // ==================== 幂等性 ====================
        props.setProperty("enable.idempotence", "true");

        // ==================== 事务配置（可选）====================
        if (enableTxn) {
            if (transactionalId == null || transactionalId.isEmpty()) {
                throw new IllegalArgumentException("Transactional ID must be set when transaction is enabled");
            }
            props.setProperty("transactional.id", transactionalId);
            props.setProperty("transaction.timeout.ms", "60000"); // 1 分钟
            LOG.info("Kafka producer transaction enabled: transactional.id={}", transactionalId);
        }

        return props;
    }

    /**
     * FeatureStat Kafka 序列化器
     */
    private static class FeatureKafkaSerializer implements KafkaRecordSerializationSchema<FeatureStat> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public FeatureKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                FeatureStat element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            try {
                // ==================== Key: tenant_id:community_id ====================
                String key = buildKey(element);
                byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);

                // ==================== Value: Protobuf 二进制 ====================
                byte[] valueBytes = element.toByteArray();

                // ==================== Timestamp: 使用事件时间 ====================
                Long recordTimestamp = element.getTs() > 0 ? element.getTs() : System.currentTimeMillis();

                // ==================== Headers: 增强元数据 ====================
                Headers headers = buildHeaders(element);

                // ==================== Partition: 自动（由 Key 决定）====================
                return new ProducerRecord<>(
                        topic,
                        null,           // partition (由 Kafka 根据 key hash 决定)
                        recordTimestamp,
                        keyBytes,
                        valueBytes,
                        headers
                );

            } catch (Exception e) {
                LOG.error("Failed to serialize FeatureStat {}: {}", 
                        element.getObjectId(), e.getMessage(), e);
                return null;
            }
        }

        /**
         * 构建 Kafka Record Key
         * 格式：tenant_id:community_id
         */
        private String buildKey(FeatureStat feature) {
            String tenantId = feature.getHeader().getTenantId();
            String communityId = feature.getCommunityId();

            if (tenantId == null || tenantId.isEmpty()) {
                tenantId = "unknown";
            }
            if (communityId == null || communityId.isEmpty()) {
                communityId = "unknown";
            }

            return tenantId + ":" + communityId;
        }

        /**
         * 构建 Kafka Headers（✅ 增强）
         */
        private Headers buildHeaders(FeatureStat feature) {
            Headers headers = new RecordHeaders();

            // 租户 ID
            addHeader(headers, "tenant_id", feature.getHeader().getTenantId());

            // Run ID
            addHeader(headers, "run_id", feature.getHeader().getRunId());

            // Event ID（幂等键）
            addHeader(headers, "event_id", feature.getHeader().getEventId());

            // Feature Set ID
            addHeader(headers, "feature_set_id", feature.getHeader().getFeatureSetId());

            // Schema Version（✅ 新增）
            addHeader(headers, "schema_version", feature.getSchemaVersion());

            // Object Type
            addHeader(headers, "object_type", feature.getObjectType());

            // Community ID（✅ 新增）
            addHeader(headers, "community_id", feature.getCommunityId());

            // Probe ID（✅ 新增）
            addHeader(headers, "probe_id", feature.getHeader().getProbeId());

            return headers;
        }

        /**
         * 添加 Header（安全处理 null）
         */
        private void addHeader(Headers headers, String key, String value) {
            if (value != null && !value.isEmpty()) {
                headers.add(key, value.getBytes(StandardCharsets.UTF_8));
            }
        }
    }

    /**
     * SessionEvent Kafka 序列化器。
     */
    private static class SessionEventKafkaSerializer implements KafkaRecordSerializationSchema<SessionEvent> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        private SessionEventKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                SessionEvent element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            try {
                String key = buildKey(element);
                byte[] keyBytes = key.getBytes(StandardCharsets.UTF_8);
                byte[] valueBytes = element.toByteArray();
                Long recordTimestamp = element.getTsEnd() > 0 ? element.getTsEnd() : System.currentTimeMillis();
                Headers headers = buildHeaders(element);

                return new ProducerRecord<>(
                        topic,
                        null,
                        recordTimestamp,
                        keyBytes,
                        valueBytes,
                        headers
                );
            } catch (Exception e) {
                LOG.error("Failed to serialize SessionEvent {}: {}",
                        element.getSessionId(), e.getMessage(), e);
                return null;
            }
        }

        private String buildKey(SessionEvent session) {
            String tenantId = session.getHeader().getTenantId();
            String communityId = session.getCommunityId();

            if (tenantId == null || tenantId.isEmpty()) {
                tenantId = "unknown";
            }
            if (communityId == null || communityId.isEmpty()) {
                communityId = session.getSessionId();
            }
            if (communityId == null || communityId.isEmpty()) {
                communityId = "unknown";
            }

            return tenantId + ":" + communityId;
        }

        private Headers buildHeaders(SessionEvent session) {
            Headers headers = new RecordHeaders();

            addHeader(headers, "tenant_id", session.getHeader().getTenantId());
            addHeader(headers, "run_id", session.getHeader().getRunId());
            addHeader(headers, "event_id", session.getHeader().getEventId());
            addHeader(headers, "feature_set_id", session.getHeader().getFeatureSetId());
            addHeader(headers, "community_id", session.getCommunityId());
            addHeader(headers, "session_id", session.getSessionId());
            addHeader(headers, "probe_id", session.getHeader().getProbeId());

            return headers;
        }
    }

    private static void addHeader(Headers headers, String key, String value) {
        if (value != null && !value.isEmpty()) {
            headers.add(key, value.getBytes(StandardCharsets.UTF_8));
        }
    }
}
