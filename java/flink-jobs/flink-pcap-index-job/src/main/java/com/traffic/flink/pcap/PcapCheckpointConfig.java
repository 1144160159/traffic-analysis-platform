package com.traffic.flink.pcap;

import org.apache.flink.streaming.api.CheckpointingMode;

import java.io.Serializable;
import java.net.URI;
import java.util.List;
import java.util.Set;

/** Immutable checkpoint and savepoint-identity input validated before graph construction. */
public final class PcapCheckpointConfig implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final Set<String> DURABLE_SCHEMES = Set.of("s3", "s3a", "hdfs", "gs", "oss", "abfs", "abfss");

    private final String storageUri;
    private final long intervalMs;
    private final long timeoutMs;
    private final long minPauseMs;
    private final int tolerableFailures;
    private final int restartAttempts;
    private final long restartDelayMs;
    private final List<String> operatorUids;

    public PcapCheckpointConfig(String storageUri, long intervalMs, long timeoutMs,
                                long minPauseMs, int tolerableFailures, int restartAttempts,
                                long restartDelayMs, List<String> operatorUids) {
        this.storageUri = storageUri; this.intervalMs = intervalMs; this.timeoutMs = timeoutMs;
        this.minPauseMs = minPauseMs; this.tolerableFailures = tolerableFailures;
        this.restartAttempts = restartAttempts; this.restartDelayMs = restartDelayMs;
        this.operatorUids = operatorUids == null ? List.of() : List.copyOf(operatorUids);
        validate();
    }

    public void validate() {
        final URI uri;
        try {
            uri = URI.create(storageUri == null ? "" : storageUri);
        } catch (IllegalArgumentException error) {
            throw new IllegalArgumentException("PCAP checkpoint URI is invalid", error);
        }
        if (!DURABLE_SCHEMES.contains(uri.getScheme()) || uri.getHost() == null || uri.getHost().isEmpty()) {
            throw new IllegalArgumentException("PCAP checkpoint storage must use an approved durable URI");
        }
        // 规范要求 checkpoint interval 30-60s、timeout ≤ 10min。
        // 原校验只拒绝 <5s，允许 5-29s 间隔且 timeout 无上限。
        if (intervalMs < 30_000 || intervalMs > 60_000) {
            throw new IllegalArgumentException(
                    "PCAP checkpoint interval must be within [30s, 60s]");
        }
        if (timeoutMs <= intervalMs || timeoutMs > 600_000) {
            throw new IllegalArgumentException(
                    "PCAP checkpoint timeout must exceed interval and be within 10min");
        }
        if (minPauseMs < 1_000 || minPauseMs >= timeoutMs) {
            throw new IllegalArgumentException("PCAP checkpoint min pause is outside the approved bounds");
        }
        if (tolerableFailures < 0 || tolerableFailures > 3 || restartAttempts <= 0 ||
                restartAttempts > 20 || restartDelayMs < 1_000 || restartDelayMs > 300_000) {
            throw new IllegalArgumentException("PCAP checkpoint failure/restart policy is outside the approved bounds");
        }
        if (operatorUids.size() != 6 || operatorUids.stream().anyMatch(value -> value == null || value.trim().isEmpty()) ||
                operatorUids.stream().distinct().count() != operatorUids.size()) {
            throw new IllegalArgumentException("PCAP carrier graph must freeze six unique operator UIDs");
        }
    }

    String getStorageUri() { return storageUri; }
    long getIntervalMs() { return intervalMs; }
    long getTimeoutMs() { return timeoutMs; }
    long getMinPauseMs() { return minPauseMs; }
    int getTolerableFailures() { return tolerableFailures; }
    int getRestartAttempts() { return restartAttempts; }
    long getRestartDelayMs() { return restartDelayMs; }
    List<String> getOperatorUids() { return operatorUids; }
    CheckpointingMode getMode() { return CheckpointingMode.AT_LEAST_ONCE; }
}
