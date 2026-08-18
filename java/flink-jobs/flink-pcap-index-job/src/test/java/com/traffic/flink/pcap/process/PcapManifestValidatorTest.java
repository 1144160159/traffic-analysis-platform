package com.traffic.flink.pcap.process;

import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.flink.pcap.source.PcapRawKafkaRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

public class PcapManifestValidatorTest {
    @Test
    void acceptsCompleteV2AndProducesStableProjectionIdentity() {
        PcapIndexedRecord first = carrier(v2Meta().build());
        PcapIndexedRecord replay = carrier(v2Meta().build());

        PcapManifestValidation result = PcapManifestValidator.validate(
                first, PcapManifestPolicy.strictV2());
        assertTrue(result.isAccepted());
        assertEquals(PcapManifestValidation.Disposition.COMPLETE_V2, result.getDisposition());
        assertEquals(first.getProjectionIdentity(), result.getProjectionIdentity());
        assertEquals(first.getProjectionIdentity(), replay.getProjectionIdentity());
    }

    @Test
    void oneHundredReplaysKeepOneProjectionIdentityAndOneSourceTuple() {
        PcapIndexMeta meta = v2Meta().build();
        java.util.Set<String> identities = new java.util.LinkedHashSet<>();
        java.util.Set<String> sourceTuples = new java.util.LinkedHashSet<>();
        for (int replay = 0; replay < 100; replay++) {
            PcapIndexedRecord record = carrier(meta);
            identities.add(record.getProjectionIdentity());
            sourceTuples.add(record.getTopic() + ":" + record.getPartition() + ":" + record.getOffset());
            assertTrue(PcapManifestValidator.validate(record, PcapManifestPolicy.strictV2())
                    .isAccepted());
        }
        assertEquals(1, identities.size());
        assertEquals(java.util.Set.of("pcap.index.v1:2:41"), sourceTuples);
    }

    @Test
    void legacyRequiresExplicitCompatibilityPolicy() {
        PcapIndexMeta legacy = base().build();
        PcapIndexedRecord record = carrier(legacy);

        PcapManifestValidation strict = PcapManifestValidator.validate(
                record, PcapManifestPolicy.strictV2());
        assertFalse(strict.isAccepted());
        assertEquals(List.of("LEGACY_MANIFEST_NOT_ALLOWED"), strict.getReasons());

        PcapManifestValidation compatible = PcapManifestValidator.validate(
                record, PcapManifestPolicy.legacyReadCompatibility());
        assertTrue(compatible.isAccepted());
        assertEquals(PcapManifestValidation.Disposition.LEGACY_V1_COMPATIBLE,
                compatible.getDisposition());
    }

    @Test
    void rejectsManifestConflictsWithOrderedExactReasons() {
        PcapIndexMeta invalid = v2Meta()
                .setManifestVersion(9)
                .clearBucket()
                .clearEtag()
                .setOriginalSize(0)
                .setStoredSize(2048)
                .setByteSize(4096)
                .setOffsetEnd(4096)
                .build();

        PcapManifestValidation result = PcapManifestValidator.validate(
                carrier(invalid), PcapManifestPolicy.strictV2());
        assertFalse(result.isAccepted());
        assertEquals(List.of("UNSUPPORTED_MANIFEST_VERSION", "MISSING_BUCKET", "MISSING_ETAG",
                "MISSING_OBJECT_SIZE", "STORED_SIZE_CONFLICT", "OBJECT_OFFSET_CONFLICT"),
                result.getReasons());
    }

    @Test
    void neverSilentlyTruncatesCommunityIds() {
        List<String> ids = new ArrayList<>();
        for (int index = 0; index < 1001; index++) ids.add("1:id-" + index);
        PcapManifestValidation result = PcapManifestValidator.validate(
                carrier(v2Meta().addAllCommunityIds(ids).build()),
                PcapManifestPolicy.strictV2());
        assertEquals(List.of("COMMUNITY_IDS_LIMIT_EXCEEDED"), result.getReasons());
    }

    public static PcapIndexMeta.Builder v2Meta() {
        return base()
                .setBucket("pcap-archive")
                .setObjectVersion("version-7")
                .setEtag("\"etag-7\"")
                .setOriginalSize(8192)
                .setStoredSize(4096)
                .setCompression("zstd")
                .setManifestVersion(2);
    }

    private static PcapIndexMeta.Builder base() {
        return PcapIndexMeta.newBuilder()
                .setTenantId("tenant-a")
                .setProbeId("probe-a")
                .setFileKey("tenant-a/probe-a/object.pcap.zst")
                .setTsStart(1_700_000_000_000L)
                .setTsEnd(1_700_000_001_000L)
                .setByteSize(4096)
                .setZstdLevel(3)
                .setSha256("a".repeat(64))
                .setOffsetStart(0)
                .setOffsetEnd(4096)
                .setCreatedTs(1_700_000_001_000L);
    }

    public static PcapIndexedRecord carrier(PcapIndexMeta meta) {
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", bytes(meta.getTenantId()));
        headers.add("probe_id", bytes(meta.getProbeId()));
        headers.add("file_key", bytes(meta.getFileKey()));
        headers.add("sha256", bytes(meta.getSha256()));
        headers.add("content_type", bytes("application/x-protobuf"));
        headers.add("proto_message_type", bytes("traffic.v1.PcapIndexMeta"));
        headers.add("proto_schema_version", bytes("v1"));
        ConsumerRecord<byte[], byte[]> source = new ConsumerRecord<>("pcap.index.v1", 2, 41,
                bytes(meta.getTenantId() + ":" + meta.getProbeId()), meta.toByteArray());
        headers.forEach(header -> source.headers().add(header));
        return new PcapIndexedRecord(meta, PcapRawKafkaRecord.fromConsumerRecord(source));
    }

    private static byte[] bytes(String value) { return value.getBytes(StandardCharsets.UTF_8); }
}
