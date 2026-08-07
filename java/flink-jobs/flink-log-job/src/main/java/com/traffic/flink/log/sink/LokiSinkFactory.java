package com.traffic.flink.log.sink;

import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.state.ListState;
import org.apache.flink.api.common.state.ListStateDescriptor;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.runtime.state.FunctionInitializationContext;
import org.apache.flink.runtime.state.FunctionSnapshotContext;
import org.apache.flink.streaming.api.checkpoint.CheckpointedFunction;
import org.apache.flink.streaming.api.functions.sink.RichSinkFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/** Loki sink with checkpointed retry buffering and ACK-before-clear semantics. */
public class LokiSinkFactory {
    private static final Logger LOG = LoggerFactory.getLogger(LokiSinkFactory.class);

    public static LokiSink createSink() {
        return new LokiSink();
    }

    public static class LokiSink extends RichSinkFunction<DeviceLog> implements CheckpointedFunction {
        private static final long serialVersionUID = 1L;
        private static final String STATE_NAME = "loki-pending-streams-v1";

        private transient List<String> pendingStreams;
        private transient ListState<String> pendingState;

        private String lokiUrl;
        private int batchSize;
        private int maxRetries;
        private int connectTimeoutMs;
        private int readTimeoutMs;

        public LokiSink() {
        }

        LokiSink(String lokiUrl, int batchSize, int maxRetries, int connectTimeoutMs, int readTimeoutMs) {
            this.lokiUrl = lokiUrl;
            this.batchSize = batchSize;
            this.maxRetries = maxRetries;
            this.connectTimeoutMs = connectTimeoutMs;
            this.readTimeoutMs = readTimeoutMs;
        }

        @Override
        public void open(Configuration params) {
            if (pendingStreams == null) {
                pendingStreams = new ArrayList<>();
            }
            if (isBlank(lokiUrl)) {
                lokiUrl = System.getenv().getOrDefault(
                        "LOKI_URL", "http://loki.observability.svc:3100");
            }
            if (batchSize <= 0) {
                batchSize = positiveEnv("LOKI_BATCH_SIZE", 100);
            }
            if (maxRetries <= 0) {
                maxRetries = positiveEnv("LOKI_MAX_RETRIES", 3);
            }
            if (connectTimeoutMs <= 0) {
                connectTimeoutMs = positiveEnv("LOKI_CONNECT_TIMEOUT_MS", 5_000);
            }
            if (readTimeoutMs <= 0) {
                readTimeoutMs = positiveEnv("LOKI_READ_TIMEOUT_MS", 30_000);
            }
        }

        @Override
        public void invoke(DeviceLog log, Context ctx) throws Exception {
            pendingStreams.add(streamEntry(log));
            if (pendingStreams.size() >= batchSize) {
                flush();
            }
        }

        @Override
        public void snapshotState(FunctionSnapshotContext context) throws Exception {
            // A successful checkpoint may not advance beyond an unacknowledged
            // HTTP side effect. A failed push fails the checkpoint and leaves
            // the in-memory retry buffer intact.
            flush();
            pendingState.clear();
            for (String stream : pendingStreams) {
                pendingState.add(stream);
            }
        }

        @Override
        public void initializeState(FunctionInitializationContext context) throws Exception {
            pendingState = context.getOperatorStateStore().getListState(
                    new ListStateDescriptor<>(STATE_NAME, String.class));
            pendingStreams = new ArrayList<>();
            if (context.isRestored()) {
                for (String stream : pendingState.get()) {
                    pendingStreams.add(stream);
                }
            }
        }

        @Override
        public void close() throws Exception {
            flush();
            super.close();
        }

        synchronized void flush() throws Exception {
            if (pendingStreams == null || pendingStreams.isEmpty()) {
                return;
            }

            List<String> attempt = new ArrayList<>(pendingStreams);
            byte[] body = payload(attempt).getBytes(StandardCharsets.UTF_8);
            Exception lastFailure = null;
            for (int retry = 1; retry <= maxRetries; retry++) {
                try {
                    int status = pushOnce(body);
                    if (status >= 200 && status < 300) {
                        pendingStreams.subList(0, attempt.size()).clear();
                        LOG.debug("Loki acknowledged {} log streams", attempt.size());
                        return;
                    }
                    lastFailure = new IOException("Loki push returned HTTP " + status);
                } catch (Exception e) {
                    lastFailure = e;
                }

                LOG.warn("Loki push failed (attempt {}/{}); retaining {} streams: {}",
                        retry, maxRetries, attempt.size(), lastFailure.getMessage());
                if (retry < maxRetries) {
                    try {
                        Thread.sleep(Math.min(2_000L, 100L << (retry - 1)));
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        throw new IOException("Interrupted while retrying Loki push", e);
                    }
                }
            }
            throw new IOException(
                    "Loki push failed after " + maxRetries + " attempts; retry buffer retained",
                    lastFailure);
        }

