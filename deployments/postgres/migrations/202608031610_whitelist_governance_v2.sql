-- F-WHITELIST-001: durable whitelist governance command boundary.
-- Expand-only migration. Existing routes and whitelist columns remain compatible.
BEGIN;

ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS last_action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS last_trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_version_positive;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_version_positive CHECK (version > 0);

CREATE INDEX IF NOT EXISTS idx_whitelist_visible_updated
  ON whitelist (tenant_id,updated_at DESC,id) WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS whitelist_entry_versions (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  version BIGINT NOT NULL CHECK (version > 0),
  action_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  actor_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  previous_status TEXT NOT NULL DEFAULT '',
  resulting_status TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,entry_id,version)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_entry_versions_lookup
  ON whitelist_entry_versions (tenant_id,entry_id,version DESC);

CREATE TABLE IF NOT EXISTS whitelist_event_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version = 2),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_whitelist_event_outbox_ready
  ON whitelist_event_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_whitelist_event_outbox_reclaim
  ON whitelist_event_outbox (locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS whitelist_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  expected_version BIGINT NOT NULL CHECK (expected_version >= 0),
  resulting_version BIGINT NOT NULL CHECK (resulting_version > 0),
  event_id UUID NOT NULL REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_command_requests_entry
  ON whitelist_command_requests (tenant_id,entry_id,created_at DESC);

CREATE TABLE IF NOT EXISTS whitelist_rule_effects (
  tenant_id TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  entry_version BIGINT NOT NULL CHECK (entry_version > 0),
  event_id UUID NOT NULL REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  desired_state TEXT NOT NULL CHECK (desired_state IN ('effective','revoked')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','failed')),
  rule_revision TEXT NOT NULL DEFAULT '',
  ack_event_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  acknowledged_at TIMESTAMPTZ,
  PRIMARY KEY (tenant_id,entry_id,entry_version),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_rule_effects_pending
  ON whitelist_rule_effects (requested_at,entry_id) WHERE status='pending';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031610','whitelist immutable versions audit outbox idempotency rule ACK and soft archive')
ON CONFLICT (version) DO NOTHING;

COMMIT;
