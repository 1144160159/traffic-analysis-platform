-- F-CAMPAIGN-001 / WP-08 and T-CH-001 / WP-22
-- Expand-only immutable event projection for campaign aggregate and membership
-- streams. This is deliberately separate from the legacy traffic.campaigns
-- table so W6 shadow/backfill/reconcile/cutover remains reversible.
--
-- Verify:
--   SELECT database,name,engine FROM system.tables
--   WHERE database='traffic' AND name IN
--     ('campaign_projection_events_v2_local','campaign_projection_events_v2');
--   SELECT tenant_id,stream,count(),uniqExact(event_id)
--   FROM traffic.campaign_projection_events_v2 FINAL
--   GROUP BY tenant_id,stream;
-- Rollback: disable CAMPAIGN_TARGET_PROJECTION_V2_ENABLED. Retain the table as
-- immutable replay evidence; do not delete acknowledged projections.

CREATE TABLE IF NOT EXISTS traffic.campaign_projection_events_v2_local (
  projection_id      FixedString(64),
  event_id            UUID,
  tenant_id           String,
  stream              LowCardinality(String),
  projection_key      String,
  projection_version  UInt64,
  campaign_id         String,
  relation_id         String,
  alert_id            String,
  event_type          LowCardinality(String),
  schema_version      UInt16,
  trace_id            String,
  received_at         DateTime64(3, 'UTC'),
  payload             String CODEC(ZSTD(3)),
  projection_sha256   FixedString(64),
  projected_at        DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE=ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/campaign_projection_events_v2_local',
  '{replica}',
  projected_at
)
PARTITION BY toYYYYMM(received_at)
ORDER BY (tenant_id,projection_key,projection_version,event_id)
TTL toDateTime(received_at)+INTERVAL 180 DAY
SETTINGS index_granularity=8192;

CREATE TABLE IF NOT EXISTS traffic.campaign_projection_events_v2
AS traffic.campaign_projection_events_v2_local
ENGINE=Distributed(
  traffic_cluster,
  traffic,
  campaign_projection_events_v2_local,
  cityHash64(tenant_id,projection_key)
);
