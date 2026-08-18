package com.traffic.flink.rule;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.KafkaStartingOffsets;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.flink.rule.broadcast.RuleBroadcastProcessFunction;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.sink.ClickHouseDetectionSinkFactory;
import com.traffic.flink.rule.sink.KafkaDetectionSinkFactory;
import com.traffic.flink.rule.sink.RuleDlqSinkFactory;
import com.traffic.flink.rule.sink.RuleUpdateAckKafkaSinkFactory;
import com.traffic.flink.rule.model.RuleUpdateAppliedAck;
import com.traffic.flink.rule.source.FeatureStatParseFunction;
import com.traffic.flink.rule.source.RuleJsonParseFunction;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.state.MapStateDescriptor;
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
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Duration;

/**
 * Flink Rule Job - 动态规则引擎（增强版）
 * 
 * 使用 Broadcast State Pattern 实现规则热更新
 * 
 * 数据流：
 * - 主流: feature.stat.v1 (Kafka)
 * - 广播流: rule.updates (Kafka)
 * 
 * 输出：
 * - detections.v1 (Kafka)
 * - detections_behavior_local (ClickHouse)
 * - dlq.rule-job (DLQ，规则解析失败)
 * 
 * 新增功能：
 * 1. 规则解析失败进入 DLQ
 * 2. Watermark 配置（规则流）
 * 3. 并行度校验
 * 4. 更详细的日志与指标
 */
public class RuleJob {

    private static final Logger LOG = LoggerFactory.getLogger(RuleJob.class);

    private static final OutputTag<CanonicalDlqMessage> FEATURE_PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("rule-feature-parse-dlq") {};
    private static final OutputTag<CanonicalDlqMessage> RULE_PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("rule-update-parse-dlq") {};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Rule Engine Job (Enhanced Version)...");

        // 加载配置
        ParameterTool params = ConfigUtils.loadConfig(args, "rule-job.properties");

