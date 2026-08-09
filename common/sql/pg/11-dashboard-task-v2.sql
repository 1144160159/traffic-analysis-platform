-- Canonical bootstrap counterpart of migration 202608031620_dashboard_task_v2.sql.
BEGIN;

CREATE TABLE IF NOT EXISTS dashboard_tasks (
  task_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  action_id TEXT NOT NULL, task_type TEXT NOT NULL, target TEXT NOT NULL,
  priority TEXT NOT NULL CHECK (priority IN ('low','medium','high','critical')),
  status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0), snapshot_id TEXT NOT NULL,
  reason TEXT NOT NULL, requested_by TEXT NOT NULL, trace_id TEXT NOT NULL,
  input JSONB NOT NULL DEFAULT '{}'::jsonb, result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, cancelled_at TIMESTAMPTZ,
  UNIQUE (tenant_id,task_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_tasks_tenant_status_time
  ON dashboard_tasks (tenant_id,status,updated_at DESC,task_id);

CREATE TABLE IF NOT EXISTS dashboard_task_history (
  history_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0), action_id TEXT NOT NULL,
  previous_status TEXT NOT NULL DEFAULT '', resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  snapshot JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,task_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_history_lookup
  ON dashboard_task_history (tenant_id,task_id,revision DESC);

CREATE TABLE IF NOT EXISTS dashboard_task_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''), payload JSONB NOT NULL,
  trace_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_outbox_ready
  ON dashboard_task_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_outbox_reclaim
  ON dashboard_task_outbox (locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS dashboard_task_requests (
  tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64), action_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  trace_id TEXT NOT NULL, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,idempotency_key), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_requests_task
  ON dashboard_task_requests (tenant_id,task_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY, description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031620','dashboard durable task history audit outbox and idempotent command receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
