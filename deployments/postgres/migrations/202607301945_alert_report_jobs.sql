-- F-ALERT-001 / WP-07
-- Expand: add immutable frozen snapshots, asynchronous jobs and transactional events.
-- Backfill: none; investigation notes are deliberately not converted into reports.
-- Verify:
--   SELECT status,count(*) FROM alert_report_jobs GROUP BY status;
--   SELECT count(*) FROM alert_report_outbox WHERE published=false;
-- Cutover: enable alert_report_jobs_v1 after PostgreSQL and report-artifacts MinIO bucket are ready.
-- Rollback: disable the feature flag and worker; preserve manifests and objects for audit.

BEGIN;

CREATE TABLE IF NOT EXISTS alert_report_jobs (
  job_id                TEXT PRIMARY KEY,
  tenant_id             TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  alert_id              TEXT NOT NULL,
  format                TEXT NOT NULL CHECK (format IN ('json','pdf','docx')),
  status                TEXT NOT NULL DEFAULT 'accepted'
                        CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')),
  idempotency_key       TEXT NOT NULL,
  requested_snapshot_id TEXT NOT NULL,
  snapshot              JSONB NOT NULL,
  snapshot_sha256       TEXT NOT NULL,
  missing_sections      JSONB NOT NULL DEFAULT '[]'::jsonb,
  source_watermarks     JSONB NOT NULL DEFAULT '{}'::jsonb,
  object_bucket         TEXT NOT NULL DEFAULT '',
  object_key            TEXT NOT NULL DEFAULT '',
  mime_type             TEXT NOT NULL DEFAULT '',
  artifact_sha256       TEXT NOT NULL DEFAULT '',
  size_bytes            BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  error_message         TEXT NOT NULL DEFAULT '',
  attempts              INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until          TIMESTAMPTZ,
  locked_by             TEXT NOT NULL DEFAULT '',
  created_by            TEXT NOT NULL DEFAULT '',
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at          TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_alert_time
  ON alert_report_jobs (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_queue
  ON alert_report_jobs (status,next_attempt_at,created_at)
  WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS alert_report_outbox (
  event_id          UUID PRIMARY KEY,
  job_id            TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT,
  tenant_id         TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  event_type        TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version    INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key     TEXT NOT NULL,
  payload           JSONB NOT NULL,
  published         BOOLEAN NOT NULL DEFAULT false,
  attempts          INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error        TEXT NOT NULL DEFAULT '',
  next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until      TIMESTAMPTZ,
  locked_by         TEXT NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at      TIMESTAMPTZ,
  UNIQUE (job_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_report_outbox_pending
  ON alert_report_outbox (next_attempt_at,created_at)
  WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607301945','F-ALERT deterministic report jobs')
ON CONFLICT (version) DO NOTHING;

COMMIT;
