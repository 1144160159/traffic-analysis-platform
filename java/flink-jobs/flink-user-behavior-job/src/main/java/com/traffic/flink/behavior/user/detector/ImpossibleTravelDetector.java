package com.traffic.flink.behavior.user.detector;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.behavior.user.baseline.BaselineAwareUserEvent;
import com.traffic.flink.behavior.user.baseline.BaselineSnapshot;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * 异地登录检测器 (Impossible Travel)
 *
 * 业务场景：同一用户在 30 分钟内从地理位置相距较远的两个 IP 登录
 * 例如：北京(10.0.0.1) → 纽约(192.168.1.1) 在 10 分钟内 → 物理上不可能
 */
public class ImpossibleTravelDetector extends KeyedProcessFunction<String, BaselineAwareUserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(ImpossibleTravelDetector.class);
    private static final long TRAVEL_WINDOW_MS = 30 * 60_000L; // 30 min
    // 上次登录状态保留 1 天，覆盖最大窗口（24h）并约束 key(tenant|user) 状态增长
    private static final long STATE_TTL_HOURS = 24L;
    private ValueState<UserEvent> lastLoginState;

    @Override public void open(Configuration params) {
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.hours(STATE_TTL_HOURS))
                .setUpdateType(StateTtlConfig.UpdateType.OnReadAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();
        ValueStateDescriptor<UserEvent> descriptor =
                new ValueStateDescriptor<>("last-login", UserEvent.class);
        descriptor.enableTimeToLive(ttlConfig);
        lastLoginState = getRuntimeContext().getState(descriptor);
    }

    @Override
    public void processElement(BaselineAwareUserEvent input, Context ctx, Collector<AnomalyEvent> out) throws Exception {
        UserEvent event = input.getEvent();
        long travelWindowMs = input.longThreshold(
                "impossible_travel_window_seconds", TRAVEL_WINDOW_MS / 1000L, 1L, 86_400L) * 1000L;
        if (!"login".equals(event.getEventType()) && !"LOGIN_SUCCESS".equals(event.getEventType())) return;

        UserEvent last = lastLoginState.value();
        if (last != null) {
            long interval = event.getTimestamp() - last.getTimestamp();
            String ip1 = last.getSourceIp(), ip2 = event.getSourceIp();
            if (interval > 0 && interval < travelWindowMs && ip1 != null && ip2 != null && !ip1.equals(ip2)) {
                // 检查是否跨地域（简化：前两段 IP 不同）
                String[] p1 = ip1.split("\\."), p2 = ip2.split("\\.");
                if (p1.length == 4 && p2.length == 4 && (!p1[0].equals(p2[0]) || !p1[1].equals(p2[1]))) {
                    AnomalyEvent anomaly = new AnomalyEvent(
                        event.getTenantId(), event.getUserId(), event.getUsername(),
                        "IMPOSSIBLE_TRAVEL", "high", 0.85f,
                        String.format("Impossible travel: %s→%s in %ds", ip1, ip2, interval / 1000),
                        event.getTimestamp(), last.getEventId(), event.getEventId());
                    anomaly.sourceIp1 = ip1; anomaly.sourceIp2 = ip2;
                    anomaly.detailJson = baselineEvidence(input.getBaseline(), String.format(
                            "\"from_ip\":\"%s\",\"to_ip\":\"%s\",\"interval_sec\":%d,\"window_seconds\":%d",
                            AnomalyEvent.escapeJson(ip1), AnomalyEvent.escapeJson(ip2),
                            interval / 1000, travelWindowMs / 1000L));
                    out.collect(anomaly);
                    LOG.warn("Impossible travel detected: user={} {}→{} in {}s", event.getUsername(), ip1, ip2, interval / 1000);
                }
            }
        }
        lastLoginState.update(event);
    }

    private static String baselineEvidence(BaselineSnapshot baseline, String detail) {
        if (baseline == null) return "{" + detail + "}";
        return String.format("{%s,\"baseline_version\":%d,\"baseline_snapshot_sha256\":\"%s\"}",
                detail, baseline.getBaselineVersion(), baseline.getSnapshotSha256());
    }
}
