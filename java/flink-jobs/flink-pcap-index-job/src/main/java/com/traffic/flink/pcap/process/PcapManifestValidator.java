package com.traffic.flink.pcap.process;

import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import java.util.ArrayList;
import java.util.List;
import java.util.regex.Pattern;

/** Pure validator for object-manifest and Kafka-source authority boundaries. */
public final class PcapManifestValidator {
    private static final Pattern SHA256 = Pattern.compile("^[0-9a-fA-F]{64}$");
    private PcapManifestValidator() { }

    public static PcapManifestValidation validate(PcapIndexedRecord record, PcapManifestPolicy policy) {
        if (record == null || policy == null) {
            throw new IllegalArgumentException("PCAP carrier and manifest policy are required");
        }
        PcapIndexMeta meta = record.getMeta();
        List<String> reasons = new ArrayList<>();
        requireText(meta.getTenantId(), "MISSING_TENANT_ID", reasons);
        requireText(meta.getProbeId(), "MISSING_PROBE_ID", reasons);
        requireText(meta.getFileKey(), "MISSING_FILE_KEY", reasons);
        if (meta.getTsStart() <= 0 || meta.getTsEnd() < meta.getTsStart()) reasons.add("INVALID_TIME_RANGE");
        if (meta.getCreatedTs() <= 0) reasons.add("MISSING_EVENT_CREATED_TS");
        if (!SHA256.matcher(meta.getSha256()).matches()) reasons.add("INVALID_OBJECT_SHA256");
        if (!SHA256.matcher(record.getRawSha256()).matches()) reasons.add("INVALID_RAW_SHA256");
        if (!SHA256.matcher(record.getKafkaKeySha256()).matches()) reasons.add("INVALID_KAFKA_KEY_SHA256");
        if (!SHA256.matcher(record.getKafkaHeadersSha256()).matches()) reasons.add("INVALID_KAFKA_HEADERS_SHA256");
        if (record.getTopic().trim().isEmpty() || record.getPartition() < 0 || record.getOffset() < 0) {
            reasons.add("INVALID_KAFKA_SOURCE_TUPLE");
        }
        if (meta.getCommunityIdsCount() > policy.getMaxCommunityIds()) reasons.add("COMMUNITY_IDS_LIMIT_EXCEEDED");
        if (policy.isRequireCommunityIds() && meta.getCommunityIdsCount() == 0) reasons.add("MISSING_COMMUNITY_IDS");
        if (policy.isRequireBloomFilter() && blank(meta.getBloomFilterB64())) reasons.add("MISSING_BLOOM_FILTER");

        boolean legacy = meta.getManifestVersion() == 0;
        if (legacy) {
            if (!policy.isAllowLegacyV1()) reasons.add("LEGACY_MANIFEST_NOT_ALLOWED");
            if (meta.getByteSize() == 0 || Long.compareUnsigned(meta.getByteSize(), policy.getMaxObjectBytes()) > 0) {
                reasons.add("INVALID_LEGACY_BYTE_SIZE");
            }
        } else {
            if (meta.getManifestVersion() != 2) reasons.add("UNSUPPORTED_MANIFEST_VERSION");
            requireText(meta.getBucket(), "MISSING_BUCKET", reasons);
            requireText(meta.getEtag(), "MISSING_ETAG", reasons);
            requireText(meta.getCompression(), "MISSING_COMPRESSION", reasons);
            if (policy.isRequireObjectVersion()) requireText(meta.getObjectVersion(), "MISSING_OBJECT_VERSION", reasons);
            if (meta.getOriginalSize() == 0 || meta.getStoredSize() == 0) reasons.add("MISSING_OBJECT_SIZE");
            if (Long.compareUnsigned(meta.getOriginalSize(), policy.getMaxObjectBytes()) > 0 ||
                    Long.compareUnsigned(meta.getStoredSize(), policy.getMaxObjectBytes()) > 0) {
                reasons.add("OBJECT_SIZE_LIMIT_EXCEEDED");
            }
            if (meta.getByteSize() != meta.getStoredSize()) reasons.add("STORED_SIZE_CONFLICT");
            if (Long.compareUnsigned(meta.getOffsetEnd(), meta.getOffsetStart()) < 0 ||
                    Long.compareUnsigned(meta.getOffsetEnd(), meta.getStoredSize()) > 0) {
                reasons.add("OBJECT_OFFSET_CONFLICT");
            }
            String compression = meta.getCompression().toLowerCase(java.util.Locale.ROOT);
            if (!"zstd".equals(compression) && !"none".equals(compression)) reasons.add("UNSUPPORTED_COMPRESSION");
        }

        String expectedIdentity = PcapIndexedRecord.computeProjectionIdentity(meta,
                record.getTopic() + ":" + record.getPartition() + ":" + record.getOffset(),
                record.getRawSha256(), record.getKafkaKeySha256(), record.getKafkaHeadersSha256());
        if (!expectedIdentity.equals(record.getProjectionIdentity())) reasons.add("PROJECTION_IDENTITY_CONFLICT");
        if (!reasons.isEmpty()) {
            return new PcapManifestValidation(PcapManifestValidation.Disposition.REJECTED,
                    reasons, expectedIdentity);
        }
        return new PcapManifestValidation(legacy
                ? PcapManifestValidation.Disposition.LEGACY_V1_COMPATIBLE
                : PcapManifestValidation.Disposition.COMPLETE_V2,
                List.of(), expectedIdentity);
    }

    private static void requireText(String value, String reason, List<String> reasons) {
        if (blank(value)) reasons.add(reason);
    }

    private static boolean blank(String value) { return value == null || value.trim().isEmpty(); }
}
