-- ============================================================================
-- ClickHouse 初始化脚本 (本地开发环境 - 单节点)
-- 
-- 用于 Docker Compose 本地开发
-- 生产环境使用集群版 DDL (6 Shard × 2 Replica)
-- ============================================================================

CREATE DATABASE IF NOT EXISTS traffic;

-- ============================================================================
-- alerts_local: 告警表 (ReplacingMergeTree)
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.alerts_local
(
    -- 租户与标识
    tenant_id String,
    alert_id String,
    
    -- 网络五元组
    src_ip String,
    dst_ip String,
    src_port UInt16,
    dst_port UInt16,
    protocol UInt8,
    protocol_name String,
    
    -- 告警分类
    alert_type String,
    severity String,
    labels Array(String),
    
    -- 时间信息
    first_seen DateTime64(3),
    last_seen DateTime64(3),
    
    -- 状态信息
    status String,
    assignee String,
    count Int32,
    score Float32,
    
    -- 版本控制
    updated_ts DateTime64(3),
    
    -- 关联信息
    community_id String,
    session_id String,
    campaign_id String,
    
    -- 模型/规则版本
    model_version String,
    rule_version String,
    feature_set_id String,
    
    -- 证据
    evidence_ids Array(String),
    
    -- 去重指纹
    dedup_fingerprint String,
    
    -- 事件 ID (幂等键)
    event_id String,
    
    -- Arkime 链接
    arkime_session_link String DEFAULT '',
    
    -- 用户反馈
    feedback_label LowCardinality(String) DEFAULT '',
    feedback_count UInt16 DEFAULT 0,
    
    -- 状态版本号 (乐观锁)
    state_version UInt64 DEFAULT 0,
    
    -- 入库时间
    ingest_ts DateTime64(3) DEFAULT now64(3),
    
    -- 索引
    INDEX idx_community_id community_id TYPE bloom_filter GRANULARITY 1,
    INDEX idx_alert_type alert_type TYPE bloom_filter GRANULARITY 1,
    INDEX idx_severity severity TYPE bloom_filter GRANULARITY 1,
    INDEX idx_src_ip src_ip TYPE bloom_filter GRANULARITY 1,
    INDEX idx_dst_ip dst_ip TYPE bloom_filter GRANULARITY 1
)
ENGINE = ReplacingMergeTree(updated_ts)
PARTITION BY toYYYYMM(last_seen)
ORDER BY (tenant_id, alert_id)
TTL toDateTime(last_seen) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- evidence_local: 证据表
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.evidence_local
(
    tenant_id String,
    evidence_id String,
    alert_id String,
    
    ts DateTime64(3),
    
    type LowCardinality(String),
    summary String,
    
    metrics_json String,
    snippet_ref_json String,
    arkime_link String,
    visualization_url String DEFAULT '',
    
    confidence Float32,
    
    event_id String,
    ingest_ts DateTime64(3) DEFAULT now64(3),
    
    INDEX idx_alert_id alert_id TYPE bloom_filter GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, alert_id, evidence_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- detections_behavior_local: 行为检测结果 (用于本地测试)
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.detections_behavior_local
(
    tenant_id String,
    run_id String,
    feature_set_id String,
    model_version String,
    event_id String,
    
    community_id String,
    object_type LowCardinality(String),
    object_id String,
    
    ts DateTime64(3),
    
    labels Array(LowCardinality(String)),
    scores Array(Float32),
    top_label LowCardinality(String),
    top_score Float32,
    
    ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, top_label, object_type, object_id)
TTL toDateTime(ts) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- detections_business_local: 业务检测结果 (用于本地测试)
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.detections_business_local
(
    tenant_id String,
    run_id String,
    feature_set_id String,
    model_version String,
    rule_version String,
    event_id String,
    
    ts DateTime64(3),
    
    community_id String,
    session_id String,
    campaign_id String,
    
    detection_type LowCardinality(String),
    label LowCardinality(String),
    score Float32,
    
    ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = MergeTree()
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, detection_type, label, community_id)
TTL toDateTime(ts) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- alert_trend_hour: 小时级告警趋势 (物化视图目标表)
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.alert_trend_hour
(
    tenant_id String,
    hour DateTime,
    severity LowCardinality(String),
    alert_type LowCardinality(String),
    cnt UInt64
)
ENGINE = SummingMergeTree()
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, severity, alert_type)
TTL toDateTime(hour) + INTERVAL 30 DAY;

-- ============================================================================
-- 物化视图: 小时级告警趋势
-- ============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS traffic.mv_alert_trend_hour
TO traffic.alert_trend_hour
AS
SELECT
    tenant_id,
    toStartOfHour(last_seen) AS hour,
    severity,
    alert_type,
    count() AS cnt
FROM traffic.alerts_local
GROUP BY tenant_id, hour, severity, alert_type;

-- ============================================================================
-- dedup_stats_local: 去重统计表
-- ============================================================================
CREATE TABLE IF NOT EXISTS traffic.dedup_stats_local
(
    tenant_id String,
    fingerprint String,
    
    alert_type LowCardinality(String),
    severity LowCardinality(String),
    src_ip String,
    dst_ip String,
    dst_port UInt16,
    
    first_seen DateTime64(3),
    last_seen DateTime64(3),
    occurrence_count UInt64,
    
    sample_alert_ids Array(String),
    
    ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplacingMergeTree(last_seen)
PARTITION BY toDate(last_seen)
ORDER BY (tenant_id, last_seen, fingerprint)
TTL toDateTime(last_seen) + INTERVAL 60 DAY
SETTINGS index_granularity = 8192;

-- ============================================================================
-- 插入测试数据 (可选)
-- ============================================================================
-- INSERT INTO traffic.alerts_local (
--     tenant_id, alert_id, src_ip, dst_ip, src_port, dst_port, 
--     protocol, protocol_name, alert_type, severity, labels,
--     first_seen, last_seen, status, assignee, count, score,
--     updated_ts, community_id, session_id, campaign_id,
--     model_version, rule_version, feature_set_id,
--     evidence_ids, dedup_fingerprint, event_id, state_version
-- ) VALUES (
--     'default', 'alert-test-001', '192.168.1.100', '10.0.0.50', 
--     12345, 80, 6, 'TCP', 'malware', 'SEVERITY_HIGH', 
--     ['suspicious', 'c2'], now64(3), now64(3), 
--     'ALERT_STATUS_NEW', '', 1, 0.85, now64(3),
--     'cid-123', 'session-123', '', 'v1.0', '', 'fs-001',
--     ['evidence-001'], 'fp-001', 'evt-001', 1
-- );

SELECT 'ClickHouse initialization completed!' AS message;
