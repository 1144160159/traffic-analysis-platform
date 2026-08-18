package com.traffic.flink.feature.receipt;

import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.HashSet;
import java.util.Set;

/**
 * RunReceiptProcessor —— run-scoped 会话化(S2)真实聚合:
 * 按 run 聚合 envelope 流(packets/bytes/会话数),窗口闭合(watermark 越过
 * window_end + 宽限)后产出 SESSIONIZATION StageReceipt(analysis.receipts.v1)。
 *
 * 诚实边界:S3/S4(识别/检测)阶段回执由后续数据面接线产出;本处理器只报 S2。
 */
public final class RunReceiptProcessor extends KeyedProcessFunction<String, RunEnvelopeRecord, String> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(RunReceiptProcessor.class);

    /** run 窗口闭合后的收尾宽限(事件时间,毫秒)。 */
    private final long graceMs;

    private transient ValueState<RunAgg> aggState;
    private transient ValueState<Boolean> firedState;

    public RunReceiptProcessor(long graceMs) {
        if (graceMs < 0) {
            throw new IllegalArgumentException("graceMs must be >= 0");
        }
        this.graceMs = graceMs;
    }

    /** 每 run 聚合。 */
    public static final class RunAgg implements java.io.Serializable {
        private static final long serialVersionUID = 1L;
        public String tenantId = "";
        public String runId = "";
        public String executionSpecSha256 = "";
        public String fencingToken = "";
        public long windowEndMs = 0;
        public long flows = 0;
        public long packets = 0;
        public long bytes = 0;
        public long maxTsEnd = 0;
        public final Set<String> communities = new HashSet<>();
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        aggState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-receipt-agg", RunAgg.class));
        firedState = getRuntimeContext().getState(
                new ValueStateDescriptor<>("run-receipt-fired", Boolean.class));
    }

    @Override
    public void processElement(
            RunEnvelopeRecord env, Context ctx, Collector<String> out) throws Exception {
        if (env == null || env.runId() == null || env.runId().isEmpty()) {
            return;
        }
        RunAgg agg = aggState.value();
        if (agg == null) {
            agg = new RunAgg();
            agg.tenantId = env.tenantId();
            agg.runId = env.runId();
            agg.executionSpecSha256 = env.executionSpecSha256();
            agg.fencingToken = env.fencingToken();
            agg.windowEndMs = env.windowEndMs();
        } else if (env.fencingToken() != null && !env.fencingToken().isEmpty()) {
            // 开闸前(rev-1 订阅)派生的 envelope 无 fence;开闸后(rev-2)才携带
            // 权威 fence。回执必须携带与 S2 attempt 匹配的 fence,聚合取最新
            // 非空值(fail-closed:全程无 fence 则回执 fence 为空,由权威侧
            // stale-fence 隔离,不伪造)。
            agg.fencingToken = env.fencingToken();
        }
        agg.flows++;
        agg.packets += env.event().packetsFwd() + env.event().packetsBwd();
        agg.bytes += env.event().bytesFwd() + env.event().bytesBwd();
        if (env.event().communityId() != null && !env.event().communityId().isEmpty()) {
            agg.communities.add(env.event().communityId());
        }
        if (env.event().tsEnd() > agg.maxTsEnd) {
            agg.maxTsEnd = env.event().tsEnd();
        }
        aggState.update(agg);

        // 事件时间定时器:window_end + 宽限后产出回执(数据 watermark 正常推进时)。
        long fireAt = agg.windowEndMs > 0 ? agg.windowEndMs + graceMs : agg.maxTsEnd + graceMs;
        ctx.timerService().registerEventTimeTimer(fireAt);
        // 处理时间兜底定时器:run 窗口是墙钟区间,回放数据的事件时间可能永远
        // 到不了 window_end(水位停滞),事件时间定时器永远不会触发,导致
        // S2 回执饥饿。墙钟越过 window_end + 宽限即闭合窗口。窗口已过期的
        // run(如 offsets 重置后的追赶)再延迟一个 graceMs,让积压 envelope
        // 先全部聚合,避免首个元素即触发导致 flows=1 的早产回执。
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
        RunAgg agg = aggState.value();
        if (agg == null || agg.flows == 0) {
            return;
        }
        firedState.update(true);
        long sessions = agg.communities.size();
        long watermarkMs = ctx.timerService().currentWatermark();
        if (watermarkMs == Long.MIN_VALUE || watermarkMs <= 0) {
            // 处理时间兜底路径没有可用数据水位;报告已聚合数据的最大事件时间
            // (诚实口径,不伪造水位)。
            watermarkMs = agg.maxTsEnd;
        }
        out.collect(receiptJson(agg, sessions, watermarkMs));
        LOG.info("S2 receipt emitted run={} flows={} sessions={} packets={} bytes={}",
                agg.runId, agg.flows, sessions, agg.packets, agg.bytes);
    }

    private String receiptJson(RunAgg agg, long sessions, long watermarkMs) {
        String eventId = java.util.UUID.nameUUIDFromBytes(
                ("flink-run-receipt:" + agg.tenantId + ":" + agg.runId + ":SESSIONIZATION:1")
                        .getBytes(java.nio.charset.StandardCharsets.UTF_8)).toString();
        return "{\"schema_version\":\"1\",\"tenant_id\":\"" + esc(agg.tenantId)
                + "\",\"run_id\":\"" + esc(agg.runId)
                + "\",\"event_id\":\"" + eventId
                + "\",\"execution_node_id\":\"SESSIONIZATION\",\"attempt\":1"
                + ",\"fencing_token\":\"" + esc(agg.fencingToken)
                + "\",\"provider\":\"flink-run-receipt\""
                + ",\"input_count\":" + agg.flows
                + ",\"output_count\":" + sessions
                + ",\"error_count\":0,\"reject_count\":0"
                + ",\"watermark_ms\":" + watermarkMs
                + ",\"fence\":{\"kind\":\"session_fence\",\"flows\":" + agg.flows
                + ",\"sessions\":" + sessions + ",\"packets\":" + agg.packets
                + ",\"bytes\":" + agg.bytes + "}"
                + ",\"payload_hash\":\"" + esc(agg.executionSpecSha256) + "\"}";
    }

    private static String esc(String v) {
        if (v == null) {
            return "";
        }
        return v.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
