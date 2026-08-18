package com.traffic.flink.behavior.receipt;

import com.traffic.flink.behavior.detector.DetectionAggregate;
import com.traffic.flink.behavior.detector.EncryptedTrafficRecognizer;
import com.traffic.flink.behavior.detector.SelectedThreatDetector;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * RunBehaviorReceiptProcessor —— run-scoped S3/S4 真实聚合与回执:
 * 按 run 聚合 envelope 流,对每条流执行 识别→检测→聚合 纯函数管线,窗口闭合后
 * 产出 ENCRYPTED_RECOGNIZER / RULE_DETECTION / BEHAVIOR_DETECTION /
 * DETECTION_AGGREGATE 四条 StageReceipt。
 *
 * 诚实边界:envelope 只携带流级聚合特征(无 payload/TLS 指纹),识别器对缺失
 * is_encrypted 的输入按契约产出 INCOMPATIBLE;检测器基于真实流级特征
 * (pps/bps/字节包比/flags)的阈值规则,产出真实 NEGATIVE/POSITIVE。
 */
public final class RunBehaviorReceiptProcessor
        extends KeyedProcessFunction<String, RunBehaviorEnvelopeRecord, String> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(RunBehaviorReceiptProcessor.class);

    /** 冻结检测器 exact-set(required=RULE+BEHAVIOR;与计划目录默认一致)。 */
    static final List<String> REQUIRED_DETECTORS = List.of("rule-flood-v1", "behavior-baseline-v1");
    /** 冻结识别模型引用(目录默认)。 */
    static final String RECOGNITION_MODEL_ID = "recognition-default-v1";
    static final String RECOGNITION_MODEL_SHA256 =
            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855";

    private final long graceMs;

    private transient ValueState<BehaviorAgg> aggState;
    private transient ValueState<Boolean> firedState;

    public RunBehaviorReceiptProcessor(long graceMs) {
        if (graceMs < 0) {
            throw new IllegalArgumentException("graceMs must be >= 0");
        }
        this.graceMs = graceMs;
    }

    /** 每 run 行为聚合。 */
    /** 每输入×required detector typed disposition 结果侧输出(落库 analysis_detections)。 */
    public static final org.apache.flink.util.OutputTag<RunDetectionResultRow> RESULTS_TAG =
            new org.apache.flink.util.OutputTag<RunDetectionResultRow>("run-detection-results") {};

    public static final class BehaviorAgg implements java.io.Serializable {
        private static final long serialVersionUID = 1L;
        public String tenantId = "";
        public String runId = "";
        public String executionSpecSha256 = "";
        public String fencingToken = "";
        public long windowEndMs = 0;
        public long flows = 0;
        // 识别
        public long recognized = 0;
        public long notEncrypted = 0;
        public long unknown = 0;
        public long incompatible = 0;
        // 规则检测器
        public long rulePositive = 0;
        public long ruleNegative = 0;
        public long ruleInconclusive = 0;
        public long ruleError = 0;
        public long ruleIncompatible = 0;
        public long ruleNotRun = 0;
        // 行为检测器
        public long behaviorPositive = 0;
        public long behaviorNegative = 0;
        public long behaviorInconclusive = 0;
        public long behaviorError = 0;
        public long behaviorIncompatible = 0;
        public long behaviorNotRun = 0;
        // 聚合(每输入一条 typed disposition)
        public long aggPositive = 0;
        public long aggNegative = 0;
        public long aggInconclusive = 0;
        public long aggIncompatible = 0;
        public long aggError = 0;
        public long aggNotRun = 0;
        public final Set<String> communities = new HashSet<>();
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        aggState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-behavior-agg", BehaviorAgg.class));
        firedState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-behavior-fired", Boolean.class));
    }

    @Override
    public void processElement(RunBehaviorEnvelopeRecord env, Context ctx, Collector<String> out) throws Exception {
        if (env == null || env.runId() == null || env.runId().isEmpty()) {
            return;
        }
        BehaviorAgg agg = aggState.value();
        if (agg == null) {
            agg = new BehaviorAgg();
            agg.tenantId = env.tenantId();
            agg.runId = env.runId();
            agg.executionSpecSha256 = env.executionSpecSha256();
            agg.fencingToken = env.fencingToken();
            agg.windowEndMs = env.windowEndMs();
        } else if (env.fencingToken() != null && !env.fencingToken().isEmpty()) {
            agg.fencingToken = env.fencingToken();
        }
        RunBehaviorEnvelopeRecord.Event e = env.event();
        String inputId = e.flowId() != null && !e.flowId().isEmpty() ? e.flowId() : e.communityId();

        // 识别(指纹特征来自探针 DPI 的 feature_observation;缺失必要特征→契约内
        // INCOMPATIBLE,不做任何推断)。
        EncryptedTrafficRecognizer.RecognitionOutcome recognition =
                new EncryptedTrafficRecognizer().recognize(inputId,
                        new EncryptedTrafficRecognizer.RecognitionSelection(
                                RECOGNITION_MODEL_ID, RECOGNITION_MODEL_SHA256),
                        fingerprintFeatures(e));
        String recognitionState = recognition.state().name();
        switch (recognitionState) {
            case "RECOGNIZED": agg.recognized++; break;
            case "NOT_ENCRYPTED": agg.notEncrypted++; break;
            case "UNKNOWN": agg.unknown++; break;
            case "INCOMPATIBLE": agg.incompatible++; break;
            default: agg.incompatible++; break;
        }

        // 检测(真实阈值规则:RDT-001 洪泛形态 / BDT-001 持续高带宽)
        List<SelectedThreatDetector.DetectionOutcome> outcomes =
                SelectedThreatDetector.detect(inputId,
                        new SelectedThreatDetector.ThreatDetectionSelection(REQUIRED_DETECTORS, Map.of(
                                "rule-flood-v1", 0.5, "behavior-baseline-v1", 0.5)),
                        featureEnvelope(e), recognitionState, new RuleDetectorExecutor());
        for (SelectedThreatDetector.DetectionOutcome o : outcomes) {
            if ("rule-flood-v1".equals(o.detectorId())) {
                tally(o, agg, true);
            } else {
                tally(o, agg, false);
            }
        }

        // 每输入×required detector 恰一条 typed disposition(§7.4):缺失检测器记 NOT_RUN;
        // 侧输出结果行落库 analysis_detections(调度域 per-input 结果权威存储)。
        java.util.Map<String, SelectedThreatDetector.DetectionOutcome> outcomeById = new java.util.HashMap<>();
        for (SelectedThreatDetector.DetectionOutcome o : outcomes) {
            outcomeById.put(o.detectorId(), o);
        }
        long notRunForInput = 0;
        for (String detectorId : REQUIRED_DETECTORS) {
            SelectedThreatDetector.DetectionOutcome o = outcomeById.get(detectorId);
            String disposition = o != null ? o.disposition().name() : "NOT_RUN";
            if (o == null) {
                notRunForInput++;
            }
            ctx.output(RESULTS_TAG, new RunDetectionResultRow(
                    agg.tenantId, agg.runId, agg.executionSpecSha256, inputId, detectorId,
                    disposition,
                    o != null ? o.score() : 0.0,
                    o != null ? String.join(",", o.labels()) : "",
                    o != null ? String.join(",", o.evidenceRefs()) : "",
                    e.tsEnd()));
        }
        if (notRunForInput > 0) {
            agg.aggNotRun++;
        }

        DetectionAggregate.AggregatedDetection combined =
                DetectionAggregate.aggregate(inputId, REQUIRED_DETECTORS.size(), outcomes);
        if (combined.hasTrustedPositive()) {
            agg.aggPositive++;
        } else if (combined.allExplicitNegative()) {
            agg.aggNegative++;
        } else if (combined.incompatible() > 0) {
            agg.aggIncompatible++;
        } else if (combined.error() > 0) {
            agg.aggError++;
        } else {
            agg.aggInconclusive++;
        }
        if (e.communityId() != null && !e.communityId().isEmpty()) {
            agg.communities.add(e.communityId());
        }
        agg.flows++;
        aggState.update(agg);

        long nowMs = System.currentTimeMillis();
        long wallClose = agg.windowEndMs > 0 ? agg.windowEndMs + graceMs : nowMs + graceMs;
        long ptFireAt = Math.max(wallClose, nowMs + graceMs);
        ctx.timerService().registerProcessingTimeTimer(ptFireAt);
    }

    private static void tally(SelectedThreatDetector.DetectionOutcome o, BehaviorAgg agg, boolean rule) {
        switch (o.disposition()) {
            case POSITIVE:
                if (rule) { agg.rulePositive++; } else { agg.behaviorPositive++; }
                break;
            case NEGATIVE:
                if (rule) { agg.ruleNegative++; } else { agg.behaviorNegative++; }
                break;
            case INCONCLUSIVE:
                if (rule) { agg.ruleInconclusive++; } else { agg.behaviorInconclusive++; }
                break;
            case INCOMPATIBLE:
                if (rule) { agg.ruleIncompatible++; } else { agg.behaviorIncompatible++; }
                break;
            case ERROR:
                if (rule) { agg.ruleError++; } else { agg.behaviorError++; }
                break;
            case NOT_RUN:
            default:
                if (rule) { agg.ruleNotRun++; } else { agg.behaviorNotRun++; }
                break;
        }
    }

    /** 指纹特征(识别器输入):transport_security 1=TLS/2=QUIC → 加密;缺失则
     *  不推断(is_encrypted 缺席 → 契约内 INCOMPATIBLE)。 */
    private static Map<String, Object> fingerprintFeatures(RunBehaviorEnvelopeRecord.Event e) {
        RunBehaviorEnvelopeRecord.FeatureObservation fo = e.featureObservation();
        java.util.HashMap<String, Object> features = new java.util.HashMap<>();
        if (fo == null) {
            return features;
        }
        long sec = fo.transportSecurity();
        if (sec == 1 || sec == 2) {
            features.put("is_encrypted", 1);
        }
        if (fo.tlsVersion() != null && !fo.tlsVersion().isEmpty()) {
            features.put("tls_version", fo.tlsVersion());
        }
        if (fo.ja3() != null && !fo.ja3().isEmpty()) {
            features.put("ja3", fo.ja3());
        }
        if (fo.sni() != null && !fo.sni().isEmpty()) {
            features.put("sni", fo.sni());
        }
        return features;
    }

    /** 流级特征 envelope(检测器输入;全部来自 replay 真实聚合)。 */
    private static Map<String, Object> featureEnvelope(RunBehaviorEnvelopeRecord.Event e) {
        double avgBytesPerPacket = e.totalPackets() > 0 ? (double) e.totalBytes() / e.totalPackets() : 0;
        double byteRatio = e.totalBytes() > 0 ? (double) e.bytesFwd() / e.totalBytes() : 0;
        java.util.HashMap<String, Object> features = new java.util.HashMap<>();
        features.put("pps", e.pps());
        features.put("bps", e.bps());
        features.put("duration_ms", (double) e.durationMs());
        features.put("total_packets", (double) e.totalPackets());
        features.put("total_bytes", (double) e.totalBytes());
        features.put("avg_bytes_per_packet", avgBytesPerPacket);
        features.put("byte_ratio_fwd", byteRatio);
        features.put("tcp_flags_fwd", (double) e.tcpFlagsFwd());
        features.put("tcp_flags_bwd", (double) e.tcpFlagsBwd());
        features.put("tos", (double) e.tos());
        features.put("subflow_count", (double) e.subflowCount());
        return features;
    }

    /** 阈值规则执行器(RDT-001/BDT-001,确定性)。 */
    static final class RuleDetectorExecutor implements SelectedThreatDetector.DetectorExecutor {
        @Override
        public Map<String, Object> execute(String detectorId, String inputObjectId,
                                           Map<String, Object> features) {
            double pps = num(features.get("pps"));
            double bps = num(features.get("bps"));
            double avgBytesPerPacket = num(features.get("avg_bytes_per_packet"));
            double durationMs = num(features.get("duration_ms"));
            switch (detectorId) {
                case "rule-flood-v1":
                    // 洪泛形态:高包速率且小包(典型 DoS/扫描形态)
                    boolean flood = pps > 200 && avgBytesPerPacket > 0 && avgBytesPerPacket < 120;
                    return Map.of("positive", flood, "score", Math.min(0.99, pps / 2000.0),
                            "labels", flood ? List.of("flood_morphology") : List.of(),
                            "evidence", List.of("pps=" + pps, "avg_bytes_per_packet=" + avgBytesPerPacket));
                case "behavior-baseline-v1":
                    // 持续高带宽基线(超出基线即阳性候选)
                    boolean heavy = bps > 2_000_000 && durationMs > 200;
                    return Map.of("positive", heavy, "score", Math.min(0.99, bps / 20_000_000.0),
                            "labels", heavy ? List.of("sustained_high_bandwidth") : List.of(),
                            "evidence", List.of("bps=" + bps, "duration_ms=" + durationMs));
                default:
                    return null;
            }
        }

        private static double num(Object v) {
            return v instanceof Number ? ((Number) v).doubleValue() : 0.0;
        }
    }

    @Override
    public void onTimer(long timestamp, OnTimerContext ctx, Collector<String> out) throws Exception {
        Boolean fired = firedState.value();
        if (Boolean.TRUE.equals(fired)) {
            return;
        }
        BehaviorAgg agg = aggState.value();
        if (agg == null || agg.flows == 0) {
            return;
        }
        firedState.update(true);
        out.collect(recognitionReceipt(agg));
        out.collect(detectorReceipt(agg, "RULE_DETECTION", "rule-flood-v1",
                agg.rulePositive, agg.ruleNegative, agg.ruleInconclusive, agg.ruleIncompatible, agg.ruleError, agg.ruleNotRun));
        out.collect(detectorReceipt(agg, "BEHAVIOR_DETECTION", "behavior-baseline-v1",
                agg.behaviorPositive, agg.behaviorNegative, agg.behaviorInconclusive, agg.behaviorIncompatible, agg.behaviorError, agg.behaviorNotRun));
        out.collect(aggregateReceipt(agg));
        LOG.info("S3/S4 receipts emitted run={} flows={} positive={} negative={}",
                agg.runId, agg.flows, agg.aggPositive, agg.aggNegative);
    }

    private String recognitionReceipt(BehaviorAgg agg) {
        String eventId = uuid("flink-behavior-receipt", agg, "ENCRYPTED_RECOGNIZER");
        return "{\"schema_version\":\"1\",\"tenant_id\":\"" + esc(agg.tenantId)
                + "\",\"run_id\":\"" + esc(agg.runId)
                + "\",\"event_id\":\"" + eventId
                + "\",\"execution_node_id\":\"ENCRYPTED_RECOGNIZER\",\"attempt\":1"
                + ",\"fencing_token\":\"" + esc(agg.fencingToken)
                + "\",\"provider\":\"flink-behavior-receipt\""
                + ",\"input_count\":" + agg.flows
                + ",\"output_count\":" + agg.flows
                + ",\"error_count\":0,\"reject_count\":0,\"watermark_ms\":0"
                + ",\"fence\":{\"kind\":\"recognition_fence\",\"flows\":" + agg.flows
                + ",\"recognized\":" + agg.recognized
                + ",\"not_encrypted\":" + agg.notEncrypted
                + ",\"unknown\":" + agg.unknown
                + ",\"incompatible\":" + agg.incompatible + "}"
                + ",\"payload_hash\":\"" + esc(agg.executionSpecSha256) + "\"}";
    }

    private String detectorReceipt(BehaviorAgg agg, String node, String detectorId,
                                   long positive, long negative, long inconclusive,
                                   long incompatible, long error, long notRun) {
        String eventId = uuid("flink-behavior-receipt", agg, node);
        return "{\"schema_version\":\"1\",\"tenant_id\":\"" + esc(agg.tenantId)
                + "\",\"run_id\":\"" + esc(agg.runId)
                + "\",\"event_id\":\"" + eventId
                + "\",\"execution_node_id\":\"" + node + "\",\"attempt\":1"
                + ",\"fencing_token\":\"" + esc(agg.fencingToken)
                + "\",\"provider\":\"flink-behavior-receipt\""
                + ",\"input_count\":" + agg.flows
                + ",\"output_count\":" + (positive + negative)
                + ",\"error_count\":0,\"reject_count\":0,\"watermark_ms\":0"
                + ",\"fence\":{\"kind\":\"detector_fence\",\"detector\":\"" + detectorId + "\""
                + ",\"total\":" + agg.flows
                + ",\"positive\":" + positive
                + ",\"negative\":" + negative
                + ",\"inconclusive\":" + inconclusive
                + ",\"incompatible\":" + incompatible
                + ",\"error\":" + error
                + ",\"not_run\":" + notRun + "}"
                + ",\"payload_hash\":\"" + esc(agg.executionSpecSha256) + "\"}";
    }

    private String aggregateReceipt(BehaviorAgg agg) {
        String eventId = uuid("flink-behavior-receipt", agg, "DETECTION_AGGREGATE");
        return "{\"schema_version\":\"1\",\"tenant_id\":\"" + esc(agg.tenantId)
                + "\",\"run_id\":\"" + esc(agg.runId)
                + "\",\"event_id\":\"" + eventId
                + "\",\"execution_node_id\":\"DETECTION_AGGREGATE\",\"attempt\":1"
                + ",\"fencing_token\":\"" + esc(agg.fencingToken)
                + "\",\"provider\":\"flink-behavior-receipt\""
                + ",\"input_count\":" + agg.flows
                + ",\"output_count\":" + agg.flows
                + ",\"error_count\":0,\"reject_count\":0,\"watermark_ms\":0"
                + ",\"fence\":{\"kind\":\"detection_fence\",\"total\":" + agg.flows
                + ",\"positive\":" + agg.aggPositive
                + ",\"negative\":" + agg.aggNegative
                + ",\"inconclusive\":" + agg.aggInconclusive
                + ",\"incompatible\":" + agg.aggIncompatible
                + ",\"error\":" + agg.aggError
                + ",\"not_run\":" + agg.aggNotRun
                + ",\"detectors\":[\"rule-flood-v1\",\"behavior-baseline-v1\"]}"
                + ",\"payload_hash\":\"" + esc(agg.executionSpecSha256) + "\"}";
    }

    private static String uuid(String prefix, BehaviorAgg agg, String node) {
        return java.util.UUID.nameUUIDFromBytes(
                (prefix + ":" + agg.tenantId + ":" + agg.runId + ":" + node + ":1")
                        .getBytes(StandardCharsets.UTF_8)).toString();
    }

    private static String esc(String v) {
        if (v == null) {
            return "";
        }
        return v.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
