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
import com.traffic.flink.behavior.model.ShadowEvaluationRequest;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.proto.traffic.v1.FeatureStat;

import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.Serializable;
import java.util.ArrayList;
import java.util.List;
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
        extends BroadcastProcessFunction<FeatureStat, ModelUpdateEvent, FeatureStat>
        implements CheckpointedFunction {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(ModelUpdateBroadcastHandler.class);

    /**
     * Operator state：已应用的活跃模型元数据（version/artifactUri/sha）。
     * 广播状态本身随 checkpoint 恢复，但广播流不会重放历史事件，而
     * ModelRegistry 的热更新模型只存在于 JVM 静态表 → 恢复后注册表为空、
     * ACK 却声称已应用。此状态让恢复时在 open() 中重新加载模型。
     */
    private static final ListStateDescriptor<ModelUpdateState> RESTORED_MODELS_DESC =
            new ListStateDescriptor<>(
                    "model-update-restored-models-v1", ModelUpdateState.class);

    private transient ListState<ModelUpdateState> restoredModelsState;
    private transient List<ModelUpdateState> modelsToReload;

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

    /** Full immutable candidate events survive savepoint/restart for the N012 observer. */
    public static final MapStateDescriptor<String, ModelUpdateEvent> SHADOW_PACKAGE_EVENT_STATE =
            new MapStateDescriptor<>(
                    "model-shadow-package-events", String.class, ModelUpdateEvent.class);

    public static final OutputTag<ModelUpdateAppliedAck> MODEL_UPDATE_ACK_TAG =
            new OutputTag<ModelUpdateAppliedAck>("model-update-applied-acks") {};

    public static final OutputTag<ShadowEvaluationRequest> SHADOW_EVALUATION_REQUEST_TAG =
            new OutputTag<ShadowEvaluationRequest>("champion-challenger-shadow-requests") {};

    private static final int MAX_PROCESSED_EVENTS = 2048;

    private final BehaviorJobConfig registryConfig;
    private final boolean activationEnabled;
    private transient ModelRegistry taskManagerRegistry;
    // 已应用模型的内存镜像（key=modelStateKey），用于 snapshotState 持久化
    private transient Map<String, ModelUpdateState> appliedModelMirror;

    public ModelUpdateBroadcastHandler() {
        this.registryConfig = null;
        this.activationEnabled = false;
    }

    public ModelUpdateBroadcastHandler(ModelRegistry registry) {
        this.registryConfig = null;
        this.activationEnabled = true;
        this.taskManagerRegistry = registry;
    }

    public ModelUpdateBroadcastHandler(BehaviorJobConfig registryConfig) {
        this.registryConfig = registryConfig;
        this.activationEnabled = registryConfig != null && registryConfig.isModelHotUpdateEnabled();
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
            int subtask = getRuntimeContext().getIndexOfThisSubtask();
            taskManagerRegistry = new ModelRegistry(
                    registryConfig, activationEnabled, "shadow-subtask-" + subtask);
            LOG.info("TaskManager ModelRegistry initialized for model update broadcast subtask {}",
                    subtask);
        }
        // checkpoint 恢复：重载广播状态里已应用的模型元数据到注册表，
        // 避免恢复后注册表为空却对重放 ACK 声称已应用。
        appliedModelMirror = new java.util.HashMap<>();
        if (modelsToReload != null && !modelsToReload.isEmpty()) {
            for (ModelUpdateState state : modelsToReload) {
                if (state.isPending() || state.getArtifactUri() == null
                        || state.getArtifactUri().isBlank()
                        || state.getVersion() == null || state.getVersion().isBlank()) {
                    continue;
                }
                try {
                    taskManagerRegistry.applyModelUpdate(
                            state.getTenantId(), state.getModelId(), state.getModelName(),
                            state.getModelType(), state.getVersion(), state.getArtifactUri(),
                            state.getArtifactSha256(), state.getWarmupScore() > 0f
                                    ? state.getWarmupScore() : 0.5f);
                    appliedModelMirror.put(modelStateKeyOf(state), state);
                    LOG.info("Reloaded model from checkpoint restore: tenant={}, modelId={}, "
                                    + "version={}, sha256={}, subtask={}/{}",
                            state.getTenantId(), state.getModelId(), state.getVersion(),
                            state.getArtifactSha256(), getRuntimeContext().getIndexOfThisSubtask(),
                            getRuntimeContext().getNumberOfParallelSubtasks());
                } catch (Exception reloadError) {
                    // 重载失败不阻塞启动：后续该模型的激活/重放 ACK 会因本地
                    // 版本不匹配而发 failed ACK（fail-closed 可见），由
                    // rule-manager 重试或人工处理。
                    LOG.error("Model reload from checkpoint restore failed: tenant={}, modelId={}, "
                                    + "version={}: {}",
                            state.getTenantId(), state.getModelId(), state.getVersion(),
                            reloadError.getMessage(), reloadError);
                }
            }
            modelsToReload = null;
        }
    }

    @Override
    public void snapshotState(FunctionSnapshotContext context) throws Exception {
        if (restoredModelsState == null || appliedModelMirror == null) {
            return;
        }
        restoredModelsState.clear();
        for (ModelUpdateState model : appliedModelMirror.values()) {
            if (!model.isPending() && model.getArtifactUri() != null
                    && !model.getArtifactUri().isBlank()) {
                restoredModelsState.add(model);
            }
        }
    }

    @Override
    public void initializeState(FunctionInitializationContext context) throws Exception {
        restoredModelsState = context.getOperatorStateStore()
                .getListState(RESTORED_MODELS_DESC);
        modelsToReload = new ArrayList<>();
        if (context.isRestored()) {
            for (ModelUpdateState model : restoredModelsState.get()) {
                modelsToReload.add(model);
            }
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
        if (registryConfig == null || !registryConfig.isModelShadowEvaluationEnabled()) return;
        String tenantId = value.hasHeader() ? value.getHeader().getTenantId() : "";
        String prefix = (tenantId == null ? "" : tenantId) + '\u001f';
        java.util.List<ModelUpdateEvent> candidates = new java.util.ArrayList<>();
        for (Map.Entry<String, ModelUpdateEvent> entry
                : ctx.getBroadcastState(SHADOW_PACKAGE_EVENT_STATE).immutableEntries()) {
            if (entry.getKey().startsWith(prefix)) candidates.add(entry.getValue());
        }
        candidates.sort(java.util.Comparator.comparing(ModelUpdateEvent::getModelId));
        if (!candidates.isEmpty()) {
            ctx.output(SHADOW_EVALUATION_REQUEST_TAG,
                    new ShadowEvaluationRequest(value, candidates));
        }
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

        int subtaskIndex = getRuntimeContext().getIndexOfThisSubtask();
        int parallelism = getRuntimeContext().getNumberOfParallelSubtasks();
        String modelKey = modelStateKey(event);
        String processedKey = processedEventKey(event);
        if (event.getEventId() != null && !event.getEventId().isBlank()
                && processedEvents.contains(processedKey)) {
            if ("shadow-load".equals(event.getAction())) {
                ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowRejected(
                        event, "duplicate", "shadow-load event was already processed",
                        subtaskIndex, parallelism));
			} else if (isActivationAction(event.getAction())) {
				ModelUpdateState current = state.get(modelKey);
				boolean consumerIdentityMatches = !event.getAction().startsWith("rollback-")
						|| rollbackConsumerIdentityMatches(event);
				if (isExactActivationReplay(current, event) && consumerIdentityMatches
						&& localModelVersionMatches(event)) {
					ModelUpdateAppliedAck replayAck = ModelUpdateAppliedAck.replayed(
							event, current.getArtifactSha256(), current.getWarmupScore(),
							subtaskIndex, parallelism);
					if (event.getAction().startsWith("rollback-")) {
						replayAck.withConsumerIdentity(
								registryConfig.getModelConsumerDeploymentId(),
								registryConfig.getModelConsumerProfileSha256());
					}
					ctx.output(MODEL_UPDATE_ACK_TAG, replayAck);
				} else {
					ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.failed(
							event, "processed activation identity differs from restored state"
									+ (localModelVersionMatches(event)
											? "" : " (local model version mismatch)"),
							subtaskIndex, parallelism));
				}
            }
            LOG.info("Acknowledged processed model update event replay: eventId={}, tenant={}, modelId={}",
                    event.getEventId(), event.getTenantId(), event.getModelId());
            return;
        }

        switch (safe(event.getAction())) {
            case "shadow-load":
                processShadowLoad(event, ctx, state, processedEvents, processedKey,
                        modelKey, subtaskIndex, parallelism);
                break;

            case "activated":
            case "activate":
            case "rollback-activated":
			case "rollback-compensate":
                try {
					if (event.getAction().startsWith("rollback-") && !rollbackConsumerIdentityMatches(event)) {
						throw new IllegalStateException(
								"rollback event consumer deployment or profile differs from this runtime");
					}
                    if (!activationEnabled) {
                        throw new IllegalStateException(
                                "activation consumer is disabled; only consumer-ready and shadow-load are allowed");
                    }
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
                    updateState.setTenantId(event.getTenantId());
                    updateState.setModelId(event.getModelId());
                    updateState.setModelName(event.getModelName());
                    updateState.setPending(false);
					updateState.setArtifactSha256(receipt.getArtifactSha256());
					updateState.setWarmupScore(receipt.getWarmupScore());
					updateState.setRollbackId(event.getRollbackId());
					updateState.setRollbackPhase(event.getRollbackPhase());
                    state.put(modelKey, updateState);
                    if (appliedModelMirror != null) {
                        appliedModelMirror.put(modelKey, updateState);
                    }
                    recordProcessedEvent(processedEvents, processedKey);
					ModelUpdateAppliedAck appliedAck = ModelUpdateAppliedAck.applied(
                            event,
                            new ModelUpdateAppliedAck.ModelRegistryReceipt(
                                    receipt.getArtifactSha256(), receipt.getWarmupScore()),
							subtaskIndex, parallelism);
					if (event.getAction().startsWith("rollback-")) {
						appliedAck.withConsumerIdentity(
								registryConfig.getModelConsumerDeploymentId(),
								registryConfig.getModelConsumerProfileSha256());
					}
					ctx.output(MODEL_UPDATE_ACK_TAG, appliedAck);
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
                ctx.getBroadcastState(SHADOW_PACKAGE_EVENT_STATE).remove(modelKey);
                if (appliedModelMirror != null) {
                    appliedModelMirror.remove(modelKey);
                }
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

    private void processShadowLoad(ModelUpdateEvent event, Context ctx,
                                   BroadcastState<String, ModelUpdateState> state,
                                   BroadcastState<String, Long> processedEvents,
                                   String processedKey, String modelKey,
                                   int subtaskIndex, int parallelism) throws Exception {
        ModelUpdateState current = state.get(modelKey);
        String disposition = shadowDisposition(current, event);
        if (!"new".equals(disposition)) {
            recordProcessedEvent(processedEvents, processedKey);
            ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowRejected(
                    event, disposition,
                    "duplicate".equals(disposition)
                            ? "shadow package revision and digest are already staged"
                            : "shadow package aggregate revision is stale or conflicting",
                    subtaskIndex, parallelism));
            return;
        }
        try {
            if (taskManagerRegistry == null) {
                throw new IllegalStateException("TaskManager ModelRegistry is unavailable");
            }
            ModelRegistry.ShadowStageReceipt receipt = taskManagerRegistry.stageShadowPackage(event);
            if (!receipt.isStaged()) {
                recordProcessedEvent(processedEvents, processedKey);
                ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowRejected(
                        event, "duplicate", "shadow package is already staged",
                        subtaskIndex, parallelism));
                return;
            }
            ModelUpdateState updateState = new ModelUpdateState(
                    event.getModelType(), event.getVersion(), event.getArtifactManifestUri(),
                    event.getAction(), event.getEventId(), System.currentTimeMillis());
            updateState.setPending(false);
            updateState.setStage("SHADOW_READY");
            updateState.setPackageId(receipt.getPackageId());
            updateState.setPackageSha256(receipt.getPackageSha256());
            updateState.setAggregateRevision(receipt.getAggregateRevision());
            state.put(modelKey, updateState);
            ctx.getBroadcastState(SHADOW_PACKAGE_EVENT_STATE).put(modelKey, event);
            recordProcessedEvent(processedEvents, processedKey);
            ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowReady(
                    event, receipt.getPackageSha256(), subtaskIndex, parallelism));
            LOG.info("Governed model package staged in shadow only: eventId={}, tenant={}, modelId={}, "
                            + "packageId={}, revision={}, subtask={}/{}",
                    event.getEventId(), event.getTenantId(), event.getModelId(),
                    receipt.getPackageId(), receipt.getAggregateRevision(), subtaskIndex, parallelism);
        } catch (ModelRegistry.StaleShadowRevisionException stale) {
            recordProcessedEvent(processedEvents, processedKey);
            ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowRejected(
                    event, "stale", stale.getMessage(), subtaskIndex, parallelism));
        } catch (Exception loadError) {
            ctx.output(MODEL_UPDATE_ACK_TAG, ModelUpdateAppliedAck.shadowRejected(
                    event, "failed", loadError.getMessage(), subtaskIndex, parallelism));
            LOG.error("Governed shadow package load failed: eventId={}, tenant={}, modelId={}, revision={}",
                    event.getEventId(), event.getTenantId(), event.getModelId(),
                    event.getAggregateRevision(), loadError);
        }
    }

    private static String modelStateKey(ModelUpdateEvent event) {
        return safe(event.getTenantId()) + '\u001f' + safe(event.getModelId());
    }

    private static String modelStateKeyOf(ModelUpdateState state) {
        return safe(state.getTenantId()) + '\u001f' + safe(state.getModelId());
    }

    /**
     * 发 applied/replayed ACK 前校验本地注册表实际加载版本与事件一致，
     * 防止"ACK 声称已应用、实际跑旧模型/内置模型"。
     */
    private boolean localModelVersionMatches(ModelUpdateEvent event) {
        if (taskManagerRegistry == null || registryConfig == null) {
            return false;
        }
        try {
            String localVersion = taskManagerRegistry.getModelVersion(
                    event.getTenantId(), event.getModelId());
            return safe(event.getVersion()).equals(safe(localVersion));
        } catch (Exception e) {
            LOG.warn("Local model version lookup failed: tenant={}, modelId={}: {}",
                    event.getTenantId(), event.getModelId(), e.getMessage());
            return false;
        }
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
                || "rollback-activated".equals(action)
				|| "rollback-compensate".equals(action);
    }

	static boolean isExactActivationReplay(ModelUpdateState currentState, ModelUpdateEvent event) {
		return currentState != null
				&& !currentState.isPending()
				&& safe(event.getEventId()).equals(currentState.getEventId())
				&& safe(event.getVersion()).equals(currentState.getVersion())
				&& safe(event.getArtifactUri()).equals(currentState.getArtifactUri())
				&& safe(event.getAction()).equals(currentState.getAction())
				&& safe(event.getArtifactSha256()).equals(currentState.getArtifactSha256())
				&& safe(event.getRollbackId()).equals(currentState.getRollbackId())
				&& safe(event.getRollbackPhase()).equals(currentState.getRollbackPhase());
	}

	private boolean rollbackConsumerIdentityMatches(ModelUpdateEvent event) {
		return registryConfig != null
				&& safe(event.getConsumerDeploymentId()).equals(registryConfig.getModelConsumerDeploymentId())
				&& safe(event.getConsumerProfileSha256()).equals(registryConfig.getModelConsumerProfileSha256());
	}

    static boolean isDuplicateEvent(ModelUpdateState currentState, ModelUpdateEvent event) {
        return currentState != null
                && event.getEventId() != null
                && !event.getEventId().isBlank()
                && event.getEventId().equals(currentState.getEventId());
    }

    static String shadowDisposition(ModelUpdateState currentState, ModelUpdateEvent event) {
        if (currentState == null || currentState.getAggregateRevision() == 0) {
            return "new";
        }
        if (event.getAggregateRevision() < currentState.getAggregateRevision()) {
            return "stale";
        }
        if (event.getAggregateRevision() == currentState.getAggregateRevision()) {
            return safe(event.getPackageSha256()).equals(currentState.getPackageSha256())
                    ? "duplicate" : "stale";
        }
        return "new";
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
        private String stage;
        private String packageId;
        private String packageSha256;
        private long aggregateRevision;
		private String artifactSha256;
		private float warmupScore;
		private String rollbackId;
		private String rollbackPhase;
		private String tenantId;
		private String modelId;
		private String modelName;

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
            this.stage = "PENDING";
            this.packageId = "";
            this.packageSha256 = "";
			this.artifactSha256 = "";
			this.warmupScore = 0.0f;
			this.rollbackId = "";
			this.rollbackPhase = "";
			this.tenantId = "";
			this.modelId = "";
			this.modelName = "";
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

        public String getStage() { return stage; }
        public void setStage(String stage) { this.stage = stage; }

        public String getPackageId() { return packageId; }
        public void setPackageId(String packageId) { this.packageId = packageId; }

        public String getPackageSha256() { return packageSha256; }
        public void setPackageSha256(String packageSha256) { this.packageSha256 = packageSha256; }

        public long getAggregateRevision() { return aggregateRevision; }
        public void setAggregateRevision(long aggregateRevision) { this.aggregateRevision = aggregateRevision; }

		public String getArtifactSha256() { return artifactSha256; }
		public void setArtifactSha256(String value) { this.artifactSha256 = value; }

		public float getWarmupScore() { return warmupScore; }
		public void setWarmupScore(float value) { this.warmupScore = value; }

		public String getRollbackId() { return rollbackId; }
		public void setRollbackId(String value) { this.rollbackId = value == null ? "" : value; }

		public String getRollbackPhase() { return rollbackPhase; }
		public void setRollbackPhase(String value) { this.rollbackPhase = value == null ? "" : value; }

		public String getTenantId() { return tenantId; }
		public void setTenantId(String value) { this.tenantId = value == null ? "" : value; }

		public String getModelId() { return modelId; }
		public void setModelId(String value) { this.modelId = value == null ? "" : value; }

		public String getModelName() { return modelName; }
		public void setModelName(String value) { this.modelName = value == null ? "" : value; }

        @Override
        public String toString() {
            return String.format("ModelUpdateState{type=%s, version=%s, action=%s, stage=%s, "
                            + "revision=%d, pending=%s}",
                    modelType, version, action, stage, aggregateRevision, pending);
        }
    }
}
