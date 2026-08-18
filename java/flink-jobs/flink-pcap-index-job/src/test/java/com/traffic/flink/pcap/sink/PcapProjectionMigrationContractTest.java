package com.traffic.flink.pcap.sink;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import static org.junit.jupiter.api.Assertions.*;

class PcapProjectionMigrationContractTest {
    private static final Pattern LOCAL_TABLE = Pattern.compile(
            "CREATE TABLE IF NOT EXISTS traffic\\.pcap_index_v2_local \\((.*?)\\)\\s*ENGINE",
            Pattern.DOTALL);
    private static final Pattern COLUMN = Pattern.compile(
            "(?m)^\\s{2}([a-z][a-z0-9_]*)\\s+(.+)$");

    @Test
    void migrationCreatesAnIsolatedReplayConvergentTableWithTheExactJavaContract() throws Exception {
        Path repository = findRepositoryRoot();
        Path migration = repository.resolve(
                "deployments/clickhouse/migrations/202608141430_m02_pcap_projection_v2.sql");
        String sql = Files.readString(migration);
        String statements = sql.replaceAll("(?m)--.*$", "");
        assertFalse(statements.toUpperCase().contains("DROP "));
        assertFalse(statements.toUpperCase().contains("RENAME "));
        assertFalse(statements.toUpperCase().contains("ALTER TABLE"));
        assertFalse(statements.toUpperCase().contains("ON CLUSTER"),
                "the migration runner applies each immutable migration directly to every node");
        assertFalse(Pattern.compile("traffic\\.pcap_index(?:_local)?\\b(?!_v2)")
                .matcher(statements).find(), "migration must not mutate a legacy PCAP table");

        Matcher local = LOCAL_TABLE.matcher(statements);
        assertTrue(local.find(), "manifest-v2 local table DDL is absent");
        Matcher column = COLUMN.matcher(local.group(1));
        List<String> names = new java.util.ArrayList<>();
        List<String> types = new java.util.ArrayList<>();
        while (column.find()) {
            names.add(column.group(1));
            types.add(column.group(2).trim().replaceFirst(",$", ""));
        }

        PcapProjectionColumns contract = PcapProjectionColumns.manifestV2();
        assertEquals(contract.ordered(), names);
        assertEquals(contract.orderedTypes(), types);
        assertTrue(statements.contains("ReplicatedReplacingMergeTree("));
        assertTrue(statements.contains("'{replica}',\n  created_ts"));
        assertTrue(statements.contains("ORDER BY (tenant_id, probe_id, file_key, projection_identity)"));
        assertTrue(statements.contains("AS traffic.pcap_index_v2_local"));
        assertTrue(statements.contains("cityHash64(tenant_id, file_key)"));
    }

    private static Path findRepositoryRoot() {
        Path current = Path.of("").toAbsolutePath().normalize();
        while (current != null) {
            if (Files.isRegularFile(current.resolve("agent.md")) &&
                    Files.isDirectory(current.resolve("deployments/clickhouse/migrations"))) {
                return current;
            }
            current = current.getParent();
        }
        throw new IllegalStateException("traffic-analysis-platform repository root was not found");
    }
}
