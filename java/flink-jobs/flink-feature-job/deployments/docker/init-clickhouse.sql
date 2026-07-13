-- =============================================================================
-- ClickHouse Initialization Script for Feature Job
-- 本地表定义（单节点开发环境）
-- =============================================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS traffic;

-- ==================== feature_stat_local 表 ====================
-- L1 统计特征表（本地 MergeTree）
CREATE TABLE IF NOT EXISTS traffic.feature_stat_local
(
    -- 基础字段
    tenant_id LowCardinality(String),
    run_id String,
    feature_set_id LowCardinality(String),
    schema_version LowCardinality(String),
    event_id String,
    
    -- 对象标识
    object_type LowCardinality(String),
    object_id String,
    community_id String,
    
    -- 时间
    ts DateTime64(3, 'UTC'),
    
    -- 协议与基础统计
    protocol UInt8,
    duration_ms UInt32,
    pps Float32,
    bps Float32,
    up_down_ratio Float32,
    
    -- 包长统计
    pktlen_mean Float32,
    pktlen_std Float32,
    
    -- IAT 统计
    iat_mean_ms Float32,
    iat_std_ms Float32,
    
    -- Active/Idle 统计
    active_mean_ms Float32,
    idle_mean_ms Float32,
    
    -- TCP Flags
    tcp_flag_syn_cnt UInt16,
    tcp_flag_ack_cnt UInt16,
    
    -- TCP 窗口
    tcp_init_win_bytes_fwd UInt32,
    tcp_init_win_bytes_bwd UInt32,
    
    -- 扩展字段（20 个槽位）
    extra Array(Float32),
    
    -- 摄入时间
    ingest_ts DateTime64(3, 'UTC'),
    
    -- 分区与排序优化字段
    ts_date Date MATERIALIZED toDate(ts),
    ts_hour DateTime MATERIALIZED toStartOfHour(ts)
)
ENGINE = MergeTree()
PARTITION BY (tenant_id, ts_date)
ORDER BY (tenant_id, community_id, ts)
TTL ts + INTERVAL 30 DAY
SETTINGS 
    index_granularity = 8192,
    min_bytes_for_wide_part = 0,
    min_rows_for_wide_part = 0;

-- ==================== 物化视图：按小时聚合 ====================
CREATE TABLE IF NOT EXISTS traffic.feature_stat_hourly
(
    tenant_id LowCardinality(String),
    ts_hour DateTime,
    protocol UInt8,
    
    -- 计数
    count UInt64,
    
    -- PPS 统计
    pps_sum Float64,
    pps_min Float32,
    pps_max Float32,
    
    -- BPS 统计
    bps_sum Float64,
    bps_min Float32,
    bps_max Float32,
    
    -- Duration 统计
    duration_sum UInt64,
    duration_min UInt32,
    duration_max UInt32,
    
    -- 包长统计
    pktlen_mean_sum Float64,
    pktlen_std_sum Float64,
    
    -- 特征分布（用于异常检测）
    high_pps_count UInt64,
    high_bps_count UInt64,
    zero_packets_count UInt64
)
ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMM(ts_hour))
ORDER BY (tenant_id, ts_hour, protocol)
TTL ts_hour + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS traffic.feature_stat_hourly_mv
TO traffic.feature_stat_hourly
AS SELECT
    tenant_id,
    ts_hour,
    protocol,
    count() AS count,
    sum(pps) AS pps_sum,
    min(pps) AS pps_min,
    max(pps) AS pps_max,
    sum(bps) AS bps_sum,
    min(bps) AS bps_min,
    max(bps) AS bps_max,
    sum(duration_ms) AS duration_sum,
    min(duration_ms) AS duration_min,
    max(duration_ms) AS duration_max,
    sum(pktlen_mean) AS pktlen_mean_sum,
    sum(pktlen_std) AS pktlen_std_sum,
    countIf(pps > 10000) AS high_pps_count,
    countIf(bps > 1e9) AS high_bps_count,
    countIf(duration_ms = 0) AS zero_packets_count
FROM traffic.feature_stat_local
GROUP BY tenant_id, ts_hour, protocol;

-- ==================== 索引 ====================
-- 二级索引（加速特定查询）
ALTER TABLE traffic.feature_stat_local
    ADD INDEX IF NOT EXISTS idx_object_id object_id TYPE bloom_filter(0.01) GRANULARITY 4;

ALTER TABLE traffic.feature_stat_local
    ADD INDEX IF NOT EXISTS idx_event_id event_id TYPE bloom_filter(0.01) GRANULARITY 4;

ALTER TABLE traffic.feature_stat_local
    ADD INDEX IF NOT EXISTS idx_pps pps TYPE minmax GRANULARITY 4;

ALTER TABLE traffic.feature_stat_local
    ADD INDEX IF NOT EXISTS idx_bps bps TYPE minmax GRANULARITY 4;

-- ==================== 系统设置（开发环境）====================
-- 注意：生产环境应在 config.xml 中配置
SYSTEM DROP MARK CACHE;

-- ==================== 验证 ====================
SELECT 'ClickHouse initialization completed' AS status;
SELECT name, engine FROM system.tables WHERE database = 'traffic';
