package com.traffic.flink.pcap;

import com.traffic.flink.pcap.process.PcapIndexedRecordProcessFunction;
import com.traffic.flink.pcap.process.PcapManifestPolicy;
import com.traffic.flink.pcap.process.PcapManifestValidatorTest;
import com.traffic.flink.pcap.sink.ClickHousePcapCarrierSinkFactory;
import com.traffic.flink.pcap.sink.DLQSinkFactory;
import com.traffic.flink.pcap.sink.PcapClickHouseIntegrationConfig;
import com.traffic.flink.pcap.sink.PcapProjectionColumns;
import com.traffic.flink.pcap.source.PcapConsumerConfig;
import com.traffic.flink.pcap.source.PcapConsumerPipeline;
import com.traffic.flink.pcap.source.PcapConsumerPipelineResult;
import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.api.common.JobExecutionResult;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.core.execution.JobClient;
import org.apache.flink.core.execution.SavepointFormatType;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.runtime.jobgraph.SavepointConfigOptions;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.AdminClientConfig;
import org.apache.kafka.clients.admin.NewTopic;
import org.apache.kafka.clients.consumer.OffsetAndMetadata;
import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.apache.kafka.clients.consumer.ConsumerRecords;
import org.apache.kafka.clients.consumer.KafkaConsumer;
import org.apache.kafka.clients.producer.KafkaProducer;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;
import org.apache.kafka.common.TopicPartition;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.apache.kafka.common.serialization.ByteArrayDeserializer;
import org.apache.kafka.common.serialization.ByteArraySerializer;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.junit.jupiter.api.io.TempDir;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.nio.file.FileAlreadyExistsException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.StandardOpenOption;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.time.Duration;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.UUID;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.TimeUnit;
import java.util.function.BooleanSupplier;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.fail;

