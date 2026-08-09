-- F-ALERT-004: independently consumed assignment execution and terminal receipts.
BEGIN;

ALTER TABLE alert_assignment_batch_items
  DROP CONSTRAINT IF EXISTS alert_assignment_batch_items_status_check;
ALTER TABLE alert_assignment_batch_items
  ADD CONSTRAINT alert_assignment_batch_items_status_check
  CHECK (status IN ('accepted','projecting','applied','conflicted','forbidden','failed','cancelled'));

CREATE TABLE IF NOT EXISTS alert_assignment_states (
  tenant_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  state_version BIGINT NOT NULL CHECK (state_version > 0),
  assignee TEXT NOT NULL,
  source_batch_id UUID NOT NULL,
  source_event_id UUID NOT NULL,
  previous_state_version BIGINT NOT NULL CHECK (previous_state_version > 0),
  previous_assignee TEXT NOT NULL DEFAULT '',
  projection_status TEXT NOT NULL DEFAULT 'pending'
    CHECK (projection_status IN ('pending','applied','conflicted','failed')),
  last_error TEXT NOT NULL DEFAULT '',
  trace_id TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant_id, alert_id),
  UNIQUE (source_event_id, alert_id),
  FOREIGN KEY (tenant_id, source_batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_assignment_states_batch
  ON alert_assignment_states (tenant_id, source_batch_id, alert_id);

CREATE TABLE IF NOT EXISTS alert_assignment_state_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  alert_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  previous_state_version BIGINT NOT NULL CHECK (previous_state_version > 0),
  resulting_state_version BIGINT NOT NULL CHECK (resulting_state_version > previous_state_version),
  previous_assignee TEXT NOT NULL DEFAULT '',
  resulting_assignee TEXT NOT NULL,
  requested_by TEXT NOT NULL,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, alert_id, resulting_state_version),
  FOREIGN KEY (tenant_id, batch_id, alert_id)
    REFERENCES alert_assignment_batch_items (tenant_id, batch_id, alert_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_batch_inbox (
  event_id UUID PRIMARY KEY,
  event_type TEXT NOT NULL CHECK (event_type IN ('alert.batch-assignment.requested.v1','alert.assignment.changed.v1')),
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  source_topic TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  headers_sha256 TEXT NOT NULL CHECK (headers_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL,
  UNIQUE (source_topic, source_partition, source_offset),
  FOREIGN KEY (tenant_id, batch_id)
    REFERENCES alert_assignment_batches (tenant_id, batch_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_projection_receipts (
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  batch_id UUID NOT NULL,
  alert_id TEXT NOT NULL,
  expected_state_version BIGINT NOT NULL CHECK (expected_state_version > 0),
  resulting_state_version BIGINT NOT NULL DEFAULT 0 CHECK (resulting_state_version >= 0),
  previous_assignee TEXT NOT NULL DEFAULT '',
  resulting_assignee TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL CHECK (outcome IN ('applied','conflicted','failed')),
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  source_topic TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  trace_id TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (event_id, alert_id),
  UNIQUE (tenant_id, batch_id, alert_id),
  FOREIGN KEY (event_id) REFERENCES alert_assignment_batch_inbox(event_id) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id, batch_id, alert_id)
    REFERENCES alert_assignment_batch_items (tenant_id, batch_id, alert_id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS alert_assignment_batch_dlq_receipts (
  source_topic TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  dlq_topic TEXT NOT NULL CHECK (dlq_topic = 'dlq.v1'),
  event_id TEXT NOT NULL DEFAULT '',
  event_type TEXT NOT NULL DEFAULT '',
  tenant_id TEXT NOT NULL DEFAULT '',
  batch_id TEXT NOT NULL DEFAULT '',
  aggregate_version BIGINT,
  trace_id TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL,
  error_message TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  headers_sha256 TEXT NOT NULL CHECK (headers_sha256 ~ '^[0-9a-f]{64}$'),
  headers JSONB NOT NULL CHECK (jsonb_typeof(headers) = 'object'),
  acknowledged_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (source_topic, source_partition, source_offset)
);

INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608092130', 'independent alert batch assignment execution and terminal receipts v1')
ON CONFLICT (version) DO NOTHING;

COMMIT;
