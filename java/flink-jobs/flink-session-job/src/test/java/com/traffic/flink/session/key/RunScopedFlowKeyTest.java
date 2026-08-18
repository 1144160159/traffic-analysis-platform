package com.traffic.flink.session.key;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/** RunScopedFlowKey 测试(oracle:ATC-SES)。 */
class RunScopedFlowKeyTest {

    @Test
    void rejectsEmptyComponents() {
        assertThrows(IllegalArgumentException.class, () -> RunScopedFlowKey.of("", "r", "c"));
        assertThrows(IllegalArgumentException.class, () -> RunScopedFlowKey.of("t", "", "c"));
        assertThrows(IllegalArgumentException.class, () -> RunScopedFlowKey.of("t", "r", ""));
        assertThrows(IllegalArgumentException.class, () -> RunScopedFlowKey.of(null, "r", "c"));
    }

    @Test
    void stableKeyIsUnambiguous() {
        // (t="ab", r="c") 与 (t="a", r="bc") 不得同键(长度前缀防碰撞)
        RunScopedFlowKey a = RunScopedFlowKey.of("ab", "c", "x");
        RunScopedFlowKey b = RunScopedFlowKey.of("a", "bc", "x");
        assertNotEquals(a.stableKey(), b.stableKey());
    }

    @Test
    void stableKeyIsDeterministic() {
        RunScopedFlowKey a = RunScopedFlowKey.of("tenant-a", "run-1", "1:abc==");
        RunScopedFlowKey b = RunScopedFlowKey.of("tenant-a", "run-1", "1:abc==");
        assertEquals(a.stableKey(), b.stableKey());
        assertEquals(a, b);
        assertEquals(a.hashCode(), b.hashCode());
    }

    @Test
    void distinctRunsProduceDistinctKeys() {
        RunScopedFlowKey a = RunScopedFlowKey.of("tenant-a", "run-1", "1:abc==");
        RunScopedFlowKey b = RunScopedFlowKey.of("tenant-a", "run-2", "1:abc==");
        assertNotEquals(a.stableKey(), b.stableKey());
    }
}
