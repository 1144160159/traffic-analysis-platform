package com.traffic.flink.pcap.process;

import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.streaming.api.operators.ProcessOperator;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.OneInputStreamOperatorTestHarness;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.util.ArrayDeque;
import java.util.List;
import java.util.Queue;

import static org.assertj.core.api.Assertions.assertThat;

class PcapIndexProcessFunctionTest {

    private OneInputStreamOperatorTestHarness<PcapIndexMeta, PcapIndexMeta> testHarness;

    @BeforeEach
    void setUp() throws Exception {
        PcapIndexProcessFunction function = new PcapIndexProcessFunction(10 * 1024L, 60_000L);
        testHarness = new OneInputStreamOperatorTestHarness<>(new ProcessOperator<>(function));
        testHarness.open();
    }

    @AfterEach
    void tearDown() throws Exception {
        if (testHarness != null) {
            testHarness.close();
        }
    }

    @Test
    @DisplayName("有效 PCAP 索引进入主输出且不写 DLQ")
    void validMetaShouldPassThroughMainOutput() throws Exception {
        PcapIndexMeta meta = validMetaBuilder()
                .setSha256("abc123def456")
                .addCommunityIds("1:community-a==")
                .setBloomFilterB64("Ymxvb20=")
                .build();

        testHarness.processElement(new StreamRecord<>(meta, meta.getTsEnd()));

        List<PcapIndexMeta> output = testHarness.extractOutputValues();
        assertThat(output).hasSize(1);
        assertThat(output.get(0).getFileKey()).isEqualTo(meta.getFileKey());
        assertThat(dlqOutput()).isEmpty();
    }

    @Test
    @DisplayName("缺少可降级字段时仍通过主输出")
    void missingOptionalForensicsFieldsShouldWarnButPass() throws Exception {
        PcapIndexMeta meta = validMetaBuilder()
                .clearSha256()
                .clearCommunityIds()
                .clearBloomFilterB64()
                .build();

        testHarness.processElement(new StreamRecord<>(meta, meta.getTsEnd()));

        List<PcapIndexMeta> output = testHarness.extractOutputValues();
        assertThat(output).hasSize(1);
        assertThat(output.get(0).getTenantId()).isEqualTo("tenant-1");
        assertThat(output.get(0).getCommunityIdsCount()).isZero();
        assertThat(dlqOutput()).isEmpty();
    }

    @Test
    @DisplayName("缺少 tenant_id 的坏数据写入 DLQ")
    void missingTenantShouldRouteToDlq() throws Exception {
        PcapIndexMeta meta = validMetaBuilder()
                .clearTenantId()
                .build();

        testHarness.processElement(new StreamRecord<>(meta, meta.getTsEnd()));

        assertThat(testHarness.extractOutputValues()).isEmpty();
        Queue<StreamRecord<String>> dlq = dlqOutput();
        assertThat(dlq).hasSize(1);
        assertThat(dlq.peek().getValue())
                .contains("\"reason\":\"Missing tenant_id\"")
                .contains("\"file_key\":\"pcap/tenant-1/probe-1/capture-001.pcap.zst\"");
    }

    @Test
    @DisplayName("时间倒挂的坏数据写入 DLQ")
    void invalidTimeRangeShouldRouteToDlq() throws Exception {
        PcapIndexMeta meta = validMetaBuilder()
                .setTsStart(2_000L)
                .setTsEnd(1_000L)
                .build();

        testHarness.processElement(new StreamRecord<>(meta, meta.getTsEnd()));

        assertThat(testHarness.extractOutputValues()).isEmpty();
        Queue<StreamRecord<String>> dlq = dlqOutput();
        assertThat(dlq).hasSize(1);
        assertThat(dlq.peek().getValue()).contains("Invalid time range: ts_start=2000, ts_end=1000");
    }

    private Queue<StreamRecord<String>> dlqOutput() {
        Queue<StreamRecord<String>> output = testHarness.getSideOutput(PcapIndexProcessFunction.DLQ_TAG);
        return output == null ? new ArrayDeque<>() : output;
    }

    private PcapIndexMeta.Builder validMetaBuilder() {
        return PcapIndexMeta.newBuilder()
                .setTenantId("tenant-1")
                .setProbeId("probe-1")
                .setFileKey("pcap/tenant-1/probe-1/capture-001.pcap.zst")
                .setTsStart(1_000L)
                .setTsEnd(2_000L)
                .setByteSize(1_024L)
                .setZstdLevel(3)
                .setSha256("abc123def456")
                .addCommunityIds("1:community-a==")
                .setBloomFilterB64("Ymxvb20=")
                .setCreatedTs(2_000L);
    }
}
