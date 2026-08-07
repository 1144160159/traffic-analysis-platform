-- Canonical bootstrap counterpart of migration 202608082100_dashboard_task_compensation_v1.sql.
BEGIN;

ALTER TABLE dashboard_tasks DROP CONSTRAINT IF EXISTS dashboard_tasks_status_check;
ALTER TABLE dashboard_tasks ADD CONSTRAINT dashboard_tasks_status_check CHECK (status IN (
  'accepted','running','completed','partial','failed','cancelled',
  'compensating','compensated','compensation_partial','compensation_failed'
));
ALTER TABLE dashboard_task_event_inbox DROP CONSTRAINT IF EXISTS dashboard_task_event_inbox_event_type_check;
ALTER TABLE dashboard_task_event_inbox ADD CONSTRAINT dashboard_task_event_inbox_event_type_check CHECK (event_type IN (
  'traffic.dashboard.v1.TaskRequested','traffic.dashboard.v1.TaskResult',
  'traffic.dashboard.v1.TaskCompensationRequested','traffic.dashboard.v1.TaskCompensationResult'
));

CREATE TABLE IF NOT EXISTS dashboard_task_compensation_requests (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  expected_revision BIGINT NOT NULL CHECK (expected_revision > 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > expected_revision),
  action_id TEXT NOT NULL CHECK (action_id = 'dashboard-task-compensate'),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 2000), requested_by TEXT NOT NULL,
  trace_id TEXT NOT NULL, response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key), UNIQUE (tenant_id,task_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_requests_task
  ON dashboard_task_compensation_requests (tenant_id,task_id,created_at DESC);
CREATE TABLE IF NOT EXISTS dashboard_task_compensation_attempts (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_compensation_requests(request_event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL UNIQUE REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_attempts_ready
  ON dashboard_task_compensation_attempts (available_at,created_at,request_event_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_attempts_reclaim
  ON dashboard_task_compensation_attempts (locked_until,request_event_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS dashboard_task_compensation_receipts (
  task_id UUID PRIMARY KEY REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, request_event_id UUID NOT NULL UNIQUE,
  provider TEXT NOT NULL CHECK (provider <> ''), provider_receipt_id TEXT NOT NULL CHECK (provider_receipt_id <> ''),
  status TEXT NOT NULL CHECK (status IN ('compensated','compensation_partial','compensation_failed')),
  effect_state TEXT NOT NULL CHECK (effect_state IN ('confirmed','none','unknown')),
  compensated_effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(compensated_effect_ids) = 'array'),
  result JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, compensated_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,provider,provider_receipt_id),
  CHECK (status <> 'compensated' OR effect_state = 'confirmed'),
  CHECK (status <> 'compensated' OR jsonb_array_length(compensated_effect_ids) > 0),
  CHECK (status <> 'compensation_failed' OR effect_state <> 'unknown')
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_receipts_tenant_time
  ON dashboard_task_compensation_receipts (tenant_id,compensated_at DESC,task_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608082100','dashboard task provider-confirmed compensation lifecycle')
ON CONFLICT (version) DO NOTHING;
COMMIT;
