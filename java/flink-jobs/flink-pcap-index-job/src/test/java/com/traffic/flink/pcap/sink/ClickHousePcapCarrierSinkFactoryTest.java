package com.traffic.flink.pcap.sink;

import com.traffic.flink.pcap.process.PcapManifestValidatorTest;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.junit.jupiter.api.Test;

import java.sql.PreparedStatement;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

class ClickHousePcapCarrierSinkFactoryTest {
    @Test
    void sqlUsesOneExactOrderedColumnContractAndRejectsDrift() {
        PcapProjectionColumns columns = PcapProjectionColumns.manifestV2();
        String sql = ClickHousePcapCarrierSinkFactory.buildInsertSql("traffic.pcap_index_v2", columns);
        assertTrue(sql.startsWith("INSERT INTO traffic.pcap_index_v2 (tenant_id, probe_id, file_key, bucket"));
        assertEquals(columns.size(), sql.length() - sql.replace("?", "").length());
        assertThrows(IllegalArgumentException.class,
                () -> ClickHousePcapCarrierSinkFactory.buildInsertSql("pcap_index; DROP TABLE x", columns));

        List<String> permuted = new ArrayList<>(columns.ordered());
        Collections.swap(permuted, 0, 1);
        assertThrows(IllegalArgumentException.class, () -> PcapProjectionColumns.ofForTest(permuted));
        List<String> duplicate = new ArrayList<>(columns.ordered());
        duplicate.set(1, duplicate.get(0));
        assertThrows(IllegalArgumentException.class, () -> PcapProjectionColumns.ofForTest(duplicate));
    }

    @Test
    void binderBindsEveryManifestAndSourceFieldWithoutExecutingBatch() throws Exception {
        PreparedStatement statement = mock(PreparedStatement.class);
        PcapIndexMeta meta = PcapManifestValidatorTest.v2Meta()
                .setCommunityId("1:primary")
                .addAllCommunityIds(List.of("1:primary", "1:secondary"))
                .setFlowId("flow-7")
                .setBloomFilterB64("AAECAw==")
                .build();
        PcapIndexedRecord record = PcapManifestValidatorTest.carrier(meta);

        new ClickHousePcapCarrierSinkFactory.PcapIndexedRecordStatementBuilder(
                PcapProjectionColumns.manifestV2()).accept(statement, record);

        verify(statement).setString(4, "pcap-archive");
        verify(statement).setLong(7, 8192L);
        verify(statement).setLong(8, 4096L);
        verify(statement).setString(11, "pcap.index.v1");
        verify(statement).setInt(12, 2);
        verify(statement).setLong(13, 41L);
        verify(statement).setString(17, record.getProjectionIdentity());
        verify(statement).setLong(18, meta.getTsStart());
        verify(statement).setLong(19, meta.getTsEnd());
        verify(statement).setLong(29, meta.getCreatedTs());
        verify(statement, never()).executeBatch();
        assertEquals(29, Arrays.stream(PcapProjectionColumns.manifestV2().ordered().toArray()).count());
    }

    @Test
    void sinkStartupRequiresAttestedReplayAndShardContracts() {
        PcapProjectionColumns columns = PcapProjectionColumns.manifestV2();
        PcapClickHouseConfig valid = config(columns.digest(), "ReplicatedReplacingMergeTree",
                "cityHash64(tenant_id,file_key)");
        assertNotNull(ClickHousePcapCarrierSinkFactory.createPcapIndexSink(valid, columns));

        assertThrows(IllegalArgumentException.class, () ->
                ClickHousePcapCarrierSinkFactory.createPcapIndexSink(
                        config("0".repeat(64), "ReplicatedReplacingMergeTree",
                                "cityHash64(tenant_id,file_key)"), columns));
        assertThrows(IllegalArgumentException.class, () ->
                ClickHousePcapCarrierSinkFactory.createPcapIndexSink(
                        config(columns.digest(), "ReplicatedMergeTree", "rand()"), columns));
    }

    private static PcapClickHouseConfig config(String digest, String engine, String shard) {
        return new PcapClickHouseConfig("jdbc:clickhouse://clickhouse:8123/traffic", "pcap_index_v2",
                "flink-pcap", "secret", digest, engine, shard, 1000, 2000, 3);
    }
}
