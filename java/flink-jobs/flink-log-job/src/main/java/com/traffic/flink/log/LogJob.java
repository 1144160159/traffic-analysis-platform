package com.traffic.flink.log;

import com.traffic.flink.common.ConfigUtil;
import com.traffic.flink.common.ProtoDeserializer;
import com.traffic.flink.log.enricher.AssetEnricher;
import com.traffic.flink.log.parser.SyslogParser;
import com.traffic.flink.log.sink.LokiSinkFactory;
import com.traffic.flink.log.sink.OpenSearchSinkFactory;
import com.traffic.proto.traffic.v1.DeviceLog;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Flink Log Job — 设备日志采集、解析、富化、存储
 *
 * 业务管线：
 *   设备 Syslog (UDP 514) → Fluent Bit → Kafka device.logs.v1
 *   → Flink Log Job (解析RFC5424/3164 → 关联Asset Service → 添加tenant_id)
 *   → Loki (存储) + OpenSearch (检索)
 */
public class LogJob {
    private static final Logger LOG = LoggerFactory.getLogger(LogJob.class);

    public static void main(String[] args) throws Exception {
        LOG.info("Starting Log Job (Syslog Parser & Enricher)...");

        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.enableCheckpointing(60_000);
        env.getCheckpointConfig().setCheckpointStorage(ConfigUtil.CHECKPOINT_PATH);
        env.getCheckpointConfig().setCheckpointTimeout(ConfigUtil.CHECKPOINT_TIMEOUT_MS);
        env.getCheckpointConfig().setMinPauseBetweenCheckpoints(ConfigUtil.CHECKPOINT_MIN_PAUSE_MS);

        // Kafka Source: device.logs.v1
        KafkaSource<DeviceLog> source = KafkaSource.<DeviceLog>builder()
                .setBootstrapServers(ConfigUtil.KAFKA_BROKERS)
                .setTopics("device.logs.v1")
                .setGroupId("flink-log-job")
                .setStartingOffsets(OffsetsInitializer.earliest())
                .setValueOnlyDeserializer(new ProtoDeserializer<>(DeviceLog.class))
                .setProperties(ConfigUtil.kafkaClientProperties())
                .build();

        DataStream<DeviceLog> logStream = env.fromSource(source,
                WatermarkStrategy.<DeviceLog>forMonotonousTimestamps()
                        .withTimestampAssigner((log, ts) -> log.getTimestamp()),
                "Kafka-DeviceLogs")
                .uid("kafka-source").name("Kafka Source (device.logs.v1)");

        // Parse Syslog → extract structured fields
        DataStream<DeviceLog> parsedStream = logStream
                .map(new SyslogParser())
                .uid("syslog-parser").name("Syslog Parser (RFC5424/3164)");

        // Enrich with Asset data (device type, tenant, location)
        DataStream<DeviceLog> enrichedStream = parsedStream
                .map(new AssetEnricher())
                .uid("asset-enricher").name("Asset Enricher");

        // Sink 1: Loki (log storage)
        enrichedStream.addSink(LokiSinkFactory.createSink())
                .uid("loki-sink").name("Loki Sink");

        // Sink 2: OpenSearch (full-text search)
        enrichedStream.addSink(OpenSearchSinkFactory.createSink())
                .uid("opensearch-sink").name("OpenSearch Sink");

        LOG.info("Log Job pipeline: Kafka → Parse → Enrich → Loki + OpenSearch");
        env.execute("Device Log Job");
    }
}
