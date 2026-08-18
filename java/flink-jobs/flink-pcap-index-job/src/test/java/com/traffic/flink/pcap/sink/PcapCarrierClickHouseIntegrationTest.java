package com.traffic.flink.pcap.sink;

import com.traffic.flink.pcap.process.PcapManifestValidatorTest;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.junit.jupiter.api.Test;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.sql.Statement;
import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.junit.jupiter.api.Assumptions.assumeTrue;

/** Runs only against an explicitly owned ephemeral ClickHouse instance. */
class PcapCarrierClickHouseIntegrationTest {
    private static final String SENTINEL = "codex_ephemeral_m02_clickhouse";

    @Test
    void migrationColumnContractAndCarrierBinderRoundTripOnEphemeralClickHouse() throws Exception {
        String jdbcUrl = System.getenv("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_JDBC_URL");
        assumeTrue(jdbcUrl != null && System.getenv("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_SENTINEL")
                .equals(SENTINEL), "owned ephemeral ClickHouse is not configured");
        String user = System.getenv().getOrDefault("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_USER", "m02");
        String password = System.getenv().getOrDefault("PCAP_CARRIER_EPHEMERAL_CLICKHOUSE_PASSWORD", "");
        PcapProjectionColumns columns = PcapProjectionColumns.manifestV2();

        try (Connection connection = DriverManager.getConnection(jdbcUrl, user, password)) {
            List<String> liveNames = new ArrayList<>();
            List<String> liveTypes = new ArrayList<>();
            try (PreparedStatement statement = connection.prepareStatement(
                    "SELECT name,type FROM system.columns WHERE database='traffic' " +
                            "AND table='pcap_index_v2' ORDER BY position")) {
                try (ResultSet rows = statement.executeQuery()) {
                    while (rows.next()) {
                        liveNames.add(rows.getString(1));
                        liveTypes.add(rows.getString(2));
                    }
                }
            }
            assertTrue(columns.matchesLive(liveNames, liveTypes));

            // The single-node test table proves DDL/binding compatibility, but must not
            // be accepted as a production replicated activation receipt.
            assertThrows(SQLException.class, () -> PcapClickHouseSchemaAttestor.attest(
                    jdbcUrl, "traffic", "pcap_index_v2", "pcap_index_v2_local", user, password,
                    columns, 10, 1000, 1));

            PcapIndexMeta meta = PcapManifestValidatorTest.v2Meta()
                    .setCommunityId("1:ephemeral")
                    .addCommunityIds("1:ephemeral")
                    .setFlowId("flow-ephemeral")
                    .setBloomFilterB64("AAECAw==")
                    .build();
            PcapIndexedRecord record = PcapManifestValidatorTest.carrier(meta);
            try (PreparedStatement insert = connection.prepareStatement(
                    ClickHousePcapCarrierSinkFactory.buildInsertSql("traffic.pcap_index_v2", columns))) {
                new ClickHousePcapCarrierSinkFactory.PcapIndexedRecordStatementBuilder(columns)
                        .accept(insert, record);
                assertTrue(insert.executeUpdate() >= 0);
            }
            try (PreparedStatement query = connection.prepareStatement(
                    "SELECT kafka_topic,kafka_partition,kafka_offset,projection_identity," +
                            "original_size,stored_size,toUnixTimestamp64Milli(ts_start)," +
                            "toUnixTimestamp64Milli(ts_end),toUnixTimestamp64Milli(created_ts) " +
                            "FROM traffic.pcap_index_v2 " +
                            "WHERE tenant_id=? AND file_key=?")) {
                query.setString(1, meta.getTenantId());
                query.setString(2, meta.getFileKey());
                try (ResultSet row = query.executeQuery()) {
                    assertTrue(row.next());
                    assertEquals(record.getTopic(), row.getString(1));
                    assertEquals(record.getPartition(), row.getInt(2));
                    assertEquals(record.getOffset(), row.getLong(3));
                    assertEquals(record.getProjectionIdentity(), row.getString(4));
                    assertEquals(meta.getOriginalSize(), row.getLong(5));
                    assertEquals(meta.getStoredSize(), row.getLong(6));
                    assertEquals(meta.getTsStart(), row.getLong(7));
                    assertEquals(meta.getTsEnd(), row.getLong(8));
                    assertEquals(meta.getCreatedTs(), row.getLong(9));
                    assertFalse(row.next());
                }
            }
        }
    }
}
