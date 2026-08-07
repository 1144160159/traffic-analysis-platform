package com.traffic.flink.log.sink;

import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.streaming.connectors.elasticsearch.RequestIndexer;
import org.elasticsearch.action.delete.DeleteRequest;
import org.elasticsearch.action.index.IndexRequest;
import org.elasticsearch.action.update.UpdateRequest;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class OpenSearchSinkFactoryTest {
    @Test
    void projectionUsesStableLogIdAndBusinessDateIndex() {
        DeviceLog log = DeviceLog.newBuilder()
                .setLogId("log-001")
                .setTenantId("tenant-a")
                .setTimestamp(1_704_067_200_000L)
                .setMessage("hello")
                .build();
        CapturingIndexer indexer = new CapturingIndexer();

        new OpenSearchSinkFactory.LogIndexer().process(log, null, indexer);

        assertEquals(1, indexer.indexRequests.size());
        assertEquals("log-001", indexer.indexRequests.get(0).id());
        assertEquals("device-logs-2024.01.01", indexer.indexRequests.get(0).index());
    }

    @Test
    void missingStableIdFailsClosed() {
        DeviceLog log = DeviceLog.newBuilder().setTimestamp(1_704_067_200_000L).build();
        assertThrows(
                IllegalArgumentException.class,
                () -> new OpenSearchSinkFactory.LogIndexer().process(log, null, new CapturingIndexer()));
    }

    private static class CapturingIndexer implements RequestIndexer {
        private final List<IndexRequest> indexRequests = new ArrayList<>();

        @Override
        public void add(DeleteRequest... deleteRequests) {
        }

        @Override
        public void add(IndexRequest... requests) {
            indexRequests.addAll(List.of(requests));
        }

        @Override
        public void add(UpdateRequest... updateRequests) {
        }
    }
}
