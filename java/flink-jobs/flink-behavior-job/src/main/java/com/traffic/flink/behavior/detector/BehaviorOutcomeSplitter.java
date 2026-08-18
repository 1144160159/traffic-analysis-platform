package com.traffic.flink.behavior.detector;

import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.metrics.Counter;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * 按 typed outcome 分流行为推理结果：
 * - DETECTED → 主流输出
 * - LOW_CONFIDENCE → LOW_CONFIDENCE_TAG（人工复核）
 * - MODEL_ERROR / TIMEOUT → MODEL_ERROR_TAG（接入 DLQ）
 * - NO_DETECTION → 计数（阴性，正常不输出事件）
 *
 * 接线了此前仅定义未使用的三个侧输出标签，推理失败/超时不再静默丢失。
 */
public class BehaviorOutcomeSplitter extends ProcessFunction<BehaviorInferenceOutcome, DetectionBehavior> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BehaviorOutcomeSplitter.class);

    private final OutputTag<DetectionBehavior> lowConfidenceTag;
    private final OutputTag<String> modelErrorTag;
    private final OutputTag<FeatureStat> featureAnomalyTag;
    private transient Counter noDetectionCounter;
    private transient Counter modelErrorCounter;

    public BehaviorOutcomeSplitter(
            OutputTag<DetectionBehavior> lowConfidenceTag,
            OutputTag<String> modelErrorTag,
            OutputTag<FeatureStat> featureAnomalyTag) {
        this.lowConfidenceTag = lowConfidenceTag;
        this.modelErrorTag = modelErrorTag;
        this.featureAnomalyTag = featureAnomalyTag;
    }

    @Override
    public void open(org.apache.flink.configuration.Configuration parameters) throws Exception {
        super.open(parameters);
        noDetectionCounter = getRuntimeContext().getMetricGroup().counter("behavior_no_detection_total");
        modelErrorCounter = getRuntimeContext().getMetricGroup().counter("behavior_errors_routed_total");
    }

    @Override
    public void processElement(BehaviorInferenceOutcome outcome, Context context, Collector<DetectionBehavior> out) {
        if (outcome == null) {
            return;
        }
        switch (outcome.getStatus()) {
            case BehaviorInferenceOutcome.STATUS_DETECTED:
                if (outcome.getDetection() != null) {
                    out.collect(outcome.getDetection());
                }
                break;
            case BehaviorInferenceOutcome.STATUS_LOW_CONFIDENCE:
                if (outcome.getDetection() != null && lowConfidenceTag != null) {
                    context.output(lowConfidenceTag, outcome.getDetection());
                }
                break;
            case BehaviorInferenceOutcome.STATUS_MODEL_ERROR:
            case BehaviorInferenceOutcome.STATUS_TIMEOUT:
                if (modelErrorCounter != null) {
                    modelErrorCounter.inc();
                }
                if (modelErrorTag != null) {
                    context.output(modelErrorTag, outcome.getStatus() + ": "
                            + (outcome.getErrorMessage() == null ? "" : outcome.getErrorMessage()));
                }
                break;
            case BehaviorInferenceOutcome.STATUS_NO_DETECTION:
            default:
                // 阴性：模型正常推理无命中。计数而非静默，不输出事件。
                if (noDetectionCounter != null) {
                    noDetectionCounter.inc();
                }
                break;
        }
    }
}
