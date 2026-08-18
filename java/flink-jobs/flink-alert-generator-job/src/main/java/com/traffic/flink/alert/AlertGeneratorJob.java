package com.traffic.flink.alert;

import com.traffic.flink.alert.generator.AlertGenerator;
import com.traffic.flink.alert.generator.BusinessAlertGenerator;
import com.traffic.flink.alert.sink.AlertDlqSinkFactory;
import com.traffic.flink.alert.sink.ClickHouseAlertSinkFactory;
import com.traffic.flink.alert.sink.KafkaAlertSinkFactory;
import com.traffic.flink.alert.sink.OpenSearchAlertSinkFactory;
import com.traffic.flink.alert.source.BehaviorDetectionParseFunction;
import com.traffic.flink.alert.source.BusinessDetectionParseFunction;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.KafkaStartingOffsets;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.DetectionBusiness;
import com.traffic.proto.traffic.v1.Evidence;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.java.tuple.Tuple2;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.util.OutputTag;

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
    private static final OutputTag<CanonicalDlqMessage> BEHAVIOR_PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("alert-behavior-parse-dlq") {};
    private static final OutputTag<CanonicalDlqMessage> BUSINESS_PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("alert-business-parse-dlq") {};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Alert Generator Job...");

        // 加载配置
        ParameterTool params = ConfigUtils.loadConfig(args, "alert-generator-job.properties");

        // ==================== 配置参数 ====================
        
        // Kafka 配置
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String canonicalInputTopic = ConfigUtils.get(params, "kafka.input.topic", "detections.v1");
        String behaviorInputTopic = ConfigUtils.get(params, "kafka.input.topic.behavior", "detections.behavior.v1");
        String businessInputTopic = ConfigUtils.get(params, "kafka.input.topic.business", "detections.business.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "alerts.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-alert-generator-job");
        if (!"dlq.v1".equals(dlqTopic)) {
            throw new IllegalArgumentException("Alert generator failures must use canonical dlq.v1");
        }

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
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        // OpenSearch 配置
        String opensearchUrl = ConfigUtils.get(params, "opensearch.url", "http://localhost:9200");
        String opensearchLegacyIndex = ConfigUtils.get(params, "opensearch.index", "alerts");
        boolean opensearchV2Enabled = ConfigUtils.getBoolean(params, "opensearch.alerts.v2.enabled", false);
        String opensearchWriteAlias = ConfigUtils.get(params, "opensearch.alerts.write.alias", "alerts-v2-write");
        String opensearchIndex = resolveOpenSearchWriteTarget(
                opensearchV2Enabled, opensearchLegacyIndex, opensearchWriteAlias);
        String opensearchUser = ConfigUtils.get(params, "opensearch.user", "");
        String opensearchPassword = ConfigUtils.get(params, "opensearch.password", "");

        // 凭证必须显式配置（env/Secret 注入，禁止默认口令直连生产存储）
        validateCredentials(clickhouseUser, clickhousePassword, opensearchUser, opensearchPassword);

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
        boolean enableLegacyBehaviorDetection = ConfigUtils.getBoolean(
                params, "enable.legacy.behavior.detection", false);
        boolean enableBusinessDetection = ConfigUtils.getBoolean(
                params, "enable.business.detection", false);

        LOG.info("OpenSearch alert projection target={}, v2Enabled={}",
                opensearchIndex, opensearchV2Enabled);

        // ==================== 创建执行环境 ====================
        
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);

        // 配置 Checkpoint
        configureCheckpoint(env, checkpointPath, checkpointInterval, checkpointTimeout);

        // 配置重启策略。默认覆盖短时 Kafka/存储故障窗口，仍允许通过合同化参数调整。
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                ConfigUtils.getInt(params, "restart.attempts", 10),
                org.apache.flink.api.common.time.Time.seconds(
                        ConfigUtils.getInt(params, "restart.delay.seconds", 30))
        ));

        // ==================== Behavior Detection Source ====================
        
        KafkaSource<RawKafkaRecord> behaviorSource = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(canonicalInputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(KafkaStartingOffsets.from(params))
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        WatermarkStrategy<DetectionBehavior> behaviorWatermarkStrategy = WatermarkStrategy
                .<DetectionBehavior>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                .withTimestampAssigner((detection, timestamp) -> detection.getTs())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<RawKafkaRecord> behaviorStream = env
                .fromSource(behaviorSource, WatermarkStrategy.noWatermarks(), "Kafka-Behavior-Detection-Source")
                .uid("behavior-detection-source")
                .name("Behavior Detection Events Source");

        SingleOutputStreamOperator<DetectionBehavior> parsedBehaviorStream = behaviorStream
                .process(new BehaviorDetectionParseFunction(BEHAVIOR_PARSE_DLQ_TAG))
                .uid("filter-invalid-behavior-detections")
                .name("Validate Behavior Detections with source tuple");

        DataStream<DetectionBehavior> validBehaviorStream = parsedBehaviorStream
                .assignTimestampsAndWatermarks(behaviorWatermarkStrategy)
                .uid("behavior-detection-watermarks")
                .name("Assign Behavior Detection Watermarks");

        DataStream<CanonicalDlqMessage> dlqStream =
                parsedBehaviorStream.getSideOutput(BEHAVIOR_PARSE_DLQ_TAG);

        DataStream<DetectionBehavior> allBehaviorStream = validBehaviorStream;
        if (enableLegacyBehaviorDetection) {
            KafkaSource<RawKafkaRecord> legacyBehaviorSource = KafkaSource.<RawKafkaRecord>builder()
                    .setBootstrapServers(kafkaBrokers)
                    .setTopics(behaviorInputTopic)
                    .setGroupId(groupId + "-legacy-behavior")
                    .setStartingOffsets(KafkaStartingOffsets.from(params))
                    .setDeserializer(new RawKafkaRecordDeserializationSchema())
                    .setProperties(ConfigUtils.kafkaClientProperties(params))
                    .setProperty("partition.discovery.interval.ms", "30000")
                    .build();
            DataStream<RawKafkaRecord> legacyBehaviorRawStream = env
                    .fromSource(legacyBehaviorSource, WatermarkStrategy.noWatermarks(),
                            "Kafka-Legacy-Behavior-Detection-Source")
                    .uid("legacy-behavior-detection-source")
                    .name("Legacy Behavior Detection Events Source");
            SingleOutputStreamOperator<DetectionBehavior> parsedLegacyBehaviorStream =
                    legacyBehaviorRawStream
                            .process(new BehaviorDetectionParseFunction(BEHAVIOR_PARSE_DLQ_TAG))
                            .uid("validate-legacy-behavior-detections")
                            .name("Validate Legacy Behavior Detections with source tuple");
            DataStream<DetectionBehavior> legacyBehaviorStream = parsedLegacyBehaviorStream
                    .assignTimestampsAndWatermarks(behaviorWatermarkStrategy)
                    .uid("legacy-behavior-detection-watermarks")
                    .name("Assign Legacy Behavior Detection Watermarks");
            allBehaviorStream = allBehaviorStream.union(legacyBehaviorStream);
            dlqStream = dlqStream.union(
                    parsedLegacyBehaviorStream.getSideOutput(BEHAVIOR_PARSE_DLQ_TAG));
        }

        // ==================== Behavior Alert 生成 ====================
        
        SingleOutputStreamOperator<Tuple2<Alert, Evidence>> behaviorAlertStream = allBehaviorStream
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
                // 去重状态由 ValueState 升级为 MapState（按指纹独立去重），
                // 旧状态无法热迁移，换新 UID 强制从空状态冷启动。
                .uid("behavior-alert-generator-v2")
                .name("Behavior Alert Generator");

        // ==================== Business Detection Source (可选) ====================
        
        DataStream<Tuple2<Alert, Evidence>> businessAlertStream = null;
        SingleOutputStreamOperator<DetectionBusiness> parsedBusinessStream = null;

        if (enableBusinessDetection) {
            KafkaSource<RawKafkaRecord> businessSource = KafkaSource.<RawKafkaRecord>builder()
                    .setBootstrapServers(kafkaBrokers)
                    .setTopics(businessInputTopic)
                    .setGroupId(groupId + "-business")
                    .setStartingOffsets(KafkaStartingOffsets.from(params))
                    .setDeserializer(new RawKafkaRecordDeserializationSchema())
                    .setProperties(ConfigUtils.kafkaClientProperties(params))
                    .setProperty("partition.discovery.interval.ms", "30000")
                    .build();

            WatermarkStrategy<DetectionBusiness> businessWatermarkStrategy = WatermarkStrategy
                    .<DetectionBusiness>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                    .withTimestampAssigner((detection, timestamp) -> detection.getTs())
                    .withIdleness(Duration.ofMinutes(1));

            DataStream<RawKafkaRecord> businessStream = env
                    .fromSource(businessSource, WatermarkStrategy.noWatermarks(), "Kafka-Business-Detection-Source")
                    .uid("business-detection-source")
                    .name("Business Detection Events Source");

            parsedBusinessStream = businessStream
                    .process(new BusinessDetectionParseFunction(BUSINESS_PARSE_DLQ_TAG))
                    .uid("filter-invalid-business-detections")
                    .name("Validate Business Detections with source tuple");

            DataStream<DetectionBusiness> validBusinessStream = parsedBusinessStream
                    .assignTimestampsAndWatermarks(businessWatermarkStrategy)
                    .uid("business-detection-watermarks")
                    .name("Assign Business Detection Watermarks");

            // Business Alert 生成
            businessAlertStream = validBusinessStream
                    .keyBy(detection -> detection.getHeader().getTenantId() + ":" + detection.getCommunityId())
                    .process(new BusinessAlertGenerator(
                            dedupWindowMinutes,
                            arkimeUrl,
                            arkimeTimeBuffer,
                            severityCritical,
                            severityHigh,
                            severityMedium,
                            severityLow
                    ))
                    // 去重状态由 ValueState 升级为 MapState，换新 UID 冷启动
                    .uid("business-alert-generator-v2")
                    .name("Business Alert Generator");

            dlqStream = dlqStream.union(
                    parsedBusinessStream.getSideOutput(BUSINESS_PARSE_DLQ_TAG));
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

        dlqStream
                .sinkTo(AlertDlqSinkFactory.create(kafkaBrokers, dlqTopic))
                .setParallelism(sinkParallelism)
                .uid("alert-generator-dlq-sink")
                .name("Canonical DLQ Sink");

        // ==================== 调试输出 ====================
        
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            alertStream.print("Alert").setParallelism(1).uid("print-alert-sink");
            evidenceStream.print("Evidence").setParallelism(1).uid("print-evidence-sink");
        }

        // ==================== 打印配置摘要 ====================
        
        LOG.info("========== Alert Generator Job Configuration ==========");
        LOG.info("Canonical Detection Input Topic: {}", canonicalInputTopic);
        LOG.info("Legacy Behavior Input Topic: {} (enabled={})",
                behaviorInputTopic, enableLegacyBehaviorDetection);
        LOG.info("Business Input Topic: {} (enabled={})", businessInputTopic, enableBusinessDetection);
        LOG.info("Output Topic: {}", outputTopic);
        LOG.info("DLQ Topic: {}", dlqTopic);
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

    static String resolveOpenSearchWriteTarget(boolean v2Enabled, String legacyIndex, String writeAlias) {
        String target = v2Enabled ? writeAlias : legacyIndex;
        if (target == null || target.isBlank()) {
            throw new IllegalArgumentException("OpenSearch alert write target must not be blank");
        }
        return target;
    }

    /**
     * 凭证校验：ClickHouse 与 OpenSearch 的用户名/密码必须显式配置。
     * 禁止默认口令（OpenSearch admin/admin）与空密码直连生产存储；
     * 未配置时 fail-fast，由部署侧经环境变量/Secret 注入
     * （ConfigUtils 支持 CLICKHOUSE_PASSWORD/OPENSEARCH_USER/OPENSEARCH_PASSWORD
     * 等环境变量映射到对应配置键）。
     */
    private static void validateCredentials(
            String clickhouseUser,
            String clickhousePassword,
            String opensearchUser,
            String opensearchPassword) {
        if (clickhouseUser == null || clickhouseUser.isBlank()) {
            throw new IllegalArgumentException(
                    "clickhouse.user must be configured via environment/Secret (CLICKHOUSE_USER)");
        }
        if (clickhousePassword == null || clickhousePassword.isBlank()) {
            throw new IllegalArgumentException(
                    "clickhouse.password must be configured via environment/Secret (CLICKHOUSE_PASSWORD); "
                            + "empty default password is forbidden");
        }
        if (opensearchUser == null || opensearchUser.isBlank() || "admin".equals(opensearchUser)) {
            throw new IllegalArgumentException(
                    "opensearch.user must be explicitly configured via environment/Secret "
                            + "(OPENSEARCH_USER); default 'admin' is forbidden");
        }
        if (opensearchPassword == null || opensearchPassword.isBlank()) {
            throw new IllegalArgumentException(
                    "opensearch.password must be configured via environment/Secret (OPENSEARCH_PASSWORD); "
                            + "default 'admin' is forbidden");
        }
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
