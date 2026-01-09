-- =========================================================================================
-- MERGED ClickHouse DDL for Traffic Analysis Platform
-- 合并来源：
-- - clickhouse_ddl.sql (基础表)
-- - alert_clickhouse.sql (告警服务扩展)
-- - auth_clickhouse.sql (认证服务)
-- - forensics_clickhouse.sql (取证服务)
-- - graph_clickhouse.sql (图服务)
-- - rules_clickhouse.sql (规则服务)
--
-- 合并策略：
-- 1. 基础表保留原有定义
-- 2. 扩展表追加新字段（使用 DEFAULT 避免迁移问题）
-- 3. 新增表独立创建
-- 4. 物化视图按服务分组
--
-- 变量说明：
-- - ${CH_CLUSTER}: ClickHouse 集群名称
-- - ${CH_DB}: 数据库名称
-- - ${CH_KEEPER_PREFIX}: Keeper 路径前缀
-- =========================================================================================

CREATE DATABASE IF NOT EXISTS ${CH_DB} ON CLUSTER ${CH_CLUSTER};

-- =========================================================================================
-- 核心流量分析表（基础）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- flows_raw: 原始流数据
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.flows_raw_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,
  feature_set_id String,
  event_id String,

  flow_id String,
  community_id String,

  ts_start DateTime64(3),
  ts_end   DateTime64(3),
  duration_ms UInt32,

  protocol UInt8,
  src_ip String,
  dst_ip String,
  src_port UInt16,
  dst_port UInt16,
  direction_def LowCardinality(String),

  packets_fwd UInt32,
  packets_bwd UInt32,
  bytes_fwd UInt64,
  bytes_bwd UInt64,

  pps Float32,
  bps Float32,

  pktlen_min UInt16,
  pktlen_max UInt16,
  pktlen_mean Float32,
  pktlen_std  Float32,

  iat_min_ms  Float32,
  iat_max_ms  Float32,
  iat_mean_ms Float32,
  iat_std_ms  Float32,

  tcp_flags_or_fwd UInt16,
  tcp_flags_or_bwd UInt16,
  tos_or UInt8,

  active_min_ms Float32,
  active_mean_ms Float32,
  active_max_ms Float32,
  active_std_ms  Float32,

  idle_min_ms Float32,
  idle_mean_ms Float32,
  idle_max_ms Float32,
  idle_std_ms  Float32,

  subflow_count UInt16,

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/flows_raw_local',
  '{replica}'
)
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, community_id, flow_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.flows_raw
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.flows_raw_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  flows_raw_local,
  cityHash64(tenant_id, community_id)
);

-- -----------------------------------------------------------------------------------------
-- sessions: 会话级别聚合数据
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.sessions_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,
  feature_set_id String,
  event_id String,

  session_id String,
  community_id String,

  ts_start DateTime64(3),
  ts_end   DateTime64(3),
  duration_ms UInt32,

  protocol UInt8,
  client_ip String,
  server_ip String,
  client_port UInt16,
  server_port UInt16,

  packets_total UInt32,
  bytes_total UInt64,
  bytes_up UInt64,
  bytes_down UInt64,
  up_down_ratio Float32,

  num_pkts UInt32,
  avg_payload Float32,
  min_payload UInt16,
  max_payload UInt16,
  std_payload Float32,

  is_established UInt8 DEFAULT 0  COMMENT 'TCP 连接是否成功建立（三次握手完成）',
  flow_ids Array(String) DEFAULT [] COMMENT '关联的 Flow ID 列表（最多 100 个）',
  end_reason LowCardinality(String) DEFAULT '' COMMENT '会话结束原因（FIN/RST/TIMEOUT/ERROR）',
  has_syn UInt8 DEFAULT 0 COMMENT '是否包含 SYN 标志',
  has_fin UInt8 DEFAULT 0 COMMENT '是否包含 FIN 标志',
  has_rst UInt8 DEFAULT 0 COMMENT '是否包含 RST 标志',

  mean_iat_ms Float32,
  min_iat_ms  Float32,
  max_iat_ms  Float32,
  std_iat_ms  Float32,

  flags_syn UInt16,
  flags_ack UInt16,
  flags_fin UInt16,
  flags_psh UInt16,
  flags_rst UInt16,

  dns_pkt_cnt UInt16,
  tcp_pkt_cnt UInt16,
  udp_pkt_cnt UInt16,
  icmp_pkt_cnt UInt16,

  evidence_count UInt16,

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/sessions_local',
  '{replica}'
)
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, community_id, session_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.sessions
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.sessions_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  sessions_local,
  cityHash64(tenant_id, community_id)
);

