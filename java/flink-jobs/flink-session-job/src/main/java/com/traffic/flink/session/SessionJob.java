package com.traffic.flink.session;

import com.traffic.flink.session.aggregator.SessionAggregator;
import com.traffic.flink.session.processor.SessionizeProcessFunction;
import com.traffic.flink.session.sink.CheckpointAwareSessionClickHouseSink;
import com.traffic.flink.session.sink.FlowRawClickHouseSinkFunction;
import com.traffic.flink.session.sink.KafkaSinkFactory;
import com.traffic.flink.session.sink.OpenSearchSinkFactory;
import com.traffic.flink.session.source.FlowEventParseFunction;
import com.traffic.flink.session.source.FlowLatenessFunction;
import com.traffic.flink.session.source.ValidatedFlowInput;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.DeploymentActivation;
import com.traffic.flink.common.ExternalWriteGate;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.common.sourcefact.SourceFactClickHouseSink;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FiveTuple;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;

import com.esotericsoftware.kryo.serializers.JavaSerializer;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.windowing.assigners.EventTimeSessionWindows;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Properties;
import java.util.concurrent.TimeUnit;

/**
 * Flink Session Job（V2 版本）
 * 
 * 核心增强：
 * 1. 支持两种模式：window（窗口聚合）/ process（KeyedProcessFunction）
 * 2. Active Timeout + Idle Timeout（process 模式）
 * 3. ClickHouse AsyncSink + DLQ 降级
 * 4. OpenSearch 双写（可选）
 * 5. 自定义 Prometheus Metrics
 */
public class SessionJob {

    private static final Logger LOG = LoggerFactory.getLogger(SessionJob.class);

    private static final OutputTag<CanonicalDlqMessage> FLOW_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("flow-canonical-dlq"){};
    private static final OutputTag<SourceQualityReceipt> FLOW_PARSE_QUALITY_TAG =
            new OutputTag<SourceQualityReceipt>("flow-parse-quality"){};
    private static final OutputTag<SourceQualityReceipt> FLOW_EVENT_QUALITY_TAG =
            new OutputTag<SourceQualityReceipt>("flow-event-quality"){};
    private static final OutputTag<ValidatedFlowInput> FLOW_ACCEPTED_FACT_TAG =
            new OutputTag<ValidatedFlowInput>("flow-accepted-source-fact"){};
    // 迟到流侧输出（process 模式）：迟到数据不再静默丢弃，统一进 canonical DLQ
    private static final OutputTag<FlowEvent> SESSION_LATE_FLOW_TAG =
            new OutputTag<FlowEvent>("session-late-flow"){};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Session Aggregation Job V2...");

        // 加载配置
        SessionJobConfig config = SessionJobConfig.fromArgs(args);
        DeploymentActivation activation = config.getDeploymentActivation();
        EventTimePolicy eventTimePolicy = config.eventTimePolicy();

        // 创建执行环境
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();

        // 为 protobuf 消息注册 Kryo JavaSerializer（Java 序列化，protobuf 支持
        // writeReplace）。SessionAccumulator.tuple 是非 transient FiveTuple，
        // 窗口模式累加器入 checkpoint 时必须能序列化；不注册会走 Kryo
        // FieldSerializer 并因缺少无参构造而失败。
        env.getConfig().registerTypeWithKryoSerializer(FiveTuple.class, JavaSerializer.class);

        // 配置并行度
        env.setParallelism(config.getParallelism());
        env.setMaxParallelism(config.getMaxParallelism());

        // 配置 Checkpoint
        configureCheckpoint(env, config);

        // 配置状态后端 (RocksDB)
        configureStateBackend(env, config);

        // 配置重启策略
        env.setRestartStrategy(RestartStrategies.failureRateRestart(
            3,
            Time.of(5, TimeUnit.MINUTES),
            Time.of(30, TimeUnit.SECONDS)
        ));

        // 创建 Kafka Source
        KafkaSource<RawKafkaRecord> source = createKafkaSource(config);

        // 配置水印策略
        WatermarkStrategy<ValidatedFlowInput> watermarkStrategy =
                eventTimePolicy.watermarkStrategy(input -> input.getFlow().getTsEnd());

