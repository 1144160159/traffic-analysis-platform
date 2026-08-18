package com.traffic.flink.behavior.user;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.UserEvent;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

class UserEventParseFunctionTest {
    private static final long EVENT_TIME = 1_700_000_000_000L;
    private static final long INGEST_TIME = EVENT_TIME + 1_000L;

    @Test
    void strictEnvelopeRetainsSourceTuple() {
        UserEventParseFunction.ParseResult result = UserEventParseFunction.parse(
                raw(validEvent(), headers(validEvent()), Set.of(), INGEST_TIME),
                "user.events.v1", 5_000L);
        assertNotNull(result.input);
        assertEquals(3, result.input.getSource().getPartition());
        assertEquals(17L, result.input.getSource().getOffset());
        assertEquals("tenant-a|user-7", result.input.identityKey());
    }

    @Test
    void corruptUnknownDuplicateHeaderAndEnvelopeMismatchFailClosed() {
        RawKafkaRecord corrupt = new RawKafkaRecord(
                "user.events.v1", 3, 17L, INGEST_TIME,
                "tenant-a:user-7".getBytes(StandardCharsets.UTF_8),
                new byte[]{(byte) 0xff, 0x01}, headers(validEvent()));
        assertEquals("BAD_SCHEMA", UserEventParseFunction.parse(
                corrupt, "user.events.v1", 5_000L).failure.errorCode());

        byte[] payload = validEvent().toByteArray();
        byte[] unknown = Arrays.copyOf(payload, payload.length + 3);
        unknown[payload.length] = (byte) 0xa0;
        unknown[payload.length + 1] = 0x06;
        unknown[payload.length + 2] = 0x01;
        RawKafkaRecord unknownRecord = new RawKafkaRecord(
                "user.events.v1", 3, 17L, INGEST_TIME,
                "tenant-a:user-7".getBytes(StandardCharsets.UTF_8), unknown,
                headers(validEvent()));
        assertEquals("BAD_SCHEMA", UserEventParseFunction.parse(
                unknownRecord, "user.events.v1", 5_000L).failure.errorCode());

        assertEquals("ENVELOPE_MISMATCH", UserEventParseFunction.parse(
                raw(validEvent(), headers(validEvent()), Set.of("tenant_id"), INGEST_TIME),
                "user.events.v1", 5_000L).failure.errorCode());
        Map<String, String> wrong = headers(validEvent());
        wrong.put("tenant_id", "tenant-b");
        assertEquals("ENVELOPE_MISMATCH", UserEventParseFunction.parse(
                raw(validEvent(), wrong, Set.of(), INGEST_TIME),
                "user.events.v1", 5_000L).failure.errorCode());
    }

    @Test
    void futureAndInvalidDomainEventsProduceTraceableQualityReceipt() {
        UserEvent future = validEvent().toBuilder().setTimestamp(INGEST_TIME + 5_001L).build();
        RawKafkaRecord source = raw(future, headers(future), Set.of(), INGEST_TIME);
        UserEventParseFunction.ParseResult result = UserEventParseFunction.parse(
                source, "user.events.v1", 5_000L);
        assertEquals("BAD_TIMESTAMP", result.failure.errorCode());
        SourceQualityReceipt receipt = UserEventParseFunction.rejectionReceipt(
                source, result.failure, "flink-user-behavior-job", INGEST_TIME + 1L);
        assertEquals("invalid", receipt.getCategory());
        assertEquals(17L, receipt.getOffset());

        UserEvent unsupported = validEvent().toBuilder().setEventType("login").build();
        assertEquals("VALIDATION_ERROR", UserEventParseFunction.parse(
                raw(unsupported, headers(unsupported), Set.of(), INGEST_TIME),
                "user.events.v1", 5_000L).failure.errorCode());
    }

    static UserEvent validEvent() {
        return UserEvent.newBuilder()
                .setEventId("event-1").setTenantId("tenant-a").setUserId("user-7")
                .setUsername("alice").setEventType("user_create")
                .setResource("user").setAction("create").setResult("success")
                .setTimestamp(EVENT_TIME).build();
    }

    static RawKafkaRecord raw(
            UserEvent event,
            Map<String, String> headers,
            Set<String> duplicates,
            long ingestTime) {
        return new RawKafkaRecord(
                "user.events.v1", 3, 17L, ingestTime,
                (event.getTenantId() + ":" + event.getUserId()).getBytes(StandardCharsets.UTF_8),
                event.toByteArray(), headers, duplicates);
    }

    static Map<String, String> headers(UserEvent event) {
        Map<String, String> headers = new HashMap<>();
        headers.put("tenant_id", event.getTenantId());
        headers.put("event_id", event.getEventId());
        headers.put("event_type", "traffic.user.command.v1." + event.getEventType());
        headers.put("schema_version", "1");
        headers.put("aggregate_version", "1");
        headers.put("trace_id", "trace-1");
        return headers;
    }
}