-- =========================================================================================
-- 特征工程表（L1/L2/L3）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- feature_stat (L1): 统计特征
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_stat_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,
  feature_set_id String,
  schema_version LowCardinality(String),
  event_id String,

  object_type LowCardinality(String),
  object_id String,
  community_id String,

  ts DateTime64(3),

  protocol UInt8,
  duration_ms UInt32,
  pps Float32,
  bps Float32,
  up_down_ratio Float32,

  pktlen_mean Float32,
  pktlen_std  Float32,
  iat_mean_ms Float32,
  iat_std_ms  Float32,

  active_mean_ms Float32,
  idle_mean_ms Float32,

  tcp_flag_syn_cnt UInt16,
  tcp_flag_ack_cnt UInt16,

  tcp_init_win_bytes_fwd UInt32,
  tcp_init_win_bytes_bwd UInt32,

  extra Array(Float32),

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/feature_stat_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, object_type, object_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_stat
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.feature_stat_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  feature_stat_local,
  cityHash64(tenant_id, community_id)
);

-- -----------------------------------------------------------------------------------------
-- feature_seq (L2): 序列特征
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_seq_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,
  feature_set_id String,
  event_id String,

  object_type LowCardinality(String),
  object_id String,
  community_id String,

  window_id String,
  ts_start DateTime64(3),
  ts_end   DateTime64(3),

  pktlen_seq_hash String,
  iat_seq_hash String,

  wavelet_releng_fwd Float32,
  wavelet_releng_bwd Float32,
  wavelet_entropy_fwd Float32,
  wavelet_entropy_bwd Float32,
  wavelet_detail_mean_fwd Float32,
  wavelet_detail_mean_bwd Float32,
  wavelet_detail_std_fwd Float32,
  wavelet_detail_std_bwd Float32,

  seq_blob_ref String,

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/feature_seq_local',
  '{replica}'
)
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, community_id, object_type, object_id, window_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_seq
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.feature_seq_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  feature_seq_local,
  cityHash64(tenant_id, community_id)
);

-- -----------------------------------------------------------------------------------------
-- feature_fp (L3): 指纹特征
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_fp_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,
  feature_set_id String,
  event_id String,

  community_id String,
  session_id String,

  ts DateTime64(3),

  is_encrypted UInt8,
  tls_version LowCardinality(String),
  ja3 LowCardinality(String),

  sni_hash String,
  cert_sha256 String,
  cert_is_self_signed UInt8,
  pubkey_len UInt16,

  hex_freq  Array(Float32),
  hex_ratio Array(Float32),

  entropy_payload Float32,
  chi_square_bfd Float32,

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/feature_fp_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, session_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.feature_fp
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.feature_fp_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  feature_fp_local,
  cityHash64(tenant_id, community_id)
);

-- =========================================================================================
-- 检测结果表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- detections_behavior: 行为检测结果
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.detections_behavior_local
ON CLUSTER ${CH_CLUSTER}
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
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/detections_behavior_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, top_label, object_type, object_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.detections_behavior
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.detections_behavior_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  detections_behavior_local,
  cityHash64(tenant_id, community_id)
);

-- -----------------------------------------------------------------------------------------
-- detections_business: 业务检测结果
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.detections_business_local
ON CLUSTER ${CH_CLUSTER}
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
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/detections_business_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, detection_type, label, community_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.detections_business
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.detections_business_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  detections_business_local,
  cityHash64(tenant_id, community_id)
);

-- =========================================================================================
-- 告警与证据表（包含扩展字段）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- alerts: 告警表（ReplacingMergeTree 支持更新）
-- 扩展字段来源：alert_clickhouse.sql
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS 
alerts_local ON CLUSTER '{cluster}' (
    -- 租户与标识
    tenant_id String,
    alert_id String,
    
    -- 网络五元组（冗余，便于查询）
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
    
    -- 时间信息（用于分区和排序）
    first_seen DateTime64(3),
    last_seen DateTime64(3),
    
    -- 状态信息（聚合最新值）
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
    
    -- 事件 ID
    event_id String

    arkime_session_link String DEFAULT '', -- 新增：Arkime 会话链接
    feedback_label LowCardinality(String) DEFAULT '', -- 新增：用户反馈标签
    feedback_count UInt16 DEFAULT 0, -- 新增：反馈次数
    state_version UInt64 DEFAULT 0, -- 新增：状态版本号（乐观锁）
    ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
    '/clickhouse/tables/{shard}/traffic/alerts_local',
    '{replica}',
    updated_ts  -- 使用 updated_ts 作为版本号
)
ENGINE = ReplicatedReplacingMergeTree(
    '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alerts_local',
    '{replica}',
    updated_ts  -- 使用 updated_ts 作为版本号
)
PARTITION BY toYYYYMM(last_seen)
ORDER BY (tenant_id, alert_id)
TTL toDateTime(last_seen) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alerts
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alerts_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alerts_local,
  cityHash64(tenant_id, alert_id)
);
-- -----------------------------------------------------------------------------------------
-- evidence: 证据表
-- 扩展字段来源：alert_clickhouse.sql
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.evidence_local
ON CLUSTER ${CH_CLUSTER}
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
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/evidence_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, alert_id, evidence_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.evidence
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.evidence_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  evidence_local,
  cityHash64(tenant_id, alert_id)
);

