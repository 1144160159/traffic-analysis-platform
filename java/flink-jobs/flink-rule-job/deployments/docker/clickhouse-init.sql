-- ============================================================================
-- ClickHouse Initialization for Rule Job (Local Development)
-- ============================================================================

-- 创建数据库
CREATE DATABASE IF NOT EXISTS traffic;

-- 切换数据库
USE traffic;

-- ============================================================================
-- DetectionBehavior 表（本地表）
-- ============================================================================
CREATE TABLE IF NOT EXISTS detections_behavior_local
(
    -- Header
    tenant_id       String,
    run_id          String,
    feature_set_id  String,
    model_version   String,
    event_id        String,
    
    -- Identifiers
    community_id    String,
    object_type     String,
    object_id       String,
    
    -- Timestamp
    ts              DateTime64(3),
    
    -- Detection Results
    labels          Array(String),
    scores          Array(Float32),
    top_label       String,
    top_score       Float32,
    
    -- Metadata
    ingest_ts       DateTime64(3),
    
    -- 物化列（用于查询优化）
    _date           Date MATERIALIZED toDate(ts),
    _hour           UInt8 MATERIALIZED toHour(ts)
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(ts)
ORDER BY (tenant_id, ts, community_id, event_id)
TTL ts + INTERVAL 30 DAY
SETTINGS 
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- ============================================================================
-- 物化视图：按规则类型聚合
-- ============================================================================
CREATE TABLE IF NOT EXISTS detections_by_rule_hourly
(
    tenant_id       String,
    top_label       String,
    hour            DateTime,
    count           UInt64,
    avg_score       Float64,
    max_score       Float32,
    unique_objects  AggregateFunction(uniq, String)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (tenant_id, top_label, hour)
TTL hour + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS detections_by_rule_hourly_mv
TO detections_by_rule_hourly
AS SELECT
    tenant_id,
    top_label,
    toStartOfHour(ts) as hour,
    count() as count,
    avg(top_score) as avg_score,
    max(top_score) as max_score,
    uniqState(object_id) as unique_objects
FROM detections_behavior_local
GROUP BY tenant_id, top_label, hour;

-- ============================================================================
-- 物化视图：按 IP 聚合（用于图分析）
-- ============================================================================
CREATE TABLE IF NOT EXISTS detections_by_ip_daily
(
    tenant_id       String,
    date            Date,
    ip              String,
    detection_count UInt64,
    rule_types      AggregateFunction(groupUniqArray, String),
    max_score       Float32,
    first_seen      DateTime64(3),
    last_seen       DateTime64(3)
)
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(date)
ORDER BY (tenant_id, date, ip)
TTL date + INTERVAL 90 DAY;

-- ============================================================================
-- 索引优化
-- ============================================================================
-- 为常用查询创建二级索引
ALTER TABLE detections_behavior_local 
    ADD INDEX idx_top_label (top_label) TYPE bloom_filter GRANULARITY 4;

ALTER TABLE detections_behavior_local 
    ADD INDEX idx_object_id (object_id) TYPE bloom_filter GRANULARITY 4;

ALTER TABLE detections_behavior_local 
    ADD INDEX idx_top_score (top_score) TYPE minmax GRANULARITY 4;

-- ============================================================================
-- 查询视图
-- ============================================================================
CREATE VIEW IF NOT EXISTS v_recent_detections AS
SELECT 
    tenant_id,
    ts,
    community_id,
    object_id,
    top_label,
    top_score,
    labels,
    model_version
FROM detections_behavior_local
WHERE ts > now() - INTERVAL 1 HOUR
ORDER BY ts DESC;

-- ============================================================================
-- 测试数据（可选）
-- ============================================================================
-- INSERT INTO detections_behavior_local 
-- (tenant_id, run_id, feature_set_id, model_version, event_id, 
--  community_id, object_type, object_id, ts, 
--  labels, scores, top_label, top_score, ingest_ts)
-- VALUES 
-- ('tenant-1', 'run-001', 'fs-001', 'rule-engine-v1', 'evt-001',
--  '1:abc123', 'flow', '192.168.1.1:443-10.0.0.1:52345', now(),
--  ['threshold', 'high-pps'], [0.85], 'threshold', 0.85, now());

-- ============================================================================
-- 权限设置（生产环境）
-- ============================================================================
-- CREATE USER IF NOT EXISTS flink IDENTIFIED BY 'flink_password';
-- GRANT SELECT, INSERT ON traffic.* TO flink;
