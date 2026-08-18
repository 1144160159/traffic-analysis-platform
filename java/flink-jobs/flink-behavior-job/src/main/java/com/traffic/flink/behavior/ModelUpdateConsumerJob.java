package com.traffic.flink.behavior;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.detector.ModelUpdateBroadcastHandler;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.sink.ModelUpdateAckKafkaSinkFactory;
import com.traffic.flink.behavior.source.ModelConsumerReadinessSource;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.streaming.api.functions.source.RichParallelSourceFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.concurrent.TimeUnit;

/** Consumer-first N011 job: shadow loading and ACK only, with no serving path. */
public final class ModelUpdateConsumerJob {
    private static final Logger LOG = LoggerFactory.getLogger(ModelUpdateConsumerJob.class);

    private ModelUpdateConsumerJob() {}

    public static void main(String[] args) throws Exception {
        execute(BehaviorJobConfig.fromArgs(args));
    }

    static void execute(BehaviorJobConfig config) throws Exception {
        validateConsumerOnlyConfig(config);
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(config.getParallelism());
        env.setMaxParallelism(config.getMaxParallelism());
        BehaviorDetectionJob.configureCheckpoint(env, config);
        BehaviorDetectionJob.configureStateBackend(env, config);
        env.setRestartStrategy(RestartStrategies.failureRateRestart(
                3, Time.of(5, TimeUnit.MINUTES), Time.of(30, TimeUnit.SECONDS)));

        buildPipeline(env, config);
        LOG.info("Starting shadow-only model update consumer: deployment={}, profile={}, parallelism={}",
                config.getModelConsumerDeploymentId(), config.getModelConsumerProfileSha256(),
                config.getParallelism());
        env.execute("Governed Model Update Consumer (Shadow Only)");
    }

    static void buildPipeline(StreamExecutionEnvironment env, BehaviorJobConfig config) {
        validateConsumerOnlyConfig(config);
        KafkaSource<ModelUpdateEvent> updateSource = BehaviorDetectionJob.createModelUpdateSource(config);
        DataStream<ModelUpdateEvent> updates = env
                .fromSource(updateSource, WatermarkStrategy.noWatermarks(), "Kafka-ModelUpdates-ShadowOnly")
                .uid("model-shadow-kafka-source")
                .name("Kafka Source (model-updates shadow-only)");
        DataStream<FeatureStat> idleCarrier = env
                .addSource(new IdleFeatureCarrierSource())
                .setParallelism(1)
                .uid("model-shadow-idle-carrier")
                .name("Idle Feature Carrier (no business records)");
        SingleOutputStreamOperator<FeatureStat> shadowOperator = idleCarrier
                .connect(updates.broadcast(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                        ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE))
                .process(new ModelUpdateBroadcastHandler(config))
                .setParallelism(config.getParallelism())
                .uid("model-shadow-broadcast")
                .name("Governed Model Shadow Loader");

        DataStream<ModelUpdateAppliedAck> shadowAcks =
                shadowOperator.getSideOutput(ModelUpdateBroadcastHandler.MODEL_UPDATE_ACK_TAG);
        DataStream<ModelUpdateAppliedAck> readiness = env
                .addSource(new ModelConsumerReadinessSource(config))
                .setParallelism(config.getParallelism())
                .uid("model-shadow-consumer-readiness")
                .name("Model Shadow Consumer Readiness");
        shadowAcks.union(readiness)
                .sinkTo(ModelUpdateAckKafkaSinkFactory.createSink(
                        config.getKafkaBrokers(), config.getModelAppliedTopic()))
                .setParallelism(config.getParallelism())
                .uid("model-shadow-ack-sink")
                .name("Kafka Sink (model shadow ACK)");
    }

    static void validateConsumerOnlyConfig(BehaviorJobConfig config) {
        config.validateModelUpdateConsumerConfig();
        if (!config.isModelUpdateConsumerEnabled()) {
            throw new IllegalArgumentException("MODEL_UPDATE_CONSUMER_V1_ENABLED must be true");
        }
        if (config.isModelHotUpdateEnabled()) {
            throw new IllegalArgumentException("shadow-only model consumer forbids MODEL_HOT_UPDATE");
        }
        if (!"off".equalsIgnoreCase(config.getDetectionMode())) {
            throw new IllegalArgumentException("shadow-only model consumer requires detection.mode=off");
        }
        if (config.getParallelism() <= 0) {
            throw new IllegalArgumentException("shadow-only model consumer parallelism must be positive");
        }
    }

    /** Keeps the connected operator alive without emitting a synthetic FeatureStat. */
    static final class IdleFeatureCarrierSource extends RichParallelSourceFunction<FeatureStat> {
        private static final long serialVersionUID = 1L;
        private volatile boolean running = true;

        @Override
        public void run(SourceContext<FeatureStat> context) throws Exception {
            context.markAsTemporarilyIdle();
            while (running) {
                try {
                    Thread.sleep(1000L);
                } catch (InterruptedException interrupted) {
                    if (running) {
                        throw interrupted;
                    }
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }

        @Override
        public void cancel() {
            running = false;
        }
    }
}
