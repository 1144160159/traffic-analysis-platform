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
 * 暴力破解登录检测器
 *
 * 业务场景：同一用户在 10 分钟内连续 5 次登录失败，随后成功 → 标记为暴力破解
 * 匹配 agent.md 数据模型中定义的用户行为异常
 */
public class BruteForceLoginDetector extends KeyedProcessFunction<String, UserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(BruteForceLoginDetector.class);
    private static final int MAX_FAILURES = 5;
    private static final long WINDOW_MS = 10 * 60_000L; // 10 min
    private ValueState<Integer> failCountState;
    private ValueState<Long> firstFailTimeState;

    @Override public void open(Configuration params) {
        failCountState = getRuntimeContext().getState(new ValueStateDescriptor<>("fail-count", Integer.class));
        firstFailTimeState = getRuntimeContext().getState(new ValueStateDescriptor<>("first-fail-time", Long.class));
    }

    @Override
    public void processElement(UserEvent event, Context ctx, Collector<AnomalyEvent> out) throws Exception {
        String result = event.getResult() != null ? event.getResult().toLowerCase() : "";
        boolean isFailure = result.contains("fail") || result.contains("denied") || result.contains("error");
        boolean isSuccess = result.equals("success");

        if (isFailure) {
            Integer count = failCountState.value();
            if (count == null || count == 0) {
                firstFailTimeState.update(event.getTimestamp());
            }
            failCountState.update((count != null ? count : 0) + 1);
        } else if (isSuccess) {
            Integer count = failCountState.value();
            Long firstTime = firstFailTimeState.value();
            if (count != null && count >= MAX_FAILURES && firstTime != null &&
                    (event.getTimestamp() - firstTime) < WINDOW_MS) {
                AnomalyEvent anomaly = new AnomalyEvent(
                    event.getTenantId(), event.getUserId(), event.getUsername(),
                    "BRUTE_FORCE", "critical", 0.95f,
                    String.format("Brute force: %d failures before success from %s", count, event.getSourceIp()));
                anomaly.sourceIp1 = event.getSourceIp();
                anomaly.detailJson = String.format("{\"failures\":%d,\"source_ip\":\"%s\",\"window_min\":10}", count, event.getSourceIp());
                out.collect(anomaly);
                LOG.warn("Brute force detected: user={} {} failures from {}", event.getUsername(), count, event.getSourceIp());
            }
            // Reset
            failCountState.clear();
            firstFailTimeState.clear();
        }
    }
}
