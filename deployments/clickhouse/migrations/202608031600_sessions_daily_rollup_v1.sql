-- T-CH-005 / WP-22
-- Expand-only, versioned daily rollup retained beyond detailed session facts.
--
-- Apply this migration directly on every ClickHouse node with the authoritative
-- migration runner. Do not use ON CLUSTER. The materialized view begins
-- aggregating new local session rows only; historical backfill, shadow
-- reconciliation and read cutover require a separately approved bounded job.
--
-- Rollback: stop readers from selecting sessions_daily_rollup_v1 and detach the
-- materialized view only in an approved window. Retain acknowledged aggregate
-- rows throughout the rollback and observation window.

CREATE TABLE IF NOT EXISTS traffic.sessions_daily_rollup_v1_local (
  tenant_id          String,
  day                Date,
  protocol           UInt8,
  aggregate_version  UInt16,
  session_count      SimpleAggregateFunction(sum, UInt64),
  packet_count       SimpleAggregateFunction(sum, UInt64),
  byte_count         SimpleAggregateFunction(sum, UInt64),
  first_event_time   SimpleAggregateFunction(min, DateTime),
  last_event_time    SimpleAggregateFunction(max, DateTime)
)
ENGINE=ReplicatedAggregatingMergeTree(
  '/clickhouse/tables/{shard}/sessions_daily_rollup_v1_local',
  '{replica}'
)
PARTITION BY toYYYYMM(day)
ORDER BY (tenant_id,day,protocol,aggregate_version)
TTL toDateTime(day)+INTERVAL 365 DAY
SETTINGS index_granularity=8192;

CREATE TABLE IF NOT EXISTS traffic.sessions_daily_rollup_v1
AS traffic.sessions_daily_rollup_v1_local
ENGINE=Distributed(
  traffic_cluster,
  traffic,
  sessions_daily_rollup_v1_local,
  cityHash64(tenant_id,toString(day))
);

CREATE MATERIALIZED VIEW IF NOT EXISTS traffic.mv_sessions_daily_rollup_v1_local
TO traffic.sessions_daily_rollup_v1_local
AS
SELECT
  tenant_id,
  toDate(toDateTime(ts_end / 1000)) AS day,
  protocol,
  toUInt16(1) AS aggregate_version,
  count() AS session_count,
  sum(toUInt64(num_pkts)) AS packet_count,
  sum(bytes_total) AS byte_count,
  min(toDateTime(ts_start / 1000)) AS first_event_time,
  max(toDateTime(ts_end / 1000)) AS last_event_time
FROM traffic.sessions_local
GROUP BY tenant_id,day,protocol,aggregate_version;
