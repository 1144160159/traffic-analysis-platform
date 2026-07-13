-- ============================================================================
-- FILE: docker/clickhouse-init.sql
--
-- ClickHouse 初始化脚本
-- 创建 Behavior Detection Job 所需的表结构
-- ============================================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS traffic;

-- 切换到 traffic 数据库
USE traffic;

-- ============================================================================
-- 检测结果表（本地表）
-- ============================================================================

CREATE TABLE IF NOT EXISTS detections_behavior_local
(
    -- 租户与追踪
    tenant_id       String,
    run_id          String,
    feature_set_id  String,
    
    -- 模型信息
    model_version   LowCardinality(String),
    
    -- 事件标识
    event_id        String,
    community_id    String,
    
    -- 对象信息
    object_type     LowCardinality(String),
    object_id       String,
    
    -- 时间戳
    ts              DateTime64(3),
    
    -- 检测结果
    labels          Array(LowCardinality(String)),
    scores          Array(Float32),
    top_label       LowCardinality(String),
    top_score       Float32,
    
    -- 摄入时间
    ingest_ts       DateTime64(3) DEFAULT now64(3),
    
    -- 分区日期（自动计算）
    _partition_date Date DEFAULT toDate(ts)
)
ENGINE = MergeTree()
PARTITION BY (tenant_id, toYYYYMM(_partition_date))
ORDER BY (tenant_id, top_label, ts, event_id)
TTL _partition_date + INTERVAL 30 DAY
SETTINGS 
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- ============================================================================
-- 检测结果表（分布式表 - 生产环境使用）
-- ============================================================================

-- 注释掉，本地开发环境不需要分布式表
-- CREATE TABLE IF NOT EXISTS detections_behavior
-- (
--     tenant_id       String,
--     run_id          String,
--     feature_set_id  String,
--     model_version   LowCardinality(String),
--     event_id        String,
--     community_id    String,
--     object_type     LowCardinality(String),
--     object_id       String,
--     ts              DateTime64(3),
--     labels          Array(LowCardinality(String)),
--     scores          Array(Float32),
--     top_label       LowCardinality(String),
--     top_score       Float32,
--     ingest_ts       DateTime64(3) DEFAULT now64(3),
--     _partition_date Date DEFAULT toDate(ts)
-- )
-- ENGINE = Distributed('traffic_cluster', 'traffic', 'detections_behavior_local', 
--     xxHash64(concat(tenant_id, community_id)));

-- ============================================================================
-- 聚合物化视图：按小时统计检测数量
-- ============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_detections_hourly
ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMM(hour))
ORDER BY (tenant_id, top_label, model_version, hour)
TTL hour + INTERVAL 90 DAY
AS SELECT
    tenant_id,
    top_label,
    model_version,
    toStartOfHour(ts) AS hour,
    count() AS detection_count,
    sum(top_score) AS total_score,
    avg(top_score) AS avg_score,
    min(top_score) AS min_score,
    max(top_score) AS max_score
FROM detections_behavior_local
GROUP BY tenant_id, top_label, model_version, hour;

-- ============================================================================
-- 聚合物化视图：按天统计检测数量
-- ============================================================================

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_detections_daily
ENGINE = SummingMergeTree()
PARTITION BY (tenant_id, toYYYYMM(day))
ORDER BY (tenant_id, top_label, model_version, day)
TTL day + INTERVAL 365 DAY
AS SELECT
    tenant_id,
    top_label,
    model_version,
    toDate(ts) AS day,
    count() AS detection_count,
    sum(top_score) AS total_score,
    avg(top_score) AS avg_score,
    min(top_score) AS min_score,
    max(top_score) AS max_score,
    uniqExact(object_id) AS unique_objects,
    uniqExact(community_id) AS unique_sessions
FROM detections_behavior_local
GROUP BY tenant_id, top_label, model_version, day;

-- ============================================================================
-- 辅助表：模型性能统计
-- ============================================================================

CREATE TABLE IF NOT EXISTS model_performance_stats
(
    tenant_id       String,
    model_version   LowCardinality(String),
    top_label       LowCardinality(String),
    stat_hour       DateTime,
    
    -- 统计指标
    total_inferences    UInt64,
    true_positives      UInt64 DEFAULT 0,
    false_positives     UInt64 DEFAULT 0,
    precision           Float32 DEFAULT 0,
    recall              Float32 DEFAULT 0,
    f1_score            Float32 DEFAULT 0,
    
    -- 性能指标
    avg_score           Float32,
    p50_score           Float32,
    p90_score           Float32,
    p99_score           Float32,
    
    -- 时间戳
    updated_at          DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY (tenant_id, toYYYYMM(stat_hour))
ORDER BY (tenant_id, model_version, top_label, stat_hour)
TTL stat_hour + INTERVAL 90 DAY;

-- ============================================================================
-- 辅助表：热点对象统计
-- ============================================================================

CREATE TABLE IF NOT EXISTS hot_objects_stats
(
    tenant_id       String,
    object_type     LowCardinality(String),
    object_id       String,
    stat_date       Date,
    
    -- 检测统计
    detection_count     UInt64,
    unique_labels       Array(LowCardinality(String)),
    label_counts        Array(UInt64),
    max_score           Float32,
    avg_score           Float32,
    
    -- 时间范围
    first_seen          DateTime64(3),
    last_seen           DateTime64(3),
    
    -- 更新时间
    updated_at          DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(updated_at)
PARTITION BY (tenant_id, toYYYYMM(stat_date))
ORDER BY (tenant_id, object_type, stat_date, detection_count DESC)
TTL stat_date + INTERVAL 30 DAY;

-- ============================================================================
-- 索引优化
-- ============================================================================

-- 为常用查询添加 Bloom Filter 索引
ALTER TABLE detections_behavior_local
    ADD INDEX IF NOT EXISTS idx_event_id event_id TYPE bloom_filter(0.01) GRANULARITY 4,
    ADD INDEX IF NOT EXISTS idx_community_id community_id TYPE bloom_filter(0.01) GRANULARITY 4,
    ADD INDEX IF NOT EXISTS idx_object_id object_id TYPE bloom_filter(0.01) GRANULARITY 4;

-- ============================================================================
-- 示例查询（用于验证）
-- ============================================================================

-- 插入测试数据
INSERT INTO detections_behavior_local 
    (tenant_id, run_id, feature_set_id, model_version, event_id, community_id, 
     object_type, object_id, ts, labels, scores, top_label, top_score)
VALUES
    ('tenant-001', 'run-001', 'fs-001', 'v1.0', 'evt-001', 'comm-001',
     'flow', 'obj-001', now64(3), ['port_scan', 'normal'], [0.85, 0.15], 'port_scan', 0.85);

-- 验证数据
SELECT * FROM detections_behavior_local LIMIT 1;

-- 清理测试数据
-- TRUNCATE TABLE detections_behavior_local;

SELECT 'ClickHouse initialization completed successfully!' AS status;
