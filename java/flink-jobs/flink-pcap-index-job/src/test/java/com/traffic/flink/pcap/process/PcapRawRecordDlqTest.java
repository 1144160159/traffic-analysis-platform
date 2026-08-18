package com.traffic.flink.pcap.process;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.flink.pcap.source.PcapRawKafkaRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.junit.jupiter.api.Test;

import java.util.Optional;

import static org.junit.jupiter.api.Assertions.*;

class PcapRawRecordDlqTest {
    private static final String OBJECT_SHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

    @Test
    void validRecordProducesStableIndexedCarrier() {
        PcapRawKafkaRecord raw = validRaw("tenant-a:probe-a", "tenant-a", "probe-a", OBJECT_SHA);
        PcapIndexParseFunction.ParseResult first = PcapIndexParseFunction.parseRecord(raw);
        PcapIndexParseFunction.ParseResult replay = PcapIndexParseFunction.parseRecord(raw);
        assertNotNull(first.indexedRecord);
        assertNull(first.deadLetter);
        PcapIndexedRecord indexed = first.indexedRecord;
        assertEquals("pcap.index.v1", indexed.getTopic());
        assertEquals(2, indexed.getPartition());
        assertEquals(19, indexed.getOffset());
        assertEquals(indexed.getProjectionIdentity(), replay.indexedRecord.getProjectionIdentity());
        assertEquals(raw.getRawSha256(), indexed.getRawSha256());
    }

    @Test
    void malformedAndIdentityConflictBecomeOneCanonicalDeadLetter() throws Exception {
        PcapRawKafkaRecord malformed = raw(new byte[]{0x0a, (byte) 0xff},
                "tenant-a:probe-a", "tenant-a", "probe-a", "file-a", OBJECT_SHA);
        PcapIndexParseFunction.ParseResult badProto = PcapIndexParseFunction.parseRecord(malformed);
        assertNull(badProto.indexedRecord);
        assertEquals("INVALID_PROTOBUF", badProto.deadLetter.getErrorCode());
        JsonNode dlq = new ObjectMapper().readTree(badProto.deadLetter.toCanonicalJson());
        assertEquals("pcap.index.v1", dlq.get("original_topic").asText());
        assertEquals(2, dlq.get("original_partition").asInt());
        assertEquals(19, dlq.get("original_offset").asLong());
        assertEquals(malformed.getRawSha256(), dlq.get("metadata").get("raw_sha256").asText());

        PcapRawKafkaRecord conflict = validRaw("tenant-a:probe-b", "tenant-a", "probe-a", OBJECT_SHA);
        PcapIndexParseFunction.ParseResult identity = PcapIndexParseFunction.parseRecord(conflict);
        assertNull(identity.indexedRecord);
        assertEquals("IDENTITY_CONFLICT", identity.deadLetter.getErrorCode());
    }

    @Test
    void invalidManifestReasonIsDeterministic() {
        PcapIndexMeta meta = validMeta().toBuilder().setSha256("short").build();
        PcapRawKafkaRecord raw = raw(meta.toByteArray(), "tenant-a:probe-a",
                "tenant-a", "probe-a", "file-a", "short");
        PcapIndexParseFunction.ParseResult result = PcapIndexParseFunction.parseRecord(raw);
        assertNull(result.indexedRecord);
        assertEquals("INVALID_OBJECT_SHA256", result.deadLetter.getErrorCode());
    }

    private static PcapRawKafkaRecord validRaw(String key, String tenant, String probe, String sha) {
        return raw(validMeta().toByteArray(), key, tenant, probe, "file-a", sha);
    }

    private static PcapIndexMeta validMeta() {
        return PcapIndexMeta.newBuilder()
                .setTenantId("tenant-a").setProbeId("probe-a").setFileKey("file-a")
                .setTsStart(1_000).setTsEnd(2_000).setByteSize(4_096)
                .setOffsetStart(0).setOffsetEnd(4_096).setSha256(OBJECT_SHA).setCreatedTs(2_100)
                .build();
    }

    private static PcapRawKafkaRecord raw(byte[] value, String key, String tenant,
                                           String probe, String file, String sha) {
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", tenant.getBytes()); headers.add("probe_id", probe.getBytes());
        headers.add("file_key", file.getBytes()); headers.add("sha256", sha.getBytes());
        headers.add("content_type", "application/x-protobuf".getBytes());
        headers.add("proto_message_type", "traffic.v1.PcapIndexMeta".getBytes());
        headers.add("proto_schema_version", "v1".getBytes());
        return PcapRawKafkaRecord.fromConsumerRecord(new ConsumerRecord<>(
                "pcap.index.v1", 2, 19, 2_000L,
                org.apache.kafka.common.record.TimestampType.CREATE_TIME,
                key.length(), value.length, key.getBytes(), value, headers, Optional.of(1)));
    }
}
