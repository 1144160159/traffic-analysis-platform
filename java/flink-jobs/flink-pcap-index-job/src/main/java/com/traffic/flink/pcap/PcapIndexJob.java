package com.traffic.flink.pcap;

import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.KafkaStartingOffsets;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.pcap.process.PcapIndexProcessFunction;
import com.traffic.flink.pcap.process.PcapIndexedRecordProcessFunction;
import com.traffic.flink.pcap.process.PcapManifestPolicy;
import com.traffic.flink.pcap.sink.ClickHousePcapSinkFactory;
import com.traffic.flink.pcap.sink.ClickHousePcapCarrierSinkFactory;
import com.traffic.flink.pcap.sink.DLQSinkFactory;
import com.traffic.flink.pcap.sink.PcapClickHouseConfig;
import com.traffic.flink.pcap.sink.PcapClickHouseSchemaAttestor;
import com.traffic.flink.pcap.sink.PcapProjectionColumns;
import com.traffic.flink.pcap.source.PcapConsumerConfig;
import com.traffic.flink.pcap.source.PcapConsumerPipeline;
import com.traffic.flink.pcap.source.PcapConsumerPipelineResult;
import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Duration;
import java.util.List;
import java.util.Properties;

/**
 * Flink PCAP Index Job (修复版 v2)
 * 
 * 修复内容：
 * 1. 使用 PcapIndexProcessFunction 进行完整业务验证
 * 2. 连接 DLQ Sink 处理无效数据
 * 3. 增加配置参数校验与日志
 * 4. 优化 Checkpoint 配置
 * 
 * 输入: pcap.index.v1 (Kafka)
 * 输出: 
 *   - pcap_index_v2 (carrier graph) or pcap_index (explicit legacy rollback graph)
 *   - dlq.pcap-index-job (Kafka DLQ)
 */
