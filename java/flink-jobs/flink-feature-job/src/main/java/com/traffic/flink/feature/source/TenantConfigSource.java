package com.traffic.flink.feature.source;

import com.traffic.flink.feature.config.TenantConfig;
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
 * 租户配置源（从 PostgreSQL 加载）
 * 
 * 定期轮询 tenant_config 表，发现变更时发送到 BroadcastStream
 */
public class TenantConfigSource extends RichSourceFunction<TenantConfig> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(TenantConfigSource.class);

    private final String jdbcUrl;
    private final String username;
    private final String password;
    private final long pollIntervalMs;

    private volatile boolean running = true;
    private transient Connection connection;

    // 缓存已加载的配置（用于检测变更）
    private transient Map<String, Long> configVersions;

    public TenantConfigSource(String jdbcUrl, String username, String password, long pollIntervalMs) {
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
        
        LOG.info("TenantConfigSource initialized: url={}, pollInterval={}ms", jdbcUrl, pollIntervalMs);
    }

    @Override
    public void run(SourceContext<TenantConfig> ctx) throws Exception {
        while (running) {
            try {
                // 查询 tenant_config 表
                String sql = "SELECT t.tenant_id, " +
                             "       COALESCE((tc.config_value->>'priority')::int, 10) AS priority, " +
                             "       COALESCE((tc.config_value->>'enable_l2')::boolean, true) AS enable_l2, " +
                             "       COALESCE((tc.config_value->>'sampling_rate')::float, 1.0) AS sampling_rate, " +
                             "       COALESCE((tc.config_value->>'max_events_per_second')::int, -1) AS max_eps, " +
                             "       COALESCE((tc.config_value->>'enable_degradation')::boolean, false) AS enable_degradation, " +
                             "       COALESCE(tc.updated_at, t.updated_at) AS updated_at " +
                             "FROM tenants t " +
                             "LEFT JOIN tenant_config tc ON t.tenant_id = tc.tenant_id AND tc.config_key = 'feature_job' " +
                             "WHERE t.status = 'active'";

                try (PreparedStatement stmt = connection.prepareStatement(sql);
                     ResultSet rs = stmt.executeQuery()) {

                    while (rs.next()) {
                        String tenantId = rs.getString("tenant_id");
                        int priority = rs.getInt("priority");
                        boolean enableL2 = rs.getBoolean("enable_l2");
                        float samplingRate = rs.getFloat("sampling_rate");
                        int maxEps = rs.getInt("max_eps");
                        boolean enableDegradation = rs.getBoolean("enable_degradation");
                        long updatedAt = rs.getTimestamp("updated_at").getTime();

                        // 检测配置是否变更
                        Long cachedVersion = configVersions.get(tenantId);
                        if (cachedVersion == null || cachedVersion < updatedAt) {
                            // 配置有更新，发送到 BroadcastStream
                            TenantConfig config = new TenantConfig(tenantId);
                            config.setPriority(priority);
                            config.setEnableL2(enableL2);
                            config.setSamplingRate(samplingRate);
                            config.setMaxEventsPerSecond(maxEps);
                            config.setEnableDegradation(enableDegradation);
                            config.setUpdatedAt(updatedAt);

                            synchronized (ctx.getCheckpointLock()) {
                                ctx.collect(config);
                            }
                            
                            configVersions.put(tenantId, updatedAt);
                            LOG.info("Tenant config updated: {}", config);
                        }
                    }
                }

                // 休眠
                Thread.sleep(pollIntervalMs);

            } catch (Exception e) {
                LOG.error("Failed to load tenant config: {}", e.getMessage(), e);
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
}