        protected int pushOnce(byte[] body) throws IOException {
            HttpURLConnection connection = (HttpURLConnection) new URL(
                    stripTrailingSlash(lokiUrl) + "/loki/api/v1/push").openConnection();
            connection.setRequestMethod("POST");
            connection.setRequestProperty("Content-Type", "application/json");
            connection.setConnectTimeout(connectTimeoutMs);
            connection.setReadTimeout(readTimeoutMs);
            connection.setDoOutput(true);
            try {
                try (OutputStream output = connection.getOutputStream()) {
                    output.write(body);
                }
                int status = connection.getResponseCode();
                InputStream response = status >= 400
                        ? connection.getErrorStream()
                        : connection.getInputStream();
                if (response != null) {
                    try (InputStream ignored = response) {
                        byte[] drain = new byte[1024];
                        while (ignored.read(drain) != -1) {
                            // Drain response so the HTTP connection can close cleanly.
                        }
                    }
                }
                return status;
            } finally {
                connection.disconnect();
            }
        }

        int pendingCount() {
            return pendingStreams == null ? 0 : pendingStreams.size();
        }
    }

    static String streamEntry(DeviceLog log) {
        if (log == null) {
            throw new IllegalArgumentException("DeviceLog must not be null");
        }
        if (log.getTimestamp() <= 0) {
            throw new IllegalArgumentException("DeviceLog timestamp must be positive");
        }
        final long timestampNanos;
        try {
            timestampNanos = Math.multiplyExact(log.getTimestamp(), 1_000_000L);
        } catch (ArithmeticException e) {
            throw new IllegalArgumentException("DeviceLog timestamp overflows Loki nanoseconds", e);
        }

        String line = "{"
                + "\"log_id\":" + jsonString(log.getLogId()) + ","
                + "\"tenant_id\":" + jsonString(log.getTenantId()) + ","
                + "\"timestamp\":" + log.getTimestamp() + ","
                + "\"message\":" + jsonString(log.getMessage()) + ","
                + "\"parsed\":" + jsonString(log.getParsed())
                + "}";

        return "{\"stream\":{"
                + "\"tenant_id\":" + jsonString(labelValue(log.getTenantId(), "unknown")) + ","
                + "\"device_ip\":" + jsonString(labelValue(log.getDeviceIp(), "unknown")) + ","
                + "\"device_type\":" + jsonString(labelValue(log.getDeviceType(), "unknown")) + ","
                + "\"severity\":" + jsonString(String.valueOf(log.getSeverity())) + ","
                + "\"source\":" + jsonString(labelValue(log.getSource(), "unknown"))
                + "},\"values\":[[" + jsonString(String.valueOf(timestampNanos)) + ","
                + jsonString(line) + "]]}";
    }

    static String payload(List<String> streams) {
        return "{\"streams\":[" + String.join(",", streams) + "]}";
    }

    private static String jsonString(String value) {
        if (value == null) {
            return "\"\"";
        }
        StringBuilder escaped = new StringBuilder(value.length() + 2).append('"');
        for (int i = 0; i < value.length(); i++) {
            char current = value.charAt(i);
            switch (current) {
                case '"': escaped.append("\\\""); break;
                case '\\': escaped.append("\\\\"); break;
                case '\b': escaped.append("\\b"); break;
                case '\f': escaped.append("\\f"); break;
                case '\n': escaped.append("\\n"); break;
                case '\r': escaped.append("\\r"); break;
                case '\t': escaped.append("\\t"); break;
                default:
                    if (current < 0x20) {
                        escaped.append(String.format("\\u%04x", (int) current));
                    } else {
                        escaped.append(current);
                    }
            }
        }
        return escaped.append('"').toString();
    }

    private static int positiveEnv(String name, int fallback) {
        String raw = System.getenv(name);
        if (isBlank(raw)) {
            return fallback;
        }
        try {
            int value = Integer.parseInt(raw);
            return value > 0 ? value : fallback;
        } catch (NumberFormatException e) {
            LOG.warn("Ignoring invalid {}={}", name, raw);
            return fallback;
        }
    }

    private static String labelValue(String value, String fallback) {
        return isBlank(value) ? fallback : value;
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    private static String stripTrailingSlash(String value) {
        int end = value.length();
        while (end > 0 && value.charAt(end - 1) == '/') {
            end--;
        }
        return value.substring(0, end);
    }
}
