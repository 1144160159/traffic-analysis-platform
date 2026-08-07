-- T-CH-002 / WP-22
-- Expand-only deterministic shard candidate for alert feedback.
--
-- Apply this migration on every ClickHouse node with the authoritative runner.
-- Do not enable V2 projection until system.tables/system.columns/system.replicas
-- are healthy on every shard and replica. This migration performs no backfill,
-- read cutover, V1 write stop, or destructive cleanup.
--
-- Rollback: set MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V2_ENABLED=false. Retain
-- both V2 tables and V1 writes throughout the observation/rollback window.

CREATE TABLE IF NOT EXISTS traffic.alert_feedback_v2_local (
  feedback_id       String,
  alert_id          String,
  tenant_id         String,
  user_id           String,
  label             LowCardinality(String),
  reason_code       String,
  comment           String,
  add_to_whitelist  UInt8,
  alert_type        String,
  severity          LowCardinality(String),
  model_version     String,
  rule_version      String,
  created_at        DateTime
)
ENGINE=ReplicatedMergeTree(
  '/clickhouse/tables/{shard}/alert_feedback_v2_local',
  '{replica}'
)
PARTITION BY toYYYYMM(created_at)
ORDER BY (tenant_id,alert_id,feedback_id)
TTL created_at+INTERVAL 365 DAY;

CREATE TABLE IF NOT EXISTS traffic.alert_feedback_v2
AS traffic.alert_feedback_v2_local
ENGINE=Distributed(
  traffic_cluster,
  traffic,
  alert_feedback_v2_local,
  cityHash64(tenant_id,alert_id)
);
