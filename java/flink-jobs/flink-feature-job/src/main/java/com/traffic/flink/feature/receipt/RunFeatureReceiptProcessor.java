package com.traffic.flink.feature.receipt;

import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.security.MessageDigest;
import java.nio.charset.StandardCharsets;
import java.util.HashSet;
import java.util.Set;

/**
 * RunFeatureReceiptProcessor —— run-scoped 特征提取(S2 FEATURE_EXTRACTION)真实聚合:
 * 按 run 聚合 envelope 流,从流级特征(pktlen/iat/duration/pps/bps/flags/tos)派生
 * 会话级特征 exact-set,窗口闭合后产出 FEATURE_EXTRACTION StageReceipt。
 *
 * 诚实边界:深层模态(payload/TLS/QUIC 序列)不在 envelope 内,按特征池契约进入
 * modality_missingness_mask,禁止伪造全零;每 run 产出确定性 feature_hash。
 */
public final class RunFeatureReceiptProcessor extends KeyedProcessFunction<String, RunEnvelopeRecord, String> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(RunFeatureReceiptProcessor.class);

    private final long graceMs;

    private transient ValueState<FeatureAgg> aggState;
    private transient ValueState<Boolean> firedState;

    public RunFeatureReceiptProcessor(long graceMs) {
        if (graceMs < 0) {
            throw new IllegalArgumentException("graceMs must be >= 0");
        }
        this.graceMs = graceMs;
    }

    /** 每 run 特征聚合。 */
    public static final class FeatureAgg implements java.io.Serializable {
        private static final long serialVersionUID = 1L;
        public String tenantId = "";
        public String runId = "";
        public String executionSpecSha256 = "";
        public String fencingToken = "";
        public long windowEndMs = 0;
        public long flows = 0;
        public long packetsFwd = 0;
        public long packetsBwd = 0;
        public long bytesFwd = 0;
        public long bytesBwd = 0;
        public long durationSumMs = 0;
        public long durationMaxMs = 0;
        public double ppsSum = 0;
        public double bpsSum = 0;
        public double pktlenMeanSum = 0;   // 加权(packets)累加
        public double iatMeanSum = 0;
        public long tcpSynFwdFlows = 0;
        public long tcpSynBwdFlows = 0;
        public long tcpAckFwdFlows = 0;
        public long subflowTotal = 0;
        public long tosSum = 0;
        public final Set<String> communities = new HashSet<>();
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        aggState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-feature-agg", FeatureAgg.class));
        firedState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-feature-fired", Boolean.class));
    }

    @Override
    public void processElement(RunEnvelopeRecord env, Context ctx, Collector<String> out) throws Exception {
        if (env == null || env.runId() == null || env.runId().isEmpty()) {
            return;
        }
        FeatureAgg agg = aggState.value();
        if (agg == null) {
            agg = new FeatureAgg();
            agg.tenantId = env.tenantId();
            agg.runId = env.runId();
            agg.executionSpecSha256 = env.executionSpecSha256();
            agg.fencingToken = env.fencingToken();
            agg.windowEndMs = env.windowEndMs();
        } else if (env.fencingToken() != null && !env.fencingToken().isEmpty()) {
            // 权威 fence 以开闸后(rev-2)订阅为准(与 SESSIONIZATION 回执同语义)。
            agg.fencingToken = env.fencingToken();
        }
        RunEnvelopeRecord.Event e = env.event();
        agg.flows++;
        agg.packetsFwd += e.packetsFwd();
        agg.packetsBwd += e.packetsBwd();
        agg.bytesFwd += e.bytesFwd();
        agg.bytesBwd += e.bytesBwd();
        agg.durationSumMs += e.durationMs();
        if (e.durationMs() > agg.durationMaxMs) {
            agg.durationMaxMs = e.durationMs();
        }
        agg.ppsSum += e.pps();
        agg.bpsSum += e.bps();
        if (e.pktlen() != null && e.totalPackets() > 0) {
            agg.pktlenMeanSum += e.pktlen().mean() * e.totalPackets();
        }
        if (e.iat() != null) {
            agg.iatMeanSum += e.iat().mean();
        }
        if ((e.tcpFlagsFwd() & 0x02) != 0) { // SYN
            agg.tcpSynFwdFlows++;
        }
        if ((e.tcpFlagsBwd() & 0x02) != 0) {
            agg.tcpSynBwdFlows++;
        }
        if ((e.tcpFlagsFwd() & 0x10) != 0) { // ACK
            agg.tcpAckFwdFlows++;
        }
        agg.subflowTotal += e.subflowCount();
        agg.tosSum += e.tos();
        if (e.communityId() != null && !e.communityId().isEmpty()) {
            agg.communities.add(e.communityId());
        }
        aggState.update(agg);

        long nowMs = System.currentTimeMillis();
        long wallClose = agg.windowEndMs > 0 ? agg.windowEndMs + graceMs : nowMs + graceMs;
        long ptFireAt = Math.max(wallClose, nowMs + graceMs);
        ctx.timerService().registerProcessingTimeTimer(ptFireAt);
    }

    @Override
    public void onTimer(long timestamp, OnTimerContext ctx, Collector<String> out) throws Exception {
        Boolean fired = firedState.value();
        if (Boolean.TRUE.equals(fired)) {
            return;
        }
        FeatureAgg agg = aggState.value();
        if (agg == null || agg.flows == 0) {
            return;
        }
        firedState.update(true);
        long sessions = agg.communities.size();
        out.collect(receiptJson(agg, sessions));
        LOG.info("S2 FEATURE_EXTRACTION receipt emitted run={} flows={} sessions={}",
                agg.runId, agg.flows, sessions);
    }

    private String receiptJson(FeatureAgg agg, long sessions) {
        // 派生特征 exact-set(确定性;深层模态进入 missingness,禁止伪造)。
        long totalPackets = agg.packetsFwd + agg.packetsBwd;
        long totalBytes = agg.bytesFwd + agg.bytesBwd;
        double avgDurationMs = agg.flows > 0 ? (double) agg.durationSumMs / agg.flows : 0;
        double avgPps = agg.flows > 0 ? agg.ppsSum / agg.flows : 0;
        double avgBps = agg.flows > 0 ? agg.bpsSum / agg.flows : 0;
        double pktlenMean = totalPackets > 0 ? agg.pktlenMeanSum / totalPackets : 0;
        double iatMean = agg.flows > 0 ? agg.iatMeanSum / agg.flows : 0;
        double avgTos = agg.flows > 0 ? (double) agg.tosSum / agg.flows : 0;
        String features = "flows=" + agg.flows
                + ";sessions=" + sessions
                + ";packets_fwd=" + agg.packetsFwd
                + ";packets_bwd=" + agg.packetsBwd
                + ";bytes_fwd=" + agg.bytesFwd
                + ";bytes_bwd=" + agg.bytesBwd
                + ";duration_sum_ms=" + agg.durationSumMs
                + ";duration_max_ms=" + agg.durationMaxMs
                + ";avg_duration_ms=" + avgDurationMs
                + ";avg_pps=" + avgPps
                + ";avg_bps=" + avgBps
                + ";pktlen_mean=" + pktlenMean
                + ";iat_mean=" + iatMean
                + ";tcp_syn_fwd_flows=" + agg.tcpSynFwdFlows
                + ";tcp_syn_bwd_flows=" + agg.tcpSynBwdFlows
                + ";tcp_ack_fwd_flows=" + agg.tcpAckFwdFlows
                + ";subflow_total=" + agg.subflowTotal
                + ";avg_tos=" + avgTos;
        String featureHash = sha256(features);
        String missingness = "[\"payload_histogram\",\"payload_b64\",\"sanitized_l4_b64\","
                + "\"tls_record_type_seq\",\"tls_record_version_seq\",\"tls_record_length_seq\","
                + "\"tls_handshake_type_seq\",\"quic_long_header_packet_count\",\"quic_version_seq\","
                + "\"ja3\",\"ja3s\",\"sni\"]";
        String eventId = java.util.UUID.nameUUIDFromBytes(
                ("flink-feature-receipt:" + agg.tenantId + ":" + agg.runId + ":FEATURE_EXTRACTION:1")
                        .getBytes(StandardCharsets.UTF_8)).toString();
        return "{\"schema_version\":\"1\",\"tenant_id\":\"" + esc(agg.tenantId)
                + "\",\"run_id\":\"" + esc(agg.runId)
                + "\",\"event_id\":\"" + eventId
                + "\",\"execution_node_id\":\"FEATURE_EXTRACTION\",\"attempt\":1"
                + ",\"fencing_token\":\"" + esc(agg.fencingToken)
                + "\",\"provider\":\"flink-feature-receipt\""
                + ",\"input_count\":" + agg.flows
                + ",\"output_count\":" + sessions
                + ",\"error_count\":0,\"reject_count\":0"
                + ",\"watermark_ms\":0"
                + ",\"fence\":{\"kind\":\"feature_fence\",\"flows\":" + agg.flows
                + ",\"sessions\":" + sessions
                + ",\"features_computed\":18"
                + ",\"feature_hash\":\"" + featureHash + "\""
                + ",\"modality_missingness_mask\":" + missingness + "}"
                + ",\"payload_hash\":\"" + esc(agg.executionSpecSha256) + "\"}";
    }

    private static String sha256(String input) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] d = md.digest(input.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : d) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (java.security.NoSuchAlgorithmException e) {
            throw new IllegalStateException("sha-256 unavailable", e);
        }
    }

    private static String esc(String v) {
        if (v == null) {
            return "";
        }
        return v.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
