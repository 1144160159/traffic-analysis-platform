package com.traffic.flink.pcap.source;

import com.traffic.flink.pcap.process.PcapIndexParseFunction;
import com.traffic.flink.pcap.sink.DLQSinkFactory;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;

import java.util.List;

/** Builds the N009 consumer-first topology without executing the Flink job. */
public final class PcapConsumerPipeline {
    private PcapConsumerPipeline() { }

    public static PcapConsumerPipelineResult build(
            StreamExecutionEnvironment env, PcapConsumerConfig config) {
        if (env == null || config == null) throw new IllegalArgumentException("PCAP environment and config are required");
        config.validate();
        KafkaSource<PcapRawKafkaRecord> source = KafkaSource.<PcapRawKafkaRecord>builder()
                .setBootstrapServers(config.getBrokers())
                .setTopics(config.getInputTopic())
                .setGroupId(config.getGroupId())
                .setStartingOffsets(config.getStartingOffsets())
                .setDeserializer(new PcapKafkaRecordDeserializationSchema())
                .setProperties(config.getCheckpointBoundSourceProperties())
                .build();
        DataStream<PcapRawKafkaRecord> raw = env
                .fromSource(source, WatermarkStrategy.noWatermarks(), "PCAP Raw Kafka Source")
                .uid(PcapConsumerConfig.SOURCE_UID)
                .name("PCAP Raw Kafka Source");
        SingleOutputStreamOperator<PcapIndexedRecord> parsed = raw
                .process(new PcapIndexParseFunction())
                .uid(PcapConsumerConfig.PARSE_UID)
                .name("PCAP Raw Parse and Classify");
        parsed.getSideOutput(PcapIndexParseFunction.DLQ_TAG)
                .sinkTo(DLQSinkFactory.createDLQSink(
                        config.getBrokers(), config.getDlqTopic(), config.getKafkaProperties()))
                .uid(PcapConsumerConfig.DLQ_UID)
                .name("PCAP Canonical DLQ Sink");
        return new PcapConsumerPipelineResult(parsed, List.of(
                PcapConsumerConfig.SOURCE_UID,
                PcapConsumerConfig.PARSE_UID,
                PcapConsumerConfig.DLQ_UID), config.getDownstreamCapability());
    }
}
