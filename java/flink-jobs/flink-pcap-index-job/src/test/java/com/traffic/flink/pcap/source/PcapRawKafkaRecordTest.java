package com.traffic.flink.pcap.source;

import org.apache.flink.api.common.ExecutionConfig;
import org.apache.flink.api.java.typeutils.runtime.kryo.KryoSerializer;
import org.apache.flink.util.Collector;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.apache.kafka.common.header.internals.RecordHeaders;
import org.apache.kafka.common.record.TimestampType;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

import static org.junit.jupiter.api.Assertions.*;

class PcapRawKafkaRecordTest {
    @Test
    void preservesCoordinatesDuplicateHeadersAndDefensiveCopies() {
        byte[] key = "tenant-a:probe-a".getBytes();
        byte[] value = {1, 2, 3, 4};
        byte[] headerValue = "tenant-a".getBytes();
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", headerValue);
        headers.add("tenant_id", "tenant-shadow".getBytes());
        ConsumerRecord<byte[], byte[]> source = record(key, value, headers);
        PcapRawKafkaRecord raw = PcapRawKafkaRecord.fromConsumerRecord(source);

        key[0] = 'X'; value[0] = 9; headerValue[0] = 'X';
        assertEquals("pcap.index.v1", raw.getTopic());
        assertEquals(3, raw.getPartition());
        assertEquals(41, raw.getOffset());
        assertEquals("tenant-a:probe-a", raw.keyAsString());
        assertArrayEquals(new byte[]{1, 2, 3, 4}, raw.getValue());
        assertEquals(List.of("tenant-a", "tenant-shadow"), raw.headerValues("tenant_id"));
        assertThrows(IllegalArgumentException.class, () -> raw.requiredSingleHeader("tenant_id"));

        byte[] returned = raw.getValue();
        returned[1] = 8;
        assertArrayEquals(new byte[]{1, 2, 3, 4}, raw.getValue());
        assertEquals(raw.getRawSha256(), PcapRawKafkaRecord.sha256Hex(new byte[]{1, 2, 3, 4}));
        assertEquals("pcap.index.v1:3:41", raw.sourceIdentity());
    }

    @Test
    void deserializerCollectsExactlyOneCarrierAndRejectsStructuralFaults() throws Exception {
        PcapKafkaRecordDeserializationSchema schema = new PcapKafkaRecordDeserializationSchema();
        List<PcapRawKafkaRecord> output = new ArrayList<>();
        Collector<PcapRawKafkaRecord> collector = new ListCollector<>(output);
        schema.deserialize(record(new byte[]{1}, new byte[]{2}, new RecordHeaders()), collector);
        assertEquals(1, output.size());
        assertThrows(Exception.class, () -> schema.deserialize(null, collector));
        assertThrows(Exception.class, () -> schema.deserialize(
                new ConsumerRecord<>("pcap.index.v1", -1, 0, new byte[0], new byte[0]), collector));
    }

    @Test
    void flinkKryoCopyPreservesCarrierAndCannotMutateHeaderEvidence() {
        RecordHeaders headers = new RecordHeaders();
        headers.add("tenant_id", "tenant-a".getBytes());
        PcapRawKafkaRecord raw = PcapRawKafkaRecord.fromConsumerRecord(
                record("tenant-a:probe-a".getBytes(), new byte[]{1, 2, 3}, headers));

        KryoSerializer<PcapRawKafkaRecord> serializer = new KryoSerializer<>(
                PcapRawKafkaRecord.class, new ExecutionConfig());
        PcapRawKafkaRecord copied = serializer.copy(raw);

        assertEquals(raw.sourceIdentity(), copied.sourceIdentity());
        assertEquals(raw.getRawSha256(), copied.getRawSha256());
        assertEquals(List.of("tenant-a"), copied.headerValues("tenant_id"));
        assertThrows(UnsupportedOperationException.class,
                () -> copied.getHeaders().add(copied.getHeaders().get(0)));
        byte[] exposed = copied.getHeaders().get(0).getValue();
        exposed[0] = 'X';
        assertEquals(List.of("tenant-a"), copied.headerValues("tenant_id"));
    }

    static ConsumerRecord<byte[], byte[]> record(byte[] key, byte[] value, RecordHeaders headers) {
        return new ConsumerRecord<>("pcap.index.v1", 3, 41, 1_725_000_000_000L,
                TimestampType.CREATE_TIME, key == null ? 0 : key.length,
                value == null ? 0 : value.length, key, value, headers, Optional.of(7));
    }

    private static final class ListCollector<T> implements Collector<T> {
        private final List<T> values;
        private ListCollector(List<T> values) { this.values = values; }
        @Override public void collect(T record) { values.add(record); }
        @Override public void close() { }
    }
}
