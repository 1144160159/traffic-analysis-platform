package com.traffic.flink.common;

import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.Header;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/** Kafka record with immutable source coordinates retained before decoding. */
public final class RawKafkaRecord implements Serializable {

    private static final long serialVersionUID = 1L;

    private final String topic;
    private final int partition;
    private final long offset;
    private final long timestamp;
    private final byte[] key;
    private final byte[] value;
    private final Map<String, String> headers;
    private final Set<String> duplicateHeaderNames;

    public RawKafkaRecord(
            String topic,
            int partition,
            long offset,
            long timestamp,
            byte[] key,
            byte[] value,
            Map<String, String> headers) {
        this(topic, partition, offset, timestamp, key, value, headers, Set.of());
    }

    public RawKafkaRecord(
            String topic,
            int partition,
            long offset,
            long timestamp,
            byte[] key,
            byte[] value,
            Map<String, String> headers,
            Set<String> duplicateHeaderNames) {
        this.topic = topic;
        this.partition = partition;
        this.offset = offset;
        this.timestamp = timestamp;
        this.key = key == null ? null : key.clone();
        this.value = value == null ? null : value.clone();
        this.headers = headers == null ? new HashMap<>() : new HashMap<>(headers);
        this.duplicateHeaderNames = duplicateHeaderNames == null
                ? new HashSet<>() : new HashSet<>(duplicateHeaderNames);
    }

    public static RawKafkaRecord fromConsumerRecord(ConsumerRecord<byte[], byte[]> record) {
        Map<String, String> headerMap = new HashMap<>();
        Set<String> seenHeaderNames = new HashSet<>();
        Set<String> duplicateHeaderNames = new HashSet<>();
        for (Header header : record.headers()) {
            if (!seenHeaderNames.add(header.key())) duplicateHeaderNames.add(header.key());
            if (header.value() != null) {
                headerMap.put(header.key(), new String(header.value(), StandardCharsets.UTF_8));
            }
        }
        return new RawKafkaRecord(
                record.topic(), record.partition(), record.offset(), record.timestamp(),
                record.key(), record.value(), headerMap, duplicateHeaderNames);
    }

    public String getTopic() { return topic; }
    public int getPartition() { return partition; }
    public long getOffset() { return offset; }
    public long getTimestamp() { return timestamp; }
    public byte[] getKey() { return key == null ? null : key.clone(); }
    public byte[] getValue() { return value == null ? null : value.clone(); }
    public Map<String, String> getHeaders() { return new HashMap<>(headers); }
    public Set<String> getDuplicateHeaderNames() {
        return new HashSet<>(duplicateHeaderNames);
    }

    public String keyAsString() {
        return key == null || key.length == 0 ? "" : new String(key, StandardCharsets.UTF_8);
    }

    public String header(String name) {
        return headers.getOrDefault(name, "");
    }
}
