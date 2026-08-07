-- F-COMMON-003 / T-SCHEMA-001 / WP-01
-- Expand: create alert workbench tables and add retry/lease fields used by the
-- response-action outbox worker. Business services never execute this DDL.
-- Backfill: existing pending rows receive next_attempt_at=now() from the
-- additive default; no business state is synthesized.
-- Verify:
--   SELECT column_name FROM information_schema.columns
--     WHERE table_name='alert_response_outbox'
--       AND column_name IN ('next_attempt_at','locked_until','locked_by');
--   SELECT count(*) FROM alert_response_outbox
--     WHERE published=false AND next_attempt_at IS NULL;
-- Cutover: deploy the application only after the migration job succeeds.
-- Rollback: roll back the application. Keep additive columns and evidence rows.

BEGIN;

CREATE TABLE IF NOT EXISTS alert_saved_views (
  view_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL,
  name       TEXT NOT NULL,
  filters    JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,name)
);

CREATE TABLE IF NOT EXISTS alert_response_actions (
  job_id       TEXT PRIMARY KEY,
  tenant_id    TEXT NOT NULL,
  alert_id     TEXT NOT NULL,
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  reason       TEXT NOT NULL,
  dry_run      BOOLEAN NOT NULL DEFAULT true,
  status       TEXT NOT NULL,
  detail       JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_response_outbox (
  outbox_id      BIGSERIAL PRIMARY KEY,
  job_id         TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE CASCADE,
  tenant_id      TEXT NOT NULL,
  event_type     TEXT NOT NULL,
  payload        JSONB NOT NULL,
  published      BOOLEAN NOT NULL DEFAULT false,
  attempts       INTEGER NOT NULL DEFAULT 0,
  last_error     TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until   TIMESTAMPTZ,
  locked_by      TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at   TIMESTAMPTZ
);

ALTER TABLE alert_response_outbox
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE alert_response_outbox
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE alert_response_outbox
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_retry
  ON alert_response_outbox (next_attempt_at,outbox_id)
  WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607301900','F-ALERT workbench and response outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
