////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-behavior-job/src/main/java/com/traffic/flink/behavior/detector/ModelUpdateBroadcastHandler.java
// Flink BroadcastProcessFunction — 接收 Kafka model-updates 广播到所有并行子任务
//
// 架构:
//   1. Kafka model-updates topic → Broadcast Stream (keyBy constant → broadcast)
//   2. Every BehaviorDetectorFunction instance receives the broadcast event
//   3. ModelRegistry on each TaskManager hot-reloads the updated model
//
// 广播状态模式:
//   - Broadcast State 存储 modelId → (version, artifactUri)
//   - 每个并行子任务的 ModelRegistry 独立管理自己的模型生命周期
//   - 无需分布式协调 — 每个子任务消费同一份 Kafka 消息
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.util.Map;

/**
 * 模型更新广播处理器
 *
 * Broadcast State Key: modelId (String)
 * Broadcast State Value: ModelUpdateState (包含 version + artifactUri + timestamp)
 *
 * 工作流程:
 *   1. 接收 Go Model Registry 通过 Kafka 发送的模型更新事件
 *   2. 更新 Broadcast State
 *   3. BehaviorDetectorFunction 在 processElement 中检查 Broadcast State
 *   4. 如果检测到新版本，触发 ModelRegistry.reload(modelId, version, artifactUri)
 */
