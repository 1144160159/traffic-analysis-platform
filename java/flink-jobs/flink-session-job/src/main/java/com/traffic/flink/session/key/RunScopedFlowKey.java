package com.traffic.flink.session.key;

import java.util.Objects;

/**
 * RunScopedFlowKey —— 派生分支会话键(tenant+run+community)。
 *
 * 约束(76.13.2):拒绝空 tenant/run/community;stableKey 无歧义编码;
 * 与 base 链 v1 key 合同(tenant_id+community_id)并存,派生分支才带 run。
 */
public final class RunScopedFlowKey {
    private final String tenantId;
    private final String runId;
    private final String communityId;

    private RunScopedFlowKey(String tenantId, String runId, String communityId) {
        this.tenantId = tenantId;
        this.runId = runId;
        this.communityId = communityId;
    }

    public static RunScopedFlowKey of(String tenantId, String runId, String communityId) {
        if (tenantId == null || tenantId.trim().isEmpty()) {
            throw new IllegalArgumentException("tenantId must not be empty");
        }
        if (runId == null || runId.trim().isEmpty()) {
            throw new IllegalArgumentException("runId must not be empty");
        }
        if (communityId == null || communityId.trim().isEmpty()) {
            throw new IllegalArgumentException("communityId must not be empty");
        }
        return new RunScopedFlowKey(tenantId, runId, communityId);
    }

    /** 无歧义编码(长度前缀,防分隔符碰撞)。 */
    public String stableKey() {
        return len(tenantId) + tenantId + len(runId) + runId + len(communityId) + communityId;
    }

    private static String len(String s) {
        return String.format("%04d", s.length());
    }

    public String tenantId() { return tenantId; }
    public String runId() { return runId; }
    public String communityId() { return communityId; }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof RunScopedFlowKey)) return false;
        RunScopedFlowKey that = (RunScopedFlowKey) o;
        return tenantId.equals(that.tenantId) && runId.equals(that.runId) && communityId.equals(that.communityId);
    }

    @Override
    public int hashCode() { return Objects.hash(tenantId, runId, communityId); }
}
