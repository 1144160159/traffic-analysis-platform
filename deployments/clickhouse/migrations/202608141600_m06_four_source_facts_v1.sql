-- M06 replayable source facts for the four canonical consumer rails.
--
-- These are additive tables. They retain exact Kafka coordinates and original
-- protobuf bytes; no writer is enabled by applying this migration alone.

CREATE TABLE IF NOT EXISTS traffic.source_flow_facts_v1_local (
  rail                       LowCardinality(String),
  tenant_id                  String,
  aggregate_id               String,
  event_id                   String,
  event_time_ms              Int64,
  ingest_time_ms             Int64,
  schema_version             LowCardinality(String),
  source_topic               LowCardinality(String),
  source_partition           Int32,
  source_offset              Int64,
  source_timestamp_ms        Int64,
  source_payload_sha256      FixedString(64),
  source_version             UInt64,
  projection_identity        FixedString(64),
  source_quality_receipt_id  String,
  payload_base64             String,
  projection_hash            FixedString(64),
  inserted_at                DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/source_flow_facts_v1_local',
  '{replica}',
  source_version
)
PARTITION BY toYYYYMMDD(toDateTime(event_time_ms / 1000))
ORDER BY (tenant_id, projection_identity)
TTL toDateTime(event_time_ms / 1000) + INTERVAL 90 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.source_flow_facts_v1
AS traffic.source_flow_facts_v1_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  source_flow_facts_v1_local,
  cityHash64(tenant_id, projection_identity)
);

CREATE TABLE IF NOT EXISTS traffic.source_asset_facts_v1_local (
  rail                       LowCardinality(String),
  tenant_id                  String,
  aggregate_id               String,
  event_id                   String,
  event_time_ms              Int64,
  ingest_time_ms             Int64,
  schema_version             LowCardinality(String),
  source_topic               LowCardinality(String),
  source_partition           Int32,
  source_offset              Int64,
  source_timestamp_ms        Int64,
  source_payload_sha256      FixedString(64),
  source_version             UInt64,
  projection_identity        FixedString(64),
  source_quality_receipt_id  String,
  payload_base64             String,
  projection_hash            FixedString(64),
  inserted_at                DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/source_asset_facts_v1_local',
  '{replica}',
  source_version
)
PARTITION BY toYYYYMMDD(toDateTime(event_time_ms / 1000))
ORDER BY (tenant_id, projection_identity)
TTL toDateTime(event_time_ms / 1000) + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.source_asset_facts_v1
AS traffic.source_asset_facts_v1_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  source_asset_facts_v1_local,
  cityHash64(tenant_id, projection_identity)
);

CREATE TABLE IF NOT EXISTS traffic.source_device_log_facts_v1_local (
  rail                       LowCardinality(String),
  tenant_id                  String,
  aggregate_id               String,
  event_id                   String,
  event_time_ms              Int64,
  ingest_time_ms             Int64,
  schema_version             LowCardinality(String),
  source_topic               LowCardinality(String),
  source_partition           Int32,
  source_offset              Int64,
  source_timestamp_ms        Int64,
  source_payload_sha256      FixedString(64),
  source_version             UInt64,
  projection_identity        FixedString(64),
  source_quality_receipt_id  String,
  payload_base64             String,
  projection_hash            FixedString(64),
  inserted_at                DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/source_device_log_facts_v1_local',
  '{replica}',
  source_version
)
PARTITION BY toYYYYMMDD(toDateTime(event_time_ms / 1000))
ORDER BY (tenant_id, projection_identity)
TTL toDateTime(event_time_ms / 1000) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.source_device_log_facts_v1
AS traffic.source_device_log_facts_v1_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  source_device_log_facts_v1_local,
  cityHash64(tenant_id, projection_identity)
);

CREATE TABLE IF NOT EXISTS traffic.source_user_behavior_facts_v1_local (
  rail                       LowCardinality(String),
  tenant_id                  String,
  aggregate_id               String,
  event_id                   String,
  event_time_ms              Int64,
  ingest_time_ms             Int64,
  schema_version             LowCardinality(String),
  source_topic               LowCardinality(String),
  source_partition           Int32,
  source_offset              Int64,
  source_timestamp_ms        Int64,
  source_payload_sha256      FixedString(64),
  source_version             UInt64,
  projection_identity        FixedString(64),
  source_quality_receipt_id  String,
  payload_base64             String,
  projection_hash            FixedString(64),
  inserted_at                DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/source_user_behavior_facts_v1_local',
  '{replica}',
  source_version
)
PARTITION BY toYYYYMMDD(toDateTime(event_time_ms / 1000))
ORDER BY (tenant_id, projection_identity)
TTL toDateTime(event_time_ms / 1000) + INTERVAL 180 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.source_user_behavior_facts_v1
AS traffic.source_user_behavior_facts_v1_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  source_user_behavior_facts_v1_local,
  cityHash64(tenant_id, projection_identity)
);
