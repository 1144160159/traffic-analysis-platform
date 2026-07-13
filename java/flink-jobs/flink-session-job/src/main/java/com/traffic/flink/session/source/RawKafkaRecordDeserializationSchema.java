package com.traffic.flink.session.source;

import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.connector.kafka.source.reader.deserializer.KafkaRecordDeserializationSchema;
import org.apache.flink.util.Collector;
import org.apache.kafka.clients.consumer.ConsumerRecord;

import java.io.IOException;

/**
 * Keeps Kafka key/value/headers/coordinates intact before FlowEvent parsing.
 */
public class RawKafkaRecordDeserializationSchema implements KafkaRecordDeserializationSchema<RawKafkaRecord> {

    private static final long serialVersionUID = 1L;

    @Override
    public void deserialize(ConsumerRecord<byte[], byte[]> record, Collector<RawKafkaRecord> out) throws IOException {
        out.collect(RawKafkaRecord.fromConsumerRecord(record));
    }

    @Override
    public TypeInformation<RawKafkaRecord> getProducedType() {
        return TypeInformation.of(RawKafkaRecord.class);
    }
}
