package com.traffic.flink.session;

import com.traffic.flink.common.ConfigUtil;
import com.traffic.flink.common.DeploymentActivation;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import org.apache.flink.api.java.utils.ParameterTool;

import java.io.Serializable;
import java.time.Duration;
import java.util.regex.Pattern;

/**
 * Session Job 配置类（扩展版）
 * 
 * 新增配置项：
 * 1. session.mode：window | process（支持两种模式切换）
 * 2. ClickHouse 异步写入相关配置
 * 3. OpenSearch 双写相关配置
 * 4. DLQ Topic 配置
 */
public class SessionJobConfig implements Serializable {

    private static final long serialVersionUID = 1L;
    public static final String INPUT_TOPIC = "flow.events.v1";
    public static final String AUDIT_TOPIC = "audit.logs";
    public static final String CANONICAL_GROUP = "flink-session-job";
    private static final Pattern TRANSACTIONAL_PREFIX =
            Pattern.compile("[A-Za-z0-9._-]{1,200}");

    // ==================== 模式配置 ====================
    private final String sessionMode; // window | process

    // ==================== Kafka 配置 ====================
    private final String kafkaBrokers;
    private final String inputTopic;
    private final String outputTopic;
    private final String lateDataTopic;
    private final String inputDlqTopic;
    private final String chDlqTopic;
    private final String osDlqTopic;
    private final String consumerGroupId;
    private final String auditTopic;
    private final String dlqTransactionalIdPrefix;
    private final String outputTransactionalIdPrefix;
    private final String qualityTransactionalIdPrefix;
    private final long kafkaTransactionTimeoutMs;
    private final DeploymentActivation deploymentActivation;

    // Kafka 消费者高级参数
    private final int fetchMinBytes;
    private final int fetchMaxWaitMs;
    private final int maxPollRecords;
    private final int maxPartitionFetchBytes;
    private final int requestTimeoutMs;

    // ==================== ClickHouse 配置 ====================
    private final String clickhouseUrl;
    private final String clickhouseDatabase;
    private final String clickhouseTable;
    private final boolean flowRawSinkEnabled;
    private final String flowRawClickhouseTable;
    private final boolean sourceFactSinkEnabled;
    private final String sourceFactClickhouseTable;
    private final String clickhouseUser;
    private final String clickhousePassword;
    private final int clickhouseBatchSize;
    private final long clickhouseBatchIntervalMs;
    private final int clickhouseMaxRetries;
    private final int clickhouseThreadPoolSize;
    private final long clickhouseTimeoutMs;
    private final int clickhouseAsyncCapacity;

    // ==================== OpenSearch 配置 ====================
    private final boolean openSearchEnabled;
    private final String[] openSearchHosts;
    private final int openSearchPort;
    private final String openSearchScheme;
    private final String openSearchIndex;
    private final String openSearchUser;
    private final String openSearchPassword;
    private final int openSearchBatchSize;
    private final long openSearchBatchIntervalMs;

    // ==================== Checkpoint 配置 ====================
    private final String checkpointPath;
    private final long checkpointIntervalMs;
    private final long checkpointTimeoutMs;
    private final long checkpointMinPauseMs;

    // ==================== Session 窗口与 Timeout 配置 ====================
    private final long sessionGapMs;
    private final long activeTimeoutMs;
    private final long watermarkDelayMs;
    private final long watermarkIdlenessMs;
    private final long allowedLatenessMs;
    private final long maxFutureSkewMs;

    // ==================== State 配置 ====================
    private final long stateTtlMs;
    private final boolean stateTtlEnabled;

    // ==================== 性能配置 ====================
    private final int parallelism;
    private final int maxParallelism;

