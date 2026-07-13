package com.traffic.flink.alert;

import com.traffic.flink.alert.generator.AlertGenerator;
import com.traffic.flink.alert.generator.BusinessAlertGenerator;
import com.traffic.flink.alert.sink.ClickHouseAlertSinkFactory;
import com.traffic.flink.alert.sink.KafkaAlertSinkFactory;
import com.traffic.flink.alert.sink.OpenSearchAlertSinkFactory;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.DetectionBusiness;
import com.traffic.proto.traffic.v1.Evidence;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.java.tuple.Tuple2;
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
 * Alert Generator Job (重构版)
 * 
 * 将检测结果（DetectionBehavior + DetectionBusiness）转换为告警（Alert）和证据（Evidence）
 * 
 * 数据流：
 * - 输入: 
 *   - detections.behavior.v1 (Kafka) - 行为检测结果
 *   - detections.business.v1 (Kafka) - 业务检测结果（规则触发）
 * - 输出: 
 *   - alerts.v1 (Kafka)
 *   - alerts_local (ClickHouse)
 *   - alerts (OpenSearch)
 *   - evidence_local (ClickHouse)
 * 
 * 核心功能：
 * 1. 告警生成与去重（State TTL 自动清理）
 * 2. 证据提取与关联
 * 3. Arkime 链接生成
 * 4. 多存储写入（CH + OS + Kafka）
 * 
 * 修复内容：
 * - 添加 DetectionBusiness 输入流
 * - 优化 Checkpoint 配置
 * - 添加完整的配置参数支持
 */
public class AlertGeneratorJob {

    private static final Logger LOG = LoggerFactory.getLogger(AlertGeneratorJob.class);

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Alert Generator Job...");

        // 加载配置
        ParameterTool params = ConfigUtils.loadConfig(args, "alert-generator-job.properties");

        // ==================== 配置参数 ====================
        
        // Kafka 配置
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String behaviorInputTopic = ConfigUtils.get(params, "kafka.input.topic.behavior", "detections.behavior.v1");
        String businessInputTopic = ConfigUtils.get(params, "kafka.input.topic.business", "detections.business.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "alerts.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-alert-generator-job");

        // Checkpoint 配置（集群默认使用本地挂载路径，S3/MinIO 作为可选文件系统）
        String checkpointPath = ConfigUtils.get(params, "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/alert-generator-job");
        long checkpointInterval = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60000);
        long checkpointTimeout = ConfigUtils.getLong(params, "checkpoint.timeout.ms", 120000);

        // ClickHouse 配置
        String clickhouseUrl = ConfigUtils.get(params, "clickhouse.url", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123");
        String clickhouseDatabase = ConfigUtils.get(params, "clickhouse.database", "traffic");
        String clickhouseAlertTable = ConfigUtils.get(params, "clickhouse.alert.table", "alerts");
        String clickhouseEvidenceTable = ConfigUtils.get(params, "clickhouse.evidence.table", "evidence");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        // OpenSearch 配置
        String opensearchUrl = ConfigUtils.get(params, "opensearch.url", "http://localhost:9200");
        String opensearchIndex = ConfigUtils.get(params, "opensearch.index", "alerts");
        String opensearchUser = ConfigUtils.get(params, "opensearch.user", "admin");
        String opensearchPassword = ConfigUtils.get(params, "opensearch.password", "admin");

        // Arkime 配置
        String arkimeUrl = ConfigUtils.get(params, "arkime.url", "http://localhost:8005");
        int arkimeTimeBuffer = ConfigUtils.getInt(params, "arkime.time.buffer.seconds", 120);

        // 去重配置
        long dedupWindowMinutes = ConfigUtils.getLong(params, "dedup.window.minutes", 10);

        // Severity 阈值配置
        float severityCritical = ConfigUtils.getFloat(params, "severity.threshold.critical", 0.9f);
        float severityHigh = ConfigUtils.getFloat(params, "severity.threshold.high", 0.7f);
        float severityMedium = ConfigUtils.getFloat(params, "severity.threshold.medium", 0.5f);
        float severityLow = ConfigUtils.getFloat(params, "severity.threshold.low", 0.3f);

        // 并行度配置
        int parallelism = ConfigUtils.getInt(params, "parallelism", 4);
        int sinkParallelism = ConfigUtils.getInt(params, "sink.parallelism", 2);

        // 是否启用 Business 检测输入
        boolean enableBusinessDetection = ConfigUtils.getBoolean(params, "enable.business.detection", true);

        // ==================== 创建执行环境 ====================
        
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);

