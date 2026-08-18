package com.traffic.flink.log;

import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.DeploymentActivation;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import org.apache.flink.api.java.utils.ParameterTool;

import java.io.Serializable;
import java.util.Objects;
import java.util.Properties;
import java.util.regex.Pattern;

/** Validated runtime contract for the device-log consumer. */
public final class LogJobConfig implements Serializable {
    private static final long serialVersionUID = 1L;

    public static final String INPUT_TOPIC = "device.logs.v1";
    public static final String DLQ_TOPIC = "dlq.v1";
    public static final String AUDIT_TOPIC = "audit.logs";
    public static final String CANONICAL_GROUP = "flink-log-job";

    private static final Pattern TRANSACTIONAL_PREFIX =
            Pattern.compile("[A-Za-z0-9._-]{1,200}");

    private final String kafkaBrokers;
    private final String inputTopic;
    private final String dlqTopic;
    private final String auditTopic;
    private final String consumerGroup;
    private final String dlqTransactionalIdPrefix;
    private final String qualityTransactionalIdPrefix;
    private final DeploymentActivation activation;
    private final boolean projectionWritesEnabled;
    private final boolean sourceFactWritesEnabled;
    private final String clickhouseUrl;
    private final String clickhouseTable;
    private final String clickhouseUser;
    private final String clickhousePassword;
    private final int clickhouseBatchSize;
    private final int clickhouseMaxRetries;
    private final long checkpointIntervalMs;
    private final long checkpointTimeoutMs;
    private final long checkpointMinPauseMs;
    private final long kafkaTransactionTimeoutMs;
    private final long watermarkDelayMs;
    private final long watermarkIdlenessMs;
    private final long allowedLatenessMs;
    private final long maxFutureSkewMs;
    private final long maxClockRollbackMs;
    private final int parallelism;
    private final int restartAttempts;
    private final int restartDelaySeconds;
    private final Properties kafkaProperties;

    private LogJobConfig(
            String kafkaBrokers,
            String inputTopic,
            String dlqTopic,
            String auditTopic,
            String consumerGroup,
            String dlqTransactionalIdPrefix,
            String qualityTransactionalIdPrefix,
            DeploymentActivation activation,
            boolean projectionWritesEnabled,
            boolean sourceFactWritesEnabled,
            String clickhouseUrl,
            String clickhouseTable,
            String clickhouseUser,
            String clickhousePassword,
            int clickhouseBatchSize,
            int clickhouseMaxRetries,
            long checkpointIntervalMs,
            long checkpointTimeoutMs,
            long checkpointMinPauseMs,
            long kafkaTransactionTimeoutMs,
            long watermarkDelayMs,
            long watermarkIdlenessMs,
            long allowedLatenessMs,
            long maxFutureSkewMs,
            long maxClockRollbackMs,
            int parallelism,
            int restartAttempts,
            int restartDelaySeconds,
            Properties kafkaProperties) {
        this.kafkaBrokers = requireNonBlank(kafkaBrokers, "kafka.brokers");
        this.inputTopic = Objects.requireNonNull(inputTopic, "inputTopic");
        this.dlqTopic = Objects.requireNonNull(dlqTopic, "dlqTopic");
        this.auditTopic = Objects.requireNonNull(auditTopic, "auditTopic");
        this.consumerGroup = requireNonBlank(consumerGroup, "kafka.group.id");
        this.dlqTransactionalIdPrefix = requireNonBlank(
                dlqTransactionalIdPrefix, "kafka.dlq.transactional.id.prefix");
        this.qualityTransactionalIdPrefix = requireNonBlank(
                qualityTransactionalIdPrefix, "kafka.quality.transactional.id.prefix");
        this.activation = Objects.requireNonNull(activation, "activation");
        this.projectionWritesEnabled = projectionWritesEnabled;
        this.sourceFactWritesEnabled = sourceFactWritesEnabled;
        this.clickhouseUrl = requireNonBlank(clickhouseUrl, "clickhouse.url");
        this.clickhouseTable = requireNonBlank(clickhouseTable, "clickhouse.source.fact.table");
        this.clickhouseUser = clickhouseUser == null ? "" : clickhouseUser;
        this.clickhousePassword = clickhousePassword == null ? "" : clickhousePassword;
        this.clickhouseBatchSize = clickhouseBatchSize;
        this.clickhouseMaxRetries = clickhouseMaxRetries;
        this.checkpointIntervalMs = checkpointIntervalMs;
        this.checkpointTimeoutMs = checkpointTimeoutMs;
        this.checkpointMinPauseMs = checkpointMinPauseMs;
        this.kafkaTransactionTimeoutMs = kafkaTransactionTimeoutMs;
        this.watermarkDelayMs = watermarkDelayMs;
        this.watermarkIdlenessMs = watermarkIdlenessMs;
        this.allowedLatenessMs = allowedLatenessMs;
        this.maxFutureSkewMs = maxFutureSkewMs;
        this.maxClockRollbackMs = maxClockRollbackMs;
        this.parallelism = parallelism;
        this.restartAttempts = restartAttempts;
        this.restartDelaySeconds = restartDelaySeconds;
        this.kafkaProperties = copy(kafkaProperties);
        validate();
    }

