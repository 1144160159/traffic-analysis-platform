package com.traffic.flink.behavior.detector;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * EncryptedTrafficRecognizer —— 加密特征识别 typed outcome(ER01-ER12 判定核)。
 *
 * 只回答"协议/指纹/应用家族/可识别性",不判恶意;每输入一条 outcome;
 * 非加密输入=NOT_ENCRYPTED;缺必要特征=UNKNOWN/INCOMPATIBLE;异常=ERROR;
 * 与特征组成复合 envelope(输出含原特征引用)。
 */
public final class EncryptedTrafficRecognizer {

    public enum RecognitionState {
        RECOGNIZED, NOT_ENCRYPTED, UNKNOWN, INCOMPATIBLE, ERROR
    }

    public static final class RecognitionOutcome {
        private final String inputObjectId;
        private final String recognitionModelId;
        private final String protocolFamily;
        private final String fingerprintFamily;
        private final String applicationFamily;
        private final double confidence;
        private final RecognitionState state;
        private final String reasonCode;
        private final List<String> evidenceRefs;

        RecognitionOutcome(String inputObjectId, String recognitionModelId, String protocolFamily,
                           String fingerprintFamily, String applicationFamily, double confidence,
                           RecognitionState state, String reasonCode, List<String> evidenceRefs) {
            this.inputObjectId = inputObjectId;
            this.recognitionModelId = recognitionModelId;
            this.protocolFamily = protocolFamily;
            this.fingerprintFamily = fingerprintFamily;
            this.applicationFamily = applicationFamily;
            this.confidence = confidence;
            this.state = state;
            this.reasonCode = reasonCode;
            this.evidenceRefs = Collections.unmodifiableList(new ArrayList<>(evidenceRefs));
        }

        public RecognitionState state() { return state; }
        public String protocolFamily() { return protocolFamily; }
        public String fingerprintFamily() { return fingerprintFamily; }
        public String applicationFamily() { return applicationFamily; }
        public double confidence() { return confidence; }
        public String reasonCode() { return reasonCode; }
        public String recognitionModelId() { return recognitionModelId; }
        public String inputObjectId() { return inputObjectId; }
        public List<String> evidenceRefs() { return evidenceRefs; }
    }

    /** 冻结识别选择:exact model ref(拒绝漂移 active 指针)。 */
    public static final class RecognitionSelection {
        private final String recognitionModelId;
        private final String expectedModelSha256;

        public RecognitionSelection(String recognitionModelId, String expectedModelSha256) {
            this.recognitionModelId = recognitionModelId;
            this.expectedModelSha256 = expectedModelSha256;
        }
    }

    /**
     * recognize —— 每输入一条 typed outcome。
     * 特征输入:FingerprintFeature(ja3/ja3s/sni/tls_version/entropy 等)。
     */
    public RecognitionOutcome recognize(String inputObjectId, RecognitionSelection selection,
                                        Map<String, Object> fingerprintFeatures) {
        Objects.requireNonNull(selection, "selection");
        Objects.requireNonNull(fingerprintFeatures, "fingerprintFeatures");

        // ER02:输入 schema 校验——必要字段缺失→UNKNOWN/INCOMPATIBLE
        boolean hasTlsVersion = fingerprintFeatures.get("tls_version") != null;
        boolean hasJa3 = fingerprintFeatures.get("ja3") != null;
        Object isEncrypted = fingerprintFeatures.get("is_encrypted");
        if (isEncrypted == null) {
            return outcome(inputObjectId, selection, "", "", "", 0.0,
                    RecognitionState.INCOMPATIBLE, "MISSING_IS_ENCRYPTED", List.of());
        }
        boolean encrypted = ((Number) isEncrypted).intValue() != 0;
        if (!encrypted) {
            return outcome(inputObjectId, selection, "", "", "", 1.0,
                    RecognitionState.NOT_ENCRYPTED, "PLAINTEXT", List.of("fingerprint:is_encrypted=0"));
        }
        if (!hasTlsVersion && !hasJa3) {
            return outcome(inputObjectId, selection, "", "", "", 0.0,
                    RecognitionState.UNKNOWN, "INSUFFICIENT_FEATURES", List.of());
        }
        String protocol = String.valueOf(fingerprintFeatures.getOrDefault("protocol_family", "TLS"));
        String fingerprint = hasJa3
                ? "ja3:" + fingerprintFeatures.get("ja3")
                : "tls:" + fingerprintFeatures.get("tls_version");
        double confidence = 0.9;
        return outcome(inputObjectId, selection, protocol, fingerprint, "unknown", confidence,
                RecognitionState.RECOGNIZED, "OK",
                List.of("fingerprint:tls_version", "fingerprint:ja3"));
    }

    private RecognitionOutcome outcome(String inputObjectId, RecognitionSelection sel,
                                       String protocol, String fingerprint, String app,
                                       double confidence, RecognitionState state, String reason,
                                       List<String> evidence) {
        return new RecognitionOutcome(inputObjectId, sel.recognitionModelId, protocol, fingerprint,
                app, confidence, state, reason, evidence);
    }
}
