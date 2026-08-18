////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/BehaviorDetectionJob.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.config.BehaviorActivationPolicy;
import com.traffic.flink.behavior.detector.BehaviorDetectorFunction;
import com.traffic.flink.behavior.detector.BehaviorInferenceOutcome;
import com.traffic.flink.behavior.detector.BehaviorOutcomeSplitter;
import com.traffic.flink.behavior.detector.ChampionChallengerShadowFunction;
import com.traffic.flink.behavior.detector.ModelRegistry;
import com.traffic.flink.behavior.detector.ModelUpdateBroadcastHandler;
import com.traffic.flink.behavior.detector.SyncBehaviorDetector;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.flink.behavior.model.ChampionChallengerObservation;
import com.traffic.flink.behavior.model.ShadowEvaluationRequest;
import com.traffic.flink.behavior.source.ModelConsumerReadinessSource;
import com.traffic.flink.behavior.sink.BehaviorClickHouseSinkFactory;
import com.traffic.flink.behavior.sink.BehaviorDlqSinkFactory;
import com.traffic.flink.behavior.sink.BehaviorKafkaSinkFactory;
import com.traffic.flink.behavior.sink.ModelUpdateAckKafkaSinkFactory;
import com.traffic.flink.behavior.sink.ChampionChallengerObservationKafkaSinkFactory;
import com.traffic.flink.behavior.source.BehaviorFeatureLatenessFunction;
import com.traffic.flink.behavior.source.BehaviorFeatureParseFunction;
import com.traffic.flink.behavior.source.ValidatedBehaviorFeature;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.RawKafkaRecordDeserializationSchema;
import com.traffic.flink.common.ProtoTypeInformation;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.serialization.DeserializationSchema;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.common.eventtime.WatermarkStrategy;
import org.apache.flink.api.common.restartstrategy.RestartStrategies;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.connector.kafka.source.KafkaSource;
import org.apache.flink.connector.kafka.source.enumerator.initializer.OffsetsInitializer;
import org.apache.flink.contrib.streaming.state.EmbeddedRocksDBStateBackend;
import org.apache.flink.runtime.state.storage.FileSystemCheckpointStorage;
import org.apache.flink.streaming.api.CheckpointingMode;
import org.apache.flink.streaming.api.datastream.AsyncDataStream;
import org.apache.flink.streaming.api.datastream.DataStream;
import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.time.Duration;
import java.util.concurrent.TimeUnit;

/**
 * Flink Behavior Detection Job
 * 
 * 使用机器学习模型对网络流量进行行为检测
 * 
 * 功能：
 * 1. 扫描检测 - 识别端口扫描、网络扫描行为
 * 2. 隧道检测 - 识别 DNS 隧道、ICMP 隧道、HTTP 隧道
 * 3. DGA 检测 - 识别域名生成算法生成的恶意域名
 * 4. 加密流量分析 - 分析加密流量中的异常行为
 * 5. 异常检测 - 基于统计的异常行为检测
 * 6. C2 通信检测 - 识别命令与控制通信
 * 7. 数据外泄检测 - 识别数据泄露行为
 * 8. 僵尸网络检测 - 识别僵尸网络控制和被控行为
 * 9. 恶意软件检测 - 识别恶意软件通信模式
 * 10. 钓鱼检测 - 识别钓鱼网站访问行为
 * 
 * 输入: feature.stat.v1 (Kafka)
 * 输出: detections.v1 (Kafka) + detections_behavior_local (ClickHouse)
 * 
 * 架构特点：
 * - 使用异步 I/O 进行模型推理，提高吞吐量
 * - 支持多模型并行推理
 * - 支持模型热更新
 * - 支持灰度发布
 */
public class BehaviorDetectionJob {

    private static final Logger LOG = LoggerFactory.getLogger(BehaviorDetectionJob.class);

    /**
     * 侧输出标签：低置信度检测结果（需要人工复核）
     */
    public static final OutputTag<DetectionBehavior> LOW_CONFIDENCE_TAG = 
            new OutputTag<DetectionBehavior>("low-confidence") {};