    private SessionJobConfig(Builder builder) {
        this.sessionMode = builder.sessionMode;
        this.kafkaBrokers = builder.kafkaBrokers;
        this.inputTopic = builder.inputTopic;
        this.outputTopic = builder.outputTopic;
        this.lateDataTopic = builder.lateDataTopic;
        this.inputDlqTopic = builder.inputDlqTopic;
        this.chDlqTopic = builder.chDlqTopic;
        this.osDlqTopic = builder.osDlqTopic;
        this.consumerGroupId = builder.consumerGroupId;
        this.auditTopic = builder.auditTopic;
        this.dlqTransactionalIdPrefix = builder.dlqTransactionalIdPrefix;
        this.outputTransactionalIdPrefix = builder.outputTransactionalIdPrefix;
        this.qualityTransactionalIdPrefix = builder.qualityTransactionalIdPrefix;
        this.kafkaTransactionTimeoutMs = builder.kafkaTransactionTimeoutMs;
        this.deploymentActivation = builder.deploymentActivation;
        this.fetchMinBytes = builder.fetchMinBytes;
        this.fetchMaxWaitMs = builder.fetchMaxWaitMs;
        this.maxPollRecords = builder.maxPollRecords;
        this.maxPartitionFetchBytes = builder.maxPartitionFetchBytes;
        this.requestTimeoutMs = builder.requestTimeoutMs;
        this.clickhouseUrl = builder.clickhouseUrl;
        this.clickhouseDatabase = builder.clickhouseDatabase;
        this.clickhouseTable = builder.clickhouseTable;
        this.flowRawSinkEnabled = builder.flowRawSinkEnabled;
        this.flowRawClickhouseTable = builder.flowRawClickhouseTable;
        this.sourceFactSinkEnabled = builder.sourceFactSinkEnabled;
        this.sourceFactClickhouseTable = builder.sourceFactClickhouseTable;
        this.clickhouseUser = builder.clickhouseUser;
        this.clickhousePassword = builder.clickhousePassword;
        this.clickhouseBatchSize = builder.clickhouseBatchSize;
        this.clickhouseBatchIntervalMs = builder.clickhouseBatchIntervalMs;
        this.clickhouseMaxRetries = builder.clickhouseMaxRetries;
        this.clickhouseThreadPoolSize = builder.clickhouseThreadPoolSize;
        this.clickhouseTimeoutMs = builder.clickhouseTimeoutMs;
        this.clickhouseAsyncCapacity = builder.clickhouseAsyncCapacity;
        this.openSearchEnabled = builder.openSearchEnabled;
        this.openSearchHosts = builder.openSearchHosts;
        this.openSearchPort = builder.openSearchPort;
        this.openSearchScheme = builder.openSearchScheme;
        this.openSearchIndex = builder.openSearchIndex;
        this.openSearchUser = builder.openSearchUser;
        this.openSearchPassword = builder.openSearchPassword;
        this.openSearchBatchSize = builder.openSearchBatchSize;
        this.openSearchBatchIntervalMs = builder.openSearchBatchIntervalMs;
        this.checkpointPath = builder.checkpointPath;
        this.checkpointIntervalMs = builder.checkpointIntervalMs;
        this.checkpointTimeoutMs = builder.checkpointTimeoutMs;
        this.checkpointMinPauseMs = builder.checkpointMinPauseMs;
        this.sessionGapMs = builder.sessionGapMs;
        this.activeTimeoutMs = builder.activeTimeoutMs;
        this.watermarkDelayMs = builder.watermarkDelayMs;
        this.watermarkIdlenessMs = builder.watermarkIdlenessMs;
        this.allowedLatenessMs = builder.allowedLatenessMs;
        this.maxFutureSkewMs = builder.maxFutureSkewMs;
        this.stateTtlMs = builder.stateTtlMs;
        this.stateTtlEnabled = builder.stateTtlEnabled;
        this.parallelism = builder.parallelism;
        this.maxParallelism = builder.maxParallelism;
        if (!"dlq.v1".equals(inputDlqTopic)
                || !"dlq.v1".equals(lateDataTopic)
                || !"dlq.v1".equals(chDlqTopic)
                || !"dlq.v1".equals(osDlqTopic)) {
            throw new IllegalArgumentException(
                    "Session job parse late and projection failures must use canonical dlq.v1");
        }
        validate();
    }

