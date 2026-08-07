-- T-PG-002 / F-FORENSICS-001 fresh-schema authority.
BEGIN;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS last_action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS last_trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_revision_positive;
ALTER TABLE tasks ADD CONSTRAINT tasks_revision_positive CHECK (revision > 0);
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_progress_range;
ALTER TABLE tasks ADD CONSTRAINT tasks_progress_range CHECK (progress BETWEEN 0 AND 100);

CREATE TABLE IF NOT EXISTS forensics_task_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  previous_status TEXT NOT NULL DEFAULT '',
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  snapshot JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,task_id,revision),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_history_lookup
  ON forensics_task_history (tenant_id,task_id,revision DESC);

CREATE TABLE IF NOT EXISTS forensics_task_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
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
CREATE INDEX IF NOT EXISTS idx_forensics_task_outbox_ready
  ON forensics_task_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_forensics_task_outbox_reclaim
  ON forensics_task_outbox (locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS forensics_task_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL,
  operation TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES forensics_task_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_requests_task
  ON forensics_task_requests (tenant_id,task_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031600','forensics task revision history audit outbox idempotency and soft retention')
ON CONFLICT (version) DO NOTHING;
COMMIT;
