////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/CepJob.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep;

import com.traffic.flink.cep.patterns.*;
import com.traffic.flink.cep.select.*;
import com.traffic.flink.cep.sink.ClickHouseSinkFactory;
import com.traffic.flink.cep.sink.KafkaSinkFactory;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.Campaign;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.cep.CEP;
import org.apache.flink.cep.PatternStream;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.KeyedStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.time.Duration;

/**
 * Flink CEP 关联分析作业
 * 
 * 使用 CEP 实现跨时间窗口的告警关联，识别多阶段攻击
 * 
 * 输入: alerts.v1 (Kafka)
 * 输出: campaigns.v1 (Kafka) + campaigns_local (ClickHouse)
 * 
 * 支持的模式:
 * 1. 扫描-利用 (Scan-Exploit)
 * 2. 暴力破解 (Brute Force)
 * 3. 横向移动 (Lateral Movement)
 * 4. 数据外泄 (Data Exfiltration)
 * 5. C2 信标 (C2 Beacon)
 */
public class CepJob {

    private static final Logger LOG = LoggerFactory.getLogger(CepJob.class);

    // DLQ 侧输出标签
    private static final OutputTag<String> DLQ_TAG = new OutputTag<String>("dlq"){};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting CEP Correlation Job...");

        // 加载配置
        ParameterTool params = ConfigUtils.loadConfig(args, "cep-job.properties");

