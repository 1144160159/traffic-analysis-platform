package com.traffic.flink.behavior.source;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.flink.behavior.model.ModelConsumerProfile;
import org.apache.flink.streaming.api.functions.source.RichParallelSourceFunction;

/** Emits one stable readiness receipt per model-update consumer subtask. */
public final class ModelConsumerReadinessSource
        extends RichParallelSourceFunction<ModelUpdateAppliedAck> {
    private static final long serialVersionUID = 1L;

    private final String deploymentId;
    private final String profileSha256;
    private final String runtimeContract;
    private final String runtimeVersion;
    private final int featureSchemaVersion;
    private final int graphSchemaVersion;
    private volatile boolean running = true;

    public ModelConsumerReadinessSource(BehaviorJobConfig config) {
        ModelConsumerProfile.verifyConfiguredProfile(config);
        this.deploymentId = config.getModelConsumerDeploymentId();
        this.profileSha256 = config.getModelConsumerProfileSha256();
        this.runtimeContract = config.getModelRuntimeContract();
        this.runtimeVersion = config.getModelRuntimeVersion();
        this.featureSchemaVersion = config.getModelFeatureSchemaVersion();
        this.graphSchemaVersion = config.getModelGraphSchemaVersion();
    }

    @Override
    public void run(SourceContext<ModelUpdateAppliedAck> context) {
        if (!running) {
            return;
        }
        int subtask = getRuntimeContext().getIndexOfThisSubtask();
        int parallelism = getRuntimeContext().getNumberOfParallelSubtasks();
        ModelUpdateAppliedAck receipt = ModelUpdateAppliedAck.consumerReady(
                deploymentId, profileSha256, runtimeContract, runtimeVersion,
                featureSchemaVersion, graphSchemaVersion, subtask, parallelism);
        synchronized (context.getCheckpointLock()) {
            if (running) {
                context.collect(receipt);
            }
        }
    }

    @Override
    public void cancel() {
        running = false;
    }
}
