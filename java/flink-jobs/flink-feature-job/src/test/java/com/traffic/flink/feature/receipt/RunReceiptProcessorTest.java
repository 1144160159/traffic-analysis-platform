package com.traffic.flink.feature.receipt;

import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.streaming.util.KeyedOneInputStreamOperatorTestHarness;
import org.apache.flink.streaming.util.ProcessFunctionTestHarnesses;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.stream.Collectors;
import java.util.stream.StreamSupport;

import static org.assertj.core.api.Assertions.assertThat;

class RunReceiptProcessorTest {

    private static RunEnvelopeRecord env(String runId, String tenant, long tsEnd,
                                         long windowEnd, long packets, long bytes, String community) {
        RunEnvelopeRecord r = new RunEnvelopeRecord();
        setField(r, "tenantId", tenant);
        setField(r, "runId", runId);
        setField(r, "executionSpecSha256", "spec-1");
        setField(r, "fencingToken", "fence-1");
        setField(r, "windowEndMs", windowEnd);
        RunEnvelopeRecord.Event e = new RunEnvelopeRecord.Event();
        setField(e, "communityId", community);
        setField(e, "flowId", "flow-1");
        setField(e, "tsEnd", tsEnd);
        setField(e, "packetsFwd", packets);
        setField(e, "bytesFwd", bytes);
        setField(r, "event", e);
        return r;
    }

    private static void setField(Object target, String name, Object value) {
        try {
            java.lang.reflect.Field f = target.getClass().getDeclaredField(name);
            f.setAccessible(true);
            f.set(target, value);
        } catch (Exception e) {
            throw new RuntimeException(e);
        }
    }

    @Test
    void emitsS2ReceiptAfterWindowGrace() throws Exception {
        RunReceiptProcessor proc = new RunReceiptProcessor(10_000L);
        KeyedOneInputStreamOperatorTestHarness<String, RunEnvelopeRecord, String> h =
                ProcessFunctionTestHarnesses.forKeyedProcessFunction(
                        proc, RunEnvelopeRecord::runId, TypeInformation.of(String.class));
        h.open();
        long windowEnd = 1700000000000L;
        h.processElement(env("run-1", "default", windowEnd - 5000, windowEnd, 10, 1000, "1:cafe"), windowEnd - 5000);
        h.processElement(env("run-1", "default", windowEnd - 1000, windowEnd, 20, 2000, "1:beef"), windowEnd - 1000);
        // 推进水位线越过 window_end + grace,触发事件时间定时器
        h.processWatermark(windowEnd + 10_000L + 1);

        List<String> receipts = StreamSupport.stream(h.extractOutputStreamRecords().spliterator(), false)
                .map(rec -> rec.getValue())
                .collect(Collectors.toList());
        assertThat(receipts).hasSize(1);
        String receipt = receipts.get(0);
        assertThat(receipt).contains("\"execution_node_id\":\"SESSIONIZATION\"");
        assertThat(receipt).contains("\"run_id\":\"run-1\"");
        assertThat(receipt).contains("\"input_count\":2");
        assertThat(receipt).contains("\"output_count\":2"); // 2 个 community → 2 会话
        assertThat(receipt).contains("\"packets\":30");
        assertThat(receipt).contains("\"bytes\":3000");
        assertThat(receipt).contains("\"fencing_token\":\"fence-1\"");
    }

    @Test
    void noReceiptBeforeGrace() throws Exception {
        RunReceiptProcessor proc = new RunReceiptProcessor(10_000L);
        KeyedOneInputStreamOperatorTestHarness<String, RunEnvelopeRecord, String> h =
                ProcessFunctionTestHarnesses.forKeyedProcessFunction(
                        proc, RunEnvelopeRecord::runId, TypeInformation.of(String.class));
        h.open();
        long windowEnd = 1700000000000L;
        h.processElement(env("run-1", "default", windowEnd - 1000, windowEnd, 10, 1000, "1:cafe"), windowEnd - 1000);
        h.processWatermark(windowEnd + 1000); // 未过宽限
        assertThat(StreamSupport.stream(h.extractOutputStreamRecords().spliterator(), false)
                .collect(Collectors.toList())).isEmpty();
    }
}
