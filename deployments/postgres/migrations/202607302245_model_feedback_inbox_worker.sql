-- F-ALERT-001 / F-MODEL-001 / T-PG-001 / WP-07
-- Expand: add recoverable leases, terminal dead-letter state and projection
-- acknowledgement to the model feedback inbox.
-- Backfill: existing pending/failed rows are immediately eligible. Stale
-- processing rows are reclaimed after locked_until; rows created before this
-- migration receive an expired lease.
-- Rollback: set MODEL_FEEDBACK_CLICKHOUSE_PROJECTION_V1_ENABLED=false. Retain
-- the columns and rows for reconciliation; they are additive evidence.

BEGIN;

ALTER TABLE model_feedback_inbox
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ;

UPDATE model_feedback_inbox
SET locked_until=COALESCE(locked_until,now()-interval '1 second')
WHERE status='processing';

ALTER TABLE model_feedback_inbox
  DROP CONSTRAINT IF EXISTS model_feedback_inbox_status_check;
ALTER TABLE model_feedback_inbox
  ADD CONSTRAINT model_feedback_inbox_status_check
  CHECK (status IN ('pending','processing','applied','failed','dead_letter'))
  NOT VALID;
ALTER TABLE model_feedback_inbox
  VALIDATE CONSTRAINT model_feedback_inbox_status_check;

CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_reclaim
  ON model_feedback_inbox (locked_until,updated_at,feedback_id)
  WHERE status='processing';
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_dead_letter
  ON model_feedback_inbox (updated_at,feedback_id)
  WHERE status='dead_letter';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302245','reliable model feedback inbox ClickHouse materializer')
ON CONFLICT (version) DO NOTHING;

COMMIT;
