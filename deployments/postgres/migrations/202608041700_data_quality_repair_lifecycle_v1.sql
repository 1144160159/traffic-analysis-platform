-- T-DQ-001: immutable repair lifecycle commands and bounded replay evidence.
-- Expand only. Replay execution remains default-off and no runtime DDL is used.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_repair_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN (
    'planned','dry_run_completed','approval_submitted','approved','rejected',
    'execution_started','execution_completed','execution_partial','execution_failed','reconciled','cancelled'
  )),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,repair_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_history_lookup
  ON data_quality_repair_history (tenant_id,repair_id,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_repair_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL CHECK (action_id <> ''),
  operation TEXT NOT NULL CHECK (operation <> ''),
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES data_quality_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_requests_lookup
  ON data_quality_repair_requests (tenant_id,repair_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041700','immutable data quality repair dry-run approval replay and reconcile lifecycle')
ON CONFLICT (version) DO NOTHING;

COMMIT;
