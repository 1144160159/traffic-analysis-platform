-- Canonical bootstrap counterpart of migration 202608041930_dashboard_task_execution_pipeline_v1.sql.
BEGIN;

CREATE TABLE IF NOT EXISTS dashboard_task_execution_attempts (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  task_id UUID NOT NULL UNIQUE REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_attempts_ready
  ON dashboard_task_execution_attempts (available_at,created_at,request_event_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_attempts_reclaim
  ON dashboard_task_execution_attempts (locked_until,request_event_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS dashboard_task_execution_receipts (
  task_id UUID PRIMARY KEY REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, request_event_id UUID NOT NULL UNIQUE,
  provider TEXT NOT NULL CHECK (provider <> ''),
  provider_receipt_id TEXT NOT NULL CHECK (provider_receipt_id <> ''),
  status TEXT NOT NULL CHECK (status IN ('completed','partial','failed')),
  effect_state TEXT NOT NULL CHECK (effect_state IN ('confirmed','none','unknown')),
  effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(effect_ids) = 'array'),
  result JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, executed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,provider,provider_receipt_id),
  CHECK (status <> 'completed' OR effect_state = 'confirmed'),
  CHECK (status <> 'completed' OR jsonb_array_length(effect_ids) > 0)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_receipts_tenant_time
  ON dashboard_task_execution_receipts (tenant_id,executed_at DESC,task_id);

CREATE TABLE IF NOT EXISTS dashboard_task_event_inbox (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL CHECK (event_type IN (
    'traffic.dashboard.v1.TaskRequested','traffic.dashboard.v1.TaskResult'
  )),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, kafka_topic TEXT NOT NULL CHECK (kafka_topic <> ''),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_event_inbox_task
  ON dashboard_task_event_inbox (tenant_id,task_id,aggregate_version,event_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041930','dashboard task Kafka execution inbox provider receipts and terminal result outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
