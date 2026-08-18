package com.traffic.flink.alert.router;

import org.junit.jupiter.api.Test;

import java.util.Arrays;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/**
 * RunScopeRouter 路由核心测试(oracle:ATC-RTR-T-001/-T-002)。
 */
class RunScopeRouterTest {

    private RunScopeRouter.Subscription sub(String runId, String tenant, String state,
                                            long start, long end, String fence, String activeFence) {
        return new RunScopeRouter.Subscription(tenant, runId, "task-" + runId, "spec-1",
                1, state, start, end, fence, activeFence);
    }

    @Test
    void overlappingTasksDeriveMultipleEnvelopes() {
        List<RunScopeRouter.Subscription> subs = Arrays.asList(
                sub("run-1", "tenant-a", "ACTIVE", 0, 10_000, "f1", "f1"),
                sub("run-2", "tenant-a", "ACTIVE", 0, 10_000, "f2", "f2"));
        List<RunScopeRouter.Envelope> out = RunScopeRouter.route(subs, "tenant-a", 5_000, "c-1", 1);
        assertEquals(2, out.size(), "重叠任务必须各自派生");
        assertEquals("run-1", out.get(0).runId());
        assertEquals("run-2", out.get(1).runId());
    }

    @Test
    void staleFencingTokenIsRejected() {
        List<RunScopeRouter.Subscription> subs = List.of(
                sub("run-1", "tenant-a", "ACTIVE", 0, 10_000, "old-fence", "new-fence"));
        List<RunScopeRouter.Envelope> out = RunScopeRouter.route(subs, "tenant-a", 5_000, "c-1", 1);
        assertTrue(out.isEmpty(), "旧 fencing token 必须拒绝派生");
    }

    @Test
    void prepareAndCancelledStatesDoNotRoute() {
        List<RunScopeRouter.Subscription> subs = Arrays.asList(
                sub("run-1", "tenant-a", "PREPARE", 0, 10_000, "f1", "f1"),
                sub("run-2", "tenant-a", "CANCELLED", 0, 10_000, "f2", "f2"));
        List<RunScopeRouter.Envelope> out = RunScopeRouter.route(subs, "tenant-a", 5_000, "c-1", 1);
        assertTrue(out.isEmpty());
    }

    @Test
    void outsideWindowDoesNotRoute() {
        List<RunScopeRouter.Subscription> subs = List.of(
                sub("run-1", "tenant-a", "ACTIVE", 0, 10_000, "f1", "f1"));
        assertTrue(RunScopeRouter.route(subs, "tenant-a", 20_000, "c-1", 1).isEmpty());
    }

    @Test
    void crossTenantDoesNotRoute() {
        List<RunScopeRouter.Subscription> subs = List.of(
                sub("run-1", "tenant-b", "ACTIVE", 0, 10_000, "f1", "f1"));
        assertTrue(RunScopeRouter.route(subs, "tenant-a", 5_000, "c-1", 1).isEmpty());
    }

    @Test
    void zeroSubscriptionsDeriveZeroEnvelopes() {
        assertTrue(RunScopeRouter.route(List.of(), "tenant-a", 5_000, "c-1", 1).isEmpty());
    }

    @Test
    void envelopeCarriesExecutionContext() {
        List<RunScopeRouter.Subscription> subs = List.of(
                sub("run-1", "tenant-a", "ACTIVE", 0, 10_000, "f1", "f1"));
        RunScopeRouter.Envelope e = RunScopeRouter.route(subs, "tenant-a", 5_000, "c-1", 3).get(0);
        assertEquals("tenant-a", e.tenantId());
        assertEquals("spec-1", e.executionSpecSha256());
        assertEquals("S1", e.stageId());
        assertEquals(3, e.attempt());
        assertEquals("f1", e.fencingToken());
    }
}
