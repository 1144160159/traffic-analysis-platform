-- F-CAMPAIGN-001 / F-ALERT-002 / T-PG-001 / T-SCHEMA-001
-- Expand: add immutable run, per-campaign and per-alert receipts for the
-- manifest-driven historical campaign membership backfill.
-- Backfill: the campaign-membership-backfill command consumes an authorized
-- ClickHouse export manifest. It processes one bounded campaign per serializable
-- transaction, binds only legacy linked rows, inserts missing links and never
-- resurrects an explicitly unlinked relation.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608010930';
--   SELECT run_id,status,manifest_sha256,completed_campaign_count,failed_campaign_count
--     FROM campaign_membership_backfill_runs ORDER BY created_at DESC;
--   SELECT tenant_id,campaign_id,count(*) FROM campaign_alert_links
--     WHERE status='linked' AND campaign_revision=0 GROUP BY tenant_id,campaign_id;
-- Cutover: run against an immutable export in a canary tenant, reconcile every
-- receipt/event/outbox count, then enable CAMPAIGN_AGGREGATE_V2_ENABLED.
-- Rollback: stop the command and keep all additive receipts and emitted events.
-- Corrective rollback is a new authorized manifest; completed evidence is never
-- deleted or rewritten.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS campaign_membership_backfill_runs (
  run_id                    UUID PRIMARY KEY,
  tenant_id                 TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_kind               TEXT NOT NULL CHECK (source_kind='clickhouse_export'),
  source_uri                TEXT NOT NULL CHECK (length(source_uri)>0),
  source_sha256             TEXT NOT NULL CHECK (length(source_sha256)=64),
  source_snapshot_id        TEXT NOT NULL CHECK (length(source_snapshot_id)>0),
  source_as_of              TIMESTAMPTZ NOT NULL,
  manifest                  JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'),
  manifest_sha256           TEXT NOT NULL CHECK (length(manifest_sha256)=64),
  reason                    TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  status                    TEXT NOT NULL CHECK (status IN ('running','completed','partial','failed')),
  campaign_count            INTEGER NOT NULL CHECK (campaign_count>0 AND campaign_count<=100),
  source_member_count       INTEGER NOT NULL CHECK (source_member_count>=0),
  completed_campaign_count  INTEGER NOT NULL DEFAULT 0 CHECK (completed_campaign_count>=0),
  failed_campaign_count     INTEGER NOT NULL DEFAULT 0 CHECK (failed_campaign_count>=0),
  inserted_count            INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0),
  bound_count               INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0),
  unchanged_count           INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0),
  skipped_unlinked_count    INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0),
  created_by                TEXT NOT NULL DEFAULT '',
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at              TIMESTAMPTZ,
  UNIQUE (tenant_id,manifest_sha256),
  CHECK (completed_campaign_count+failed_campaign_count<=campaign_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_runs_tenant
  ON campaign_membership_backfill_runs(tenant_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_membership_backfill_campaigns (
  run_id                     UUID NOT NULL REFERENCES campaign_membership_backfill_runs(run_id) ON DELETE RESTRICT,
  tenant_id                  TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id                TEXT NOT NULL,
  manifest_sha256            TEXT NOT NULL CHECK (length(manifest_sha256)=64),
  expected_campaign_revision BIGINT NOT NULL CHECK (expected_campaign_revision>=0),
  starting_campaign_revision BIGINT CHECK (starting_campaign_revision>=0),
  resulting_campaign_revision BIGINT CHECK (resulting_campaign_revision>=0),
  source_member_count        INTEGER NOT NULL CHECK (source_member_count>=0 AND source_member_count<=1000),
  resulting_member_count     INTEGER CHECK (resulting_member_count>=0),
  inserted_count             INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0),
  bound_count                INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0),
  unchanged_count            INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0),
  skipped_unlinked_count     INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0),
  status                     TEXT NOT NULL CHECK (status IN ('running','completed','failed')),
  error_code                 TEXT NOT NULL DEFAULT '',
  error_message              TEXT NOT NULL DEFAULT '',
  aggregate_event_id         UUID REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT,
  started_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at               TIMESTAMPTZ,
  PRIMARY KEY (run_id,campaign_id),
  CHECK (inserted_count+bound_count+unchanged_count+skipped_unlinked_count<=source_member_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_campaigns_tenant
  ON campaign_membership_backfill_campaigns(tenant_id,campaign_id,started_at DESC);

CREATE TABLE IF NOT EXISTS campaign_membership_backfill_items (
  run_id                    UUID NOT NULL,
  tenant_id                 TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id               TEXT NOT NULL,
  alert_id                  TEXT NOT NULL,
  source_ordinal            INTEGER NOT NULL CHECK (source_ordinal>=0),
  outcome                   TEXT NOT NULL CHECK (outcome IN ('inserted','bound','unchanged','skipped_explicit_unlink')),
  relation_id               UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  relation_revision         BIGINT NOT NULL CHECK (relation_revision>0),
  campaign_revision         BIGINT NOT NULL CHECK (campaign_revision>=0),
  event_id                  UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id,campaign_id,alert_id),
  UNIQUE (run_id,campaign_id,source_ordinal),
  FOREIGN KEY (run_id,campaign_id) REFERENCES campaign_membership_backfill_campaigns(run_id,campaign_id)
    ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  CHECK ((outcome IN ('inserted','bound') AND event_id IS NOT NULL) OR
         (outcome IN ('unchanged','skipped_explicit_unlink') AND event_id IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_items_tenant_alert
  ON campaign_membership_backfill_items(tenant_id,alert_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010930','manifest-driven resumable campaign membership backfill receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
