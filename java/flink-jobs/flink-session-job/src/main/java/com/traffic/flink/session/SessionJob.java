package com.traffic.flink.session;

import com.traffic.flink.session.aggregator.SessionAggregator;
import com.traffic.flink.session.processor.SessionizeProcessFunction;
import com.traffic.flink.session.sink.ClickHouseAsyncSinkFactory;
import com.traffic.flink.session.sink.FlowRawClickHouseSinkFunction;
import com.traffic.flink.session.sink.KafkaSinkFactory;
import com.traffic.flink.session.sink.OpenSearchSinkFactory;
import com.traffic.flink.session.source.FlowEventParseFunction;
import com.traffic.flink.session.source.RawKafkaRecord;
import com.traffic.flink.session.source.RawKafkaRecordDeserializationSchema;
import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.FlowEvent;
import com.traffic.proto.traffic.v1.SessionEvent;

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

import java.time.Duration;
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

    // Late Data Output Tag
    private static final OutputTag<FlowEvent> LATE_DATA_TAG = 
            new OutputTag<FlowEvent>("late-flow-events"){};

    private static final OutputTag<DeadLetter> FLOW_PARSE_DLQ_TAG =
            new OutputTag<DeadLetter>("flow-parse-dlq"){};

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Session Aggregation Job V2...");

        // 加载配置
        SessionJobConfig config = SessionJobConfig.fromArgs(args);

        // 创建执行环境
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();

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
        WatermarkStrategy<FlowEvent> watermarkStrategy = WatermarkStrategy
            .<FlowEvent>forBoundedOutOfOrderness(config.getWatermarkDelayDuration())
            .withTimestampAssigner((event, timestamp) -> event.getTsEnd())
            .withIdleness(Duration.ofMinutes(1));

        // 构建数据流：先保留 Kafka 原始 record，解析失败写入统一 DLQ，再对合法 FlowEvent 分配事件时间。
        DataStream<RawKafkaRecord> rawFlowStream = env
            .fromSource(source, WatermarkStrategy.noWatermarks(), "Kafka-RawFlowEvents")
            .uid("kafka-raw-source")
            .name("Kafka Source Raw (flow.events.v1)");

        SingleOutputStreamOperator<FlowEvent> parsedFlowStream = rawFlowStream
            .process(new FlowEventParseFunction(FLOW_PARSE_DLQ_TAG))
            .uid("flow-event-parse")
            .name("Parse FlowEvent with DLQ");

        parsedFlowStream.getSideOutput(FLOW_PARSE_DLQ_TAG)
            .sinkTo(KafkaSinkFactory.createDeadLetterSink(
                config.getKafkaBrokers(),
                config.getInputDlqTopic()))
            .uid("flow-parse-dlq-sink")
            .name("Kafka Sink (Flow Parse DLQ)");

        DataStream<FlowEvent> validFlowStream = parsedFlowStream
            .assignTimestampsAndWatermarks(watermarkStrategy)
            .uid("flow-event-watermark")
            .name("Assign FlowEvent Watermarks");

        if (config.isFlowRawSinkEnabled()) {
            validFlowStream
                .addSink(new FlowRawClickHouseSinkFunction(
                    config.getClickhouseUrl(),
                    config.getFlowRawClickhouseTable(),
                    config.getClickhouseUser(),
                    config.getClickhousePassword(),
                    config.getClickhouseBatchSize(),
                    config.getClickhouseBatchIntervalMs(),
                    config.getClickhouseMaxRetries()))
                .uid("flow-raw-clickhouse-sink")
                .name("ClickHouse Sink (flows_raw)");
        }

        // 根据模式选择处理逻辑
        SingleOutputStreamOperator<SessionEvent> sessionStream;
        DataStream<FlowEvent> lateDataStream;

        if (config.isProcessMode()) {
            LOG.info("Using PROCESS mode (KeyedProcessFunction with Active/Idle Timeout)");
            sessionStream = buildProcessModeStream(validFlowStream, config);
            lateDataStream = sessionStream.getSideOutput(LATE_DATA_TAG);
        } else {
            LOG.info("Using WINDOW mode (EventTimeSessionWindows)");
            sessionStream = buildWindowModeStream(validFlowStream, config);
            lateDataStream = sessionStream.getSideOutput(LATE_DATA_TAG);
        }

        // Late Data Sink
        lateDataStream
            .sinkTo(KafkaSinkFactory.createLateDataSink(
                config.getKafkaBrokers(),
                config.getLateDataTopic()))
            .uid("late-data-sink")
            .name("Kafka Sink (Late Data)");

        // ==================== Sink 配置 ====================

        // Sink 1: ClickHouse（异步写入 + DLQ）
        DataStream<SessionEvent> chFailedStream = ClickHouseAsyncSinkFactory.addAsyncSink(
            sessionStream, config);

        // ClickHouse 写入失败的数据发送到 DLQ
        chFailedStream
            .sinkTo(KafkaSinkFactory.createSessionDlqSink(
                config.getKafkaBrokers(),
                config.getChDlqTopic()))
            .uid("ch-dlq-sink")
            .name("Kafka Sink (CH DLQ)");

        // Sink 2: Kafka 输出
        sessionStream
            .sinkTo(KafkaSinkFactory.createSink(
                config.getKafkaBrokers(),
                config.getOutputTopic()))
            .uid("kafka-sink")
            .name("Kafka Sink (session.events.v1)");

        // Sink 3: OpenSearch（可选）
        if (config.isOpenSearchEnabled()) {
            LOG.info("OpenSearch sink enabled: index={}", config.getOpenSearchIndex());
            sessionStream
                .addSink(OpenSearchSinkFactory.createSink(config))
                .uid("opensearch-sink")
                .name("OpenSearch Sink (sessions)");
        }

        // 打印配置信息
        printConfiguration(config);

        // 执行作业
        env.execute("Session Aggregation Job V2");
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
            .process(new SessionizeProcessFunction(config, LATE_DATA_TAG))
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
            .sideOutputLateData(LATE_DATA_TAG)
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
        LOG.info("  Input Topic: {}", config.getInputTopic());
        LOG.info("  Output Topic: {}", config.getOutputTopic());
        LOG.info("  Late Data Topic: {}", config.getLateDataTopic());
        LOG.info("  Input Parse DLQ Topic: {}", config.getInputDlqTopic());
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
}
