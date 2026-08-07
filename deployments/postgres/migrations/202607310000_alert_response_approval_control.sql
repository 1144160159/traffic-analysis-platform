-- F-COMMON-003 / F-PLAYBOOK-001 / T-PG-002 / T-KAFKA-001 / WP-07
-- Expand: add request revision, cancellable outbox state, immutable independent
-- approval decisions and idempotent cancel/compensation control requests.
-- Backfill: pre-existing actions are new aggregates at expected_revision=0;
-- no approval, cancellation, compensation or external effect is synthesized.
-- Verify:
--   SELECT count(*) FROM alert_response_actions WHERE expected_revision<>0;
--   SELECT count(*) FROM alert_response_outbox
--     WHERE cancelled_at IS NOT NULL AND published=true;
--   SELECT tenant_id,idempotency_key,count(*) FROM alert_response_approvals
--     GROUP BY 1,2 HAVING count(*)>1;
-- Cutover: deploy the API and consumer only after this migration succeeds.
-- Rollback: disable the additive approval/control routes, stop new outbox
-- producers and retain all decision/audit evidence. Do not drop evidence rows.

BEGIN;

ALTER TABLE alert_response_actions
  ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;

ALTER TABLE alert_response_outbox
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE alert_response_execution_receipts
  ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_delivery
  ON alert_response_outbox(next_attempt_at,outbox_id)
  WHERE published=false AND cancelled_at IS NULL;

CREATE TABLE IF NOT EXISTS alert_response_approvals (
  approval_id        UUID PRIMARY KEY,
  job_id             TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  decision           TEXT NOT NULL CHECK (decision IN ('approve','reject')),
  expected_revision  BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key    TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason             TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  decided_by         TEXT NOT NULL CHECK (decided_by<>''),
  resulting_revision BIGINT NOT NULL,
  resulting_status   TEXT NOT NULL,
  approval_status    TEXT NOT NULL CHECK (approval_status IN ('approved','rejected')),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_approvals_job
  ON alert_response_approvals(tenant_id,job_id,created_at);

CREATE TABLE IF NOT EXISTS alert_response_control_requests (
  request_id         UUID PRIMARY KEY,
  job_id             TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  operation          TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  expected_revision  BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key    TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason             TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  requested_by       TEXT NOT NULL CHECK (requested_by<>''),
  state              TEXT NOT NULL CHECK (state IN ('cancelled','blocked_external_executor')),
  resulting_revision BIGINT NOT NULL,
  resulting_status   TEXT NOT NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_alert_response_control_job
  ON alert_response_control_requests(tenant_id,job_id,created_at);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607310000','independent alert response approval and control workflow')
ON CONFLICT (version) DO NOTHING;

COMMIT;
