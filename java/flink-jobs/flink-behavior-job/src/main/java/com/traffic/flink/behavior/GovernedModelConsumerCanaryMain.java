package com.traffic.flink.behavior;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.ModelConsumerProfile;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.common.ConfigUtil;
import org.apache.flink.api.common.JobStatus;
import org.apache.flink.core.execution.JobClient;
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

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.Set;
import java.util.concurrent.TimeUnit;

/**
 * Isolated Kubernetes verifier for the complete Kafka -> Flink shadow-load ->
 * Kafka ACK rail.  It never enables serving outputs or activation.
 */
public final class GovernedModelConsumerCanaryMain {
    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final int PARALLELISM = 4;

    private GovernedModelConsumerCanaryMain() {}

    public static void main(String[] args) throws Exception {
        Map<String, String> options = parseArgs(args);
        String brokers = required(options, "brokers");
        String updateTopic = required(options, "update-topic");
        String ackTopic = required(options, "ack-topic");
        Path eventPath = Path.of(required(options, "event")).toAbsolutePath().normalize();
        Path publicKey = Path.of(required(options, "public-key")).toAbsolutePath().normalize();
        Path cache = Path.of(required(options, "cache")).toAbsolutePath().normalize();
        int timeoutSeconds = Integer.parseInt(options.getOrDefault("timeout-seconds", "180"));
        if (!Files.isRegularFile(eventPath) || !Files.isRegularFile(publicKey)
                || timeoutSeconds < 30 || timeoutSeconds > 600) {
            throw new IllegalArgumentException("canary paths or timeout are invalid: event="
                    + eventPath + "/" + Files.isRegularFile(eventPath) + ", public_key="
                    + publicKey + "/" + Files.isRegularFile(publicKey) + ", timeout=" + timeoutSeconds);
        }
        ModelUpdateEvent event = ModelUpdateEvent.fromJson(Files.readAllBytes(eventPath));
        String provisionalDeploymentId = "m08-n011-canary-provisional";
        BehaviorJobConfig provisional = config(
                brokers, updateTopic, ackTopic, provisionalDeploymentId, provisionalDeploymentId,
                "0".repeat(64), publicKey, cache, event);
        String profileSha256 = ModelConsumerProfile.calculateSha256(provisional);
        String deploymentId = "m08-n011-canary-" + profileSha256.substring(0, 12)
                + "-" + event.getEventId();
        String groupId = deploymentId;
        BehaviorJobConfig config = config(
                brokers, updateTopic, ackTopic, groupId, deploymentId,
                profileSha256, publicKey, cache, event);

        ensureTopics(brokers, updateTopic, ackTopic);
        StreamExecutionEnvironment env = StreamExecutionEnvironment.createLocalEnvironment(PARALLELISM);
        env.setParallelism(PARALLELISM);
        env.setMaxParallelism(128);
        ModelUpdateConsumerJob.buildPipeline(env, config);

        JobClient job = env.executeAsync("M08 N011 Governed Model Consumer Canary");
        try {
            waitForRunning(job, timeoutSeconds);
            publishEvent(brokers, updateTopic, event, eventPath);
            CanaryResult result = collectExactAcks(
                    brokers, ackTopic, event, deploymentId, profileSha256, timeoutSeconds);
            System.out.println(MAPPER.writeValueAsString(Map.of(
                    "status", "PASS",
                    "stage", "CONSUMER_SHADOW_READY",
                    "activation_applied", false,
                    "serving_outputs_emitted", false,
                    "consumer_ready_subtasks", result.consumerReadySubtasks,
                    "shadow_ready_subtasks", result.shadowReadySubtasks,
                    "package_sha256", event.getPackageSha256(),
                    "consumer_profile_sha256", profileSha256)));
        } finally {
            job.cancel().get(30, TimeUnit.SECONDS);
        }
    }

    private static BehaviorJobConfig config(
            String brokers, String updateTopic, String ackTopic, String groupId,
            String deploymentId, String profileSha256, Path publicKey, Path cache,
            ModelUpdateEvent event) {
        ModelUpdateEvent.Compatibility compatibility = event.getCompatibility();
        if (compatibility == null) {
            throw new IllegalArgumentException("shadow event compatibility is required");
        }
        return new BehaviorJobConfig.Builder()
                .kafkaBrokers(brokers)
                .modelUpdateTopic(updateTopic)
                .modelAppliedTopic(ackTopic)
                .consumerGroupId(groupId)
                .detectionMode("off")
                .enabledModels(Set.of())
                .modelUpdateConsumerEnabled(true)
                .modelHotUpdateEnabled(false)
                .modelConsumerDeploymentId(deploymentId)
                .modelConsumerProfileSha256(profileSha256)
                .modelRuntimeContract(compatibility.getRuntimeContract())
                .modelRuntimeVersion(compatibility.getRuntimeVersion())
                .modelFeatureSchemaVersion(compatibility.getFeatureSchemaVersion())
                .modelGraphSchemaVersion(compatibility.getGraphSchemaVersion())
                .modelSigningPublicKeyFile(publicKey.toString())
                .modelPath(cache.toString())
                .modelCacheSize(8)
                .parallelism(PARALLELISM)
                .maxParallelism(128)
                .build();
    }

