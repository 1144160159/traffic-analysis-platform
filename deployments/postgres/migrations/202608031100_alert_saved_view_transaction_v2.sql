-- T-PG-002 / F-ALERT-006: saved view business state, history, audit and outbox share one transaction.
BEGIN;

ALTER TABLE alert_saved_views
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS alert_saved_view_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key),
  UNIQUE (event_id)
);

CREATE TABLE IF NOT EXISTS alert_saved_view_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  name TEXT NOT NULL,
  filters JSONB NOT NULL,
  action TEXT NOT NULL,
  changed_by TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, view_id, revision),
  UNIQUE (event_id)
);

CREATE TABLE IF NOT EXISTS alert_saved_view_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL DEFAULT 'alert_saved_view',
  aggregate_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  locked_by TEXT NOT NULL DEFAULT '',
  locked_until TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_alert_saved_view_outbox_ready
  ON alert_saved_view_outbox (next_retry_at, occurred_at, outbox_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031100', 'alert saved view atomic history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;

COMMIT;
