package com.traffic.flink.log;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.ConfigUtils;
import com.traffic.flink.common.KafkaStartingOffsets;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.common.sourcefact.SourceFactClickHouseSink;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.flink.log.enricher.AssetEnricher;
import com.traffic.flink.log.parser.SyslogParser;
import com.traffic.flink.log.sink.LogDlqSinkFactory;
import com.traffic.flink.log.sink.LogSourceQualitySinkFactory;
import com.traffic.flink.log.sink.LokiSinkFactory;
import com.traffic.flink.log.sink.OpenSearchSinkFactory;
import com.traffic.flink.log.source.DeviceLogEventTimeFunction;
import com.traffic.flink.log.source.DeviceLogParseFunction;
import com.traffic.flink.log.source.ValidatedDeviceLog;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.api.java.utils.ParameterTool;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.util.OutputTag;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Consumer-first device-log pipeline.
 *
 * <p>The source tuple survives decoding until validation has either accepted the
 * record or emitted a canonical DLQ fact. The exactly-once DLQ sink participates
 * in the same Flink checkpoint, so a DLQ write failure prevents source-offset
 * progress. Loki/OpenSearch remain separately gated projections; this job does
 * not claim a ClickHouse projection.</p>
 */
public final class LogJob {
    private static final Logger LOG = LoggerFactory.getLogger(LogJob.class);
    private static final OutputTag<CanonicalDlqMessage> PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("device-log-parse-dlq-v1") {};
    private static final OutputTag<CanonicalDlqMessage> EVENT_TIME_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("device-log-event-time-dlq-v1") {};
    private static final OutputTag<SourceQualityReceipt> PARSE_QUALITY_TAG =
            new OutputTag<SourceQualityReceipt>("device-log-parse-quality-v1") {};
    private static final OutputTag<SourceQualityReceipt> EVENT_TIME_QUALITY_TAG =
            new OutputTag<SourceQualityReceipt>("device-log-event-time-quality-v1") {};
    private static final OutputTag<ValidatedDeviceLog> ACCEPTED_SOURCE_FACT_TAG =
            new OutputTag<ValidatedDeviceLog>("device-log-accepted-source-fact-v1") {};

    private LogJob() {}

    public static void main(String[] args) throws Exception {
        ParameterTool params = ConfigUtils.loadConfig(args, "log-job.properties");
        LogJobConfig config = LogJobConfig.from(params);
        EventTimePolicy eventTimePolicy = config.eventTimePolicy();
        LOG.info("Starting Device Log consumer: group={}, activation={}, projections={}",
                config.getConsumerGroup(), config.getActivation(),
                config.projectionWritesEnabled());

        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(config.getParallelism());
        env.enableCheckpointing(config.getCheckpointIntervalMs(), CheckpointingMode.EXACTLY_ONCE);
        env.getCheckpointConfig().setCheckpointStorage(
                ConfigUtils.get(params, "checkpoint.path",
                        "s3://flink-checkpoints/checkpoints/log-job"));
        env.getCheckpointConfig().setCheckpointTimeout(config.getCheckpointTimeoutMs());
        env.getCheckpointConfig().setMinPauseBetweenCheckpoints(
                config.getCheckpointMinPauseMs());
        env.getCheckpointConfig().setMaxConcurrentCheckpoints(1);
        env.getCheckpointConfig().setTolerableCheckpointFailureNumber(0);
        // RocksDB 状态后端（增量）：作业含 keyed state（maxEventTimeState 等），
        // 此前默认堆内存状态后端，不满足"必须配置 RocksDB"的规范要求。
        env.setStateBackend(new org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend(true));
        env.setRestartStrategy(RestartStrategies.fixedDelayRestart(
                config.getRestartAttempts(), Time.seconds(config.getRestartDelaySeconds())));

        KafkaSource<RawKafkaRecord> source = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getInputTopic())
                .setGroupId(config.getConsumerGroup())
                .setStartingOffsets(KafkaStartingOffsets.from(params))
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(config.getKafkaConsumerProperties())
                .build();

        DataStream<RawKafkaRecord> rawStream = env
                .fromSource(source, WatermarkStrategy.noWatermarks(), "Kafka device.logs.v1")
                .uid("device-log-kafka-source-v2")
                .name("Kafka Source (raw device.logs.v1)");

