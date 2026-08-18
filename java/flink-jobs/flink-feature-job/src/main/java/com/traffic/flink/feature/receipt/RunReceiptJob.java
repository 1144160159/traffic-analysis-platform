package com.traffic.flink.feature.receipt;

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
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.kafka.clients.consumer.OffsetResetStrategy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Properties;

/**
 * RunReceiptJob —— run-scoped S2 回执作业:
 * analysis.envelopes.v1(run envelope)→ 按 run 聚合 → SESSIONIZATION StageReceipt → analysis.receipts.v1。
 * 事件时间水位线基于 envelope.event.ts_end;窗口闭合 + 宽限后产出回执。
 */
public final class RunReceiptJob {

    private static final Logger LOG = LoggerFactory.getLogger(RunReceiptJob.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private RunReceiptJob() {}

    public static void main(String[] args) throws Exception {
        String brokers = System.getenv().getOrDefault("KAFKA_BROKERS", "kafka-bootstrap.middleware.svc:9092");
        String envelopeTopic = System.getenv().getOrDefault("RUN_RECEIPT_ENVELOPE_TOPIC", "analysis.envelopes.v1");
        String receiptTopic = System.getenv().getOrDefault("RUN_RECEIPT_TOPIC", "analysis.receipts.v1");
        String group = System.getenv().getOrDefault("RUN_RECEIPT_GROUP", "run-receipt-v1");
        long graceMs = Long.parseLong(System.getenv().getOrDefault("RUN_RECEIPT_GRACE_MS", "30000"));

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

        DataStream<RunEnvelopeRecord> envelopes = env
                .fromSource(source, WatermarkStrategy.noWatermarks(), "RunEnvelopeSource")
                .uid("run-receipt-envelope-source")
                .name("Kafka Source (analysis.envelopes.v1)")
                .map(raw -> MAPPER.readValue(raw, RunEnvelopeRecord.class))
                .name("Parse RunEnvelope")
                .uid("run-receipt-envelope-parse")
                .assignTimestampsAndWatermarks(
                        WatermarkStrategy.<RunEnvelopeRecord>forBoundedOutOfOrderness(Duration.ofSeconds(10))
                                .withTimestampAssigner((e, ts) -> e.event().tsEnd() > 0 ? e.event().tsEnd() : ts))
                .uid("run-receipt-watermark")
                .name("Envelope EventTime Watermark");

        DataStream<String> receipts = envelopes
                .keyBy(e -> e.runId() == null ? "" : e.runId())
                .process(new RunReceiptProcessor(graceMs))
                .uid("run-session-receipt")
                .name("S2 Sessionization Receipt");

        // S2 第二节点:FEATURE_EXTRACTION 回执(同流按 run 聚合,流级特征派生)。
        DataStream<String> featureReceipts = envelopes
                .keyBy(e -> e.runId() == null ? "" : e.runId())
                .process(new RunFeatureReceiptProcessor(graceMs))
                .uid("run-feature-receipt")
                .name("S2 Feature Extraction Receipt");

        DataStream<String> allReceipts = receipts.union(featureReceipts);

        Properties producerProps = ConfigUtil.kafkaClientProperties();
        producerProps.setProperty("acks", "all");
        producerProps.setProperty("compression.type", "lz4");
        producerProps.setProperty("enable.idempotence", "true");

        allReceipts.sinkTo(KafkaSink.<String>builder()
                        .setBootstrapServers(brokers)
                        .setRecordSerializer(KafkaRecordSerializationSchema.<String>builder()
                                .setTopic(receiptTopic)
                                .setValueSerializationSchema(new SimpleStringSchema(StandardCharsets.UTF_8))
                                .build())
                        .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                        .setKafkaProducerConfig(producerProps)
                        .build())
                .uid("run-receipt-sink")
                .name("Kafka Sink (analysis.receipts.v1)");

        LOG.info("RunReceiptJob start: envelopes={} receipts={} group={} graceMs={}",
                envelopeTopic, receiptTopic, group, graceMs);
        env.execute("Run Receipt Job (S2)");
    }
}
