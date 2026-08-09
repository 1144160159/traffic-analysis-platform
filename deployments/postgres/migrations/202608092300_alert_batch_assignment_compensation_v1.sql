-- F-ALERT-004: revision-safe per-item batch-assignment compensation.
BEGIN;

ALTER TABLE alert_assignment_batch_items
  ADD COLUMN IF NOT EXISTS previous_status TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_assignment_batch_items
  ADD COLUMN IF NOT EXISTS resulting_status TEXT NOT NULL DEFAULT 'assigned';

ALTER TABLE alert_assignment_states
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'assigned';
ALTER TABLE alert_assignment_states
  ADD COLUMN IF NOT EXISTS previous_status TEXT NOT NULL DEFAULT '';

ALTER TABLE alert_assignment_state_history
  ADD COLUMN IF NOT EXISTS previous_status TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_assignment_state_history
  ADD COLUMN IF NOT EXISTS resulting_status TEXT NOT NULL DEFAULT 'assigned';

ALTER TABLE alert_assignment_projection_receipts
  ADD COLUMN IF NOT EXISTS previous_status TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_assignment_projection_receipts
  ADD COLUMN IF NOT EXISTS resulting_status TEXT NOT NULL DEFAULT 'assigned';

ALTER TABLE alert_assignment_batch_outbox
  ADD COLUMN IF NOT EXISTS aggregate_type TEXT NOT NULL DEFAULT 'alert_assignment_batch';
ALTER TABLE alert_assignment_batch_outbox
  ADD COLUMN IF NOT EXISTS aggregate_id TEXT NOT NULL DEFAULT '';
UPDATE alert_assignment_batch_outbox
SET aggregate_id = batch_id::text
WHERE aggregate_id = '';
ALTER TABLE alert_assignment_batch_outbox
  DROP CONSTRAINT IF EXISTS alert_assignment_batch_outbox_aggregate_identity_check;
ALTER TABLE alert_assignment_batch_outbox
  ADD CONSTRAINT alert_assignment_batch_outbox_aggregate_identity_check
  CHECK (length(aggregate_type) > 0 AND length(aggregate_id) > 0) NOT VALID;
ALTER TABLE alert_assignment_batch_outbox
  VALIDATE CONSTRAINT alert_assignment_batch_outbox_aggregate_identity_check;

ALTER TABLE alert_assignment_batch_inbox
  ADD COLUMN IF NOT EXISTS aggregate_type TEXT NOT NULL DEFAULT 'alert_assignment_batch';
ALTER TABLE alert_assignment_batch_inbox
  ADD COLUMN IF NOT EXISTS aggregate_id TEXT NOT NULL DEFAULT '';
UPDATE alert_assignment_batch_inbox
SET aggregate_id = batch_id::text
WHERE aggregate_id = '';
ALTER TABLE alert_assignment_batch_inbox
  DROP CONSTRAINT IF EXISTS alert_assignment_batch_inbox_aggregate_identity_check;
ALTER TABLE alert_assignment_batch_inbox
  ADD CONSTRAINT alert_assignment_batch_inbox_aggregate_identity_check
  CHECK (length(aggregate_type) > 0 AND length(aggregate_id) > 0) NOT VALID;
ALTER TABLE alert_assignment_batch_inbox
  VALIDATE CONSTRAINT alert_assignment_batch_inbox_aggregate_identity_check;

CREATE TABLE IF NOT EXISTS alert_assignment_compensation_requests (
  request_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  action_id TEXT NOT NULL CHECK (action_id = 'alert-batch-assignment-compensate'),
  expected_batch_revision BIGINT NOT NULL CHECK (expected_batch_revision > 0),
  status TEXT NOT NULL CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  total_count INTEGER NOT NULL CHECK (total_count BETWEEN 1 AND 100),
  accepted_count INTEGER NOT NULL DEFAULT 0 CHECK (accepted_count >= 0),
  compensated_count INTEGER NOT NULL DEFAULT 0 CHECK (compensated_count >= 0),
  conflicted_count INTEGER NOT NULL DEFAULT 0 CHECK (conflicted_count >= 0),
  failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  requested_by TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id, idempotency_key),
  UNIQUE (tenant_id, batch_id),
  UNIQUE (tenant_id, request_id),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT,
  CHECK (accepted_count + compensated_count + conflicted_count + failed_count <= total_count)
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_compensation_requests_status
  ON alert_assignment_compensation_requests (tenant_id, status, updated_at DESC);

