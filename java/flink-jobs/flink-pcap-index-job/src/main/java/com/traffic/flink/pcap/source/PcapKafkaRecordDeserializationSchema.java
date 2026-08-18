package com.traffic.flink.pcap.source;

import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.connector.kafka.source.reader.deserializer.KafkaRecordDeserializationSchema;
import org.apache.flink.util.Collector;
import org.apache.kafka.clients.consumer.ConsumerRecord;

import java.io.IOException;

/** Converts each Kafka record to exactly one raw carrier without parsing bytes. */
public final class PcapKafkaRecordDeserializationSchema
        implements KafkaRecordDeserializationSchema<PcapRawKafkaRecord> {
    private static final long serialVersionUID = 1L;

    @Override
    public void deserialize(ConsumerRecord<byte[], byte[]> record, Collector<PcapRawKafkaRecord> out)
            throws IOException {
        if (record == null || out == null) throw new IOException("Kafka record and collector are required");
        try {
            out.collect(PcapRawKafkaRecord.fromConsumerRecord(record));
        } catch (RuntimeException error) {
            String coordinate = record.topic() + "/" + record.partition() + "/" + record.offset();
            throw new IOException("invalid raw PCAP Kafka record at " + coordinate, error);
        }
    }

    @Override
    public TypeInformation<PcapRawKafkaRecord> getProducedType() {
        return TypeInformation.of(PcapRawKafkaRecord.class);
    }
}
