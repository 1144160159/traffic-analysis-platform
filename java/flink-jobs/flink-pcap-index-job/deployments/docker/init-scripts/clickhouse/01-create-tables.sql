-- =============================================================================
-- ClickHouse PCAP Index 表初始化脚本
-- =============================================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS traffic;

-- =============================================================================
-- pcap_index_local 表（本地表，用于开发测试）
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.pcap_index_local
(
    -- 租户与探针
    tenant_id       LowCardinality(String)  COMMENT '租户 ID',
    probe_id        LowCardinality(String)  COMMENT '探针 ID',
    
    -- 文件标识
    file_key        String                  COMMENT 'S3/MinIO 文件路径',
    
    -- 时间范围
    ts_start        DateTime64(3)           COMMENT 'PCAP 开始时间（毫秒精度）',
    ts_end          DateTime64(3)           COMMENT 'PCAP 结束时间（毫秒精度）',
    
    -- 文件元数据
    byte_size       UInt64                  COMMENT '文件大小（字节）',
    zstd_level      UInt8 DEFAULT 0         COMMENT 'Zstd 压缩级别（0=未压缩）',
    sha256          String DEFAULT ''       COMMENT '文件 SHA256 校验和',
    
    -- 流标识
    community_id    String DEFAULT ''       COMMENT '主要 Community ID',
    flow_id         String DEFAULT ''       COMMENT 'Flow ID',
    
    -- 文件偏移（可选，用于大文件分片）
    offset_start    Nullable(UInt64)        COMMENT '偏移起始位置',
    offset_end      Nullable(UInt64)        COMMENT '偏移结束位置',
    
    -- 快速检索字段
    bloom_filter_b64 String DEFAULT ''      COMMENT 'IP 地址 BloomFilter（Base64 编码）',
    community_ids   Array(String) DEFAULT [] COMMENT 'Community ID 列表（最多 1000 个）',
    
    -- 创建时间
    created_ts      DateTime64(3) DEFAULT now64(3) COMMENT '记录创建时间'
)
ENGINE = ReplacingMergeTree(created_ts)
PARTITION BY toYYYYMMDD(ts_start)
ORDER BY (tenant_id, probe_id, file_key, ts_start)
TTL ts_start + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- 索引优化
-- =============================================================================

-- Community ID 索引（用于取证查询）
ALTER TABLE traffic.pcap_index_local 
ADD INDEX IF NOT EXISTS idx_community_id community_id TYPE bloom_filter(0.01) GRANULARITY 4;

-- 时间范围索引
ALTER TABLE traffic.pcap_index_local 
ADD INDEX IF NOT EXISTS idx_ts_range (ts_start, ts_end) TYPE minmax GRANULARITY 4;

-- 文件大小索引
ALTER TABLE traffic.pcap_index_local 
ADD INDEX IF NOT EXISTS idx_byte_size byte_size TYPE minmax GRANULARITY 4;

-- =============================================================================
-- 物化视图：按小时统计
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS traffic.pcap_index_hourly_stats
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (tenant_id, probe_id, hour)
AS SELECT
    tenant_id,
    probe_id,
    toStartOfHour(ts_start) AS hour,
    count() AS file_count,
    sum(byte_size) AS total_bytes,
    avg(byte_size) AS avg_file_size,
    max(byte_size) AS max_file_size,
    sum(length(community_ids)) AS total_community_ids
FROM traffic.pcap_index_local
GROUP BY tenant_id, probe_id, hour;

-- =============================================================================
-- 物化视图：按天统计
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS traffic.pcap_index_daily_stats
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day)
AS SELECT
    tenant_id,
    toDate(ts_start) AS day,
    count() AS file_count,
    sum(byte_size) AS total_bytes,
    uniqExact(probe_id) AS probe_count
FROM traffic.pcap_index_local
GROUP BY tenant_id, day;

-- =============================================================================
-- 测试数据（可选）
-- =============================================================================
-- INSERT INTO traffic.pcap_index_local (
--     tenant_id, probe_id, file_key, ts_start, ts_end, 
--     byte_size, community_id, community_ids
-- ) VALUES (
--     'tenant-001', 'probe-001', 
--     's3://pcap-bucket/2024/01/15/capture-001.pcap.zst',
--     now64(3), now64(3) + 60000,
--     1073741824, '1:abc123', ['1:abc123', '1:def456']
-- );
