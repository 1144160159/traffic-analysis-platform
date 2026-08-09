-- F-PLAYBOOK-001 / T-PG-001 / T-SCHEMA-001
-- Expand the existing drill-only playbook ledger into a tenant-scoped live
-- execution workflow. Existing drill rows remain valid and no historical row
-- is promoted to an external effect.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608021000';
--   SELECT mode,status,approval_status,executor_status,count(*)
--     FROM alert_playbook_executions GROUP BY 1,2,3,4 ORDER BY 1,2,3,4;
--   SELECT e.execution_id FROM alert_playbook_executions e
--     LEFT JOIN alert_playbook_definitions d
--       ON d.tenant_id=e.tenant_id AND d.name=e.playbook_name
--     WHERE e.mode='live' AND d.name IS NULL;
-- Rollback: disable PLAYBOOK_EXECUTION_V2_ENABLED and stop the worker before
-- rolling back the service image. Retain the additive ledger and receipts so
-- accepted or externally effected work remains recoverable and auditable.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_mode_check;
ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_status_check;

ALTER TABLE alert_playbook_executions
  ADD COLUMN IF NOT EXISTS playbook_version INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS workflow_revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required',
  ADD COLUMN IF NOT EXISTS executor_status TEXT NOT NULL DEFAULT 'simulated',
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS request_sha256 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS execution_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_mode_check
  CHECK (mode IN ('legacy','drill','live'));
ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_status_check
  CHECK (status IN (
    'succeeded','rolled_back','rollback_recorded',
    'pending_approval','approved_awaiting_executor','running','completed','partial',
    'failed','cancelled','compensation_queued','compensating','compensated',
    'compensation_failed'
  ));
ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_workflow_revision_check;
ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_workflow_revision_check
  CHECK (workflow_revision > 0);
ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_approval_status_check;
ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_approval_status_check
  CHECK (approval_status IN ('not_required','pending','approved','rejected','cancelled'));
ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_executor_status_check;
ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_executor_status_check
  CHECK (executor_status IN (
    'simulated','not_dispatched','not_configured','queued','running','succeeded',
    'partial','failed','cancelled','compensating','compensated','compensation_failed'
  ));
ALTER TABLE alert_playbook_executions
  DROP CONSTRAINT IF EXISTS alert_playbook_execution_attempts_check;
ALTER TABLE alert_playbook_executions
  ADD CONSTRAINT alert_playbook_execution_attempts_check CHECK (attempts >= 0);

CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_playbook_execution_idempotency
  ON alert_playbook_executions(tenant_id,idempotency_key)
  WHERE idempotency_key<>'';
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_dispatch
  ON alert_playbook_executions(next_attempt_at,created_at,execution_id)
  WHERE mode='live' AND status IN ('approved_awaiting_executor','compensation_queued');

CREATE TABLE IF NOT EXISTS alert_playbook_execution_approvals (
  approval_id          UUID PRIMARY KEY,
  execution_id         TEXT NOT NULL,
  tenant_id            TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  decision             TEXT NOT NULL CHECK (decision IN ('approve','reject')),
  expected_revision    BIGINT NOT NULL CHECK (expected_revision > 0),
  idempotency_key      TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason               TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  decided_by           TEXT NOT NULL CHECK (decided_by<>''),
  resulting_revision   BIGINT NOT NULL CHECK (resulting_revision > 0),
  resulting_status     TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id)
    REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_approvals_job
  ON alert_playbook_execution_approvals(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_execution_controls (
  request_id           UUID PRIMARY KEY,
  execution_id         TEXT NOT NULL,
  tenant_id            TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  operation            TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  expected_revision    BIGINT NOT NULL CHECK (expected_revision > 0),
  idempotency_key      TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason               TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  requested_by         TEXT NOT NULL CHECK (requested_by<>''),
  resulting_revision   BIGINT NOT NULL CHECK (resulting_revision > 0),
  resulting_status     TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id)
    REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_controls_job
  ON alert_playbook_execution_controls(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_step_receipts (
  receipt_id           UUID PRIMARY KEY,
  execution_id         TEXT NOT NULL,
  tenant_id            TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  phase                TEXT NOT NULL CHECK (phase IN ('execute','compensate')),
  attempt              INTEGER NOT NULL CHECK (attempt > 0),
  step_index           INTEGER NOT NULL CHECK (step_index >= 0),
  action_type          TEXT NOT NULL,
  provider             TEXT NOT NULL,
  provider_receipt_id  TEXT NOT NULL,
  status               TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')),
  external_effect      BOOLEAN NOT NULL DEFAULT false,
  payload              JSONB NOT NULL,
  payload_sha256       TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (execution_id,phase,attempt,step_index),
  UNIQUE (tenant_id,provider,provider_receipt_id,phase),
  FOREIGN KEY (tenant_id,execution_id)
    REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_step_receipts_job
  ON alert_playbook_step_receipts(tenant_id,execution_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_playbook_execution_outbox (
  outbox_id            BIGSERIAL PRIMARY KEY,
  event_id             UUID NOT NULL UNIQUE,
  execution_id         TEXT NOT NULL,
  tenant_id            TEXT NOT NULL,
  playbook_name        TEXT NOT NULL,
  event_type           TEXT NOT NULL,
  schema_version       INTEGER NOT NULL DEFAULT 2,
  aggregate_version    BIGINT NOT NULL CHECK (aggregate_version > 0),
  partition_key        TEXT NOT NULL,
  payload              JSONB NOT NULL,
  published            BOOLEAN NOT NULL DEFAULT false,
  attempts             INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until         TIMESTAMPTZ,
  locked_by            TEXT NOT NULL DEFAULT '',
  last_error           TEXT NOT NULL DEFAULT '',
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at         TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,execution_id)
    REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_dispatch
  ON alert_playbook_execution_outbox(next_attempt_at,outbox_id) WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608021000','versioned playbook live execution approval step receipts compensation and outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
