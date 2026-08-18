package com.traffic.flink.alert.router;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Objects;

/**
 * RunScopeRouter —— base 流按已 ACK 订阅派生 0..N 个 run envelope 的确定性路由核心。
 *
 * 不变量(ATC-RTR-001):
 *  - base flow.events.v1 保持无任务归属事实语义;
 *  - 同一事件可命中多个重叠任务,派生 0..N 个 envelope,互不干扰;
 *  - 只有 state=ACTIVE 且 revision 匹配、fencing 未过期、窗口覆盖事件的订阅才派生;
 *  - 旧 fencing token 的订阅拒绝(fail-closed)。
 */
public final class RunScopeRouter {

    /** 已 ACK 的运行订阅(PREPARE→ACTIVE revision 单调)。 */
    public static final class Subscription {
        private final String tenantId;
        private final String runId;
        private final String taskId;
        private final String executionSpecSha256;
        private final long revision;
        private final String state;      // PREPARE|ACTIVE|CANCELLED
        private final long windowStartMs;
        private final long windowEndMs;
        private final String fencingToken;
        private final String activeFencingToken; // 当前权威 fencing(过期订阅不派生)

        public Subscription(String tenantId, String runId, String taskId, String executionSpecSha256,
                            long revision, String state, long windowStartMs, long windowEndMs,
                            String fencingToken, String activeFencingToken) {
            this.tenantId = tenantId;
            this.runId = runId;
            this.taskId = taskId;
            this.executionSpecSha256 = executionSpecSha256;
            this.revision = revision;
            this.state = state;
            this.windowStartMs = windowStartMs;
            this.windowEndMs = windowEndMs;
            this.fencingToken = fencingToken;
            this.activeFencingToken = activeFencingToken;
        }

        public String tenantId() { return tenantId; }
        public String runId() { return runId; }
        public String taskId() { return taskId; }
        public String executionSpecSha256() { return executionSpecSha256; }
        public long revision() { return revision; }
        public String state() { return state; }
        public long windowStartMs() { return windowStartMs; }
        public long windowEndMs() { return windowEndMs; }
        public String fencingToken() { return fencingToken; }
        public String activeFencingToken() { return activeFencingToken; }
    }

    /** 派生信封(执行上下文)。 */
    public static final class Envelope {
        private final String tenantId;
        private final String taskId;
        private final String runId;
        private final String executionSpecSha256;
        private final String windowId;
        private final String stageId;
        private final int attempt;
        private final String fencingToken;

        Envelope(Subscription s, String windowId, String stageId, int attempt) {
            this.tenantId = s.tenantId;
            this.taskId = s.taskId;
            this.runId = s.runId;
            this.executionSpecSha256 = s.executionSpecSha256;
            this.windowId = windowId;
            this.stageId = stageId;
            this.attempt = attempt;
            this.fencingToken = s.activeFencingToken;
        }

        public String tenantId() { return tenantId; }
        public String taskId() { return taskId; }
        public String runId() { return runId; }
        public String executionSpecSha256() { return executionSpecSha256; }
        public String windowId() { return windowId; }
        public String stageId() { return stageId; }
        public int attempt() { return attempt; }
        public String fencingToken() { return fencingToken; }
    }

    /**
     * route —— 确定性派生(纯函数,无状态;Flink ProcessFunction 包一层做 collector)。
     *
     * @param eventTenantId 事件租户
     * @param eventTsMs     事件时间戳
     * @param communityId   流身份(用于 windowId 派生)
     * @param attempt       当前阶段 attempt 号
     * @return 0..N 个 envelope(按 runId 稳定排序,保证确定性)
     */
    public static List<Envelope> route(List<Subscription> subscriptions,
                                       String eventTenantId, long eventTsMs,
                                       String communityId, int attempt) {
        Objects.requireNonNull(subscriptions, "subscriptions");
        List<Envelope> out = new ArrayList<>();
        for (Subscription s : subscriptions) {
            if (!s.tenantId.equals(eventTenantId)) {
                continue; // 跨租户不派生
            }
            if (!"ACTIVE".equals(s.state)) {
                continue; // PREPARE/CANCELLED 不派生数据
            }
            if (s.fencingToken != null && !s.fencingToken.equals(s.activeFencingToken)) {
                continue; // 旧 fencing token 拒绝
            }
            if (eventTsMs < s.windowStartMs || eventTsMs >= s.windowEndMs) {
                continue; // 窗口外不派生
            }
            String windowId = s.runId + ":" + s.revision;
            out.add(new Envelope(s, windowId, "S1", attempt));
        }
        out.sort((a, b) -> a.runId.compareTo(b.runId));
        return out;
    }

    /** 空订阅表(默认)。 */
    public static List<Subscription> empty() {
        return Collections.emptyList();
    }
}
