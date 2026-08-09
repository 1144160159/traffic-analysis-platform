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
        builder.setBulkFlushBackoff(true);
        builder.setBulkFlushBackoffType(ElasticsearchSink.FlushBackoffType.EXPONENTIAL);
        builder.setBulkFlushBackoffRetries(3);
        builder.setBulkFlushBackoffDelay(1000);
        builder.setFailureHandler((action, failure, restStatusCode, indexer) -> {
            throw new RuntimeException(
                    "OpenSearch device-log bulk item failed after retries: status=" + restStatusCode,
                    failure);
        });
        return builder.build();
    }

    static class LogIndexer implements ElasticsearchSinkFunction<DeviceLog> {
        private static final DateTimeFormatter FMT = DateTimeFormatter.ofPattern("yyyy.MM.dd").withZone(ZoneId.of("UTC"));

        @Override public void process(DeviceLog log, RuntimeContext ctx, RequestIndexer indexer) {
            if (log == null || log.getLogId().isBlank()) {
                throw new IllegalArgumentException("OpenSearch device-log projection requires log_id");
            }
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
            // Stable log_id makes a checkpoint replay overwrite the same projection.
            IndexRequest req = Requests.indexRequest().index(index).id(log.getLogId()).source(doc);
            indexer.add(req);
        }
    }
}
