-- F-ALERT-001 / WP-07
-- Expand: create the sharded alert feedback source and distributed read table.
-- Backfill: not required; existing tables and rows are retained.
-- Verify:
--   SELECT database,name,engine FROM system.tables
--   WHERE database='traffic' AND name IN ('alert_feedback_local','alert_feedback');
-- Cutover: deploy alert-service without ClickHouse DDL at startup after this
-- migration has been executed on the traffic_cluster.
-- Rollback: retain both tables; the change is additive and contains evidence.
-- Compatibility: retain the current rand() sharding expression in this
-- adoption migration. Its deterministic V2 replacement requires the separate
-- W6 shadow/backfill/cutover sequence and must not be smuggled into DDL exit.

CREATE TABLE IF NOT EXISTS traffic.alert_feedback_local (
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
  '/clickhouse/tables/{shard}/alert_feedback_local',
  '{replica}'
)
PARTITION BY toYYYYMM(created_at)
ORDER BY (tenant_id,alert_id,created_at)
TTL created_at+INTERVAL 365 DAY;

CREATE TABLE IF NOT EXISTS traffic.alert_feedback
AS traffic.alert_feedback_local
ENGINE=Distributed(
  traffic_cluster,
  traffic,
  alert_feedback_local,
  rand()
);
