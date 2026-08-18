package com.traffic.flink.alert.router;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.ConfigUtil;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.serialization.SimpleStringSchema;
import org.apache.flink.connector.base.DeliveryGuarantee;
import org.apache.flink.connector.kafka.sink.KafkaRecordSerializationSchema;
import org.apache.flink.connector.kafka.sink.KafkaSink;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.streaming.api.datastream.BroadcastStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.kafka.clients.consumer.OffsetResetStrategy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.util.Properties;

/**
 * RunRouterJob —— 共享分支 run 派生(Flink 常驻):
 * flow.events.v1(base,事实流)× analysis.run.events.v1(订阅广播) → analysis.envelopes.v1(run-scoped)。
 *
 * 合同(ATC-RTR-001):base 流保持无归属事实语义;同一事件可命中多个重叠 run,
 * 派生 0..N 个 envelope;只有 ACTIVE、revision 单调、窗口覆盖、fencing 有效的订阅才派生。
 */
public final class RunRouterJob {

    private static final Logger LOG = LoggerFactory.getLogger(RunRouterJob.class);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private RunRouterJob() {}

    public static void main(String[] args) throws Exception {
        RunRouterJobConfig config = RunRouterJobConfig.fromEnv();
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(1);
        env.enableCheckpointing(30_000L);

        // 1. 订阅广播流(analysis.run.events.v1,JSON)
        KafkaSource<String> subscriptionSource = KafkaSource.<String>builder()
                .setBootstrapServers(config.kafkaBrokers())
                .setTopics(config.subscriptionTopic())
                .setGroupId(config.subscriptionConsumerGroup())
                .setStartingOffsets(OffsetsInitializer.committedOffsets(OffsetResetStrategy.EARLIEST))
                .setValueOnlyDeserializer(new SimpleStringSchema())
                .setProperties(consumerProps())
                .build();
        DataStream<String> subscriptionRaw = env
                .fromSource(subscriptionSource, WatermarkStrategy.noWatermarks(), "RunSubscriptionSource")
                .uid("run-subscription-source")
                .name("Kafka Source (analysis.run.events.v1)");

        DataStream<RunSubscriptionRecord> subscriptions = subscriptionRaw
                .map(raw -> MAPPER.readValue(raw, RunSubscriptionRecord.class))
                .name("Parse RunSubscription")
                .uid("run-subscription-parse");

        BroadcastStream<RunSubscriptionRecord> broadcast = subscriptions
                .broadcast(RunRouterProcessFunction.SUBSCRIPTION_STATE);

        // 2. base 流(flow.events.v1,protobuf FlowEvent)
        KafkaSource<RawKafkaRecord> flowSource = KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(config.kafkaBrokers())
                .setTopics(config.flowTopic())
                .setGroupId(config.flowConsumerGroup())
                // 新订阅组从最新开始(候选部署不重放历史;回放验证以新 run 窗口内事件为准)
                .setStartingOffsets(OffsetsInitializer.latest())
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(consumerProps())
                .build();
        DataStream<RawKafkaRecord> flowRaw = env
                .fromSource(flowSource, WatermarkStrategy.noWatermarks(), "BaseFlowSource")
                .uid("run-router-flow-source")
                .name("Kafka Source Raw (flow.events.v1)");

        SingleOutputStreamOperator<String> envelopes = flowRaw
                .connect(broadcast)
                .process(new RunRouterProcessFunction(config.flowTopic(), config.flowConsumerGroup()))
                .uid("run-scope-router")
                .name("RunScopeRouter (flow × subscription → envelopes)");

        // 3. envelope sink(analysis.envelopes.v1,JSON)
        Properties producerProps = producerProps();
        KafkaSink<String> envelopeSink = KafkaSink.<String>builder()
                .setBootstrapServers(config.kafkaBrokers())
                .setRecordSerializer(KafkaRecordSerializationSchema.<String>builder()
                        .setTopic(config.envelopeTopic())
                        .setValueSerializationSchema(new SimpleStringSchema(StandardCharsets.UTF_8))
                        .build())
                .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                .setKafkaProducerConfig(producerProps)
                .build();
        envelopes.sinkTo(envelopeSink)
                .uid("run-envelope-sink")
                .name("Kafka Sink (analysis.envelopes.v1)");

        // 4. 解析失败 DLQ(不静默丢弃)
        envelopes.getSideOutput(RunRouterProcessFunction.FLOW_PARSE_DLQ_TAG)
                .sinkTo(KafkaSink.<String>builder()
                        .setBootstrapServers(config.kafkaBrokers())
                        .setRecordSerializer(KafkaRecordSerializationSchema.<String>builder()
                                .setTopic(config.dlqTopic())
                                .setValueSerializationSchema(new SimpleStringSchema(StandardCharsets.UTF_8))
                                .build())
                        .setDeliveryGuarantee(DeliveryGuarantee.AT_LEAST_ONCE)
                        .setKafkaProducerConfig(producerProps)
                        .build())
                .uid("run-router-dlq-sink")
                .name("RunRouter FlowParse DLQ");

        LOG.info("RunRouterJob start: flow={} subscriptions={} envelopes={} group={}",
                config.flowTopic(), config.subscriptionTopic(), config.envelopeTopic(),
                config.flowConsumerGroup());
        env.execute("Run Scope Router Job");
    }

    private static Properties consumerProps() {
        Properties props = ConfigUtil.kafkaClientProperties();
        props.setProperty("partition.discovery.interval.ms", "30000");
        props.setProperty("enable.auto.commit", "false");
        props.setProperty("commit.offsets.on.checkpoint", "true");
        props.setProperty("isolation.level", "read_committed");
        return props;
    }

    private static Properties producerProps() {
        Properties props = ConfigUtil.kafkaClientProperties();
        props.setProperty("acks", "all");
        props.setProperty("retries", "3");
        props.setProperty("compression.type", "lz4");
        props.setProperty("enable.idempotence", "true");
        return props;
    }
}
