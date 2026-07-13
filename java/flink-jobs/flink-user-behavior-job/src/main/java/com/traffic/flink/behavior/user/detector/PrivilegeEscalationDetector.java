package com.traffic.flink.behavior.user.detector;

import com.traffic.flink.behavior.user.model.AnomalyEvent;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
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
public class PrivilegeEscalationDetector extends KeyedProcessFunction<String, UserEvent, AnomalyEvent> {
    private static final Logger LOG = LoggerFactory.getLogger(PrivilegeEscalationDetector.class);
    private static final Set<String> ADMIN_ROLES = Set.of("admin", "super_admin", "operator");
    private static final Set<String> LOW_ROLES = Set.of("viewer", "analyst", "readonly");
    // MapState: role_name → last_assigned_time
    private MapState<String, Long> roleHistory;

    @Override public void open(Configuration params) {
        roleHistory = getRuntimeContext().getMapState(new MapStateDescriptor<>("role-history", String.class, Long.class));
    }

    @Override
    public void processElement(UserEvent event, Context ctx, Collector<AnomalyEvent> out) throws Exception {
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
                if (t != null && (now - t) < 60 * 60_000L) { hadLowRole = true; break; } // 1小时内的提升
            }
            if (hadLowRole) {
                AnomalyEvent anomaly = new AnomalyEvent(
                    event.getTenantId(), event.getUserId(), event.getUsername(),
                    "PRIVILEGE_ESCALATION", "critical", 0.90f,
                    String.format("Privilege escalation: %s role within 1h from %s", roleName, event.getSourceIp()));
                anomaly.sourceIp1 = event.getSourceIp();
                anomaly.detailJson = String.format("{\"role\":\"%s\",\"source_ip\":\"%s\"}", roleName, event.getSourceIp());
                out.collect(anomaly);
                LOG.warn("Privilege escalation: user={} role={} from {}", event.getUsername(), roleName, event.getSourceIp());
            }
        }

        // 记录角色变更历史
        roleHistory.put(roleName, now);
        // 限制历史大小
        if (((Collection<?>) roleHistory.entries()).size() > 20) {
            Iterator<String> it = ((Collection<Map.Entry<String,Long>>) roleHistory.entries()).stream()
                    .sorted(Map.Entry.comparingByValue()).map(Map.Entry::getKey).iterator();
            while (it.hasNext() && ((Collection<?>) roleHistory.entries()).size() > 10) {
                roleHistory.remove(it.next());
            }
        }
    }

    private String extractRole(String input) {
        if (input == null) return null;
        // "role:admin" → "admin", "grant admin" → "admin"
        for (String role : ADMIN_ROLES) if (input.toLowerCase().contains(role)) return role;
        for (String role : LOW_ROLES) if (input.toLowerCase().contains(role)) return role;
        return null;
    }
}
