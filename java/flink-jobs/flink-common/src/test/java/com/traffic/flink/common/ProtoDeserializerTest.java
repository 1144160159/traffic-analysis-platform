package com.traffic.flink.common;

import com.google.protobuf.UnknownFieldSet;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.SessionEvent;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

public class ProtoDeserializerTest {

    @Test
    public void stripUnknownFieldsRemovesNestedUnknownFields() throws Exception {
        SessionEvent source = sessionWithUnknownFields();
        ProtoDeserializer<SessionEvent> deserializer =
                new ProtoDeserializer<>(SessionEvent.class, true, true);

        SessionEvent parsed = deserializer.deserialize(source.toByteArray());

        assertEquals("evt-1", parsed.getHeader().getEventId());
        assertEquals("session-1", parsed.getSessionId());
        assertTrue(parsed.getUnknownFields().asMap().isEmpty());
        assertTrue(parsed.getHeader().getUnknownFields().asMap().isEmpty());
    }

    @Test
    public void defaultModeKeepsUnknownFields() throws Exception {
        SessionEvent source = sessionWithUnknownFields();
        ProtoDeserializer<SessionEvent> deserializer =
                new ProtoDeserializer<>(SessionEvent.class);

        SessionEvent parsed = deserializer.deserialize(source.toByteArray());

        assertFalse(parsed.getUnknownFields().asMap().isEmpty());
        assertFalse(parsed.getHeader().getUnknownFields().asMap().isEmpty());
    }

    private static SessionEvent sessionWithUnknownFields() {
        UnknownFieldSet.Field field = UnknownFieldSet.Field.newBuilder()
                .addVarint(123L)
                .build();
        UnknownFieldSet unknownFields = UnknownFieldSet.newBuilder()
                .addField(99, field)
                .build();

        EventHeader header = EventHeader.newBuilder()
                .setEventId("evt-1")
                .setTenantId("tenant-a")
                .setUnknownFields(unknownFields)
                .build();

        return SessionEvent.newBuilder()
                .setHeader(header)
                .setSessionId("session-1")
                .setUnknownFields(unknownFields)
                .build();
    }
}
