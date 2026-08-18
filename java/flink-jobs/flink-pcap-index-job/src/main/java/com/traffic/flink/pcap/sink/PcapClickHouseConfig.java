package com.traffic.flink.pcap.sink;

import java.io.Serializable;

/** Startup-attested ClickHouse configuration for the dormant carrier sink. */
public final class PcapClickHouseConfig implements Serializable {
    private static final long serialVersionUID = 1L;
    private final String jdbcUrl;
    private final String table;
    private final String username;
    private final String password;
    private final String attestedColumnDigest;
    private final String attestedLocalEngine;
    private final String attestedShardExpression;
    private final int batchSize;
    private final long batchIntervalMs;
    private final int maxRetries;

    PcapClickHouseConfig(String jdbcUrl, String table, String username, String password,
                         String attestedColumnDigest, String attestedLocalEngine,
                         String attestedShardExpression, int batchSize,
                         long batchIntervalMs, int maxRetries) {
        this.jdbcUrl = jdbcUrl; this.table = table; this.username = username; this.password = password;
        this.attestedColumnDigest = attestedColumnDigest;
        this.attestedLocalEngine = attestedLocalEngine;
        this.attestedShardExpression = attestedShardExpression;
        this.batchSize = batchSize; this.batchIntervalMs = batchIntervalMs; this.maxRetries = maxRetries;
    }

    void validate(PcapProjectionColumns columns) {
        if (jdbcUrl == null || !jdbcUrl.startsWith("jdbc:clickhouse://") || jdbcUrl.contains("\n")) {
            throw new IllegalArgumentException("approved ClickHouse JDBC URL is required");
        }
        if (!("pcap_index_v2".equals(table) || "traffic.pcap_index_v2".equals(table))) {
            throw new IllegalArgumentException("carrier sink must target the approved distributed PCAP table");
        }
        if (username == null || username.trim().isEmpty() || password == null) {
            throw new IllegalArgumentException("ClickHouse credentials must come from an explicit secret binding");
        }
        if (!columns.digest().equals(attestedColumnDigest)) {
            throw new IllegalArgumentException("live ClickHouse column digest differs from the PCAP projection contract");
        }
        if (!"ReplicatedReplacingMergeTree".equals(attestedLocalEngine)) {
            throw new IllegalArgumentException("PCAP local table does not have the approved replay-convergent engine");
        }
        String compactShard = attestedShardExpression == null ? "" : attestedShardExpression.replace(" ", "");
        if (!"cityHash64(tenant_id,file_key)".equals(compactShard)) {
            throw new IllegalArgumentException("PCAP distributed table does not have the approved stable shard key");
        }
        if (batchSize <= 0 || batchSize > 5000 || batchIntervalMs <= 0 ||
                batchIntervalMs > 30_000 || maxRetries < 0 || maxRetries > 10) {
            throw new IllegalArgumentException("ClickHouse batch retry settings are outside the approved bounds");
        }
    }

    String getJdbcUrl() { return jdbcUrl; }
    String getTable() { return table; }
    String getUsername() { return username; }
    String getPassword() { return password; }
    int getBatchSize() { return batchSize; }
    long getBatchIntervalMs() { return batchIntervalMs; }
    int getMaxRetries() { return maxRetries; }
}
