package com.traffic.flink.pcap.source;

import com.traffic.proto.traffic.v1.PcapIndexMeta;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.Map;

/** Validated PCAP metadata bound to its immutable Kafka source identity. */
public final class PcapIndexedRecord implements Serializable {
    private static final long serialVersionUID = 1L;
    private final PcapIndexMeta meta;
    private final PcapRawKafkaRecord source;
    private final String topic;
    private final int partition;
    private final long offset;
    private final byte[] key;
    private final String rawSha256;
    private final String kafkaKeySha256;
    private final String kafkaHeadersSha256;
    private final Map<String, String> canonicalHeaders;
    private final String projectionIdentity;

    public PcapIndexedRecord(PcapIndexMeta meta, PcapRawKafkaRecord raw) {
        if (meta == null || raw == null) throw new IllegalArgumentException("indexed meta and source are required");
        String expectedKey = meta.getTenantId() + ":" + meta.getProbeId();
        if (!expectedKey.equals(raw.keyAsString())) throw new IllegalArgumentException("Kafka key conflicts with PCAP payload identity");
        this.meta = meta;
        source = raw;
        topic = raw.getTopic(); partition = raw.getPartition(); offset = raw.getOffset();
        key = raw.getKey(); rawSha256 = raw.getRawSha256();
        kafkaKeySha256 = PcapRawKafkaRecord.sha256Hex(key);
        kafkaHeadersSha256 = raw.headersSha256();
        canonicalHeaders = new LinkedHashMap<>();
        for (String name : new String[]{"tenant_id", "probe_id", "file_key", "sha256", "content_type", "proto_message_type", "proto_schema_version"}) {
            canonicalHeaders.put(name, raw.requiredSingleHeader(name));
        }
        requireHeader("tenant_id", meta.getTenantId()); requireHeader("probe_id", meta.getProbeId());
        requireHeader("file_key", meta.getFileKey()); requireHeader("sha256", meta.getSha256());
        requireHeader("content_type", "application/x-protobuf");
        requireHeader("proto_message_type", "traffic.v1.PcapIndexMeta");
        String version = canonicalHeaders.get("proto_schema_version");
        if (!"v1".equals(version) && !"1".equals(version)) throw new IllegalArgumentException("unsupported PCAP protobuf schema version");
        projectionIdentity = computeProjectionIdentity(meta, raw.sourceIdentity(), rawSha256,
                kafkaKeySha256, kafkaHeadersSha256);
    }

    private void requireHeader(String name, String expected) {
        if (!expected.equals(canonicalHeaders.get(name))) throw new IllegalArgumentException("Kafka header " + name + " conflicts with PCAP payload");
    }
    public PcapIndexMeta getMeta() { return meta; }
    public PcapRawKafkaRecord getSource() { return source; }
    public String getTopic() { return topic; }
    public int getPartition() { return partition; }
    public long getOffset() { return offset; }
    public byte[] getKey() { return key.clone(); }
    public String getRawSha256() { return rawSha256; }
    public String getKafkaKeySha256() { return kafkaKeySha256; }
    public String getKafkaHeadersSha256() { return kafkaHeadersSha256; }
    public Map<String, String> getCanonicalHeaders() { return new LinkedHashMap<>(canonicalHeaders); }
    public String getProjectionIdentity() { return projectionIdentity; }

    public static String computeProjectionIdentity(PcapIndexMeta meta, String sourceIdentity,
                                                    String rawSha256, String keySha256,
                                                    String headersSha256) {
        String canonical = String.join("\u0000",
                meta.getTenantId(), meta.getProbeId(), meta.getBucket(), meta.getFileKey(),
                meta.getObjectVersion(), meta.getEtag(), Long.toUnsignedString(meta.getOriginalSize()),
                Long.toUnsignedString(meta.getStoredSize()), meta.getCompression(),
                Integer.toUnsignedString(meta.getManifestVersion()), meta.getSha256(), sourceIdentity,
                rawSha256, keySha256, headersSha256);
        return PcapRawKafkaRecord.sha256Hex(canonical.getBytes(StandardCharsets.UTF_8));
    }
}
