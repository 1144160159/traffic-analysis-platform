package com.traffic.flink.behavior;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.detector.ModelUpdateBroadcastHandler;
import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ShadowEvaluationRequest;
import com.traffic.flink.behavior.sink.ChampionChallengerObservationKafkaSinkFactory;
import com.traffic.flink.common.ConfigUtil;
import com.traffic.flink.common.ProtoTypeInformation;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.api.common.JobStatus;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.serialization.DeserializationSchema;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.core.execution.JobClient;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.NewTopic;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.errors.TopicExistsException;
import org.apache.kafka.common.serialization.ByteArrayDeserializer;
import org.apache.kafka.common.serialization.ByteArraySerializer;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.TimeUnit;

/**
 * Isolated K8s verifier for the N012 Kafka -> Flink -> Kafka comparison rail.
 * The graph uses the same production observer function and deliberately has no
 * detection, ClickHouse, DLQ, readiness or model-activation ACK sink.
 */
public final class ChampionChallengerShadowCanaryMain {
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private ChampionChallengerShadowCanaryMain() {}

    public static void main(String[] args) throws Exception {
        Map<String, String> options = parseArgs(args);
        String brokers = required(options, "brokers");
        String featureTopic = required(options, "feature-topic");
        String updateTopic = required(options, "update-topic");
        String observationTopic = required(options, "observation-topic");
        String ackTopic = required(options, "ack-topic");
        String detectionTopic = required(options, "detection-topic");
        Path eventPath = Path.of(required(options, "event")).toAbsolutePath().normalize();
        Path publicKey = Path.of(required(options, "public-key")).toAbsolutePath().normalize();
        Path cache = Path.of(required(options, "cache")).toAbsolutePath().normalize();
        int timeoutSeconds = Integer.parseInt(options.getOrDefault("timeout-seconds", "240"));
        if (!Files.isRegularFile(eventPath) || !Files.isRegularFile(publicKey)
                || timeoutSeconds < 30 || timeoutSeconds > 600) {
            throw new IllegalArgumentException("canary paths or timeout are invalid");
        }

        ModelUpdateEvent event = ModelUpdateEvent.fromJson(Files.readAllBytes(eventPath));
        String runId = UUID.randomUUID().toString();
        BehaviorJobConfig config = config(
                brokers, featureTopic, updateTopic, observationTopic,
                publicKey, cache, event, runId);
        config.validateChampionChallengerShadowConfig();
        ensureTopics(brokers, featureTopic, updateTopic, observationTopic, ackTopic, detectionTopic);

        StreamExecutionEnvironment env = StreamExecutionEnvironment.createLocalEnvironment(1);
        env.setParallelism(1);
        env.setMaxParallelism(128);
        DataStream<FeatureStat> features = env.fromSource(
                        featureSource(config), WatermarkStrategy.noWatermarks(), "N012-Canary-Features")
                .uid("n012-canary-feature-source");
        DataStream<ModelUpdateEvent> updates = env.fromSource(
                        BehaviorDetectionJob.createModelUpdateSource(config),
                        WatermarkStrategy.noWatermarks(), "N012-Canary-ModelUpdates")
                .uid("n012-canary-model-update-source");
        SingleOutputStreamOperator<FeatureStat> carried = features
                .connect(updates.broadcast(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                        ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE))
                .process(new ModelUpdateBroadcastHandler(config))
                .returns(ProtoTypeInformation.forMessage(FeatureStat.class))
                .uid("n012-canary-model-update-broadcast");
        DataStream<ShadowEvaluationRequest> requests = carried.getSideOutput(
                ModelUpdateBroadcastHandler.SHADOW_EVALUATION_REQUEST_TAG);
        DataStream<ChampionChallengerObservation> observations =
                BehaviorDetectionJob.buildChampionChallengerObserver(requests, config);
        observations.sinkTo(ChampionChallengerObservationKafkaSinkFactory.create(
                        brokers, observationTopic))
                .uid("n012-canary-observation-sink");

        JobClient job = env.executeAsync("M08 N012 Champion Challenger Shadow Canary");
        try {
            waitForRunning(job, timeoutSeconds);
            publish(brokers, updateTopic, event.getModelId(), Files.readAllBytes(eventPath));
            // The broadcast side verifies and stages the complete immutable package before
            // recording the candidate in state. Keep the feature strictly after that step.
            Thread.sleep(15_000L);
            FeatureStat feature = canaryFeature(event, runId);
            publish(brokers, featureTopic, event.getTenantId(), feature.toByteArray());
            JsonNode observation = collectObservation(
                    brokers, observationTopic, feature, event, runId, timeoutSeconds);
            assertTopicEmpty(brokers, ackTopic, runId + "-ack-verifier");
            assertTopicEmpty(brokers, detectionTopic, runId + "-detection-verifier");
            System.out.println(MAPPER.writeValueAsString(Map.ofEntries(
                    Map.entry("status", "PASS"),
                    Map.entry("stage", "CHAMPION_CHALLENGER_COMPARED"),
                    Map.entry("observation_id", observation.path("observation_id").asText()),
                    Map.entry("observation_status", observation.path("status").asText()),
                    Map.entry("serving_result_source", observation.path("serving_result_source").asText()),
                    Map.entry("challenger_package_sha256", event.getPackageSha256()),
                    Map.entry("activation_ack_emitted", false),
                    Map.entry("detection_emitted", false),
                    Map.entry("clickhouse_sink_present", false),
                    Map.entry("dlq_sink_present", false))));
        } finally {
            job.cancel().get(30, TimeUnit.SECONDS);
        }
    }