        // 构建数据流：先保留 Kafka 原始 record，解析失败写入统一 DLQ，再对合法 FlowEvent 分配事件时间。
        DataStream<RawKafkaRecord> rawFlowStream = env
            .fromSource(source, WatermarkStrategy.noWatermarks(), "Kafka-RawFlowEvents")
            .uid("kafka-raw-source")
            .name("Kafka Source Raw (flow.events.v1)");

        SingleOutputStreamOperator<ValidatedFlowInput> parsedFlowStream = rawFlowStream
            .process(new FlowEventParseFunction(
                    config.getInputTopic(), config.getConsumerGroupId(), eventTimePolicy,
                    FLOW_DLQ_TAG, FLOW_PARSE_QUALITY_TAG))
            .uid("flow-event-parse")
            .name("Parse FlowEvent with DLQ");

        DataStream<ValidatedFlowInput> timestampedFlowStream = parsedFlowStream
            .assignTimestampsAndWatermarks(watermarkStrategy)
            .uid("flow-event-watermark")
            .name("Assign FlowEvent Watermarks");

        SingleOutputStreamOperator<FlowEvent> validFlowStream = timestampedFlowStream
            .keyBy(ValidatedFlowInput::identityKey)
            .process(new FlowLatenessFunction(
                    eventTimePolicy, config.getConsumerGroupId(),
                    FLOW_DLQ_TAG, FLOW_EVENT_QUALITY_TAG, FLOW_ACCEPTED_FACT_TAG))
            .uid("flow-lateness-classifier-v1")
            .name("Classify Super-Late FlowEvent with Source Tuple");

        parsedFlowStream.getSideOutput(FLOW_DLQ_TAG)
            .union(validFlowStream.getSideOutput(FLOW_DLQ_TAG))
            .sinkTo(KafkaSinkFactory.createDeadLetterSink(
                    config.getKafkaBrokers(), config.getInputDlqTopic(),
                    config.getDlqTransactionalIdPrefix(),
                    config.getKafkaTransactionTimeoutMs()))
            .uid("flow-parse-dlq-sink")
            .name("Checkpoint-coupled canonical Flow DLQ");

        parsedFlowStream.getSideOutput(FLOW_PARSE_QUALITY_TAG)
            .union(validFlowStream.getSideOutput(FLOW_EVENT_QUALITY_TAG))
            .sinkTo(KafkaSinkFactory.createSourceQualitySink(
                    config.getKafkaBrokers(), config.getAuditTopic(),
                    config.getQualityTransactionalIdPrefix(),
                    config.getKafkaTransactionTimeoutMs()))
            .uid("flow-source-quality-sink-v1")
            .name("Checkpoint-coupled Flow source-quality receipts");

        if (config.isSourceFactSinkEnabled()) {
            validFlowStream.getSideOutput(FLOW_ACCEPTED_FACT_TAG)
                .map(input -> toFlowSourceFact(input, config.getConsumerGroupId()))
                .uid("flow-source-fact-mapper-v1")
                .name("Map accepted FlowEvent source facts")
                .addSink(new SourceFactClickHouseSink(
                        config.getClickhouseUrl(),
                        config.getSourceFactClickhouseTable(),
                        config.getClickhouseUser(),
                        config.getClickhousePassword(),
                        config.getClickhouseBatchSize(),
                        config.getClickhouseMaxRetries()))
                .uid("flow-source-fact-clickhouse-v1")
                .name("ClickHouse source_flow_facts_v1");
        }

        if (config.isFlowRawSinkEnabled()) {
            validFlowStream
                .filter(new ExternalWriteGate<>(activation))
                .uid("flow-raw-external-write-gate-v1")
                .name("Deployment Gate (flows_raw)")
                .addSink(new FlowRawClickHouseSinkFunction(
                    config.getClickhouseUrl(),
                    config.getFlowRawClickhouseTable(),
                    config.getClickhouseUser(),
                    config.getClickhousePassword(),
                    config.getClickhouseBatchSize(),
                    config.getClickhouseBatchIntervalMs(),
                    config.getClickhouseMaxRetries(),
                    activation))
                .uid("flow-raw-clickhouse-sink")
                .name("ClickHouse Sink (flows_raw)");
        }

        // 根据模式选择处理逻辑
        SingleOutputStreamOperator<SessionEvent> sessionStream;