-- =========================================================================================
-- 战役分析表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- campaigns: 战役聚合
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.campaigns_local
ON CLUSTER ${CH_CLUSTER}
(
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
    
    -- 新增字段（CEP 输出）
    campaign_type LowCardinality(String) DEFAULT '' COMMENT '战役类型',
    attack_phases Array(String) DEFAULT [] COMMENT '攻击阶段列表',
    rule_ids Array(String) DEFAULT [] COMMENT '关联规则 ID 列表',
    model_ids Array(String) DEFAULT [] COMMENT '关联模型 ID 列表'
)
ENGINE = ReplicatedReplacingMergeTree(
    '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/campaigns_local',
    '{replica}',
    ts_end
)
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, campaign_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- 创建分布式表
CREATE TABLE IF NOT EXISTS ${CH_DB}.campaigns
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.campaigns_local
ENGINE = Distributed(
    ${CH_CLUSTER},
    ${CH_DB},
    campaigns_local,
    cityHash64(tenant_id, campaign_id)
);

-- 为 campaign_type 创建 Bloom Filter 索引
ALTER TABLE ${CH_DB}.campaigns_local ON CLUSTER ${CH_CLUSTER}
ADD INDEX IF NOT EXISTS idx_campaign_type campaign_type TYPE bloom_filter GRANULARITY 1;

-- 为 attack_phases 创建索引（数组字段）
ALTER TABLE ${CH_DB}.campaigns_local ON CLUSTER ${CH_CLUSTER}
ADD INDEX IF NOT EXISTS idx_attack_phases attack_phases TYPE bloom_filter GRANULARITY 1;

-- =========================================================================================
-- 运行报告表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- run_report: 运行报告
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.run_report_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  run_id String,

  mode LowCardinality(String),
  input_source String,

  feature_set_id String,
  model_version String,
  rule_version String,

  start_ts DateTime64(3),
  end_ts DateTime64(3),

  throughput_pps Float64,
  throughput_bps Float64,
  lag_ms_p95 Float64,

  dlq_count UInt64,
  error_topk_json String,
  metrics_json String,

  created_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/run_report_local',
  '{replica}',
  created_ts
)
PARTITION BY toDate(created_ts)
ORDER BY (tenant_id, created_ts, run_id)
TTL toDateTime(created_ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.run_report
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.run_report_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  run_report_local,
  cityHash64(tenant_id, run_id)
);

-- =========================================================================================
-- PCAP 索引表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- pcap_index: PCAP 文件索引
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.pcap_index_local ON CLUSTER ${CH_CLUSTER}
(
  -- 租户与探针标识
  tenant_id String,
  probe_id String,
  
  -- 文件标识
  file_key String,
  
  -- 时间范围
  ts_start DateTime64(3),
  ts_end   DateTime64(3),
  
  -- 文件元数据
  byte_size UInt64,
  zstd_level UInt8,
  sha256 String,
  
  -- 流标识（单个，用于快速查询）
  community_id String,
  flow_id String,
  
  -- 文件偏移（用于精确裁剪）
  offset_start Nullable(UInt64),
  offset_end Nullable(UInt64),
  
  -- ✅ 新增：快速检索字段
  bloom_filter_b64 String DEFAULT '' COMMENT 'Base64编码的BloomFilter，用于IP快速检索',
  community_ids Array(String) DEFAULT [] COMMENT 'PCAP文件包含的所有Community ID列表（最多1000个）',
  
  -- 创建时间
  created_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/pcap_index_local',
  '{replica}',
  created_ts  -- ✅ 使用 created_ts 作为版本号（幂等写入）
)
PARTITION BY toDate(ts_start)
ORDER BY (tenant_id, ts_start, probe_id, file_key)
TTL toDateTime(ts_start) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- 创建分布式表
CREATE TABLE IF NOT EXISTS ${CH_DB}.pcap_index ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.pcap_index_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  pcap_index_local,
  cityHash64(tenant_id, file_key)
);

-- ==================== 索引优化（可选）====================

-- 为 community_id 创建 Bloom Filter 索引（加速点查）
ALTER TABLE ${CH_DB}.pcap_index_local ON CLUSTER ${CH_CLUSTER}
    ADD INDEX IF NOT EXISTS idx_community_id community_id TYPE bloom_filter GRANULARITY 1;

