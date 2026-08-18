package com.traffic.flink.behavior.sink;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class ChampionChallengerObservationKafkaSinkFactoryTest {
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Test
    void serializesDeterministicKeySnakeCaseContractAndChampionInvariant() throws Exception {
        ChampionChallengerObservation value = validObservation();
        ChampionChallengerObservationKafkaSinkFactory.ObservationSerializer serializer =
                new ChampionChallengerObservationKafkaSinkFactory.ObservationSerializer(
                        "model-shadow-observations.v1");

        ProducerRecord<byte[], byte[]> record = serializer.serialize(value, null, null);
        JsonNode payload = MAPPER.readTree(record.value());

        assertThat(record.topic()).isEqualTo("model-shadow-observations.v1");
        assertThat(new String(record.key(), StandardCharsets.UTF_8))
                .isEqualTo(value.getObservationId());
        assertThat(payload.path("observation_id").asText()).isEqualTo(value.getObservationId());
        assertThat(payload.path("serving_result_source").asText()).isEqualTo("champion");
        assertThat(payload.path("challenger_package_sha256").asText()).isEqualTo("b".repeat(64));
        assertThat(payload.has("observationId")).isFalse();
        assertThat(new String(record.headers().lastHeader("serving_result_source").value(),
                StandardCharsets.UTF_8)).isEqualTo("champion");
    }

    @Test
    void refusesUnboundPackageIdentity() {
        ChampionChallengerObservation invalid = ChampionChallengerObservation.builder()
                .observationId("a".repeat(64))
                .tenantId("tenant-1")
                .sourceEventId("event-1")
                .challengerPackageId("package-1")
                .challengerPackageSha256("not-a-digest")
                .status("error")
                .build();
        ChampionChallengerObservationKafkaSinkFactory.ObservationSerializer serializer =
                new ChampionChallengerObservationKafkaSinkFactory.ObservationSerializer("topic.v1");

        assertThatThrownBy(() -> serializer.serialize(invalid, null, null))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("champion-only serving invariant");
    }

    private static ChampionChallengerObservation validObservation() {
        return ChampionChallengerObservation.builder()
                .observationId("a".repeat(64))
                .tenantId("tenant-1")
                .sourceEventId("event-1")
                .objectId("flow-1")
                .communityId("1:community")
                .eventTimeMs(1_000L)
                .observedAtMs(2_000L)
                .sampleBucket(10)
                .championModelId("champion")
                .championVersion("v1")
                .championLabel("benign")
                .championScore(0.2f)
                .championDetected(false)
                .championLatencyNanos(10L)
                .challengerModelId("candidate")
                .challengerVersion("v2")
                .challengerPackageId("package-1")
                .challengerPackageSha256("b".repeat(64))
                .challengerAggregateRevision(2L)
                .challengerLabel("malicious")
                .challengerScore(0.8f)
                .challengerDetected(true)
                .challengerLatencyNanos(20L)
                .challengerCpuNanos(15L)
                .challengerHeapDeltaBytes(0L)
                .absoluteScoreDelta(0.6f)
                .decisionChanged(true)
                .labelChanged(true)
                .status("compared")
                .build();
    }
}
