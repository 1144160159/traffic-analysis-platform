-- F-ALERT-001 / F-ADAPTER-001 cooperative report cancellation.
-- Expand: add revisioned cancellation state, immutable transition history and
-- idempotent control requests without deleting existing report jobs or objects.
-- Backfill: every existing job receives revision 1 and one legacy transition.
-- Cutover: deploy the alert-service worker and API that understand
-- cancel_requested before exposing the UI cancel control.
-- Verify:
--   SELECT status,count(*) FROM alert_report_jobs GROUP BY status;
--   SELECT count(*) FROM alert_report_jobs j LEFT JOIN alert_report_job_history h
--     ON h.job_id=j.job_id AND h.revision=j.revision WHERE h.job_id IS NULL;
--   SELECT count(*) FROM alert_report_jobs
--     WHERE status='cancelled' AND (object_key<>'' OR artifact_sha256<>'');
-- Rollback: hide the cancel control and stop accepting new control requests;
-- preserve jobs, history, outbox and objects. Do not downgrade while any job is
-- cancel_requested.

BEGIN;

ALTER TABLE alert_report_jobs
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS cancellation_reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;

ALTER TABLE alert_report_jobs
  DROP CONSTRAINT IF EXISTS alert_report_jobs_status_check,
  DROP CONSTRAINT IF EXISTS chk_alert_report_status,
  DROP CONSTRAINT IF EXISTS chk_alert_report_revision;
ALTER TABLE alert_report_jobs
  ADD CONSTRAINT chk_alert_report_status
    CHECK (status IN ('accepted','running','cancel_requested','completed','partial','failed','cancelled','compensating','compensated','compensation_failed')),
  ADD CONSTRAINT chk_alert_report_revision CHECK (revision > 0);

CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_cancel_cleanup
  ON alert_report_jobs(next_attempt_at,created_at)
  WHERE status='cancel_requested';

CREATE TABLE IF NOT EXISTS alert_report_job_history (
  transition_id BIGSERIAL PRIMARY KEY,
  job_id        TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  from_status   TEXT NOT NULL,
  to_status     TEXT NOT NULL,
  revision      BIGINT NOT NULL CHECK (revision > 0),
  actor         TEXT NOT NULL,
  reason        TEXT NOT NULL,
  trace_id      TEXT NOT NULL,
  detail        JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_history_tenant_job
  ON alert_report_job_history(tenant_id,job_id,revision);

INSERT INTO alert_report_job_history(
  job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail
)
SELECT job_id,tenant_id,'',status,revision,COALESCE(NULLIF(created_by,''),'migration'),
       'legacy alert report lifecycle backfill','',jsonb_build_object('backfilled',true)
  FROM alert_report_jobs
ON CONFLICT (job_id,revision) DO NOTHING;

CREATE TABLE IF NOT EXISTS alert_report_control_requests (
  request_id          UUID PRIMARY KEY,
  tenant_id           TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  job_id              TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  operation           TEXT NOT NULL CHECK (operation IN ('cancel','compensate')),
  idempotency_key     TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash        TEXT NOT NULL CHECK (length(request_hash)=64),
  expected_revision   BIGINT NOT NULL CHECK (expected_revision > 0),
  resulting_revision  BIGINT NOT NULL CHECK (resulting_revision > 0),
  result_payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  actor               TEXT NOT NULL,
  reason              TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id            TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_control_job
  ON alert_report_control_requests(tenant_id,job_id,created_at);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041900','F-ALERT revisioned cooperative report cancellation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