-- 为 sha256 创建索引（文件去重）
ALTER TABLE ${CH_DB}.pcap_index_local ON CLUSTER ${CH_CLUSTER}
    ADD INDEX IF NOT EXISTS idx_sha256 sha256 TYPE bloom_filter GRANULARITY 1;
-- =========================================================================================
-- 告警服务扩展表（来源：alert_clickhouse.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- alert_feedback: 用户反馈表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_feedback_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  feedback_id String,
  alert_id String,

  user_id String,
  label LowCardinality(String),
  reason_code LowCardinality(String),
  comment String,

  add_to_whitelist UInt8,

  alert_type LowCardinality(String),
  severity LowCardinality(String),
  model_version String,
  rule_version String,

  ts DateTime64(3),
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alert_feedback_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, label, alert_type, alert_id, feedback_id)
TTL toDateTime(ts) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_feedback
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alert_feedback_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alert_feedback_local,
  cityHash64(tenant_id, alert_id)
);

-- -----------------------------------------------------------------------------------------
-- whitelist_rules: 白名单规则表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.whitelist_rules_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  rule_id String,

  rule_type LowCardinality(String),
  src_ip String,
  dst_ip String,
  src_port UInt16,
  dst_port UInt16,
  protocol UInt8,

  alert_type LowCardinality(String),
  reason_code LowCardinality(String),
  comment String,

  status LowCardinality(String),
  created_by String,
  created_ts DateTime64(3),
  updated_ts DateTime64(3),
  expires_at DateTime64(3),

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/whitelist_rules_local',
  '{replica}',
  updated_ts
)
PARTITION BY toDate(created_ts)
ORDER BY (tenant_id, status, rule_type, src_ip, dst_ip, rule_id)
TTL toDateTime(created_ts) + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.whitelist_rules
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.whitelist_rules_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  whitelist_rules_local,
  cityHash64(tenant_id, rule_id)
);

-- -----------------------------------------------------------------------------------------
-- alert_state_transitions: 告警状态转换历史
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_state_transitions_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  alert_id String,
  transition_id String,

  old_status LowCardinality(String),
  new_status LowCardinality(String),
  old_assignee String,
  new_assignee String,

  changed_by String,
  change_reason String,
  state_version UInt64,

  ts DateTime64(3),
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alert_state_transitions_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, alert_id, ts, transition_id)
TTL toDateTime(ts) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_state_transitions
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alert_state_transitions_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alert_state_transitions_local,
  cityHash64(tenant_id, alert_id)
);

-- -----------------------------------------------------------------------------------------
-- dedup_stats: 去重统计表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.dedup_stats_local
ON CLUSTER ${CH_CLUSTER}
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
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/dedup_stats_local',
  '{replica}',
  last_seen
)
PARTITION BY toDate(last_seen)
ORDER BY (tenant_id, last_seen, fingerprint)
TTL toDateTime(last_seen) + INTERVAL 60 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.dedup_stats
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.dedup_stats_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  dedup_stats_local,
  cityHash64(tenant_id, fingerprint)
);

-- -----------------------------------------------------------------------------------------
-- storage_health_events: 存储健康状态事件
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.storage_health_events_local
ON CLUSTER ${CH_CLUSTER}
(
  storage_type LowCardinality(String),
  storage_name String,
  status LowCardinality(String),
  error_message String,
  consecutive_failures UInt16,

  ts DateTime64(3),
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/storage_health_events_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (storage_type, storage_name, ts)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.storage_health_events
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.storage_health_events_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  storage_health_events_local,
  cityHash64(storage_type, storage_name)
);

-- -----------------------------------------------------------------------------------------
-- model_feedback_metrics: 模型反馈指标聚合
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.model_feedback_metrics_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  model_version String,
  alert_type LowCardinality(String),

  hour DateTime,

  total_alerts UInt64,
  tp_count UInt64,
  fp_count UInt64,
  unlabeled_count UInt64,

  precision Float32,
  recall Float32,
  f1_score Float32,

  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/model_feedback_metrics_local',
  '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, model_version, alert_type, hour)
TTL toDateTime(hour) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.model_feedback_metrics
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.model_feedback_metrics_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  model_feedback_metrics_local,
  cityHash64(tenant_id, model_version)
);

-- -----------------------------------------------------------------------------------------
-- alert_correlation_graph: 告警关联图
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_correlation_graph_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  edge_id String,

  source_alert_id String,
  target_alert_id String,

  correlation_type LowCardinality(String),
  correlation_score Float32,

  shared_entities Array(String),
  time_delta_ms Int64,

  ts DateTime64(3),
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alert_correlation_graph_local',
  '{replica}'
)
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, source_alert_id, target_alert_id, edge_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_correlation_graph
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alert_correlation_graph_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alert_correlation_graph_local,
  cityHash64(tenant_id, source_alert_id)
);