        // 配置参数
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String featureTopic = ConfigUtils.get(params, "kafka.feature.topic", "feature.stat.v1");
        String ruleUpdateTopic = ConfigUtils.get(params, "kafka.rule.topic", "rule.updates");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "detections.v1");
        String ruleAppliedTopic = ConfigUtils.get(
                params, "kafka.rule.applied.topic", "rule-update-applied.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-rule-job");
        if (!"dlq.v1".equals(dlqTopic)) {
            throw new IllegalArgumentException("Rule job failures must use canonical dlq.v1");
        }
        if (!"rule-update-applied.v1".equals(ruleAppliedTopic)) {
            throw new IllegalArgumentException(
                    "Rule job application receipts must use rule-update-applied.v1");
        }
        String checkpointPath = ConfigUtils.get(params, "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/rule-job");

        String clickhouseUrl = ConfigUtils.get(params, "clickhouse.url", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123");
        String clickhouseDatabase = ConfigUtils.get(params, "clickhouse.database", "traffic");
        String clickhouseTable = ConfigUtils.get(params, "clickhouse.table", "detections_behavior");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        int parallelism = ConfigUtils.getInt(params, "parallelism", 4);
        long checkpointInterval = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60000);

        // 创建执行环境
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);

        // 配置 Checkpoint
        configureCheckpoint(env, checkpointPath, checkpointInterval);

        // 配置重启策略。默认覆盖短时 Kafka/存储故障窗口，仍允许通过合同化参数调整。
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                ConfigUtils.getInt(params, "restart.attempts", 10),
                org.apache.flink.api.common.time.Time.seconds(
                        ConfigUtils.getInt(params, "restart.delay.seconds", 30))
        ));

        // ==================== 主流：Feature 数据 ====================
        KafkaSource<RawKafkaRecord> featureSource = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(featureTopic)
                .setGroupId(groupId)
                .setStartingOffsets(KafkaStartingOffsets.from(params))
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        WatermarkStrategy<FeatureStat> featureWatermark = WatermarkStrategy
                .<FeatureStat>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                .withTimestampAssigner((feature, timestamp) -> feature.getTs())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<RawKafkaRecord> featureStream = env
                .fromSource(featureSource, WatermarkStrategy.noWatermarks(), "Kafka-Feature-Source")
                .uid("feature-source")
                .name("Feature Stats Source");

        SingleOutputStreamOperator<FeatureStat> parsedFeatureStream = featureStream
                .process(new FeatureStatParseFunction(FEATURE_PARSE_DLQ_TAG))
                .uid("filter-invalid-features")
                .name("Validate FeatureStat with source tuple");

        DataStream<FeatureStat> validFeatureStream = parsedFeatureStream
                .assignTimestampsAndWatermarks(featureWatermark)
                .uid("feature-stat-watermarks")
                .name("Assign FeatureStat Watermarks");

        // ==================== 广播流：规则更新 ====================
        KafkaSource<RawKafkaRecord> ruleSource = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(ruleUpdateTopic)
                .setGroupId(groupId + "-rule")
                .setStartingOffsets(OffsetsInitializer.earliest()) // 从头读取规则
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        // 规则流使用单调递增水印（规则按时间顺序到达）
        WatermarkStrategy<Rule> ruleWatermark = WatermarkStrategy
                .<Rule>forMonotonousTimestamps()
                .withIdleness(Duration.ofMinutes(5));

        DataStream<RawKafkaRecord> ruleStringStream = env
                .fromSource(ruleSource, WatermarkStrategy.noWatermarks(), "Kafka-Rule-Source")
                .uid("rule-source")
                .name("Rule Updates Source");

        SingleOutputStreamOperator<Rule> parsedRuleStream = ruleStringStream
                .process(new RuleJsonParseFunction(RULE_PARSE_DLQ_TAG))
                .uid("parse-rules")
                .name("Validate Rule JSON with source tuple");

        DataStream<Rule> ruleStream = parsedRuleStream
                .assignTimestampsAndWatermarks(ruleWatermark)
                .uid("rule-update-watermarks")
                .name("Assign Rule Update Watermarks");

        DataStream<CanonicalDlqMessage> dlqStream = parsedFeatureStream
                .getSideOutput(FEATURE_PARSE_DLQ_TAG)
                .union(parsedRuleStream.getSideOutput(RULE_PARSE_DLQ_TAG));

        // 创建广播流
        MapStateDescriptor<String, Rule> ruleStateDesc = 
                RuleBroadcastProcessFunction.getRuleStateDescriptor();
        
        BroadcastStream<Rule> ruleBroadcastStream = ruleStream.broadcast(ruleStateDesc);

        // ==================== 连接主流与广播流 ====================
        SingleOutputStreamOperator<DetectionBehavior> detectionStream = validFeatureStream
                .connect(ruleBroadcastStream)
                .process(new RuleBroadcastProcessFunction())
                .uid("rule-matcher")
                .name("Rule Matcher");
        DataStream<RuleUpdateAppliedAck> ruleUpdateAcks = detectionStream
                .getSideOutput(RuleBroadcastProcessFunction.RULE_UPDATE_ACK_TAG);

        // ==================== Sink ====================

        // Sink 1: ClickHouse
        detectionStream.addSink(
                ClickHouseDetectionSinkFactory.createDetectionSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseTable,
                        clickhouseUser,
                        clickhousePassword
                )
        ).uid("clickhouse-sink").name("ClickHouse Detection Sink");

        // Sink 2: Kafka
        detectionStream.sinkTo(
                KafkaDetectionSinkFactory.createDetectionSink(kafkaBrokers, outputTopic)
        ).uid("kafka-sink").name("Kafka Detection Sink");

        // Sink 3: DLQ（规则解析失败）
        dlqStream.sinkTo(RuleDlqSinkFactory.create(kafkaBrokers, dlqTopic))
                .uid("dlq-sink")
                .name("Canonical DLQ Sink");

        ruleUpdateAcks.sinkTo(
                RuleUpdateAckKafkaSinkFactory.create(kafkaBrokers, ruleAppliedTopic))
                .uid("rule-update-applied-ack-sink")
                .name("Kafka Sink (rule-update-applied.v1)");

        // 打印统计信息（调试）
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            detectionStream.print("Detection").uid("print-sink");
        }

        LOG.info("========== Job Configuration ==========");
        LOG.info("  Feature Topic: {}", featureTopic);
        LOG.info("  Rule Update Topic: {}", ruleUpdateTopic);
        LOG.info("  Output Topic: {}", outputTopic);
        LOG.info("  Rule Applied Topic: {}", ruleAppliedTopic);
        LOG.info("  DLQ Topic: {}", dlqTopic);
        LOG.info("  ClickHouse: {}.{}", clickhouseDatabase, clickhouseTable);
        LOG.info("  Parallelism: {}", parallelism);
        LOG.info("  Checkpoint Interval: {}ms", checkpointInterval);
        LOG.info("=======================================");

        // 执行作业
        env.execute("Rule Engine Job (Enhanced)");
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

        CheckpointConfig config = env.getCheckpointConfig();
        config.setCheckpointTimeout(120000);
        config.setMinPauseBetweenCheckpoints(intervalMs / 2);
        config.setMaxConcurrentCheckpoints(1);
        config.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION
        );

        EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);
        env.setStateBackend(stateBackend);
        config.setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));
    }

}
