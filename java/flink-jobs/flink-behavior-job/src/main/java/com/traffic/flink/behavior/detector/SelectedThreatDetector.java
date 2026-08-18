package com.traffic.flink.behavior.detector;

import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * SelectedThreatDetector —— 恶意流量检测 typed outcome(DT01-DT12 判定核)。
 *
 * 不变量:只运行计划冻结 detector/rule exact-set;每 input×required detector 一个
 * typed outcome;NEGATIVE 必须是实际执行结果;空集合≠阴性;POSITIVE 带 label/score/evidence。
 */
public final class SelectedThreatDetector {

    public enum Disposition {
        POSITIVE, NEGATIVE, INCONCLUSIVE, INCOMPATIBLE, ERROR, NOT_RUN
    }

    public static final class DetectionOutcome {
        private final String inputObjectId;
        private final String detectorId;
        private final Disposition disposition;
        private final double score;
        private final List<String> labels;
        private final List<String> evidenceRefs;
        private final String reasonCode;

        DetectionOutcome(String inputObjectId, String detectorId, Disposition disposition,
                         double score, List<String> labels, List<String> evidenceRefs, String reasonCode) {
            this.inputObjectId = inputObjectId;
            this.detectorId = detectorId;
            this.disposition = disposition;
            this.score = score;
            this.labels = Collections.unmodifiableList(new ArrayList<>(labels));
            this.evidenceRefs = Collections.unmodifiableList(new ArrayList<>(evidenceRefs));
            this.reasonCode = reasonCode;
        }

        public Disposition disposition() { return disposition; }
        public String detectorId() { return detectorId; }
        public double score() { return score; }
        public List<String> labels() { return labels; }
        public List<String> evidenceRefs() { return evidenceRefs; }
        public String reasonCode() { return reasonCode; }
    }

    /** 冻结检测选择:detector/rule exact-set + 阈值。 */
    public static final class ThreatDetectionSelection {
        private final List<String> detectorIds;
        private final Map<String, Double> thresholds; // detectorId → 阳性阈值

        public ThreatDetectionSelection(List<String> detectorIds, Map<String, Double> thresholds) {
            this.detectorIds = Collections.unmodifiableList(new ArrayList<>(detectorIds));
            this.thresholds = Collections.unmodifiableMap(new HashMap<>(thresholds));
        }

        public List<String> detectorIds() { return detectorIds; }
        public double threshold(String detectorId) { return thresholds.getOrDefault(detectorId, 0.5); }
    }

    /** 检测器执行端口:规则/模型实现(生产为 rule matcher/model inference 适配)。 */
    /** 稳定 sha256 hex(聚合/指纹用)。 */
    public static String sha256(String input) {
        try {
            java.security.MessageDigest md = java.security.MessageDigest.getInstance("SHA-256");
            byte[] d = md.digest(input.getBytes(java.nio.charset.StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : d) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (java.security.NoSuchAlgorithmException e) {
            throw new IllegalStateException("sha-256 unavailable", e);
        }
    }

    public interface DetectorExecutor {
        /** 返回 null=异常(ERROR);否则 {positive: bool, score, labels, evidence}。 */
        Map<String, Object> execute(String detectorId, String inputObjectId,
                                    Map<String, Object> featureEnvelope);
    }

    /**
     * detect —— 每 input×required detector 一个 typed outcome(DT01-DT12)。
     * recognition state INCOMPATIBLE/ERROR 按 completion policy 产生显式结果。
     */
    public static List<DetectionOutcome> detect(String inputObjectId,
                                                ThreatDetectionSelection selection,
                                                Map<String, Object> featureEnvelope,
                                                String recognitionState,
                                                DetectorExecutor executor) {
        Objects.requireNonNull(selection, "selection");
        List<DetectionOutcome> out = new ArrayList<>();
        for (String detectorId : selection.detectorIds()) {
            out.add(detectOne(inputObjectId, detectorId, selection.threshold(detectorId),
                    featureEnvelope, recognitionState, executor));
        }
        return out;
    }

    private static DetectionOutcome detectOne(String inputObjectId, String detectorId, double threshold,
                                              Map<String, Object> featureEnvelope, String recognitionState,
                                              DetectorExecutor executor) {        // DT03:INCOMPATIBLE/ERROR 的识别输入按 completion policy 产生显式结果
        if ("INCOMPATIBLE".equals(recognitionState)) {
            return new DetectionOutcome(inputObjectId, detectorId, Disposition.INCOMPATIBLE,
                    0.0, List.of(), List.of(), "RECOGNITION_INCOMPATIBLE");
        }
        if ("ERROR".equals(recognitionState)) {
            return new DetectionOutcome(inputObjectId, detectorId, Disposition.ERROR,
                    0.0, List.of(), List.of(), "RECOGNITION_ERROR");
        }
        Map<String, Object> result;
        try {
            result = executor.execute(detectorId, inputObjectId, featureEnvelope);
        } catch (RuntimeException e) {
            return new DetectionOutcome(inputObjectId, detectorId, Disposition.ERROR,
                    0.0, List.of(), List.of(), "EXECUTOR_EXCEPTION");
        }
        if (result == null) {
            return new DetectionOutcome(inputObjectId, detectorId, Disposition.ERROR,
                    0.0, List.of(), List.of(), "EXECUTOR_NULL");
        }
        boolean positive = Boolean.TRUE.equals(result.get("positive"));
        double score = result.get("score") instanceof Number ? ((Number) result.get("score")).doubleValue() : 0.0;
        @SuppressWarnings("unchecked")
        List<String> labels = result.get("labels") instanceof List ? (List<String>) result.get("labels") : List.of();
        @SuppressWarnings("unchecked")
        List<String> evidence = result.get("evidence") instanceof List ? (List<String>) result.get("evidence") : List.of();
        // NEGATIVE 必须是实际执行结果(executor 已执行且未命中)
        return new DetectionOutcome(inputObjectId, detectorId,
                positive ? Disposition.POSITIVE : Disposition.NEGATIVE,
                score, labels, evidence, positive ? "MATCHED" : "NO_MATCH");
    }
}

// sha256 稳定哈希(聚合/指纹用)。
