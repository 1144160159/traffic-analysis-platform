package com.traffic.flink.pcap.sink;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.ArrayList;
import java.util.List;

/** Reads live ClickHouse schema facts before any Kafka source is constructed. */
public final class PcapClickHouseSchemaAttestor {
    private PcapClickHouseSchemaAttestor() { }

    public static PcapClickHouseConfig attest(
            String jdbcUrl, String database, String table, String localTable,
            String username, String password, PcapProjectionColumns columns,
            int batchSize, long batchIntervalMs, int maxRetries) throws SQLException {
        validateIdentifier(database, "database");
        validateIdentifier(table, "distributed table");
        validateIdentifier(localTable, "local table");
        if (jdbcUrl == null || !jdbcUrl.startsWith("jdbc:clickhouse://")) {
            throw new IllegalArgumentException("approved ClickHouse JDBC URL is required");
        }
        if (columns == null) throw new IllegalArgumentException("PCAP projection columns are required");

        try (Connection connection = DriverManager.getConnection(jdbcUrl, username, password)) {
            List<String> names = new ArrayList<>();
            List<String> types = new ArrayList<>();
            try (PreparedStatement statement = connection.prepareStatement(
                    "SELECT name,type FROM system.columns WHERE database=? AND table=? ORDER BY position")) {
                statement.setString(1, database);
                statement.setString(2, table);
                try (ResultSet rows = statement.executeQuery()) {
                    while (rows.next()) {
                        names.add(rows.getString(1));
                        types.add(rows.getString(2));
                    }
                }
            }
            if (!columns.matchesLive(names, types)) {
                throw new SQLException("live ClickHouse PCAP column names/types differ from the manifest-v2 exact contract");
            }

            String localEngine = tableFact(connection, database, localTable, "engine");
            String distributedEngine = tableFact(connection, database, table, "engine_full");
            if (!"ReplicatedReplacingMergeTree".equals(localEngine)) {
                throw new SQLException("live ClickHouse PCAP local engine is not replay convergent");
            }
            String compact = distributedEngine.replace("`", "").replace(" ", "")
                    .replace("\n", "").replace("\t", "");
            if (!compact.contains("cityHash64(tenant_id,file_key)")) {
                throw new SQLException("live ClickHouse PCAP distributed shard expression is not stable");
            }
            return new PcapClickHouseConfig(jdbcUrl, table, username, password, columns.digest(),
                    localEngine, "cityHash64(tenant_id,file_key)", batchSize,
                    batchIntervalMs, maxRetries);
        }
    }

    private static String tableFact(Connection connection, String database, String table, String field)
            throws SQLException {
        String sql = "SELECT " + field + " FROM system.tables WHERE database=? AND name=?";
        try (PreparedStatement statement = connection.prepareStatement(sql)) {
            statement.setString(1, database);
            statement.setString(2, table);
            try (ResultSet rows = statement.executeQuery()) {
                if (!rows.next()) throw new SQLException("ClickHouse table is absent: " + database + "." + table);
                String value = rows.getString(1);
                if (rows.next()) throw new SQLException("ClickHouse table fact is ambiguous: " + database + "." + table);
                return value == null ? "" : value;
            }
        }
    }

    private static void validateIdentifier(String value, String field) {
        if (value == null || !value.matches("[a-z][a-z0-9_]*")) {
            throw new IllegalArgumentException("unsafe ClickHouse " + field + " identifier");
        }
    }
}