    /**
     * 侧输出标签：模型推理错误
     */
    public static final OutputTag<String> MODEL_ERROR_TAG = 
            new OutputTag<String>("model-errors") {};

    /**
     * 侧输出标签：特征异常（用于模型反馈）
     */
    public static final OutputTag<FeatureStat> FEATURE_ANOMALY_TAG = 
            new OutputTag<FeatureStat>("feature-anomalies") {};
    private static final OutputTag<CanonicalDlqMessage> FEATURE_PARSE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("behavior-feature-parse-dlq") {};
    private static final OutputTag<CanonicalDlqMessage> FEATURE_LATE_DLQ_TAG =
            new OutputTag<CanonicalDlqMessage>("behavior-feature-late-dlq") {};

    public static void main(String[] args) throws Exception {
        LOG.info("========================================");
        LOG.info("Starting Behavior Detection Job...");
        LOG.info("========================================");

        // 加载配置
        BehaviorJobConfig config = BehaviorJobConfig.fromArgs(args);
        config.validateChampionChallengerShadowConfig();
        LOG.info("Configuration loaded: {}", config);
        BehaviorActivationPolicy activation = BehaviorActivationPolicy.validate(config);
        if (!activation.shouldRun()) {
            if (config.isModelUpdateConsumerEnabled()) {
                ModelUpdateConsumerJob.execute(config);
                return;
            }
            LOG.info("Behavior detection is default-off; no Kafka consumer or producer is started");
            return;
        }
        LOG.info("Behavior detection activation: mode={}, profileId={}, profileSha256={}",
                activation.getMode(), activation.getProfileId(), activation.getProfileSha256());

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
        WatermarkStrategy<ValidatedBehaviorFeature> watermarkStrategy = WatermarkStrategy
                .<ValidatedBehaviorFeature>forBoundedOutOfOrderness(
                        Duration.ofMillis(config.getWatermarkDelayMs()))
                .withTimestampAssigner((input, timestamp) -> input.getFeature().getTs())
                .withIdleness(Duration.ofMinutes(1));

        // 构建数据流
        DataStream<RawKafkaRecord> featureStream = env
                .fromSource(source, WatermarkStrategy.noWatermarks(), "Kafka-FeatureStats")
                .uid("kafka-source")
                .name("Kafka Source (feature.stat.v1)");

        SingleOutputStreamOperator<ValidatedBehaviorFeature> parsedFeatureStream = featureStream
                .process(new BehaviorFeatureParseFunction(FEATURE_PARSE_DLQ_TAG))
                .uid("filter-invalid")
                .name("Validate FeatureStat with source tuple");

        SingleOutputStreamOperator<ValidatedBehaviorFeature> timestampedFeatureStream =
                parsedFeatureStream
                        .assignTimestampsAndWatermarks(watermarkStrategy)
                        .uid("behavior-feature-watermarks")
                        .name("Assign Behavior Feature Watermarks");

        SingleOutputStreamOperator<FeatureStat> validFeatureStream = timestampedFeatureStream
                .process(new BehaviorFeatureLatenessFunction(
                        config.getAllowedLatenessMs(), FEATURE_LATE_DLQ_TAG))
                .returns(ProtoTypeInformation.forMessage(FeatureStat.class))
                .uid("behavior-feature-lateness")
                .name("Classify Late Behavior Features");

        DataStream<CanonicalDlqMessage> dlqStream = parsedFeatureStream
                .getSideOutput(FEATURE_PARSE_DLQ_TAG)
                .union(validFeatureStream.getSideOutput(FEATURE_LATE_DLQ_TAG));

        DataStream<FeatureStat> featuresWithModelUpdates = validFeatureStream;
        DataStream<ShadowEvaluationRequest> shadowEvaluationRequests = null;
        if (config.isModelUpdateConsumerConfigured()) {
            config.validateModelUpdateConsumerConfig();
            if (config.isModelHotUpdateEnabled() && !activation.allowsHotUpdates()) {
                throw new IllegalArgumentException(
                        "model hot update is only permitted in explicitly activated dynamic mode");
            }
            KafkaSource<ModelUpdateEvent> modelUpdateSource = createModelUpdateSource(config);
            DataStream<ModelUpdateEvent> modelUpdateStream = env
                    .fromSource(modelUpdateSource, WatermarkStrategy.noWatermarks(), "Kafka-ModelUpdates")
                    .uid("kafka-model-update-source")
                    .name("Kafka Source (model-updates)");

            SingleOutputStreamOperator<FeatureStat> updatedFeatures = validFeatureStream
                    .connect(modelUpdateStream.broadcast(
                            ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                            ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                            ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE))
                    .process(new ModelUpdateBroadcastHandler(config))
                    .returns(ProtoTypeInformation.forMessage(FeatureStat.class))
                    .uid("model-update-broadcast")
                    .name("Model Update Broadcast");
            DataStream<ModelUpdateAppliedAck> modelAppliedAcks =
                    updatedFeatures.getSideOutput(ModelUpdateBroadcastHandler.MODEL_UPDATE_ACK_TAG);
            if (!config.isModelShadowEvaluationEnabled()) {
                DataStream<ModelUpdateAppliedAck> consumerReadiness = env
                        .addSource(new ModelConsumerReadinessSource(config))
                        .setParallelism(config.getParallelism())
                        .uid("model-update-consumer-readiness-source")
                        .name("Model Update Consumer Readiness");
                modelAppliedAcks.union(consumerReadiness).sinkTo(
                                ModelUpdateAckKafkaSinkFactory.createSink(
                                        config.getKafkaBrokers(), config.getModelAppliedTopic()))
                        .uid("model-update-applied-ack-sink")
                        .name("Kafka Sink (model-update-applied.v1)");
            } else {
                // N011 remains the sole authoritative readiness/ACK quorum.
                // N012 consumes the immutable event independently and must not
                // let observer replicas satisfy or contaminate that quorum.
                LOG.info("N012 shadow observer suppresses model readiness and shadow ACK publication");
            }
            featuresWithModelUpdates = updatedFeatures;
            shadowEvaluationRequests = updatedFeatures.getSideOutput(
                    ModelUpdateBroadcastHandler.SHADOW_EVALUATION_REQUEST_TAG);
        }

        // N012 observer branch: the serving detector below remains unchanged.
        // Disabling the flag removes this entire branch from the job graph.
        if (config.isModelShadowEvaluationEnabled()) {
            if (shadowEvaluationRequests == null) {
                throw new IllegalStateException(
                        "shadow evaluation request stream is unavailable");
            }
            DataStream<ChampionChallengerObservation> shadowObservations =
                    buildChampionChallengerObserver(shadowEvaluationRequests, config);
            shadowObservations.sinkTo(
                            ChampionChallengerObservationKafkaSinkFactory.create(
                                    config.getKafkaBrokers(),
                                    config.getModelShadowObservationTopic()))
                    .uid("champion-challenger-shadow-observation-sink")
                    .name("Kafka Sink (model-shadow-observations.v1)");
        }

        if (config.isModelShadowObservationOnly()) {
            LOG.info("N012 observation-only mode: serving, ClickHouse and DLQ sinks are absent");
            printJobInfo(config);
            env.execute("Behavior Champion Challenger Shadow Observation Job");
            return;
        }

        // The serving registry is deliberately absent from observation-only jobs.
        ModelRegistry modelRegistry = new ModelRegistry(config);
        LOG.info("Model registry initialized with {} models", modelRegistry.getModelCount());

        // 行为检测（使用异步 I/O 进行模型推理）
        SingleOutputStreamOperator<DetectionBehavior> detectionStream;
        DataStream<DetectionBehavior> lowConfidenceStream = null;
        DataStream<String> modelErrorStream = null;
        
        if (config.isAsyncInferenceEnabled()) {
            // 异步模式：高吞吐量，适合生产环境。
            // 推理结果带 typed outcome（DETECTED/LOW_CONFIDENCE/NO_DETECTION/
            // MODEL_ERROR/TIMEOUT），由 BehaviorOutcomeSplitter 分流：
            // 命中→主流；低置信度→LOW_CONFIDENCE_TAG；错误/超时→MODEL_ERROR_TAG→DLQ。
            SingleOutputStreamOperator<BehaviorInferenceOutcome> outcomeStream =
                    AsyncDataStream.unorderedWait(
                            featuresWithModelUpdates,
                            new BehaviorDetectorFunction(config, modelRegistry),
                            config.getAsyncTimeoutMs(),
                            TimeUnit.MILLISECONDS,
                            config.getAsyncCapacity()
                    ).uid("async-behavior-detector").name("Async Behavior Detector");
            SingleOutputStreamOperator<DetectionBehavior> splitStream = outcomeStream
                    .process(new BehaviorOutcomeSplitter(LOW_CONFIDENCE_TAG, MODEL_ERROR_TAG, FEATURE_ANOMALY_TAG))
                    .uid("behavior-outcome-splitter-v1")
                    .name("Split Behavior Inference Outcomes");
            detectionStream = splitStream;
            lowConfidenceStream = splitStream.getSideOutput(LOW_CONFIDENCE_TAG);
            modelErrorStream = splitStream.getSideOutput(MODEL_ERROR_TAG);
        } else {
            // 同步模式：用于调试和测试
            detectionStream = featuresWithModelUpdates
                    .flatMap(new SyncBehaviorDetector(config, modelRegistry))
                    .uid("sync-behavior-detector")
                    .name("Sync Behavior Detector");
        }

        // 模型推理错误/超时接入 canonical DLQ（不再静默丢失）。
        // 错误消息无 Kafka 源坐标，使用合成 RawKafkaRecord 保留语义载荷。
        if (modelErrorStream != null) {
            final String featureTopic = config.getInputTopic();
            dlqStream = dlqStream.union(modelErrorStream.map(error -> {
                        RawKafkaRecord synthetic = new RawKafkaRecord(
                                featureTopic, -1, -1L, System.currentTimeMillis(),
                                null, new byte[0], new java.util.HashMap<>());
                        return CanonicalDlqMessage.failure(
                                synthetic, "MODEL_INFERENCE_ERROR", "model_inference",
                                error, "", "", "", "", "",
                                "flink-behavior-job", "text/plain", "inference-error", "v1");
                    })
                    .uid("behavior-model-error-dlq-mapper-v1")
                    .name("Map model inference errors to canonical DLQ"));
        }
        if (lowConfidenceStream != null && config.isDebugPrintEnabled()) {
            lowConfidenceStream.print("LowConfidence").setParallelism(1).uid("print-low-confidence");
        }

        // 过滤空结果和低置信度结果
        DataStream<DetectionBehavior> validDetections = detectionStream
                .filter(detection -> detection != null && 
                        detection.getTopScore() >= config.getMinConfidenceThreshold())
                .uid("filter-low-confidence")
                .name("Filter Low Confidence");

        // Sink 1: ClickHouse（主存储）
        validDetections.addSink(
                BehaviorClickHouseSinkFactory.createSink(
                        config.getClickhouseUrl(),
                        config.getClickhouseDatabase(),
                        config.getClickhouseTable(),
                        config.getClickhouseUser(),
                        config.getClickhousePassword(),
                        config.getClickhouseBatchSize(),
                        config.getClickhouseBatchIntervalMs()
                )
        ).uid("clickhouse-sink").name("ClickHouse Sink (detections_behavior)");

        // Sink 2: Kafka（供下游 AlertJob 消费）
        validDetections.sinkTo(
                BehaviorKafkaSinkFactory.createSink(
                        config.getKafkaBrokers(),
                        config.getOutputTopic()
                )
        ).uid("kafka-sink").name("Kafka Sink (detections.v1)");

        dlqStream.sinkTo(BehaviorDlqSinkFactory.create(
                        config.getKafkaBrokers(), config.getDlqTopic()))
                .uid("behavior-dlq-sink")
                .name("Canonical DLQ Sink");

        // 调试模式：打印检测结果
        if (config.isDebugPrintEnabled()) {
            validDetections.print("Detection").uid("print-sink");
        }

        // 打印作业信息
        printJobInfo(config);

        // 执行作业
        env.execute("Behavior Detection Job");
    }

