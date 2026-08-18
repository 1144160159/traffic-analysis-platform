-- M07 durable graph projection inbox, monotonic identity state and watermarks.
-- NebulaGraph remains a rebuildable read model; PostgreSQL owns delivery state.
BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS graph_projection_inbox_v1 (
  inbox_sequence      BIGSERIAL NOT NULL UNIQUE,
  event_id            TEXT PRIMARY KEY,
  tenant_id           TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  event_type          TEXT NOT NULL CHECK (event_type IN (
                        'graph.entity.upserted.v1',
                        'graph.relation.upserted.v1',
                        'graph.relation.revoked.v1')),
  schema_version      TEXT NOT NULL CHECK (schema_version='1'),
  projection_kind     TEXT NOT NULL CHECK (projection_kind IN ('entity','relation')),
  projection_id       TEXT NOT NULL,
  partition_key       TEXT NOT NULL,
  aggregate_type      TEXT NOT NULL,
  aggregate_id        TEXT NOT NULL,
  aggregate_version   BIGINT NOT NULL CHECK (aggregate_version>0),
  source_event_id     TEXT NOT NULL,
  source_system       TEXT NOT NULL,
  source_sha256       TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  projection_sha256   TEXT NOT NULL CHECK (projection_sha256 ~ '^[0-9a-f]{64}$'),
  revoked             BOOLEAN NOT NULL,
  source_topic        TEXT NOT NULL CHECK (source_topic='graph.projections.v1'),
  source_partition    INTEGER NOT NULL CHECK (source_partition>=0),
  source_offset       BIGINT NOT NULL CHECK (source_offset>=0),
  source_timestamp_ms BIGINT NOT NULL CHECK (source_timestamp_ms>0),
  raw_payload         BYTEA NOT NULL,
  payload_sha256      TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  projection_state    TEXT NOT NULL DEFAULT 'PENDING' CHECK (projection_state IN ('PENDING','APPLIED','DEAD')),
  attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  claim_token         UUID,
  claimed_at          TIMESTAMPTZ,
  claimed_by          TEXT NOT NULL DEFAULT '',
  last_error_code     TEXT NOT NULL DEFAULT '',
  last_error_detail   TEXT NOT NULL DEFAULT '',
  trace_id            TEXT NOT NULL,
  occurred_at_ms      BIGINT NOT NULL CHECK (occurred_at_ms>0),
  received_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at          TIMESTAMPTZ,
  UNIQUE (source_topic,source_partition,source_offset),
  UNIQUE (tenant_id,projection_kind,projection_id,aggregate_version),
  CHECK (octet_length(raw_payload)>0),
  CHECK ((claim_token IS NULL AND claimed_at IS NULL AND claimed_by='')
      OR (claim_token IS NOT NULL AND claimed_at IS NOT NULL AND claimed_by<>'')),
  CHECK ((projection_state='APPLIED' AND applied_at IS NOT NULL AND claim_token IS NULL AND last_error_code='')
      OR projection_state<>'APPLIED'),
  CHECK ((projection_kind='entity' AND event_type='graph.entity.upserted.v1') OR projection_kind='relation'),
  CHECK ((event_type='graph.relation.revoked.v1')=(projection_kind='relation' AND revoked))
);

CREATE INDEX IF NOT EXISTS idx_graph_projection_inbox_v1_ready
  ON graph_projection_inbox_v1(next_attempt_at,inbox_sequence)
  WHERE projection_state='PENDING';
CREATE INDEX IF NOT EXISTS idx_graph_projection_inbox_v1_partition_order
  ON graph_projection_inbox_v1(source_topic,source_partition,source_offset)
  WHERE projection_state<>'APPLIED';

CREATE TABLE IF NOT EXISTS graph_projection_current_v1 (
  tenant_id           TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  projection_kind     TEXT NOT NULL CHECK (projection_kind IN ('entity','relation')),
  projection_id       TEXT NOT NULL,
  aggregate_type      TEXT NOT NULL,
  aggregate_id        TEXT NOT NULL,
  aggregate_version   BIGINT NOT NULL CHECK (aggregate_version>0),
  event_id            TEXT NOT NULL REFERENCES graph_projection_inbox_v1(event_id) ON DELETE RESTRICT,
  source_event_id     TEXT NOT NULL,
  source_sha256       TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
  projection_sha256   TEXT NOT NULL CHECK (projection_sha256 ~ '^[0-9a-f]{64}$'),
  revoked             BOOLEAN NOT NULL,
  valid_from_ms       BIGINT NOT NULL CHECK (valid_from_ms>0),
  valid_to_ms         BIGINT NOT NULL DEFAULT 0 CHECK (valid_to_ms=0 OR valid_to_ms>=valid_from_ms),
  nebula_acknowledged BOOLEAN NOT NULL DEFAULT false,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,projection_kind,projection_id)
);

CREATE TABLE IF NOT EXISTS graph_projection_watermarks_v1 (
  source_topic        TEXT NOT NULL CHECK (source_topic='graph.projections.v1'),
  source_partition    INTEGER NOT NULL CHECK (source_partition>=0),
  source_offset       BIGINT NOT NULL CHECK (source_offset>=0),
  event_id            TEXT NOT NULL REFERENCES graph_projection_inbox_v1(event_id) ON DELETE RESTRICT,
  tenant_id           TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  projection_sha256   TEXT NOT NULL CHECK (projection_sha256 ~ '^[0-9a-f]{64}$'),
  source_timestamp_ms BIGINT NOT NULL CHECK (source_timestamp_ms>0),
  projected_at        TIMESTAMPTZ NOT NULL,
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,source_topic,source_partition)
);

CREATE TABLE IF NOT EXISTS graph_projection_dead_letters_v1 (
  dead_letter_id      UUID PRIMARY KEY,
  event_id            TEXT NOT NULL UNIQUE REFERENCES graph_projection_inbox_v1(event_id) ON DELETE RESTRICT,
  tenant_id           TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  source_topic        TEXT NOT NULL,
  source_partition    INTEGER NOT NULL CHECK (source_partition>=0),
  source_offset       BIGINT NOT NULL CHECK (source_offset>=0),
  payload_sha256      TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  error_code          TEXT NOT NULL,
  error_detail        TEXT NOT NULL,
  replay_authorized   BOOLEAN NOT NULL DEFAULT false,
  replay_authorized_by TEXT,
  replay_authorized_at TIMESTAMPTZ,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((replay_authorized AND replay_authorized_by IS NOT NULL AND replay_authorized_at IS NOT NULL)
      OR (NOT replay_authorized AND replay_authorized_by IS NULL AND replay_authorized_at IS NULL))
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608141900','M07 durable graph projection inbox and watermarks')
ON CONFLICT(version) DO NOTHING;

COMMIT;
