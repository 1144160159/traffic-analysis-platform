-- =============================================================================
-- ClickHouse 初始化脚本（本地开发环境）
-- 创建 campaigns_local 表用于 CEP Job 输出
-- =============================================================================

CREATE DATABASE IF NOT EXISTS traffic;

USE traffic;

-- =============================================================================
-- campaigns_local 表（单节点版本，无 Replicated 引擎）
-- =============================================================================
CREATE TABLE IF NOT EXISTS campaigns_local (
    -- 基础字段
    tenant_id String COMMENT '租户 ID',
    campaign_id String COMMENT '战役 ID',
    
    -- 时间范围
    ts_start DateTime64(3) COMMENT '战役开始时间',
    ts_end DateTime64(3) COMMENT '战役结束时间',
    
    -- 关联实体
    entities Array(String) DEFAULT [] COMMENT '关联实体列表（格式：ip:x.x.x.x）',
    alerts Array(String) DEFAULT [] COMMENT '关联告警 ID 列表',
    
    -- 评分与摘要
    score Float32 DEFAULT 0 COMMENT '战役综合评分（0-1）',
    summary String DEFAULT '' COMMENT '战役摘要描述',
    
    -- 事件追踪
    event_id String COMMENT '事件 ID（幂等键）',
    ingest_ts DateTime64(3) DEFAULT now64(3) COMMENT '入库时间',
    
    -- CEP 输出字段
    campaign_type LowCardinality(String) DEFAULT '' COMMENT '战役类型（scan_exploit/brute_force/lateral_movement/data_exfiltration/c2_communication）',
    attack_phases Array(String) DEFAULT [] COMMENT '攻击阶段列表（ATT&CK）',
    rule_ids Array(String) DEFAULT [] COMMENT '关联规则 ID 列表',
    model_ids Array(String) DEFAULT [] COMMENT '关联模型 ID 列表',
    
    -- 索引
    INDEX idx_campaign_type campaign_type TYPE bloom_filter GRANULARITY 1,
    INDEX idx_attack_phases attack_phases TYPE bloom_filter GRANULARITY 1,
    INDEX idx_entities entities TYPE bloom_filter GRANULARITY 1
)
ENGINE = MergeTree()
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, campaign_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- =============================================================================
-- 物化视图：按小时聚合战役统计
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS campaign_stats_hourly
ENGINE = SummingMergeTree()
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, campaign_type)
TTL toDateTime(hour) + INTERVAL 30 DAY
AS
SELECT
    tenant_id,
    toStartOfHour(ts_end) AS hour,
    campaign_type,
    count() AS campaign_count,
    avg(score) AS avg_score,
    max(score) AS max_score,
    uniqExact(campaign_id) AS unique_campaigns,
    length(arrayDistinct(arrayFlatten(groupArray(entities)))) AS unique_entities,
    length(arrayDistinct(arrayFlatten(groupArray(alerts)))) AS unique_alerts
FROM campaigns_local
GROUP BY tenant_id, hour, campaign_type;

-- =============================================================================
-- 物化视图：按攻击阶段统计
-- =============================================================================
CREATE MATERIALIZED VIEW IF NOT EXISTS campaign_phase_stats_daily
ENGINE = SummingMergeTree()
PARTITION BY toDate(day)
ORDER BY (tenant_id, day, attack_phase)
TTL toDateTime(day) + INTERVAL 30 DAY
AS
SELECT
    tenant_id,
    toDate(ts_end) AS day,
    arrayJoin(attack_phases) AS attack_phase,
    count() AS phase_count,
    avg(score) AS avg_score
FROM campaigns_local
WHERE length(attack_phases) > 0
GROUP BY tenant_id, day, attack_phase;

-- =============================================================================
-- 视图：最近24小时战役概览
-- =============================================================================
CREATE OR REPLACE VIEW v_campaigns_24h AS
SELECT
    tenant_id,
    campaign_type,
    count() AS campaign_count,
    avg(score) AS avg_score,
    max(score) AS max_score,
    min(ts_start) AS earliest_start,
    max(ts_end) AS latest_end,
    uniqExact(arrayJoin(entities)) AS unique_entities
FROM campaigns_local
WHERE ts_end >= now() - INTERVAL 24 HOUR
GROUP BY tenant_id, campaign_type
ORDER BY campaign_count DESC;

-- =============================================================================
-- 视图：高危战役（score >= 0.7）
-- =============================================================================
CREATE OR REPLACE VIEW v_high_risk_campaigns AS
SELECT
    tenant_id,
    campaign_id,
    campaign_type,
    score,
    summary,
    ts_start,
    ts_end,
    length(alerts) AS alert_count,
    length(entities) AS entity_count,
    attack_phases
FROM campaigns_local
WHERE score >= 0.7
ORDER BY ts_end DESC
LIMIT 100;

-- =============================================================================
-- 测试数据（可选）
-- =============================================================================
-- INSERT INTO campaigns_local (
--     tenant_id, campaign_id, ts_start, ts_end, 
--     entities, alerts, score, summary, 
--     event_id, campaign_type, attack_phases
-- ) VALUES (
--     'test-tenant', 
--     'campaign-test-001', 
--     now64(3) - 3600000, 
--     now64(3), 
--     ['ip:192.168.1.100', 'ip:10.0.0.50'],
--     ['alert-001', 'alert-002'],
--     0.85, 
--     '测试战役：扫描后利用攻击', 
--     'event-001', 
--     'scan_exploit', 
--     ['reconnaissance', 'initial_access']
-- );

-- =============================================================================
-- 常用查询示例
-- =============================================================================
-- 
-- 1. 查看最近战役：
-- SELECT tenant_id, campaign_id, campaign_type, score, 
--        formatDateTime(ts_start, '%Y-%m-%d %H:%i:%s') AS start_time 
-- FROM campaigns_local 
-- ORDER BY ts_start DESC 
-- LIMIT 10;
--
-- 2. 按类型统计：
-- SELECT campaign_type, count() AS cnt, avg(score) AS avg_score
-- FROM campaigns_local
-- WHERE ts_end >= today() - 7
-- GROUP BY campaign_type
-- ORDER BY cnt DESC;
--
-- 3. 查找特定 IP 相关战役：
-- SELECT campaign_id, campaign_type, score, entities
-- FROM campaigns_local
-- WHERE hasAny(entities, ['ip:192.168.1.100'])
-- ORDER BY ts_end DESC;