    private void validate() {
        if (!INPUT_TOPIC.equals(inputTopic)) {
            throw new IllegalArgumentException("Session job input is pinned to flow.events.v1");
        }
        if (!AUDIT_TOPIC.equals(auditTopic)) {
            throw new IllegalArgumentException("Session job quality receipts are pinned to audit.logs");
        }
        if (!TRANSACTIONAL_PREFIX.matcher(dlqTransactionalIdPrefix).matches()
                || !TRANSACTIONAL_PREFIX.matcher(outputTransactionalIdPrefix).matches()
                || !TRANSACTIONAL_PREFIX.matcher(qualityTransactionalIdPrefix).matches()) {
            throw new IllegalArgumentException("Session job Kafka transactional prefix is invalid");
        }
        if (dlqTransactionalIdPrefix.equals(qualityTransactionalIdPrefix)
                || dlqTransactionalIdPrefix.equals(outputTransactionalIdPrefix)
                || outputTransactionalIdPrefix.equals(qualityTransactionalIdPrefix)) {
            throw new IllegalArgumentException(
                    "DLQ, output and quality transactional prefixes must differ");
        }
        if (checkpointIntervalMs <= 0L || checkpointTimeoutMs <= 0L
                || kafkaTransactionTimeoutMs <= checkpointTimeoutMs + checkpointIntervalMs) {
            throw new IllegalArgumentException(
                    "Kafka transaction timeout must exceed checkpoint timeout plus interval");
        }
        if (!isProcessMode() && !isWindowMode()) {
            throw new IllegalArgumentException("session.mode must be process or window");
        }
        if (sessionGapMs <= 0 || activeTimeoutMs <= 0 || watermarkDelayMs < 0
                || watermarkIdlenessMs <= 0 || allowedLatenessMs < 0 || maxFutureSkewMs < 0) {
            throw new IllegalArgumentException("session timeout, watermark and lateness configuration is invalid");
        }
        if (isProcessMode() && !stateTtlEnabled) {
            throw new IllegalArgumentException("process mode requires state.ttl.enabled=true");
        }
        long timerHorizon = Math.max(sessionGapMs, activeTimeoutMs);
        timerHorizon = saturatedAdd(timerHorizon, watermarkDelayMs);
        timerHorizon = saturatedAdd(timerHorizon, allowedLatenessMs);
        if (isProcessMode() && stateTtlMs <= timerHorizon) {
            throw new IllegalArgumentException(
                    "state.ttl.ms must exceed active/idle timeout plus watermark delay and allowed lateness");
        }
        if (parallelism <= 0 || maxParallelism <= 0 || parallelism > maxParallelism) {
            throw new IllegalArgumentException("parallelism must be positive and not exceed max.parallelism");
        }
        if (sourceFactSinkEnabled && !deploymentActivation.externalWritesEnabled()) {
            throw new IllegalArgumentException(
                    "source-fact ClickHouse writes require an externally writable activation");
        }
        if (sourceFactSinkEnabled
                && !"traffic.source_flow_facts_v1".equals(sourceFactClickhouseTable)) {
            throw new IllegalArgumentException(
                    "flow source facts are pinned to traffic.source_flow_facts_v1");
        }
    }

    private static long saturatedAdd(long left, long right) {
        if (right > 0 && left > Long.MAX_VALUE - right) return Long.MAX_VALUE;
        return left + right;
    }