-- -----------------------------------------------------------------------------------------
-- notification_events: 通知事件记录
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.notification_events_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  notification_id String,
  alert_id String,

  channel LowCardinality(String),
  status LowCardinality(String),
  error_message String,

  rule_id String,
  recipient String,

  sent_at DateTime64(3),
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/notification_events_local',
  '{replica}'
)
PARTITION BY toDate(sent_at)
ORDER BY (tenant_id, sent_at, alert_id, notification_id)
TTL toDateTime(sent_at) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.notification_events
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.notification_events_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  notification_events_local,
  cityHash64(tenant_id, alert_id)
);

-- =========================================================================================
-- Graph Service 表（来源：graph_clickhouse.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- graph_query_log: 图查询日志
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_query_log_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  query_id String,
  user_id String,
  
  query_type LowCardinality(String),
  center_ip String,
  center_ips Array(String),
  depth UInt8,
  run_id String,
  
  query_start_time DateTime64(3),
  query_end_time DateTime64(3),
  
  node_count UInt32,
  edge_count UInt32,
  path_count UInt32,
  result_size_bytes UInt64,
  
  duration_ms UInt32,
  cache_hit UInt8,
  
  ch_query_count UInt16,
  ch_total_duration_ms UInt32,
  ch_rows_read UInt64,
  ch_bytes_read UInt64,
  
  status LowCardinality(String),
  error_code String,
  error_message String,
  
  trace_id String,
  client_ip String,
  user_agent String,
  
  created_at DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/graph_query_log_local',
  '{replica}'
)
PARTITION BY toDate(created_at)
ORDER BY (tenant_id, created_at, query_type, status)
TTL toDateTime(created_at) + INTERVAL 7 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_query_log
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.graph_query_log_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  graph_query_log_local,
  cityHash64(tenant_id, query_id)
);

-- ==================== 创建物化视图（可选）：会话结束原因统计 ====================

CREATE MATERIALIZED VIEW IF NOT EXISTS sessions_end_reason_hourly_local
ON CLUSTER '{cluster}'
ENGINE = ReplicatedSummingMergeTree(
    '/clickhouse/tables/{shard}/traffic/sessions_end_reason_hourly_local',
    '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, end_reason, protocol)
TTL toDateTime(hour) + INTERVAL 30 DAY
AS
SELECT
    tenant_id,
    toStartOfHour(ts_end) AS hour,
    end_reason,
    protocol,
    count() AS session_count,
    sum(packets_total) AS total_packets,
    sum(bytes_total) AS total_bytes
FROM sessions_local
GROUP BY tenant_id, hour, end_reason, protocol;

CREATE TABLE IF NOT EXISTS sessions_end_reason_hourly
ON CLUSTER '{cluster}'
AS sessions_end_reason_hourly_local
ENGINE = Distributed(
    '{cluster}',
    traffic,
    sessions_end_reason_hourly_local,
    cityHash64(tenant_id, hour)
);

-- ==================== 创建物化视图（可选）：TCP 连接成功率统计 ====================

CREATE MATERIALIZED VIEW IF NOT EXISTS sessions_tcp_success_rate_hourly_local
ON CLUSTER '{cluster}'
ENGINE = ReplicatedSummingMergeTree(
    '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/sessions_tcp_success_rate_hourly_local',
    '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, protocol)
TTL toDateTime(hour) + INTERVAL 30 DAY
AS
SELECT
    tenant_id,
    toStartOfHour(ts_end) AS hour,
    protocol,
    countIf(is_established = 1) AS established_count,
    countIf(is_established = 0) AS failed_count,
    count() AS total_count
FROM sessions_local
WHERE protocol = 6 -- 仅 TCP
GROUP BY tenant_id, hour, protocol;

CREATE TABLE IF NOT EXISTS sessions_tcp_success_rate_hourly
ON CLUSTER '{cluster}'
AS sessions_tcp_success_rate_hourly_local
ENGINE = Distributed(
    '{cluster}',
    traffic,
    sessions_tcp_success_rate_hourly_local,
    cityHash64(tenant_id, hour)
);

-- -----------------------------------------------------------------------------------------
-- graph_cache_stats: 图缓存统计（物化视图）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_cache_stats_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  hour DateTime,
  query_type LowCardinality(String),
  
  total_queries UInt64,
  cache_hits UInt64,
  cache_misses UInt64,
  
  avg_duration_ms Float32,
  p95_duration_ms Float32,
  p99_duration_ms Float32,
  
  total_nodes UInt64,
  total_edges UInt64,
  
  error_count UInt64,
  timeout_count UInt64
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/graph_cache_stats_local',
  '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, query_type)
TTL toDateTime(hour) + INTERVAL 30 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_graph_cache_stats_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.graph_cache_stats_local
AS
SELECT
  tenant_id,
  toStartOfHour(created_at) AS hour,
  query_type,
  
  count() AS total_queries,
  countIf(cache_hit = 1) AS cache_hits,
  countIf(cache_hit = 0) AS cache_misses,
  
  avg(duration_ms) AS avg_duration_ms,
  quantile(0.95)(duration_ms) AS p95_duration_ms,
  quantile(0.99)(duration_ms) AS p99_duration_ms,
  
  sum(node_count) AS total_nodes,
  sum(edge_count) AS total_edges,
  
  countIf(status = 'error') AS error_count,
  countIf(status = 'timeout') AS timeout_count
FROM ${CH_DB}.graph_query_log_local
GROUP BY tenant_id, hour, query_type;

CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_cache_stats
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.graph_cache_stats_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  graph_cache_stats_local,
  cityHash64(tenant_id, hour)
);

-- -----------------------------------------------------------------------------------------
-- graph_hot_ips: 热点 IP 统计（物化视图）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_hot_ips_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  date Date,
  ip String,
  
  query_count UInt64,
  total_neighbors UInt64,
  avg_session_count Float32,
  
  last_query_time DateTime64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/graph_hot_ips_local',
  '{replica}',
  last_query_time
)
PARTITION BY date
ORDER BY (tenant_id, date, query_count DESC, ip)
TTL toDateTime(date) + INTERVAL 7 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_graph_hot_ips_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.graph_hot_ips_local
AS
SELECT
  tenant_id,
  toDate(created_at) AS date,
  center_ip AS ip,
  
  count() AS query_count,
  sum(node_count) AS total_neighbors,
  avg(node_count) AS avg_session_count,
  
  max(created_at) AS last_query_time
