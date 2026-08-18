package com.traffic.flink.behavior.sink;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.PropertyNamingStrategies;
import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

/** At-least-once JSON observation sink keyed by deterministic observation_id. */
public final class ChampionChallengerObservationKafkaSinkFactory {
    private ChampionChallengerObservationKafkaSinkFactory() {}

    public static KafkaSink<ChampionChallengerObservation> create(String brokers, String topic) {
        if (brokers == null || brokers.isBlank()) {
            throw new IllegalArgumentException("Kafka brokers are required");
        }
        if (topic == null || !topic.matches("^[a-zA-Z0-9._-]+$")) {
            throw new IllegalArgumentException("shadow observation topic is invalid");
        }
        Properties properties = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.RETRIES_CONFIG, String.valueOf(Integer.MAX_VALUE));
        properties.setProperty(ProducerConfig.MAX_IN_FLIGHT_REQUESTS_PER_CONNECTION, "5");
        properties.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        return KafkaSink.<ChampionChallengerObservation>builder()
                .setBootstrapServers(brokers)
                .setKafkaProducerConfig(properties)
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setRecordSerializer(new ObservationSerializer(topic))
                .build();
    }

    static final class ObservationSerializer
            implements KafkaRecordSerializationSchema<ChampionChallengerObservation> {
        private static final long serialVersionUID = 1L;
        private static final ObjectMapper MAPPER = new ObjectMapper()
                .setPropertyNamingStrategy(PropertyNamingStrategies.SNAKE_CASE);
        private final String topic;

        ObservationSerializer(String topic) { this.topic = topic; }

        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                ChampionChallengerObservation observation,
                KafkaSinkContext context,
                Long timestamp) {
            validate(observation);
            try {
                ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                        topic,
                        null,
                        observation.getObservedAtMs(),
                        observation.getObservationId().getBytes(StandardCharsets.UTF_8),
                        MAPPER.writeValueAsBytes(observation));
                record.headers().add("observation_id",
                        observation.getObservationId().getBytes(StandardCharsets.UTF_8));
                record.headers().add("source_event_id",
                        observation.getSourceEventId().getBytes(StandardCharsets.UTF_8));
                record.headers().add("challenger_package_id",
                        observation.getChallengerPackageId().getBytes(StandardCharsets.UTF_8));
                record.headers().add("comparison_status",
                        observation.getStatus().getBytes(StandardCharsets.UTF_8));
                record.headers().add("serving_result_source", "champion".getBytes(StandardCharsets.UTF_8));
                return record;
            } catch (Exception error) {
                throw new IllegalStateException("cannot serialize shadow observation", error);
            }
        }

        private static void validate(ChampionChallengerObservation value) {
            if (value == null
                    || value.getSchemaVersion() != 1
                    || value.getObservationId() == null
                    || !value.getObservationId().matches("^[0-9a-f]{64}$")
                    || value.getTenantId() == null || value.getTenantId().isBlank()
                    || value.getSourceEventId() == null || value.getSourceEventId().isBlank()
                    || value.getChallengerPackageId() == null
                    || value.getChallengerPackageId().isBlank()
                    || value.getChallengerPackageSha256() == null
                    || !value.getChallengerPackageSha256().matches("^[0-9a-f]{64}$")
                    || !"champion".equals(value.getServingResultSource())) {
                throw new IllegalArgumentException(
                        "shadow observation identity or champion-only serving invariant is invalid");
            }
        }
    }
}
