package com.traffic.flink.behavior.sink;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

class BehaviorDlqSinkFactoryTest {

    @Test
    void onlyCanonicalDlqTopicIsAccepted() {
        assertThrows(IllegalArgumentException.class,
                () -> BehaviorDlqSinkFactory.create("broker:9092", "dlq.behavior-job"));
        assertNotNull(BehaviorDlqSinkFactory.create("broker:9092", "dlq.v1"));
    }
}