    private static void ensureTopics(String brokers, String... topics) throws Exception {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        try (AdminClient admin = AdminClient.create(properties)) {
            List<NewTopic> requested = new ArrayList<>();
            for (String topic : topics) {
                requested.add(new NewTopic(topic, 1, (short) 1));
            }
            try {
                admin.createTopics(requested).all().get(30, TimeUnit.SECONDS);
            } catch (java.util.concurrent.ExecutionException error) {
                if (!(error.getCause() instanceof TopicExistsException)) {
                    throw error;
                }
            }
        }
    }

    private static void waitForRunning(JobClient job, int timeoutSeconds) throws Exception {
        Instant deadline = Instant.now().plusSeconds(timeoutSeconds);
        while (Instant.now().isBefore(deadline)) {
            JobStatus status = job.getJobStatus().get(10, TimeUnit.SECONDS);
            if (status == JobStatus.RUNNING) {
                Thread.sleep(2000L);
                return;
            }
            if (status.isTerminalState()) {
                throw new IllegalStateException("Flink canary terminated before RUNNING: " + status);
            }
            Thread.sleep(250L);
        }
        throw new IllegalStateException("Flink canary did not reach RUNNING before deadline");
    }

    private static void publishEvent(String brokers, String topic, ModelUpdateEvent event,
                                     Path eventPath) throws Exception {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG, ByteArraySerializer.class.getName());
        properties.setProperty(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG, ByteArraySerializer.class.getName());
        properties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        properties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        try (KafkaProducer<byte[], byte[]> producer = new KafkaProducer<>(properties)) {
            byte[] key = event.getModelId().getBytes(java.nio.charset.StandardCharsets.UTF_8);
            producer.send(new ProducerRecord<>(topic, key, Files.readAllBytes(eventPath)))
                    .get(30, TimeUnit.SECONDS);
        }
    }

    private static CanaryResult collectExactAcks(
            String brokers, String topic, ModelUpdateEvent event,
            String deploymentId, String profileSha256, int timeoutSeconds) {
        Properties properties = ConfigUtil.kafkaClientProperties();
        properties.setProperty(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ConsumerConfig.GROUP_ID_CONFIG, deploymentId + "-verifier");
        properties.setProperty(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG, ByteArrayDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG, ByteArrayDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        properties.setProperty(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, "false");
        Set<Integer> consumerReady = new HashSet<>();
        Set<Integer> shadowReady = new HashSet<>();
        Instant deadline = Instant.now().plusSeconds(timeoutSeconds);
        try (KafkaConsumer<byte[], byte[]> consumer = new KafkaConsumer<>(properties)) {
            consumer.subscribe(List.of(topic));
            while (Instant.now().isBefore(deadline)
                    && (consumerReady.size() < PARALLELISM || shadowReady.size() < PARALLELISM)) {
                for (ConsumerRecord<byte[], byte[]> record : consumer.poll(Duration.ofMillis(500))) {
                    try {
                        JsonNode ack = MAPPER.readTree(record.value());
                        int subtask = ack.path("subtask_index").asInt(-1);
                        if (subtask < 0 || subtask >= PARALLELISM
                                || ack.path("parallelism").asInt() != PARALLELISM) {
                            throw new IllegalArgumentException("ACK subtask quorum identity is invalid");
                        }
                        String type = ack.path("ack_type").asText();
                        String status = ack.path("status").asText();
                        if ("consumer_ready".equals(type)
                                && deploymentId.equals(ack.path("consumer_deployment_id").asText())) {
                            if (!"consumer_ready".equals(status)
                                    || !profileSha256.equals(ack.path("consumer_profile_sha256").asText())) {
                                throw new IllegalArgumentException("consumer readiness ACK drifted");
                            }
                            consumerReady.add(subtask);
                        } else if ("shadow_load".equals(type)
                                && event.getEventId().equals(ack.path("event_id").asText())) {
                            if (!"shadow_ready".equals(status)
                                    || !event.getPackageSha256().equals(ack.path("package_sha256").asText())) {
                                throw new IllegalArgumentException("shadow-load ACK failed or drifted: " + ack);
                            }
                            shadowReady.add(subtask);
                        }
                    } catch (RuntimeException runtime) {
                        throw runtime;
                    } catch (Exception error) {
                        throw new IllegalArgumentException("cannot validate model consumer ACK", error);
                    }
                }
            }
        }
        if (consumerReady.size() != PARALLELISM || shadowReady.size() != PARALLELISM) {
            throw new IllegalStateException("incomplete ACK quorum: consumer=" + consumerReady
                    + ", shadow=" + shadowReady);
        }
        return new CanaryResult(consumerReady.size(), shadowReady.size());
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

    private static final class CanaryResult {
        final int consumerReadySubtasks;
        final int shadowReadySubtasks;

        CanaryResult(int consumerReadySubtasks, int shadowReadySubtasks) {
            this.consumerReadySubtasks = consumerReadySubtasks;
            this.shadowReadySubtasks = shadowReadySubtasks;
        }
    }
}