        SingleOutputStreamOperator<ValidatedDeviceLog> validatedStream = rawStream
                .process(new DeviceLogParseFunction(
                        eventTimePolicy,
                        PARSE_DLQ_TAG,
                        config.getConsumerGroup(),
                        PARSE_QUALITY_TAG))
                .uid("device-log-strict-deserializer-v1")
                .name("Strict DeviceLog envelope validation");

        WatermarkStrategy<ValidatedDeviceLog> watermarkStrategy =
                eventTimePolicy.watermarkStrategy(input -> input.getLog().getTimestamp());

        SingleOutputStreamOperator<DeviceLog> acceptedStream = validatedStream
                .assignTimestampsAndWatermarks(watermarkStrategy)
                .uid("device-log-watermark-v1")
                .name("Bounded DeviceLog event-time watermark")
                .keyBy(ValidatedDeviceLog::identityKey)
                .process(new DeviceLogEventTimeFunction(
                        eventTimePolicy,
                        EVENT_TIME_DLQ_TAG,
                        config.getConsumerGroup(),
                        EVENT_TIME_QUALITY_TAG,
                        ACCEPTED_SOURCE_FACT_TAG))
                .uid("device-log-event-time-barrier-v1")
                .name("DeviceLog lateness and clock rollback barrier");

        DataStream<CanonicalDlqMessage> dlqStream = validatedStream
                .getSideOutput(PARSE_DLQ_TAG)
                .union(acceptedStream.getSideOutput(EVENT_TIME_DLQ_TAG));

        dlqStream.sinkTo(LogDlqSinkFactory.create(config))
                .uid("device-log-canonical-dlq-sink-v1")
                .name("Checkpoint-coupled canonical DLQ sink");

        DataStream<SourceQualityReceipt> qualityReceipts = validatedStream
                .getSideOutput(PARSE_QUALITY_TAG)
                .union(acceptedStream.getSideOutput(EVENT_TIME_QUALITY_TAG));
        qualityReceipts.sinkTo(LogSourceQualitySinkFactory.create(config))
                .uid("device-log-source-quality-sink-v1")
                .name("Checkpoint-coupled source quality receipt sink");

        DataStream<ValidatedDeviceLog> acceptedInputs =
                acceptedStream.getSideOutput(ACCEPTED_SOURCE_FACT_TAG);
        if (config.sourceFactWritesEnabled()) {
            acceptedInputs
                    .map(input -> toDeviceLogSourceFact(input, config.getConsumerGroup()))
                    .uid("device-log-source-fact-mapper-v1")
                    .name("Map accepted DeviceLog source facts")
                    .addSink(new SourceFactClickHouseSink(
                            config.getClickhouseUrl(),
                            config.getClickhouseTable(),
                            config.getClickhouseUser(),
                            config.getClickhousePassword(),
                            config.getClickhouseBatchSize(),
                            config.getClickhouseMaxRetries()))
                    .uid("device-log-source-fact-clickhouse-v1")
                    .name("ClickHouse source_device_log_facts_v1");
        }

        DataStream<DeviceLog> enrichedStream = acceptedStream
                .map(new SyslogParser())
                .uid("device-log-syslog-parser-v2")
                .name("Syslog Parser (validated RFC5424/3164)")
                .map(new AssetEnricher())
                .uid("device-log-asset-enricher-v2")
                .name("Asset Enricher");

        if (config.projectionWritesEnabled()) {
            enrichedStream.addSink(LokiSinkFactory.createSink())
                    .uid("device-log-loki-sink-v2")
                    .name("Loki DeviceLog projection");
            acceptedInputs.addSink(OpenSearchSinkFactory.createVersionedSink())
                    .uid("device-log-opensearch-sink-v2")
                    .name("OpenSearch versioned DeviceLog projection");
        } else {
            LOG.info("Consumer-ready mode: Loki/OpenSearch projections are disabled");
        }

        env.execute("Device Log Consumer v2");
    }

    static SourceFactRecord toDeviceLogSourceFact(
            ValidatedDeviceLog input, String consumerGroup) {
        DeviceLog log = input.getLog();
        return SourceFactRecord.fromAccepted(
                "device_log",
                log.getTenantId(),
                log.getDeviceIp(),
                log.getLogId(),
                log.getTimestamp(),
                input.getSource().getTimestamp(),
                "v1",
                input.getSource(),
                consumerGroup,
                input.getSource().getOffset() + 1L);
    }
}
