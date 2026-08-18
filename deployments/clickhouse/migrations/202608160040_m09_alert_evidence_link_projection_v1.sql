-- T1-M09-N012 / P027-P028
-- Replayable ClickHouse projection for PostgreSQL-authoritative alert/evidence
-- relationship events.  It is additive and is never an object-access authority.
-- Rollback: stop the consumer and retain all projected revisions for replay.

CREATE TABLE IF NOT EXISTS traffic.alert_evidence_links_v1_local (
  tenant_id            String,
  relation_id          UUID,
  alert_id             String,
  evidence_id          String,
  evidence_type        LowCardinality(String),
  status               LowCardinality(String),
  relation_revision    UInt64,
  event_id             UUID,
  event_type           LowCardinality(String),
  schema_version       UInt16,
  source_store         LowCardinality(String),
  object_bucket        String,
  object_key           String,
  object_version       String,
  object_sha256        String,
  size_bytes           UInt64,
  content_type         String,
  manifest_revision    UInt64,
  reason               String,
  trace_id             String,
  source_topic         LowCardinality(String),
  source_partition     Int32,
  source_offset        Int64,
  source_timestamp     DateTime64(3, 'UTC'),
  payload_sha256       FixedString(64),
  payload_json         String CODEC(ZSTD(3)),
  projected_at         DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE=ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/alert_evidence_links_v1_local',
  '{replica}',
  relation_revision
)
PARTITION BY toYYYYMM(projected_at)
ORDER BY (tenant_id,alert_id,evidence_id)
TTL toDateTime(projected_at)+INTERVAL 365 DAY
SETTINGS index_granularity=8192;

CREATE TABLE IF NOT EXISTS traffic.alert_evidence_links_v1
AS traffic.alert_evidence_links_v1_local
ENGINE=Distributed(
  traffic_cluster,
  traffic,
  alert_evidence_links_v1_local,
  cityHash64(tenant_id,alert_id)
);
