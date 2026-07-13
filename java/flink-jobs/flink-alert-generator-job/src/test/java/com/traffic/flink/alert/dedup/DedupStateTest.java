package com.traffic.flink.alert.dedup;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.io.*;

import static org.junit.jupiter.api.Assertions.*;

/**
 * DedupState 单元测试
 */
class DedupStateTest {

    @Test
    @DisplayName("基本 getter/setter 测试")
    void testGetterSetter() {
        DedupState state = new DedupState();

        state.setFingerprint("fp-123");
        state.setAlertId("alert-1");
        state.setFirstSeen(1000L);
        state.setLastSeen(2000L);
        state.setCount(5);
        state.setStateVersion(3L);

        assertEquals("fp-123", state.getFingerprint());
        assertEquals("alert-1", state.getAlertId());
        assertEquals(1000L, state.getFirstSeen());
        assertEquals(2000L, state.getLastSeen());
        assertEquals(5, state.getCount());
        assertEquals(3L, state.getStateVersion());
    }

    @Test
    @DisplayName("序列化/反序列化测试")
    void testSerialization() throws Exception {
        DedupState original = new DedupState();
        original.setFingerprint("fp-serialize-test");
        original.setAlertId("alert-serialize");
        original.setFirstSeen(1700000000000L);
        original.setLastSeen(1700000060000L);
        original.setCount(10);
        original.setStateVersion(5L);

        // 序列化
        ByteArrayOutputStream baos = new ByteArrayOutputStream();
        ObjectOutputStream oos = new ObjectOutputStream(baos);
        oos.writeObject(original);
        oos.close();

        // 反序列化
        ByteArrayInputStream bais = new ByteArrayInputStream(baos.toByteArray());
        ObjectInputStream ois = new ObjectInputStream(bais);
        DedupState deserialized = (DedupState) ois.readObject();
        ois.close();

        // 验证
        assertEquals(original.getFingerprint(), deserialized.getFingerprint());
        assertEquals(original.getAlertId(), deserialized.getAlertId());
        assertEquals(original.getFirstSeen(), deserialized.getFirstSeen());
        assertEquals(original.getLastSeen(), deserialized.getLastSeen());
        assertEquals(original.getCount(), deserialized.getCount());
        assertEquals(original.getStateVersion(), deserialized.getStateVersion());
    }

    @Test
    @DisplayName("toString 测试")
    void testToString() {
        DedupState state = new DedupState();
        state.setFingerprint("fp-test");
        state.setAlertId("alert-test");
        state.setFirstSeen(1000L);
        state.setLastSeen(2000L);
        state.setCount(3);
        state.setStateVersion(2L);

        String str = state.toString();

        assertTrue(str.contains("fp-test"));
        assertTrue(str.contains("alert-test"));
        assertTrue(str.contains("1000"));
        assertTrue(str.contains("2000"));
        assertTrue(str.contains("count=3"));
        assertTrue(str.contains("stateVersion=2"));
    }

    @Test
    @DisplayName("默认值测试")
    void testDefaultValues() {
        DedupState state = new DedupState();

        assertEquals("", state.getFingerprint());
        assertEquals("", state.getAlertId());
        assertEquals(0L, state.getFirstSeen());
        assertEquals(0L, state.getLastSeen());
        assertEquals(0, state.getCount());
        assertEquals(0L, state.getStateVersion());
    }

    @Test
    @DisplayName("equals 和 hashCode 测试")
    void testEqualsAndHashCode() {
        DedupState state1 = new DedupState();
        state1.setFingerprint("fp-1");
        state1.setAlertId("alert-1");
        state1.setFirstSeen(1000L);
        state1.setLastSeen(2000L);
        state1.setCount(5);
        state1.setStateVersion(2L);

        DedupState state2 = new DedupState();
        state2.setFingerprint("fp-1");
        state2.setAlertId("alert-1");
        state2.setFirstSeen(1000L);
        state2.setLastSeen(2000L);
        state2.setCount(5);
        state2.setStateVersion(2L);

        DedupState state3 = new DedupState();
        state3.setFingerprint("fp-2");
        state3.setAlertId("alert-2");

        // 相同内容应相等
        assertEquals(state1, state2);
        assertEquals(state1.hashCode(), state2.hashCode());

        // 不同内容应不相等
        assertNotEquals(state1, state3);
    }
}