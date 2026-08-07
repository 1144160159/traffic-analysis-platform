package com.traffic.flink.alert.sink;

import com.traffic.proto.traffic.v1.Alert;
import org.elasticsearch.action.index.IndexRequest;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OpenSearchAlertSinkFactoryTest {
    @Test
    void projectionUsesStableAlertId() throws Exception {
        Alert alert = Alert.newBuilder()
                .setTenantId("tenant-a")
                .setAlertId("alert-001")
                .setEventId("event-001")
                .setTraceId("0123456789abcdef0123456789abcdef")
                .setFirstSeen(1_700_000_000_000L)
                .setLastSeen(1_700_000_001_000L)
                .build();

        IndexRequest request = new OpenSearchAlertSinkFactory.AlertElasticsearchSinkFunction(
                "alerts-v1").buildRequest(alert);

        assertEquals("alerts-v1", request.index());
        assertEquals("alert-001", request.id());
        assertTrue(request.source().utf8ToString().contains(
                "\"trace_id\":\"0123456789abcdef0123456789abcdef\""));
    }

    @Test
    void missingStableIdFailsClosed() {
        Alert alert = Alert.newBuilder().setTenantId("tenant-a").build();
        OpenSearchAlertSinkFactory.AlertElasticsearchSinkFunction sink =
                new OpenSearchAlertSinkFactory.AlertElasticsearchSinkFunction("alerts-v1");
        assertThrows(IllegalArgumentException.class, () -> sink.buildRequest(alert));
    }
}