        // 配置参数
        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", "alerts.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "campaigns.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.cep-job");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-cep-job");
        String checkpointPath = ConfigUtils.get(params, "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/cep-job");

        String clickhouseUrl = ConfigUtils.get(params, "clickhouse.url", "clickhouse-1.middleware.svc:8123,clickhouse-2.middleware.svc:8123");
        String clickhouseDatabase = ConfigUtils.get(params, "clickhouse.database", "traffic");
        String clickhouseTable = ConfigUtils.get(params, "clickhouse.table", "campaigns");
        String clickhouseUser = resolveConfigOrEnv(params, "clickhouse.user", "CLICKHOUSE_USER", "default");
        String clickhousePassword = resolveConfigOrEnv(params, "clickhouse.password", "CLICKHOUSE_PASSWORD", "");

        int parallelism = ConfigUtils.getInt(params, "parallelism", 4);
        long checkpointInterval = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60000);

        // 模式配置
        PatternConfig patternConfig = loadPatternConfig(params);

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

        // ==================== Kafka Source ====================
        KafkaSource<Alert> source = KafkaSource.<Alert>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.latest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(Alert.class))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .setProperty("partition.discovery.interval.ms", "30000")
                .build();

        // Watermark 策略：允许 30 秒乱序
        WatermarkStrategy<Alert> watermarkStrategy = WatermarkStrategy
                .<Alert>forBoundedOutOfOrderness(Duration.ofSeconds(30))
                .withTimestampAssigner((alert, timestamp) -> alert.getLastSeen())
                .withIdleness(Duration.ofMinutes(1));

        DataStream<Alert> alertStream = env
                .fromSource(source, watermarkStrategy, "Alert-Source")
                .uid("alert-source")
                .name("Alert Events Source");

        // ==================== 验证过滤（带 DLQ）====================
        SingleOutputStreamOperator<Alert> validAlerts = alertStream
                .process(new AlertValidationFunction())
                .uid("validate-alerts")
                .name("Validate Alerts");

        // 获取 DLQ 侧输出并写入 Kafka
        DataStream<String> dlqStream = validAlerts.getSideOutput(DLQ_TAG);
        
        if (ConfigUtils.getBoolean(params, "dlq.enabled", true)) {
            dlqStream.sinkTo(createDlqSink(kafkaBrokers, dlqTopic))
                    .uid("dlq-sink")
                    .name("DLQ Sink");
            LOG.info("DLQ enabled, topic: {}", dlqTopic);
        }

        // 按源 IP 分组（关联分析通常基于攻击者）
        KeyedStream<Alert, String> keyedAlerts = validAlerts.keyBy(Alert::getSrcIp);

        // ==================== CEP 模式匹配 ====================

        // 模式 1: 扫描-利用
        DataStream<Campaign> scanExploitCampaigns = applyScanExploitPattern(
                keyedAlerts, patternConfig);

        // 模式 2: 暴力破解
        DataStream<Campaign> bruteForceCampaigns = applyBruteForcePattern(
                keyedAlerts, patternConfig);

        // 模式 3: 横向移动
        DataStream<Campaign> lateralMovementCampaigns = applyLateralMovementPattern(
                keyedAlerts, patternConfig);

        // 模式 4: 数据外泄
        DataStream<Campaign> dataExfilCampaigns = applyDataExfilPattern(
                keyedAlerts, patternConfig);

        // 模式 5: C2 信标
        DataStream<Campaign> c2BeaconCampaigns = applyC2BeaconPattern(
                keyedAlerts, patternConfig);

        // 合并所有 Campaign 流
        DataStream<Campaign> allCampaigns = scanExploitCampaigns
                .union(bruteForceCampaigns)
                .union(lateralMovementCampaigns)
                .union(dataExfilCampaigns)
                .union(c2BeaconCampaigns);

        // ==================== Sink ====================

        // Kafka Sink
        allCampaigns.sinkTo(
                KafkaSinkFactory.createCampaignSink(kafkaBrokers, outputTopic)
        ).uid("kafka-sink").name("Kafka Campaign Sink");

        // ClickHouse Sink
        allCampaigns.addSink(
                ClickHouseSinkFactory.createCampaignSink(
                        clickhouseUrl,
                        clickhouseDatabase,
                        clickhouseTable,
                        clickhouseUser,
                        clickhousePassword
                )
        ).uid("clickhouse-sink").name("ClickHouse Campaign Sink");

        // 打印统计信息（调试）
        if (ConfigUtils.getBoolean(params, "debug.print", false)) {
            allCampaigns.print("Campaign").uid("print-sink");
        }

        LOG.info("Job configured:");
        LOG.info("  Input Topic: {}", inputTopic);
        LOG.info("  Output Topic: {}", outputTopic);
        LOG.info("  DLQ Topic: {}", dlqTopic);
        LOG.info("  ClickHouse: {}.{}", clickhouseDatabase, clickhouseTable);
        LOG.info("  Parallelism: {}", parallelism);

        // 执行作业
        env.execute("CEP Correlation Job");
    }

    private static String resolveConfigOrEnv(ParameterTool params, String key, String envName, String defaultValue) {
        String value = ConfigUtils.get(params, key, "");
        if (value != null && !value.isEmpty() && !value.startsWith("${")) {
            return value;
        }

        String envValue = System.getenv(envName);
        if (envValue != null && !envValue.isEmpty()) {
            return envValue;
        }
        return defaultValue;
    }

    /**
     * 告警验证函数（带 DLQ 侧输出）
     */
    private static class AlertValidationFunction extends ProcessFunction<Alert, Alert> {

        private static final long serialVersionUID = 1L;

        @Override
        public void processElement(Alert alert, Context ctx, Collector<Alert> out) {
            try {
                // 验证必要字段
                if (alert == null) {
                    ctx.output(DLQ_TAG, buildDlqMessage("null_alert", "Alert is null", null));
                    return;
                }

                if (alert.getAlertId() == null || alert.getAlertId().isEmpty()) {
                    ctx.output(DLQ_TAG, buildDlqMessage("missing_alert_id", 
                            "Alert ID is missing", alert));
                    return;
                }

                if (alert.getSrcIp() == null || alert.getSrcIp().isEmpty()) {
                    ctx.output(DLQ_TAG, buildDlqMessage("missing_src_ip", 
                            "Source IP is missing", alert));
                    return;
                }

                if (alert.getTenantId() == null || alert.getTenantId().isEmpty()) {
                    ctx.output(DLQ_TAG, buildDlqMessage("missing_tenant_id", 
                            "Tenant ID is missing", alert));
                    return;
                }

                // 验证时间戳
                if (alert.getLastSeen() <= 0) {
                    ctx.output(DLQ_TAG, buildDlqMessage("invalid_timestamp", 
                            "Invalid last_seen timestamp", alert));
                    return;
                }

                // 验证通过，输出到主流
                out.collect(alert);

            } catch (Exception e) {
                ctx.output(DLQ_TAG, buildDlqMessage("processing_error", 
                        "Error processing alert: " + e.getMessage(), alert));
            }
        }

        private String buildDlqMessage(String errorCode, String errorMessage, Alert alert) {
            StringBuilder sb = new StringBuilder();
            sb.append("{");
            sb.append("\"error_code\":\"").append(errorCode).append("\",");
            sb.append("\"error_message\":\"").append(escapeJson(errorMessage)).append("\",");
            sb.append("\"timestamp\":").append(System.currentTimeMillis()).append(",");
            sb.append("\"source\":\"cep-job\"");
            
            if (alert != null) {
                sb.append(",\"alert_id\":\"")
                  .append(alert.getAlertId() != null ? alert.getAlertId() : "")
                  .append("\"");
                sb.append(",\"tenant_id\":\"")
                  .append(alert.getTenantId() != null ? alert.getTenantId() : "")
                  .append("\"");
                sb.append(",\"src_ip\":\"")
                  .append(alert.getSrcIp() != null ? alert.getSrcIp() : "")
                  .append("\"");
                sb.append(",\"alert_type\":\"")
                  .append(alert.getAlertType() != null ? alert.getAlertType() : "")
                  .append("\"");
            }
            
            sb.append("}");
            return sb.toString();
        }

        private String escapeJson(String str) {
            if (str == null) return "";
            return str.replace("\\", "\\\\")
                      .replace("\"", "\\\"")
                      .replace("\n", "\\n")
                      .replace("\r", "\\r")
                      .replace("\t", "\\t");
        }
    }

    /**
     * 创建 DLQ Kafka Sink
     */
    private static KafkaSink<String> createDlqSink(String brokers, String topic) {
        return KafkaSink.<String>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(KafkaRecordSerializationSchema.builder()
                        .setTopic(topic)
                        .setValueSerializationSchema(new SimpleStringSchema())
                        .build())
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(com.traffic.flink.common.ConfigUtil.kafkaClientProperties())
                .build();
    }

    /**
     * 应用扫描-利用模式
     */
    private static DataStream<Campaign> applyScanExploitPattern(
            KeyedStream<Alert, String> keyedAlerts, 
            PatternConfig config
    ) {
        PatternStream<Alert> patternStream = CEP.pattern(
                keyedAlerts,
                ScanExploitPattern.create(config)
        );

        return patternStream
                .process(new ScanExploitSelector())
                .uid("scan-exploit-pattern")
                .name("Scan-Exploit Pattern");
    }

    /**
     * 应用暴力破解模式
     */
    private static DataStream<Campaign> applyBruteForcePattern(
            KeyedStream<Alert, String> keyedAlerts,
            PatternConfig config
    ) {
        PatternStream<Alert> patternStream = CEP.pattern(
                keyedAlerts,
                BruteForcePattern.create(config)
        );

        return patternStream
                .process(new BruteForceSelector())
                .uid("brute-force-pattern")
                .name("Brute Force Pattern");
    }

    /**
     * 应用横向移动模式
     */
    private static DataStream<Campaign> applyLateralMovementPattern(
            KeyedStream<Alert, String> keyedAlerts,
            PatternConfig config
    ) {
        PatternStream<Alert> patternStream = CEP.pattern(
                keyedAlerts,
                LateralMovementPattern.create(config)
        );

        return patternStream
                .process(new LateralMovementSelector())
                .uid("lateral-movement-pattern")
                .name("Lateral Movement Pattern");
    }

    /**
     * 应用数据外泄模式
     */
    private static DataStream<Campaign> applyDataExfilPattern(
            KeyedStream<Alert, String> keyedAlerts,
            PatternConfig config
    ) {
        PatternStream<Alert> patternStream = CEP.pattern(
                keyedAlerts,
                DataExfilPattern.create(config)
        );

        return patternStream
                .process(new DataExfilSelector())
                .uid("data-exfil-pattern")
                .name("Data Exfiltration Pattern");
    }

    /**
     * 应用 C2 信标模式
     */
    private static DataStream<Campaign> applyC2BeaconPattern(
            KeyedStream<Alert, String> keyedAlerts,
            PatternConfig config
    ) {
        PatternStream<Alert> patternStream = CEP.pattern(
                keyedAlerts,
                C2BeaconPattern.create(config)
        );

        return patternStream
                .process(new C2BeaconSelector())
                .uid("c2-beacon-pattern")
                .name("C2 Beacon Pattern");
    }

    /**
     * 加载模式配置
     */
    private static PatternConfig loadPatternConfig(ParameterTool params) {
        PatternConfig config = new PatternConfig();
        
        // 扫描-利用模式配置
        config.setScanExploitWindowMinutes(
                ConfigUtils.getInt(params, "pattern.scan_exploit.window_minutes", 30));
        config.setMinScanCount(
                ConfigUtils.getInt(params, "pattern.scan_exploit.min_scan_count", 1));
        
        // 暴力破解模式配置
        config.setBruteForceWindowMinutes(
                ConfigUtils.getInt(params, "pattern.brute_force.window_minutes", 10));
        config.setMinFailedAttempts(
                ConfigUtils.getInt(params, "pattern.brute_force.min_failed", 5));
        
        // 横向移动模式配置
        config.setLateralMovementWindowMinutes(
                ConfigUtils.getInt(params, "pattern.lateral_movement.window_minutes", 60));
        config.setMinHops(
                ConfigUtils.getInt(params, "pattern.lateral_movement.min_hops", 2));
        
        // 数据外泄模式配置
        config.setDataExfilWindowMinutes(
                ConfigUtils.getInt(params, "pattern.data_exfil.window_minutes", 30));
        config.setMinExfilBytes(
                ConfigUtils.getLong(params, "pattern.data_exfil.min_bytes", 10 * 1024 * 1024));
        
        // C2 信标模式配置
        config.setC2WindowMinutes(
                ConfigUtils.getInt(params, "pattern.c2.window_minutes", 120));
        config.setMinBeaconCount(
                ConfigUtils.getInt(params, "pattern.c2.min_beacon_count", 5));
        config.setBeaconIntervalToleranceRatio(
                ConfigUtils.getFloat(params, "pattern.c2.beacon_interval_tolerance_ratio", 0.1f));
        
        return config;
    }

    /**
     * 配置 Checkpoint
     */
    private static void configureCheckpoint(
            StreamExecutionEnvironment env,
            String checkpointPath,
            long intervalMs
    ) {
        // 启用 Checkpoint
        env.enableCheckpointing(intervalMs, CheckpointingMode.EXACTLY_ONCE);

        CheckpointConfig config = env.getCheckpointConfig();

        // CEP 作业状态较大，需要更长的超时时间
        config.setCheckpointTimeout(180000); // 3 分钟

        // Checkpoint 之间的最小间隔
        config.setMinPauseBetweenCheckpoints(intervalMs / 2);

        // 同时只允许一个 Checkpoint
        config.setMaxConcurrentCheckpoints(1);

        // 作业取消时保留 Checkpoint
        config.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION
        );

        // 使用 RocksDB 状态后端（支持增量 Checkpoint）
        // Flink 1.18: EmbeddedRocksDBStateBackend 构造函数参数 enableIncrementalCheckpointing
        // RocksDB 线程数通过 flink-conf.yaml 的 state.backend.rocksdb.thread.num 配置
        env.setStateBackend(new EmbeddedRocksDBStateBackend(true));

        // 设置 Checkpoint 存储路径
        config.setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));
    }
}
