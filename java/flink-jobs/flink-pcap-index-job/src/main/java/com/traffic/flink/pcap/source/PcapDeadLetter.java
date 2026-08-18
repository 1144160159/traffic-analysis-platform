package com.traffic.flink.pcap.source;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;

import java.io.Serializable;
import java.time.Instant;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.Map;

/** Canonical dlq.v1 JSON payload for one rejected PCAP source coordinate. */
public final class PcapDeadLetter implements Serializable {
    private static final long serialVersionUID = 1L;
    private static final ObjectMapper JSON = new ObjectMapper();

    private final PcapRawKafkaRecord source;
    private final String errorCode;
    private final String errorMessage;
    private final String tenantId;
    private final String probeId;

    public PcapDeadLetter(PcapRawKafkaRecord source, String errorCode, String errorMessage,
                          String tenantId, String probeId) {
        if (source == null || isBlank(errorCode)) throw new IllegalArgumentException("DLQ source and error code are required");
        this.source = source;
        this.errorCode = errorCode;
        this.errorMessage = errorMessage == null ? "" : errorMessage;
        this.tenantId = tenantId == null ? "" : tenantId;
        this.probeId = probeId == null ? "" : probeId;
    }

    public PcapRawKafkaRecord getSource() { return source; }
    public String getErrorCode() { return errorCode; }
    public String getErrorMessage() { return errorMessage; }
    public String getTenantId() { return tenantId; }
    public String getProbeId() { return probeId; }
    public String projectionIdentity() { return "flink-pcap-index:" + source.sourceIdentity(); }

    public byte[] toCanonicalJson() throws JsonProcessingException {
        Map<String, Object> headers = new LinkedHashMap<>();
        int headerIndex = 0;
        for (PcapRawKafkaRecord.KafkaHeader header : source.getHeaders()) {
            headers.put(header.getKey() + "#" + headerIndex++,
                    Base64.getEncoder().encodeToString(header.getValue()));
        }
        Instant sourceTime = source.getTimestamp() > 0 ? Instant.ofEpochMilli(source.getTimestamp()) : Instant.EPOCH;
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("headers_sha256", source.headersSha256());
        metadata.put("raw_sha256", source.getRawSha256());
        metadata.put("projection_identity", projectionIdentity());
        metadata.put("timestamp_type", source.getTimestampType());

        Map<String, Object> document = new LinkedHashMap<>();
        document.put("original_topic", source.getTopic());
        document.put("original_partition", source.getPartition());
        document.put("original_offset", source.getOffset());
        document.put("original_key", source.keyAsString());
        document.put("original_value_b64", Base64.getEncoder().encodeToString(source.getValue()));
        document.put("original_headers", headers);
        document.put("original_timestamp", sourceTime.toString());
        document.put("content_type", "application/x-protobuf");
        document.put("proto_message_type", "traffic.v1.PcapIndexMeta");
        document.put("proto_schema_version", "v1");
        document.put("error_code", errorCode);
        document.put("error_message", errorMessage);
        document.put("error_type", "PERMANENT");
        document.put("failed_at", sourceTime.toString());
        document.put("retry_count", 0);
        document.put("service_name", "flink-pcap-index-job");
        document.put("processing_host", "flink-taskmanager");
        document.put("processed_at", sourceTime.toString());
        if (!tenantId.isEmpty()) document.put("tenant_id", tenantId);
        document.put("event_id", projectionIdentity());
        if (!probeId.isEmpty()) document.put("probe_id", probeId);
        document.put("metadata", metadata);
        document.put("replay_policy", Map.of("max_retries", 0, "require_manual_ack", true));
        return JSON.writeValueAsBytes(document);
    }

    private static boolean isBlank(String value) { return value == null || value.trim().isEmpty(); }
}
