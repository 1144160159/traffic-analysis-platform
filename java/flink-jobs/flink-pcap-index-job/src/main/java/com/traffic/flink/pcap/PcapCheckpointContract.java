package com.traffic.flink.pcap;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.List;
import java.util.Locale;

/** Stable digest recorded beside a PCAP savepoint before graph replacement. */
public final class PcapCheckpointContract implements Serializable {
    private static final long serialVersionUID = 1L;
    private final String digest;
    private final List<String> operatorUids;

    PcapCheckpointContract(PcapCheckpointConfig config) {
        operatorUids = List.copyOf(config.getOperatorUids());
        String canonical = String.join("\n",
                "pcap-checkpoint-contract-v1", config.getStorageUri(),
                Long.toString(config.getIntervalMs()), Long.toString(config.getTimeoutMs()),
                Long.toString(config.getMinPauseMs()), Integer.toString(config.getTolerableFailures()),
                Integer.toString(config.getRestartAttempts()), Long.toString(config.getRestartDelayMs()),
                config.getMode().name(), "max-concurrent=1", "retain-on-cancellation=true",
                String.join(",", operatorUids));
        digest = sha256(canonical);
    }

    public String getDigest() { return digest; }
    public List<String> getOperatorUids() { return operatorUids; }

    private static String sha256(String value) {
        try {
            byte[] bytes = MessageDigest.getInstance("SHA-256")
                    .digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder result = new StringBuilder(64);
            for (byte item : bytes) result.append(String.format(Locale.ROOT, "%02x", item));
            return result.toString();
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }
}