        if (config.isProcessMode()) {
            LOG.info("Using PROCESS mode (KeyedProcessFunction with Active/Idle Timeout)");
            sessionStream = buildProcessModeStream(validFlowStream, config);

            // 迟到流（超过 allowedLateness 的 FlowEvent）进入 canonical DLQ，
            // 不再静默丢弃。Kafka 源坐标在解析后已不可用，使用合成的
            // RawKafkaRecord（topic 为输入 topic，坐标置 -1）保留语义载荷。
            sessionStream
                .getSideOutput(SESSION_LATE_FLOW_TAG)
                .map(new LateFlowToDlqMapper(config.getInputTopic()))
                .uid("session-late-flow-dlq-mapper-v1")
                .name("Map late FlowEvent to canonical DLQ")
                .sinkTo(KafkaSinkFactory.createDeadLetterSink(
                        config.getKafkaBrokers(), config.getLateDataTopic(),
                        config.getDlqTransactionalIdPrefix(),
                        config.getKafkaTransactionTimeoutMs()))
                .uid("session-late-flow-dlq-sink-v1")
                .name("Checkpoint-coupled canonical late-flow DLQ");
        } else {
            LOG.info("Using WINDOW mode (EventTimeSessionWindows)");
            sessionStream = buildWindowModeStream(validFlowStream, config);
        }

        // ==================== Sink 配置 ====================

        DataStream<SessionEvent> externalSessionStream = sessionStream
            .filter(new ExternalWriteGate<>(activation))
            .uid("session-external-write-gate-v1")
            .name("Deployment Gate (Session external sinks)");

        // Sink 1: ClickHouse. A failed/partial batch fails the checkpoint;
        // storage outages are retryable infrastructure failures, not poison input.
        externalSessionStream
            .addSink(new CheckpointAwareSessionClickHouseSink(
                    config.getClickhouseUrl(),
                    config.getClickhouseTable(),
                    config.getClickhouseUser(),
                    config.getClickhousePassword(),
                    config.getClickhouseBatchSize(),
                    config.getClickhouseMaxRetries(),
                    activation))
            .uid("clickhouse-async-sink")
            .name("ClickHouse Checkpoint-Aware Batch Sink (sessions)");

        // Sink 2: Kafka 输出（EXACTLY_ONCE，事务与 checkpoint 耦合）
        externalSessionStream
            .sinkTo(KafkaSinkFactory.createSink(
                config.getKafkaBrokers(),
                config.getOutputTopic(),
                config.getOutputTransactionalIdPrefix(),
                config.getKafkaTransactionTimeoutMs()))
            .uid("kafka-sink")
            .name("Kafka Sink (session.events.v1)");

        // Sink 3: OpenSearch（可选）
        if (config.isOpenSearchEnabled()) {
            LOG.info("OpenSearch sink enabled: index={}", config.getOpenSearchIndex());
            externalSessionStream
                .addSink(OpenSearchSinkFactory.createSink(config))
                .uid("opensearch-sink")
                .name("OpenSearch Sink (sessions)");
        }

        // 打印配置信息
        printConfiguration(config);

