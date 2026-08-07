-- F-CAMPAIGN-001 / T-PG-001 / T-MINIO-001
-- Expand the frozen campaign-report request into a leased, retryable executor.
-- PostgreSQL remains authoritative for request, job state, audit and the object
-- manifest; MinIO contains only the immutable artifact bytes.
--
-- Rollback: disable CAMPAIGN_AGGREGATE_V2_ENABLED and roll back the service
-- image. Keep these additive columns so accepted/in-flight jobs remain
-- inspectable and can be resumed by the previous candidate after repair.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS job_id TEXT;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_bucket TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS artifact_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE campaign_reports DROP CONSTRAINT IF EXISTS campaign_reports_attempts_check;
ALTER TABLE campaign_reports ADD CONSTRAINT campaign_reports_attempts_check CHECK (attempts >= 0);
ALTER TABLE campaign_reports DROP CONSTRAINT IF EXISTS campaign_reports_size_bytes_check;
ALTER TABLE campaign_reports ADD CONSTRAINT campaign_reports_size_bytes_check CHECK (size_bytes >= 0);

CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_job_id
  ON campaign_reports(job_id) WHERE job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_campaign_reports_executor_pending
  ON campaign_reports(next_attempt_at,created_at,report_id)
  WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608011000','leased deterministic campaign report executor and object manifest')
ON CONFLICT (version) DO NOTHING;

COMMIT;
