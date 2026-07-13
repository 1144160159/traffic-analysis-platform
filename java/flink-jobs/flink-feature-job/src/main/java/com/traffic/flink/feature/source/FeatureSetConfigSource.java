package com.traffic.flink.feature.source;

import com.traffic.flink.feature.config.FeatureSetConfig;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.source.RichSourceFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.HashMap;
import java.util.Map;

/**
 * Feature Set 配置源（从 PostgreSQL 加载）
 * 
 * 定期轮询 feature_sets 表，发现变更时发送到 BroadcastStream
 */
public class FeatureSetConfigSource extends RichSourceFunction<FeatureSetConfig> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(FeatureSetConfigSource.class);

    private final String jdbcUrl;
    private final String username;
    private final String password;
    private final long pollIntervalMs;

    private volatile boolean running = true;
    private transient Connection connection;

    // 缓存已加载的配置（用于检测变更）
    private transient Map<String, Long> configVersions;

    public FeatureSetConfigSource(String jdbcUrl, String username, String password, long pollIntervalMs) {
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
                String sql = "SELECT feature_set_id, schema_version, params, updated_at " +
                             "FROM feature_sets " +
                             "WHERE status = 'active'";

                try (PreparedStatement stmt = connection.prepareStatement(sql);
                     ResultSet rs = stmt.executeQuery()) {

                    while (rs.next()) {
                        String featureSetId = rs.getString("feature_set_id");
                        String schemaVersion = rs.getString("schema_version");
                        String paramsJson = rs.getString("params");
                        long updatedAt = rs.getTimestamp("updated_at").getTime();

                        // 检测配置是否变更
                        Long cachedVersion = configVersions.get(featureSetId);
                        if (cachedVersion == null || cachedVersion < updatedAt) {
                            // 配置有更新，发送到 BroadcastStream
                            FeatureSetConfig config = parseConfig(featureSetId, schemaVersion, paramsJson, updatedAt);
                            
                            synchronized (ctx.getCheckpointLock()) {
                                ctx.collect(config);
                            }
                            
                            configVersions.put(featureSetId, updatedAt);
                            LOG.info("Feature Set config updated: {}", config);
                        }
                    }
                }

                // 休眠
                Thread.sleep(pollIntervalMs);

            } catch (Exception e) {
                LOG.error("Failed to load feature set config: {}", e.getMessage(), e);
                // 失败后等待更长时间再重试
                Thread.sleep(pollIntervalMs * 5);
            }
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
    private FeatureSetConfig parseConfig(String featureSetId, String schemaVersion, String paramsJson, long updatedAt) {
        FeatureSetConfig config = new FeatureSetConfig(featureSetId, schemaVersion);
        config.setUpdatedAt(updatedAt);

        // 简化实现：直接使用默认值
        // 生产环境应使用 Jackson/Gson 解析 params JSON
        config.setIatThresholdMs(1000.0f);
        config.setEnableL2Trigger(true);

        FeatureSetConfig.L2TriggerThresholds thresholds = new FeatureSetConfig.L2TriggerThresholds();
        thresholds.setHighPpsThreshold(10000.0f);
        thresholds.setHighBpsThreshold(1e9f);
        thresholds.setEncryptedStdPayloadThreshold(100.0f);
        config.setL2Thresholds(thresholds);

        return config;
    }
}