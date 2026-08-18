package com.traffic.flink.log.source;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class DeviceLogParseFunctionTest {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final long EVENT_TIME = 1_700_000_000_000L;
    private static final long INGEST_TIME = EVENT_TIME + 1_000L;

    @Test
    void acceptsStrictEnvelopeAndRetainsSourceTuple() {
        DeviceLogParseFunction.ParseResult result = DeviceLogParseFunction.parse(
                source(validLog(), Map.of(), null, INGEST_TIME), 5_000L);

        assertNotNull(result.input);
        assertNull(result.failure);
        assertEquals("tenant-a:192.0.2.8", result.input.identityKey());
        assertEquals(2, result.input.getSource().getPartition());
        assertEquals(41L, result.input.getSource().getOffset());
    }

    @Test
    void corruptAndUnknownProtobufAreRejectedAsBadSchema() {
        DeviceLogParseFunction.ParseResult corrupt = DeviceLogParseFunction.parse(
                raw(new byte[]{(byte) 0xff, 0x01}, headers(validLog()), null, INGEST_TIME),
                5_000L);
        assertEquals("BAD_SCHEMA", corrupt.failure.errorCode());

        byte[] original = validLog().toByteArray();
        byte[] withUnknownField = Arrays.copyOf(original, original.length + 3);
        withUnknownField[original.length] = (byte) 0xa0;
        withUnknownField[original.length + 1] = 0x06;
        withUnknownField[original.length + 2] = 0x01;
        DeviceLogParseFunction.ParseResult unknown = DeviceLogParseFunction.parse(
                raw(withUnknownField, headers(validLog()), null, INGEST_TIME), 5_000L);
        assertEquals("BAD_SCHEMA", unknown.failure.errorCode());
    }

    @Test
    void tenantDeviceKeyAndHeadersMustAgreeWithBody() throws Exception {
        DeviceLog log = validLog();
        DeviceLogParseFunction.ParseResult wrongKey = DeviceLogParseFunction.parse(
                source(log, Map.of(), "tenant-b:192.0.2.8", INGEST_TIME), 5_000L);
        assertEquals("ENVELOPE_MISMATCH", wrongKey.failure.errorCode());

        DeviceLogParseFunction.ParseResult wrongTenantHeader = DeviceLogParseFunction.parse(
                source(log, Map.of("tenant_id", "tenant-b"), null, INGEST_TIME), 5_000L);
        assertEquals("ENVELOPE_MISMATCH", wrongTenantHeader.failure.errorCode());
        JsonNode dlq = JSON.readTree(wrongTenantHeader.failure.toJson());
        assertEquals("device.logs.v1", dlq.get("original_topic").asText());
        assertEquals(41L, dlq.get("original_offset").asLong());
        assertEquals("traffic.v1.DeviceLog", dlq.get("proto_message_type").asText());
        assertTrue(dlq.get("replay_policy").get("require_manual_ack").asBoolean());

        RawKafkaRecord duplicateHeader = new RawKafkaRecord(
                "device.logs.v1", 2, 41L, INGEST_TIME,
                "tenant-a:192.0.2.8".getBytes(StandardCharsets.UTF_8),
                log.toByteArray(), headers(log), Set.of("tenant_id"));
        assertEquals("ENVELOPE_MISMATCH", DeviceLogParseFunction.parse(
                duplicateHeader, 5_000L).failure.errorCode());
    }

    @Test
    void invalidIdentitySourceAndSyslogAreRejected() {
        DeviceLog missingTenant = validLog().toBuilder().clearTenantId().build();
        assertEquals("VALIDATION_ERROR", DeviceLogParseFunction.parse(
                source(missingTenant, Map.of(), null, INGEST_TIME), 5_000L)
                .failure.errorCode());

        DeviceLog hostname = validLog().toBuilder().setDeviceIp("router.example").build();
        assertEquals("VALIDATION_ERROR", DeviceLogParseFunction.parse(
                source(hostname, Map.of(), null, INGEST_TIME), 5_000L)
                .failure.errorCode());

        DeviceLog badSource = validLog().toBuilder().setSource("SYSLOG").build();
        assertEquals("VALIDATION_ERROR", DeviceLogParseFunction.parse(
                source(badSource, Map.of(), null, INGEST_TIME), 5_000L)
                .failure.errorCode());

        DeviceLog malformed = validLog().toBuilder().setMessage("not a syslog record").build();
        assertEquals("VALIDATION_ERROR", DeviceLogParseFunction.parse(
                source(malformed, Map.of(), null, INGEST_TIME), 5_000L)
                .failure.errorCode());
    }

    @Test
    void futureEventTimeAndWrongTopicAreRejected() {
        DeviceLog future = validLog().toBuilder().setTimestamp(INGEST_TIME + 5_001L).build();
        assertEquals("BAD_TIMESTAMP", DeviceLogParseFunction.parse(
                source(future, Map.of(), null, INGEST_TIME), 5_000L)
                .failure.errorCode());

        RawKafkaRecord wrongTopic = new RawKafkaRecord(
                "device.logs.legacy", 2, 41L, INGEST_TIME,
                "tenant-a:192.0.2.8".getBytes(StandardCharsets.UTF_8),
                validLog().toByteArray(), headers(validLog()));
        assertEquals("WRONG_SOURCE_TOPIC",
                DeviceLogParseFunction.parse(wrongTopic, 5_000L).failure.errorCode());
    }

    @Test
    void rejectionReceiptBindsSourceTuplePayloadAndStableCategory() {
        RawKafkaRecord source = source(validLog(), Map.of("tenant_id", "tenant-b"), null, INGEST_TIME);
        DeviceLogParseFunction.ParseResult result = DeviceLogParseFunction.parse(source, 5_000L);
        SourceQualityReceipt receipt = DeviceLogParseFunction.failureReceipt(
                source, result.failure, "flink-log-job-shadow-candidate", INGEST_TIME + 10L);
        SourceQualityReceipt replay = DeviceLogParseFunction.failureReceipt(
                source, result.failure, "flink-log-job-shadow-candidate", INGEST_TIME + 20L);
        assertEquals(receipt.getReceiptId(), replay.getReceiptId());
        assertEquals(41L, receipt.getOffset());
    }

    private static DeviceLog validLog() {
        return DeviceLog.newBuilder()
                .setLogId("log-001")
                .setTenantId("tenant-a")
                .setDeviceIp("192.0.2.8")
                .setDeviceType("firewall")
                .setFacility(16)
                .setSeverity(6)
                .setTimestamp(EVENT_TIME)
                .setMessage("<134>Jan  1 00:00:00 fw-01 accepted connection")
                .setParsed("{\"collector\":\"syslog\"}")
                .setSource("syslog")
                .build();
    }

    private static RawKafkaRecord source(
            DeviceLog log,
            Map<String, String> headerOverrides,
            String keyOverride,
            long ingestTime) {
        Map<String, String> values = headers(log);
        values.putAll(headerOverrides);
        String key = keyOverride == null
                ? log.getTenantId() + ":" + log.getDeviceIp() : keyOverride;
        return raw(log.toByteArray(), values, key, ingestTime);
    }

    private static RawKafkaRecord raw(
            byte[] value,
            Map<String, String> headers,
            String keyOverride,
            long ingestTime) {
        String key = keyOverride == null ? "tenant-a:192.0.2.8" : keyOverride;
        return new RawKafkaRecord(
                "device.logs.v1", 2, 41L, ingestTime,
                key.getBytes(StandardCharsets.UTF_8), value, headers);
    }

    private static Map<String, String> headers(DeviceLog log) {
        Map<String, String> headers = new HashMap<>();
        headers.put("tenant_id", log.getTenantId());
        headers.put("device_ip", log.getDeviceIp());
        headers.put("event_id", log.getLogId());
        headers.put("source", log.getSource());
        headers.put("schema_version", "device-log/v1");
        headers.put("content_type", "application/x-protobuf");
        headers.put("message_type", "traffic.v1.DeviceLog");
        headers.put("trace_id", "trace-1");
        headers.put("run_id", "run-1");
        headers.put("probe_id", "probe-1");
        return headers;
    }
}
