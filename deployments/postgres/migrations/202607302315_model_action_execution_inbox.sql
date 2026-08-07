-- F-MODEL-001 / T-KAFKA-001 / WP-15
-- Expand: separate durable dispatch and execution acceptance from business
-- completion. Kafka acknowledgement is represented as dispatched; a canonical
-- consumer materializes awaiting_executor. Only a later executor/provider
-- receipt may move an action to completed/partial/failed.
-- Backfill: stable event IDs are derived from job IDs. Historical asynchronous
-- jobs that were marked completed at broker acknowledgement are corrected to
-- dispatched; they are not automatically replayed.
-- Rollback: set MODEL_ACTION_INBOX_V1_ENABLED=false and
-- MODEL_ACTION_OUTBOX_V1_ENABLED=false. Keep all additive evidence tables and
-- do not restore historical dispatched rows to completed.

BEGIN;

ALTER TABLE model_action_jobs
  ADD COLUMN IF NOT EXISTS event_id UUID,
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS result JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT '';

UPDATE model_action_jobs
SET event_id=COALESCE(
      event_id,
      uuid_generate_v5(uuid_ns_oid(),'model.action.requested.v1:'||job_id)
    )
WHERE event_id IS NULL;

ALTER TABLE model_action_jobs
  ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_model_action_jobs_event
  ON model_action_jobs(event_id);

ALTER TABLE model_action_jobs
  DROP CONSTRAINT IF EXISTS model_action_jobs_status_check;
ALTER TABLE model_action_jobs
  ADD CONSTRAINT model_action_jobs_status_check
  CHECK (status IN (
    'queued','running','dispatched','awaiting_executor',
    'completed','partial','failed','cancelled'
  ));

UPDATE model_action_jobs
SET status='dispatched',
    result=result || jsonb_build_object(
      'migration_note','historical Kafka acknowledgement was not business completion'
    ),
    updated_at=now()
WHERE action IN (
        'append-feedback-samples','request-retraining','request-evaluation'
      )
  AND status='completed';

CREATE TABLE IF NOT EXISTS model_action_outbox (
  outbox_id          BIGSERIAL PRIMARY KEY,
  event_id           UUID NOT NULL UNIQUE REFERENCES model_action_jobs(event_id) ON DELETE RESTRICT,
  job_id             TEXT NOT NULL UNIQUE REFERENCES model_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  model_id           UUID NOT NULL,
  partition_key      TEXT NOT NULL,
  event_type         TEXT NOT NULL DEFAULT 'model.action.requested.v1',
  schema_version     INTEGER NOT NULL DEFAULT 1 CHECK (schema_version=1),
  aggregate_version  BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version=1),
  payload            JSONB NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','published','dead')),
  attempt_count      INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until       TIMESTAMPTZ,
  locked_by          TEXT NOT NULL DEFAULT '',
  last_error         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_model_action_outbox_pending
  ON model_action_outbox(available_at,outbox_id)
  WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_model_action_outbox_reclaim
  ON model_action_outbox(locked_until,outbox_id)
  WHERE status='processing';

INSERT INTO model_action_outbox (
  event_id,job_id,tenant_id,model_id,partition_key,payload
)
SELECT
  action.event_id,
  action.job_id,
  action.tenant_id,
  action.model_id,
  action.model_id::text,
  jsonb_build_object(
    'event_id',action.event_id::text,
    'event_type','model.action.requested.v1',
    'schema_version',1,
    'aggregate_version',1,
    'job_id',action.job_id,
    'action_id',action.action_id,
    'tenant_id',action.tenant_id,
    'model_id',action.model_id::text,
    'version',action.version,
    'action',action.action,
    'target',action.target,
    'payload',action.payload,
    'status','queued',
    'requested_by',action.requested_by,
    'trace_id',action.trace_id,
    'created_at',action.created_at
  )
FROM model_action_jobs action
WHERE action.action IN (
        'append-feedback-samples','request-retraining','request-evaluation'
      )
  AND action.status='queued'
ON CONFLICT (event_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS model_action_execution_inbox (
  event_id           UUID PRIMARY KEY,
  job_id             TEXT NOT NULL UNIQUE REFERENCES model_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  model_id           UUID NOT NULL,
  action_id          TEXT NOT NULL,
  action             TEXT NOT NULL,
  state              TEXT NOT NULL DEFAULT 'awaiting_executor'
                     CHECK (state IN (
                       'awaiting_executor','processing','completed',
                       'partial','failed','cancelled','dead_letter'
                     )),
  payload            JSONB NOT NULL,
  attempt_count      INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until       TIMESTAMPTZ,
  locked_by          TEXT NOT NULL DEFAULT '',
  result             JSONB NOT NULL DEFAULT '{}'::jsonb,
  error              TEXT NOT NULL DEFAULT '',
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  received_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at       TIMESTAMPTZ,
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_model_action_execution_ready
  ON model_action_execution_inbox(available_at,received_at)
  WHERE state='awaiting_executor';
CREATE INDEX IF NOT EXISTS idx_model_action_execution_tenant_model
  ON model_action_execution_inbox(tenant_id,model_id,received_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302315','model action transactional outbox and execution inbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