    /**
     * 从命令行参数 + session-job.properties + 环境变量构建配置。
     * 此前仅用 ParameterTool.fromArgs(args)，session-job.properties 中的
     * 调优值（checkpoint.interval.ms 等）从未加载；ConfigUtil.loadConfig
     * 按 文件 < 环境变量 < 系统属性 < 命令行 合并。
     */
    public static SessionJobConfig fromArgs(String[] args) throws Exception {
        ParameterTool params = ConfigUtil.loadConfig(args, "session-job.properties");

        String osHostsStr = params.get("opensearch.hosts", "opensearch.middleware.svc");
        String[] osHosts = osHostsStr.split(",");
        String consumerGroup = params.get("consumer.group", "flink-session-job");
        DeploymentActivation activation = DeploymentActivation.from(
                params, "flink-session-job", consumerGroup);
        String candidateSuffix = activation.isCandidateBound()
                ? "-" + activation.getCandidateSha256().substring(0, 12) : "";

        return new Builder()
            .sessionMode(params.get("session.mode", "process"))
            .kafkaBrokers(params.get("kafka.brokers", ConfigUtil.KAFKA_BROKERS))
            .inputTopic(params.get("input.topic", ConfigUtil.TOPIC_FLOW_EVENTS))
            .outputTopic(params.get("output.topic", ConfigUtil.TOPIC_SESSION_EVENTS))
            .lateDataTopic(params.get("late.data.topic", "dlq.v1"))
            .inputDlqTopic(params.get("input.dlq.topic", "dlq.v1"))
            .chDlqTopic(params.get("ch.dlq.topic", "dlq.v1"))
            .osDlqTopic(params.get("os.dlq.topic", "dlq.v1"))
            .consumerGroupId(consumerGroup)
            .auditTopic(params.get("audit.topic", AUDIT_TOPIC))
            .dlqTransactionalIdPrefix(params.get(
                    "kafka.dlq.transactional.id.prefix",
                    "flink-session-job-dlq-v1" + candidateSuffix))
            .outputTransactionalIdPrefix(params.get(
                    "kafka.output.transactional.id.prefix",
                    "flink-session-job-output-v1" + candidateSuffix))
            .qualityTransactionalIdPrefix(params.get(
                    "kafka.quality.transactional.id.prefix",
                    "flink-session-job-quality-v1" + candidateSuffix))
            .kafkaTransactionTimeoutMs(params.getLong("kafka.transaction.timeout.ms", 900000L))
            .deploymentActivation(activation)
            .fetchMinBytes(params.getInt("kafka.fetch.min.bytes", 1))
            .fetchMaxWaitMs(params.getInt("kafka.fetch.max.wait.ms", 500))
            .maxPollRecords(params.getInt("kafka.max.poll.records", 500))
            .maxPartitionFetchBytes(params.getInt("kafka.max.partition.fetch.bytes", 1048576))
            .requestTimeoutMs(params.getInt("kafka.request.timeout.ms", 30000))
            .clickhouseUrl(params.get("clickhouse.url", ConfigUtil.buildClickHouseUrl()))
            .clickhouseDatabase(params.get("clickhouse.database", ConfigUtil.CLICKHOUSE_DATABASE))
            .clickhouseTable(params.get("clickhouse.table", "sessions"))
            .flowRawSinkEnabled(params.getBoolean("flow.raw.sink.enabled", true))
            .flowRawClickhouseTable(params.get("flow.raw.clickhouse.table", "flows_raw"))
            .sourceFactSinkEnabled(params.getBoolean("source.fact.sink.enabled", false))
            .sourceFactClickhouseTable(params.get(
                    "source.fact.clickhouse.table", "traffic.source_flow_facts_v1"))
            .clickhouseUser(params.get("clickhouse.user", ConfigUtil.CLICKHOUSE_USER))
            .clickhousePassword(params.get("clickhouse.password", ConfigUtil.CLICKHOUSE_PASSWORD))
            .clickhouseBatchSize(params.getInt("clickhouse.batch.size", ConfigUtil.CLICKHOUSE_BATCH_SIZE))
            .clickhouseBatchIntervalMs(params.getLong("clickhouse.batch.interval.ms", ConfigUtil.CLICKHOUSE_BATCH_INTERVAL_MS))
            .clickhouseMaxRetries(params.getInt("clickhouse.max.retries", 3))
            .clickhouseThreadPoolSize(params.getInt("clickhouse.thread.pool.size", 4))
            .clickhouseTimeoutMs(params.getLong("clickhouse.timeout.ms", 30000L))
            .clickhouseAsyncCapacity(params.getInt("clickhouse.async.capacity", 100))
            .openSearchEnabled(params.getBoolean("opensearch.enabled", false))
            .openSearchHosts(osHosts)
            .openSearchPort(params.getInt("opensearch.port", 9200))
            .openSearchScheme(params.get("opensearch.scheme", "http"))
            .openSearchIndex(params.get("opensearch.index", "sessions_v1"))
            .openSearchUser(params.get("opensearch.user", ""))
            .openSearchPassword(params.get("opensearch.password", ""))
            .openSearchBatchSize(params.getInt("opensearch.batch.size", 1000))
            .openSearchBatchIntervalMs(params.getLong("opensearch.batch.interval.ms", 5000))
            .checkpointPath(params.get("checkpoint.path", ConfigUtil.CHECKPOINT_PATH + "/session-job"))
            .checkpointIntervalMs(params.getLong("checkpoint.interval.ms", ConfigUtil.CHECKPOINT_INTERVAL_MS))
            .checkpointTimeoutMs(params.getLong("checkpoint.timeout.ms", ConfigUtil.CHECKPOINT_TIMEOUT_MS))
            .checkpointMinPauseMs(params.getLong("checkpoint.min.pause.ms", ConfigUtil.CHECKPOINT_MIN_PAUSE_MS))
            .sessionGapMs(params.getLong("session.gap.ms", ConfigUtil.SESSION_GAP_MS))
            .activeTimeoutMs(params.getLong("active.timeout.ms", 1800000L))
            .watermarkDelayMs(params.getLong("watermark.delay.ms", ConfigUtil.WATERMARK_DELAY_MS))
            .watermarkIdlenessMs(params.getLong("watermark.idleness.ms", 60000L))
            .allowedLatenessMs(params.getLong("allowed.lateness.ms", 0L))
            .maxFutureSkewMs(params.getLong("event.max.future.skew.ms", 300000L))
            .stateTtlMs(params.getLong("state.ttl.ms", 3600000L))
            .stateTtlEnabled(params.getBoolean("state.ttl.enabled", true))
            .parallelism(params.getInt("parallelism", 4))
            .maxParallelism(params.getInt("max.parallelism", 128))
            .build();
    }

