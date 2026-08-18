package com.traffic.flink.behavior.user;

import com.traffic.flink.behavior.user.detector.*;
import com.traffic.flink.behavior.user.baseline.BaselineActivationAck;
import com.traffic.flink.behavior.user.baseline.BaselineActivationAckKafkaSinkFactory;
import com.traffic.flink.behavior.user.baseline.BaselineAwareUserEvent;
import com.traffic.flink.behavior.user.baseline.BaselineLifecycleEvent;
import com.traffic.flink.behavior.user.baseline.BaselineLifecycleParseFunction;
import com.traffic.flink.behavior.user.baseline.BaselineLifecycleProcessFunction;
import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.behavior.user.sink.*;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.DeploymentActivation;
import com.traffic.flink.common.DeterministicId;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.common.sourcefact.SourceFactClickHouseSink;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.AlertStatus;
import com.traffic.proto.traffic.v1.Severity;
import com.traffic.proto.traffic.v1.UserEvent;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.util.OutputTag;
import org.apache.kafka.clients.consumer.OffsetResetStrategy;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.Properties;

/**
 * Flink User Behavior Job — 用户行为异常检测
 *
 * 业务管线:
 *   Keycloak/APISIX → Kafka user.events.v1
 *   → Flink User Behavior Job
 *       ├── ImpossibleTravelDetector (异地登录: 30min内不同城市)
 *       ├── BruteForceDetector (暴力破解: 5次失败→成功)
 *       ├── PrivilegeEscalationDetector (权限提升: viewer→admin)
 *       └── UnusualAccessDetector (异常时间/异常IP访问)
 *   → AnomalyEvent → Kafka alerts.v1 + ClickHouse
 */
public class UserBehaviorJob {
    private static final Logger LOG = LoggerFactory.getLogger(UserBehaviorJob.class);
    private static final OutputTag<CanonicalDlqMessage> PARSE_DLQ =
            new OutputTag<CanonicalDlqMessage>("user-event-parse-dlq-v1") {};
    private static final OutputTag<CanonicalDlqMessage> QUALITY_DLQ =
            new OutputTag<CanonicalDlqMessage>("user-event-quality-dlq-v1") {};
    private static final OutputTag<SourceQualityReceipt> PARSE_QUALITY =
            new OutputTag<SourceQualityReceipt>("user-event-parse-quality-v1") {};
    private static final OutputTag<SourceQualityReceipt> EVENT_QUALITY =
            new OutputTag<SourceQualityReceipt>("user-event-terminal-quality-v1") {};
    private static final OutputTag<ValidatedUserEvent> ACCEPTED_SOURCE_FACT =
            new OutputTag<ValidatedUserEvent>("user-event-accepted-source-fact-v1") {};