/**
 * PCAP Index Job 集成测试 — K8s Kafka (kubectl exec)
 */
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class PcapIndexJobIntegrationTest {

    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexJobIntegrationTest.class);

    private String kubeExec(String namespace, String pod, String... cmd) throws Exception {
        // Use -- to separate kubectl args from container command args
        String[] fullCmd = new String[cmd.length + 6];
        fullCmd[0] = "kubectl"; fullCmd[1] = "exec"; fullCmd[2] = "-n";
        fullCmd[3] = namespace; fullCmd[4] = pod; fullCmd[5] = "--";
        System.arraycopy(cmd, 0, fullCmd, 6, cmd.length);
        ProcessBuilder processBuilder = new ProcessBuilder(fullCmd).redirectErrorStream(true);
        Map<String, String> environment = processBuilder.environment();
        for (String proxyVariable : new String[]{
                "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
                "http_proxy", "https_proxy", "all_proxy"
        }) {
            environment.remove(proxyVariable);
        }
        Process p = processBuilder.start();
        StringBuilder sb = new StringBuilder();
        try (BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()))) {
            String line; while ((line = r.readLine()) != null) sb.append(line).append("\n");
        }
        int exitCode = p.waitFor();
        String output = sb.toString().trim();
        assertThat(exitCode)
                .as("kubectl exec failed: %s", output)
                .isZero();
        return output;
    }

    @Test @Order(1)
    @EnabledIfEnvironmentVariable(named = "K8S_INTEGRATION_TEST_ENABLED", matches = "true")
    @DisplayName("K8s Kafka produce → consume (kubectl exec)")
    void testKafkaProduceConsume() throws Exception {
        String msg = "pcap-test-" + UUID.randomUUID().toString().substring(0, 8);
        kubeExec("middleware", "kafka-0", "bash", "-c",
                "echo '" + msg + "' | /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server localhost:9092 --topic perf-test");
        Thread.sleep(2000);
        String result = kubeExec("middleware", "kafka-0",
                "/opt/kafka/bin/kafka-console-consumer.sh", "--bootstrap-server", "localhost:9092",
                "--topic", "perf-test", "--from-beginning", "--max-messages", "1", "--timeout-ms", "5000");
        LOG.info("Kafka round-trip: {}", result.length() > 0 ? "OK" : "FAIL");
        assertThat(result).isNotEmpty();
    }

    @Test @Order(2)
    @EnabledIfEnvironmentVariable(named = "K8S_INTEGRATION_TEST_ENABLED", matches = "true")
    @DisplayName("K8s ClickHouse 连通性验证 (kubectl exec)")
    void testClickHouseConnectivity() throws Exception {
        // Simple: verify ClickHouse is reachable and can execute queries
        String result = kubeExec("middleware", "clickhouse-1-0", "bash", "-c",
                "clickhouse-client --query 'SELECT 1'");
        assertThat(result).contains("1");
        LOG.info("ClickHouse connectivity OK: {}", result.trim());
    }

    @Test @Order(3)
    @DisplayName("Protobuf 序列化/反序列化")
    void testProtobufRoundTrip() throws Exception {
        long now = System.currentTimeMillis();
        PcapIndexMeta original = PcapIndexMeta.newBuilder()
                .setTenantId("proto-test").setProbeId("p1").setFileKey("f.pcap")
                .setTsStart(now - 30000).setTsEnd(now).setByteSize(2048).setZstdLevel(5)
                .setSha256("deadbeef").setCommunityId("1:p==").setFlowId("f1")
                .setBloomFilterB64("cHJvdG8=").addCommunityIds("1:a==").addCommunityIds("1:b==")
                .setCreatedTs(now).build();

        PcapIndexMeta parsed = PcapIndexMeta.parseFrom(original.toByteArray());
        assertThat(parsed.getTenantId()).isEqualTo("proto-test");
        assertThat(parsed.getByteSize()).isEqualTo(2048);
        assertThat(parsed.getZstdLevel()).isEqualTo(5);
        assertThat(parsed.getSha256()).isEqualTo("deadbeef");
        assertThat(parsed.getCommunityIdsCount()).isEqualTo(2);
        LOG.info("Protobuf round-trip OK: {} bytes", original.toByteArray().length);
    }

    @Test @Order(4)
    @DisplayName("Community IDs 大数组 (1500 IDs)")
    void testLargeCommunityIdsArray() {
        PcapIndexMeta.Builder b = PcapIndexMeta.newBuilder()
                .setTenantId("t").setProbeId("p").setFileKey("f").setByteSize(1)
                .setTsStart(1).setTsEnd(1);
        for (int i = 0; i < 1500; i++) b.addCommunityIds("1:id" + i + "==");
        PcapIndexMeta m = b.build();
        assertThat(m.getCommunityIdsCount()).isEqualTo(1500);
        assertThat(m.toByteArray().length).isGreaterThan(0);
        LOG.info("Large array: {} IDs, {} bytes", 1500, m.toByteArray().length);
    }

    @Test @Order(5)
    @EnabledIfEnvironmentVariable(named = "M02_PCAP_KAFKA_INTEGRATION_ENABLED", matches = "true")
    @DisplayName("真实 Kafka + Flink 重启：失败 checkpoint 不提交 offset，重放 identity 稳定")
    void testCheckpointBoundOffsetAcrossRealKafkaRestart(@TempDir Path tempDir) throws Exception {
        String brokers = requiredEnvironment("M02_PCAP_KAFKA_BOOTSTRAP_SERVERS");
        if (!"true".equalsIgnoreCase(requiredEnvironment("M02_PCAP_KAFKA_BROKER_OWNED_BY_TEST"))) {
            fail("N010 integration refuses a broker that is not explicitly owned by this test");
        }

        String groupId = "m02-pcap-restart-" + UUID.randomUUID();
        Properties adminProperties = new Properties();
        adminProperties.setProperty(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);

        try (AdminClient admin = AdminClient.create(adminProperties)) {
            createCanonicalTopicsOnEmptyBroker(admin);
            PcapIndexMeta firstMeta = PcapManifestValidatorTest.v2Meta().build();
            long producedOffset = produceStrictV2Record(brokers, firstMeta);
            TopicPartition sourcePartition = new TopicPartition(PcapConsumerConfig.INPUT_TOPIC, 0);

            Path probeRoot = tempDir.resolve("restart-probe");
            Files.createDirectories(probeRoot);
            Path checkpointRoot = tempDir.resolve("checkpoints");
            Files.createDirectories(checkpointRoot);

            JobClient jobClient = null;
            try {
                jobClient = startAcceptanceJob(
                        brokers, groupId, probeRoot, checkpointRoot, null);
                CompletableFuture<JobExecutionResult> executionResult = jobClient.getJobExecutionResult();
                waitForMarker(jobClient, executionResult,
                        probeRoot.resolve(RestartBarrierSink.FIRST_FAILURE),
                        Duration.ofSeconds(20), "Flink did not deliver the PCAP record to the sink");
                waitForMarker(jobClient, executionResult,
                        probeRoot.resolve(RestartBarrierSink.SECOND_ATTEMPT),
                        Duration.ofSeconds(30), "Flink did not replay the PCAP record after injected failure");

                long offsetBeforeRelease = committedOffset(admin, groupId, sourcePartition);
                assertThat(offsetBeforeRelease)
                        .as("source offset must not advance before the replay reaches a successful checkpoint")
                        .isLessThanOrEqualTo(producedOffset);

                Files.write(probeRoot.resolve(RestartBarrierSink.RELEASE), new byte[]{1},
                        StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
                long expectedCommittedOffset = producedOffset + 1;
                waitUntil(() -> committedOffsetUnchecked(admin, groupId, sourcePartition)
                                >= expectedCommittedOffset,
                        Duration.ofSeconds(30), "successful checkpoint did not commit the Kafka offset");

                List<String> attempts = Files.readAllLines(
                        probeRoot.resolve(RestartBarrierSink.ATTEMPTS), StandardCharsets.UTF_8);
                assertThat(attempts).hasSizeGreaterThanOrEqualTo(2);
                assertThat(attempts.stream().map(line -> line.split("\\|", -1)[0]).distinct())
                        .as("the same Kafka authority tuple must be replayed")
                        .containsExactly(PcapConsumerConfig.INPUT_TOPIC + ":0:" + producedOffset);
                assertThat(attempts.stream().map(line -> line.split("\\|", -1)[1]).distinct())
                        .as("the replay must retain one deterministic logical projection identity")
                        .hasSize(1);
                assertThat(Files.readAllLines(
                        probeRoot.resolve(RestartBarrierSink.SUCCESSES), StandardCharsets.UTF_8))
                        .as("the sink must expose one successful logical projection")
                        .hasSize(1);
                assertThat(committedOffset(admin, groupId, sourcePartition))
                        .isEqualTo(expectedCommittedOffset);

                long invalidOffset = produceRawRecord(brokers, firstMeta,
                        new byte[]{0x0a, 0x7f});
                PcapIndexMeta incompleteManifest = PcapManifestValidatorTest.v2Meta()
                        .setFileKey("tenant-a/probe-a/incomplete-object.pcap.zst")
                        .clearBucket()
                        .build();
                long incompleteManifestOffset = produceStrictV2Record(
                        brokers, incompleteManifest);
                PcapIndexMeta lateMeta = PcapManifestValidatorTest.v2Meta()
                        .setFileKey("tenant-a/probe-a/late-object.pcap.zst")
                        .setTsStart(firstMeta.getTsStart() - 60_000L)
                        .setTsEnd(firstMeta.getTsStart() - 59_000L)
                        .setCreatedTs(firstMeta.getCreatedTs() - 60_000L)
                        .build();
                long lateOffset = produceStrictV2Record(brokers, lateMeta);
                waitUntil(() -> committedOffsetUnchecked(admin, groupId, sourcePartition)
                                >= lateOffset + 1,
                        Duration.ofSeconds(30), "mixed DLQ and late-event checkpoint did not commit");

                String deadLetter = consumeCanonicalDlq(brokers, invalidOffset);
                assertThat(deadLetter)
                        .contains("\"original_topic\":\"pcap.index.v1\"")
                        .contains("\"original_partition\":0")
                        .contains("\"original_offset\":" + invalidOffset)
                        .contains("\"error_code\":\"INVALID_PROTOBUF\"");
                String manifestDeadLetter = consumeCanonicalDlq(
                        brokers, incompleteManifestOffset);
                assertThat(manifestDeadLetter)
                        .contains("\"original_offset\":" + incompleteManifestOffset)
                        .contains("\"error_code\":\"MANIFEST_REJECTED\"")
                        .contains("MISSING_BUCKET");
                assertThat(Files.readAllLines(
                        probeRoot.resolve(RestartBarrierSink.SUCCESSES), StandardCharsets.UTF_8))
                        .anyMatch(line -> line.startsWith(
                                PcapConsumerConfig.INPUT_TOPIC + ":0:" + lateOffset + "|"));

                Path savepointRoot = tempDir.resolve("savepoints");
                Files.createDirectories(savepointRoot);
                String savepoint = jobClient.triggerSavepoint(
                                savepointRoot.toUri().toString(), SavepointFormatType.CANONICAL)
                        .get(30, TimeUnit.SECONDS);
                assertThat(Files.exists(Path.of(java.net.URI.create(savepoint)))).isTrue();
                jobClient.cancel().get(10, TimeUnit.SECONDS);
                jobClient = null;

                PcapIndexMeta restoredMeta = PcapManifestValidatorTest.v2Meta()
                        .setFileKey("tenant-a/probe-a/after-savepoint.pcap.zst")
                        .setTsStart(firstMeta.getTsStart() + 60_000L)
                        .setTsEnd(firstMeta.getTsEnd() + 60_000L)
                        .setCreatedTs(firstMeta.getCreatedTs() + 60_000L)
                        .build();
                long restoredOffset = produceStrictV2Record(brokers, restoredMeta);
                Path restoredCheckpointRoot = tempDir.resolve("restored-checkpoints");
                Files.createDirectories(restoredCheckpointRoot);
                jobClient = startAcceptanceJob(
                        brokers, groupId, probeRoot, restoredCheckpointRoot, savepoint);
                CompletableFuture<JobExecutionResult> restoredExecution = jobClient.getJobExecutionResult();
                waitUntil(() -> committedOffsetUnchecked(admin, groupId, sourcePartition)
                                >= restoredOffset + 1,
                        Duration.ofSeconds(30), "savepoint-restored graph did not commit the next offset");
                waitForContent(jobClient, restoredExecution,
                        probeRoot.resolve(RestartBarrierSink.SUCCESSES),
                        PcapConsumerConfig.INPUT_TOPIC + ":0:" + restoredOffset + "|",
                        Duration.ofSeconds(20), "restored graph did not preserve the stable carrier route");
                assertThat(committedOffset(admin, groupId, sourcePartition))
                        .isEqualTo(restoredOffset + 1);
            } finally {
                if (jobClient != null) {
                    try {
                        jobClient.cancel().get(10, TimeUnit.SECONDS);
                    } catch (Exception ignored) {
                        LOG.warn("N010 acceptance job was already terminal during cancellation", ignored);
                    }
                }
            }
        }
    }

    @Test @Order(6)
    @EnabledIfEnvironmentVariable(
            named = "M02_PCAP_KAFKA_CLICKHOUSE_INTEGRATION_ENABLED", matches = "true")
    @DisplayName("真实 Kafka + Flink + ClickHouse：故障重放物理可见且 FINAL 逻辑唯一")
    void testRealClickHouseReplayConvergenceAndLateRecord(@TempDir Path tempDir) throws Exception {
        String brokers = requiredEnvironment("M02_PCAP_KAFKA_BOOTSTRAP_SERVERS");
        if (!"true".equalsIgnoreCase(requiredEnvironment("M02_PCAP_KAFKA_BROKER_OWNED_BY_TEST"))) {
            fail("N010 integration refuses a broker that is not explicitly owned by this test");
        }
        String jdbcUrl = requiredEnvironment("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_JDBC_URL");
        if (!"codex_ephemeral_m02_clickhouse".equals(
                requiredEnvironment("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_SENTINEL"))) {
            fail("N010 integration refuses a ClickHouse instance that is not explicitly owned by this test");
        }
        String clickHouseUser = System.getenv().getOrDefault(
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_USER", "m02");
        String clickHousePassword = System.getenv().getOrDefault(
                "PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_PASSWORD", "");
        String groupId = "m02-pcap-clickhouse-restart-" + UUID.randomUUID();
        TopicPartition sourcePartition = new TopicPartition(PcapConsumerConfig.INPUT_TOPIC, 0);
        Path probeRoot = tempDir.resolve("clickhouse-restart-probe");
        Path checkpointRoot = tempDir.resolve("clickhouse-checkpoints");
        Files.createDirectories(probeRoot);
        Files.createDirectories(checkpointRoot);

        Properties adminProperties = new Properties();
        adminProperties.setProperty(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        try (AdminClient admin = AdminClient.create(adminProperties)) {
            createCanonicalTopicsOnEmptyBroker(admin);
            PcapIndexMeta meta = PcapManifestValidatorTest.v2Meta()
                    .setTenantId("tenant-clickhouse-" + UUID.randomUUID())
                    .setProbeId("probe-clickhouse")
                    .setFileKey("tenant-clickhouse/probe-clickhouse/replayed.pcap.zst")
                    .build();
            // The unique tenant isolates both physical replay rows and their FINAL projection.
            long sourceOffset = produceStrictV2Record(brokers, meta);
            StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
            env.setParallelism(1);
            env.setRestartStrategy(RestartStrategies.fixedDelayRestart(2, Time.milliseconds(100)));
            env.enableCheckpointing(250L, CheckpointingMode.AT_LEAST_ONCE);
            env.getCheckpointConfig().setCheckpointTimeout(10_000L);
            env.getCheckpointConfig().setMinPauseBetweenCheckpoints(50L);
            env.getCheckpointConfig().setMaxConcurrentCheckpoints(1);
            env.getCheckpointConfig().setCheckpointStorage(
                    new FileSystemCheckpointStorage(checkpointRoot.toUri().toString()));

            Properties kafkaProperties = checkpointKafkaProperties();
            PcapConsumerConfig consumerConfig = new PcapConsumerConfig(
                    brokers, PcapConsumerConfig.INPUT_TOPIC, groupId,
                    PcapConsumerConfig.DLQ_TOPIC, kafkaProperties,
                    OffsetsInitializer.earliest(), true, true,
                    "N010_N011_REVIEWED_CARRIER_SINK");
            PcapConsumerPipelineResult raw = PcapConsumerPipeline.build(env, consumerConfig);
            org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator<PcapIndexedRecord>
                    validated = raw.getIndexedRecords()
                    .process(new PcapIndexedRecordProcessFunction(PcapManifestPolicy.strictV2()))
                    .uid("pcap-manifest-validation-v2-clickhouse-integration")
                    .name("PCAP Strict Manifest Validation ClickHouse Integration");
            org.apache.flink.streaming.api.datastream.DataStream<PcapDeadLetter> manifestDlq =
                    validated.getSideOutput(PcapIndexedRecordProcessFunction.DLQ_TAG);
            manifestDlq.sinkTo(DLQSinkFactory.createDLQSink(
                            brokers, PcapConsumerConfig.DLQ_TOPIC, kafkaProperties))
                    .uid("pcap-manifest-dlq-v2-clickhouse-integration")
                    .name("PCAP Manifest DLQ ClickHouse Integration");
            validated.addSink(ClickHousePcapCarrierSinkFactory.createPcapIndexSink(
                            PcapClickHouseIntegrationConfig.ownedSingleNode(
                                    jdbcUrl, clickHouseUser, clickHousePassword, 1),
                            PcapProjectionColumns.manifestV2()))
                    .uid("pcap-clickhouse-carrier-sink-v2-integration")
                    .name("PCAP Real ClickHouse Carrier Sink Integration");
            validated.addSink(new CoordinatedFailureSink(probeRoot.toString(), meta.getTenantId()))
                    .uid("pcap-clickhouse-failure-window-v1")
                    .name("PCAP ClickHouse Failure Window");

            JobClient jobClient = null;
            try {
                jobClient = env.executeAsync("N010 Real ClickHouse Replay Convergence");
                CompletableFuture<JobExecutionResult> execution = jobClient.getJobExecutionResult();
                waitForMarker(jobClient, execution,
                        probeRoot.resolve(CoordinatedFailureSink.FIRST_ATTEMPT),
                        Duration.ofSeconds(20), "ClickHouse failure-window sink did not receive the carrier");
                waitUntil(() -> clickHouseCountUnchecked(
                                jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(), false) >= 1,
                        Duration.ofSeconds(20), "first physical ClickHouse projection was not visible");
                assertThat(committedOffset(admin, groupId, sourcePartition))
                        .isLessThanOrEqualTo(sourceOffset);
                Files.write(probeRoot.resolve(CoordinatedFailureSink.FAIL_NOW), new byte[]{1},
                        StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
                waitForMarker(jobClient, execution,
                        probeRoot.resolve(CoordinatedFailureSink.SECOND_ATTEMPT),
                        Duration.ofSeconds(30), "ClickHouse branch did not replay after the injected failure");
                waitUntil(() -> clickHouseCountUnchecked(
                                jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(), false) >= 2,
                        Duration.ofSeconds(20), "replay did not leave a detectable physical duplicate");
                Files.write(probeRoot.resolve(CoordinatedFailureSink.RELEASE), new byte[]{1},
                        StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
                waitUntil(() -> committedOffsetUnchecked(admin, groupId, sourcePartition)
                                >= sourceOffset + 1,
                        Duration.ofSeconds(30), "ClickHouse replay checkpoint did not commit source offset");

                assertThat(clickHouseCount(
                        jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(), false))
                        .isGreaterThanOrEqualTo(2);
                assertThat(clickHouseCount(
                        jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(), true))
                        .isEqualTo(1);
                assertThat(clickHouseDistinctFact(
                        jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(),
                        "projection_identity"))
                        .isEqualTo(1);
                assertThat(clickHouseDistinctFact(
                        jdbcUrl, clickHouseUser, clickHousePassword, meta.getTenantId(),
                        "kafka_offset"))
                        .isEqualTo(1);
                assertThat(clickHouseTimestampMillis(
                        jdbcUrl, clickHouseUser, clickHousePassword,
                        meta.getTenantId(), meta.getFileKey()))
                        .containsExactly(meta.getTsStart(), meta.getTsEnd(), meta.getCreatedTs());

                PcapIndexMeta late = meta.toBuilder()
                        .setFileKey("tenant-clickhouse/probe-clickhouse/late.pcap.zst")
                        .setTsStart(meta.getTsStart() - 300_000L)
                        .setTsEnd(meta.getTsStart() - 299_000L)
                        .setCreatedTs(meta.getCreatedTs() - 300_000L)
                        .build();
                long lateOffset = produceStrictV2Record(brokers, late);
                waitUntil(() -> clickHouseCountUnchecked(
                                jdbcUrl, clickHouseUser, clickHousePassword,
                                late.getTenantId(), true, late.getFileKey()) == 1,
                        Duration.ofSeconds(20), "late PCAP metadata did not reach ClickHouse");
                waitUntil(() -> committedOffsetUnchecked(admin, groupId, sourcePartition)
                                >= lateOffset + 1,
                        Duration.ofSeconds(30), "late PCAP checkpoint did not commit source offset");
            } finally {
                if (jobClient != null) {
                    try {
                        jobClient.cancel().get(10, TimeUnit.SECONDS);
                    } catch (Exception ignored) {
                        LOG.warn("N010 ClickHouse acceptance job was already terminal", ignored);
                    }
                }
            }
        }
    }

    private static JobClient startAcceptanceJob(
            String brokers, String groupId, Path probeRoot, Path checkpointRoot,
            String savepointPath) throws Exception {
        Configuration flinkConfiguration = new Configuration();
        if (savepointPath != null) {
            flinkConfiguration.set(SavepointConfigOptions.SAVEPOINT_PATH, savepointPath);
            flinkConfiguration.set(SavepointConfigOptions.SAVEPOINT_IGNORE_UNCLAIMED_STATE, false);
        }
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment(
                flinkConfiguration);
        env.setParallelism(1);
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(2, Time.milliseconds(100)));
        env.enableCheckpointing(250L, CheckpointingMode.AT_LEAST_ONCE);
        env.getCheckpointConfig().setCheckpointTimeout(10_000L);
        env.getCheckpointConfig().setMinPauseBetweenCheckpoints(50L);
        env.getCheckpointConfig().setMaxConcurrentCheckpoints(1);
        env.getCheckpointConfig().setCheckpointStorage(
                new FileSystemCheckpointStorage(checkpointRoot.toUri().toString()));

        Properties kafkaProperties = checkpointKafkaProperties();
        PcapConsumerConfig consumerConfig = new PcapConsumerConfig(
                brokers,
                PcapConsumerConfig.INPUT_TOPIC,
                groupId,
                PcapConsumerConfig.DLQ_TOPIC,
                kafkaProperties,
                OffsetsInitializer.earliest(),
                true,
                true,
                "N010_N011_REVIEWED_CARRIER_SINK");

        org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator<PcapIndexedRecord>
                validated = PcapConsumerPipeline.build(env, consumerConfig).getIndexedRecords()
                .process(new PcapIndexedRecordProcessFunction(PcapManifestPolicy.strictV2()))
                .uid("pcap-manifest-validation-v2-integration")
                .name("PCAP Strict Manifest Validation Integration");
        validated.getSideOutput(PcapIndexedRecordProcessFunction.DLQ_TAG)
                .sinkTo(DLQSinkFactory.createDLQSink(
                        brokers, PcapConsumerConfig.DLQ_TOPIC, kafkaProperties))
                .uid("pcap-manifest-dlq-v2-integration")
                .name("PCAP Manifest DLQ Integration");
        validated.addSink(new RestartBarrierSink(probeRoot.toString()))
                .uid("pcap-checkpoint-restart-probe-v1")
                .name("PCAP Checkpoint Restart Probe");
        return env.executeAsync("N010 PCAP Kafka Checkpoint Restart Acceptance");
    }

    private static Properties checkpointKafkaProperties() {
        Properties properties = new Properties();
        properties.setProperty("acks", "all");
        properties.setProperty("enable.auto.commit", "false");
        properties.setProperty("commit.offsets.on.checkpoint", "true");
        return properties;
    }

    private static long produceStrictV2Record(String brokers, PcapIndexMeta meta) throws Exception {
        return produceRawRecord(brokers, meta, meta.toByteArray());
    }

    private static long produceRawRecord(String brokers, PcapIndexMeta headerMeta,
                                         byte[] value) throws Exception {
        Properties producerProperties = new Properties();
        producerProperties.setProperty(ProducerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        producerProperties.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProperties.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProperties.setProperty(ProducerConfig.KEY_SERIALIZER_CLASS_CONFIG,
                ByteArraySerializer.class.getName());
        producerProperties.setProperty(ProducerConfig.VALUE_SERIALIZER_CLASS_CONFIG,
                ByteArraySerializer.class.getName());

        byte[] key = bytes(headerMeta.getTenantId() + ":" + headerMeta.getProbeId());
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", bytes(headerMeta.getTenantId()));
        headers.add("probe_id", bytes(headerMeta.getProbeId()));
        headers.add("file_key", bytes(headerMeta.getFileKey()));
        headers.add("sha256", bytes(headerMeta.getSha256()));
        headers.add("content_type", bytes("application/x-protobuf"));
        headers.add("proto_message_type", bytes("traffic.v1.PcapIndexMeta"));
        headers.add("proto_schema_version", bytes("v1"));
        ProducerRecord<byte[], byte[]> record = new ProducerRecord<>(
                PcapConsumerConfig.INPUT_TOPIC, 0, null, key, value, headers);
        try (KafkaProducer<byte[], byte[]> producer = new KafkaProducer<>(producerProperties)) {
            return producer.send(record).get(10, TimeUnit.SECONDS).offset();
        }
    }

    private static String consumeCanonicalDlq(String brokers, long originalOffset) throws Exception {
        Properties properties = new Properties();
        properties.setProperty(ConsumerConfig.BOOTSTRAP_SERVERS_CONFIG, brokers);
        properties.setProperty(ConsumerConfig.GROUP_ID_CONFIG, "m02-pcap-dlq-read-" + UUID.randomUUID());
        properties.setProperty(ConsumerConfig.ENABLE_AUTO_COMMIT_CONFIG, "false");
        properties.setProperty(ConsumerConfig.AUTO_OFFSET_RESET_CONFIG, "earliest");
        properties.setProperty(ConsumerConfig.KEY_DESERIALIZER_CLASS_CONFIG,
                ByteArrayDeserializer.class.getName());
        properties.setProperty(ConsumerConfig.VALUE_DESERIALIZER_CLASS_CONFIG,
                ByteArrayDeserializer.class.getName());
        TopicPartition partition = new TopicPartition(PcapConsumerConfig.DLQ_TOPIC, 0);
        try (KafkaConsumer<byte[], byte[]> consumer = new KafkaConsumer<>(properties)) {
            consumer.assign(List.of(partition));
            consumer.seekToBeginning(List.of(partition));
            long deadline = System.nanoTime() + Duration.ofSeconds(20).toNanos();
            while (System.nanoTime() < deadline) {
                ConsumerRecords<byte[], byte[]> records = consumer.poll(Duration.ofMillis(100));
                for (org.apache.kafka.clients.consumer.ConsumerRecord<byte[], byte[]> record : records) {
                    org.apache.kafka.common.header.Header sourceOffset =
                            record.headers().lastHeader("source_offset");
                    if (sourceOffset != null && Long.parseLong(
                            new String(sourceOffset.value(), StandardCharsets.UTF_8)) == originalOffset) {
                        return new String(record.value(), StandardCharsets.UTF_8);
                    }
                }
            }
        }
        fail("canonical DLQ did not retain source offset " + originalOffset);
        return "";
    }

    private static long clickHouseCount(String jdbcUrl, String user, String password,
                                        String tenant, boolean finalView) throws Exception {
        return clickHouseCount(jdbcUrl, user, password, tenant, finalView, null);
    }

    private static long clickHouseCount(String jdbcUrl, String user, String password,
                                        String tenant, boolean finalView, String fileKey)
            throws Exception {
        String sql = "SELECT count() FROM traffic.pcap_index_v2" + (finalView ? " FINAL" : "")
                + " WHERE tenant_id=?" + (fileKey == null ? "" : " AND file_key=?");
        try (Connection connection = DriverManager.getConnection(jdbcUrl, user, password);
             PreparedStatement statement = connection.prepareStatement(sql)) {
            statement.setString(1, tenant);
            if (fileKey != null) statement.setString(2, fileKey);
            try (ResultSet result = statement.executeQuery()) {
                assertThat(result.next()).isTrue();
                return result.getLong(1);
            }
        }
    }

    private static long clickHouseCountUnchecked(String jdbcUrl, String user, String password,
                                                 String tenant, boolean finalView) {
        return clickHouseCountUnchecked(jdbcUrl, user, password, tenant, finalView, null);
    }

    private static long clickHouseCountUnchecked(String jdbcUrl, String user, String password,
                                                 String tenant, boolean finalView,
                                                 String fileKey) {
        try {
            return clickHouseCount(jdbcUrl, user, password, tenant, finalView, fileKey);
        } catch (Exception ignored) {
            return -1L;
        }
    }

    private static long clickHouseDistinctFact(String jdbcUrl, String user, String password,
                                               String tenant, String column) throws Exception {
        if (!"projection_identity".equals(column) && !"kafka_offset".equals(column)) {
            throw new IllegalArgumentException("unapproved ClickHouse fact column");
        }
        try (Connection connection = DriverManager.getConnection(jdbcUrl, user, password);
             PreparedStatement statement = connection.prepareStatement(
                     "SELECT uniqExact(" + column + ") FROM traffic.pcap_index_v2 WHERE tenant_id=?")) {
            statement.setString(1, tenant);
            try (ResultSet result = statement.executeQuery()) {
                assertThat(result.next()).isTrue();
                return result.getLong(1);
            }
        }
    }

    private static List<Long> clickHouseTimestampMillis(
            String jdbcUrl, String user, String password, String tenant, String fileKey)
            throws Exception {
        try (Connection connection = DriverManager.getConnection(jdbcUrl, user, password);
             PreparedStatement statement = connection.prepareStatement(
                     "SELECT toUnixTimestamp64Milli(ts_start),toUnixTimestamp64Milli(ts_end)," +
                             "toUnixTimestamp64Milli(created_ts) FROM traffic.pcap_index_v2 FINAL " +
                             "WHERE tenant_id=? AND file_key=?")) {
            statement.setString(1, tenant);
            statement.setString(2, fileKey);
            try (ResultSet result = statement.executeQuery()) {
                assertThat(result.next()).isTrue();
                List<Long> values = List.of(result.getLong(1), result.getLong(2), result.getLong(3));
                assertThat(result.next()).isFalse();
                return values;
            }
        }
    }

    private static void createCanonicalTopicsOnEmptyBroker(AdminClient admin) throws Exception {
        try {
            admin.createTopics(Arrays.asList(
                    new NewTopic(PcapConsumerConfig.INPUT_TOPIC, 1, (short) 1),
                    new NewTopic(PcapConsumerConfig.DLQ_TOPIC, 1, (short) 1)))
                    .all().get(10, TimeUnit.SECONDS);
        } catch (ExecutionException error) {
            fail("N010 integration requires an empty owned broker; canonical topic creation failed",
                    error.getCause());
        }
    }

    private static long committedOffset(AdminClient admin, String groupId,
                                        TopicPartition partition) throws Exception {
        Map<TopicPartition, OffsetAndMetadata> offsets = admin.listConsumerGroupOffsets(groupId)
                .partitionsToOffsetAndMetadata().get(10, TimeUnit.SECONDS);
        OffsetAndMetadata committed = offsets.get(partition);
        return committed == null ? -1L : committed.offset();
    }

    private static long committedOffsetUnchecked(AdminClient admin, String groupId,
                                                 TopicPartition partition) {
        try {
            return committedOffset(admin, groupId, partition);
        } catch (Exception ignored) {
            return -1L;
        }
    }

    private static void waitUntil(BooleanSupplier condition, Duration timeout, String failure)
            throws Exception {
        long deadline = System.nanoTime() + timeout.toNanos();
        while (System.nanoTime() < deadline) {
            if (condition.getAsBoolean()) return;
            Thread.sleep(25L);
        }
        fail(failure);
    }

    private static void waitForMarker(JobClient jobClient,
                                      CompletableFuture<JobExecutionResult> executionResult,
                                      Path marker, Duration timeout, String failure) throws Exception {
        long deadline = System.nanoTime() + timeout.toNanos();
        while (System.nanoTime() < deadline) {
            if (Files.exists(marker)) return;
            if (executionResult.isDone()) {
                try {
                    executionResult.get(5, TimeUnit.SECONDS);
                } catch (ExecutionException terminal) {
                    throw new AssertionError(failure + "; job terminated", terminal.getCause());
                }
                fail(failure + "; job completed before the marker");
            }
            Thread.sleep(25L);
        }
        String status;
        try {
            status = jobClient.getJobStatus().get(5, TimeUnit.SECONDS).toString();
        } catch (Exception unavailable) {
            status = "UNAVAILABLE";
        }
        fail(failure + "; job status=" + status);
    }

    private static void waitForContent(JobClient jobClient,
                                       CompletableFuture<JobExecutionResult> executionResult,
                                       Path file, String prefix, Duration timeout,
                                       String failure) throws Exception {
        long deadline = System.nanoTime() + timeout.toNanos();
        while (System.nanoTime() < deadline) {
            if (Files.exists(file) && Files.readAllLines(file, StandardCharsets.UTF_8)
                    .stream().anyMatch(line -> line.startsWith(prefix))) {
                return;
            }
            if (executionResult.isDone()) {
                try {
                    executionResult.get(5, TimeUnit.SECONDS);
                } catch (ExecutionException terminal) {
                    throw new AssertionError(failure + "; job terminated", terminal.getCause());
                }
                fail(failure + "; job completed before the expected output");
            }
            Thread.sleep(25L);
        }
        String status;
        try {
            status = jobClient.getJobStatus().get(5, TimeUnit.SECONDS).toString();
        } catch (Exception unavailable) {
            status = "UNAVAILABLE";
        }
        fail(failure + "; job status=" + status);
    }

    private static String requiredEnvironment(String name) {
        String value = System.getenv(name);
        if (value == null || value.trim().isEmpty()) fail(name + " is required");
        return value.trim();
    }

    private static byte[] bytes(String value) {
        return value.getBytes(StandardCharsets.UTF_8);
    }

    /**
     * External-file controlled sink so failure state survives Flink task re-instantiation and
     * remains observable outside the user-code classloader. This is test-only failure injection.
     */
    public static final class RestartBarrierSink extends RichSinkFunction<PcapIndexedRecord> {
        private static final long serialVersionUID = 1L;
        static final String ATTEMPTS = "attempts.log";
        static final String FIRST_FAILURE = "first-failure.marker";
        static final String SECOND_ATTEMPT = "second-attempt.marker";
        static final String RELEASE = "release.marker";
        static final String SUCCESSES = "successes.log";

        private final String root;

        RestartBarrierSink(String root) {
            this.root = root;
        }

        @Override
        public void invoke(PcapIndexedRecord value, Context context) throws Exception {
            Path rootPath = Path.of(root);
            String sourceTuple = value.getTopic() + ":" + value.getPartition() + ":" + value.getOffset();
            String attempt = sourceTuple + "|" + value.getProjectionIdentity() + System.lineSeparator();
            Files.write(rootPath.resolve(ATTEMPTS), attempt.getBytes(StandardCharsets.UTF_8),
                    StandardOpenOption.CREATE, StandardOpenOption.APPEND, StandardOpenOption.WRITE);

            try {
                Files.write(rootPath.resolve(FIRST_FAILURE), new byte[]{1},
                        StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
                throw new IllegalStateException("N010_INJECTED_SINK_FAILURE_BEFORE_CHECKPOINT");
            } catch (FileAlreadyExistsException replay) {
                Files.write(rootPath.resolve(SECOND_ATTEMPT), new byte[]{1},
                        StandardOpenOption.CREATE, StandardOpenOption.WRITE);
            }

            long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
            while (!Files.exists(rootPath.resolve(RELEASE))) {
                if (System.nanoTime() >= deadline) {
                    throw new IllegalStateException("N010_REPLAY_RELEASE_TIMEOUT");
                }
                Thread.sleep(25L);
            }
            Files.write(rootPath.resolve(SUCCESSES), attempt.getBytes(StandardCharsets.UTF_8),
                    StandardOpenOption.CREATE, StandardOpenOption.APPEND, StandardOpenOption.WRITE);
        }
    }

    public static final class CoordinatedFailureSink extends RichSinkFunction<PcapIndexedRecord> {
        private static final long serialVersionUID = 1L;
        static final String FIRST_ATTEMPT = "clickhouse-first-attempt.marker";
        static final String FAIL_NOW = "clickhouse-fail-now.marker";
        static final String SECOND_ATTEMPT = "clickhouse-second-attempt.marker";
        static final String RELEASE = "clickhouse-release.marker";

        private final String root;
        private final String targetTenant;

        CoordinatedFailureSink(String root, String targetTenant) {
            this.root = root;
            this.targetTenant = targetTenant;
        }

        @Override
        public void invoke(PcapIndexedRecord value, Context context) throws Exception {
            if (!targetTenant.equals(value.getMeta().getTenantId())) return;
            Path rootPath = Path.of(root);
            try {
                Files.write(rootPath.resolve(FIRST_ATTEMPT), new byte[]{1},
                        StandardOpenOption.CREATE_NEW, StandardOpenOption.WRITE);
                waitForControl(rootPath.resolve(FAIL_NOW));
                throw new IllegalStateException("N010_CLICKHOUSE_ACK_BEFORE_CHECKPOINT_FAILURE");
            } catch (FileAlreadyExistsException replay) {
                Files.write(rootPath.resolve(SECOND_ATTEMPT), new byte[]{1},
                        StandardOpenOption.CREATE, StandardOpenOption.WRITE);
                waitForControl(rootPath.resolve(RELEASE));
            }
        }

        private static void waitForControl(Path marker) throws Exception {
            long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
            while (!Files.exists(marker)) {
                if (System.nanoTime() >= deadline) {
                    throw new IllegalStateException("N010_CLICKHOUSE_FAILURE_CONTROL_TIMEOUT");
                }
                Thread.sleep(25L);
            }
        }
    }
}
