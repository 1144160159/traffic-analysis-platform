package com.traffic.flink.pcap.sink;

import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.junit.jupiter.api.Test;

import java.sql.PreparedStatement;
import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;

class ClickHousePcapSinkFactoryTest {

    @Test
    void parameterBindingNeverExecutesOrClaimsBatchAck() throws Exception {
        PreparedStatement statement = mock(PreparedStatement.class);
        PcapIndexMeta meta = PcapIndexMeta.newBuilder()
                .setTenantId("tenant-a")
                .setProbeId("probe-a")
                .setFileKey("pcap/tenant-a/object.pcap.zst")
                .setTsStart(1_700_000_000_000L)
                .setTsEnd(1_700_000_001_000L)
                .setByteSize(4096L)
                .setZstdLevel(3)
                .setSha256("abc123")
                .build();

        new ClickHousePcapSinkFactory.PcapIndexStatementBuilder().accept(statement, meta);

        verify(statement, never()).executeBatch();
        assertFalse(Arrays.stream(ClickHousePcapSinkFactory.class.getDeclaredMethods())
                .anyMatch(method -> method.getName().equals("recordInsertSuccess")));
        assertFalse(Arrays.stream(ClickHousePcapSinkFactory.class.getDeclaredMethods())
                .anyMatch(method -> method.getName().equals("getSuccessCount")));
    }
}
