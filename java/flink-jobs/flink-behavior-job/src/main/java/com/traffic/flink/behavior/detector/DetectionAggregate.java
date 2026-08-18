package com.traffic.flink.behavior.detector;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * DetectionAggregate —— fan-out 结果按 run×input 聚合(ATC-AGG-001 判定核)。
 *
 * 不变量:每个 accepted input 有且仅有一个聚合视图;positive/negative/error/
 * inconclusive 计数分别守恒;聚合 hash 确定性;不产业务结论(FindingConclusion 属
 * Finalizer);输入×detector 缺口→ERROR。
 */
public final class DetectionAggregate {

    /** 单输入聚合视图。 */
    public static final class AggregatedDetection {
        private final String inputObjectId;
        private final int detectorCount;
        private final int positive;
        private final int negative;
        private final int inconclusive;
        private final int incompatible;
        private final int error;
        private final int notRun;
        private final String aggregateHash;

        AggregatedDetection(String inputObjectId, int detectorCount, int positive, int negative,
                            int inconclusive, int incompatible, int error, int notRun, String hash) {
            this.inputObjectId = inputObjectId;
            this.detectorCount = detectorCount;
            this.positive = positive;
            this.negative = negative;
            this.inconclusive = inconclusive;
            this.incompatible = incompatible;
            this.error = error;
            this.notRun = notRun;
            this.aggregateHash = hash;
        }

        public int detectorCount() { return detectorCount; }
        public int positive() { return positive; }
        public int negative() { return negative; }
        public int inconclusive() { return inconclusive; }
        public int incompatible() { return incompatible; }
        public int error() { return error; }
        public int notRun() { return notRun; }
        public String aggregateHash() { return aggregateHash; }
        public boolean hasTrustedPositive() { return positive > 0; }
        public boolean allExplicitNegative() {
            return detectorCount > 0 && negative == detectorCount;
        }
    }

    /**
     * aggregate —— 按输入聚合;required detector 数与结果数必须一致(守恒)。
     */
    public static AggregatedDetection aggregate(String inputObjectId, int requiredDetectorCount,
                                                List<SelectedThreatDetector.DetectionOutcome> outcomes) {
        Objects.requireNonNull(outcomes, "outcomes");
        if (requiredDetectorCount <= 0) {
            throw new IllegalArgumentException("requiredDetectorCount must be positive");
        }
        if (outcomes.size() != requiredDetectorCount) {
            throw new IllegalStateException("detector outcome gap: expected " + requiredDetectorCount
                    + " got " + outcomes.size());
        }
        int positive = 0, negative = 0, inconclusive = 0, incompatible = 0, error = 0, notRun = 0;
        for (SelectedThreatDetector.DetectionOutcome o : outcomes) {
            switch (o.disposition()) {
                case POSITIVE: positive++; break;
                case NEGATIVE: negative++; break;
                case INCONCLUSIVE: inconclusive++; break;
                case INCOMPATIBLE: incompatible++; break;
                case ERROR: error++; break;
                case NOT_RUN: notRun++; break;
                default: throw new IllegalStateException("unknown disposition");
            }
        }
        String hash = SelectedThreatDetector.sha256(inputObjectId + "\u001f" + positive + "\u001f"
                + negative + "\u001f" + inconclusive + "\u001f" + incompatible + "\u001f" + error + "\u001f" + notRun);
        return new AggregatedDetection(inputObjectId, requiredDetectorCount, positive, negative,
                inconclusive, incompatible, error, notRun, hash);
    }

    /** 运行级计数(守恒 receipt 输入)。 */
    public static final class RunCounts {
        public long inputs;
        public long positiveInputs;
        public long allNegativeInputs;
        public long errorInputs;
        public String receiptHash;

        RunCounts(long inputs, long positiveInputs, long allNegativeInputs, long errorInputs, String receiptHash) {
            this.inputs = inputs;
            this.positiveInputs = positiveInputs;
            this.allNegativeInputs = allNegativeInputs;
            this.errorInputs = errorInputs;
            this.receiptHash = receiptHash;
        }
    }

    /** 运行级守恒:inputs = positiveInputs + allNegativeInputs + 其余(单输入维度)。 */
    public static RunCounts runCounts(List<AggregatedDetection> perInput) {
        long inputs = perInput.size();
        long pos = 0, neg = 0, err = 0;
        for (AggregatedDetection a : perInput) {
            if (a.hasTrustedPositive()) {
                pos++;
            } else if (a.allExplicitNegative()) {
                neg++;
            } else {
                err++;
            }
        }
        String hash = SelectedThreatDetector.sha256(inputs + "\u001f" + pos + "\u001f" + neg + "\u001f" + err);
        return new RunCounts(inputs, pos, neg, err, hash);
    }

    private static String sha256(String s) { return SelectedThreatDetector.sha256(s); }
}