    // ==================== Getters ====================

    public String getSessionMode() { return sessionMode; }
    public boolean isProcessMode() { return "process".equalsIgnoreCase(sessionMode); }
    public boolean isWindowMode() { return "window".equalsIgnoreCase(sessionMode); }

    public String getKafkaBrokers() { return kafkaBrokers; }
    public String getInputTopic() { return inputTopic; }
    public String getOutputTopic() { return outputTopic; }
    public String getLateDataTopic() { return lateDataTopic; }
    public String getInputDlqTopic() { return inputDlqTopic; }
    public String getChDlqTopic() { return chDlqTopic; }
    public String getOsDlqTopic() { return osDlqTopic; }
    public String getConsumerGroupId() { return consumerGroupId; }
    public String getAuditTopic() { return auditTopic; }
    public String getDlqTransactionalIdPrefix() { return dlqTransactionalIdPrefix; }
    public String getOutputTransactionalIdPrefix() { return outputTransactionalIdPrefix; }
    public String getQualityTransactionalIdPrefix() { return qualityTransactionalIdPrefix; }
    public long getKafkaTransactionTimeoutMs() { return kafkaTransactionTimeoutMs; }
    public DeploymentActivation getDeploymentActivation() { return deploymentActivation; }
    public int getFetchMinBytes() { return fetchMinBytes; }
    public int getFetchMaxWaitMs() { return fetchMaxWaitMs; }
    public int getMaxPollRecords() { return maxPollRecords; }
    public int getMaxPartitionFetchBytes() { return maxPartitionFetchBytes; }
    public int getRequestTimeoutMs() { return requestTimeoutMs; }

    public String getClickhouseUrl() { return clickhouseUrl; }
    public String getClickhouseDatabase() { return clickhouseDatabase; }
    public String getClickhouseTable() { return clickhouseTable; }
    public boolean isFlowRawSinkEnabled() { return flowRawSinkEnabled; }
    public String getFlowRawClickhouseTable() { return flowRawClickhouseTable; }
    public boolean isSourceFactSinkEnabled() { return sourceFactSinkEnabled; }
    public String getSourceFactClickhouseTable() { return sourceFactClickhouseTable; }
    public String getClickhouseUser() { return clickhouseUser; }
    public String getClickhousePassword() { return clickhousePassword; }
    public int getClickhouseBatchSize() { return clickhouseBatchSize; }
    public long getClickhouseBatchIntervalMs() { return clickhouseBatchIntervalMs; }
    public int getClickhouseMaxRetries() { return clickhouseMaxRetries; }
    public int getClickhouseThreadPoolSize() { return clickhouseThreadPoolSize; }
    public long getClickhouseTimeoutMs() { return clickhouseTimeoutMs; }
    public int getClickhouseAsyncCapacity() { return clickhouseAsyncCapacity; }

    public boolean isOpenSearchEnabled() { return openSearchEnabled; }
    public String[] getOpenSearchHosts() { return openSearchHosts; }
    public int getOpenSearchPort() { return openSearchPort; }
    public String getOpenSearchScheme() { return openSearchScheme; }
    public String getOpenSearchIndex() { return openSearchIndex; }
    public String getOpenSearchUser() { return openSearchUser; }
    public String getOpenSearchPassword() { return openSearchPassword; }
    public int getOpenSearchBatchSize() { return openSearchBatchSize; }
    public long getOpenSearchBatchIntervalMs() { return openSearchBatchIntervalMs; }

