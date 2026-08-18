package com.traffic.flink.log;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.flink.log.source.ValidatedDeviceLog;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class DeviceLogSourceFactMapperTest {
    @Test
    void sourceOffsetBecomesPositiveReplayVersion() {
        DeviceLog log = DeviceLog.newBuilder()
                .setTenantId("tenant-a")
                .setLogId("log-2")
                .setDeviceIp("10.0.0.2")
                .setTimestamp(1_000L)
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "device.logs.v1", 1, 0L, 1_010L,
                null, log.toByteArray(), Map.of());

        SourceFactRecord fact = LogJob.toDeviceLogSourceFact(
                new ValidatedDeviceLog(source, log), "flink-log-job");

        assertEquals("device_log", fact.getRail());
        assertEquals("10.0.0.2", fact.getAggregateId());
        assertEquals(1L, fact.getSourceVersion());
        assertEquals(0L, fact.getSourceOffset());
    }
}
