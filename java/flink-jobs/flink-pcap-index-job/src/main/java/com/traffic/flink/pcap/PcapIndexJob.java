package com.traffic.flink.pcap;

import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.pcap.process.PcapIndexProcessFunction;
import com.traffic.flink.pcap.sink.ClickHousePcapSinkFactory;
import com.traffic.flink.pcap.sink.DLQSinkFactory;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
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
 *   - pcap_index_local (ClickHouse)
 *   - dlq.pcap-index-job (Kafka DLQ)
 */
public class PcapIndexJob {

    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexJob.class);
    private static final String JOB_NAME = "PCAP Index Job v2";

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
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.pcap-index-job");

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

        // 配置 Checkpoint
        configureCheckpoint(env, checkpointPath, checkpointInterval);

        // 配置重启策略（固定延迟重启，最多 3 次）
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                3, // 最大重启次数
                org.apache.flink.api.common.time.Time.seconds(10) // 重启延迟
        ));

        // ==================== 3. Kafka Source ====================
        KafkaSource<PcapIndexMeta> source = KafkaSource.<PcapIndexMeta>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.latest())
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
    private static void configureCheckpoint(
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

    /**
     * 校验必要配置参数
     */
    private static void validateConfig(ParameterTool params) {
        String[] requiredKeys = {
                "kafka.brokers",
                "kafka.input.topic",
                "clickhouse.url",
                "clickhouse.database",
                "clickhouse.table"
        };

        for (String key : requiredKeys) {
            if (!params.has(key) || params.get(key).isEmpty()) {
                throw new IllegalArgumentException(
                        String.format("Required configuration missing: %s", key)
                );
            }
        }

        LOG.info("Configuration validation passed");
    }
}
