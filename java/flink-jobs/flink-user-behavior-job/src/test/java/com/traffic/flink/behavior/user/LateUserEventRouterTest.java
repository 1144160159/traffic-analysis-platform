package com.traffic.flink.behavior.user;

import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.UserEvent;
import org.junit.jupiter.api.Test;

import java.util.Base64;

import static org.junit.jupiter.api.Assertions.assertArrayEquals;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class LateUserEventRouterTest {
    @Test
    void businessTimeAllowanceDefinesLateBoundary() {
        assertFalse(LateUserEventRouter.isTooLate(900L, Long.MIN_VALUE, 100L));
        assertTrue(LateUserEventRouter.isTooLate(900L, 1_000L, 100L));
        assertFalse(LateUserEventRouter.isTooLate(901L, 1_000L, 100L));
    }

    @Test
    void deadLetterIdentityAndPayloadAreReplayStable() {
        UserEvent event = UserEvent.newBuilder()
                .setEventId("user-event-7")
                .setTenantId("tenant-a")
                .setUserId("user-3")
                .setTimestamp(900L)
                .build();

        DeadLetter first = LateUserEventRouter.toDeadLetter(
                event, "user.events.v1", 1_000L, 100L);
        DeadLetter replay = LateUserEventRouter.toDeadLetter(
                event, "user.events.v1", 1_000L, 100L);

        assertEquals(first.getEventId(), replay.getEventId());
        assertEquals("tenant-a|user-3", first.getSourceKey());
        assertArrayEquals(event.toByteArray(), Base64.getDecoder().decode(first.getRawPayload()));
    }
}