    public static void main(String[] args) throws Exception {
        ParameterTool params = ConfigUtils.loadConfig(args, "user-behavior-job.properties");

        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", "user.events.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "alerts.v1");
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", "dlq.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-user-behavior-job");
        DeploymentActivation activation = DeploymentActivation.from(
                params, "flink-user-behavior-job", groupId);
        boolean legacyProjectionDefault =
                activation.getMode() == DeploymentActivation.Mode.LEGACY;
        boolean projectionWritesEnabled = ConfigUtils.getBoolean(
                params, "projection.writes.enabled", legacyProjectionDefault);
        boolean sourceFactWritesEnabled = ConfigUtils.getBoolean(
                params, "source.fact.writes.enabled", false);
        if (projectionWritesEnabled
                && activation.getMode() == DeploymentActivation.Mode.SHADOW) {
            throw new IllegalArgumentException("shadow activation must not enable user projections");
        }
        if (sourceFactWritesEnabled && !activation.externalWritesEnabled()) {
            throw new IllegalArgumentException(
                    "source-fact ClickHouse writes require an externally writable activation");
        }
        String checkpointPath = ConfigUtils.get(
                params,
                "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/user-behavior-job");
        long checkpointIntervalMs = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60_000L);
        long checkpointTimeoutMs = ConfigUtils.getLong(params, "checkpoint.timeout.ms", 600_000L);
        int parallelism = ConfigUtils.getInt(params, "parallelism", 2);
        long maxOutOfOrderSeconds = ConfigUtils.getLong(
                params, "watermark.max.out.of.orderness.seconds", 30L);
        long allowedLatenessSeconds = ConfigUtils.getLong(
                params, "watermark.allowed.lateness.seconds", 120L);
        long watermarkIdlenessMs = ConfigUtils.getLong(params, "watermark.idleness.ms", 60_000L);
        long maxFutureSkewMs = ConfigUtils.getLong(params, "event.max.future.skew.ms", 300_000L);
        long maxClockRollbackMs = ConfigUtils.getLong(params, "event.max.clock.rollback.ms", 0L);
        long kafkaTransactionTimeoutMs = ConfigUtils.getLong(
                params, "kafka.transaction.timeout.ms", 900_000L);
        String auditTopic = ConfigUtils.get(params, "kafka.audit.topic", "audit.logs");
        String suffix = activation.isCandidateBound()
                ? "-" + activation.getCandidateSha256().substring(0, 12) : "";
        String dlqTransactionPrefix = ConfigUtils.get(
                params, "kafka.dlq.transactional.id.prefix",
                "flink-user-behavior-dlq-v1" + suffix);
        String alertTransactionPrefix = ConfigUtils.get(
                params, "kafka.alert.transactional.id.prefix",
                "flink-user-behavior-alert-v1" + suffix);
        String qualityTransactionPrefix = ConfigUtils.get(
                params, "kafka.quality.transactional.id.prefix",
                "flink-user-behavior-quality-v1" + suffix);
        boolean baselineLifecycleEnabled = ConfigUtils.getBoolean(
                params, "baseline.lifecycle.enabled", false);
        String baselineLifecycleTopic = ConfigUtils.get(
                params, "kafka.baseline.lifecycle.topic", "baseline.lifecycle.v1");
        String baselineLifecycleGroup = ConfigUtils.get(
                params, "kafka.baseline.lifecycle.group", "flink-user-behavior-baseline-v1");
        String baselineAckTopic = ConfigUtils.get(
                params, "kafka.baseline.ack.topic", "baseline.activation-acks.v1");
        String baselineConsumerId = ConfigUtils.get(
                params, "baseline.consumer.id", "flink-user-behavior-job");
        String baselineAckTransactionPrefix = ConfigUtils.get(
                params, "kafka.baseline.ack.transactional.id.prefix",
                "flink-user-behavior-baseline-ack-v1" + suffix);
        if (!"user.events.v1".equals(inputTopic)
                || !"dlq.v1".equals(dlqTopic)
                || !"audit.logs".equals(auditTopic)) {
            throw new IllegalArgumentException("user behavior input, DLQ and audit topics are canonical and pinned");
        }
        if (dlqTransactionPrefix.equals(qualityTransactionPrefix)
                || dlqTransactionPrefix.equals(alertTransactionPrefix)
                || alertTransactionPrefix.equals(qualityTransactionPrefix)) {
            throw new IllegalArgumentException(
                    "DLQ, alert and quality transactional prefixes must differ");
        }
        if (baselineLifecycleEnabled) {
            if (activation.getMode() != DeploymentActivation.Mode.PRODUCTION
                    || !activation.isCandidateBound() || !activation.externalWritesEnabled()
                    || !"baseline.lifecycle.v1".equals(baselineLifecycleTopic)
                    || !"flink-user-behavior-baseline-v1".equals(baselineLifecycleGroup)
                    || !"baseline.activation-acks.v1".equals(baselineAckTopic)
                    || !"flink-user-behavior-job".equals(baselineConsumerId)
                    || baselineAckTransactionPrefix.equals(dlqTransactionPrefix)
                    || baselineAckTransactionPrefix.equals(qualityTransactionPrefix)
                    || baselineAckTransactionPrefix.equals(alertTransactionPrefix)) {
                throw new IllegalArgumentException(
                        "behavior baseline lifecycle requires the production candidate and exact topic/group/consumer identities");
            }
        }
        if (kafkaTransactionTimeoutMs <= checkpointTimeoutMs + checkpointIntervalMs) {
            throw new IllegalArgumentException("Kafka transaction timeout must exceed checkpoint timeout plus interval");
        }
        EventTimePolicy eventTimePolicy = new EventTimePolicy(
                maxOutOfOrderSeconds * 1000L,
                watermarkIdlenessMs,
                allowedLatenessSeconds * 1000L,
                maxFutureSkewMs,
                maxClockRollbackMs);
        String clickhouseUrl = ConfigUtils.get(
                params,
                "clickhouse.url",
                "jdbc:clickhouse://clickhouse-1.middleware.svc:8123/traffic");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");
        String clickhouseAnomalyTable = ConfigUtils.get(
                params, "clickhouse.anomaly.table", "traffic.user_anomalies_v2");
        String clickhouseSourceFactTable = ConfigUtils.get(
                params, "clickhouse.source.fact.table", "traffic.source_user_behavior_facts_v1");
        if (sourceFactWritesEnabled
                && !"traffic.source_user_behavior_facts_v1".equals(clickhouseSourceFactTable)) {
            throw new IllegalArgumentException(
                    "user source facts are pinned to traffic.source_user_behavior_facts_v1");
        }
        int clickhouseBatchSize = ConfigUtils.getInt(params, "clickhouse.batch.size", 500);
        long clickhouseBatchIntervalMs = ConfigUtils.getLong(
                params, "clickhouse.batch.interval.ms", 2_000L);
        int clickhouseMaxRetries = ConfigUtils.getInt(params, "clickhouse.max.retries", 3);
        String replayId = ConfigUtils.get(params, "replay.id", "");

        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);
        env.enableCheckpointing(checkpointIntervalMs, CheckpointingMode.EXACTLY_ONCE);
        env.getCheckpointConfig().setCheckpointTimeout(checkpointTimeoutMs);
        env.getCheckpointConfig().setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));
        env.getCheckpointConfig().setMinPauseBetweenCheckpoints(checkpointIntervalMs / 2);
        env.getCheckpointConfig().setMaxConcurrentCheckpoints(1);
        env.getCheckpointConfig().setExternalizedCheckpointCleanup(
                org.apache.flink.streaming.api.environment.CheckpointConfig
                        .ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION);
        // RocksDB 状态后端（增量），检测器 keyed state 与基线广播态与其余作业一致
        org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend stateBackend =
                new org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend(true);
        env.setStateBackend(stateBackend);
        // 显式重启策略：固定延迟重启，覆盖短时 Kafka/存储故障窗口
        env.setRestartStrategy(org.apache.flink.api.common.restartstrategy.RestartStrategies
                .fixedDelayRestart(
                        ConfigUtils.getInt(params, "restart.attempts", 10),
                        org.apache.flink.api.common.time.Time.seconds(
                                ConfigUtils.getInt(params, "restart.delay.seconds", 30))));

        Properties consumerProperties = ConfigUtils.kafkaClientProperties(params);
        consumerProperties.setProperty("enable.auto.commit", "false");
        consumerProperties.setProperty("commit.offsets.on.checkpoint", "true");
        consumerProperties.setProperty("isolation.level", "read_committed");
        KafkaSource<RawKafkaRecord> source = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.earliest())
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(consumerProperties)
                .build();

        DataStream<RawKafkaRecord> rawEvents = env.fromSource(
                source, WatermarkStrategy.noWatermarks(), "Kafka-RawUserEvents")
                .uid("user-event-raw-kafka-source-v2")
                .name("Kafka Raw Source (user.events.v1)");
        SingleOutputStreamOperator<ValidatedUserEvent> parsedEvents = rawEvents
                .process(new UserEventParseFunction(
                        inputTopic, groupId, eventTimePolicy, PARSE_DLQ, PARSE_QUALITY))
                .uid("user-event-strict-parser-v1")
                .name("Strict UserEvent envelope parser");
        DataStream<ValidatedUserEvent> timestampedEvents = parsedEvents
                .assignTimestampsAndWatermarks(eventTimePolicy.watermarkStrategy(
                        value -> value.getEvent().getTimestamp()))
                .uid("user-event-watermark-v1")
                .name("Shared UserEvent event-time watermark");
        SingleOutputStreamOperator<UserEvent> events = timestampedEvents
                .keyBy(ValidatedUserEvent::identityKey)
                .process(new UserEventTimeFunction(
                        eventTimePolicy, groupId, QUALITY_DLQ, EVENT_QUALITY,
                        ACCEPTED_SOURCE_FACT))
                .uid("user-event-quality-barrier-v1")
                .name("UserEvent duplicate late and conflict barrier");

        parsedEvents.getSideOutput(PARSE_DLQ)
                .union(events.getSideOutput(QUALITY_DLQ))
                .sinkTo(createCanonicalDeadLetterSink(
                        kafkaBrokers, dlqTopic, dlqTransactionPrefix,
                        kafkaTransactionTimeoutMs, params))
                .uid("user-event-canonical-dlq-sink-v1")
                .name("Checkpoint-coupled canonical UserEvent DLQ");
        parsedEvents.getSideOutput(PARSE_QUALITY)
                .union(events.getSideOutput(EVENT_QUALITY))
                .sinkTo(createSourceQualitySink(
                        kafkaBrokers, auditTopic, qualityTransactionPrefix,
                        kafkaTransactionTimeoutMs, params))
                .uid("user-event-source-quality-sink-v1")
                .name("Checkpoint-coupled UserEvent quality receipts");

        if (sourceFactWritesEnabled) {
            events.getSideOutput(ACCEPTED_SOURCE_FACT)
                    .map(input -> toUserSourceFact(input, groupId))
                    .uid("user-event-source-fact-mapper-v1")
                    .name("Map accepted UserEvent source facts")
                    .addSink(new SourceFactClickHouseSink(
                            clickhouseUrl,
                            clickhouseSourceFactTable,
                            clickhouseUser,
                            clickhousePassword,
                            clickhouseBatchSize,
                            clickhouseMaxRetries))
                    .uid("user-event-source-fact-clickhouse-v1")
                    .name("ClickHouse source_user_behavior_facts_v1");
        }

        DataStream<BaselineAwareUserEvent> baselineAwareEvents;
        if (baselineLifecycleEnabled) {
            Properties baselineConsumerProperties = ConfigUtils.kafkaClientProperties(params);
            baselineConsumerProperties.setProperty("enable.auto.commit", "false");
            baselineConsumerProperties.setProperty("commit.offsets.on.checkpoint", "true");
            baselineConsumerProperties.setProperty("isolation.level", "read_committed");
            KafkaSource<RawKafkaRecord> baselineSource = KafkaSource.<RawKafkaRecord>builder()
                    .setBootstrapServers(kafkaBrokers)
                    .setTopics(baselineLifecycleTopic)
                    .setGroupId(baselineLifecycleGroup)
                    .setStartingOffsets(OffsetsInitializer.committedOffsets(OffsetResetStrategy.EARLIEST))
                    .setDeserializer(new RawKafkaRecordDeserializationSchema())
                    .setProperties(baselineConsumerProperties)
                    .build();
            DataStream<BaselineLifecycleEvent> baselineEvents = env
                    .fromSource(baselineSource, WatermarkStrategy.noWatermarks(), "Kafka-BaselineLifecycle")
                    .uid("behavior-baseline-lifecycle-source-v1")
                    .name("Kafka Source (baseline.lifecycle.v1)")
                    .process(new BaselineLifecycleParseFunction(
                            baselineLifecycleTopic, activation.getCandidateSha256(), baselineConsumerId))
                    .uid("behavior-baseline-lifecycle-parser-v1")
                    .name("Strict behavior baseline lifecycle parser");
            SingleOutputStreamOperator<BaselineAwareUserEvent> applied = events
                    .connect(baselineEvents.broadcast(
                            BaselineLifecycleProcessFunction.STAGED_BASELINES,
                            BaselineLifecycleProcessFunction.ACTIVE_BASELINES,
                            BaselineLifecycleProcessFunction.PROCESSED_EVENTS))
                    .process(new BaselineLifecycleProcessFunction(baselineConsumerId))
                    .uid("behavior-baseline-stage-activate-v1")
                    .name("Stage and activate checkpointed behavior baselines");
            DataStream<BaselineActivationAck> activationAcks =
                    applied.getSideOutput(BaselineLifecycleProcessFunction.ACTIVATION_ACKS);
            activationAcks.sinkTo(BaselineActivationAckKafkaSinkFactory.create(
                            kafkaBrokers, baselineAckTopic, baselineAckTransactionPrefix,
                            kafkaTransactionTimeoutMs, params))
                    .uid("behavior-baseline-activation-ack-sink-v1")
                    .name("Exactly-once Kafka Sink (baseline.activation-acks.v1)");
            baselineAwareEvents = applied;
        } else {
            baselineAwareEvents = events
                    .map(event -> new BaselineAwareUserEvent(event, null))
                    .returns(BaselineAwareUserEvent.class)
                    .uid("behavior-baseline-compatibility-wrapper-v1")
                    .name("Behavior baseline compatibility wrapper");
        }

        // Detector 1: Impossible Travel
        DataStream<AnomalyEvent> travelAnomalies = baselineAwareEvents
                .keyBy(e -> e.getEvent().getTenantId() + "|" + e.getEvent().getUserId())
                .process(new ImpossibleTravelDetector())
                .uid("travel-detector").name("Impossible Travel Detector");

        // Detector 2: Brute Force Login
        DataStream<AnomalyEvent> bruteAnomalies = baselineAwareEvents
                .keyBy(e -> e.getEvent().getTenantId() + "|" + e.getEvent().getUserId())
                .process(new BruteForceLoginDetector())
                .uid("brute-detector").name("Brute Force Login Detector");

        // Detector 3: Privilege Escalation
        DataStream<AnomalyEvent> privAnomalies = baselineAwareEvents
                .keyBy(e -> e.getEvent().getTenantId() + "|" + e.getEvent().getUserId())
                .process(new PrivilegeEscalationDetector())
                .uid("priv-detector").name("Privilege Escalation Detector");

        // Merge all anomaly streams
        DataStream<AnomalyEvent> allAnomalies = travelAnomalies.union(bruteAnomalies, privAnomalies)
                .filter(a -> a != null)
                .uid("merge-anomalies").name("Merge All Anomalies")
                .map(anomaly -> {
                    anomaly.replayId = replayId;
                    if (anomaly.eventVersion <= 0) {
                        anomaly.eventVersion = 1L;
                    }
                    return anomaly;
                })
                .returns(AnomalyEvent.class)
                .uid("mark-replay-context").name("Mark Replay Context");
        if (projectionWritesEnabled) {
            // Sink 1: Kafka alerts.v1 (protobuf Alert, shared downstream contract)
            KafkaSink<Alert> alertSink = createAlertSink(
                    kafkaBrokers, outputTopic, alertTransactionPrefix, kafkaTransactionTimeoutMs);
            allAnomalies
                    .map(UserBehaviorJob::toAlert)
                    .uid("anomaly-to-alert").name("Convert AnomalyEvent to Alert")
                    .sinkTo(alertSink)
                    .uid("alert-kafka-sink").name("Kafka Sink (" + outputTopic + ")");

            // Sink 2: ClickHouse
            allAnomalies.addSink(new ClickHouseAnomalySink(
                            clickhouseUrl,
                            clickhouseUser,
                            clickhousePassword,
                            clickhouseAnomalyTable,
                            clickhouseBatchSize,
                            clickhouseBatchIntervalMs,
                            clickhouseMaxRetries))
                    .uid("ch-sink").name("ClickHouse Sink (user_anomalies_v2)");
        } else {
            LOG.info("Consumer-ready mode: alerts.v1 and ClickHouse projections are disabled");
        }

        LOG.info("User Behavior Job started: input={}, output={}, checkpoint={}, parallelism={}",
                inputTopic, outputTopic, checkpointPath, parallelism);
        env.execute("User Behavior Anomaly Detection Job");
    }

    static SourceFactRecord toUserSourceFact(
            ValidatedUserEvent input, String consumerGroup) {
        UserEvent event = input.getEvent();
        return SourceFactRecord.fromAccepted(
                "user_behavior",
                event.getTenantId(),
                event.getUserId(),
                event.getEventId(),
                event.getTimestamp(),
                input.getSource().getTimestamp(),
                "v1",
                input.getSource(),
                consumerGroup,
                input.getSource().getOffset() + 1L);
    }

    static Alert toAlert(AnomalyEvent anomaly) {
        long eventTime = anomaly.detectedAt > 0 ? anomaly.detectedAt : System.currentTimeMillis();
        String tenantId = nonBlank(anomaly.tenantId, "default");
        String userId = nonBlank(anomaly.userId, "unknown-user");
        String detectorType = nonBlank(anomaly.detectorType, "USER_BEHAVIOR");
        String alertType = "user_behavior." + detectorType.toLowerCase(Locale.ROOT);
        String alertId = nonBlank(anomaly.anomalyId,
                DeterministicId.uuid(
                        "flink-user-anomaly-fallback/v1",
                        tenantId,
                        userId,
                        detectorType,
                        eventTime));
        String srcIp = nonBlank(anomaly.sourceIp2, nonBlank(anomaly.sourceIp1, "0.0.0.0"));
        String fingerprint = tenantId + ":" + userId + ":" + detectorType;

        return Alert.newBuilder()
                .setTenantId(tenantId)
                .setAlertId(alertId)
                .setFirstSeen(eventTime)
                .setLastSeen(eventTime)
                .setSeverity(mapSeverity(anomaly.severity, anomaly.score))
                .setAlertType(alertType)
                .setScore(anomaly.score)
                .addLabels("user_behavior")
                .addLabels(detectorType)
                .setSrcIp(srcIp)
                .setDstIp("0.0.0.0")
                .setSrcPort(0)
                .setDstPort(0)
                .setProtocol(0)
                .setProtocolName("USER")
                .setCommunityId(tenantId + ":" + userId)
                .setSessionId(userId)
                .setCampaignId("")
                .setModelVersion("user-behavior-rules-v1")
                .setRuleVersion(detectorType)
                .setFeatureSetId("user-behavior")
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .setDedupFingerprint(fingerprint)
                .setUpdatedTs(eventTime)
                .setEventId("user-anomaly:" + alertId)
                .setIngestTs(System.currentTimeMillis())
                .setCount(1)
                .setArkimeSessionLink("")
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(1)
                .build();
    }

    /**
     * 严重度阈值（与告警生成器口径一致，提为命名常量避免散落魔法数字）
     */
    private static final float SEVERITY_CRITICAL_THRESHOLD = 0.9f;
    private static final float SEVERITY_HIGH_THRESHOLD = 0.7f;
    private static final float SEVERITY_MEDIUM_THRESHOLD = 0.5f;
    private static final float SEVERITY_LOW_THRESHOLD = 0.3f;

    private static Severity mapSeverity(String severity, float score) {
        String normalized = severity == null ? "" : severity.toLowerCase(Locale.ROOT);
        switch (normalized) {
            case "critical":
                return Severity.SEVERITY_CRITICAL;
            case "high":
                return Severity.SEVERITY_HIGH;
            case "medium":
                return Severity.SEVERITY_MEDIUM;
            case "low":
                return Severity.SEVERITY_LOW;
            case "info":
                return Severity.SEVERITY_INFO;
            default:
                if (score >= SEVERITY_CRITICAL_THRESHOLD) return Severity.SEVERITY_CRITICAL;
                if (score >= SEVERITY_HIGH_THRESHOLD) return Severity.SEVERITY_HIGH;
                if (score >= SEVERITY_MEDIUM_THRESHOLD) return Severity.SEVERITY_MEDIUM;
                if (score >= SEVERITY_LOW_THRESHOLD) return Severity.SEVERITY_LOW;
                return Severity.SEVERITY_INFO;
        }
    }

    /**
     * 创建 Alert Kafka Sink（EXACTLY_ONCE，事务与 checkpoint 耦合；
     * 与 DLQ/quality sink 语义一致，重启不重复发 alert）
     */
    private static KafkaSink<Alert> createAlertSink(
            String brokers,
            String topic,
            String transactionalPrefix,
            long transactionTimeoutMs) {
        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        producerProps.setProperty(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG,
                String.valueOf(transactionTimeoutMs));

        return KafkaSink.<Alert>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AlertKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(transactionalPrefix)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    private static KafkaSink<CanonicalDlqMessage> createCanonicalDeadLetterSink(
            String brokers,
            String topic,
            String transactionalPrefix,
            long transactionTimeoutMs,
            ParameterTool params) {
        Properties producerProps = ConfigUtils.kafkaClientProperties(params);
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");
        producerProps.setProperty(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG,
                String.valueOf(transactionTimeoutMs));

        return KafkaSink.<CanonicalDlqMessage>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new CanonicalDeadLetterKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(transactionalPrefix)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    private static KafkaSink<SourceQualityReceipt> createSourceQualitySink(
            String brokers,
            String topic,
            String transactionalPrefix,
            long transactionTimeoutMs,
            ParameterTool params) {
        Properties producerProps = ConfigUtils.kafkaClientProperties(params);
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.TRANSACTION_TIMEOUT_CONFIG,
                String.valueOf(transactionTimeoutMs));
        return KafkaSink.<SourceQualityReceipt>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new SourceQualityKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.EXACTLY_ONCE)
                .setTransactionalIdPrefix(transactionalPrefix)
                .setKafkaProducerConfig(producerProps)
                .build();
    }

    private static String nonBlank(String value, String fallback) {
        return value == null || value.isBlank() ? fallback : value;
    }

    private static class AlertKafkaSerializer implements KafkaRecordSerializationSchema<Alert> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        AlertKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(Alert element, KafkaSinkContext context, Long timestamp) {
            if (element == null) {
                return null;
            }
            String key = nonBlank(element.getTenantId(), "unknown") + ":" +
                    nonBlank(element.getCommunityId(), element.getAlertId());
            Long recordTimestamp = element.getLastSeen() > 0 ? element.getLastSeen() : null;
            return new ProducerRecord<>(
                    topic,
                    null,
                    recordTimestamp,
                    key.getBytes(StandardCharsets.UTF_8),
                    element.toByteArray());
        }
    }

    static class CanonicalDeadLetterKafkaSerializer
            implements KafkaRecordSerializationSchema<CanonicalDlqMessage> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        CanonicalDeadLetterKafkaSerializer(String topic) {
            this.topic = topic;
        }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                CanonicalDlqMessage element, KafkaSinkContext context, Long timestamp) {
            if (element == null) {
                return null;
            }
            return new ProducerRecord<>(
                    topic,
                    null,
                    timestamp,
                    (element.tenantId() + ":" + element.originalTopic() + ":"
                            + element.originalPartition() + ":" + element.originalOffset())
                            .getBytes(StandardCharsets.UTF_8),
                    element.toJson().getBytes(StandardCharsets.UTF_8));
        }
    }

    static class SourceQualityKafkaSerializer
            implements KafkaRecordSerializationSchema<SourceQualityReceipt> {
        private static final long serialVersionUID = 1L;
        private final String topic;

        SourceQualityKafkaSerializer(String topic) { this.topic = topic; }

        @Nullable
        @Override
        public ProducerRecord<byte[], byte[]> serialize(
                SourceQualityReceipt element, KafkaSinkContext context, Long ignored) {
            if (element == null) return null;
            return new ProducerRecord<>(
                    topic,
                    element.getTenantId().getBytes(StandardCharsets.UTF_8),
                    element.toAuditEventJson());
        }
    }
}
