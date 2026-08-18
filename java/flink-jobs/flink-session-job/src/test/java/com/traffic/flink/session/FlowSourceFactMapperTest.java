package com.traffic.flink.session;

import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.sourcefact.SourceFactRecord;
import com.traffic.flink.session.source.ValidatedFlowInput;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class FlowSourceFactMapperTest {
    @Test
    void mapsAggregateVersionAndExactSourceTuple() {
        FlowEvent flow = FlowEvent.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-a")
                        .setEventId("event-7")
                        .setAggregateVersion(7L)
                        .setSchemaVersion("1")
                        .setIngestTs(2_000L))
                .setFlowId("flow-7")
                .setTsEnd(1_900L)
                .build();
        RawKafkaRecord source = new RawKafkaRecord(
                "flow.events.v1", 3, 41L, 2_100L,
                null, flow.toByteArray(), Map.of());

        SourceFactRecord fact = SessionJob.toFlowSourceFact(
                new ValidatedFlowInput(source, flow), "flink-session-job");

        assertEquals("flow", fact.getRail());
        assertEquals("flow-7", fact.getAggregateId());
        assertEquals(7L, fact.getSourceVersion());
        assertEquals(3, fact.getSourcePartition());
        assertEquals(41L, fact.getSourceOffset());
    }
}
