package com.traffic.flink.pcap;

import com.traffic.flink.pcap.source.PcapConsumerConfig;
import org.apache.flink.streaming.api.environment.CheckpointConfig;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.apache.flink.api.java.utils.ParameterTool;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

class PcapCheckpointContractTest {
    private static final List<String> UIDS = List.of(
            PcapConsumerConfig.SOURCE_UID, PcapConsumerConfig.PARSE_UID,
            PcapConsumerConfig.DLQ_UID, PcapIndexJob.MANIFEST_UID,
            PcapIndexJob.MANIFEST_DLQ_UID, PcapIndexJob.CLICKHOUSE_CARRIER_UID);

    @Test
    void configuresDurableRetainedSingleCheckpointAndStableDigest() {
        StreamExecutionEnvironment firstEnv = StreamExecutionEnvironment.getExecutionEnvironment();
        PcapCheckpointConfig config = valid(UIDS);
        PcapCheckpointContract first = PcapIndexJob.configureCheckpoint(firstEnv, config);
        PcapCheckpointContract replay = PcapIndexJob.configureCheckpoint(
                StreamExecutionEnvironment.getExecutionEnvironment(), valid(UIDS));

        assertEquals(64, first.getDigest().length());
        assertEquals(first.getDigest(), replay.getDigest());
        assertEquals(UIDS, first.getOperatorUids());
        CheckpointConfig effective = firstEnv.getCheckpointConfig();
        assertEquals(30_000L, effective.getCheckpointInterval());
        assertEquals(60_000L, effective.getCheckpointTimeout());
        assertEquals(1, effective.getMaxConcurrentCheckpoints());
        assertEquals(CheckpointConfig.ExternalizedCheckpointCleanup.RETAIN_ON_CANCELLATION,
                effective.getExternalizedCheckpointCleanup());
    }

    @Test
    void rejectsLocalStorageWeakTimingAndUidDrift() {
        assertThrows(IllegalArgumentException.class, () -> new PcapCheckpointConfig(
                "file:///tmp/checkpoints", 30_000, 60_000, 15_000, 3, 10, 30_000, UIDS));
        assertThrows(IllegalArgumentException.class, () -> new PcapCheckpointConfig(
                "s3://flink-checkpoints/pcap", 30_000, 20_000, 15_000, 3, 10, 30_000, UIDS));
        List<String> duplicate = new ArrayList<>(UIDS);
        duplicate.set(5, duplicate.get(0));
        assertThrows(IllegalArgumentException.class, () -> valid(duplicate));
    }

    @Test
    void validatesCanonicalCarrierActivationAndExplicitLegacyRollbackOnly() {
        Map<String, String> carrier = baseConfig();
        carrier.put("pcap.carrier.enabled", "true");
        carrier.put("kafka.canonical.dlq.topic", "dlq.v1");
        carrier.put("pcap.kafka.dlq.acl.attested", "true");
        carrier.put("pcap.kafka.idempotent.acl.attested", "true");
        assertDoesNotThrow(() -> PcapIndexJob.validateConfig(ParameterTool.fromMap(carrier)));

        Map<String, String> missingAcl = new HashMap<>(carrier);
        missingAcl.put("pcap.kafka.dlq.acl.attested", "false");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(missingAcl)));

        Map<String, String> autoCommit = new HashMap<>(carrier);
        autoCommit.put("enable.auto.commit", "true");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(autoCommit)));

        Map<String, String> localCheckpoint = new HashMap<>(carrier);
        localCheckpoint.put("checkpoint.path", "file:///tmp/pcap");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(localCheckpoint)));

        Map<String, String> carrierOnLegacyTable = new HashMap<>(carrier);
        carrierOnLegacyTable.put("clickhouse.table", "pcap_index");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(carrierOnLegacyTable)));

        Map<String, String> legacy = baseConfig();
        legacy.put("pcap.carrier.enabled", "false");
        legacy.put("clickhouse.table", "pcap_index");
        legacy.put("kafka.dlq.topic", "dlq.pcap-index-job");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(legacy)));
        legacy.put("pcap.legacy.private.dlq.compatibility.enabled", "true");
        assertDoesNotThrow(() -> PcapIndexJob.validateConfig(ParameterTool.fromMap(legacy)));

        Map<String, String> legacyOnCarrierTable = new HashMap<>(legacy);
        legacyOnCarrierTable.put("clickhouse.table", "pcap_index_v2");
        assertThrows(IllegalArgumentException.class,
                () -> PcapIndexJob.validateConfig(ParameterTool.fromMap(legacyOnCarrierTable)));
    }

    private static PcapCheckpointConfig valid(List<String> uids) {
        return new PcapCheckpointConfig("s3://flink-checkpoints/pcap", 30_000,
                60_000, 15_000, 3, 10, 30_000, uids);
    }

    private static Map<String, String> baseConfig() {
        Map<String, String> config = new HashMap<>();
        config.put("kafka.brokers", "broker:9092");
        config.put("kafka.input.topic", "pcap.index.v1");
        config.put("kafka.group.id", "flink-pcap-index-job-test");
        config.put("clickhouse.url", "clickhouse:8123");
        config.put("clickhouse.database", "traffic");
        config.put("clickhouse.table", "pcap_index_v2");
        config.put("checkpoint.path", "s3://flink-checkpoints/pcap");
        config.put("checkpoint.interval.ms", "30000");
        config.put("checkpoint.timeout.ms", "60000");
        config.put("checkpoint.min.pause.ms", "15000");
        config.put("checkpoint.tolerable.failures", "3");
        config.put("restart.attempts", "10");
        config.put("restart.delay.seconds", "30");
        config.put("enable.auto.commit", "false");
        config.put("commit.offsets.on.checkpoint", "true");
        return config;
    }
}
