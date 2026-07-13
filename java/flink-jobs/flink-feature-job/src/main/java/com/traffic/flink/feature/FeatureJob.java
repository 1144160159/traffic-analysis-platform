package com.traffic.flink.feature;

import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.feature.config.FeatureSetConfig;
import com.traffic.flink.feature.config.TenantConfig;
import com.traffic.flink.feature.processor.FeatureProcessFunctionV3;
import com.traffic.flink.feature.sink.ClickHouseSinkFactory;
import com.traffic.flink.feature.sink.DLQSinkFactory;
import com.traffic.flink.feature.sink.KafkaSinkFactory;
import com.traffic.flink.feature.source.FeatureSetConfigSource;
import com.traffic.flink.feature.source.TenantConfigSource;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.SessionEvent;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.BroadcastStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Duration;

/**
 * Flink Feature Extraction Job（完整增强版 v3）
 * 
 * 增强内容（P2）：
 * 1. ✅ Feature Set 动态加载（BroadcastState）
 * 2. ✅ 租户级配置支持（BroadcastState）
 * 3. ✅ L2 候选触发机制
 * 4. ✅ 降级逻辑（租户优先级 + Backpressure 检测）
 * 
 * 输入:
 *   - session.events.v1 (Kafka)
 *   - feature_sets (PostgreSQL)
 *   - tenant_config (PostgreSQL)
 * 
 * 输出:
 *   - feature.stat.v1 (Kafka)
 *   - feature_stat_local (ClickHouse)
 *   - dlq.feature-job (Kafka, 错误记录)
 *   - l2-trigger (侧输出, 候选 L2 Session)
 */
public class FeatureJob {

    private static final Logger LOG = LoggerFactory.getLogger(FeatureJob.class);

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Feature Extraction Job v3 (Full Enhanced Version)...");

        // ==================== 加载配置 ====================
        ParameterTool params = ConfigUtils.loadConfig(args, "feature-job.properties");