CREATE TABLE IF NOT EXISTS alert_assignment_compensation_items (
  tenant_id TEXT NOT NULL,
  request_id UUID NOT NULL,
  batch_id UUID NOT NULL,
  alert_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position BETWEEN 0 AND 99),
  status TEXT NOT NULL CHECK (status IN ('accepted','projecting','compensated','conflicted','failed','cancelled')),
  item_revision BIGINT NOT NULL DEFAULT 1 CHECK (item_revision > 0),
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  compensation_state_version BIGINT NOT NULL DEFAULT 0 CHECK (compensation_state_version >= 0),
  restore_assignee TEXT NOT NULL DEFAULT '',
  restore_status TEXT NOT NULL CHECK (restore_status IN ('new','triage','assigned','closed')),
  current_assignee TEXT NOT NULL DEFAULT '',
  current_status TEXT NOT NULL CHECK (current_status = 'assigned'),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, request_id, alert_id),
  UNIQUE (tenant_id, request_id, position),
  FOREIGN KEY (tenant_id, request_id)
    REFERENCES alert_assignment_compensation_requests (tenant_id, request_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id, batch_id, alert_id)
    REFERENCES alert_assignment_batch_items (tenant_id, batch_id, alert_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_compensation_items_status
  ON alert_assignment_compensation_items (tenant_id, request_id, status, position);

CREATE TABLE IF NOT EXISTS alert_assignment_compensation_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  request_id UUID NOT NULL,
  batch_id UUID NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, request_id, revision),
  FOREIGN KEY (tenant_id, request_id)
    REFERENCES alert_assignment_compensation_requests (tenant_id, request_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_compensation_item_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  request_id UUID NOT NULL,
  batch_id UUID NOT NULL,
  alert_id TEXT NOT NULL,
  item_revision BIGINT NOT NULL CHECK (item_revision > 0),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  compensation_state_version BIGINT NOT NULL DEFAULT 0 CHECK (compensation_state_version >= 0),
  actor_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(detail) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, request_id, alert_id, item_revision),
  FOREIGN KEY (tenant_id, request_id, alert_id)
    REFERENCES alert_assignment_compensation_items (tenant_id, request_id, alert_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_compensation_projection_receipts (
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  request_id UUID NOT NULL,
  batch_id UUID NOT NULL,
  alert_id TEXT NOT NULL,
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  compensation_state_version BIGINT NOT NULL DEFAULT 0 CHECK (compensation_state_version >= 0),
  restore_assignee TEXT NOT NULL DEFAULT '',
  restore_status TEXT NOT NULL CHECK (restore_status IN ('new','triage','assigned','closed')),
  outcome TEXT NOT NULL CHECK (outcome IN ('compensated','conflicted','failed')),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  source_topic TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  trace_id TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (event_id, alert_id),
  UNIQUE (tenant_id, request_id, alert_id),
  FOREIGN KEY (event_id) REFERENCES alert_assignment_batch_inbox(event_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id, request_id, alert_id)
    REFERENCES alert_assignment_compensation_items (tenant_id, request_id, alert_id) ON DELETE RESTRICT
);

ALTER TABLE alert_assignment_batch_outbox
  DROP CONSTRAINT IF EXISTS alert_assignment_batch_outbox_event_type_check;
ALTER TABLE alert_assignment_batch_outbox
  ADD CONSTRAINT alert_assignment_batch_outbox_event_type_check
  CHECK (event_type IN (
    'alert.batch-assignment.requested.v1',
    'alert.assignment.changed.v1',
    'alert.batch-assignment.cancelled.v1',
    'alert.batch-assignment.compensation-requested.v1',
    'alert.assignment.compensated.v1'
  ));

ALTER TABLE alert_assignment_batch_inbox
  DROP CONSTRAINT IF EXISTS alert_assignment_batch_inbox_event_type_check;
ALTER TABLE alert_assignment_batch_inbox
  ADD CONSTRAINT alert_assignment_batch_inbox_event_type_check
  CHECK (event_type IN (
    'alert.batch-assignment.requested.v1',
    'alert.assignment.changed.v1',
    'alert.batch-assignment.compensation-requested.v1',
    'alert.assignment.compensated.v1'
  ));

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608092300', 'revision-safe alert batch assignment compensation v1')
ON CONFLICT (version) DO NOTHING;

COMMIT;
