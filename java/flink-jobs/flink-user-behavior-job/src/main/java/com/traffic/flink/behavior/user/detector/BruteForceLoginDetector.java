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
 * 暴力破解登录检测器
 *
 * 业务场景：同一用户在 10 分钟内连续 5 次登录失败，随后成功 → 标记为暴力破解
 * 匹配 agent.md 数据模型中定义的用户行为异常
 */
public class BruteForceLoginDetector extends KeyedProcessFunction<String, BaselineAwareUserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(BruteForceLoginDetector.class);
    private static final int MAX_FAILURES = 5;
    private static final long WINDOW_MS = 10 * 60_000L; // 10 min
    // 失败计数状态保留 1 天，覆盖最大窗口（24h）并约束 key(tenant|user) 状态增长
    private static final long STATE_TTL_HOURS = 24L;
    private ValueState<Integer> failCountState;
    private ValueState<Long> firstFailTimeState;

    @Override public void open(Configuration params) {
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.hours(STATE_TTL_HOURS))
                .setUpdateType(StateTtlConfig.UpdateType.OnReadAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();
        ValueStateDescriptor<Integer> countDescriptor =
                new ValueStateDescriptor<>("fail-count", Integer.class);
        countDescriptor.enableTimeToLive(ttlConfig);
        failCountState = getRuntimeContext().getState(countDescriptor);
        ValueStateDescriptor<Long> timeDescriptor =
                new ValueStateDescriptor<>("first-fail-time", Long.class);
        timeDescriptor.enableTimeToLive(ttlConfig);
        firstFailTimeState = getRuntimeContext().getState(timeDescriptor);
    }

    @Override
    public void processElement(BaselineAwareUserEvent input, Context ctx, Collector<AnomalyEvent> out) throws Exception {
        UserEvent event = input.getEvent();
        int maxFailures = input.intThreshold("brute_force_max_failures", MAX_FAILURES, 1, 1000);
        long windowMs = input.longThreshold("brute_force_window_seconds", WINDOW_MS / 1000L, 1L, 86_400L) * 1000L;
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
            if (count != null && count >= maxFailures && firstTime != null &&
                    (event.getTimestamp() - firstTime) < windowMs) {
                AnomalyEvent anomaly = new AnomalyEvent(
                    event.getTenantId(), event.getUserId(), event.getUsername(),
                    "BRUTE_FORCE", "critical", 0.95f,
                    String.format("Brute force: %d failures before success from %s", count, event.getSourceIp()),
                    event.getTimestamp(), event.getEventId(), String.valueOf(firstTime));
                anomaly.sourceIp1 = event.getSourceIp();
                anomaly.detailJson = baselineEvidence(input.getBaseline(), String.format(
                        "\"failures\":%d,\"source_ip\":\"%s\",\"window_seconds\":%d",
                        count, AnomalyEvent.escapeJson(event.getSourceIp()), windowMs / 1000L));
                out.collect(anomaly);
                LOG.warn("Brute force detected: user={} {} failures from {}", event.getUsername(), count, event.getSourceIp());
            }
            // Reset
            failCountState.clear();
            firstFailTimeState.clear();
        }
    }

    private static String baselineEvidence(BaselineSnapshot baseline, String detail) {
        if (baseline == null) return "{" + detail + "}";
        return String.format("{%s,\"baseline_version\":%d,\"baseline_snapshot_sha256\":\"%s\"}",
                detail, baseline.getBaselineVersion(), baseline.getSnapshotSha256());
    }
}