        // Kafka 配置
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", "session.events.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "feature.stat.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.feature-job");
        String l2TriggerTopic = ConfigUtils.get(params, "kafka.l2.trigger.topic", "l2.trigger.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-feature-job");

        // PostgreSQL 配置
        String postgresUrl = ConfigUtils.get(params, "postgres.url", "jdbc:postgresql://postgres-primary.databases.svc:5432/traffic_platform");
        String postgresUser = ConfigUtils.get(params, "postgres.user", "postgres");
        String postgresPassword = ConfigUtils.get(params, "postgres.password", "");
        long configPollIntervalMs = ConfigUtils.getLong(params, "config.poll.interval.ms", 30000);

        // ClickHouse 配置
        String clickhouseUrl = ConfigUtils.get(params, "clickhouse.url", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123");
        String clickhouseDatabase = ConfigUtils.get(params, "clickhouse.database", "traffic");
        String clickhouseTable = ConfigUtils.get(params, "clickhouse.table", "feature_stat");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        // Checkpoint 配置
        String checkpointPath = ConfigUtils.get(
                params,
                "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/feature-job");
        long checkpointInterval = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60000);

        // 作业配置
        int parallelism = ConfigUtils.getInt(params, "parallelism", 4);
        int watermarkDelaySeconds = ConfigUtils.getInt(params, "watermark.delay.seconds", 10);

        // 降级配置
        boolean enableSampling = ConfigUtils.getBoolean(params, "degradation.sampling.enabled", false);
        float samplingRate = ConfigUtils.getFloat(params, "degradation.sampling.rate", 1.0f);

        // 打印配置摘要
        ConfigUtils.logConfigSummary(params, "Feature Extraction Job v3");

        // ==================== 创建执行环境 ====================
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);

        // 配置 Checkpoint
        configureCheckpoint(env, checkpointPath, checkpointInterval);

        // 配置重启策略
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                3,
                org.apache.flink.api.common.time.Time.seconds(30)
        ));

        // ==================== Kafka Source ====================
        KafkaSource<SessionEvent> sessionSource = KafkaSource.<SessionEvent>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.latest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(SessionEvent.class, true, true))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .setProperty("max.poll.records", "1000")
                .build();

        WatermarkStrategy<SessionEvent> watermarkStrategy = WatermarkStrategy
                .<SessionEvent>forBoundedOutOfOrderness(Duration.ofSeconds(watermarkDelaySeconds))
                .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<SessionEvent> sessionStream = env
                .fromSource(sessionSource, watermarkStrategy, "Kafka-Session-Source")
                .uid("session-source")
                .name("Session Events Source");

        // ==================== PostgreSQL Config Sources ====================
        
        // Feature Set 配置源
        DataStream<FeatureSetConfig> featureSetConfigStream = env
                .addSource(new FeatureSetConfigSource(
                        postgresUrl,
                        postgresUser,
                        postgresPassword,
                        configPollIntervalMs
                ))
                .uid("feature-set-config-source")
                .name("Feature Set Config Source")
                .setParallelism(1); // 单并行度

        // Tenant 配置源
        DataStream<TenantConfig> tenantConfigStream = env
                .addSource(new TenantConfigSource(
                        postgresUrl,
                        postgresUser,
                        postgresPassword,
                        configPollIntervalMs
                ))
                .uid("tenant-config-source")
                .name("Tenant Config Source")
                .setParallelism(1); // 单并行度

        // ==================== BroadcastStream ====================
        
        // 将两条配置流都映射为 Object 再合并，以便广播统一的 BroadcastStream<Object>
        DataStream<Object> featureSetAsObject = featureSetConfigStream.map(config -> (Object) config);
        DataStream<Object> tenantConfigAsObject = tenantConfigStream.map(config -> (Object) config);

        BroadcastStream<Object> configBroadcastStream = featureSetAsObject
                .union(tenantConfigAsObject)
                .broadcast(
                        FeatureProcessFunctionV3.FEATURE_SET_STATE_DESC,
                        FeatureProcessFunctionV3.TENANT_CONFIG_STATE_DESC
                );

        // ==================== 数据流处理 ====================
        
        // 过滤无效数据
        DataStream<SessionEvent> validSessionStream = sessionStream
                .filter(session -> session != null &&
                        session.getHeader() != null &&
                        session.getSessionId() != null &&
                        !session.getSessionId().isEmpty())
                .uid("filter-invalid")
                .name("Filter Invalid Sessions");

        // 特征计算（连接 BroadcastStream）
        SingleOutputStreamOperator<FeatureStat> featureStream = validSessionStream
                .connect(configBroadcastStream)
                .process(new FeatureProcessFunctionV3(enableSampling, samplingRate))
                .uid("feature-calculator-v3")
                .name("Feature Calculator v3");

        // 提取侧输出
        DataStream<String> dlqStream = featureStream.getSideOutput(FeatureProcessFunctionV3.DLQ_TAG);
        DataStream<SessionEvent> l2TriggerStream = featureStream.getSideOutput(FeatureProcessFunctionV3.L2_TRIGGER_TAG);

        // ==================== Sink 配置 ====================

        // Sink 1: ClickHouse
        featureStream.addSink(
                ClickHouseSinkFactory.createFeatureSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseTable,
                        clickhouseUser,
                        clickhousePassword
                )
        ).uid("clickhouse-sink").name("ClickHouse Sink");

        // Sink 2: Kafka（主输出）
        featureStream.sinkTo(
                KafkaSinkFactory.createFeatureSink(kafkaBrokers, outputTopic)
        ).uid("kafka-sink").name("Kafka Sink");

        // Sink 3: DLQ Kafka
        dlqStream.sinkTo(
                DLQSinkFactory.createDLQSink(kafkaBrokers, dlqTopic)
        ).uid("dlq-sink").name("DLQ Sink");

        // Sink 4: L2 Trigger Kafka
        l2TriggerStream.sinkTo(
                KafkaSinkFactory.createSessionEventSink(kafkaBrokers, l2TriggerTopic)
        ).uid("l2-trigger-sink").name("L2 Trigger Sink");

        // ==================== 调试输出（可选）====================
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            featureStream.print("Feature").uid("print-feature");
            dlqStream.print("DLQ").uid("print-dlq");
            l2TriggerStream.print("L2-Trigger").uid("print-l2");
        }

        // ==================== 执行作业 ====================
        LOG.info("Job v3 configured successfully:");
        LOG.info("  Input: {}", inputTopic);
        LOG.info("  Output: {} + {}", outputTopic, clickhouseTable);
        LOG.info("  DLQ: {}", dlqTopic);
        LOG.info("  L2 Trigger: {}", l2TriggerTopic);
        LOG.info("  Config Sources: PostgreSQL (poll interval: {}ms)", configPollIntervalMs);
        LOG.info("  Parallelism: {}", parallelism);
        LOG.info("  Checkpoint: {}ms", checkpointInterval);
        LOG.info("  Degradation: sampling={}, rate={}", enableSampling, samplingRate);

        env.execute("Feature Extraction Job v3 (Full Enhanced)");
    }

    /**
     * 配置 Checkpoint
     */
    private static void configureCheckpoint(
            StreamExecutionEnvironment env,
            String checkpointPath,
            long intervalMs
    ) {
        env.enableCheckpointing(intervalMs, CheckpointingMode.EXACTLY_ONCE);

        CheckpointConfig checkpointConfig = env.getCheckpointConfig();
        checkpointConfig.setCheckpointTimeout(120000);
        checkpointConfig.setMinPauseBetweenCheckpoints(intervalMs / 2);
        checkpointConfig.setMaxConcurrentCheckpoints(1);
        checkpointConfig.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION
        );
        checkpointConfig.setTolerableCheckpointFailureNumber(3);

        EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);
        env.setStateBackend(stateBackend);

        checkpointConfig.setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));

        LOG.info("Checkpoint configured: path={}, interval={}ms", checkpointPath, intervalMs);
    }
}
