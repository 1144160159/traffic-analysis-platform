-- T-PG-002 / IAM: user preference state, audit, history, idempotency and UserEvent outbox share one transaction.
-- Expand only. Apply before the auth-service candidate starts the outbox worker.
-- Rollback: stop the writer/worker and retain all additive columns/tables for replay and reconciliation.

BEGIN;

ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS user_settings_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  revision BIGINT NOT NULL,
  action_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,user_id,category,revision)
);

CREATE TABLE IF NOT EXISTS user_settings_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1,
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_ready ON user_settings_outbox(next_retry_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_reclaim ON user_settings_outbox(locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS user_settings_requests (
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL,
  action_id TEXT NOT NULL,
  resulting_revision BIGINT NOT NULL,
  event_id UUID NOT NULL REFERENCES user_settings_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,user_id,category,idempotency_key)
);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031500','IAM user settings atomic history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;

COMMIT;