FROM ${CH_DB}.graph_query_log_local
WHERE query_type IN ('explore', 'entity_details', 'neighbors')
  AND center_ip != ''
GROUP BY tenant_id, date, center_ip;

CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_hot_ips
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.graph_hot_ips_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  graph_hot_ips_local,
  cityHash64(tenant_id, ip)
);

-- -----------------------------------------------------------------------------------------
-- graph_slow_queries: 慢查询表（物化视图）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_slow_queries_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  query_id String,
  query_type LowCardinality(String),
  
  center_ip String,
  depth UInt8,
  run_id String,
  
  duration_ms UInt32,
  node_count UInt32,
  edge_count UInt32,
  
  ch_rows_read UInt64,
  ch_bytes_read UInt64,
  
  error_message String,
  
  created_at DateTime64(3)
)
ENGINE = ReplicatedMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/graph_slow_queries_local',
  '{replica}'
)
PARTITION BY toDate(created_at)
ORDER BY (tenant_id, created_at DESC, duration_ms DESC)
TTL toDateTime(created_at) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_graph_slow_queries_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.graph_slow_queries_local
AS
SELECT
  tenant_id,
  query_id,
  query_type,
  center_ip,
  depth,
  run_id,
  duration_ms,
  node_count,
  edge_count,
  ch_rows_read,
  ch_bytes_read,
  error_message,
  created_at
FROM ${CH_DB}.graph_query_log_local
WHERE duration_ms > 5000;

CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_slow_queries
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.graph_slow_queries_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  graph_slow_queries_local,
  cityHash64(tenant_id, query_id)
);

-- -----------------------------------------------------------------------------------------
-- graph_ip_affinity: IP 关系强度表（物化视图）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_ip_affinity_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  date Date,
  
  ip_a String,
  ip_b String,
  
  session_count UInt32,
  total_bytes UInt64,
  avg_duration_ms Float32,
  
  a_to_b_count UInt32,
  b_to_a_count UInt32,
  
  first_seen DateTime64(3),
  last_seen DateTime64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/graph_ip_affinity_local',
  '{replica}',
  last_seen
)
PARTITION BY date
ORDER BY (tenant_id, date, session_count DESC, ip_a, ip_b)
TTL toDateTime(date) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_graph_ip_affinity_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.graph_ip_affinity_local
AS
SELECT
  tenant_id,
  toDate(ts_end) AS date,
  
  client_ip AS ip_a,
  server_ip AS ip_b,
  
  count() AS session_count,
  sum(bytes_total) AS total_bytes,
  avg(duration_ms) AS avg_duration_ms,
  
  count() AS a_to_b_count,
  0 AS b_to_a_count,
  
  min(ts_start) AS first_seen,
  max(ts_end) AS last_seen