    static DataStream<ChampionChallengerObservation> buildChampionChallengerObserver(
            DataStream<ShadowEvaluationRequest> requests,
            BehaviorJobConfig config) {
        config.validateChampionChallengerShadowConfig();
        long observerTimeoutMs = config.getModelShadowPackageLoadTimeoutMs()
                + config.getAsyncTimeoutMs()
                + config.getModelShadowChallengerTimeoutMs() + 1000L;
        return AsyncDataStream.unorderedWait(
                        requests,
                        new ChampionChallengerShadowFunction(config),
                        observerTimeoutMs,
                        TimeUnit.MILLISECONDS,
                        config.getAsyncCapacity())
                .uid("champion-challenger-shadow-observer")
                .name("Champion Challenger Shadow Observer");
    }

    /**
     * 创建 Kafka Source
     */
    static KafkaSource<RawKafkaRecord> createKafkaSource(BehaviorJobConfig config) {
        String featureConsumerGroup = config.isModelShadowObservationOnly()
                ? config.getModelShadowFeatureConsumerGroupId()
                : config.getConsumerGroupId();
        return KafkaSource.<RawKafkaRecord>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getInputTopic())
                .setGroupId(featureConsumerGroup)
                .setStartingOffsets(OffsetsInitializer.committedOffsets(
                        org.apache.kafka.clients.consumer.OffsetResetStrategy.LATEST))
                .setDeserializer(new RawKafkaRecordDeserializationSchema())
                .setProperties(com.traffic.flink.common.ConfigUtil.kafkaClientProperties())
                .setProperty("partition.discovery.interval.ms", "30000")
                .setProperty("fetch.min.bytes", "1")
                .setProperty("fetch.max.wait.ms", "500")
                .setProperty("max.poll.records", "1000")
                .build();
    }

    /**
     * 创建模型热更新 Kafka Source
     */
    static KafkaSource<ModelUpdateEvent> createModelUpdateSource(BehaviorJobConfig config) {
        String modelUpdateGroup = config.isModelShadowEvaluationEnabled()
                ? config.getModelShadowUpdateConsumerGroupId()
                : config.getConsumerGroupId() + "-model-updates";
        return KafkaSource.<ModelUpdateEvent>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getModelUpdateTopic())
                .setGroupId(modelUpdateGroup)
                .setStartingOffsets(OffsetsInitializer.committedOffsets(
                        org.apache.kafka.clients.consumer.OffsetResetStrategy.LATEST))
                .setValueOnlyDeserializer(new DeserializationSchema<ModelUpdateEvent>() {
                    private static final long serialVersionUID = 1L;

                    @Override
                    public ModelUpdateEvent deserialize(byte[] message) throws IOException {
                        return ModelUpdateEvent.fromJson(message);
                    }

                    @Override
                    public boolean isEndOfStream(ModelUpdateEvent nextElement) {
                        return false;
                    }

                    @Override
                    public TypeInformation<ModelUpdateEvent> getProducedType() {
                        return TypeInformation.of(ModelUpdateEvent.class);
                    }
                })
                .setProperties(com.traffic.flink.common.ConfigUtil.kafkaClientProperties())
                .setProperty("partition.discovery.interval.ms", "30000")
                .setProperty("fetch.min.bytes", "1")
                .setProperty("fetch.max.wait.ms", "500")
                .build();
    }

    /**
     * 配置 Checkpoint
     */
    static void configureCheckpoint(StreamExecutionEnvironment env, BehaviorJobConfig config) {
        env.enableCheckpointing(config.getCheckpointIntervalMs());

        CheckpointConfig checkpointConfig = env.getCheckpointConfig();

        checkpointConfig.setCheckpointingMode(CheckpointingMode.EXACTLY_ONCE);
        checkpointConfig.setCheckpointTimeout(config.getCheckpointTimeoutMs());
        checkpointConfig.setMinPauseBetweenCheckpoints(config.getCheckpointMinPauseMs());
        checkpointConfig.setMaxConcurrentCheckpoints(1);
        checkpointConfig.setExternalizedCheckpointCleanup(
                CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION);
        
        // 启用非对齐 Checkpoint（减少反压时的 Checkpoint 延迟）
        checkpointConfig.enableUnalignedCheckpoints();

        LOG.info("Checkpoint configured: interval={}ms, timeout={}ms, path={}",
                config.getCheckpointIntervalMs(),
                config.getCheckpointTimeoutMs(),
                config.getCheckpointPath());
    }

    /**
     * 配置状态后端
     */
    static void configureStateBackend(StreamExecutionEnvironment env, BehaviorJobConfig config) {
                EmbeddedRocksDBStateBackend stateBackend = new EmbeddedRocksDBStateBackend(true);
                // Some Flink versions expose enableTtlCompactionFilter(), others do not.
                // Use reflection to call it when available to preserve compatibility.
                try {
                        java.lang.reflect.Method m = stateBackend.getClass().getMethod("enableTtlCompactionFilter");
                        m.invoke(stateBackend);
                } catch (NoSuchMethodException ignored) {
                        LOG.info("enableTtlCompactionFilter() not available in this Flink version; skipping");
                } catch (Exception e) {
                        LOG.warn("Failed to invoke enableTtlCompactionFilter(): {}", e.getMessage());
                }

                env.setStateBackend(stateBackend);
        env.getCheckpointConfig().setCheckpointStorage(
                new FileSystemCheckpointStorage(config.getCheckpointPath()));

        LOG.info("State backend configured: RocksDB with checkpoint storage at {}",
                config.getCheckpointPath());
    }

    /**
     * 打印作业信息
     */
    private static void printJobInfo(BehaviorJobConfig config) {
        LOG.info("========================================");
        LOG.info("Behavior Detection Job Configuration:");
        LOG.info("========================================");
        LOG.info("  Input Topic: {}", config.getInputTopic());
        LOG.info("  Output Topic: {}", config.getOutputTopic());
        LOG.info("  Model Update Topic: {}", config.getModelUpdateTopic());
        LOG.info("  Model Applied Topic: {}", config.getModelAppliedTopic());
        LOG.info("  Consumer Group: {}", config.isModelShadowObservationOnly()
                ? config.getModelShadowFeatureConsumerGroupId()
                : config.getConsumerGroupId());
        if (config.isModelShadowEvaluationEnabled()) {
            LOG.info("  Shadow Update Consumer Group: {}",
                    config.getModelShadowUpdateConsumerGroupId());
            LOG.info("  Shadow Observation Only: {}", config.isModelShadowObservationOnly());
        }
        LOG.info("  Parallelism: {}", config.getParallelism());
        LOG.info("  Max Parallelism: {}", config.getMaxParallelism());
        LOG.info("  Checkpoint Interval: {}ms", config.getCheckpointIntervalMs());
        LOG.info("  Watermark Delay: {}ms", config.getWatermarkDelayMs());
        LOG.info("  Async Inference: {}", config.isAsyncInferenceEnabled());
        if (config.isAsyncInferenceEnabled()) {
            LOG.info("    Async Timeout: {}ms", config.getAsyncTimeoutMs());
            LOG.info("    Async Capacity: {}", config.getAsyncCapacity());
        }
        LOG.info("  Min Confidence Threshold: {}", config.getMinConfidenceThreshold());
        LOG.info("  Model Path: {}", config.getModelPath());
        LOG.info("  Model Version: {}", config.getModelVersion());
        LOG.info("  Enabled Models: {}", config.getEnabledModels());
        LOG.info("  ClickHouse: {}/{}", config.getClickhouseUrl(), config.getClickhouseTable());
        LOG.info("========================================");
    }
}