public class PcapIndexJob {

    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexJob.class);
    private static final String JOB_NAME = "PCAP Index Job v2";
    static final String MANIFEST_UID = "pcap-manifest-validate-v2";
    static final String MANIFEST_DLQ_UID = "pcap-manifest-canonical-dlq-v2";
    static final String CLICKHOUSE_CARRIER_UID = "pcap-clickhouse-carrier-sink-v2";

    public static void main(String[] args) throws Exception {
        LOG.info("========================================");
        LOG.info("Starting {} ...", JOB_NAME);
        LOG.info("========================================");

        // ==================== 1. 加载配置 ====================
        ParameterTool params = ConfigUtils.loadConfig(args, "pcap-index-job.properties");
        validateConfig(params);

        // Kafka 配置
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", "pcap.index.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-pcap-index-job");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", PcapConsumerConfig.DLQ_TOPIC);

        // ClickHouse 配置
        String clickhouseUrl = ConfigUtils.get(params, "clickhouse.url", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123");
        String clickhouseDatabase = ConfigUtils.get(params, "clickhouse.database", "traffic");
        String clickhouseTable = ConfigUtils.get(params, "clickhouse.table", "pcap_index");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        // Checkpoint 配置
        String checkpointPath = ConfigUtils.get(params, "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/pcap-index-job");
        long checkpointInterval = ConfigUtils.getLong(params, "checkpoint.interval.ms", 30000);

        // 作业配置
        int parallelism = ConfigUtils.getInt(params, "parallelism", 2);
        int watermarkDelaySeconds = ConfigUtils.getInt(params, "watermark.delay.seconds", 5);

        // 验证配置
        long maxFileSizeGB = ConfigUtils.getLong(params, "validation.max.file.size.gb", 10);
        long maxTimeRangeHours = ConfigUtils.getLong(params, "validation.max.time.range.hours", 1);

        // 调试配置
        boolean debugPrint = ConfigUtils.getBoolean(params, "debug.print", false);

        // ==================== 2. 创建执行环境 ====================
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);

        if (ConfigUtils.getBoolean(params, "pcap.carrier.enabled", false)) {
            runCarrierGraph(params, env, kafkaBrokers, inputTopic, groupId,
                    clickhouseUrl, clickhouseDatabase, clickhouseTable,
                    clickhouseUser, clickhousePassword, checkpointPath,
                    checkpointInterval);
            return;
        }

        // 配置 Checkpoint
        configureLegacyCheckpoint(env, checkpointPath, checkpointInterval);

        // 配置重启策略。默认覆盖短时 Kafka/存储故障窗口，仍允许通过合同化参数调整。
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                ConfigUtils.getInt(params, "restart.attempts", 10),
                org.apache.flink.api.common.time.Time.seconds(
                        ConfigUtils.getInt(params, "restart.delay.seconds", 30))
        ));

        // ==================== 3. Kafka Source ====================
        KafkaSource<PcapIndexMeta> source = KafkaSource.<PcapIndexMeta>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(KafkaStartingOffsets.from(params))
                .setValueOnlyDeserializer(new ProtoDeserializer<>(PcapIndexMeta.class))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .setProperty("max.poll.records", "1000") // 批量拉取
                .build();

        // Watermark 策略（允许 5 秒乱序）
        WatermarkStrategy<PcapIndexMeta> watermarkStrategy = WatermarkStrategy
                .<PcapIndexMeta>forBoundedOutOfOrderness(Duration.ofSeconds(watermarkDelaySeconds))
                .withTimestampAssigner((meta, timestamp) -> meta.getTsStart())
                .withIdleness(Duration.ofMinutes(1)); // 空闲 1 分钟后不等待 Watermark

        // ==================== 4. 数据流处理 ====================
        DataStream<PcapIndexMeta> indexStream = env
                .fromSource(source, watermarkStrategy, "Kafka-PCAP-Index-Source")
                .uid("pcap-index-source")
                .name("PCAP Index Source");

        // 基础过滤（必要字段检查）
        DataStream<PcapIndexMeta> filteredStream = indexStream
                .filter(meta -> meta != null && 
                        meta.getTenantId() != null && !meta.getTenantId().isEmpty() &&
                        meta.getProbeId() != null && !meta.getProbeId().isEmpty() &&
                        meta.getFileKey() != null && !meta.getFileKey().isEmpty() &&
                        meta.getTsStart() > 0 &&
                        meta.getTsEnd() >= meta.getTsStart())
                .uid("filter-invalid-basic")
                .name("Filter Invalid Basic");

        // ==================== 5. 业务验证处理（核心）====================
        SingleOutputStreamOperator<PcapIndexMeta> processedStream = filteredStream
                .process(new PcapIndexProcessFunction(
                        maxFileSizeGB * 1024 * 1024 * 1024L, // GB -> Bytes
                        maxTimeRangeHours * 3600 * 1000L      // Hours -> ms
                ))
                .uid("pcap-index-processor")
                .name("PCAP Index Processor");

        // ==================== 6. ClickHouse Sink ====================
        processedStream.addSink(
                ClickHousePcapSinkFactory.createPcapIndexSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseTable,
                        clickhouseUser,
                        clickhousePassword
                )
        ).uid("clickhouse-sink").name("ClickHouse PCAP Index Sink");

        // ==================== 7. DLQ Sink（侧输出）====================
        DataStream<String> dlqStream = processedStream.getSideOutput(
                PcapIndexProcessFunction.DLQ_TAG
        );

        dlqStream.sinkTo(
                DLQSinkFactory.createDLQSink(kafkaBrokers, dlqTopic)
        ).uid("dlq-sink").name("DLQ Kafka Sink");

        // ==================== 8. 调试输出（可选）====================
        if (debugPrint) {
            processedStream.print("PCAP-Index").uid("print-sink");
            dlqStream.print("DLQ").uid("print-dlq");
        }

        // ==================== 9. 打印配置摘要 ====================
        LOG.info("========================================");
        LOG.info("Job Configuration:");
        LOG.info("  Input Topic: {}", inputTopic);
        LOG.info("  DLQ Topic: {}", dlqTopic);
        LOG.info("  ClickHouse: {}/{}.{}", clickhouseUrl, clickhouseDatabase, clickhouseTable);
        LOG.info("  Parallelism: {}", parallelism);
        LOG.info("  Checkpoint Interval: {} ms", checkpointInterval);
        LOG.info("  Watermark Delay: {} s", watermarkDelaySeconds);
        LOG.info("  Max File Size: {} GB", maxFileSizeGB);
        LOG.info("  Max Time Range: {} hours", maxTimeRangeHours);
        LOG.info("========================================");

        // ==================== 10. 执行作业 ====================
        env.execute(JOB_NAME);
    }

    /**
     * 配置 Checkpoint
     */
    private static void configureLegacyCheckpoint(
            StreamExecutionEnvironment env,
            String checkpointPath,
            long intervalMs
    ) {
        // PCAP 索引作业使用 AT_LEAST_ONCE 模式（幂等写入，无需 EXACTLY_ONCE）
        env.enableCheckpointing(intervalMs, CheckpointingMode.AT_LEAST_ONCE);

        CheckpointConfig config = env.getCheckpointConfig();
        
        // Checkpoint 超时时间（1 分钟）
        config.setCheckpointTimeout(60000);
        
        // Checkpoint 间最小暂停时间（防止频繁 Checkpoint）
        config.setMinPauseBetweenCheckpoints(intervalMs / 2);
        
        // 最大并发 Checkpoint 数量
        config.setMaxConcurrentCheckpoints(1);
        
        // 作业取消时保留 Checkpoint
        config.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION
        );

        // 允许的连续 Checkpoint 失败次数
        config.setTolerableCheckpointFailureNumber(3);

        // 使用 RocksDB State Backend（适合大状态）
        EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);
        env.setStateBackend(stateBackend);
        
        // 设置 Checkpoint 存储路径
        config.setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));

        LOG.info("Checkpoint configured: interval={} ms, path={}", intervalMs, checkpointPath);
    }

    static PcapCheckpointContract configureCheckpoint(
            StreamExecutionEnvironment env, PcapCheckpointConfig checkpoint) {
        if (env == null || checkpoint == null) {
            throw new IllegalArgumentException("PCAP environment and checkpoint contract are required");
        }
        checkpoint.validate();
        env.enableCheckpointing(checkpoint.getIntervalMs(), checkpoint.getMode());
        CheckpointConfig effective = env.getCheckpointConfig();
        effective.setCheckpointTimeout(checkpoint.getTimeoutMs());
        effective.setMinPauseBetweenCheckpoints(checkpoint.getMinPauseMs());
        effective.setMaxConcurrentCheckpoints(1);
        effective.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION);
        effective.setTolerableCheckpointFailureNumber(checkpoint.getTolerableFailures());
        env.setStateBackend(new EmbeddedRocksDBStateBackend(true));
        effective.setCheckpointStorage(new FileSystemCheckpointStorage(checkpoint.getStorageUri()));
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                checkpoint.getRestartAttempts(),
                org.apache.flink.api.common.time.Time.milliseconds(checkpoint.getRestartDelayMs())));
        return new PcapCheckpointContract(checkpoint);
    }

    private static void runCarrierGraph(
            ParameterTool params, StreamExecutionEnvironment env, String kafkaBrokers,
            String inputTopic, String groupId, String clickhouseUrl, String clickhouseDatabase,
            String clickhouseTable, String clickhouseUser, String clickhousePassword,
            String checkpointPath, long checkpointInterval) throws Exception {
        String canonicalDlq = ConfigUtils.get(params, "kafka.canonical.dlq.topic", PcapConsumerConfig.DLQ_TOPIC);
        Properties kafkaProperties = ConfigUtils.kafkaClientProperties(params);
        List<String> operatorUids = List.of(
                PcapConsumerConfig.SOURCE_UID, PcapConsumerConfig.PARSE_UID,
                PcapConsumerConfig.DLQ_UID, MANIFEST_UID, MANIFEST_DLQ_UID,
                CLICKHOUSE_CARRIER_UID);
        PcapCheckpointConfig checkpoint = new PcapCheckpointConfig(
                checkpointPath, checkpointInterval,
                ConfigUtils.getLong(params, "checkpoint.timeout.ms", 60_000),
                ConfigUtils.getLong(params, "checkpoint.min.pause.ms", checkpointInterval / 2),
                ConfigUtils.getInt(params, "checkpoint.tolerable.failures", 3),
                ConfigUtils.getInt(params, "restart.attempts", 10),
                ConfigUtils.getLong(params, "restart.delay.seconds", 30) * 1000,
                operatorUids);

        PcapProjectionColumns columns = PcapProjectionColumns.manifestV2();
        String jdbcUrl = clickhouseUrl.startsWith("jdbc:clickhouse://")
                ? clickhouseUrl : "jdbc:clickhouse://" + clickhouseUrl + "/" + clickhouseDatabase;
        PcapClickHouseConfig clickhouse = PcapClickHouseSchemaAttestor.attest(
                jdbcUrl, clickhouseDatabase, clickhouseTable,
                ConfigUtils.get(params, "clickhouse.local.table", "pcap_index_v2_local"),
                clickhouseUser, clickhousePassword, columns,
                ConfigUtils.getInt(params, "clickhouse.batch.size", 1000),
                ConfigUtils.getLong(params, "clickhouse.batch.interval.ms", 2000),
                ConfigUtils.getInt(params, "clickhouse.max.retries", 3));

        PcapCheckpointContract checkpointContract = configureCheckpoint(env, checkpoint);
        PcapConsumerConfig consumer = new PcapConsumerConfig(
                kafkaBrokers, inputTopic, groupId, canonicalDlq, kafkaProperties,
                KafkaStartingOffsets.from(params),
                ConfigUtils.getBoolean(params, "pcap.kafka.dlq.acl.attested", false),
                ConfigUtils.getBoolean(params, "pcap.kafka.idempotent.acl.attested", false),
                "N010_N011_REVIEWED_CARRIER_SINK");
        PcapConsumerPipelineResult raw = PcapConsumerPipeline.build(env, consumer);
        SingleOutputStreamOperator<PcapIndexedRecord> validated = raw.getIndexedRecords()
                .process(new PcapIndexedRecordProcessFunction(PcapManifestPolicy.strictV2()))
                .uid(MANIFEST_UID)
                .name("PCAP Manifest V2 Validate");
        DataStream<PcapDeadLetter> manifestDlq = validated.getSideOutput(
                PcapIndexedRecordProcessFunction.DLQ_TAG);
        manifestDlq.sinkTo(DLQSinkFactory.createDLQSink(
                        kafkaBrokers, canonicalDlq, kafkaProperties))
                .uid(MANIFEST_DLQ_UID)
                .name("PCAP Manifest Canonical DLQ Sink");
        validated.addSink(ClickHousePcapCarrierSinkFactory.createPcapIndexSink(clickhouse, columns))
                .uid(CLICKHOUSE_CARRIER_UID)
                .name("ClickHouse PCAP Carrier Sink");

        LOG.info("Submitting carrier graph: checkpoint_contract={}, column_contract={}, uids={}",
                checkpointContract.getDigest(), columns.digest(), operatorUids);
        env.execute("PCAP Index Carrier Job v2");
    }

    /**
     * 校验必要配置参数
     */
    static void validateConfig(ParameterTool params) {
        String[] requiredKeys = {
                "kafka.brokers",
                "kafka.input.topic",
                "kafka.group.id",
                "clickhouse.url",
                "clickhouse.database",
                "clickhouse.table",
                "checkpoint.path"
        };

        for (String key : requiredKeys) {
            if (!params.has(key) || params.get(key).isEmpty()) {
                throw new IllegalArgumentException(
                        String.format("Required configuration missing: %s", key)
                );
            }
        }

        if (!PcapConsumerConfig.INPUT_TOPIC.equals(params.get("kafka.input.topic"))) {
            throw new IllegalArgumentException("PCAP source topic must be canonical pcap.index.v1");
        }
        boolean carrierEnabled = params.getBoolean("pcap.carrier.enabled", false);
        String table = params.get("clickhouse.table");
        boolean v2Table = "pcap_index_v2".equals(table) || "traffic.pcap_index_v2".equals(table);
        boolean legacyTable = "pcap_index".equals(table) || "traffic.pcap_index".equals(table)
                || "pcap_index_local".equals(table) || "traffic.pcap_index_local".equals(table);
        if ((carrierEnabled && !v2Table) || (!carrierEnabled && !legacyTable)) {
            throw new IllegalArgumentException(
                    "PCAP ClickHouse table does not match the selected carrier or legacy graph");
        }
        String canonicalDlq = params.get("kafka.canonical.dlq.topic", PcapConsumerConfig.DLQ_TOPIC);
        String legacyDlq = params.get("kafka.dlq.topic", PcapConsumerConfig.DLQ_TOPIC);
        if (carrierEnabled) {
            if (!PcapConsumerConfig.DLQ_TOPIC.equals(canonicalDlq)) {
                throw new IllegalArgumentException("PCAP carrier graph requires canonical dlq.v1");
            }
            if (!params.getBoolean("pcap.kafka.dlq.acl.attested", false)
                    || !params.getBoolean("pcap.kafka.idempotent.acl.attested", false)) {
                throw new IllegalArgumentException("PCAP carrier graph requires canonical DLQ ACL attestations");
            }
        } else if (!PcapConsumerConfig.DLQ_TOPIC.equals(legacyDlq)) {
            if (!"dlq.pcap-index-job".equals(legacyDlq)
                    || !params.getBoolean("pcap.legacy.private.dlq.compatibility.enabled", false)) {
                throw new IllegalArgumentException(
                        "private PCAP DLQ requires the explicit legacy compatibility guard");
            }
        } else {
            throw new IllegalArgumentException(
                    "legacy value-only graph cannot write non-canonical payloads to dlq.v1");
        }

        Properties kafka = ConfigUtils.kafkaClientProperties(params);
        String autoCommit = params.get("enable.auto.commit", "false");
        String commitOnCheckpoint = params.get("commit.offsets.on.checkpoint", "true");
        if (!"false".equalsIgnoreCase(autoCommit)
                || !"true".equalsIgnoreCase(commitOnCheckpoint)) {
            throw new IllegalArgumentException(
                    "PCAP source offsets must commit only on successful checkpoints");
        }
        kafka.setProperty("enable.auto.commit", autoCommit);
        kafka.setProperty("commit.offsets.on.checkpoint", commitOnCheckpoint);

        List<String> operatorUids = List.of(
                PcapConsumerConfig.SOURCE_UID, PcapConsumerConfig.PARSE_UID,
                PcapConsumerConfig.DLQ_UID, MANIFEST_UID, MANIFEST_DLQ_UID,
                CLICKHOUSE_CARRIER_UID);
        new PcapCheckpointConfig(
                params.get("checkpoint.path"),
                requireLong(params, "checkpoint.interval.ms"),
                requireLong(params, "checkpoint.timeout.ms"),
                requireLong(params, "checkpoint.min.pause.ms"),
                requireInt(params, "checkpoint.tolerable.failures"),
                requireInt(params, "restart.attempts"),
                Math.multiplyExact(requireLong(params, "restart.delay.seconds"), 1000L),
                operatorUids);

        LOG.info("Configuration validation passed");
    }

    private static long requireLong(ParameterTool params, String key) {
        String value = params.get(key);
        if (value == null || value.trim().isEmpty()) {
            throw new IllegalArgumentException("Required configuration missing: " + key);
        }
        try {
            return Long.parseLong(value);
        } catch (NumberFormatException error) {
            throw new IllegalArgumentException("Invalid long configuration: " + key, error);
        }
    }

    private static int requireInt(ParameterTool params, String key) {
        long value = requireLong(params, key);
        if (value < Integer.MIN_VALUE || value > Integer.MAX_VALUE) {
            throw new IllegalArgumentException("Integer configuration is out of range: " + key);
        }
        return (int) value;
    }
}