    private static BehaviorJobConfig config(
            String brokers, String featureTopic, String updateTopic, String observationTopic,
            Path publicKey, Path cache, ModelUpdateEvent event, String runId) {
        ModelUpdateEvent.Compatibility compatibility = event.getCompatibility();
        if (compatibility == null) {
            throw new IllegalArgumentException("shadow event compatibility is required");
        }
        return new BehaviorJobConfig.Builder()
                .kafkaBrokers(brokers)
                .inputTopic(featureTopic)
                .modelUpdateTopic(updateTopic)
                .modelShadowObservationTopic(observationTopic)
                .consumerGroupId("n012-canary-serving-unused-" + runId)
                .modelShadowFeatureConsumerGroupId("n012-canary-features-" + runId)
                .modelShadowUpdateConsumerGroupId("n012-canary-updates-" + runId)
                .detectionMode("known_frozen")
                .modelUpdateConsumerEnabled(true)
                .modelHotUpdateEnabled(false)
                .modelConsumerDeploymentId("n012-canary-" + runId)
                .modelConsumerProfileSha256("a".repeat(64))
                .modelRuntimeContract(compatibility.getRuntimeContract())
                .modelRuntimeVersion(compatibility.getRuntimeVersion())
                .modelFeatureSchemaVersion(compatibility.getFeatureSchemaVersion())
                .modelGraphSchemaVersion(compatibility.getGraphSchemaVersion())
                .modelSigningPublicKeyFile(publicKey.toString())
                .modelPath(cache.toString())
                .modelCacheSize(8)
                .modelShadowEvaluationEnabled(true)
                .modelShadowObservationOnly(true)
                .modelShadowSampleRate(1.0d)
                .modelShadowChallengerTimeoutMs(1_000L)
                .modelShadowPackageLoadTimeoutMs(120_000L)
                .modelShadowChallengerThreads(1)
                .modelShadowChallengerQueueCapacity(8)
                .asyncTimeoutMs(5_000L)
                .asyncCapacity(8)
                .inferenceThreads(1)
                .parallelism(1)
                .maxParallelism(128)
                .build();
    }