public class ModelUpdateBroadcastHandler
        extends BroadcastProcessFunction<FeatureStat, ModelUpdateEvent, FeatureStat> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ModelUpdateBroadcastHandler.class);

    /**
     * Broadcast State 描述符 — 存储活跃模型版本信息
     */
    public static final MapStateDescriptor<String, ModelUpdateState> MODEL_UPDATE_STATE =
            new MapStateDescriptor<>(
                    "model-update-broadcast-state",
                    String.class,            // Key: modelId
                    ModelUpdateState.class   // Value: version + artifact info
            );

    public static final MapStateDescriptor<String, Long> PROCESSED_EVENT_STATE =
            new MapStateDescriptor<>("model-update-processed-events", String.class, Long.class);

    public static final OutputTag<ModelUpdateAppliedAck> MODEL_UPDATE_ACK_TAG =
            new OutputTag<ModelUpdateAppliedAck>("model-update-applied-acks") {};

    private static final int MAX_PROCESSED_EVENTS = 2048;

    private final BehaviorJobConfig registryConfig;
    private transient ModelRegistry taskManagerRegistry;

    public ModelUpdateBroadcastHandler() {
        this.registryConfig = null;
    }

    public ModelUpdateBroadcastHandler(ModelRegistry registry) {
        this.registryConfig = null;
        this.taskManagerRegistry = registry;
    }

    public ModelUpdateBroadcastHandler(BehaviorJobConfig registryConfig) {
        this.registryConfig = registryConfig;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        if (taskManagerRegistry == null) {
            if (registryConfig == null) {
                LOG.info("Model update handler opened without a registry; activation events will fail closed");
                return;
            }
            // Function instances are serialized between the submitting client and
            // TaskManagers. Construct the runtime registry here instead of trying
            // to serialize native model/loader state from the client JVM.
            taskManagerRegistry = new ModelRegistry(registryConfig);
            LOG.info("TaskManager ModelRegistry initialized for model update broadcast subtask {}",
                    getRuntimeContext().getIndexOfThisSubtask());
        }
    }

    @Override
    public void close() throws Exception {
        if (taskManagerRegistry != null) {
            taskManagerRegistry.close();
            taskManagerRegistry = null;
        }
        super.close();
    }

    @Override
    public void processElement(FeatureStat value, ReadOnlyContext ctx, Collector<FeatureStat> out) throws Exception {
        out.collect(value);
    }

    @Override
    public void processBroadcastElement(ModelUpdateEvent event, Context ctx, Collector<FeatureStat> out) throws Exception {
        LOG.info("Received model update broadcast: eventId={}, tenant={}, modelId={}, model={}, version={}, action={}",
                event.getEventId(), event.getTenantId(), event.getModelId(), event.getModelName(),
                event.getVersion(), event.getAction());

        BroadcastState<String, ModelUpdateState> state =
                ctx.getBroadcastState(MODEL_UPDATE_STATE);
        BroadcastState<String, Long> processedEvents =
                ctx.getBroadcastState(PROCESSED_EVENT_STATE);

        String processedKey = processedEventKey(event);
        if (event.getEventId() != null && !event.getEventId().isBlank()
                && processedEvents.contains(processedKey)) {
            LOG.info("Ignoring processed model update event replay: eventId={}, tenant={}, modelId={}",
                    event.getEventId(), event.getTenantId(), event.getModelId());
            return;
        }

        int subtaskIndex = getRuntimeContext().getIndexOfThisSubtask();
        int parallelism = getRuntimeContext().getNumberOfParallelSubtasks();
        String modelKey = modelStateKey(event);

        switch (event.getAction()) {
            case "activated":
            case "activate":
            case "rollback-activated":
                try {
                    if (taskManagerRegistry == null) {
                        throw new IllegalStateException("TaskManager ModelRegistry is unavailable");
                    }
                    ModelRegistry.ApplyReceipt receipt = taskManagerRegistry.applyModelUpdate(
                            event.getTenantId(), event.getModelId(), event.getModelName(),
                            event.getModelType(), event.getVersion(), event.getArtifactUri(),
                            event.getArtifactSha256(), event.getThreshold(0.5f));
                    ModelUpdateState updateState = new ModelUpdateState(
                            event.getModelType(), event.getVersion(), event.getArtifactUri(),
                            event.getAction(), event.getEventId(), System.currentTimeMillis());
                    updateState.setPending(false);
                    state.put(modelKey, updateState);
                    recordProcessedEvent(processedEvents, processedKey);
                    ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.applied(
                            event,
                            new ModelUpdateAppliedAck.ModelRegistryReceipt(
                                    receipt.getArtifactSha256(), receipt.getWarmupScore()),
                            subtaskIndex, parallelism));
                    LOG.info("Model artifact applied and acknowledged: eventId={}, tenant={}, modelId={}, "
                                    + "version={}, sha256={}, warmupScore={}, subtask={}/{}",
                            event.getEventId(), event.getTenantId(), event.getModelId(), event.getVersion(),
                            receipt.getArtifactSha256(), receipt.getWarmupScore(),
                            subtaskIndex, parallelism);
                } catch (Exception applyError) {
                    ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.failed(
                            event, applyError.getMessage(), subtaskIndex, parallelism));
                    LOG.error("Model artifact application failed: eventId={}, tenant={}, modelId={}, version={}",
                            event.getEventId(), event.getTenantId(), event.getModelId(),
                            event.getVersion(), applyError);
                }
                break;

            case "deprecated":
            case "deprecate":
                // 移除弃用的模型版本
                state.remove(modelKey);
                if (taskManagerRegistry != null) {
                    taskManagerRegistry.removeTenantModel(event.getTenantId(), event.getModelId());
                }
                recordProcessedEvent(processedEvents, processedKey);
                LOG.info("Model deprecation broadcast: tenant={}, modelId={}",
                        event.getTenantId(), event.getModelId());
                break;

            case "registered":
                // 新版本注册但不自动激活
                LOG.info("Model registration broadcast received: model={}, version={} (pending activation)",
                        event.getModelName(), event.getVersion());
                recordProcessedEvent(processedEvents, processedKey);
                break;

            default:
                LOG.warn("Unknown model update action: {}", event.getAction());
        }
    }

    private static String modelStateKey(ModelUpdateEvent event) {
        return safe(event.getTenantId()) + '\u001f' + safe(event.getModelId());
    }

    private static String processedEventKey(ModelUpdateEvent event) {
        return modelStateKey(event) + '\u001f' + safe(event.getEventId());
    }

    private static String safe(String value) {
        return value == null ? "" : value;
    }

    private static void recordProcessedEvent(BroadcastState<String, Long> state, String key) throws Exception {
        if (key.endsWith("\u001f")) {
            return;
        }
        state.put(key, System.currentTimeMillis());
        int size = 0;
        String oldestKey = null;
        long oldestTimestamp = Long.MAX_VALUE;
        for (Map.Entry<String, Long> entry : state.entries()) {
            size++;
            long timestamp = entry.getValue() == null ? 0L : entry.getValue();
            if (timestamp < oldestTimestamp) {
                oldestTimestamp = timestamp;
                oldestKey = entry.getKey();
            }
        }
        if (size > MAX_PROCESSED_EVENTS && oldestKey != null && !oldestKey.equals(key)) {
            state.remove(oldestKey);
        }
    }

    static boolean isActivationAction(String action) {
        return "activated".equals(action)
                || "activate".equals(action)
                || "rollback-activated".equals(action);
    }

    static boolean isDuplicateEvent(ModelUpdateState currentState, ModelUpdateEvent event) {
        return currentState != null
                && event.getEventId() != null
                && !event.getEventId().isBlank()
                && event.getEventId().equals(currentState.getEventId());
    }

    /**
     * 设置 TaskManager 端的 ModelRegistry 引用
     * 由 BehaviorDetectorFunction.open() 调用注入
     */
    public void setModelRegistry(ModelRegistry registry) {
        this.taskManagerRegistry = registry;
    }

    // =============================================================================
    // ModelUpdateState — Broadcast State Value
    // =============================================================================

    /**
     * 广播状态中的模型更新信息
     */
    public static class ModelUpdateState implements Serializable {
        private static final long serialVersionUID = 1L;

        private String modelType;
        private String version;
        private String artifactUri;
        private String action;
        private String eventId;
        private long timestamp;
        private boolean pending;

        public ModelUpdateState() {}

        public ModelUpdateState(String modelType, String version, String artifactUri,
                               String action, String eventId, long timestamp) {
            this.modelType = modelType;
            this.version = version;
            this.artifactUri = artifactUri;
            this.action = action;
            this.eventId = eventId;
            this.timestamp = timestamp;
            this.pending = true;
        }

        public String getModelType() { return modelType; }
        public void setModelType(String modelType) { this.modelType = modelType; }

        public String getVersion() { return version; }
        public void setVersion(String version) { this.version = version; }

        public String getArtifactUri() { return artifactUri; }
        public void setArtifactUri(String artifactUri) { this.artifactUri = artifactUri; }

        public String getAction() { return action; }
        public void setAction(String action) { this.action = action; }

        public String getEventId() { return eventId; }
        public void setEventId(String eventId) { this.eventId = eventId; }

        public long getTimestamp() { return timestamp; }
        public void setTimestamp(long timestamp) { this.timestamp = timestamp; }

        public boolean isPending() { return pending; }
        public void setPending(boolean pending) { this.pending = pending; }

        @Override
        public String toString() {
            return String.format("ModelUpdateState{type=%s, version=%s, action=%s, pending=%s}",
                    modelType, version, action, pending);
        }
    }
}
