-- Run-scoped single-node equivalent of the N012 projection columns.
-- Production uses the Replicated/Distributed engines in the authoritative
-- migration; this fixture exists only for isolated consumer semantics tests.
CREATE DATABASE IF NOT EXISTS traffic;

CREATE TABLE IF NOT EXISTS traffic.codex_ephemeral_alert_evidence_link_sentinel (
  marker String
) ENGINE=MergeTree ORDER BY marker;
INSERT INTO traffic.codex_ephemeral_alert_evidence_link_sentinel VALUES ('ephemeral-only');

CREATE TABLE IF NOT EXISTS traffic.alert_evidence_links_v1_local (
  tenant_id String, relation_id UUID, alert_id String, evidence_id String,
  evidence_type LowCardinality(String), status LowCardinality(String),
  relation_revision UInt64, event_id UUID, event_type LowCardinality(String),
  schema_version UInt16, source_store LowCardinality(String), object_bucket String,
  object_key String, object_version String, object_sha256 String, size_bytes UInt64,
  content_type String, manifest_revision UInt64, reason String, trace_id String,
  source_topic LowCardinality(String), source_partition Int32, source_offset Int64,
  source_timestamp DateTime64(3, 'UTC'), payload_sha256 FixedString(64),
  payload_json String, projected_at DateTime64(3, 'UTC') DEFAULT now64(3)
)
ENGINE=ReplacingMergeTree(relation_revision)
ORDER BY (tenant_id,alert_id,evidence_id);

CREATE TABLE IF NOT EXISTS traffic.alert_evidence_links_v1
AS traffic.alert_evidence_links_v1_local
ENGINE=ReplacingMergeTree(relation_revision)
ORDER BY (tenant_id,alert_id,evidence_id);