    public static LogJobConfig from(ParameterTool params) {
        String brokers = ConfigUtils.get(
                params, "kafka.brokers", "kafka-bootstrap.middleware.svc:9092");
        String inputTopic = ConfigUtils.get(params, "kafka.input.topic", INPUT_TOPIC);
        String dlqTopic = ConfigUtils.get(params, "kafka.dlq.topic", DLQ_TOPIC);
        String auditTopic = ConfigUtils.get(params, "kafka.audit.topic", AUDIT_TOPIC);
        String group = ConfigUtils.get(params, "kafka.group.id", CANONICAL_GROUP);
        DeploymentActivation activation = DeploymentActivation.from(params, CANONICAL_GROUP, group);

        boolean legacyProjectionDefault =
                activation.getMode() == DeploymentActivation.Mode.LEGACY;
        boolean projectionWrites = ConfigUtils.getBoolean(
                params, "projection.writes.enabled", legacyProjectionDefault);

        String defaultTransactionPrefix = "flink-log-job-dlq-v1";
        String defaultQualityTransactionPrefix = "flink-log-job-quality-v1";
        if (activation.isCandidateBound()) {
            defaultTransactionPrefix += "-" + activation.getCandidateSha256().substring(0, 12);
            defaultQualityTransactionPrefix += "-" + activation.getCandidateSha256().substring(0, 12);
        }

        return new LogJobConfig(
                brokers,
                inputTopic,
                dlqTopic,
                auditTopic,
                group,
                ConfigUtils.get(
                        params, "kafka.dlq.transactional.id.prefix", defaultTransactionPrefix),
                ConfigUtils.get(
                        params, "kafka.quality.transactional.id.prefix", defaultQualityTransactionPrefix),
                activation,
                projectionWrites,
                ConfigUtils.getBoolean(params, "source.fact.writes.enabled", false),
                ConfigUtils.get(params, "clickhouse.url",
                        "jdbc:clickhouse://clickhouse-1.middleware.svc:8123/traffic"),
                ConfigUtils.get(params, "clickhouse.source.fact.table",
                        "traffic.source_device_log_facts_v1"),
                ConfigUtils.get(params, "clickhouse.user", "default"),
                ConfigUtils.get(params, "clickhouse.password", ""),
                ConfigUtils.getInt(params, "clickhouse.batch.size", 1000),
                ConfigUtils.getInt(params, "clickhouse.max.retries", 3),
                ConfigUtils.getLong(params, "checkpoint.interval.ms", 30_000L),
                ConfigUtils.getLong(params, "checkpoint.timeout.ms", 300_000L),
                ConfigUtils.getLong(params, "checkpoint.min.pause.ms", 5_000L),
                ConfigUtils.getLong(params, "kafka.transaction.timeout.ms", 600_000L),
                ConfigUtils.getLong(params, "watermark.delay.ms", 10_000L),
                ConfigUtils.getLong(params, "watermark.idleness.ms", 60_000L),
                ConfigUtils.getLong(params, "allowed.lateness.ms", 30_000L),
                ConfigUtils.getLong(params, "event.max.future.skew.ms", 300_000L),
                ConfigUtils.getLong(params, "event.max.clock.rollback.ms", 30_000L),
                ConfigUtils.getInt(params, "parallelism", 4),
                ConfigUtils.getInt(params, "restart.attempts", 10),
                ConfigUtils.getInt(params, "restart.delay.seconds", 30),
                ConfigUtils.kafkaClientProperties(params));
    }

