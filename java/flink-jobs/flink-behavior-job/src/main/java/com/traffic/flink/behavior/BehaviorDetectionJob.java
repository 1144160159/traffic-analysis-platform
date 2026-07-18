////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/BehaviorDetectionJob.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.detector.BehaviorDetectorFunction;
import com.traffic.flink.behavior.detector.ModelRegistry;
import com.traffic.flink.behavior.detector.ModelUpdateBroadcastHandler;
import com.traffic.flink.behavior.detector.SyncBehaviorDetector;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.flink.behavior.sink.BehaviorClickHouseSinkFactory;
import com.traffic.flink.behavior.sink.BehaviorKafkaSinkFactory;
import com.traffic.flink.behavior.sink.ModelUpdateAckKafkaSinkFactory;
import com.traffic.flink.common.ProtoDeserializer;
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
 * 输出: detections.behavior.v1 (Kafka) + detections_behavior_local (ClickHouse)
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

    public static void main(String[] args) throws Exception {
        LOG.info("========================================");
        LOG.info("Starting Behavior Detection Job...");
        LOG.info("========================================");

        // 加载配置
        BehaviorJobConfig config = BehaviorJobConfig.fromArgs(args);
        LOG.info("Configuration loaded: {}", config);

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

        // 初始化模型注册表
        ModelRegistry modelRegistry = new ModelRegistry(config);
        LOG.info("Model registry initialized with {} models", modelRegistry.getModelCount());

        // 创建 Kafka Source
        KafkaSource<FeatureStat> source = createKafkaSource(config);
        KafkaSource<ModelUpdateEvent> modelUpdateSource = createModelUpdateSource(config);

        // 配置水印策略
        WatermarkStrategy<FeatureStat> watermarkStrategy = WatermarkStrategy
                .<FeatureStat>forBoundedOutOfOrderness(
                        Duration.ofMillis(config.getWatermarkDelayMs()))
                .withTimestampAssigner((feature, timestamp) -> feature.getTs())
                .withIdleness(Duration.ofMinutes(1));

        // 构建数据流
        DataStream<FeatureStat> featureStream = env
                .fromSource(source, watermarkStrategy, "Kafka-FeatureStats")
                .uid("kafka-source")
                .name("Kafka Source (feature.stat.v1)");

        // 过滤无效数据
        DataStream<FeatureStat> validFeatureStream = featureStream
                .filter(feature -> feature != null && 
                        feature.hasHeader() &&
                        feature.getHeader().getTenantId() != null &&
                        !feature.getHeader().getTenantId().isEmpty() &&
                        feature.getObjectId() != null &&
                        !feature.getObjectId().isEmpty())
                .uid("filter-invalid")
                .name("Filter Invalid Features");

        DataStream<ModelUpdateEvent> modelUpdateStream = env
                .fromSource(modelUpdateSource, WatermarkStrategy.noWatermarks(), "Kafka-ModelUpdates")
                .uid("kafka-model-update-source")
                .name("Kafka Source (model-updates)");

        SingleOutputStreamOperator<FeatureStat> featuresWithModelUpdates = validFeatureStream
                .connect(modelUpdateStream.broadcast(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE))
                .process(new ModelUpdateBroadcastHandler(config))
                .uid("model-update-broadcast")
                .name("Model Update Broadcast");

        DataStream<ModelUpdateAppliedAck> modelAppliedAcks =
                featuresWithModelUpdates.getSideOutput(ModelUpdateBroadcastHandler.MODEL_UPDATE_ACK_TAG);
        modelAppliedAcks.sinkTo(ModelUpdateAckKafkaSinkFactory.createSink(
                        config.getKafkaBrokers(), config.getModelAppliedTopic()))
                .uid("model-update-applied-ack-sink")
                .name("Kafka Sink (model-update-applied.v1)");

        // 行为检测（使用异步 I/O 进行模型推理）
        SingleOutputStreamOperator<DetectionBehavior> detectionStream;
        
        if (config.isAsyncInferenceEnabled()) {
            // 异步模式：高吞吐量，适合生产环境
            detectionStream = AsyncDataStream.unorderedWait(
                    featuresWithModelUpdates,
                    new BehaviorDetectorFunction(config, modelRegistry),
                    config.getAsyncTimeoutMs(),
                    TimeUnit.MILLISECONDS,
                    config.getAsyncCapacity()
            ).uid("async-behavior-detector").name("Async Behavior Detector");
        } else {
            // 同步模式：用于调试和测试
            detectionStream = featuresWithModelUpdates
                    .flatMap(new SyncBehaviorDetector(config, modelRegistry))
                    .uid("sync-behavior-detector")
                    .name("Sync Behavior Detector");
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
        ).uid("kafka-sink").name("Kafka Sink (detections.behavior.v1)");

        // 调试模式：打印检测结果
        if (config.isDebugPrintEnabled()) {
            validDetections.print("Detection").uid("print-sink");
        }

        // 打印作业信息
        printJobInfo(config);

        // 执行作业
        env.execute("Behavior Detection Job");
    }

    /**
     * 创建 Kafka Source
     */
    private static KafkaSource<FeatureStat> createKafkaSource(BehaviorJobConfig config) {
        return KafkaSource.<FeatureStat>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getInputTopic())
                .setGroupId(config.getConsumerGroupId())
                .setStartingOffsets(OffsetsInitializer.committedOffsets(
                        org.apache.kafka.clients.consumer.OffsetResetStrategy.LATEST))
                .setValueOnlyDeserializer(new ProtoDeserializer<>(FeatureStat.class))
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
    private static KafkaSource<ModelUpdateEvent> createModelUpdateSource(BehaviorJobConfig config) {
        return KafkaSource.<ModelUpdateEvent>builder()
                .setBootstrapServers(config.getKafkaBrokers())
                .setTopics(config.getModelUpdateTopic())
                .setGroupId(config.getConsumerGroupId() + "-model-updates")
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
    private static void configureCheckpoint(StreamExecutionEnvironment env, BehaviorJobConfig config) {
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
    private static void configureStateBackend(StreamExecutionEnvironment env, BehaviorJobConfig config) {
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
        LOG.info("  Consumer Group: {}", config.getConsumerGroupId());
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
