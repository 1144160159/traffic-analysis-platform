-- F-CAMPAIGN-001 / F-ALERT-002 / T-PG-001 / T-SCHEMA-001
-- Expand: bind every campaign membership mutation to both the relation
-- revision and the authoritative campaign aggregate revision. The durable
-- command receipt makes retries replayable even after a later unlink/relink.
-- Backfill: existing relation rows keep campaign_revision=0. Operators must
-- reconcile those rows and campaign_workbench_state.member_count before the
-- CAMPAIGN_AGGREGATE_V2_ENABLED canary is enabled for a tenant.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608010730';
--   SELECT count(*) FROM campaign_alert_links WHERE campaign_revision=0;
--   SELECT count(*) FROM campaign_membership_commands;
-- Rollback: disable CAMPAIGN_AGGREGATE_V2_ENABLED and roll back the service
-- image. Retain additive columns and command receipts for audit/replay proof.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_alert_links
  ADD COLUMN IF NOT EXISTS campaign_revision BIGINT NOT NULL DEFAULT 0
  CHECK (campaign_revision >= 0);

ALTER TABLE campaign_alert_link_history
  ADD COLUMN IF NOT EXISTS campaign_revision BIGINT NOT NULL DEFAULT 0
  CHECK (campaign_revision >= 0);

CREATE TABLE IF NOT EXISTS campaign_membership_commands (
  command_id                 UUID PRIMARY KEY,
  tenant_id                  TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  relation_id                UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  campaign_id                TEXT NOT NULL,
  alert_id                   TEXT NOT NULL,
  operation                  TEXT NOT NULL CHECK (operation IN ('link','unlink')),
  idempotency_key            TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256             TEXT NOT NULL CHECK (length(request_sha256)=64),
  expected_relation_revision BIGINT NOT NULL CHECK (expected_relation_revision >= 0),
  expected_campaign_revision BIGINT CHECK (expected_campaign_revision >= 0),
  relation_revision          BIGINT NOT NULL CHECK (relation_revision > 0),
  campaign_revision          BIGINT NOT NULL CHECK (campaign_revision >= 0),
  result                     JSONB NOT NULL,
  created_by                 TEXT NOT NULL DEFAULT '',
  created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_relation
  ON campaign_membership_commands(tenant_id,relation_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_campaign
  ON campaign_membership_commands(tenant_id,campaign_id,campaign_revision DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010730','campaign membership command receipts and aggregate revision binding')
ON CONFLICT (version) DO NOTHING;

COMMIT;
