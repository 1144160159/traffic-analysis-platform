package com.traffic.flink.behavior.detector;

import com.traffic.proto.traffic.v1.DetectionBehavior;

import java.io.Serializable;

/**
 * 行为推理结果包装：为每一条输入产生明确的 typed outcome。
 *
 * 状态语义：
 * - DETECTED：模型命中且置信度达到输出阈值
 * - LOW_CONFIDENCE：模型命中但置信度低于输出阈值（人工复核）
 * - NO_DETECTION：模型正常推理但无命中（阴性，合法结果）
 * - MODEL_ERROR：推理异常（重试耗尽）
 * - TIMEOUT：异步推理超时
 *
 * 无输出不能解释为阴性：异常与超时必须以 MODEL_ERROR/TIMEOUT 显式上抛，
 * 由下游接入 DLQ，而不是静默 complete(emptyList())。
 */
public class BehaviorInferenceOutcome implements Serializable {

    private static final long serialVersionUID = 1L;

    public static final String STATUS_DETECTED = "DETECTED";
    public static final String STATUS_LOW_CONFIDENCE = "LOW_CONFIDENCE";
    public static final String STATUS_NO_DETECTION = "NO_DETECTION";
    public static final String STATUS_MODEL_ERROR = "MODEL_ERROR";
    public static final String STATUS_TIMEOUT = "TIMEOUT";

    private final String status;
    private final DetectionBehavior detection;
    private final String errorMessage;

    public BehaviorInferenceOutcome(String status, DetectionBehavior detection, String errorMessage) {
        this.status = status;
        this.detection = detection;
        this.errorMessage = errorMessage;
    }

    public String getStatus() {
        return status;
    }

    public DetectionBehavior getDetection() {
        return detection;
    }

    public String getErrorMessage() {
        return errorMessage;
    }

    public boolean isError() {
        return STATUS_MODEL_ERROR.equals(status) || STATUS_TIMEOUT.equals(status);
    }

    public static BehaviorInferenceOutcome detected(DetectionBehavior detection) {
        return new BehaviorInferenceOutcome(STATUS_DETECTED, detection, null);
    }

    public static BehaviorInferenceOutcome lowConfidence(DetectionBehavior detection) {
        return new BehaviorInferenceOutcome(STATUS_LOW_CONFIDENCE, detection, null);
    }

    public static BehaviorInferenceOutcome noDetection() {
        return new BehaviorInferenceOutcome(STATUS_NO_DETECTION, null, null);
    }

    public static BehaviorInferenceOutcome modelError(String message) {
        return new BehaviorInferenceOutcome(STATUS_MODEL_ERROR, null, message);
    }

    public static BehaviorInferenceOutcome timeout(String message) {
        return new BehaviorInferenceOutcome(STATUS_TIMEOUT, null, message);
    }
}