        // 执行作业
        env.execute("Session Aggregation Job V2");
    }

    static SourceFactRecord toFlowSourceFact(
            ValidatedFlowInput input, String consumerGroup) {
        FlowEvent flow = input.getFlow();
        return SourceFactRecord.fromAccepted(
                "flow",
                flow.getHeader().getTenantId(),
                flow.getFlowId(),
                flow.getHeader().getEventId(),
                flow.getTsEnd(),
                flow.getHeader().getIngestTs(),
                flow.getHeader().getSchemaVersion(),
                input.getSource(),
                consumerGroup,
                flow.getHeader().getAggregateVersion());
    }

    /**
     * 构建 Process 模式流（KeyedProcessFunction）
     */
    private static SingleOutputStreamOperator<SessionEvent> buildProcessModeStream(
            DataStream<FlowEvent> flowStream, SessionJobConfig config) {

        return flowStream
            .keyBy(flow -> {
                String tenantId = flow.getHeader() != null ? flow.getHeader().getTenantId() : "unknown";
                String communityId = flow.getCommunityId() != null ? flow.getCommunityId() : "unknown";
                return tenantId + "|" + communityId;
            })
            .process(new SessionizeProcessFunction(config, SESSION_LATE_FLOW_TAG))
            .uid("session-process-function")
            .name("Sessionize (KeyedProcessFunction)");
    }

    /**
     * 构建 Window 模式流（EventTimeSessionWindows）
     */
    private static SingleOutputStreamOperator<SessionEvent> buildWindowModeStream(
            DataStream<FlowEvent> flowStream, SessionJobConfig config) {

        return flowStream
            .keyBy(flow -> {
                String tenantId = flow.getHeader() != null ? flow.getHeader().getTenantId() : "unknown";
                String communityId = flow.getCommunityId() != null ? flow.getCommunityId() : "unknown";
                return tenantId + "|" + communityId;
            })
            .window(EventTimeSessionWindows.withGap(
                org.apache.flink.streaming.api.windowing.time.Time.milliseconds(config.getSessionGapMs())))
            .allowedLateness(org.apache.flink.streaming.api.windowing.time.Time.milliseconds(config.getAllowedLatenessMs()))
            .aggregate(new SessionAggregator())
            .uid("session-window-aggregator")
            .name("Session Aggregator (Window)");
    }

    /**
     * 配置 Checkpoint
     */
    private static void configureCheckpoint(StreamExecutionEnvironment env, SessionJobConfig config) {
        env.enableCheckpointing(config.getCheckpointIntervalMs());

        CheckpointConfig checkpointConfig = env.getCheckpointConfig();

        checkpointConfig.setCheckpointingMode(CheckpointingMode.EXACTLY_ONCE);
        checkpointConfig.setCheckpointTimeout(config.getCheckpointTimeoutMs());
        checkpointConfig.setMinPauseBetweenCheckpoints(config.getCheckpointMinPauseMs());
        checkpointConfig.setMaxConcurrentCheckpoints(1);
        checkpointConfig.setExternalizedCheckpointCleanup(
            CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION);
        checkpointConfig.enableUnalignedCheckpoints();

        LOG.info("Checkpoint configured: interval={}ms, timeout={}ms, path={}",
                config.getCheckpointIntervalMs(),
                config.getCheckpointTimeoutMs(),
                config.getCheckpointPath());
    }

    /**
     * 配置状态后端 (RocksDB + S3)
     */
    private static void configureStateBackend(StreamExecutionEnvironment env, SessionJobConfig config) {
        EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);

        if (config.isStateTtlEnabled()) {
            try {
                java.lang.reflect.Method m = stateBackend.getClass().getMethod("enableTtlCompactionFilter");
                m.invoke(stateBackend);
                LOG.info("State TTL Compaction Filter enabled with TTL={}ms", config.getStateTtlMs());
            } catch (NoSuchMethodException e) {
                LOG.warn("enableTtlCompactionFilter not available in RocksDB backend, skipping");
            } catch (Exception e) {
                throw new RuntimeException("Failed to enable TTL compaction filter", e);
            }
        }

        env.setStateBackend(stateBackend);
        env.getCheckpointConfig().setCheckpointStorage(
            new FileSystemCheckpointStorage(config.getCheckpointPath()));

        LOG.info("State backend configured: RocksDB with checkpoint storage at {}",
                config.getCheckpointPath());
    }

    /**
     * 创建 Kafka Source
     */
    private static KafkaSource<RawKafkaRecord> createKafkaSource(SessionJobConfig config) {
        Properties consumerProps = com.traffic.flink.common.ConfigUtil.kafkaClientProperties();
        consumerProps.setProperty("partition.discovery.interval.ms", "30000");
        consumerProps.setProperty("fetch.min.bytes", String.valueOf(config.getFetchMinBytes()));
        consumerProps.setProperty("fetch.max.wait.ms", String.valueOf(config.getFetchMaxWaitMs()));
        consumerProps.setProperty("max.poll.records", String.valueOf(config.getMaxPollRecords()));
        consumerProps.setProperty("max.partition.fetch.bytes", String.valueOf(config.getMaxPartitionFetchBytes()));
        consumerProps.setProperty("request.timeout.ms", String.valueOf(config.getRequestTimeoutMs()));
        consumerProps.setProperty("enable.auto.commit", "false");
        consumerProps.setProperty("commit.offsets.on.checkpoint", "true");
        consumerProps.setProperty("isolation.level", "read_committed");

        return KafkaSource.<RawKafkaRecord>builder()
            .setBootstrapServers(config.getKafkaBrokers())
            .setTopics(config.getInputTopic())
            .setGroupId(config.getConsumerGroupId())
            .setStartingOffsets(OffsetsInitializer.committedOffsets(
                org.apache.kafka.clients.consumer.OffsetResetStrategy.EARLIEST))
            .setDeserializer(new RawKafkaRecordDeserializationSchema())
            .setProperties(consumerProps)
            .build();
    }

    /**
     * 打印配置信息
     */
    private static void printConfiguration(SessionJobConfig config) {
        LOG.info("========== Job Configuration ==========");
        LOG.info("  Session Mode: {}", config.getSessionMode());
        LOG.info("  Deployment Activation: {}", config.getDeploymentActivation());
        LOG.info("  Input Topic: {}", config.getInputTopic());
        LOG.info("  Output Topic: {}", config.getOutputTopic());
        LOG.info("  Late Data Topic: {}", config.getLateDataTopic());
        LOG.info("  Input Parse DLQ Topic: {}", config.getInputDlqTopic());
        LOG.info("  Source Quality Topic: {}", config.getAuditTopic());
        LOG.info("  CH DLQ Topic: {}", config.getChDlqTopic());
        LOG.info("  Session Gap (Idle Timeout): {}ms", config.getSessionGapMs());
        LOG.info("  Active Timeout: {}ms", config.getActiveTimeoutMs());
        LOG.info("  Watermark Delay: {}ms", config.getWatermarkDelayMs());
        LOG.info("  State TTL: {}ms (enabled: {})", config.getStateTtlMs(), config.isStateTtlEnabled());
        LOG.info("  Checkpoint Interval: {}ms", config.getCheckpointIntervalMs());
        LOG.info("  Parallelism: {}", config.getParallelism());
        LOG.info("  ClickHouse URL: {}", config.getClickhouseUrl());
        LOG.info("  ClickHouse Batch Size: {}", config.getClickhouseBatchSize());
        LOG.info("  Flow Raw Sink Enabled: {}", config.isFlowRawSinkEnabled());
        LOG.info("  Flow Raw ClickHouse Table: {}", config.getFlowRawClickhouseTable());
        LOG.info("  OpenSearch Enabled: {}", config.isOpenSearchEnabled());
        if (config.isOpenSearchEnabled()) {
            LOG.info("  OpenSearch Hosts: {}", String.join(",", config.getOpenSearchHosts()));
            LOG.info("  OpenSearch Index: {}", config.getOpenSearchIndex());
        }
        LOG.info("========================================");
    }

    /**
     * 将迟到 FlowEvent 映射为 canonical DLQ 消息。
     *
     * 迟到发生在 SessionizeProcessFunction（Kafka 源坐标已被消费），
     * 因此以合成 RawKafkaRecord 承载语义载荷：topic 取输入 topic，
     * partition/offset 置 -1 以表达坐标不可用；original_value 为
     * FlowEvent protobuf，业务内容不丢失。
     */
    static final class LateFlowToDlqMapper
            implements org.apache.flink.api.common.functions.MapFunction<FlowEvent, CanonicalDlqMessage> {
        private static final long serialVersionUID = 1L;
        private final String inputTopic;

        LateFlowToDlqMapper(String inputTopic) {
            this.inputTopic = inputTopic;
        }

        @Override
        public CanonicalDlqMessage map(FlowEvent flow) {
            EventHeader header = flow.hasHeader()
                    ? flow.getHeader() : EventHeader.getDefaultInstance();
            RawKafkaRecord synthetic = new RawKafkaRecord(
                    inputTopic, -1, -1L, flow.getTsEnd(),
                    (flow.getCommunityId() == null ? "" : flow.getCommunityId())
                            .getBytes(java.nio.charset.StandardCharsets.UTF_8),
                    flow.toByteArray(),
                    new java.util.HashMap<>());
            return CanonicalDlqMessage.failure(
                    synthetic, "LATE_DATA", "late_data",
                    "FlowEvent arrived after allowed lateness in sessionize",
                    header.getTenantId(), header.getEventId(), header.getTraceId(),
                    header.getRunId(), header.getProbeId(),
                    "flink-session-job", "traffic.v1.FlowEvent", "v1");
        }
    }
}
