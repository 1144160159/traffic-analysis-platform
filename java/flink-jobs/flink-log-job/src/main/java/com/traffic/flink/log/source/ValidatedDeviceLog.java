package com.traffic.flink.log.source;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.proto.traffic.v1.DeviceLog;

import java.io.Serializable;
import java.util.Objects;

/** A validated DeviceLog that retains its immutable Kafka source tuple. */
public final class ValidatedDeviceLog implements Serializable {
    private static final long serialVersionUID = 1L;

    private final RawKafkaRecord source;
    private final DeviceLog log;

    public ValidatedDeviceLog(RawKafkaRecord source, DeviceLog log) {
        this.source = Objects.requireNonNull(source, "source");
        this.log = Objects.requireNonNull(log, "log");
    }

    public RawKafkaRecord getSource() { return source; }
    public DeviceLog getLog() { return log; }

    public String identityKey() {
        return log.getTenantId() + ":" + log.getDeviceIp();
    }
}
