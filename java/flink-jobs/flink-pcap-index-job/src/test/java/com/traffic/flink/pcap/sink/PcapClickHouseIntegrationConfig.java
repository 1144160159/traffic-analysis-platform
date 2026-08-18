package com.traffic.flink.pcap.sink;

/** Test-side access to the package-owned, startup-attested sink configuration. */
public final class PcapClickHouseIntegrationConfig {
    private PcapClickHouseIntegrationConfig() { }

    public static PcapClickHouseConfig ownedSingleNode(
            String jdbcUrl, String user, String password, int batchSize) {
        PcapProjectionColumns columns = PcapProjectionColumns.manifestV2();
        return new PcapClickHouseConfig(
                jdbcUrl, "traffic.pcap_index_v2", user, password, columns.digest(),
                "ReplicatedReplacingMergeTree", "cityHash64(tenant_id,file_key)",
                batchSize, 50L, 3);
    }
}
