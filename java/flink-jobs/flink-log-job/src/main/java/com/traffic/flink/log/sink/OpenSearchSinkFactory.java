package com.traffic.flink.log.sink;

import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.log.source.ValidatedDeviceLog;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.functions.RuntimeContext;
import org.apache.flink.streaming.connectors.elasticsearch.ElasticsearchSinkFunction;
import org.apache.flink.streaming.connectors.elasticsearch.RequestIndexer;
import org.apache.flink.streaming.connectors.elasticsearch7.ElasticsearchSink;
import org.apache.http.HttpHost;
import org.elasticsearch.action.index.IndexRequest;
import org.elasticsearch.client.Requests;
import org.elasticsearch.index.VersionType;

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

    /**
     * Versioned projection used by the canonical consumer rail. Kafka offset
     * plus one is a positive, replay-stable external version; the strict
     * event-id barrier rejects another payload before it can reach this sink.
     */
    public static ElasticsearchSink<ValidatedDeviceLog> createVersionedSink() {
        String host = System.getenv().getOrDefault("OPENSEARCH_HOST", "opensearch.middleware.svc");
        int port = Integer.parseInt(System.getenv().getOrDefault("OPENSEARCH_PORT", "9200"));
        ElasticsearchSink.Builder<ValidatedDeviceLog> builder =
                new ElasticsearchSink.Builder<>(
                        List.of(new HttpHost(host, port, "http")),
                        new VersionedLogIndexer());
        builder.setBulkFlushMaxActions(1000);
        builder.setBulkFlushInterval(5000);
        builder.setBulkFlushBackoff(true);
        builder.setBulkFlushBackoffType(ElasticsearchSink.FlushBackoffType.EXPONENTIAL);
        builder.setBulkFlushBackoffRetries(3);
        builder.setBulkFlushBackoffDelay(1000);
        builder.setFailureHandler((action, failure, restStatusCode, indexer) -> {
            throw new RuntimeException(
                    "OpenSearch versioned device-log item failed after retries: status="
                            + restStatusCode,
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

    static final class VersionedLogIndexer
            implements ElasticsearchSinkFunction<ValidatedDeviceLog> {
        private static final DateTimeFormatter FMT =
                DateTimeFormatter.ofPattern("yyyy.MM.dd").withZone(ZoneId.of("UTC"));

        @Override
        public void process(
                ValidatedDeviceLog input,
                RuntimeContext context,
                RequestIndexer indexer) {
            if (input == null) throw new IllegalArgumentException("validated DeviceLog is required");
            DeviceLog log = input.getLog();
            if (log.getTenantId().isBlank() || log.getLogId().isBlank()) {
                throw new IllegalArgumentException(
                        "OpenSearch device-log projection requires tenant_id and log_id");
            }
            long sourceVersion = input.getSource().getOffset() + 1L;
            if (sourceVersion <= 0L) {
                throw new IllegalArgumentException("OpenSearch source version must be positive");
            }
            String targetId = deterministicTargetId(log);
            String index = "device-logs-" + FMT.format(Instant.ofEpochMilli(log.getTimestamp()));
            Map<String, Object> document = new LinkedHashMap<>();
            document.put("tenant_id", log.getTenantId());
            document.put("log_id", log.getLogId());
            document.put("device_ip", log.getDeviceIp());
            document.put("device_type", log.getDeviceType());
            document.put("facility", log.getFacility());
            document.put("severity", log.getSeverity());
            document.put("timestamp", Instant.ofEpochMilli(log.getTimestamp()).toString());
            document.put("message", log.getMessage());
            document.put("parsed", log.getParsed());
            document.put("source", log.getSource());
            document.put("source_topic", input.getSource().getTopic());
            document.put("source_partition", input.getSource().getPartition());
            document.put("source_offset", input.getSource().getOffset());
            document.put("source_version", sourceVersion);
            document.put("source_sha256",
                    SourceQualityReceipt.hashSource(input.getSource().getValue()));
            document.put("projection_identity", targetId);

            IndexRequest request = Requests.indexRequest()
                    .index(index)
                    .id(targetId)
                    .version(sourceVersion)
                    .versionType(VersionType.EXTERNAL_GTE)
                    .source(document);
            indexer.add(request);
        }
    }

    static String deterministicTargetId(DeviceLog log) {
        return log.getTenantId() + ":" + log.getLogId();
    }
}