FROM ${CH_DB}.sessions_local
WHERE ts_end >= today() - INTERVAL 7 DAY
GROUP BY tenant_id, date, client_ip, server_ip
HAVING session_count >= 3;

CREATE TABLE IF NOT EXISTS ${CH_DB}.graph_ip_affinity
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.graph_ip_affinity_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  graph_ip_affinity_local,
  cityHash64(tenant_id, ip_a)
);

-- =========================================================================================
-- 物化视图：告警服务聚合表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- alert_trend_hour: 小时级告警趋势
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_trend_hour_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  hour DateTime,
  severity LowCardinality(String),
  alert_type LowCardinality(String),
  cnt UInt64
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alert_trend_hour_local',
  '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, severity, alert_type)
TTL toDateTime(hour) + INTERVAL 30 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_alert_trend_hour_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.alert_trend_hour_local
AS
SELECT
  tenant_id,
  toStartOfHour(last_seen) AS hour,
  severity,
  alert_type,
  count() AS cnt
FROM ${CH_DB}.alerts_local
GROUP BY tenant_id, hour, severity, alert_type;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_trend_hour
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alert_trend_hour_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alert_trend_hour_local,
  cityHash64(tenant_id, hour)
);

CREATE MATERIALIZED VIEW IF NOT EXISTS mv_alerts_local ON CLUSTER '{cluster}'
TO alerts_local
AS
SELECT
    -- 租户与标识
    tenant_id,
    alert_id,
    
    -- 网络五元组（使用 argMax 获取最新值）
    argMax(src_ip, updated_ts) AS src_ip,
    argMax(dst_ip, updated_ts) AS dst_ip,
    argMax(src_port, updated_ts) AS src_port,
    argMax(dst_port, updated_ts) AS dst_port,
    argMax(protocol, updated_ts) AS protocol,
    argMax(protocol_name, updated_ts) AS protocol_name,
    
    -- 告警分类
    argMax(alert_type, updated_ts) AS alert_type,
    argMax(severity, updated_ts) AS severity,
    argMax(labels, updated_ts) AS labels,
    
    -- 时间信息
    min(first_seen) AS first_seen,  -- 保留最早的 first_seen
    argMax(last_seen, updated_ts) AS last_seen,
    
    -- 状态信息（聚合最新值）
    argMax(status, updated_ts) AS status,
    argMax(assignee, updated_ts) AS assignee,
    argMax(count, updated_ts) AS count,
    argMax(score, updated_ts) AS score,
    
    -- 版本控制
    max(updated_ts) AS updated_ts,
    
    -- 关联信息
    argMax(community_id, updated_ts) AS community_id,
    argMax(session_id, updated_ts) AS session_id,
    argMax(campaign_id, updated_ts) AS campaign_id,
    
    -- 模型/规则版本
    argMax(model_version, updated_ts) AS model_version,
    argMax(rule_version, updated_ts) AS rule_version,
    argMax(feature_set_id, updated_ts) AS feature_set_id,
    
    -- 证据
    argMax(evidence_ids, updated_ts) AS evidence_ids,
    
    -- 去重指纹
    argMax(dedup_fingerprint, updated_ts) AS dedup_fingerprint,
    
    -- 事件 ID
    argMax(event_id, updated_ts) AS event_id

FROM alerts_local
GROUP BY tenant_id, alert_id;

-- -----------------------------------------------------------------------------------------
-- feedback_summary_daily: 反馈日汇总
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.feedback_summary_daily_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  day Date,
  alert_type LowCardinality(String),
  label LowCardinality(String),
  model_version String,

  feedback_count UInt64,
  unique_alerts UInt64
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/feedback_summary_daily_local',
  '{replica}'
)
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day, alert_type, label, model_version)
TTL toDateTime(day) + INTERVAL 180 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_feedback_summary_daily_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.feedback_summary_daily_local
AS
SELECT
  tenant_id,
  toDate(ts) AS day,
  alert_type,
  label,
  model_version,
  count() AS feedback_count,
  uniq(alert_id) AS unique_alerts
FROM ${CH_DB}.alert_feedback_local
GROUP BY tenant_id, day, alert_type, label, model_version;

CREATE TABLE IF NOT EXISTS ${CH_DB}.feedback_summary_daily
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.feedback_summary_daily_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  feedback_summary_daily_local,
  cityHash64(tenant_id, day)
);

