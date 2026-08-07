-- T-FLINK-004 / WP-24
-- Expand-only, replayable projection for user-behavior anomalies. Runtime jobs
-- must never create or alter this schema.
--
-- Verify:
--   SELECT database,name,engine FROM system.tables
--   WHERE database='traffic' AND name IN
--     ('user_anomalies_v2_local','user_anomalies_v2');
--   SELECT tenant_id,count(),uniqExact(anomaly_id),max(event_version)
--   FROM traffic.user_anomalies_v2 FINAL GROUP BY tenant_id;
-- Rollback: stop the user-behavior candidate or point
-- CLICKHOUSE_USER_ANOMALY_TABLE back to the approved compatibility table.
-- Retain acknowledged v2 rows for replay/reconciliation evidence.

CREATE TABLE IF NOT EXISTS traffic.user_anomalies_v2_local (
  anomaly_id       String,
  tenant_id        String,
  user_id          String,
  username         String,
  detector_type    LowCardinality(String),
  severity         LowCardinality(String),
  score            Float32,
  description      String,
  detail_json      String CODEC(ZSTD(3)),
  source_ip1       String,
  source_ip2       String,
  detected_at      DateTime64(3, 'UTC'),
  event_version    UInt64,
  replay_id        String DEFAULT '',
  projected_at     DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/user_anomalies_v2_local',
  '{replica}',
  event_version
)
PARTITION BY toYYYYMM(detected_at)
ORDER BY (tenant_id, anomaly_id)
TTL toDateTime(detected_at) + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.user_anomalies_v2
AS traffic.user_anomalies_v2_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  user_anomalies_v2_local,
  cityHash64(tenant_id, anomaly_id)
);
