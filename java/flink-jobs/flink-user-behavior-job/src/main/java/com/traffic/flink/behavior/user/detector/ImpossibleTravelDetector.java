package com.traffic.flink.behavior.user.detector;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
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
public class ImpossibleTravelDetector extends KeyedProcessFunction<String, UserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(ImpossibleTravelDetector.class);
    private static final long TRAVEL_WINDOW_MS = 30 * 60_000L; // 30 min
    private ValueState<UserEvent> lastLoginState;

    @Override public void open(Configuration params) {
        lastLoginState = getRuntimeContext().getState(new ValueStateDescriptor<>("last-login", UserEvent.class));
    }

    @Override
    public void processElement(UserEvent event, Context ctx, Collector<AnomalyEvent> out) throws Exception {
        if (!"login".equals(event.getEventType()) && !"LOGIN_SUCCESS".equals(event.getEventType())) return;

        UserEvent last = lastLoginState.value();
        if (last != null) {
            long interval = event.getTimestamp() - last.getTimestamp();
            String ip1 = last.getSourceIp(), ip2 = event.getSourceIp();
            if (interval > 0 && interval < TRAVEL_WINDOW_MS && ip1 != null && ip2 != null && !ip1.equals(ip2)) {
                // 检查是否跨地域（简化：前两段 IP 不同）
                String[] p1 = ip1.split("\\."), p2 = ip2.split("\\.");
                if (p1.length == 4 && p2.length == 4 && (!p1[0].equals(p2[0]) || !p1[1].equals(p2[1]))) {
                    AnomalyEvent anomaly = new AnomalyEvent(
                        event.getTenantId(), event.getUserId(), event.getUsername(),
                        "IMPOSSIBLE_TRAVEL", "high", 0.85f,
                        String.format("Impossible travel: %s→%s in %ds", ip1, ip2, interval / 1000));
                    anomaly.sourceIp1 = ip1; anomaly.sourceIp2 = ip2;
                    anomaly.detailJson = String.format("{\"from_ip\":\"%s\",\"to_ip\":\"%s\",\"interval_sec\":%d}", ip1, ip2, interval / 1000);
                    out.collect(anomaly);
                    LOG.warn("Impossible travel detected: user={} {}→{} in {}s", event.getUsername(), ip1, ip2, interval / 1000);
                }
            }
        }
        lastLoginState.update(event);
    }
}
