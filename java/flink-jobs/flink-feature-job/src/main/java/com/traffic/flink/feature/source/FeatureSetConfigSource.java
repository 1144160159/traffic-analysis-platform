package com.traffic.flink.feature.source;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.feature.config.FeatureSetConfig;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.source.RichSourceFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.HashSet;
import java.util.HashMap;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Set;
import java.util.TreeMap;

/**
 * Feature Set 配置源（从 PostgreSQL 加载）
 * 
 * 定期轮询 feature_sets 表，发现变更时发送到 BroadcastStream
 */
public class FeatureSetConfigSource extends RichSourceFunction<FeatureSetConfig> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FeatureSetConfigSource.class);
    private static final ObjectMapper JSON = new ObjectMapper();

    private final String jdbcUrl;
    private final String username;
    private final String password;
    private final long pollIntervalMs;

    private volatile boolean running = true;
    private transient Connection connection;

    // 缓存已加载的配置（用于检测变更）
    private transient Map<String, String> configVersions;

    public FeatureSetConfigSource(String jdbcUrl, String username, String password, long pollIntervalMs) {
        if (pollIntervalMs <= 0) {
            throw new IllegalArgumentException("pollIntervalMs must be greater than zero");
        }
        this.jdbcUrl = jdbcUrl;
        this.username = username;
        this.password = password;
        this.pollIntervalMs = pollIntervalMs;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);
        
        // 加载 JDBC 驱动
        Class.forName("org.postgresql.Driver");
        
        // 建立连接
        this.connection = DriverManager.getConnection(jdbcUrl, username, password);
        this.configVersions = new HashMap<>();
        
        LOG.info("FeatureSetConfigSource initialized: url={}, pollInterval={}ms", jdbcUrl, pollIntervalMs);
    }

    @Override
    public void run(SourceContext<FeatureSetConfig> ctx) throws Exception {
        while (running) {
            try {
                // 查询 feature_sets 表
                String sql = "SELECT feature_set_id, schema_version, params, status, updated_at " +
                             "FROM feature_sets";

                Set<String> seenFeatureSetIds = new HashSet<>();

                try (PreparedStatement stmt = connection.prepareStatement(sql);
                     ResultSet rs = stmt.executeQuery()) {

                    while (rs.next()) {
                        String featureSetId = rs.getString("feature_set_id");
                        String schemaVersion = rs.getString("schema_version");
                        String description = null;
                        String paramsJson = rs.getString("params");
                        String status = rs.getString("status");
                        long updatedAt = rs.getTimestamp("updated_at").getTime();
                        seenFeatureSetIds.add(featureSetId);

                        // 检测配置是否变更
                        String fingerprint = configFingerprint(
                                schemaVersion, description, paramsJson, status, updatedAt);
                        if (!fingerprint.equals(configVersions.get(featureSetId))) {
                            try {
                                FeatureSetConfig config;
                                if ("active".equals(status)) {
                                    config = parseConfig(
                                            featureSetId, schemaVersion, description, paramsJson, updatedAt);
                                } else {
                                    config = tombstone(featureSetId, schemaVersion, updatedAt);
                                }

                                synchronized (ctx.getCheckpointLock()) {
                                    ctx.collect(config);
                                }

                                // 只有成功解析并发送后才推进版本；坏配置不得覆盖上一版。
                                configVersions.put(featureSetId, fingerprint);
                                LOG.info("Feature Set config updated: {}", config);
                            } catch (IllegalArgumentException e) {
                                LOG.error(
                                        "Rejected feature set config: featureSetId={}, schemaVersion={}, updatedAt={}, error={}",
                                        featureSetId, schemaVersion, updatedAt, e.getMessage());
                            }
                        }
                    }
                }

                // 硬删除的特征集也必须从 BroadcastState 中移除。
                Map<String, String> removed = new LinkedHashMap<>();
                for (Map.Entry<String, String> cached : configVersions.entrySet()) {
                    if (!seenFeatureSetIds.contains(cached.getKey())) {
                        removed.put(cached.getKey(), cached.getValue());
                    }
                }
                for (String removedId : removed.keySet()) {
                    synchronized (ctx.getCheckpointLock()) {
                        ctx.collect(tombstone(removedId, "", System.currentTimeMillis()));
                    }
                    configVersions.remove(removedId);
                    LOG.info("Feature Set config removed: featureSetId={}", removedId);
                }

                // 休眠
                Thread.sleep(pollIntervalMs);

            } catch (Exception e) {
                LOG.error("Failed to load feature set config: {}", e.getMessage(), e);
                // 连接可能已失效：关闭后重建，否则断连后配置热更新永久静默失效
                closeQuietly();
                try {
                    this.connection = DriverManager.getConnection(jdbcUrl, username, password);
                    LOG.info("Feature set config JDBC connection re-established");
                } catch (Exception reconnectError) {
                    LOG.error("Feature set config JDBC reconnect failed: {}", reconnectError.getMessage());
                }
                // 失败后等待更长时间再重试
                Thread.sleep(pollIntervalMs * 5);
            }
        }
    }

    private void closeQuietly() {
        if (connection != null) {
            try {
                connection.close();
            } catch (Exception ignored) {
                // best-effort close
            }
            connection = null;
        }
    }

    @Override
    public void cancel() {
        running = false;
    }

    @Override
    public void close() throws Exception {
        super.close();
        if (connection != null && !connection.isClosed()) {
            connection.close();
        }
    }

    /**
     * 解析配置 JSON
     */
    FeatureSetConfig parseConfig(
            String featureSetId,
            String schemaVersion,
            String description,
            String paramsJson,
            long updatedAt
    ) {
        requireNonBlank(featureSetId, "feature_set_id");
        requireNonBlank(schemaVersion, "schema_version");

        final JsonNode params;
        try {
            params = JSON.readTree(paramsJson == null ? "{}" : paramsJson);
        } catch (JsonProcessingException e) {
            throw new IllegalArgumentException("params must be valid JSON", e);
        }
        if (params == null || !params.isObject()) {
            throw new IllegalArgumentException("params must be a JSON object");
        }

        FeatureSetConfig config = new FeatureSetConfig(featureSetId, schemaVersion);
        config.setActive(true);
        config.setDescription(description);
        config.setUpdatedAt(updatedAt);

        config.setIatThresholdMs(readFiniteFloat(params, "iat_threshold_ms", 1000.0f, true));
        config.setEnableL2Trigger(readBoolean(params, "enable_l2_trigger", false));

        FeatureSetConfig.L2TriggerThresholds thresholds = new FeatureSetConfig.L2TriggerThresholds();
        JsonNode l2 = params.path("l2_thresholds");
        if (!l2.isMissingNode() && !l2.isObject()) {
            throw new IllegalArgumentException("l2_thresholds must be a JSON object");
        }
        thresholds.setHighPpsThreshold(readFiniteFloat(l2, "high_pps_threshold", 10000.0f, false));
        thresholds.setHighBpsThreshold(readFiniteFloat(l2, "high_bps_threshold", 1e9f, false));
        thresholds.setEncryptedStdPayloadThreshold(
                readFiniteFloat(l2, "encrypted_std_payload_threshold", 100.0f, false));
        thresholds.setTlsPort(readPort(l2, "tls_port", 443));
        thresholds.setHttpPort(readPort(l2, "http_port", 80));
        config.setL2Thresholds(thresholds);

        JsonNode weights = params.path("feature_weights");
        if (!weights.isMissingNode()) {
            if (!weights.isObject()) {
                throw new IllegalArgumentException("feature_weights must be a JSON object");
            }
            Map<String, Float> parsedWeights = new TreeMap<>();
            Iterator<Map.Entry<String, JsonNode>> fields = weights.fields();
            while (fields.hasNext()) {
                Map.Entry<String, JsonNode> field = fields.next();
                if (field.getKey().trim().isEmpty()) {
                    throw new IllegalArgumentException("feature_weights keys must be non-blank");
                }
                parsedWeights.put(
                        field.getKey(),
                        requireFiniteFloat(field.getValue(), "feature_weights." + field.getKey(), false));
            }
            config.setFeatureWeights(parsedWeights);
        }

        return config;
    }

    private static FeatureSetConfig tombstone(String featureSetId, String schemaVersion, long updatedAt) {
        FeatureSetConfig config = new FeatureSetConfig(featureSetId, schemaVersion);
        config.setActive(false);
        config.setUpdatedAt(updatedAt);
        return config;
    }

    private static String configFingerprint(
            String schemaVersion, String description, String paramsJson, String status, long updatedAt) {
        return String.valueOf(schemaVersion) + '\u0000'
                + String.valueOf(description) + '\u0000'
                + String.valueOf(paramsJson) + '\u0000'
                + String.valueOf(status) + '\u0000'
                + updatedAt;
    }

    private static boolean readBoolean(JsonNode object, String field, boolean defaultValue) {
        JsonNode value = object.path(field);
        if (value.isMissingNode()) {
            return defaultValue;
        }
        if (!value.isBoolean()) {
            throw new IllegalArgumentException(field + " must be a boolean");
        }
        return value.booleanValue();
    }

    private static float readFiniteFloat(
            JsonNode object, String field, float defaultValue, boolean strictlyPositive) {
        JsonNode value = object.path(field);
        if (value.isMissingNode()) {
            return defaultValue;
        }
        return requireFiniteFloat(value, field, strictlyPositive);
    }

    private static float requireFiniteFloat(JsonNode value, String field, boolean strictlyPositive) {
        if (!value.isNumber()) {
            throw new IllegalArgumentException(field + " must be a number");
        }
        double parsed = value.doubleValue();
        if (!Double.isFinite(parsed) || parsed > Float.MAX_VALUE || parsed < -Float.MAX_VALUE) {
            throw new IllegalArgumentException(field + " must be a finite float");
        }
        if ((strictlyPositive && parsed <= 0.0d) || (!strictlyPositive && parsed < 0.0d)) {
            throw new IllegalArgumentException(
                    field + (strictlyPositive ? " must be greater than zero" : " must not be negative"));
        }
        return (float) parsed;
    }

    private static int readPort(JsonNode object, String field, int defaultValue) {
        JsonNode value = object.path(field);
        if (value.isMissingNode()) {
            return defaultValue;
        }
        if (!value.canConvertToInt()) {
            throw new IllegalArgumentException(field + " must be an integer");
        }
        int port = value.intValue();
        if (port < 1 || port > 65535) {
            throw new IllegalArgumentException(field + " must be between 1 and 65535");
        }
        return port;
    }

    private static void requireNonBlank(String value, String field) {
        if (value == null || value.trim().isEmpty()) {
            throw new IllegalArgumentException(field + " must be non-blank");
        }
    }
}