        // 配置 Checkpoint
        configureCheckpoint(env, checkpointPath, checkpointInterval, checkpointTimeout);

        // 配置重启策略
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                3,
                org.apache.flink.api.common.time.Time.seconds(30)
        ));

        // ==================== Behavior Detection Source ====================
        
        KafkaSource<DetectionBehavior> behaviorSource = KafkaSource.<DetectionBehavior>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(behaviorInputTopic)
                .setGroupId(groupId + "-behavior")
                .setStartingOffsets(OffsetsInitializer.latest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(DetectionBehavior.class))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        WatermarkStrategy<DetectionBehavior> behaviorWatermarkStrategy = WatermarkStrategy
                .<DetectionBehavior>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                .withTimestampAssigner((detection, timestamp) -> detection.getTs())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<DetectionBehavior> behaviorStream = env
                .fromSource(behaviorSource, behaviorWatermarkStrategy, "Kafka-Behavior-Detection-Source")
                .uid("behavior-detection-source")
                .name("Behavior Detection Events Source");

        // 过滤无效检测
        DataStream<DetectionBehavior> validBehaviorStream = behaviorStream
                .filter(detection -> detection != null &&
                        detection.getHeader() != null &&
                        detection.getCommunityId() != null &&
                        !detection.getCommunityId().isEmpty() &&
                        detection.getTopScore() > 0)
                .uid("filter-invalid-behavior-detections")
                .name("Filter Invalid Behavior Detections");

        // ==================== Behavior Alert 生成 ====================
        
        SingleOutputStreamOperator<Tuple2<Alert, Evidence>> behaviorAlertStream = validBehaviorStream
                .keyBy(detection -> detection.getHeader().getTenantId() + ":" + detection.getCommunityId())
                .process(new AlertGenerator(
                        dedupWindowMinutes,
                        arkimeUrl,
                        arkimeTimeBuffer,
                        severityCritical,
                        severityHigh,
                        severityMedium,
                        severityLow
                ))
                .uid("behavior-alert-generator")
                .name("Behavior Alert Generator");

        // ==================== Business Detection Source (可选) ====================
        
        DataStream<Tuple2<Alert, Evidence>> businessAlertStream = null;

        if (enableBusinessDetection) {
            KafkaSource<DetectionBusiness> businessSource = KafkaSource.<DetectionBusiness>builder()
                    .setBootstrapServers(kafkaBrokers)
                    .setTopics(businessInputTopic)
                    .setGroupId(groupId + "-business")
                    .setStartingOffsets(OffsetsInitializer.latest())
                    .setValueOnlyDeserializer(new ProtoDeserializer<>(DetectionBusiness.class))
                    .setProperties(ConfigUtils.kafkaClientProperties(params))
                    .setProperty("partition.discovery.interval.ms", "30000")
                    .build();

            WatermarkStrategy<DetectionBusiness> businessWatermarkStrategy = WatermarkStrategy
                    .<DetectionBusiness>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                    .withTimestampAssigner((detection, timestamp) -> detection.getTs())
                    .withIdleness(Duration.ofMinutes(1));

            DataStream<DetectionBusiness> businessStream = env
                    .fromSource(businessSource, businessWatermarkStrategy, "Kafka-Business-Detection-Source")
                    .uid("business-detection-source")
                    .name("Business Detection Events Source");

            // 过滤无效检测
            DataStream<DetectionBusiness> validBusinessStream = businessStream
                    .filter(detection -> detection != null &&
                            detection.getHeader() != null &&
                            detection.getCommunityId() != null &&
                            !detection.getCommunityId().isEmpty())
                    .uid("filter-invalid-business-detections")
                    .name("Filter Invalid Business Detections");

            // Business Alert 生成
            businessAlertStream = validBusinessStream
                    .keyBy(detection -> detection.getHeader().getTenantId() + ":" + detection.getCommunityId())
                    .process(new BusinessAlertGenerator(
                            dedupWindowMinutes,
                            arkimeUrl,
                            arkimeTimeBuffer
                    ))
                    .uid("business-alert-generator")
                    .name("Business Alert Generator");
        }

        // ==================== 合并 Alert 流 ====================
        
        DataStream<Tuple2<Alert, Evidence>> mergedAlertStream;
        if (businessAlertStream != null) {
            mergedAlertStream = behaviorAlertStream.union(businessAlertStream);
        } else {
            mergedAlertStream = behaviorAlertStream;
        }

        // 分离 Alert 和 Evidence
        DataStream<Alert> alertStream = mergedAlertStream
                .map(tuple -> tuple.f0)
                .filter(alert -> alert != null)
                .uid("extract-alert")
                .name("Extract Alert");

        DataStream<Evidence> evidenceStream = mergedAlertStream
                .map(tuple -> tuple.f1)
                .filter(evidence -> evidence != null)
                .uid("extract-evidence")
                .name("Extract Evidence");

        // ==================== Alert Sinks ====================

        // Sink 1: ClickHouse
        alertStream
                .addSink(ClickHouseAlertSinkFactory.createAlertSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseAlertTable,
                        clickhouseUser,
                        clickhousePassword
                ))
                .setParallelism(sinkParallelism)
                .uid("clickhouse-alert-sink")
                .name("ClickHouse Alert Sink");

        // Sink 2: OpenSearch
        alertStream
                .addSink(OpenSearchAlertSinkFactory.createAlertSink(
                        opensearchUrl,
                        opensearchIndex,
                        opensearchUser,
                        opensearchPassword
                ))
                .setParallelism(sinkParallelism)
                .uid("opensearch-alert-sink")
                .name("OpenSearch Alert Sink");

        // Sink 3: Kafka
        alertStream
                .sinkTo(KafkaAlertSinkFactory.createAlertSink(kafkaBrokers, outputTopic))
                .setParallelism(sinkParallelism)
                .uid("kafka-alert-sink")
                .name("Kafka Alert Sink");

        // ==================== Evidence Sinks ====================

        evidenceStream
                .addSink(ClickHouseAlertSinkFactory.createEvidenceSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseEvidenceTable,
                        clickhouseUser,
                        clickhousePassword
                ))
                .setParallelism(sinkParallelism)
                .uid("clickhouse-evidence-sink")
                .name("ClickHouse Evidence Sink");

        // ==================== 调试输出 ====================
        
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            alertStream.print("Alert").setParallelism(1).uid("print-alert-sink");
            evidenceStream.print("Evidence").setParallelism(1).uid("print-evidence-sink");
        }

        // ==================== 打印配置摘要 ====================
        
        LOG.info("========== Alert Generator Job Configuration ==========");
        LOG.info("Behavior Input Topic: {}", behaviorInputTopic);
        LOG.info("Business Input Topic: {} (enabled={})", businessInputTopic, enableBusinessDetection);
        LOG.info("Output Topic: {}", outputTopic);
        LOG.info("ClickHouse: {}.{}", clickhouseDatabase, clickhouseAlertTable);
        LOG.info("OpenSearch: {}/{}", opensearchUrl, opensearchIndex);
        LOG.info("Arkime URL: {} (buffer={}s)", arkimeUrl, arkimeTimeBuffer);
        LOG.info("Dedup Window: {} minutes", dedupWindowMinutes);
        LOG.info("Severity Thresholds: critical={}, high={}, medium={}, low={}",
                severityCritical, severityHigh, severityMedium, severityLow);
        LOG.info("Parallelism: {} (sink={})", parallelism, sinkParallelism);
        LOG.info("Checkpoint: {} (interval={}ms)", checkpointPath, checkpointInterval);
        LOG.info("=========================================================");

        // 执行作业
        env.execute("Alert Generator Job");
    }

    /**
     * 配置 Checkpoint
     */
    private static void configureCheckpoint(
            StreamExecutionEnvironment env,
            String checkpointPath,
            long intervalMs,
            long timeoutMs
    ) {
        // 启用 Checkpoint
        env.enableCheckpointing(intervalMs, CheckpointingMode.EXACTLY_ONCE);

        CheckpointConfig config = env.getCheckpointConfig();
        
        // Checkpoint 超时
        config.setCheckpointTimeout(timeoutMs);
        
        // 最小间隔
        config.setMinPauseBetweenCheckpoints(intervalMs / 2);
        
        // 最大并发 Checkpoint
        config.setMaxConcurrentCheckpoints(1);
        
        // 取消时保留 Checkpoint
        config.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION
        );

        // 容忍 Checkpoint 失败次数
        config.setTolerableCheckpointFailureNumber(3);

        // 配置 RocksDB State Backend（增量 Checkpoint）
        EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);
        env.setStateBackend(stateBackend);
        
        // 配置 Checkpoint 存储
        config.setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));

        LOG.info("Checkpoint configured: path={}, interval={}ms, timeout={}ms",
                checkpointPath, intervalMs, timeoutMs);
    }
}
