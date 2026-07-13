-- ==============================================================================
-- ClickHouse 初始化脚本 - Session 表
-- ==============================================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS traffic;

-- 使用数据库
USE traffic;

-- ==============================================================================
-- 本地表（sessions_local）
-- ==============================================================================
CREATE TABLE IF NOT EXISTS sessions_local ON CLUSTER '{cluster}'
(
    -- 租户与标识
    tenant_id            String,
    run_id               String,
    feature_set_id       String,
    event_id             String,
    
    -- Session 标识
    session_id           String,
    community_id         String,
    
    -- 时间范围
    ts_start             DateTime64(3, 'UTC'),
    ts_end               DateTime64(3, 'UTC'),
    duration_ms          UInt32,
    
    -- 网络信息
    protocol             UInt8,
    client_ip            String,
    server_ip            String,
    client_port          UInt16,
    server_port          UInt16,
    
    -- 流量统计
    packets_total        UInt64,
    bytes_total          UInt64,
    bytes_up             UInt64,  -- client → server
    bytes_down           UInt64,  -- server → client
    up_down_ratio        Float32,
    
    -- 包长统计
    num_pkts             UInt32,
    avg_payload          Float32,
    min_payload          UInt32,
    max_payload          UInt32,
    std_payload          Float32,
    
    -- IAT 统计
    mean_iat_ms          Float32,
    min_iat_ms           Float32,
    max_iat_ms           Float32,
    std_iat_ms           Float32,
    
    -- TCP 标志
    flags_syn            UInt8,
    flags_ack            UInt8,
    flags_fin            UInt8,
    flags_psh            UInt8,
    flags_rst            UInt8,
    
    -- 协议统计
    dns_pkt_cnt          UInt32,
    tcp_pkt_cnt          UInt32,
    udp_pkt_cnt          UInt32,
    icmp_pkt_cnt         UInt32,
    
    -- 证据数量
    evidence_count       UInt32,
    
    -- 结束原因
    end_reason           String,
    
    -- 摄入时间戳
    ingest_ts            DateTime64(3, 'UTC')
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/sessions_local', '{replica}')
PARTITION BY toYYYYMMDD(ts_start)
ORDER BY (tenant_id, ts_start, community_id, session_id)
TTL ts_start + INTERVAL 30 DAY
SETTINGS 
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- ==============================================================================
-- 分布式表（sessions_dist）
-- ==============================================================================
CREATE TABLE IF NOT EXISTS sessions_dist ON CLUSTER '{cluster}' AS sessions_local
ENGINE = Distributed('{cluster}', traffic, sessions_local, cityHash64(tenant_id, community_id));

-- ==============================================================================
-- 物化视图 1: 按小时聚合的统计
-- ==============================================================================
CREATE TABLE IF NOT EXISTS sessions_hourly_stats_local ON CLUSTER '{cluster}'
(
    tenant_id            String,
    hour                 DateTime,
    protocol             UInt8,
    
    -- 聚合统计
    session_count        UInt64,
    total_packets        UInt64,
    total_bytes          UInt64,
    avg_duration_ms      Float64,
    avg_bytes_per_session Float64,
    
    -- 协议分布
    tcp_sessions         UInt64,
    udp_sessions         UInt64,
    icmp_sessions        UInt64,
    
    -- TCP 标志统计
    syn_count            UInt64,
    fin_count            UInt64,
    rst_count            UInt64
)
ENGINE = ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/sessions_hourly_stats_local', '{replica}')
PARTITION BY toYYYYMMDD(hour)
ORDER BY (tenant_id, hour, protocol)
TTL hour + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS sessions_hourly_stats_mv ON CLUSTER '{cluster}'
TO sessions_hourly_stats_local
AS SELECT
    tenant_id,
    toStartOfHour(ts_start) AS hour,
    protocol,
    count() AS session_count,
    sum(packets_total) AS total_packets,
    sum(bytes_total) AS total_bytes,
    avg(duration_ms) AS avg_duration_ms,
    avg(bytes_total) AS avg_bytes_per_session,
    countIf(protocol = 6) AS tcp_sessions,
    countIf(protocol = 17) AS udp_sessions,
    countIf(protocol = 1) AS icmp_sessions,
    sumIf(flags_syn, flags_syn > 0) AS syn_count,
    sumIf(flags_fin, flags_fin > 0) AS fin_count,
    sumIf(flags_rst, flags_rst > 0) AS rst_count
FROM sessions_local
GROUP BY tenant_id, hour, protocol;

-- ==============================================================================
-- 物化视图 2: Top IP 统计
-- ==============================================================================
CREATE TABLE IF NOT EXISTS sessions_top_ips_local ON CLUSTER '{cluster}'
(
    tenant_id            String,
    date                 Date,
    ip                   String,
    ip_type              Enum8('client' = 1, 'server' = 2),
    
    session_count        UInt64,
    total_bytes_up       UInt64,
    total_bytes_down     UInt64,
    avg_duration_ms      Float64
)
ENGINE = ReplicatedSummingMergeTree('/clickhouse/tables/{shard}/sessions_top_ips_local', '{replica}')
PARTITION BY date
ORDER BY (tenant_id, date, ip_type, session_count)
TTL date + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS sessions_top_client_ips_mv ON CLUSTER '{cluster}'
TO sessions_top_ips_local
AS SELECT
    tenant_id,
    toDate(ts_start) AS date,
    client_ip AS ip,
    'client' AS ip_type,
    count() AS session_count,
    sum(bytes_up) AS total_bytes_up,
    sum(bytes_down) AS total_bytes_down,
    avg(duration_ms) AS avg_duration_ms
FROM sessions_local
GROUP BY tenant_id, date, client_ip;

CREATE MATERIALIZED VIEW IF NOT EXISTS sessions_top_server_ips_mv ON CLUSTER '{cluster}'
TO sessions_top_ips_local
AS SELECT
    tenant_id,
    toDate(ts_start) AS date,
    server_ip AS ip,
    'server' AS ip_type,
    count() AS session_count,
    sum(bytes_up) AS total_bytes_up,
    sum(bytes_down) AS total_bytes_down,
    avg(duration_ms) AS avg_duration_ms
FROM sessions_local
GROUP BY tenant_id, date, server_ip;

-- ==============================================================================
-- 索引优化
-- ==============================================================================

-- Bloom Filter 索引（加速 IP 查询）
ALTER TABLE sessions_local ON CLUSTER '{cluster}'
ADD INDEX idx_client_ip (client_ip) TYPE bloom_filter GRANULARITY 4;

ALTER TABLE sessions_local ON CLUSTER '{cluster}'
ADD INDEX idx_server_ip (server_ip) TYPE bloom_filter GRANULARITY 4;

ALTER TABLE sessions_local ON CLUSTER '{cluster}'
ADD INDEX idx_community_id (community_id) TYPE bloom_filter GRANULARITY 4;

-- ==============================================================================
-- 查询优化设置
-- ==============================================================================
-- 设置默认查询参数
SET max_threads = 8;
SET max_memory_usage = 10000000000;
SET max_execution_time = 300;
