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

import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;

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

    private transient ModelRegistry taskManagerRegistry;

    public ModelUpdateBroadcastHandler() {
    }

    public ModelUpdateBroadcastHandler(ModelRegistry registry) {
        this.taskManagerRegistry = registry;
    }

    @Override
    public void processElement(FeatureStat value, ReadOnlyContext ctx, Collector<FeatureStat> out) throws Exception {
        // 从 Broadcast State 获取当前活跃的模型版本
        ReadOnlyBroadcastState<String, ModelUpdateState> state =
                ctx.getBroadcastState(MODEL_UPDATE_STATE);

        // 检查是否有模型更新需要加载
        for (Map.Entry<String, ModelUpdateState> entry : state.immutableEntries()) {
            String modelId = entry.getKey();
            ModelUpdateState updateState = entry.getValue();
            if (updateState != null && updateState.isPending()) {
                // 触发模型热重载
                if (taskManagerRegistry != null) {
                    LOG.info("Triggering model reload: modelId={}, version={}, artifact={}",
                            modelId, updateState.getVersion(), updateState.getArtifactUri());
                    taskManagerRegistry.applyModelUpdate(
                            modelId,
                            updateState.getModelType(),
                            updateState.getVersion(),
                            updateState.getArtifactUri());
                    updateState.setPending(false);
                }
            }
        }

        out.collect(value);
    }

    @Override
    public void processBroadcastElement(ModelUpdateEvent event, Context ctx, Collector<FeatureStat> out) throws Exception {
        LOG.info("Received model update broadcast: model={}, version={}, action={}",
                event.getModelName(), event.getVersion(), event.getAction());

        BroadcastState<String, ModelUpdateState> state =
                ctx.getBroadcastState(MODEL_UPDATE_STATE);

        switch (event.getAction()) {
            case "activated":
            case "activate":
                // 激活新模型版本
                ModelUpdateState updateState = new ModelUpdateState(
                        event.getModelType(),
                        event.getVersion(),
                        event.getArtifactUri(),
                        event.getAction(),
                        System.currentTimeMillis()
                );
                updateState.setPending(true);
                state.put(event.getModelName(), updateState);
                LOG.info("Model activation broadcast stored: model={}, version={}",
                        event.getModelName(), event.getVersion());
                ModelRegistry.applyGlobalModelUpdate(
                        event.getModelName(),
                        event.getModelType(),
                        event.getVersion(),
                        event.getArtifactUri());
                if (taskManagerRegistry != null) {
                    taskManagerRegistry.applyModelUpdate(
                            event.getModelName(),
                            event.getModelType(),
                            event.getVersion(),
                            event.getArtifactUri());
                    updateState.setPending(false);
                }
                break;

            case "deprecated":
            case "deprecate":
                // 移除弃用的模型版本
                state.remove(event.getModelName());
                LOG.info("Model deprecation broadcast: removed model={}", event.getModelName());
                break;

            case "registered":
                // 新版本注册但不自动激活
                LOG.info("Model registration broadcast received: model={}, version={} (pending activation)",
                        event.getModelName(), event.getVersion());
                break;

            default:
                LOG.warn("Unknown model update action: {}", event.getAction());
        }
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
        private long timestamp;
        private boolean pending;

        public ModelUpdateState() {}

        public ModelUpdateState(String modelType, String version, String artifactUri,
                               String action, long timestamp) {
            this.modelType = modelType;
            this.version = version;
            this.artifactUri = artifactUri;
            this.action = action;
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
