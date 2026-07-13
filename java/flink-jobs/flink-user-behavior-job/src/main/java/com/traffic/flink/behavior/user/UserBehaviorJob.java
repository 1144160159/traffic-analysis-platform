package com.traffic.flink.behavior.user;

import com.traffic.flink.behavior.user.detector.*;
import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.behavior.user.sink.*;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.ProtoDeserializer;
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
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.kafka.clients.producer.ProducerConfig;
import org.apache.kafka.clients.producer.ProducerRecord;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import javax.annotation.Nullable;
import java.nio.charset.StandardCharsets;
import java.util.Locale;
import java.util.Properties;
import java.util.UUID;

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

    public static void main(String[] args) throws Exception {
        ParameterTool params = ConfigUtils.loadConfig(args, "user-behavior-job.properties");

        String kafkaBrokers = ConfigUtils.get(params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", "user.events.v1");
        String outputTopic = ConfigUtils.get(params, "kafka.output.topic", "alerts.v1");
        String groupId = ConfigUtils.get(params, "kafka.group.id", "flink-user-behavior-job");
        String checkpointPath = ConfigUtils.get(
                params,
                "checkpoint.path",
                "s3://flink-checkpoints/checkpoints/user-behavior-job");
        long checkpointIntervalMs = ConfigUtils.getLong(params, "checkpoint.interval.ms", 60_000L);
        long checkpointTimeoutMs = ConfigUtils.getLong(params, "checkpoint.timeout.ms", 600_000L);
        int parallelism = ConfigUtils.getInt(params, "parallelism", 2);
        String clickhouseUrl = ConfigUtils.get(
                params,
                "clickhouse.url",
                "jdbc:clickhouse://clickhouse-1.middleware.svc:8123/traffic");
        String clickhouseUser = ConfigUtils.get(params, "clickhouse.user", "default");
        String clickhousePassword = ConfigUtils.get(params, "clickhouse.password", "");

        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(parallelism);
        env.enableCheckpointing(checkpointIntervalMs, CheckpointingMode.AT_LEAST_ONCE);
        env.getCheckpointConfig().setCheckpointTimeout(checkpointTimeoutMs);
        env.getCheckpointConfig().setCheckpointStorage(new FileSystemCheckpointStorage(checkpointPath));

        KafkaSource<UserEvent> source = KafkaSource.<UserEvent>builder()
                .setBootstrapServers(kafkaBrokers)
                .setTopics(inputTopic)
                .setGroupId(groupId)
                .setStartingOffsets(OffsetsInitializer.earliest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(UserEvent.class))
                .setProperties(ConfigUtils.kafkaClientProperties(params))
                .build();

        DataStream<UserEvent> events = env.fromSource(source,
                WatermarkStrategy.<UserEvent>forMonotonousTimestamps()
                        .withTimestampAssigner((e, ts) -> e.getTimestamp()),
                "Kafka-UserEvents")
                .uid("kafka-source").name("Kafka Source (user.events.v1)")
                .filter(e -> e != null && e.getEventId() != null && !e.getEventId().isEmpty())
                .uid("null-filter").name("Filter Invalid Events");

        // Detector 1: Impossible Travel
        DataStream<AnomalyEvent> travelAnomalies = events
                .keyBy(e -> e.getTenantId() + "|" + e.getUserId())
                .process(new ImpossibleTravelDetector())
                .uid("travel-detector").name("Impossible Travel Detector");

        // Detector 2: Brute Force Login
        DataStream<AnomalyEvent> bruteAnomalies = events
                .keyBy(e -> e.getTenantId() + "|" + e.getUserId())
                .process(new BruteForceLoginDetector())
                .uid("brute-detector").name("Brute Force Login Detector");

        // Detector 3: Privilege Escalation
        DataStream<AnomalyEvent> privAnomalies = events
                .keyBy(e -> e.getTenantId() + "|" + e.getUserId())
                .process(new PrivilegeEscalationDetector())
                .uid("priv-detector").name("Privilege Escalation Detector");

        // Merge all anomaly streams
        DataStream<AnomalyEvent> allAnomalies = travelAnomalies.union(bruteAnomalies, privAnomalies)
                .filter(a -> a != null)
                .uid("merge-anomalies").name("Merge All Anomalies");

        // Sink 1: Kafka alerts.v1 (protobuf Alert, shared downstream contract)
        KafkaSink<Alert> alertSink = createAlertSink(kafkaBrokers, outputTopic);
        allAnomalies
                .map(UserBehaviorJob::toAlert)
                .uid("anomaly-to-alert").name("Convert AnomalyEvent to Alert")
                .sinkTo(alertSink)
                .uid("alert-kafka-sink").name("Kafka Sink (" + outputTopic + ")");

        // Sink 2: ClickHouse
        allAnomalies.addSink(new ClickHouseAnomalySink(clickhouseUrl, clickhouseUser, clickhousePassword))
                .uid("ch-sink").name("ClickHouse Sink (user_anomalies)");

        LOG.info("User Behavior Job started: input={}, output={}, checkpoint={}, parallelism={}",
                inputTopic, outputTopic, checkpointPath, parallelism);
        env.execute("User Behavior Anomaly Detection Job");
    }

    static Alert toAlert(AnomalyEvent anomaly) {
        long eventTime = anomaly.detectedAt > 0 ? anomaly.detectedAt : System.currentTimeMillis();
        String tenantId = nonBlank(anomaly.tenantId, "default");
        String userId = nonBlank(anomaly.userId, "unknown-user");
        String detectorType = nonBlank(anomaly.detectorType, "USER_BEHAVIOR");
        String alertType = "user_behavior." + detectorType.toLowerCase(Locale.ROOT);
        String alertId = nonBlank(anomaly.anomalyId,
                UUID.nameUUIDFromBytes((tenantId + ":" + userId + ":" + detectorType + ":" + eventTime)
                        .getBytes(StandardCharsets.UTF_8)).toString());
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
                if (score >= 0.9f) return Severity.SEVERITY_CRITICAL;
                if (score >= 0.7f) return Severity.SEVERITY_HIGH;
                if (score >= 0.5f) return Severity.SEVERITY_MEDIUM;
                if (score >= 0.3f) return Severity.SEVERITY_LOW;
                return Severity.SEVERITY_INFO;
        }
    }

    private static KafkaSink<Alert> createAlertSink(String brokers, String topic) {
        Properties producerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        producerProps.setProperty(ProducerConfig.ENABLE_IDEMPOTENCE_CONFIG, "true");
        producerProps.setProperty(ProducerConfig.ACKS_CONFIG, "all");
        producerProps.setProperty(ProducerConfig.COMPRESSION_TYPE_CONFIG, "lz4");

        return KafkaSink.<Alert>builder()
                .setBootstrapServers(brokers)
                .setRecordSerializer(new AlertKafkaSerializer(topic))
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
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
}
