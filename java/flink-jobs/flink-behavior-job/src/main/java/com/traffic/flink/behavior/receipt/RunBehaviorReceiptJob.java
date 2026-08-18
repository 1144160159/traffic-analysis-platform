package com.traffic.flink.behavior.receipt;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.ConfigUtil;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.kafka.clients.consumer.OffsetResetStrategy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Properties;

/**
 * RunBehaviorReceiptJob —— run-scoped S3/S4 回执作业:
 * analysis.envelopes.v1(run envelope)→ 按 run 识别/检测/聚合 →
 * ENCRYPTED_RECOGNIZER/RULE_DETECTION/BEHAVIOR_DETECTION/DETECTION_AGGREGATE
 * 四条 StageReceipt → analysis.receipts.v1。
 */
public final class RunBehaviorReceiptJob {

    private static final Logger LOG = LoggerFactory.getLogger(RunBehaviorReceiptJob.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private RunBehaviorReceiptJob() {}

    public static void main(String[] args) throws Exception {
        String brokers = System.getenv().getOrDefault("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092");
        String envelopeTopic = System.getenv().getOrDefault("RUN_BEHAVIOR_ENVELOPE_TOPIC", "analysis.envelopes.v1");
        String receiptTopic = System.getenv().getOrDefault("RUN_BEHAVIOR_RECEIPT_TOPIC", "analysis.receipts.v1");
        String group = System.getenv().getOrDefault("RUN_BEHAVIOR_GROUP", "run-behavior-receipt-v1");
        long graceMs = Long.parseLong(System.getenv().getOrDefault("RUN_BEHAVIOR_GRACE_MS", "30000"));

        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);
        env.enableCheckpointing(30_000L);

        Properties consumerProps = ConfigUtil.kafkaClientProperties();
        consumerProps.setProperty("enable.auto.commit", "false");
        consumerProps.setProperty("commit.offsets.on.checkpoint", "true");
        consumerProps.setProperty("isolation.level", "read_committed");

        KafkaSource<String> source = KafkaSource.<String>builder()
                .setBootstrapServers(brokers)
                .setTopics(envelopeTopic)
                .setGroupId(group)
                .setStartingOffsets(OffsetsInitializer.committedOffsets(OffsetResetStrategy.EARLIEST))
                .setValueOnlyDeserializer(new SimpleStringSchema())
                .setProperties(consumerProps)
                .build();

        DataStream<RunBehaviorEnvelopeRecord> envelopes = env
                .fromSource(source, WatermarkStrategy.noWatermarks(), "RunBehaviorEnvelopeSource")
                .uid("run-behavior-envelope-source")
                .name("Kafka Source (analysis.envelopes.v1)")
                .map(raw -> MAPPER.readValue(raw, RunBehaviorEnvelopeRecord.class))
                .name("Parse RunBehaviorEnvelope")
                .uid("run-behavior-envelope-parse")
                .assignTimestampsAndWatermarks(
                        WatermarkStrategy.<RunBehaviorEnvelopeRecord>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                                .withTimestampAssigner((e, ts) -> e.event().tsEnd() > 0 ? e.event().tsEnd() : ts))
                .uid("run-behavior-watermark")
                .name("Envelope EventTime Watermark");

        SingleOutputStreamOperator<String> receipts = envelopes
                .keyBy(e -> e.runId() == null ? "" : e.runId())
                .process(new RunBehaviorReceiptProcessor(graceMs))
                .uid("run-behavior-receipt")
                .name("S3/S4 Recognition & Detection Receipts");

        // 每输入×detector typed disposition 结果行落库 traffic.analysis_detections
        // (§7.4:DetectorDisposition 是 per-input 事实,不进入总体结论;调度域结果权威存储)。
        String chUrl = System.getenv().getOrDefault("CH_URL", "clickhouse-1-0.middleware.svc:8123");
        String chUser = System.getenv().getOrDefault("CH_USER", "default");
        String chPassword = System.getenv().getOrDefault("CH_PASSWORD", "");
        int chBatch = Integer.parseInt(System.getenv().getOrDefault("RUN_CH_BATCH_SIZE", "1000"));
        long chIntervalMs = Long.parseLong(System.getenv().getOrDefault("RUN_CH_BATCH_INTERVAL_MS", "2000"));
        receipts.getSideOutput(RunBehaviorReceiptProcessor.RESULTS_TAG)
                .addSink(RunDetectionResultClickHouseSinkFactory.createSink(chUrl, chUser, chPassword, chBatch, chIntervalMs))
                .uid("run-detection-results-ch")
                .name("ClickHouse Sink (traffic.analysis_detections)");

        Properties producerProps = ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("enable.idempotence", "true");

        receipts.sinkTo(KafkaSink.<String>builder()
                        .setBootstrapServers(brokers)
                        .setRecordSerializer(KafkaRecordSerializationSchema.<String>builder()
                                .setTopic(receiptTopic)
                                .setValueSerializationSchema(new SimpleStringSchema(StandardCharsets.UTF_8))
                                .build())
                        .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                        .setKafkaProducerConfig(producerProps)
                        .build())
                .uid("run-behavior-receipt-sink")
                .name("Kafka Sink (analysis.receipts.v1)");

        LOG.info("RunBehaviorReceiptJob start: envelopes={} receipts={} group={} graceMs={}",
                envelopeTopic, receiptTopic, group, graceMs);
        env.execute("Run Behavior Receipt Job (S3/S4)");
    }
}