    public String getCheckpointPath() { return checkpointPath; }
    public long getCheckpointIntervalMs() { return checkpointIntervalMs; }
    public long getCheckpointTimeoutMs() { return checkpointTimeoutMs; }
    public long getCheckpointMinPauseMs() { return checkpointMinPauseMs; }

    public long getSessionGapMs() { return sessionGapMs; }
    public Duration getSessionGapDuration() { return Duration.ofMillis(sessionGapMs); }
    public long getActiveTimeoutMs() { return activeTimeoutMs; }
    public Duration getActiveTimeoutDuration() { return Duration.ofMillis(activeTimeoutMs); }
    public long getWatermarkDelayMs() { return watermarkDelayMs; }
    public Duration getWatermarkDelayDuration() { return Duration.ofMillis(watermarkDelayMs); }
    public long getWatermarkIdlenessMs() { return watermarkIdlenessMs; }
    public long getAllowedLatenessMs() { return allowedLatenessMs; }
    public long getMaxFutureSkewMs() { return maxFutureSkewMs; }
    public EventTimePolicy eventTimePolicy() {
        return new EventTimePolicy(
                watermarkDelayMs, watermarkIdlenessMs, allowedLatenessMs,
                maxFutureSkewMs, 0L);
    }

    public long getStateTtlMs() { return stateTtlMs; }
    public Duration getStateTtlDuration() { return Duration.ofMillis(stateTtlMs); }
    public boolean isStateTtlEnabled() { return stateTtlEnabled; }

    public int getParallelism() { return parallelism; }
    public int getMaxParallelism() { return maxParallelism; }

    // ==================== Builder ====================

    public static class Builder {
        private String sessionMode = "process";
        private String kafkaBrokers = ConfigUtil.KAFKA_BROKERS;
        private String inputTopic = ConfigUtil.TOPIC_FLOW_EVENTS;
        private String outputTopic = ConfigUtil.TOPIC_SESSION_EVENTS;
        private String lateDataTopic = "dlq.v1";
        private String inputDlqTopic = "dlq.v1";
        private String chDlqTopic = "dlq.v1";
        private String osDlqTopic = "dlq.v1";
        private String consumerGroupId = "flink-session-job";
        private String auditTopic = AUDIT_TOPIC;
        private String dlqTransactionalIdPrefix = "flink-session-job-dlq-v1";
        private String outputTransactionalIdPrefix = "flink-session-job-output-v1";
        private String qualityTransactionalIdPrefix = "flink-session-job-quality-v1";
        private long kafkaTransactionTimeoutMs = 900000L;
        private DeploymentActivation deploymentActivation =
                DeploymentActivation.legacy("flink-session-job");
        private int fetchMinBytes = 1;
        private int fetchMaxWaitMs = 500;
        private int maxPollRecords = 500;
        private int maxPartitionFetchBytes = 1048576;
        private int requestTimeoutMs = 30000;
        private String clickhouseUrl = ConfigUtil.buildClickHouseUrl();
        private String clickhouseDatabase = ConfigUtil.CLICKHOUSE_DATABASE;
        private String clickhouseTable = "sessions";
        private boolean flowRawSinkEnabled = true;
        private String flowRawClickhouseTable = "flows_raw";
        private boolean sourceFactSinkEnabled = false;
        private String sourceFactClickhouseTable = "traffic.source_flow_facts_v1";
        private String clickhouseUser = ConfigUtil.CLICKHOUSE_USER;
        private String clickhousePassword = ConfigUtil.CLICKHOUSE_PASSWORD;
        private int clickhouseBatchSize = ConfigUtil.CLICKHOUSE_BATCH_SIZE;
        private long clickhouseBatchIntervalMs = ConfigUtil.CLICKHOUSE_BATCH_INTERVAL_MS;
        private int clickhouseMaxRetries = 3;
        private int clickhouseThreadPoolSize = 4;
        private long clickhouseTimeoutMs = 30000L;
        private int clickhouseAsyncCapacity = 100;
        private boolean openSearchEnabled = false;
        private String[] openSearchHosts = new String[]{"opensearch.middleware.svc"};
        private int openSearchPort = 9200;
        private String openSearchScheme = "http";
        private String openSearchIndex = "sessions_v1";
        private String openSearchUser = "";
        private String openSearchPassword = "";
        private int openSearchBatchSize = 1000;
        private long openSearchBatchIntervalMs = 5000;
        private String checkpointPath = ConfigUtil.CHECKPOINT_PATH + "/session-job";
        private long checkpointIntervalMs = ConfigUtil.CHECKPOINT_INTERVAL_MS;
        private long checkpointTimeoutMs = ConfigUtil.CHECKPOINT_TIMEOUT_MS;
        private long checkpointMinPauseMs = ConfigUtil.CHECKPOINT_MIN_PAUSE_MS;
        private long sessionGapMs = ConfigUtil.SESSION_GAP_MS;
        private long activeTimeoutMs = 1800000L;
        private long watermarkDelayMs = ConfigUtil.WATERMARK_DELAY_MS;
        private long watermarkIdlenessMs = 60000L;
        private long allowedLatenessMs = 0L;
        private long maxFutureSkewMs = 300000L;
        private long stateTtlMs = 3600000L;
        private boolean stateTtlEnabled = true;
        private int parallelism = 4;
        private int maxParallelism = 128;