    private static KafkaSource<FeatureStat> featureSource(BehaviorJobConfig config) {
        return KafkaSource.<FeatureStat>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getInputTopic())
                .setGroupId(config.getModelShadowFeatureConsumerGroupId())
                .setStartingOffsets(OffsetsInitializer.committedOffsets(
                        org.apache.kafka.clients.consumer.OffsetResetStrategy.LATEST))
                .setValueOnlyDeserializer(new DeserializationSchema<FeatureStat>() {
                    @Override
                    public FeatureStat deserialize(byte[] message) throws java.io.IOException {
                        return FeatureStat.parseFrom(message);
                    }

                    @Override
                    public boolean isEndOfStream(FeatureStat nextElement) {
                        return false;
                    }

                    @Override
                    public TypeInformation<FeatureStat> getProducedType() {
                        return ProtoTypeInformation.forMessage(FeatureStat.class);
                    }
                })
                .setProperties(ConfigUtil.kafkaClientProperties())
                .build();
    }

    private static FeatureStat canaryFeature(ModelUpdateEvent event, String runId) {
        long now = System.currentTimeMillis();
        EventHeader header = EventHeader.newBuilder()
                .setEventId("n012-feature-" + runId)
                .setTenantId(event.getTenantId())
                .setEventTs(now)
                .setIngestTs(now)
                .setEventType("traffic.feature.stat.v1")
                .setSchemaVersion("1")
                .build();
        return FeatureStat.newBuilder()
                .setHeader(header)
                .setSchemaVersion("1")
                .setObjectType("flow")
                .setObjectId("n012-object-" + runId)
                .setCommunityId("1:n012-canary")
                .setTs(now)
                .setProtocol(6)
                .setDurationMs(2_000)
                .setPps(42.0f)
                .setBps(8_192.0f)
                .setUpDownRatio(1.5f)
                .setPktlenMean(512.0f)
                .setPktlenStd(64.0f)
                .setIatMeanMs(12.0f)
                .setIatStdMs(3.0f)
                .setActiveMeanMs(500.0f)
                .setIdleMeanMs(100.0f)
                .setTcpFlagSynCnt(2)
                .setTcpFlagAckCnt(20)
                .setTcpInitWinBytesFwd(65_535)
                .setTcpInitWinBytesBwd(65_535)
                .build();
    }

    private static void ensureTopics(String brokers, String... topics) throws Exception {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        try (AdminClient admin = AdminClient.create(properties)) {
            List<NewTopic> requested = new ArrayList<>();
            for (String topic : topics) requested.add(new NewTopic(topic, 1, (short) 1));
            try {
                admin.createTopics(requested).all().get(30, TimeUnit.SECONDS);
            } catch (java.util.concurrent.ExecutionException error) {
                if (!(error.getCause() instanceof TopicExistsException)) throw error;
            }
        }
    }

    private static void publish(String brokers, String topic, String key, byte[] value)
            throws Exception {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG, ByteArraySerializer.class.getName());
        properties.setProperty(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG, ByteArraySerializer.class.getName());
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        try (KafkaProducer<byte[], byte[]> producer = new KafkaProducer<>(properties)) {
            producer.send(new ProducerRecord<>(topic, key.getBytes(StandardCharsets.UTF_8), value))
                    .get(30, TimeUnit.SECONDS);
        }
    }

    private static JsonNode collectObservation(
            String brokers, String topic, FeatureStat feature, ModelUpdateEvent event,
            String runId, int timeoutSeconds) throws Exception {
        Properties properties = consumerProperties(brokers, runId + "-observation-verifier");
        Instant deadline = Instant.now().plusSeconds(timeoutSeconds);
        try (KafkaConsumer<byte[], byte[]> consumer = new KafkaConsumer<>(properties)) {
            consumer.subscribe(List.of(topic));
            while (Instant.now().isBefore(deadline)) {
                for (ConsumerRecord<byte[], byte[]> record : consumer.poll(Duration.ofMillis(500))) {
                    JsonNode value = MAPPER.readTree(record.value());
                    if (!feature.getHeader().getEventId().equals(value.path("source_event_id").asText())) {
                        continue;
                    }
                    if (!"compared".equals(value.path("status").asText())
                            || !"champion".equals(value.path("serving_result_source").asText())
                            || !event.getPackageSha256().equals(
                                    value.path("challenger_package_sha256").asText())
                            || !value.path("champion_score").isNumber()
                            || !value.path("challenger_score").isNumber()) {
                        throw new IllegalStateException("N012 observation drifted: " + value);
                    }
                    return value;
                }
            }
        }
        throw new IllegalStateException("N012 comparison observation was not produced before deadline");
    }

    private static void assertTopicEmpty(String brokers, String topic, String group) {
        try (KafkaConsumer<byte[], byte[]> consumer = new KafkaConsumer<>(
                consumerProperties(brokers, group))) {
            consumer.subscribe(List.of(topic));
            for (ConsumerRecord<byte[], byte[]> ignored : consumer.poll(Duration.ofSeconds(3))) {
                throw new IllegalStateException("forbidden output was emitted to " + topic);
            }
        }
    }

    private static Properties consumerProperties(String brokers, String group) {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ConsumerConfig.GROUP_ID_CONFIG, group);
        properties.setProperty(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, ByteArrayDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, ByteArrayDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        properties.setProperty(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, "false");
        return properties;
    }

    private static void waitForRunning(JobClient job, int timeoutSeconds) throws Exception {
        Instant deadline = Instant.now().plusSeconds(timeoutSeconds);
        while (Instant.now().isBefore(deadline)) {
            JobStatus status = job.getJobStatus().get(10, TimeUnit.SECONDS);
            if (status == JobStatus.RUNNING) {
                Thread.sleep(2_000L);
                return;
            }
            if (status.isTerminalState()) {
                throw new IllegalStateException("Flink N012 canary terminated early: " + status);
            }
            Thread.sleep(250L);
        }
        throw new IllegalStateException("Flink N012 canary did not reach RUNNING before deadline");
    }

    private static Map<String, String> parseArgs(String[] args) {
        if (args.length % 2 != 0) {
            throw new IllegalArgumentException("canary arguments must be --key value pairs");
        }
        Map<String, String> values = new HashMap<>();
        for (int index = 0; index < args.length; index += 2) {
            if (!args[index].startsWith("--")) {
                throw new IllegalArgumentException("invalid canary argument " + args[index]);
            }
            values.put(args[index].substring(2), args[index + 1]);
        }
        return values;
    }

    private static String required(Map<String, String> values, String key) {
        String value = values.get(key);
        if (value == null || value.isBlank()) {
            throw new IllegalArgumentException("--" + key + " is required");
        }
        return value;
    }
}
