package com.traffic.flink.log.sink;

import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/** Loki Sink — 批量推送结构化日志到 Loki (HTTP) */
public class LokiSinkFactory {
    private static final Logger LOG = LoggerFactory.getLogger(LokiSinkFactory.class);

    public static LokiSink createSink() {
        return new LokiSink();
    }

    public static class LokiSink extends RichSinkFunction<DeviceLog> {
        private transient List<String> batch;
        private String lokiUrl;

        @Override public void open(Configuration params) {
            batch = new ArrayList<>();
            lokiUrl = System.getenv().getOrDefault("LOKI_URL", "http://loki.observability.svc:3100");
        }

        @Override public void invoke(DeviceLog log, Context ctx) {
            String ts = Instant.ofEpochMilli(log.getTimestamp()).toString();
            // Loki push format: {"streams":[{"stream":{"device_ip":"x","severity":"error"},"values":[["<nano>","<line>"]]}]}
            String entry = String.format(
                "{\"streams\":[{\"stream\":{\"device_ip\":\"%s\",\"device_type\":\"%s\",\"severity\":\"%d\",\"source\":\"%s\"}," +
                "\"values\":[[\"%s000000\",\"%s\"]]}]}",
                log.getDeviceIp(), log.getDeviceType(), log.getSeverity(), log.getSource(),
                ts, escapeJSON(log.getMessage()));
            batch.add(entry);
            if (batch.size() >= 100) flush();
        }

        @Override public void close() { flush(); }

        private void flush() {
            if (batch.isEmpty()) return;
            try {
                HttpURLConnection conn = (HttpURLConnection) new URL(lokiUrl + "/loki/api/v1/push").openConnection();
                conn.setRequestMethod("POST");
                conn.setRequestProperty("Content-Type", "application/json");
                conn.setDoOutput(true);
                byte[] body = ("{\"streams\":[" + String.join(",", batch) + "]}").getBytes(StandardCharsets.UTF_8);
                try (OutputStream os = conn.getOutputStream()) { os.write(body); }
                int code = conn.getResponseCode();
                if (code >= 400) LOG.warn("Loki push failed: HTTP {}", code);
                batch.clear();
            } catch (Exception e) { LOG.error("Loki flush error: {}", e.getMessage()); }
        }
    }

    private static String escapeJSON(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\").replace("\"", "\\\"").replace("\n", "\\n").replace("\r", "\\r");
    }
}
