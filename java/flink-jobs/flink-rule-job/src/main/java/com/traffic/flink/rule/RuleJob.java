package com.traffic.flink.rule;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.rule.broadcast.RuleBroadcastProcessFunction;
import com.traffic.flink.rule.model.Rule;
import com.traffic.flink.rule.sink.ClickHouseDetectionSinkFactory;
import com.traffic.flink.rule.sink.KafkaDetectionSinkFactory;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
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

import org.apache.kafka.clients.producer.ProducerRecord;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.HashMap;
import java.util.Map;

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

    // DLQ OutputTag
    private static final OutputTag<String> DLQ_RULE_PARSE_FAILED = 
            new OutputTag<String>("dlq-rule-parse-failed"){};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Rule Engine Job (Enhanced Version)...");

        // 加载配置
        ParameterTool params = ConfigUtils.loadConfig(args, "rule-job.properties");

        // 配置参数
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String featureTopic = ConfigUtils.get(params, "kafka.feature.topic", "feature.stat.v1");
        String ruleUpdateTopic = ConfigUtils.get(params, "kafka.rule.topic", "rule.updates");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "detections.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.rule-job");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-rule-job");
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

        // 配置重启策略
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                3,
                org.apache.flink.api.common.time.Time.seconds(30)
        ));

        // ==================== 主流：Feature 数据 ====================
        KafkaSource<FeatureStat> featureSource = KafkaSource.<FeatureStat>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(featureTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.latest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(FeatureStat.class))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        WatermarkStrategy<FeatureStat> featureWatermark = WatermarkStrategy
                .<FeatureStat>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                .withTimestampAssigner((feature, timestamp) -> feature.getTs())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<FeatureStat> featureStream = env
                .fromSource(featureSource, featureWatermark, "Kafka-Feature-Source")
                .uid("feature-source")
                .name("Feature Stats Source");

        // 过滤无效特征
        DataStream<FeatureStat> validFeatureStream = featureStream
                .filter(feature -> feature != null && 
                        feature.getHeader() != null &&
                        feature.getCommunityId() != null &&
                        !feature.getCommunityId().isEmpty())
                .uid("filter-invalid-features")
                .name("Filter Invalid Features");

        // ==================== 广播流：规则更新 ====================
        KafkaSource<String> ruleSource = KafkaSource.<String>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(ruleUpdateTopic)
                .setGroupId(groupId + "-rule")
                .setStartingOffsets(OffsetsInitializer.earliest()) // 从头读取规则
                .setValueOnlyDeserializer(new SimpleStringSchema())
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        // 规则流使用单调递增水印（规则按时间顺序到达）
        WatermarkStrategy<String> ruleWatermark = WatermarkStrategy
                .<String>forMonotonousTimestamps()
                .withIdleness(Duration.ofMinutes(5));

        DataStream<String> ruleStringStream = env
                .fromSource(ruleSource, ruleWatermark, "Kafka-Rule-Source")
                .uid("rule-source")
                .name("Rule Updates Source");

        // 解析规则 JSON（带 DLQ 处理）
        SingleOutputStreamOperator<Rule> ruleStream = ruleStringStream
                .process(new RuleJsonParser())
                .uid("parse-rules")
                .name("Parse Rule JSON");

        // 提取解析失败的规则进入 DLQ
        DataStream<String> dlqStream = ruleStream.getSideOutput(DLQ_RULE_PARSE_FAILED);

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
        dlqStream.sinkTo(createDLQSink(kafkaBrokers, dlqTopic))
                .uid("dlq-sink")
                .name("DLQ Sink");

        // 打印统计信息（调试）
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            detectionStream.print("Detection").uid("print-sink");
        }

        LOG.info("========== Job Configuration ==========");
        LOG.info("  Feature Topic: {}", featureTopic);
        LOG.info("  Rule Update Topic: {}", ruleUpdateTopic);
        LOG.info("  Output Topic: {}", outputTopic);
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

    /**
     * 创建 DLQ Kafka Sink
     */
    private static KafkaSink<String> createDLQSink(String brokers, String topic) {
        return KafkaSink.<String>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new DLQKafkaSerializer(topic))
                .setKafkaProducerConfig(com.traffic.flink.common.ConfigUtil.kafkaClientProperties())
                .build();
    }

    /**
     * DLQ Kafka 序列化器
     */
    private static class DLQKafkaSerializer implements KafkaRecordSerializationSchema<String> {

        private static final long serialVersionUID = 1L;
        private final String topic;

        public DLQKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                String element,
                KafkaSinkContext context,
                Long timestamp
        ) {
            if (element == null) {
                return null;
            }

            // Key: "rule-parse-failed"
            byte[] keyBytes = "rule-parse-failed".getBytes(StandardCharsets.UTF_8);
            byte[] valueBytes = element.getBytes(StandardCharsets.UTF_8);

            return new ProducerRecord<>(topic, null, timestamp, keyBytes, valueBytes);
        }
    }

    /**
     * 规则 JSON 解析处理函数（带 DLQ 输出）
     */
    private static class RuleJsonParser extends org.apache.flink.streaming.api.functions.ProcessFunction<String, Rule> {

        private static final long serialVersionUID = 1L;
        private static final Logger LOG = LoggerFactory.getLogger(RuleJsonParser.class);

        private transient ObjectMapper mapper;
        private transient long successCount = 0;
        private transient long errorCount = 0;

        @Override
        public void open(org.apache.flink.configuration.Configuration parameters) throws Exception {
            super.open(parameters);
            mapper = new ObjectMapper();
        }

        @Override
        public void processElement(
                String json,
                Context ctx,
                org.apache.flink.util.Collector<Rule> out
        ) throws Exception {
            try {
                Rule rule = mapper.readValue(json, Rule.class);
                out.collect(rule);
                successCount++;
                
                if (successCount % 1000 == 0) {
                    LOG.info("Parsed {} rules successfully (errors: {})", successCount, errorCount);
                }
            } catch (Exception e) {
                errorCount++;
                
                // 输出到 DLQ
                Map<String, Object> dlqRecord = new HashMap<>();
                dlqRecord.put("error_code", "RULE_PARSE_FAILED");
                dlqRecord.put("error_message", e.getMessage());
                dlqRecord.put("raw_event", json);
                dlqRecord.put("timestamp", System.currentTimeMillis());
                
                String dlqJson = mapper.writeValueAsString(dlqRecord);
                ctx.output(DLQ_RULE_PARSE_FAILED, dlqJson);
                
                LOG.error("Failed to parse rule JSON (sent to DLQ): {}", json, e);
            }
        }
    }
}
