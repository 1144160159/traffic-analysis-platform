package com.traffic.flink.pcap;

import com.traffic.proto.traffic.v1.PcapIndexMeta;
import org.junit.jupiter.api.*;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * PCAP Index Job 集成测试 — K8s Kafka (kubectl exec)
 */
@TestInstance(TestInstance.Lifecycle.PER_CLASS)
@TestMethodOrder(MethodOrderer.OrderAnnotation.class)
class PcapIndexJobIntegrationTest {

    private static final Logger LOG = LoggerFactory.getLogger(PcapIndexJobIntegrationTest.class);

    private String kubeExec(String namespace, String pod, String... cmd) throws Exception {
        // Use -- to separate kubectl args from container command args
        String[] fullCmd = new String[cmd.length + 6];
        fullCmd[0] = "kubectl"; fullCmd[1] = "exec"; fullCmd[2] = "-n";
        fullCmd[3] = namespace; fullCmd[4] = pod; fullCmd[5] = "--";
        System.arraycopy(cmd, 0, fullCmd, 6, cmd.length);
        Process p = new ProcessBuilder(fullCmd).redirectErrorStream(true).start();
        StringBuilder sb = new StringBuilder();
        try (BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()))) {
            String line; while ((line = r.readLine()) != null) sb.append(line).append("\n");
        }
        p.waitFor();
        return sb.toString().trim();
    }

    @Test @Order(1)
    @DisplayName("K8s Kafka produce → consume (kubectl exec)")
    void testKafkaProduceConsume() throws Exception {
        String msg = "pcap-test-" + UUID.randomUUID().toString().substring(0, 8);
        kubeExec("middleware", "kafka-0", "bash", "-c",
                "echo '" + msg + "' | /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server localhost:9092 --topic perf-test");
        Thread.sleep(2000);
        String result = kubeExec("middleware", "kafka-0",
                "/opt/kafka/bin/kafka-console-consumer.sh", "--bootstrap-server", "localhost:9092",
                "--topic", "perf-test", "--from-beginning", "--max-messages", "1", "--timeout-ms", "5000");
        LOG.info("Kafka round-trip: {}", result.length() > 0 ? "OK" : "FAIL");
        assertThat(result).isNotEmpty();
    }

    @Test @Order(2)
    @DisplayName("K8s ClickHouse 连通性验证 (kubectl exec)")
    void testClickHouseConnectivity() throws Exception {
        // Simple: verify ClickHouse is reachable and can execute queries
        String result = kubeExec("middleware", "clickhouse-1-0", "bash", "-c",
                "clickhouse-client --query 'SELECT 1'");
        assertThat(result).contains("1");
        LOG.info("ClickHouse connectivity OK: {}", result.trim());
    }

    @Test @Order(3)
    @DisplayName("Protobuf 序列化/反序列化")
    void testProtobufRoundTrip() throws Exception {
        long now = System.currentTimeMillis();
        PcapIndexMeta original = PcapIndexMeta.newBuilder()
                .setTenantId("proto-test").setProbeId("p1").setFileKey("f.pcap")
                .setTsStart(now - 30000).setTsEnd(now).setByteSize(2048).setZstdLevel(5)
                .setSha256("deadbeef").setCommunityId("1:p==").setFlowId("f1")
                .setBloomFilterB64("cHJvdG8=").addCommunityIds("1:a==").addCommunityIds("1:b==")
                .setCreatedTs(now).build();

        PcapIndexMeta parsed = PcapIndexMeta.parseFrom(original.toByteArray());
        assertThat(parsed.getTenantId()).isEqualTo("proto-test");
        assertThat(parsed.getByteSize()).isEqualTo(2048);
        assertThat(parsed.getZstdLevel()).isEqualTo(5);
        assertThat(parsed.getSha256()).isEqualTo("deadbeef");
        assertThat(parsed.getCommunityIdsCount()).isEqualTo(2);
        LOG.info("Protobuf round-trip OK: {} bytes", original.toByteArray().length);
    }

    @Test @Order(4)
    @DisplayName("Community IDs 大数组 (1500 IDs)")
    void testLargeCommunityIdsArray() {
        PcapIndexMeta.Builder b = PcapIndexMeta.newBuilder()
                .setTenantId("t").setProbeId("p").setFileKey("f").setByteSize(1)
                .setTsStart(1).setTsEnd(1);
        for (int i = 0; i < 1500; i++) b.addCommunityIds("1:id" + i + "==");
        PcapIndexMeta m = b.build();
        assertThat(m.getCommunityIdsCount()).isEqualTo(1500);
        assertThat(m.toByteArray().length).isGreaterThan(0);
        LOG.info("Large array: {} IDs, {} bytes", 1500, m.toByteArray().length);
    }
}