-- -----------------------------------------------------------------------------------------
-- dedup_summary_hourly: 去重小时汇总
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.dedup_summary_hourly_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  hour DateTime,
  alert_type LowCardinality(String),
  severity LowCardinality(String),

  unique_fingerprints UInt64,
  total_occurrences UInt64,
  avg_occurrence_per_fingerprint Float32
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/dedup_summary_hourly_local',
  '{replica}'
)
PARTITION BY toDate(hour)
ORDER BY (tenant_id, hour, alert_type, severity)
TTL toDateTime(hour) + INTERVAL 60 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_dedup_summary_hourly_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.dedup_summary_hourly_local
AS
SELECT
  tenant_id,
  toStartOfHour(last_seen) AS hour,
  alert_type,
  severity,
  uniq(fingerprint) AS unique_fingerprints,
  sum(occurrence_count) AS total_occurrences,
  avg(occurrence_count) AS avg_occurrence_per_fingerprint
FROM ${CH_DB}.dedup_stats_local
GROUP BY tenant_id, hour, alert_type, severity;

CREATE TABLE IF NOT EXISTS ${CH_DB}.dedup_summary_hourly
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.dedup_summary_hourly_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  dedup_summary_hourly_local,
  cityHash64(tenant_id, hour)
);

-- -----------------------------------------------------------------------------------------
-- alert_status_transitions_summary: 状态转换汇总
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_status_transitions_summary_local
ON CLUSTER ${CH_CLUSTER}
(
  tenant_id String,
  day Date,
  old_status LowCardinality(String),
  new_status LowCardinality(String),

  transition_count UInt64,
  avg_duration_seconds Float32
)
ENGINE = ReplicatedSummingMergeTree(
  '${CH_KEEPER_PREFIX}/{shard}/${CH_DB}/alert_status_transitions_summary_local',
  '{replica}'
)
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id, day, old_status, new_status)
TTL toDateTime(day) + INTERVAL 90 DAY;

CREATE MATERIALIZED VIEW IF NOT EXISTS ${CH_DB}.mv_alert_status_transitions_summary_local
ON CLUSTER ${CH_CLUSTER}
TO ${CH_DB}.alert_status_transitions_summary_local
AS
SELECT
  tenant_id,
  toDate(ts) AS day,
  old_status,
  new_status,
  count() AS transition_count,
  0 AS avg_duration_seconds
FROM ${CH_DB}.alert_state_transitions_local
GROUP BY tenant_id, day, old_status, new_status;

CREATE TABLE IF NOT EXISTS ${CH_DB}.alert_status_transitions_summary
ON CLUSTER ${CH_CLUSTER}
AS ${CH_DB}.alert_status_transitions_summary_local
ENGINE = Distributed(
  ${CH_CLUSTER},
  ${CH_DB},
  alert_status_transitions_summary_local,
  cityHash64(tenant_id, day)
);

-- =========================================================================================
-- 视图：查询优化
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- v_graph_query_performance: 图查询性能视图
-- -----------------------------------------------------------------------------------------
CREATE OR REPLACE VIEW ${CH_DB}.v_graph_query_performance AS
SELECT
  tenant_id,
  query_type,
  toStartOfHour(created_at) AS hour,
  
  count() AS query_count,
  avg(duration_ms) AS avg_duration_ms,
  quantile(0.50)(duration_ms) AS p50_duration_ms,
  quantile(0.95)(duration_ms) AS p95_duration_ms,
  quantile(0.99)(duration_ms) AS p99_duration_ms,
  
  countIf(cache_hit = 1) AS cache_hits,
  countIf(cache_hit = 0) AS cache_misses,
  
  avg(node_count) AS avg_nodes,
  avg(edge_count) AS avg_edges,
  
  countIf(status = 'success') AS success_count,
  countIf(status = 'error') AS error_count,
  countIf(status = 'timeout') AS timeout_count
FROM ${CH_DB}.graph_query_log
WHERE created_at > now() - INTERVAL 24 HOUR
GROUP BY tenant_id, query_type, hour
ORDER BY tenant_id, hour DESC, query_type;

-- -----------------------------------------------------------------------------------------
-- v_graph_top_ips_24h: 24小时热点 IP 视图
-- -----------------------------------------------------------------------------------------
CREATE OR REPLACE VIEW ${CH_DB}.v_graph_top_ips_24h AS
SELECT
  tenant_id,
  center_ip AS ip,
  
  count() AS query_count,
  avg(node_count) AS avg_neighbors,
  max(node_count) AS max_neighbors,
  
  sum(duration_ms) AS total_duration_ms,
  avg(duration_ms) AS avg_duration_ms,
  
  max(created_at) AS last_query_at
FROM ${CH_DB}.graph_query_log
WHERE created_at > now() - INTERVAL 24 HOUR
  AND query_type IN ('explore', 'entity_details', 'neighbors')
  AND center_ip != ''
GROUP BY tenant_id, center_ip
ORDER BY tenant_id, query_count DESC
LIMIT 100 BY tenant_id;

