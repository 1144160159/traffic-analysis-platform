package com.traffic.flink.log.source;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.DeviceLog;
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
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DeviceLogEventTimeFunctionTest {

    @Test
    void clockRollbackBeyondToleranceGoesToDlqAndDoesNotReplaceMaximum() throws Exception {
        OutputTag<CanonicalDlqMessage> dlq = tag("clock-dlq");
        try (KeyedOneInputStreamOperatorTestHarness<String, ValidatedDeviceLog, DeviceLog> harness =
                     harness(100L, 100L, dlq)) {
            harness.processElement(new StreamRecord<>(input(1_000L, "log-1"), 1_000L));
            harness.processElement(new StreamRecord<>(input(900L, "log-boundary"), 900L));
            harness.processElement(new StreamRecord<>(input(899L, "log-rollback"), 899L));
            harness.processElement(new StreamRecord<>(input(950L, "log-after"), 950L));

            assertEquals(3, harness.extractOutputValues().size());
            Queue<StreamRecord<CanonicalDlqMessage>> failures = harness.getSideOutput(dlq);
            assertNotNull(failures);
            assertEquals(1, failures.size());
            assertEquals("CLOCK_ROLLBACK", failures.peek().getValue().errorCode());
        }
    }

    @Test
    void latenessUsesStrictBoundaryAndProducesSourceTraceableDlq() throws Exception {
        OutputTag<CanonicalDlqMessage> dlq = tag("late-dlq");
        try (KeyedOneInputStreamOperatorTestHarness<String, ValidatedDeviceLog, DeviceLog> harness =
                     harness(10L, 10_000L, dlq)) {
            harness.processWatermark(new Watermark(1_000L));
            harness.processElement(new StreamRecord<>(input(990L, "at-boundary"), 990L));
            harness.processElement(new StreamRecord<>(input(989L, "too-late"), 989L));

            assertEquals(1, harness.extractOutputValues().size());
            Queue<StreamRecord<CanonicalDlqMessage>> failures = harness.getSideOutput(dlq);
            assertNotNull(failures);
            assertEquals(1, failures.size());
            CanonicalDlqMessage failure = failures.peek().getValue();
            assertEquals("LATE_EVENT", failure.errorCode());
            assertEquals(7L, failure.originalOffset());
        }
    }

    @Test
    void staticChecksAreOverflowSafeAndNoWatermarkIsNotLate() {
        assertFalse(DeviceLogEventTimeFunction.isLate(
                Long.MIN_VALUE, Long.MIN_VALUE, Long.MAX_VALUE));
        assertFalse(DeviceLogEventTimeFunction.isClockRollback(
                Long.MIN_VALUE, Long.MIN_VALUE, Long.MAX_VALUE));
        assertTrue(DeviceLogEventTimeFunction.isLate(0L, 11L, 10L));
        assertFalse(DeviceLogEventTimeFunction.isLate(1L, 11L, 10L));
    }

    @Test
    void everyTerminalClassificationEmitsOneCheckpointReceipt() throws Exception {
        OutputTag<CanonicalDlqMessage> dlq = tag("quality-dlq");
        OutputTag<SourceQualityReceipt> quality = new OutputTag<SourceQualityReceipt>("quality") {};
        EventTimePolicy policy = new EventTimePolicy(10L, 1_000L, 10L, 5_000L, 100L);
        KeyedOneInputStreamOperatorTestHarness<String, ValidatedDeviceLog, DeviceLog> harness =
                new KeyedOneInputStreamOperatorTestHarness<>(
                        new KeyedProcessOperator<>(new DeviceLogEventTimeFunction(
                                policy, dlq, "flink-log-job-shadow-candidate", quality)),
                        ValidatedDeviceLog::identityKey,
                        TypeInformation.of(String.class));
        try (harness) {
            harness.open();
            harness.setProcessingTime(10_000L);
            harness.processElement(new StreamRecord<>(input(1_000L, "accepted"), 1_000L));
            harness.processWatermark(new Watermark(1_100L));
            harness.processElement(new StreamRecord<>(input(1_089L, "late"), 1_089L));

            Queue<StreamRecord<SourceQualityReceipt>> receipts = harness.getSideOutput(quality);
            assertNotNull(receipts);
            assertEquals(2, receipts.size());
            assertEquals("accepted", receipts.remove().getValue().getCategory());
            assertEquals("late", receipts.remove().getValue().getCategory());
        }
    }

    @Test
    void duplicateAndEventIdentityConflictAreExclusive() throws Exception {
        OutputTag<CanonicalDlqMessage> dlq = tag("identity-dlq");
        OutputTag<SourceQualityReceipt> quality = new OutputTag<SourceQualityReceipt>("identity-quality") {};
        EventTimePolicy policy = new EventTimePolicy(10L, 1_000L, 10L, 5_000L, 100L);
        KeyedOneInputStreamOperatorTestHarness<String, ValidatedDeviceLog, DeviceLog> harness =
                new KeyedOneInputStreamOperatorTestHarness<>(
                        new KeyedProcessOperator<>(new DeviceLogEventTimeFunction(
                                policy, dlq, "flink-log-job-shadow-candidate", quality)),
                        ValidatedDeviceLog::identityKey,
                        TypeInformation.of(String.class));
        try (harness) {
            harness.open();
            harness.setProcessingTime(10_000L);
            harness.processElement(new StreamRecord<>(input(1_000L, "log-1", 1L, "message-a"), 1_000L));
            harness.processElement(new StreamRecord<>(input(1_000L, "log-1", 2L, "message-a"), 1_000L));
            harness.processElement(new StreamRecord<>(input(1_000L, "log-1", 3L, "message-b"), 1_000L));

            assertEquals(1, harness.extractOutputValues().size());
            Queue<StreamRecord<SourceQualityReceipt>> receipts = harness.getSideOutput(quality);
            assertEquals("accepted", receipts.remove().getValue().getCategory());
            assertEquals("duplicate", receipts.remove().getValue().getCategory());
            assertEquals("conflict", receipts.remove().getValue().getCategory());
            Queue<StreamRecord<CanonicalDlqMessage>> failures = harness.getSideOutput(dlq);
            assertEquals("EVENT_ID_CONFLICT", failures.remove().getValue().errorCode());
        }
    }

    private static KeyedOneInputStreamOperatorTestHarness<
            String, ValidatedDeviceLog, DeviceLog> harness(
            long allowedLatenessMs,
            long rollbackToleranceMs,
            OutputTag<CanonicalDlqMessage> dlq) throws Exception {
        KeyedOneInputStreamOperatorTestHarness<String, ValidatedDeviceLog, DeviceLog> harness =
                new KeyedOneInputStreamOperatorTestHarness<>(
                        new KeyedProcessOperator<>(new DeviceLogEventTimeFunction(
                                allowedLatenessMs, rollbackToleranceMs, dlq)),
                        ValidatedDeviceLog::identityKey,
                        TypeInformation.of(String.class));
        harness.open();
        harness.setProcessingTime(10_000L);
        return harness;
    }

    private static OutputTag<CanonicalDlqMessage> tag(String name) {
        return new OutputTag<CanonicalDlqMessage>(name) {};
    }

    private static ValidatedDeviceLog input(long eventTime, String logId) {
        return input(eventTime, logId, 7L, "<134>Jan  1 00:00:00 fw-01 message");
    }

    private static ValidatedDeviceLog input(
            long eventTime, String logId, long offset, String message) {
        DeviceLog log = DeviceLog.newBuilder()
                .setLogId(logId)
                .setTenantId("tenant-a")
                .setDeviceIp("192.0.2.8")
                .setTimestamp(eventTime)
                .setMessage(message)
                .setSource("syslog")
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "device.logs.v1", 1, offset, eventTime + 1_000L,
                "tenant-a:192.0.2.8".getBytes(StandardCharsets.UTF_8),
                log.toByteArray(),
                Map.of("trace_id", "trace-1", "run_id", "run-1", "probe_id", "probe-1"));
        return new ValidatedDeviceLog(source, log);
    }
}
