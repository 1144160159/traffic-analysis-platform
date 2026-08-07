package com.traffic.flink.log.sink;

import com.google.gson.JsonArray;
import com.google.gson.JsonObject;
import com.google.gson.JsonParser;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.configuration.Configuration;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.util.Collections;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class LokiSinkFactoryTest {
    @Test
    void payloadIsValidLokiJsonWithEpochNanosecondsAndEscapedLine() {
        String stream = LokiSinkFactory.streamEntry(log("line\n\"quoted\"", 1_700_000_000_123L));
        JsonObject payload = JsonParser.parseString(
                LokiSinkFactory.payload(Collections.singletonList(stream))).getAsJsonObject();

        JsonArray streams = payload.getAsJsonArray("streams");
        assertEquals(1, streams.size());
        JsonObject first = streams.get(0).getAsJsonObject();
        assertEquals("tenant-a", first.getAsJsonObject("stream").get("tenant_id").getAsString());
        JsonArray value = first.getAsJsonArray("values").get(0).getAsJsonArray();
        assertEquals("1700000000123000000", value.get(0).getAsString());
        JsonObject line = JsonParser.parseString(value.get(1).getAsString()).getAsJsonObject();
        assertEquals("line\n\"quoted\"", line.get("message").getAsString());
        assertEquals("log-1", line.get("log_id").getAsString());
    }

    @Test
    void httpFailureThrowsAndRetainsRetryBuffer() throws Exception {
        LokiSinkFactory.LokiSink sink = new LokiSinkFactory.LokiSink(
                "http://unused", 10, 1, 100, 100) {
            @Override
            protected int pushOnce(byte[] body) {
                return 503;
            }
        };
        sink.open(new Configuration());
        sink.invoke(log("failure", 1_700_000_000_123L), null);

        IOException error = assertThrows(IOException.class, sink::flush);
        assertTrue(error.getMessage().contains("retry buffer retained"));
        assertEquals(1, sink.pendingCount());
    }

    @Test
    void bufferClearsOnlyAfterTwoHundredResponse() throws Exception {
        LokiSinkFactory.LokiSink sink = new LokiSinkFactory.LokiSink(
                "http://unused", 10, 1, 100, 100) {
            @Override
            protected int pushOnce(byte[] body) {
                return 204;
            }
        };
        sink.open(new Configuration());
        sink.invoke(log("ok", 1_700_000_000_123L), null);

        sink.flush();
        assertEquals(0, sink.pendingCount());
    }

    @Test
    void invalidTimestampFailsBeforeEnteringRetryBuffer() {
        assertThrows(IllegalArgumentException.class,
                () -> LokiSinkFactory.streamEntry(log("invalid", 0L)));
    }

    private static DeviceLog log(String message, long timestamp) {
        return DeviceLog.newBuilder()
                .setLogId("log-1")
                .setTenantId("tenant-a")
                .setDeviceIp("10.0.0.8")
                .setDeviceType("firewall")
                .setSeverity(4)
                .setTimestamp(timestamp)
                .setMessage(message)
                .setParsed("{\"source\":\"syslog\"}")
                .setSource("syslog")
                .build();
    }
}
