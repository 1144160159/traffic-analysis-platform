package com.traffic.flink.pcap.process;

import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapIndexedRecord;

import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Validates a source-bound carrier without replacing its Kafka authority. */
public final class PcapIndexedRecordProcessFunction
        extends ProcessFunction<PcapIndexedRecord, PcapIndexedRecord> {
    private static final long serialVersionUID = 1L;
    public static final OutputTag<PcapDeadLetter> DLQ_TAG =
            new OutputTag<PcapDeadLetter>("pcap-manifest-canonical-dlq") { };

    private final PcapManifestPolicy policy;

    public PcapIndexedRecordProcessFunction(PcapManifestPolicy policy) {
        if (policy == null) throw new IllegalArgumentException("PCAP manifest policy is required");
        this.policy = policy;
    }

    @Override
    public void processElement(PcapIndexedRecord record, Context ctx,
                               Collector<PcapIndexedRecord> out) throws Exception {
        if (record == null || ctx == null || out == null) {
            throw new IllegalArgumentException("PCAP carrier context and collector are required");
        }
        PcapManifestValidation validation = PcapManifestValidator.validate(record, policy);
        if (validation.isAccepted()) {
            out.collect(record);
            return;
        }
        ctx.output(DLQ_TAG, new PcapDeadLetter(record.getSource(), "MANIFEST_REJECTED",
                String.join(",", validation.getReasons()), record.getMeta().getTenantId(),
                record.getMeta().getProbeId()));
    }
}