    private void validate() {
        if (!INPUT_TOPIC.equals(inputTopic)) {
            throw new IllegalArgumentException(
                    "LogJob input is pinned to canonical topic " + INPUT_TOPIC);
        }
        if (!DLQ_TOPIC.equals(dlqTopic)) {
            throw new IllegalArgumentException(
                    "LogJob failures must use canonical topic " + DLQ_TOPIC);
        }
        if (!AUDIT_TOPIC.equals(auditTopic)) {
            throw new IllegalArgumentException(
                    "LogJob quality receipts must use canonical topic " + AUDIT_TOPIC);
        }
        if (!TRANSACTIONAL_PREFIX.matcher(dlqTransactionalIdPrefix).matches()) {
            throw new IllegalArgumentException("kafka.dlq.transactional.id.prefix is invalid");
        }
        if (!TRANSACTIONAL_PREFIX.matcher(qualityTransactionalIdPrefix).matches()) {
            throw new IllegalArgumentException("kafka.quality.transactional.id.prefix is invalid");
        }
        if (qualityTransactionalIdPrefix.equals(dlqTransactionalIdPrefix)) {
            throw new IllegalArgumentException("DLQ and quality transactional prefixes must differ");
        }
        requirePositive(checkpointIntervalMs, "checkpoint.interval.ms");
        requirePositive(checkpointTimeoutMs, "checkpoint.timeout.ms");
        requireNonNegative(checkpointMinPauseMs, "checkpoint.min.pause.ms");
        requirePositive(kafkaTransactionTimeoutMs, "kafka.transaction.timeout.ms");
        if (kafkaTransactionTimeoutMs <= checkpointTimeoutMs + checkpointIntervalMs) {
            throw new IllegalArgumentException(
                    "kafka.transaction.timeout.ms must exceed checkpoint timeout plus interval");
        }
        requireNonNegative(watermarkDelayMs, "watermark.delay.ms");
        requirePositive(watermarkIdlenessMs, "watermark.idleness.ms");
        requireNonNegative(allowedLatenessMs, "allowed.lateness.ms");
        requireNonNegative(maxFutureSkewMs, "event.max.future.skew.ms");
        requireNonNegative(maxClockRollbackMs, "event.max.clock.rollback.ms");
        if (parallelism <= 0) throw new IllegalArgumentException("parallelism must be positive");
        if (restartAttempts < 0) throw new IllegalArgumentException("restart.attempts is negative");
        if (restartDelaySeconds <= 0) {
            throw new IllegalArgumentException("restart.delay.seconds must be positive");
        }
        if (projectionWritesEnabled
                && activation.getMode() == DeploymentActivation.Mode.SHADOW) {
            throw new IllegalArgumentException(
                    "shadow activation must not enable Loki/OpenSearch projections");
        }
        if (sourceFactWritesEnabled && !activation.externalWritesEnabled()) {
            throw new IllegalArgumentException(
                    "source-fact ClickHouse writes require an externally writable activation");
        }
        if (sourceFactWritesEnabled
                && !"traffic.source_device_log_facts_v1".equals(clickhouseTable)) {
            throw new IllegalArgumentException(
                    "device-log source facts are pinned to traffic.source_device_log_facts_v1");
        }
        if (clickhouseBatchSize <= 0 || clickhouseMaxRetries < 0) {
            throw new IllegalArgumentException("invalid ClickHouse source-fact batch settings");
        }
    }

    private static String requireNonBlank(String value, String field) {
        String normalized = value == null ? "" : value.trim();
        if (normalized.isEmpty()) throw new IllegalArgumentException(field + " is blank");
        return normalized;
    }

    private static void requirePositive(long value, String field) {
        if (value <= 0L) throw new IllegalArgumentException(field + " must be positive");
    }

    private static void requireNonNegative(long value, String field) {
        if (value < 0L) throw new IllegalArgumentException(field + " is negative");
    }

    private static Properties copy(Properties source) {
        Properties result = new Properties();
        if (source != null) result.putAll(source);
        return result;
    }

    public String getKafkaBrokers() { return kafkaBrokers; }
    public String getInputTopic() { return inputTopic; }
    public String getDlqTopic() { return dlqTopic; }
    public String getAuditTopic() { return auditTopic; }
    public String getConsumerGroup() { return consumerGroup; }
    public String getDlqTransactionalIdPrefix() { return dlqTransactionalIdPrefix; }
    public String getQualityTransactionalIdPrefix() { return qualityTransactionalIdPrefix; }
    public DeploymentActivation getActivation() { return activation; }
    public boolean projectionWritesEnabled() {
        return projectionWritesEnabled && activation.externalWritesEnabled();
    }
    public boolean sourceFactWritesEnabled() {
        return sourceFactWritesEnabled && activation.externalWritesEnabled();
    }
    public String getClickhouseUrl() { return clickhouseUrl; }
    public String getClickhouseTable() { return clickhouseTable; }
    public String getClickhouseUser() { return clickhouseUser; }
    public String getClickhousePassword() { return clickhousePassword; }
    public int getClickhouseBatchSize() { return clickhouseBatchSize; }
    public int getClickhouseMaxRetries() { return clickhouseMaxRetries; }
    public long getCheckpointIntervalMs() { return checkpointIntervalMs; }
    public long getCheckpointTimeoutMs() { return checkpointTimeoutMs; }
    public long getCheckpointMinPauseMs() { return checkpointMinPauseMs; }
    public long getKafkaTransactionTimeoutMs() { return kafkaTransactionTimeoutMs; }
    public long getWatermarkDelayMs() { return watermarkDelayMs; }
    public long getWatermarkIdlenessMs() { return watermarkIdlenessMs; }
    public long getAllowedLatenessMs() { return allowedLatenessMs; }
    public long getMaxFutureSkewMs() { return maxFutureSkewMs; }
    public long getMaxClockRollbackMs() { return maxClockRollbackMs; }
    public EventTimePolicy eventTimePolicy() {
        return new EventTimePolicy(
                watermarkDelayMs,
                watermarkIdlenessMs,
                allowedLatenessMs,
                maxFutureSkewMs,
                maxClockRollbackMs);
    }
    public int getParallelism() { return parallelism; }
    public int getRestartAttempts() { return restartAttempts; }
    public int getRestartDelaySeconds() { return restartDelaySeconds; }
    public Properties getKafkaConsumerProperties() {
        Properties properties = copy(kafkaProperties);
        properties.setProperty("enable.auto.commit", "false");
        properties.setProperty("commit.offsets.on.checkpoint", "true");
        properties.setProperty("isolation.level", "read_committed");
        return properties;
    }
    public Properties getKafkaProducerProperties() { return copy(kafkaProperties); }
}