        public Builder sessionMode(String sessionMode) { this.sessionMode = sessionMode; return this; }
        public Builder kafkaBrokers(String kafkaBrokers) { this.kafkaBrokers = kafkaBrokers; return this; }
        public Builder inputTopic(String inputTopic) { this.inputTopic = inputTopic; return this; }
        public Builder outputTopic(String outputTopic) { this.outputTopic = outputTopic; return this; }
        public Builder lateDataTopic(String lateDataTopic) { this.lateDataTopic = lateDataTopic; return this; }
        public Builder inputDlqTopic(String inputDlqTopic) { this.inputDlqTopic = inputDlqTopic; return this; }
        public Builder chDlqTopic(String chDlqTopic) { this.chDlqTopic = chDlqTopic; return this; }
        public Builder osDlqTopic(String osDlqTopic) { this.osDlqTopic = osDlqTopic; return this; }
        public Builder consumerGroupId(String consumerGroupId) { this.consumerGroupId = consumerGroupId; return this; }
        public Builder auditTopic(String auditTopic) { this.auditTopic = auditTopic; return this; }
        public Builder dlqTransactionalIdPrefix(String value) { this.dlqTransactionalIdPrefix = value; return this; }
        public Builder outputTransactionalIdPrefix(String value) { this.outputTransactionalIdPrefix = value; return this; }
        public Builder qualityTransactionalIdPrefix(String value) { this.qualityTransactionalIdPrefix = value; return this; }
        public Builder kafkaTransactionTimeoutMs(long value) { this.kafkaTransactionTimeoutMs = value; return this; }
        public Builder deploymentActivation(DeploymentActivation deploymentActivation) { this.deploymentActivation = deploymentActivation; return this; }
        public Builder fetchMinBytes(int fetchMinBytes) { this.fetchMinBytes = fetchMinBytes; return this; }
        public Builder fetchMaxWaitMs(int fetchMaxWaitMs) { this.fetchMaxWaitMs = fetchMaxWaitMs; return this; }
        public Builder maxPollRecords(int maxPollRecords) { this.maxPollRecords = maxPollRecords; return this; }
        public Builder maxPartitionFetchBytes(int maxPartitionFetchBytes) { this.maxPartitionFetchBytes = maxPartitionFetchBytes; return this; }
        public Builder requestTimeoutMs(int requestTimeoutMs) { this.requestTimeoutMs = requestTimeoutMs; return this; }
        public Builder clickhouseUrl(String clickhouseUrl) { this.clickhouseUrl = clickhouseUrl; return this; }
        public Builder clickhouseDatabase(String clickhouseDatabase) { this.clickhouseDatabase = clickhouseDatabase; return this; }
        public Builder clickhouseTable(String clickhouseTable) { this.clickhouseTable = clickhouseTable; return this; }
        public Builder flowRawSinkEnabled(boolean flowRawSinkEnabled) { this.flowRawSinkEnabled = flowRawSinkEnabled; return this; }
        public Builder flowRawClickhouseTable(String flowRawClickhouseTable) { this.flowRawClickhouseTable = flowRawClickhouseTable; return this; }
        public Builder sourceFactSinkEnabled(boolean value) { this.sourceFactSinkEnabled = value; return this; }
        public Builder sourceFactClickhouseTable(String value) { this.sourceFactClickhouseTable = value; return this; }
        public Builder clickhouseUser(String clickhouseUser) { this.clickhouseUser = clickhouseUser; return this; }
        public Builder clickhousePassword(String clickhousePassword) { this.clickhousePassword = clickhousePassword; return this; }
        public Builder clickhouseBatchSize(int clickhouseBatchSize) { this.clickhouseBatchSize = clickhouseBatchSize; return this; }
        public Builder clickhouseBatchIntervalMs(long clickhouseBatchIntervalMs) { this.clickhouseBatchIntervalMs = clickhouseBatchIntervalMs; return this; }
        public Builder clickhouseMaxRetries(int clickhouseMaxRetries) { this.clickhouseMaxRetries = clickhouseMaxRetries; return this; }
        public Builder clickhouseThreadPoolSize(int clickhouseThreadPoolSize) { this.clickhouseThreadPoolSize = clickhouseThreadPoolSize; return this; }
        public Builder clickhouseTimeoutMs(long clickhouseTimeoutMs) { this.clickhouseTimeoutMs = clickhouseTimeoutMs; return this; }
        public Builder clickhouseAsyncCapacity(int clickhouseAsyncCapacity) { this.clickhouseAsyncCapacity = clickhouseAsyncCapacity; return this; }
        public Builder openSearchEnabled(boolean openSearchEnabled) { this.openSearchEnabled = openSearchEnabled; return this; }
        public Builder openSearchHosts(String[] openSearchHosts) { this.openSearchHosts = openSearchHosts; return this; }
        public Builder openSearchPort(int openSearchPort) { this.openSearchPort = openSearchPort; return this; }
        public Builder openSearchScheme(String openSearchScheme) { this.openSearchScheme = openSearchScheme; return this; }
        public Builder openSearchIndex(String openSearchIndex) { this.openSearchIndex = openSearchIndex; return this; }
        public Builder openSearchUser(String openSearchUser) { this.openSearchUser = openSearchUser; return this; }
        public Builder openSearchPassword(String openSearchPassword) { this.openSearchPassword = openSearchPassword; return this; }
        public Builder openSearchBatchSize(int openSearchBatchSize) { this.openSearchBatchSize = openSearchBatchSize; return this; }
        public Builder openSearchBatchIntervalMs(long openSearchBatchIntervalMs) { this.openSearchBatchIntervalMs = openSearchBatchIntervalMs; return this; }
        public Builder checkpointPath(String checkpointPath) { this.checkpointPath = checkpointPath; return this; }
        public Builder checkpointIntervalMs(long checkpointIntervalMs) { this.checkpointIntervalMs = checkpointIntervalMs; return this; }
        public Builder checkpointTimeoutMs(long checkpointTimeoutMs) { this.checkpointTimeoutMs = checkpointTimeoutMs; return this; }
        public Builder checkpointMinPauseMs(long checkpointMinPauseMs) { this.checkpointMinPauseMs = checkpointMinPauseMs; return this; }
        public Builder sessionGapMs(long sessionGapMs) { this.sessionGapMs = sessionGapMs; return this; }
        public Builder activeTimeoutMs(long activeTimeoutMs) { this.activeTimeoutMs = activeTimeoutMs; return this; }
        public Builder watermarkDelayMs(long watermarkDelayMs) { this.watermarkDelayMs = watermarkDelayMs; return this; }
        public Builder watermarkIdlenessMs(long value) { this.watermarkIdlenessMs = value; return this; }
        public Builder allowedLatenessMs(long allowedLatenessMs) { this.allowedLatenessMs = allowedLatenessMs; return this; }
        public Builder maxFutureSkewMs(long value) { this.maxFutureSkewMs = value; return this; }
        public Builder stateTtlMs(long stateTtlMs) { this.stateTtlMs = stateTtlMs; return this; }
        public Builder stateTtlEnabled(boolean stateTtlEnabled) { this.stateTtlEnabled = stateTtlEnabled; return this; }
        public Builder parallelism(int parallelism) { this.parallelism = parallelism; return this; }
        public Builder maxParallelism(int maxParallelism) { this.maxParallelism = maxParallelism; return this; }

        public SessionJobConfig build() {
            return new SessionJobConfig(this);
        }
    }
}
