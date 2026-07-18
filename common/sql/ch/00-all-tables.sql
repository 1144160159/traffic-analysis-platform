-- =========================
-- ClickHouse DDL — 集群模式 (2 Shard x 2 Replica + 3 Keeper)
--
-- 架构:
--   xxx_local  — ReplicatedMergeTree 表 (每个 shard 的本地存储, Keeper 协调复制)
--   xxx        — Distributed 表 (跨 shard 读写入口)
--
-- 业务应用: 写入 Distributed 表, 自动按 rand() 分布到各 shard
-- 集群: traffic_cluster (定义在 K8s ConfigMap remote_servers.xml)
-- Keeper 路径: /clickhouse/tables/{shard}/{table_name}
--
-- Proto 对齐 (2026-06-07 Phase 17): 9 张表全部 100% 对齐
-- =========================

-- =============================================================================
-- 1. flows_raw — FlowEvent
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.flows_raw_local ON CLUSTER traffic_cluster (
  event_id       String,
  tenant_id      String,
  probe_id       String,
  community_id   String,
  src_ip         String,
  dst_ip         String,
  src_port       UInt32,
  dst_port       UInt32,
  protocol       UInt8,
  direction      String,
  ts_start       Int64,
  ts_end         Int64,
  duration_ms    UInt32,
  packets_fwd    UInt32,
  packets_bwd    UInt32,
  bytes_fwd      UInt64,
  bytes_bwd      UInt64,
  pps            Float32,
  bps            Float32,
  tcp_flags_fwd  UInt32,
  tcp_flags_bwd  UInt32,
  tos            UInt32,
  run_id         String,
  feature_set_id String,
  event_ts       Int64,
  ingest_ts      Int64,
  kafka_ts       Int64,
  flink_out_ts   Int64,
  pktlen_min     UInt32,
  pktlen_max     UInt32,
  pktlen_mean    Float32,
  pktlen_std     Float32,
  iat_min_ms     Float32,
  iat_max_ms     Float32,
  iat_mean_ms    Float32,
  iat_std_ms     Float32,
  active_min_ms  Float32,
  active_max_ms  Float32,
  active_mean_ms Float32,
  active_std_ms  Float32,
  idle_min_ms    Float32,
  idle_max_ms    Float32,
  idle_mean_ms   Float32,
  idle_std_ms    Float32,
  subflow_count  UInt32
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/flows_raw', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(ts_start / 1000))
ORDER BY (tenant_id, ts_end, community_id)
TTL toDateTime(ts_end / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.flows_raw ON CLUSTER traffic_cluster
AS traffic.flows_raw_local
ENGINE = Distributed(traffic_cluster, traffic, flows_raw_local, rand());

-- =============================================================================
-- 2. sessions — SessionEvent
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.sessions_local ON CLUSTER traffic_cluster (
  session_id     String,
  tenant_id      String,
  community_id   String,
  ts_start       Int64,
  ts_end         Int64,
  duration_ms    UInt32,
  packets_fwd    UInt32,
  packets_bwd    UInt32,
  bytes_fwd      UInt64,
  bytes_bwd      UInt64,
  event_id       String,
  run_id         String,
  feature_set_id String,
  event_ts       Int64,
  ingest_ts      Int64,
  kafka_ts       Int64,
  flink_out_ts   Int64,
  probe_id       String,
  src_ip         String,
  dst_ip         String,
  src_port       UInt32,
  dst_port       UInt32,
  protocol       UInt8,
  bytes_total    UInt64,
  up_down_ratio  Float32,
  num_pkts       UInt32,
  avg_payload    Float32,
  min_payload    UInt32,
  max_payload    UInt32,
  std_payload    Float32,
  mean_iat_ms    Float32,
  min_iat_ms     Float32,
  max_iat_ms     Float32,
  std_iat_ms     Float32,
  flags_syn      UInt32,
  flags_ack      UInt32,
  flags_fin      UInt32,
  flags_psh      UInt32,
  flags_rst      UInt32,
  dns_pkt_cnt    UInt32,
  tcp_pkt_cnt    UInt32,
  udp_pkt_cnt    UInt32,
  icmp_pkt_cnt   UInt32,
  has_syn        UInt8,
  has_fin        UInt8,
  has_rst        UInt8,
  is_established UInt8,
  evidence_count UInt32,
  flow_ids       Array(String),
  end_reason     String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/sessions', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(ts_start / 1000))
ORDER BY (tenant_id, ts_end, community_id)
TTL toDateTime(ts_end / 1000) + toIntervalDay(90)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.sessions ON CLUSTER traffic_cluster
AS traffic.sessions_local
ENGINE = Distributed(traffic_cluster, traffic, sessions_local, rand());

-- =============================================================================
-- 3. feature_stat — FeatureStat
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.feature_stat_local ON CLUSTER traffic_cluster (
  tenant_id                    String,
  run_id                       String,
  feature_set_id               String,
  schema_version               String,
  event_id                     String,
  object_type                  String,
  object_id                    String,
  community_id                 String,
  ts                           DateTime64(3),
  protocol                     UInt8,
  duration_ms                  UInt32,
  pps                          Float32,
  bps                          Float32,
  up_down_ratio                Float32,
  pktlen_mean                  Float32,
  pktlen_std                   Float32,
  iat_mean_ms                  Float32,
  iat_std_ms                   Float32,
  active_mean_ms               Float32,
  idle_mean_ms                 Float32,
  tcp_flag_syn_cnt             UInt16,
  tcp_flag_ack_cnt             UInt16,
  tcp_init_win_bytes_fwd       UInt32,
  tcp_init_win_bytes_bwd       UInt32,
  extra                        Array(Float32),
  ingest_ts                    DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/feature_stat', '{replica}')
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, object_type, object_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.feature_stat ON CLUSTER traffic_cluster
AS traffic.feature_stat_local
ENGINE = Distributed(traffic_cluster, traffic, feature_stat_local, rand());

-- =============================================================================
-- 4. feature_fp — FeatureFingerprint
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.feature_fp_local ON CLUSTER traffic_cluster (
  tenant_id          String,
  run_id             String,
  feature_set_id     String,
  event_id           String,
  community_id       String,
  session_id         String,
  ts                 DateTime64(3),
  is_encrypted       UInt8,
  tls_version        LowCardinality(String),
  ja3                LowCardinality(String),
  sni_hash           String,
  cert_sha256        String,
  cert_is_self_signed UInt8,
  pubkey_len         UInt16,
  hex_freq           Array(Float32),
  hex_ratio          Array(Float32),
  entropy_payload    Float32,
  chi_square_bfd     Float32,
  ingest_ts          DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/feature_fp', '{replica}')
PARTITION BY toDate(ts)
ORDER BY (tenant_id, ts, community_id, session_id)
TTL toDateTime(ts) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.feature_fp ON CLUSTER traffic_cluster
AS traffic.feature_fp_local
ENGINE = Distributed(traffic_cluster, traffic, feature_fp_local, cityHash64(tenant_id, community_id));

-- =============================================================================
-- 5. alerts — Alert
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.alerts_local ON CLUSTER traffic_cluster (
  tenant_id           String,
  alert_id            String,
  dedup_fingerprint   String,
  community_id        String,
  session_id          String,
  campaign_id         String,
  feature_set_id      String,
  src_ip              String,
  dst_ip              String,
  src_port            UInt32,
  dst_port            UInt32,
  protocol            UInt32,
  protocol_name       String,
  alert_type          String,
  labels              Array(String),
  severity            String,
  score               Float32,
  status              String,
  assignee            String,
  evidence_ids        Array(String),
  arkime_session_link String,
  feedback_label      String,
  feedback_count      UInt32,
  first_seen          Int64,
  last_seen           Int64,
  count               Int32,
  model_version       String,
  rule_version        String,
  state_version       UInt64,
  event_id            String,
  kafka_ts            Int64,
  flink_out_ts        Int64,
  created_at          Int64,
  updated_at          Int64
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/alerts', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(last_seen / 1000))
ORDER BY (tenant_id, last_seen, severity, alert_type, alert_id)
TTL toDateTime(last_seen / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.alerts ON CLUSTER traffic_cluster
AS traffic.alerts_local
ENGINE = Distributed(traffic_cluster, traffic, alerts_local, rand());

-- =============================================================================
-- 4. evidence — Evidence
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.evidence_local ON CLUSTER traffic_cluster (
  tenant_id          String,
  evidence_id        String,
  alert_id           String,
  ts                 Int64,
  type               String,
  summary            String,
  metrics_json       String,
  snippet_ref_json   String,
  arkime_link        String,
  confidence         Float32,
  event_id           String,
  ingest_ts          Int64,
  visualization_url  String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/evidence', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(ts / 1000))
ORDER BY (tenant_id, ts, alert_id, evidence_id)
TTL toDateTime(ts / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.evidence ON CLUSTER traffic_cluster
AS traffic.evidence_local
ENGINE = Distributed(traffic_cluster, traffic, evidence_local, rand());

-- =============================================================================
-- 5. campaigns — Campaign
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.campaigns_local ON CLUSTER traffic_cluster (
  tenant_id             String,
  campaign_id           String,
  ts_start              Int64,
  ts_end                Int64,
  entities              Array(String),
  alerts                Array(String),
  score                 Float32,
  summary               String,
  event_id              String,
  ingest_ts             Int64,
  campaign_type         String,
  attack_phases         Array(String),
  rule_ids              Array(String),
  model_ids             Array(String),
  header_event_id       String,
  header_tenant_id      String,
  header_run_id         String,
  header_event_ts       Int64,
  header_ingest_ts      Int64,
  header_probe_id       String,
  header_feature_set_id String
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/campaigns', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(ts_start / 1000))
ORDER BY (tenant_id, ts_end, campaign_id)
TTL toDateTime(ts_end / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.campaigns ON CLUSTER traffic_cluster
AS traffic.campaigns_local
ENGINE = Distributed(traffic_cluster, traffic, campaigns_local, rand());

-- =============================================================================
-- 6. pcap_index — PcapIndexMeta
-- =============================================================================
CREATE TABLE IF NOT EXISTS traffic.pcap_index_local ON CLUSTER traffic_cluster (
  tenant_id        String,
  probe_id         String,
  file_key         String,
  ts_start         Int64,
  ts_end           Int64,
  packet_count     UInt64,
  byte_count       UInt64,
  community_ids    Array(String),
  s3_path          String,
  compressed_size  UInt64,
  byte_size        UInt64,
  zstd_level       UInt8,
  sha256           String,
  bloom_filter_b64 String,
  flow_id          String,
  offset_start     UInt64,
  offset_end       UInt64,
  created_ts       Int64
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/pcap_index', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(ts_start / 1000))
ORDER BY (tenant_id, ts_start, probe_id, file_key)
TTL toDateTime(ts_start / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.pcap_index ON CLUSTER traffic_cluster
AS traffic.pcap_index_local
ENGINE = Distributed(traffic_cluster, traffic, pcap_index_local, rand());

-- =============================================================================
-- 扩展表 (device_logs, user_events, dlq_events) 见 01-extended.sql
-- =============================================================================
