package com.traffic.flink.behavior.user.detector;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.flink.behavior.user.baseline.BaselineAwareUserEvent;
import com.traffic.flink.behavior.user.baseline.BaselineSnapshot;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import java.util.*;

/**
 * 权限提升检测器
 *
 * 业务场景：
 *   1. 普通用户突然获得管理员权限 (viewer→admin)
 *   2. 短时间内多次角色变更
 *   3. 非工作时间权限变更
 */
public class PrivilegeEscalationDetector extends KeyedProcessFunction<String, BaselineAwareUserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(PrivilegeEscalationDetector.class);
    private static final Set<String> ADMIN_ROLES = Set.of("admin", "super_admin", "operator");
    private static final Set<String> LOW_ROLES = Set.of("viewer", "analyst", "readonly");
    // 角色历史保留 7 天，覆盖最大 escalation 窗口（24h）并约束状态增长
    private static final long ROLE_HISTORY_TTL_HOURS = 24L * 7L;
    // MapState: role_name → last_assigned_time
    private MapState<String, Long> roleHistory;

    @Override public void open(Configuration params) {
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.hours(ROLE_HISTORY_TTL_HOURS))
                .setUpdateType(StateTtlConfig.UpdateType.OnReadAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();
        MapStateDescriptor<String, Long> descriptor =
                new MapStateDescriptor<>("role-history", String.class, Long.class);
        descriptor.enableTimeToLive(ttlConfig);
        roleHistory = getRuntimeContext().getMapState(descriptor);
    }

    @Override
    public void processElement(BaselineAwareUserEvent input, Context ctx, Collector<AnomalyEvent> out) throws Exception {
        UserEvent event = input.getEvent();
        long escalationWindowMs = input.longThreshold(
                "privilege_escalation_window_seconds", 3600L, 1L, 86_400L) * 1000L;
        // 只关注角色变更和权限相关事件
        String action = event.getAction() != null ? event.getAction().toLowerCase() : "";
        if (!action.contains("role") && !action.contains("permission") && !action.contains("grant") &&
            !event.getEventType().contains("role")) return;

        // 解析角色变更: resource 字段可能包含 "role:admin" 格式
        String roleName = extractRole(event.getResource());
        if (roleName == null) roleName = extractRole(event.getAction());
        if (roleName == null) return;

        long now = event.getTimestamp();
        String isAdmin = ADMIN_ROLES.contains(roleName) ? "admin" : (LOW_ROLES.contains(roleName) ? "low" : "unknown");

        if ("admin".equals(isAdmin)) {
            // 检查是否有过 low 角色
            boolean hadLowRole = false;
            for (String role : LOW_ROLES) {
                Long t = roleHistory.get(role);
                if (t != null && (now - t) < escalationWindowMs) { hadLowRole = true; break; }
            }
            if (hadLowRole) {
                AnomalyEvent anomaly = new AnomalyEvent(
                    event.getTenantId(), event.getUserId(), event.getUsername(),
                    "PRIVILEGE_ESCALATION", "critical", 0.90f,
                    String.format("Privilege escalation: %s role within 1h from %s", roleName, event.getSourceIp()),
                    event.getTimestamp(), event.getEventId(), roleName);
                anomaly.sourceIp1 = event.getSourceIp();
                anomaly.detailJson = baselineEvidence(input.getBaseline(), String.format(
                        "\"role\":\"%s\",\"source_ip\":\"%s\",\"window_seconds\":%d",
                        AnomalyEvent.escapeJson(roleName), AnomalyEvent.escapeJson(event.getSourceIp()),
                        escalationWindowMs / 1000L));
                out.collect(anomaly);
                LOG.warn("Privilege escalation: user={} role={} from {}", event.getUsername(), roleName, event.getSourceIp());
            }
        }

        // 记录角色变更历史
        roleHistory.put(roleName, now);
        // 限制历史大小：MapState.entries() 返回 Iterable，不能强转 Collection
        // （强转会抛 ClassCastException），也不能在迭代中 remove。先拷贝到
        // List 再按时间排序，删除最旧的条目。
        List<Map.Entry<String, Long>> entries = new ArrayList<>();
        for (Map.Entry<String, Long> entry : roleHistory.entries()) {
            entries.add(entry);
        }
        if (entries.size() > 20) {
            entries.sort(Map.Entry.comparingByValue());
            int toRemove = entries.size() - 10;
            for (int i = 0; i < toRemove; i++) {
                roleHistory.remove(entries.get(i).getKey());
            }
        }
    }

    private static String baselineEvidence(BaselineSnapshot baseline, String detail) {
        if (baseline == null) return "{" + detail + "}";
        return String.format("{%s,\"baseline_version\":%d,\"baseline_snapshot_sha256\":\"%s\"}",
                detail, baseline.getBaselineVersion(), baseline.getSnapshotSha256());
    }

    private String extractRole(String input) {
        if (input == null) return null;
        // "role:admin" → "admin", "grant admin" → "admin"
        for (String role : ADMIN_ROLES) if (input.toLowerCase().contains(role)) return role;
        for (String role : LOW_ROLES) if (input.toLowerCase().contains(role)) return role;
        return null;
    }
}
