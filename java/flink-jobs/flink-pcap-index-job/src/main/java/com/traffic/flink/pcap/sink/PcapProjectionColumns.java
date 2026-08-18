package com.traffic.flink.pcap.sink;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

/** One ordered PCAP INSERT contract shared by SQL generation and binding. */
public final class PcapProjectionColumns implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final List<String> V2_COLUMNS = List.of(
            "tenant_id", "probe_id", "file_key", "bucket", "object_version", "etag",
            "original_size", "stored_size", "compression", "manifest_version",
            "kafka_topic", "kafka_partition", "kafka_offset", "kafka_key_sha256",
            "kafka_headers_sha256", "raw_sha256", "projection_identity",
            "ts_start", "ts_end", "byte_size", "zstd_level", "sha256", "community_id",
            "flow_id", "offset_start", "offset_end", "bloom_filter_b64", "community_ids",
            "created_ts");
    private static final List<String> V2_TYPES = List.of(
            "String", "String", "String", "String", "String", "String",
            "UInt64", "UInt64", "LowCardinality(String)", "UInt16",
            "String", "Int32", "Int64", "FixedString(64)", "FixedString(64)",
            "FixedString(64)", "FixedString(64)", "DateTime64(3, 'UTC')", "DateTime64(3, 'UTC')",
            "UInt64", "UInt8", "String", "String", "String", "Nullable(UInt64)",
            "Nullable(UInt64)", "String", "Array(String)", "DateTime64(3, 'UTC')");

    private final List<String> columns;
    private final String digest;

    private PcapProjectionColumns(List<String> columns) {
        this.columns = List.copyOf(columns);
        validateExactV2(this.columns);
        digest = digestOf(this.columns, V2_TYPES);
    }

    public static PcapProjectionColumns manifestV2() { return new PcapProjectionColumns(V2_COLUMNS); }

    static PcapProjectionColumns ofForTest(List<String> columns) {
        return new PcapProjectionColumns(columns);
    }

    public List<String> ordered() { return columns; }
    public List<String> orderedTypes() { return V2_TYPES; }
    public int size() { return columns.size(); }
    public String digest() { return digest; }

    public void validate() { validateExactV2(columns); }

    public boolean matchesLive(List<String> liveNames, List<String> liveTypes) {
        return columns.equals(liveNames) && V2_TYPES.equals(liveTypes) && digest.equals(digestOf(liveNames, liveTypes));
    }

    private static void validateExactV2(List<String> columns) {
        if (!columns.equals(V2_COLUMNS)) {
            throw new IllegalArgumentException("PCAP projection columns differ from the manifest-v2 exact order");
        }
        Set<String> unique = new HashSet<>(columns);
        if (unique.size() != columns.size()) {
            throw new IllegalArgumentException("PCAP projection columns contain duplicates");
        }
        if (columns.stream().anyMatch(name -> !name.matches("[a-z][a-z0-9_]*"))) {
            throw new IllegalArgumentException("PCAP projection contains an unsafe column name");
        }
    }

    private static String sha256Hex(String value) {
        try {
            byte[] bytes = MessageDigest.getInstance("SHA-256")
                    .digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(64);
            for (byte item : bytes) result.append(String.format(Locale.ROOT, "%02x", item));
            return result.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }

    private static String digestOf(List<String> names, List<String> types) {
        if (names.size() != types.size()) throw new IllegalArgumentException("PCAP column names and types differ in size");
        return sha256Hex(IntStream.range(0, names.size())
                .mapToObj(index -> names.get(index) + ":" + types.get(index))
                .collect(Collectors.joining("\n")));
    }
}
