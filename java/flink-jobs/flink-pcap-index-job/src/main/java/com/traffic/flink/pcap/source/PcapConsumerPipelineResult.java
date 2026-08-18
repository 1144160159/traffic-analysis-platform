package com.traffic.flink.pcap.source;

import org.apache.flink.streaming.api.datastream.SingleOutputStreamOperator;

import java.io.Serializable;
import java.util.List;

/** Built but not submitted N009 topology plus its stable savepoint identities. */
public final class PcapConsumerPipelineResult implements Serializable {
    private static final long serialVersionUID = 1L;
    private final SingleOutputStreamOperator<PcapIndexedRecord> indexedRecords;
    private final List<String> operatorUids;
    private final String downstreamCapability;

    PcapConsumerPipelineResult(SingleOutputStreamOperator<PcapIndexedRecord> indexedRecords,
                               List<String> operatorUids, String downstreamCapability) {
        if (indexedRecords == null || operatorUids == null || operatorUids.size() != 3) {
            throw new IllegalArgumentException("complete PCAP topology result is required");
        }
        this.indexedRecords = indexedRecords;
        this.operatorUids = List.copyOf(operatorUids);
        this.downstreamCapability = downstreamCapability;
    }
    public SingleOutputStreamOperator<PcapIndexedRecord> getIndexedRecords() { return indexedRecords; }
    public List<String> getOperatorUids() { return operatorUids; }
    public String getDownstreamCapability() { return downstreamCapability; }
}
