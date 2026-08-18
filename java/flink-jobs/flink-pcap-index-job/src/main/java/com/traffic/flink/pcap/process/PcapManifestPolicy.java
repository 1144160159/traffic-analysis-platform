package com.traffic.flink.pcap.process;

import java.io.Serializable;

/** Explicit compatibility policy for PCAP object manifests. */
public final class PcapManifestPolicy implements Serializable {
    private static final long serialVersionUID = 1L;
    private final boolean allowLegacyV1;
    private final boolean requireObjectVersion;
    private final boolean requireCommunityIds;
    private final boolean requireBloomFilter;
    private final int maxCommunityIds;
    private final long maxObjectBytes;

    public PcapManifestPolicy(boolean allowLegacyV1, boolean requireObjectVersion,
                              boolean requireCommunityIds, boolean requireBloomFilter,
                              int maxCommunityIds, long maxObjectBytes) {
        if (maxCommunityIds <= 0 || maxObjectBytes <= 0) {
            throw new IllegalArgumentException("PCAP manifest limits must be positive");
        }
        this.allowLegacyV1 = allowLegacyV1;
        this.requireObjectVersion = requireObjectVersion;
        this.requireCommunityIds = requireCommunityIds;
        this.requireBloomFilter = requireBloomFilter;
        this.maxCommunityIds = maxCommunityIds;
        this.maxObjectBytes = maxObjectBytes;
    }

    public static PcapManifestPolicy strictV2() {
        return new PcapManifestPolicy(false, false, false, false, 1000,
                10L * 1024 * 1024 * 1024);
    }

    public static PcapManifestPolicy legacyReadCompatibility() {
        return new PcapManifestPolicy(true, false, false, false, 1000,
                10L * 1024 * 1024 * 1024);
    }

    boolean isAllowLegacyV1() { return allowLegacyV1; }
    boolean isRequireObjectVersion() { return requireObjectVersion; }
    boolean isRequireCommunityIds() { return requireCommunityIds; }
    boolean isRequireBloomFilter() { return requireBloomFilter; }
    int getMaxCommunityIds() { return maxCommunityIds; }
    long getMaxObjectBytes() { return maxObjectBytes; }
}
