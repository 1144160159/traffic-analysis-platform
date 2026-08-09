-- T-PG-002: notification rule command, history, audit and outbox boundary.
BEGIN;

ALTER TABLE notification_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

ALTER TABLE notification_escalation_policies
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

ALTER TABLE alert_notification_settings
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

ALTER TABLE notification_silence_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS notification_governance_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  changed_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, aggregate_type, aggregate_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_history_aggregate
  ON notification_governance_history (tenant_id,aggregate_type,aggregate_id,revision DESC);

CREATE TABLE IF NOT EXISTS notification_governance_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_outbox_ready
  ON notification_governance_outbox (next_retry_at,occurred_at,outbox_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

CREATE TABLE IF NOT EXISTS notification_governance_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  action_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES notification_governance_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031300','T-PG-002 notification rule atomic command history outbox idempotency')
ON CONFLICT (version) DO NOTHING;

COMMIT;
