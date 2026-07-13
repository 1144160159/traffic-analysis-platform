package com.traffic.flink.session.source;

import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.Header;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;

/**
 * Raw Kafka record retained before protobuf parsing so parse failures can be
 * routed to DLQ with source coordinates and payload evidence.
 */
public class RawKafkaRecord implements Serializable {

    private static final long serialVersionUID = 1L;

    private final String topic;
    private final int partition;
    private final long offset;
    private final long timestamp;
    private final byte[] key;
    private final byte[] value;
    private final Map<String, String> headers;

    public RawKafkaRecord(
            String topic,
            int partition,
            long offset,
            long timestamp,
            byte[] key,
            byte[] value,
            Map<String, String> headers) {
        this.topic = topic;
        this.partition = partition;
        this.offset = offset;
        this.timestamp = timestamp;
        this.key = key == null ? null : key.clone();
        this.value = value == null ? null : value.clone();
        this.headers = headers == null ? new HashMap<>() : new HashMap<>(headers);
    }

    public static RawKafkaRecord fromConsumerRecord(ConsumerRecord<byte[], byte[]> record) {
        Map<String, String> headerMap = new HashMap<>();
        for (Header header : record.headers()) {
            if (header.value() != null) {
                headerMap.put(header.key(), new String(header.value(), StandardCharsets.UTF_8));
            }
        }
        return new RawKafkaRecord(
                record.topic(),
                record.partition(),
                record.offset(),
                record.timestamp(),
                record.key(),
                record.value(),
                headerMap);
    }

    public String getTopic() {
        return topic;
    }

    public int getPartition() {
        return partition;
    }

    public long getOffset() {
        return offset;
    }

    public long getTimestamp() {
        return timestamp;
    }

    public byte[] getKey() {
        return key == null ? null : key.clone();
    }

    public byte[] getValue() {
        return value == null ? null : value.clone();
    }

    public Map<String, String> getHeaders() {
        return headers;
    }

    public String keyAsString() {
        if (key == null || key.length == 0) {
            return "";
        }
        return new String(key, StandardCharsets.UTF_8);
    }

    public String header(String name) {
        return headers.getOrDefault(name, "");
    }
}
