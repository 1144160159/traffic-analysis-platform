-- F-CAMPAIGN-001 / T-PG-001 / T-SCHEMA-001
-- Expand: add the authoritative versioned campaign command ledger, history,
-- projection outbox and immutable report snapshot metadata. Existing v1 rows
-- and status values remain readable during the default-off canary.
-- Backfill: no campaign state or report completion is synthesized. Operators
-- must reconcile existing ClickHouse campaign members before enabling the v2
-- feature flag for a tenant.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608010700';
--   SELECT count(*) FROM campaign_aggregate_history;
--   SELECT count(*) FROM campaign_aggregate_outbox WHERE published=false;
-- Rollback: disable CAMPAIGN_AGGREGATE_V2_ENABLED and roll back the service
-- image. Retain additive columns/tables so accepted commands remain auditable.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS request_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS resource_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_action_jobs ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_action_jobs DROP CONSTRAINT IF EXISTS campaign_action_jobs_status_check;
ALTER TABLE campaign_action_jobs ADD CONSTRAINT campaign_action_jobs_status_check
  CHECK (status IN ('queued','accepted','running','completed','succeeded','partial','failed','cancelled','compensated'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_idempotency
  ON campaign_action_jobs(tenant_id,idempotency_key)
  WHERE idempotency_key<>'';

ALTER TABLE campaign_workbench_state ADD COLUMN IF NOT EXISTS member_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE campaign_workbench_state ADD COLUMN IF NOT EXISTS last_event_id UUID;

CREATE TABLE IF NOT EXISTS campaign_aggregate_history (
  event_id           UUID PRIMARY KEY,
  tenant_id          TEXT NOT NULL,
  campaign_id        TEXT NOT NULL,
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  event_type         TEXT NOT NULL,
  status             TEXT NOT NULL,
  assignee           TEXT NOT NULL DEFAULT '',
  member_count       INTEGER NOT NULL DEFAULT 0 CHECK (member_count >= 0),
  payload            JSONB NOT NULL,
  reason             TEXT NOT NULL,
  created_by         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,campaign_id,aggregate_revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_history_revision
  ON campaign_aggregate_history(tenant_id,campaign_id,aggregate_revision DESC);

CREATE TABLE IF NOT EXISTS campaign_aggregate_outbox (
  event_id           UUID PRIMARY KEY REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  aggregate_id       TEXT NOT NULL,
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  event_type         TEXT NOT NULL,
  schema_version     INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key      TEXT NOT NULL,
  payload            JSONB NOT NULL,
  published          BOOLEAN NOT NULL DEFAULT false,
  attempts           INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error         TEXT NOT NULL DEFAULT '',
  next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until       TIMESTAMPTZ,
  locked_by          TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at       TIMESTAMPTZ,
  UNIQUE (tenant_id,aggregate_id,aggregate_revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_pending
  ON campaign_aggregate_outbox(next_attempt_at,created_at)
  WHERE published=false;

ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS campaign_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot_id UUID;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS snapshot_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_manifest JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports DROP CONSTRAINT IF EXISTS campaign_reports_status_check;
ALTER TABLE campaign_reports ADD CONSTRAINT campaign_reports_status_check
  CHECK (status IN ('queued','accepted','running','completed','succeeded','partial','failed','cancelled'));
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_tenant_idempotency
  ON campaign_reports(tenant_id,idempotency_key)
  WHERE idempotency_key<>'';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010700','versioned campaign aggregate commands and frozen report snapshots')
ON CONFLICT (version) DO NOTHING;

COMMIT;
