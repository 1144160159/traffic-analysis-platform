-- F-ALERT-004: frozen selection and durable batch-assignment acceptance.
BEGIN;

CREATE TABLE IF NOT EXISTS alert_assignment_selections (
  selection_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  token_sha256 TEXT NOT NULL CHECK (token_sha256 ~ '^[0-9a-f]{64}$'),
  snapshot_id TEXT NOT NULL CHECK (length(snapshot_id) BETWEEN 8 AND 256),
  selection_sha256 TEXT NOT NULL CHECK (selection_sha256 ~ '^[0-9a-f]{64}$'),
  items JSONB NOT NULL CHECK (jsonb_typeof(items) = 'array'),
  item_count INTEGER NOT NULL CHECK (item_count BETWEEN 1 AND 100),
  created_by TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  consumed_by_batch_id UUID,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, token_sha256),
  UNIQUE (tenant_id, selection_id),
  CHECK ((consumed_by_batch_id IS NULL) = (consumed_at IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_selections_expiry
  ON alert_assignment_selections (tenant_id, expires_at)
  WHERE consumed_by_batch_id IS NULL;

CREATE TABLE IF NOT EXISTS alert_assignment_selection_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  selection_id UUID NOT NULL,
  trace_id TEXT NOT NULL,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key),
  FOREIGN KEY (tenant_id, selection_id)
    REFERENCES alert_assignment_selections (tenant_id, selection_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_batches (
  batch_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  action_id TEXT NOT NULL CHECK (action_id = 'alert-batch-assignment-create'),
  selection_id UUID NOT NULL,
  selection_snapshot_id TEXT NOT NULL CHECK (length(selection_snapshot_id) BETWEEN 8 AND 256),
  selection_sha256 TEXT NOT NULL CHECK (selection_sha256 ~ '^[0-9a-f]{64}$'),
  assignee TEXT NOT NULL CHECK (length(assignee) BETWEEN 1 AND 128),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 4 AND 1000),
  status TEXT NOT NULL CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  total_count INTEGER NOT NULL CHECK (total_count BETWEEN 1 AND 100),
  accepted_count INTEGER NOT NULL DEFAULT 0 CHECK (accepted_count >= 0),
  applied_count INTEGER NOT NULL DEFAULT 0 CHECK (applied_count >= 0),
  conflicted_count INTEGER NOT NULL DEFAULT 0 CHECK (conflicted_count >= 0),
  forbidden_count INTEGER NOT NULL DEFAULT 0 CHECK (forbidden_count >= 0),
  failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
  requested_by TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ,
  UNIQUE (tenant_id, batch_id),
  UNIQUE (tenant_id, selection_id),
  FOREIGN KEY (tenant_id, selection_id)
    REFERENCES alert_assignment_selections (tenant_id, selection_id) ON DELETE RESTRICT,
  CHECK (accepted_count + applied_count + conflicted_count + forbidden_count + failed_count <= total_count)
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_batches_tenant_status
  ON alert_assignment_batches (tenant_id, status, updated_at DESC);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'fk_alert_assignment_selection_consumed_batch'
      AND conrelid = 'alert_assignment_selections'::regclass
  ) THEN
    ALTER TABLE alert_assignment_selections
      ADD CONSTRAINT fk_alert_assignment_selection_consumed_batch
      FOREIGN KEY (tenant_id, consumed_by_batch_id)
      REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS alert_assignment_batch_items (
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  position INTEGER NOT NULL CHECK (position BETWEEN 0 AND 99),
  alert_id TEXT NOT NULL,
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  status TEXT NOT NULL CHECK (status IN ('accepted','applied','conflicted','forbidden','failed','cancelled')),
  item_revision BIGINT NOT NULL DEFAULT 1 CHECK (item_revision > 0),
  resulting_state_version BIGINT NOT NULL DEFAULT 0 CHECK (resulting_state_version >= 0),
  previous_assignee TEXT NOT NULL DEFAULT '',
  resulting_assignee TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  event_id UUID NOT NULL UNIQUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, batch_id, alert_id),
  UNIQUE (tenant_id, batch_id, position),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_batch_items_status
  ON alert_assignment_batch_items (tenant_id, batch_id, status, position);

CREATE TABLE IF NOT EXISTS alert_assignment_batch_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action_id TEXT NOT NULL,
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, batch_id, revision),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_batch_item_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  alert_id TEXT NOT NULL,
  item_revision BIGINT NOT NULL CHECK (item_revision > 0),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  resulting_state_version BIGINT NOT NULL DEFAULT 0 CHECK (resulting_state_version >= 0),
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(detail) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, batch_id, alert_id, item_revision),
  FOREIGN KEY (tenant_id, batch_id, alert_id)
    REFERENCES alert_assignment_batch_items (tenant_id, batch_id, alert_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_batch_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL CHECK (event_type IN ('alert.batch-assignment.requested.v1','alert.assignment.changed.v1','alert.batch-assignment.cancelled.v1')),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL,
  published_at TIMESTAMPTZ,
  UNIQUE (tenant_id, batch_id, aggregate_version, event_type),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_batch_outbox_ready
  ON alert_assignment_batch_outbox (next_retry_at, occurred_at, outbox_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS alert_assignment_batch_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  batch_id UUID NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES alert_assignment_batch_outbox (event_id) ON DELETE RESTRICT,
  trace_id TEXT NOT NULL,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608091900', 'frozen selection and durable alert batch assignment v1')
ON CONFLICT (version) DO NOTHING;

COMMIT;
