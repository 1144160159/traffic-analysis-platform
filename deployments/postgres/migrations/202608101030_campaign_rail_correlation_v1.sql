-- T1-M07-N018: dual-rail campaign projection and correlation foundations.
-- Additive only. Neither rail is enabled by this migration.

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_proto_projection_inbox_v1 (
  event_id                 UUID PRIMARY KEY,
  tenant_id                TEXT NOT NULL CHECK (btrim(tenant_id) <> '' AND lower(btrim(tenant_id)) <> 'unknown'),
  campaign_id              TEXT NOT NULL CHECK (btrim(campaign_id) <> ''),
  campaign_type            TEXT NOT NULL CHECK (btrim(campaign_type) <> ''),
  event_time_start_ms      BIGINT NOT NULL CHECK (event_time_start_ms > 0),
  event_time_end_ms        BIGINT NOT NULL CHECK (event_time_end_ms >= event_time_start_ms),
  payload_sha256           TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  payload_protobuf         BYTEA NOT NULL,
  source_topic             TEXT NOT NULL CHECK (source_topic = 'campaigns.v1'),
  source_partition         INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset            BIGINT NOT NULL CHECK (source_offset >= 0),
  received_at              TIMESTAMPTZ NOT NULL,
  state                    TEXT NOT NULL CHECK (state IN ('received','applied')),
  applied_at               TIMESTAMPTZ,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_topic,source_partition,source_offset),
  CHECK ((state='received' AND applied_at IS NULL) OR (state='applied' AND applied_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_campaign_proto_projection_inbox_v1_tenant_time
  ON campaign_proto_projection_inbox_v1(tenant_id,event_time_end_ms DESC,event_id);

CREATE TABLE IF NOT EXISTS campaign_proto_projection_current_v1 (
  tenant_id                TEXT NOT NULL,
  campaign_id              TEXT NOT NULL,
  event_id                 UUID NOT NULL UNIQUE REFERENCES campaign_proto_projection_inbox_v1(event_id) ON DELETE RESTRICT,
  payload_sha256           TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  event_time_start_ms      BIGINT NOT NULL,
  event_time_end_ms        BIGINT NOT NULL,
  campaign_type            TEXT NOT NULL,
  score                    DOUBLE PRECISION NOT NULL CHECK (score >= 0 AND score <= 1),
  source_topic             TEXT NOT NULL,
  source_partition         INTEGER NOT NULL,
  source_offset            BIGINT NOT NULL,
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,campaign_id),
  CHECK (event_time_start_ms > 0 AND event_time_end_ms >= event_time_start_ms)
);

CREATE TABLE IF NOT EXISTS campaign_consumer_readiness_v1 (
  rail_id                  TEXT NOT NULL CHECK (rail_id IN ('cep_protobuf','aggregate_json_v2','membership_json_v2')),
  candidate_sha256         TEXT NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  source_topic             TEXT NOT NULL,
  consumer_group           TEXT NOT NULL,
  state                    TEXT NOT NULL CHECK (state IN ('ready','stopped')),
  last_event_id            UUID,
  last_source_partition    INTEGER CHECK (last_source_partition IS NULL OR last_source_partition >= 0),
  last_source_offset       BIGINT CHECK (last_source_offset IS NULL OR last_source_offset >= 0),
  observed_at              TIMESTAMPTZ NOT NULL,
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (rail_id,candidate_sha256,source_topic,consumer_group),
  CHECK ((state='ready' AND last_event_id IS NOT NULL AND last_source_partition IS NOT NULL AND last_source_offset IS NOT NULL)
      OR state='stopped')
);

CREATE TABLE IF NOT EXISTS campaign_rail_correlation_v1 (
  tenant_id                TEXT NOT NULL,
  correlation_id           UUID NOT NULL,
  cep_campaign_id          TEXT NOT NULL,
  cep_event_id             UUID NOT NULL REFERENCES campaign_proto_projection_inbox_v1(event_id) ON DELETE RESTRICT,
  aggregate_campaign_id    TEXT,
  aggregate_event_id       UUID,
  relation_ids             UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
  membership_event_ids     UUID[] NOT NULL DEFAULT ARRAY[]::UUID[],
  relation_revision        BIGINT NOT NULL DEFAULT 0 CHECK (relation_revision >= 0),
  state                    TEXT NOT NULL CHECK (state IN ('cep_only','correlated','conflict','revoked')),
  correlation_version      BIGINT NOT NULL CHECK (correlation_version > 0),
  correlation_sha256       TEXT NOT NULL CHECK (correlation_sha256 ~ '^[0-9a-f]{64}$'),
  correlation_key_sha256   TEXT NOT NULL CHECK (correlation_key_sha256 ~ '^[0-9a-f]{64}$'),
  as_of                    TIMESTAMPTZ NOT NULL,
  confidence               DOUBLE PRECISION NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  source_watermarks        JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks)='object'),
  partial_reasons          TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  first_seen_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,correlation_id),
  UNIQUE (tenant_id,cep_campaign_id,correlation_version),
  CHECK ((state='correlated' AND aggregate_campaign_id IS NOT NULL AND aggregate_event_id IS NOT NULL
          AND cardinality(relation_ids)>0 AND cardinality(membership_event_ids)>0 AND relation_revision>0)
      OR (state<>'correlated' AND aggregate_campaign_id IS NULL AND aggregate_event_id IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_campaign_rail_correlation_v1_lookup
  ON campaign_rail_correlation_v1(tenant_id,cep_campaign_id,correlation_version DESC);

CREATE TABLE IF NOT EXISTS campaign_rail_reconcile_runs_v1 (
  run_id                    UUID PRIMARY KEY,
  tenant_id                 TEXT NOT NULL CHECK (btrim(tenant_id) <> ''),
  window_from               TIMESTAMPTZ NOT NULL,
  window_through            TIMESTAMPTZ NOT NULL,
  as_of                     TIMESTAMPTZ NOT NULL,
  max_items                 INTEGER NOT NULL CHECK (max_items BETWEEN 1 AND 100000),
  cep_count                 BIGINT NOT NULL CHECK (cep_count >= 0),
  aggregate_count           BIGINT NOT NULL CHECK (aggregate_count >= 0),
  correlated_count          BIGINT NOT NULL CHECK (correlated_count >= 0),
  missing_count             BIGINT NOT NULL CHECK (missing_count >= 0),
  conflict_count            BIGINT NOT NULL CHECK (conflict_count >= 0),
  extra_count               BIGINT NOT NULL CHECK (extra_count >= 0),
  manifest_sha256           TEXT NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
  state                     TEXT NOT NULL CHECK (state IN ('exact','diff','budget_exceeded')),
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (window_from < window_through AND window_through <= as_of)
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608101030','M07 campaign protobuf/JSON dual-rail correlation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
