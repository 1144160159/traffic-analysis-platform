package com.traffic.flink.alert.router;

import com.fasterxml.jackson.databind.ObjectMapper;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * RunRouterProcessFunction —— RunScopeRouter 的 Flink 包装:
 * 广播流维护 ACTIVE 订阅(analysis.run.events.v1),base 流(flow.events.v1)按
 * 租户+事件时间+窗口+fencing 派生 0..N 个 run envelope(JSON)。
 *
 * 不变量:
 *  - CANCELLED 订阅立即移除(不再派生);
 *  - 解析失败的 flow 送 DLQ 侧输出,不静默丢弃;
 *  - 窗口过期订阅惰性清理(处理该租户事件时)。
 */
public final class RunRouterProcessFunction
        extends BroadcastProcessFunction<RawKafkaRecord, RunSubscriptionRecord, String> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(RunRouterProcessFunction.class);

    public static final OutputTag<String> FLOW_PARSE_DLQ_TAG =
            new OutputTag<String>("run-router-flow-parse-dlq") {};

    public static final MapStateDescriptor<String, RunSubscriptionRecord> SUBSCRIPTION_STATE =
            new MapStateDescriptor<>("run-subscriptions", String.class, RunSubscriptionRecord.class);

    private final String inputTopic;
    private final transient ObjectMapper mapper = new ObjectMapper();
    private final String consumerGroup;

    public RunRouterProcessFunction(String inputTopic, String consumerGroup) {
        if (inputTopic == null || inputTopic.isBlank()) {
            throw new IllegalArgumentException("flow input topic is required");
        }
        this.inputTopic = inputTopic;
        this.consumerGroup = consumerGroup == null ? "" : consumerGroup;
    }

    @Override
    public void processBroadcastElement(
            RunSubscriptionRecord sub,
            Context ctx,
            Collector<String> out) throws Exception {
        if (sub == null || sub.tenantId() == null || sub.runId() == null) {
            LOG.warn("malformed subscription dropped (missing tenant/run identity)");
            return;
        }
        BroadcastState<String, RunSubscriptionRecord> state =
                ctx.getBroadcastState(SUBSCRIPTION_STATE);
        switch (sub.state() == null ? "" : sub.state()) {
            case "ACTIVE":
                // revision 单调:旧 revision 不得覆盖新 revision(防回退)
                RunSubscriptionRecord existing = state.get(sub.stateKey());
                if (existing == null || sub.revision() >= existing.revision()) {
                    state.put(sub.stateKey(), sub);
                    LOG.debug("subscription ACTIVE run={} revision={}", sub.runId(), sub.revision());
                } else {
                    LOG.warn("stale subscription revision rejected run={} rev={} < existing={}",
                            sub.runId(), sub.revision(), existing.revision());
                }
                break;
            case "CANCELLED":
                state.remove(sub.stateKey());
                LOG.debug("subscription CANCELLED run={}", sub.runId());
                break;
            case "PREPARE":
                // 两段订阅:物化时只发 PREPARE 通知;路由器只匹配 ACTIVE,
                // PREPARE 不进入广播状态(匹配以 ACTIVE 为准,防止过早派生)。
                LOG.debug("subscription PREPARE run={} (not routed; ACTIVE pending)", sub.runId());
                break;
            default:
                LOG.warn("subscription state {} not routed (fail-closed)", sub.state());
                break;
        }
    }

    @Override
    public void processElement(
            RawKafkaRecord record,
            ReadOnlyContext ctx,
            Collector<String> out) throws Exception {
        if (record == null || record.getValue() == null) {
            return;
        }
        if (!inputTopic.equals(record.getTopic())) {
            LOG.warn("record from unexpected topic {} ignored (expected {})", record.getTopic(), inputTopic);
            return;
        }
        FlowEvent flow;
        try {
            flow = FlowEvent.parseFrom(record.getValue());
        } catch (Exception e) {
            // 捕获 Exception(而非具体 protobuf 异常):uber jar 对 protobuf 做了
            // shaded 重定位,具体异常类引用会在运行时 NoClassDefFound。
            LOG.warn("flow parse failed; sent to DLQ (group={})", consumerGroup);
            ctx.output(FLOW_PARSE_DLQ_TAG, dlqJson(record, e));
            return;
        }
        if (flow.getHeader() == null || flow.getHeader().getTenantId().isEmpty()) {
            return; // 无租户上下文,不派生(共享分支事实流)
        }
        String tenantId = flow.getHeader().getTenantId();
        long tsMs = flow.getTsEnd() > 0 ? flow.getTsEnd() : flow.getHeader().getEventTs();

        Map<String, RunSubscriptionRecord> snapshot = new LinkedHashMap<>();
        for (Map.Entry<String, RunSubscriptionRecord> e :
                ctx.getBroadcastState(SUBSCRIPTION_STATE).immutableEntries()) {
            snapshot.put(e.getKey(), e.getValue());
        }
        List<RunScopeRouter.Subscription> subs = new ArrayList<>();
        Map<String, RunSubscriptionRecord> subsByRun = new LinkedHashMap<>();
        for (Map.Entry<String, RunSubscriptionRecord> e : snapshot.entrySet()) {
            RunSubscriptionRecord s = e.getValue();
            if (!s.tenantId().equals(tenantId)) {
                continue;
            }
            subsByRun.put(s.runId(), s);
            subs.add(new RunScopeRouter.Subscription(
                    s.tenantId(), s.runId(), /* taskId 未随订阅广播 */ "",
                    s.executionSpecSha256(), s.revision(), "ACTIVE",
                    s.windowStartMs(), s.windowEndMs(),
                    /* fencingToken 订阅记录 */ s.fence(),
                    /* activeFencingToken 权威 */ s.fence()));
        }
        if (subs.isEmpty()) {
            return;
        }
        String communityId = flow.getCommunityId() == null ? "" : flow.getCommunityId();
        List<RunScopeRouter.Envelope> envelopes =
                RunScopeRouter.route(subs, tenantId, tsMs, communityId, 0);
        for (RunScopeRouter.Envelope env : envelopes) {
            long windowEnd = 0;
            for (RunSubscriptionRecord r : subsByRun.values()) {
                if (r.runId().equals(env.runId())) {
                    windowEnd = r.windowEndMs();
                    break;
                }
            }
            out.collect(envelopeJson(env, flow, windowEnd));
        }
    }

    private String envelopeJson(RunScopeRouter.Envelope env, FlowEvent flow, long windowEndMs) {
        StringBuilder sb = new StringBuilder(512);
        sb.append("{\"schema_version\":\"1\",\"tenant_id\":\"")
          .append(escape(env.tenantId())).append("\",\"task_id\":\"")
          .append(escape(env.taskId())).append("\",\"run_id\":\"")
          .append(escape(env.runId())).append("\",\"execution_spec_sha256\":\"")
          .append(escape(env.executionSpecSha256())).append("\",\"window_id\":\"")
          .append(escape(env.windowId())).append("\",\"stage_id\":\"")
          .append(escape(env.stageId())).append("\",\"attempt\":")
          .append(env.attempt()).append(",\"fencing_token\":\"")
          .append(escape(env.fencingToken() == null ? "" : env.fencingToken())).append("\",\"window_end_ms\":")
          .append(windowEndMs).append(",\"event\":{")
          .append("\"community_id\":\"").append(escape(flow.getCommunityId())).append("\"")
          .append(",\"flow_id\":\"").append(escape(flow.getFlowId())).append("\"")
          .append(",\"ts_start\":").append(flow.getTsStart())
          .append(",\"ts_end\":").append(flow.getTsEnd())
          .append(",\"packets_fwd\":").append(flow.getPacketsFwd())
          .append(",\"packets_bwd\":").append(flow.getPacketsBwd())
          .append(",\"bytes_fwd\":").append(flow.getBytesFwd())
          .append(",\"bytes_bwd\":").append(flow.getBytesBwd())
          .append(",\"duration_ms\":").append(flow.getDurationMs())
          // 完整流特征(调度中心只路由,特征/识别/检测由对应组件消费)
          .append(",\"direction\":\"").append(escape(flow.getDirection())).append("\"")
          .append(",\"pps\":").append(flow.getPps())
          .append(",\"bps\":").append(flow.getBps())
          .append(",\"tcp_flags_fwd\":").append(flow.getTcpFlagsFwd())
          .append(",\"tcp_flags_bwd\":").append(flow.getTcpFlagsBwd())
          .append(",\"tos\":").append(flow.getTos())
          .append(",\"subflow_count\":").append(flow.getSubflowCount());
        if (flow.hasTuple()) {
            sb.append(",\"tuple\":{\"src_ip\":\"").append(escape(flow.getTuple().getSrcIp()))
              .append("\",\"dst_ip\":\"").append(escape(flow.getTuple().getDstIp()))
              .append("\",\"src_port\":").append(flow.getTuple().getSrcPort())
              .append(",\"dst_port\":").append(flow.getTuple().getDstPort())
              .append(",\"protocol\":").append(flow.getTuple().getProtocol()).append("}");
        }
        if (flow.hasPktlenStats()) {
            sb.append(",\"pktlen\":{\"min\":").append(flow.getPktlenStats().getMin())
              .append(",\"max\":").append(flow.getPktlenStats().getMax())
              .append(",\"mean\":").append(flow.getPktlenStats().getMean())
              .append(",\"std\":").append(flow.getPktlenStats().getStd()).append("}");
        }
        if (flow.hasIatStats()) {
            sb.append(",\"iat\":{\"min\":").append(flow.getIatStats().getMinMs())
              .append(",\"max\":").append(flow.getIatStats().getMaxMs())
              .append(",\"mean\":").append(flow.getIatStats().getMeanMs())
              .append(",\"std\":").append(flow.getIatStats().getStdMs()).append("}");
        }
        if (flow.hasActiveStats()) {
            sb.append(",\"active\":{\"min\":").append(flow.getActiveStats().getMinMs())
              .append(",\"mean\":").append(flow.getActiveStats().getMeanMs())
              .append(",\"max\":").append(flow.getActiveStats().getMaxMs())
              .append(",\"std\":").append(flow.getActiveStats().getStdMs()).append("}");
        }
        if (flow.hasIdleStats()) {
            sb.append(",\"idle\":{\"min\":").append(flow.getIdleStats().getMinMs())
              .append(",\"mean\":").append(flow.getIdleStats().getMeanMs())
              .append(",\"max\":").append(flow.getIdleStats().getMaxMs())
              .append(",\"std\":").append(flow.getIdleStats().getStdMs()).append("}");
        }
        if (flow.hasFeatureObservation()) {
            sb.append(",\"feature_observation\":{")
              .append("\"transport_security\":").append(flow.getFeatureObservation().getTransportSecurityValue())
              .append(",\"tls_version\":\"").append(escape(flow.getFeatureObservation().getTlsVersion())).append("\"")
              .append(",\"ja3\":\"").append(escape(flow.getFeatureObservation().getJa3())).append("\"")
              .append(",\"ja4\":\"").append(escape(flow.getFeatureObservation().getJa4())).append("\"")
              .append(",\"sni\":\"").append(escape(flow.getFeatureObservation().getSni())).append("\"")
              .append(",\"payload_observed_bytes\":").append(flow.getFeatureObservation().getPayloadObservedBytes())
              .append(",\"payload_nibble_counts\":[");
            for (int i = 0; i < flow.getFeatureObservation().getPayloadNibbleCountsCount(); i++) {
                if (i > 0) {
                    sb.append(",");
                }
                sb.append(flow.getFeatureObservation().getPayloadNibbleCounts(i));
            }
            sb.append("]")
              .append(",\"missing_fields\":[");
            for (int i = 0; i < flow.getFeatureObservation().getMissingFieldsCount(); i++) {
                if (i > 0) {
                    sb.append(",");
                }
                sb.append("\"").append(escape(flow.getFeatureObservation().getMissingFields(i))).append("\"");
            }
            sb.append("]}");
        }
        sb.append("}}");
        return sb.toString();
    }

    private String dlqJson(RawKafkaRecord record, Exception e) {
        return "{\"topic\":\"" + escape(record.getTopic()) + "\",\"partition\":" + record.getPartition()
                + ",\"offset\":" + record.getOffset() + ",\"error\":\"" + escape(e.getMessage()) + "\",\"group\":\""
                + escape(consumerGroup) + "\"}";
    }

    private static String escape(String v) {
        if (v == null) {
            return "";
        }
        return v.replace("\\", "\\\\").replace("\"", "\\\"");
    }
}
