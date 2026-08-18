package com.traffic.flink.behavior.user;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.proto.traffic.v1.UserEvent;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class UserSourceFactMapperTest {
    @Test
    void mapsUserAggregateAndSourceVersion() {
        UserEvent event = UserEvent.newBuilder()
                .setTenantId("tenant-a")
                .setUserId("user-3")
                .setEventId("event-3")
                .setTimestamp(2_000L)
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "user.events.v1", 2, 8L, 2_010L,
                null, event.toByteArray(), Map.of());

        SourceFactRecord fact = UserBehaviorJob.toUserSourceFact(
                new ValidatedUserEvent(source, event), "flink-user-behavior-job");

        assertEquals("user_behavior", fact.getRail());
        assertEquals("user-3", fact.getAggregateId());
        assertEquals(9L, fact.getSourceVersion());
        assertEquals(8L, fact.getSourceOffset());
    }
}
