-- M03 SessionEvent / FeatureStat additive persistence contract.
--
-- Expand only: safe to run repeatedly before the producer deployment.  This
-- migration intentionally performs no UPDATE, DELETE, DROP or table rebuild.
-- Nullable source_watermark_ms means the producing event carried no observed
-- Flink operator watermark; event_time_* remains a separate factual boundary.

ALTER TABLE traffic.sessions_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS event_schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS aggregate_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS identity_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS session_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_watermark_ms Nullable(Int64) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS completeness LowCardinality(String) DEFAULT 'SESSION_COMPLETENESS_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS is_partial UInt8 DEFAULT 1,
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [];

ALTER TABLE traffic.sessions ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS event_schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS aggregate_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS identity_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS session_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_watermark_ms Nullable(Int64) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS completeness LowCardinality(String) DEFAULT 'SESSION_COMPLETENESS_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS is_partial UInt8 DEFAULT 1,
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [];

ALTER TABLE traffic.feature_stat_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS event_schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS aggregate_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_watermark_ms Nullable(Int64) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS feature_category LowCardinality(String) DEFAULT 'FEATURE_CATEGORY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS availability LowCardinality(String) DEFAULT 'FEATURE_AVAILABILITY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS algorithm_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS window_id String DEFAULT '',
  ADD COLUMN IF NOT EXISTS value_unit String DEFAULT '',
  ADD COLUMN IF NOT EXISTS is_partial UInt8 DEFAULT 1,
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_reason String DEFAULT '';

ALTER TABLE traffic.feature_stat ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS event_schema_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS aggregate_version UInt64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_start_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS event_time_end_ms Int64 DEFAULT 0,
  ADD COLUMN IF NOT EXISTS source_watermark_ms Nullable(Int64) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS source_event_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS evidence_ids Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS feature_category LowCardinality(String) DEFAULT 'FEATURE_CATEGORY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS availability LowCardinality(String) DEFAULT 'FEATURE_AVAILABILITY_UNSPECIFIED',
  ADD COLUMN IF NOT EXISTS algorithm_version String DEFAULT '',
  ADD COLUMN IF NOT EXISTS window_id String DEFAULT '',
  ADD COLUMN IF NOT EXISTS value_unit String DEFAULT '',
  ADD COLUMN IF NOT EXISTS is_partial UInt8 DEFAULT 1,
  ADD COLUMN IF NOT EXISTS missing_fields Array(String) DEFAULT [],
  ADD COLUMN IF NOT EXISTS missing_reason String DEFAULT '';
