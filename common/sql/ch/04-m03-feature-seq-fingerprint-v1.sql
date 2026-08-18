-- M03 N007 additive sequence/fingerprint persistence contract.
-- Repeatable expand migration: no DROP, DELETE, UPDATE, backfill or rebuild.

CREATE TABLE IF NOT EXISTS traffic.feature_seq_local ON CLUSTER traffic_cluster (
  tenant_id String,
  run_id String,
  feature_set_id String,
  event_id String,
  object_type LowCardinality(String),
  object_id String,
  community_id String,
  window_id String,
  ts_start DateTime64(3),
  ts_end DateTime64(3),
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
  feature_category LowCardinality(String) DEFAULT 'FEATURE_CATEGORY_UNSPECIFIED',
  availability LowCardinality(String) DEFAULT 'FEATURE_AVAILABILITY_UNSPECIFIED',
  schema_version String DEFAULT '',
  algorithm_version String DEFAULT '',
  value_unit String DEFAULT '',
  source_event_ids Array(String) DEFAULT [],
  evidence_ids Array(String) DEFAULT [],
  missing_fields Array(String) DEFAULT [],
  missing_reason String DEFAULT '',
  ingest_ts DateTime64(3) DEFAULT now64(3)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/feature_seq', '{replica}')
PARTITION BY toDate(ts_end)
ORDER BY (tenant_id, ts_end, community_id, object_type, object_id, window_id)
TTL toDateTime(ts_end) + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.feature_seq ON CLUSTER traffic_cluster
AS traffic.feature_seq_local
ENGINE = Distributed(traffic_cluster, traffic, feature_seq_local, cityHash64(tenant_id, community_id));

ALTER TABLE traffic.feature_fp_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS ja4 String DEFAULT '',
  ADD COLUMN IF NOT EXISTS sni String DEFAULT '',
  ADD COLUMN IF NOT EXISTS quic_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS transport_security LowCardinality(String) DEFAULT 'TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS raw_traffic_ref String DEFAULT '',
  ADD COLUMN IF NOT EXISTS feature_category LowCardinality(String) DEFAULT 'FEATURE_CATEGORY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS availability LowCardinality(String) DEFAULT 'FEATURE_AVAILABILITY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS algorithm_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS window_id String DEFAULT '',
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_reason String DEFAULT '';

ALTER TABLE traffic.feature_fp ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS ja4 String DEFAULT '',
  ADD COLUMN IF NOT EXISTS sni String DEFAULT '',
  ADD COLUMN IF NOT EXISTS quic_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS transport_security LowCardinality(String) DEFAULT 'TRANSPORT_SECURITY_PROTOCOL_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS raw_traffic_ref String DEFAULT '',
  ADD COLUMN IF NOT EXISTS feature_category LowCardinality(String) DEFAULT 'FEATURE_CATEGORY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS availability LowCardinality(String) DEFAULT 'FEATURE_AVAILABILITY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS algorithm_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS window_id String DEFAULT '',
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_reason String DEFAULT '';
