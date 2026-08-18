package com.traffic.flink.behavior.user;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.UserEvent;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.streaming.api.operators.KeyedProcessOperator;
import org.apache.flink.streaming.api.watermark.Watermark;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.KeyedOneInputStreamOperatorTestHarness;
import org.apache.flink.util.OutputTag;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.Queue;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

class UserEventTimeFunctionTest {
    @Test
    void acceptedDuplicateConflictAndStrictLateAreExclusive() throws Exception {
        OutputTag<CanonicalDlqMessage> dlq = new OutputTag<CanonicalDlqMessage>("dlq") {};
        OutputTag<SourceQualityReceipt> quality = new OutputTag<SourceQualityReceipt>("quality") {};
        EventTimePolicy policy = new EventTimePolicy(10L, 1_000L, 100L, 5_000L, 0L);
        KeyedOneInputStreamOperatorTestHarness<String, ValidatedUserEvent, UserEvent> harness =
                new KeyedOneInputStreamOperatorTestHarness<>(
                        new KeyedProcessOperator<>(new UserEventTimeFunction(
                                policy, "flink-user-behavior-job", dlq, quality)),
                        ValidatedUserEvent::identityKey,
                        TypeInformation.of(String.class));
        try (harness) {
            harness.open();
            harness.setProcessingTime(2_000L);
            harness.processElement(new StreamRecord<>(input("event-1", 1_000L, 0, "payload-a"), 1_000L));
            harness.processElement(new StreamRecord<>(input("event-1", 1_000L, 1, "payload-a"), 1_000L));
            harness.processElement(new StreamRecord<>(input("event-1", 1_000L, 2, "payload-b"), 1_000L));
            harness.processWatermark(new Watermark(1_200L));
            harness.processElement(new StreamRecord<>(input("event-2", 1_100L, 3, "payload-c"), 1_100L));
            harness.processElement(new StreamRecord<>(input("event-3", 1_099L, 4, "payload-d"), 1_099L));

            assertEquals(2, harness.extractOutputValues().size());
            Queue<StreamRecord<SourceQualityReceipt>> receipts = harness.getSideOutput(quality);
            assertNotNull(receipts);
            assertEquals(5, receipts.size());
            assertEquals("accepted", receipts.remove().getValue().getCategory());
            assertEquals("duplicate", receipts.remove().getValue().getCategory());
            assertEquals("conflict", receipts.remove().getValue().getCategory());
            assertEquals("accepted", receipts.remove().getValue().getCategory());
            assertEquals("late", receipts.remove().getValue().getCategory());
            Queue<StreamRecord<CanonicalDlqMessage>> failures = harness.getSideOutput(dlq);
            assertNotNull(failures);
            assertEquals(2, failures.size());
        }
    }

    private static ValidatedUserEvent input(
            String eventID, long eventTime, long offset, String payloadIdentity) {
        UserEvent event = UserEvent.newBuilder()
                .setEventId(eventID).setTenantId("tenant-a").setUserId("user-7")
                .setEventType("user_create").setResult("success").setTimestamp(eventTime)
                .setAction(payloadIdentity).build();
        RawKafkaRecord source = new RawKafkaRecord(
                "user.events.v1", 1, offset, eventTime + 500L,
                "tenant-a:user-7".getBytes(StandardCharsets.UTF_8), event.toByteArray(),
                Map.of("tenant_id", "tenant-a", "event_id", eventID));
        return new ValidatedUserEvent(source, event);
    }
}
