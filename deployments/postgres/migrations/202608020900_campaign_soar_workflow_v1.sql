-- F-CAMPAIGN-001 / F-PLAYBOOK-001 / T-PG-001
-- Expand: add a tenant-scoped campaign SOAR approval, execution receipt and
-- compensation ledger. The request row is inserted in the same transaction as
-- campaign_action_jobs, campaign aggregate history/outbox and the audit row.
-- Backfill: none. Historical SoarRequested events have no trustworthy provider
-- receipt and must not be promoted to a terminal effect.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608020900';
--   SELECT status,approval_status,executor_status,count(*) FROM campaign_soar_jobs
--     GROUP BY 1,2,3 ORDER BY 1,2,3;
--   SELECT j.job_id FROM campaign_soar_jobs j LEFT JOIN campaign_action_jobs a
--     ON a.job_id=j.job_id AND a.tenant_id=j.tenant_id WHERE a.job_id IS NULL;
-- Rollback: stop the campaign SOAR worker and roll back the service image.
-- Retain all additive tables and status values so accepted work stays auditable.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_action_jobs DROP CONSTRAINT IF EXISTS campaign_action_jobs_status_check;
ALTER TABLE campaign_action_jobs ADD CONSTRAINT campaign_action_jobs_status_check
  CHECK (status IN (
    'queued','accepted','pending_approval','approved_awaiting_executor','running',
    'completed','succeeded','partial','failed','cancelled','compensation_queued',
    'compensating','compensated','compensation_failed'
  ));
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_job
  ON campaign_action_jobs(tenant_id,job_id);

CREATE TABLE IF NOT EXISTS campaign_soar_jobs (
  job_id               TEXT PRIMARY KEY REFERENCES campaign_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id            TEXT NOT NULL,
  campaign_id          TEXT NOT NULL,
  playbook_id          TEXT NOT NULL,
  target               TEXT NOT NULL,
  source_snapshot_id   TEXT NOT NULL,
  campaign_revision    BIGINT NOT NULL CHECK (campaign_revision > 0),
  status               TEXT NOT NULL CHECK (status IN (
    'pending_approval','approved_awaiting_executor','running','completed','partial',
    'failed','cancelled','compensation_queued','compensating','compensated','compensation_failed'
  )),
  approval_status      TEXT NOT NULL CHECK (approval_status IN ('pending','approved','rejected','cancelled')),
  executor_status      TEXT NOT NULL CHECK (executor_status IN (
    'not_dispatched','not_configured','queued','running','succeeded','partial','failed',
    'cancelled','compensating','compensated','compensation_failed'
  )),
  revision             BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  request               JSONB NOT NULL DEFAULT '{}'::jsonb,
  execution_receipt    JSONB NOT NULL DEFAULT '{}'::jsonb,
  compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message         TEXT NOT NULL DEFAULT '',
  attempts              INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until          TIMESTAMPTZ,
  locked_by             TEXT NOT NULL DEFAULT '',
  requested_by          TEXT NOT NULL,
  approved_by           TEXT NOT NULL DEFAULT '',
  approved_at           TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at          TIMESTAMPTZ,
  UNIQUE (tenant_id,job_id),
  FOREIGN KEY (tenant_id,job_id) REFERENCES campaign_action_jobs(tenant_id,job_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_campaign
  ON campaign_soar_jobs(tenant_id,campaign_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_dispatch
  ON campaign_soar_jobs(next_attempt_at,created_at)
  WHERE status IN ('approved_awaiting_executor','compensation_queued');

CREATE TABLE IF NOT EXISTS campaign_soar_approvals (
  approval_id          UUID PRIMARY KEY,
  job_id               TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id            TEXT NOT NULL,
  campaign_id          TEXT NOT NULL,
  decision             TEXT NOT NULL CHECK (decision IN ('approve','reject')),
  expected_revision    BIGINT NOT NULL CHECK (expected_revision > 0),
  idempotency_key      TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason               TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  decided_by           TEXT NOT NULL CHECK (decided_by<>''),
  resulting_revision   BIGINT NOT NULL CHECK (resulting_revision > 0),
  resulting_status     TEXT NOT NULL,
  approval_status      TEXT NOT NULL,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_approvals_job
  ON campaign_soar_approvals(tenant_id,job_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_soar_execution_receipts (
  receipt_id          UUID PRIMARY KEY,
  job_id              TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id           TEXT NOT NULL,
  campaign_id         TEXT NOT NULL,
  phase               TEXT NOT NULL CHECK (phase IN ('execute','compensate')),
  attempt             INTEGER NOT NULL CHECK (attempt > 0),
  provider            TEXT NOT NULL,
  provider_receipt_id TEXT NOT NULL,
  status              TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')),
  external_effect     BOOLEAN NOT NULL DEFAULT false,
  payload             JSONB NOT NULL,
  payload_sha256      TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,phase,attempt),
  UNIQUE (tenant_id,provider,provider_receipt_id,phase)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_receipts_job
  ON campaign_soar_execution_receipts(tenant_id,job_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_soar_control_requests (
  request_id          UUID PRIMARY KEY,
  job_id              TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT,
  tenant_id           TEXT NOT NULL,
  campaign_id         TEXT NOT NULL,
  operation           TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  expected_revision   BIGINT NOT NULL CHECK (expected_revision > 0),
  idempotency_key     TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason              TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  requested_by        TEXT NOT NULL CHECK (requested_by<>''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  resulting_status   TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_control_job
  ON campaign_soar_control_requests(tenant_id,job_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608020900','campaign SOAR approval execution receipt and compensation workflow')
ON CONFLICT (version) DO NOTHING;

COMMIT;
