package com.traffic.flink.log.sink;

import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.functions.RuntimeContext;
import org.apache.flink.streaming.connectors.elasticsearch.ElasticsearchSinkFunction;
import org.apache.flink.streaming.connectors.elasticsearch.RequestIndexer;
import org.apache.flink.streaming.connectors.elasticsearch7.ElasticsearchSink;
import org.apache.http.HttpHost;
import org.elasticsearch.action.index.IndexRequest;
import org.elasticsearch.client.Requests;

import java.time.Instant;
import java.time.ZoneId;
import java.time.format.DateTimeFormatter;
import java.util.*;

/** OpenSearch Sink — 全文检索设备日志 */
public class OpenSearchSinkFactory {
    public static ElasticsearchSink<DeviceLog> createSink() {
        String host = System.getenv().getOrDefault("OPENSEARCH_HOST", "opensearch.middleware.svc");
        int port = Integer.parseInt(System.getenv().getOrDefault("OPENSEARCH_PORT", "9200"));
        List<HttpHost> hosts = List.of(new HttpHost(host, port, "http"));

        ElasticsearchSink.Builder<DeviceLog> builder = new ElasticsearchSink.Builder<>(hosts, new LogIndexer());
        builder.setBulkFlushMaxActions(1000);
        builder.setBulkFlushInterval(5000);
        return builder.build();
    }

    static class LogIndexer implements ElasticsearchSinkFunction<DeviceLog> {
        private static final DateTimeFormatter FMT = DateTimeFormatter.ofPattern("yyyy.MM.dd").withZone(ZoneId.of("UTC"));

        @Override public void process(DeviceLog log, RuntimeContext ctx, RequestIndexer indexer) {
            String index = "device-logs-" + FMT.format(Instant.ofEpochMilli(log.getTimestamp()));
            Map<String, Object> doc = new LinkedHashMap<>();
            doc.put("tenant_id", log.getTenantId());
            doc.put("device_ip", log.getDeviceIp());
            doc.put("device_type", log.getDeviceType());
            doc.put("facility", log.getFacility());
            doc.put("severity", log.getSeverity());
            doc.put("timestamp", Instant.ofEpochMilli(log.getTimestamp()).toString());
            doc.put("message", log.getMessage());
            doc.put("source", log.getSource());
            IndexRequest req = Requests.indexRequest().index(index).source(doc);
            indexer.add(req);
        }
    }
}
