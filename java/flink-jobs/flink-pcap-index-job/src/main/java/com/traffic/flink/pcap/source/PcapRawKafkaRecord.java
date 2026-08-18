package com.traffic.flink.pcap.source;

import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.Header;

import java.io.Serializable;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Locale;
import java.util.Objects;

/** Immutable Kafka source record retained before protobuf parsing. */
public final class PcapRawKafkaRecord implements Serializable {
    private static final long serialVersionUID = 1L;
    public static final int MAX_VALUE_BYTES = 10 * 1024 * 1024;
    public static final int MAX_HEADER_BYTES = 64 * 1024;

    private final String topic;
    private final int partition;
    private final long offset;
    private final long timestamp;
    private final String timestampType;
    private final byte[] key;
    private final byte[] value;
    private final List<KafkaHeader> headers;
    private final String rawSha256;

    private PcapRawKafkaRecord(ConsumerRecord<byte[], byte[]> record) {
        topic = requireText(record.topic(), "topic");
        if (record.partition() < 0 || record.offset() < 0) {
            throw new IllegalArgumentException("Kafka partition and offset must be nonnegative");
        }
        partition = record.partition();
        offset = record.offset();
        timestamp = record.timestamp();
        timestampType = String.valueOf(record.timestampType());
        key = copy(record.key());
        value = copy(record.value());
        if (value.length > MAX_VALUE_BYTES) {
            throw new IllegalArgumentException("Kafka PCAP value exceeds the bounded carrier size");
        }
        List<KafkaHeader> copiedHeaders = new ArrayList<>();
        int headerBytes = 0;
        for (Header header : record.headers()) {
            String name = requireText(header.key(), "header key");
            byte[] headerValue = copy(header.value());
            headerBytes += name.getBytes(StandardCharsets.UTF_8).length + headerValue.length;
            if (headerBytes > MAX_HEADER_BYTES) {
                throw new IllegalArgumentException("Kafka PCAP headers exceed the bounded carrier size");
            }
            copiedHeaders.add(new KafkaHeader(name, headerValue));
        }
        // Keep the serialized field concrete and mutable. Flink 1.18 falls back to Kryo for this
        // immutable carrier; Kryo instantiates the stored collection type before copying fields.
        // Collections.unmodifiableList therefore fails on every real source-to-operator handoff.
        // Immutability is preserved at the accessor boundary with defensive element copies.
        headers = copiedHeaders;
        rawSha256 = sha256Hex(value);
    }

    public static PcapRawKafkaRecord fromConsumerRecord(ConsumerRecord<byte[], byte[]> record) {
        return new PcapRawKafkaRecord(Objects.requireNonNull(record, "consumer record is required"));
    }

    public String getTopic() { return topic; }
    public int getPartition() { return partition; }
    public long getOffset() { return offset; }
    public long getTimestamp() { return timestamp; }
    public String getTimestampType() { return timestampType; }
    public byte[] getKey() { return copy(key); }
    public byte[] getValue() { return copy(value); }
    public List<KafkaHeader> getHeaders() {
        List<KafkaHeader> result = new ArrayList<>(headers.size());
        for (KafkaHeader header : headers) {
            result.add(new KafkaHeader(header.getKey(), header.getValue()));
        }
        return Collections.unmodifiableList(result);
    }
    public String getRawSha256() { return rawSha256; }
    public String sourceIdentity() { return topic + ":" + partition + ":" + offset; }
    public String keyAsString() { return new String(key, StandardCharsets.UTF_8); }

    public List<String> headerValues(String name) {
        List<String> values = new ArrayList<>();
        for (KafkaHeader header : headers) {
            if (header.getKey().equalsIgnoreCase(name)) {
                values.add(new String(header.getValue(), StandardCharsets.UTF_8));
            }
        }
        return Collections.unmodifiableList(values);
    }

    public String requiredSingleHeader(String name) {
        List<String> values = headerValues(name);
        if (values.size() != 1 || values.get(0).trim().isEmpty()) {
            throw new IllegalArgumentException("Kafka header " + name + " must occur exactly once");
        }
        return values.get(0);
    }

    public String firstHeader(String name) {
        List<String> values = headerValues(name);
        return values.isEmpty() ? "" : values.get(0);
    }

    public String headersSha256() {
        MessageDigest digest = sha256();
        for (KafkaHeader header : headers) {
            digest.update(header.getKey().toLowerCase(Locale.ROOT).getBytes(StandardCharsets.UTF_8));
            digest.update((byte) 0);
            digest.update(header.getValue());
            digest.update((byte) 0);
        }
        return hex(digest.digest());
    }

    static String sha256Hex(byte[] bytes) {
        return hex(sha256().digest(bytes));
    }

    private static MessageDigest sha256() {
        try {
            return MessageDigest.getInstance("SHA-256");
        } catch (NoSuchAlgorithmException error) {
            throw new IllegalStateException("SHA-256 is unavailable", error);
        }
    }

    private static String hex(byte[] digest) {
        StringBuilder result = new StringBuilder(digest.length * 2);
        for (byte value : digest) result.append(String.format(Locale.ROOT, "%02x", value));
        return result.toString();
    }

    private static byte[] copy(byte[] bytes) { return bytes == null ? new byte[0] : bytes.clone(); }
    private static String requireText(String value, String field) {
        if (value == null || value.trim().isEmpty()) throw new IllegalArgumentException(field + " is required");
        return value;
    }

    /** Header order and duplicates are part of the source evidence. */
    public static final class KafkaHeader implements Serializable {
        private static final long serialVersionUID = 1L;
        private final String key;
        private final byte[] value;
        KafkaHeader(String key, byte[] value) { this.key = key; this.value = copy(value); }
        public String getKey() { return key; }
        public byte[] getValue() { return copy(value); }
    }
}
