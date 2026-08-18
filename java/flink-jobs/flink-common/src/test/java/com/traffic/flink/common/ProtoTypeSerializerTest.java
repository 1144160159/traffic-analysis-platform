package com.traffic.flink.common;

import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.core.memory.DataInputDeserializer;
import org.apache.flink.core.memory.DataOutputSerializer;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ProtoTypeSerializerTest {
    @Test
    void roundTripsGeneratedMessageWithoutReflectiveKryo() throws Exception {
        FeatureStat expected = FeatureStat.newBuilder()
                .setHeader(EventHeader.newBuilder()
                        .setTenantId("tenant-a")
                        .setEventId("event-a"))
                .setCommunityId("1:n012-canary")
                .setPps(42.0f)
                .setBps(8192.0f)
                .build();
        ProtoTypeSerializer<FeatureStat> serializer =
                new ProtoTypeSerializer<>(FeatureStat.class);
        DataOutputSerializer output = new DataOutputSerializer(256);
        serializer.serialize(expected, output);

        FeatureStat actual = serializer.deserialize(
                new DataInputDeserializer(output.getCopyOfBuffer()));
        assertEquals(expected, actual);
        assertTrue(serializer.snapshotConfiguration()
                .resolveSchemaCompatibility(new ProtoTypeSerializer<>(FeatureStat.class))
                .isCompatibleAsIs());
    }
}
