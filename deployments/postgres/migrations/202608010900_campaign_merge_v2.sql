-- F-CAMPAIGN-001 / T-PG-001 / T-SCHEMA-001
-- Expand: persist one immutable receipt and one per-alert outcome manifest for
-- every source-to-target campaign merge. The service locks both aggregate rows
-- in lexical campaign-id order and updates state, membership histories, both
-- outboxes, the job receipt and audit in one serializable transaction.
-- Backfill: none. Historical campaigns remain unchanged until an explicitly
-- authorized merge command is submitted with both expected revisions.
-- Shadow/verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608010900';
--   SELECT count(*) FROM campaign_merge_receipts WHERE manifest_sha256='';
--   SELECT r.merge_id FROM campaign_merge_receipts r LEFT JOIN campaign_merge_items i
--     ON i.merge_id=r.merge_id GROUP BY r.merge_id,r.source_member_count
--     HAVING count(i.*)<>r.source_member_count;
-- Cutover: expose campaign-merge only with CAMPAIGN_AGGREGATE_V2_ENABLED=true.
-- Rollback: disable the feature and roll back the service image. Retain these
-- additive immutable receipts and event histories for audit and replay proof.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS campaign_merge_receipts (
  merge_id                       UUID PRIMARY KEY,
  job_id                         TEXT NOT NULL UNIQUE,
  tenant_id                      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_campaign_id             TEXT NOT NULL,
  target_campaign_id             TEXT NOT NULL,
  idempotency_key                TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256                 TEXT NOT NULL CHECK (length(request_sha256)=64),
  source_expected_revision       BIGINT NOT NULL CHECK (source_expected_revision>=0),
  target_expected_revision       BIGINT NOT NULL CHECK (target_expected_revision>=0),
  source_revision                BIGINT NOT NULL CHECK (source_revision>source_expected_revision),
  target_revision                BIGINT NOT NULL CHECK (target_revision>target_expected_revision),
  source_member_count            INTEGER NOT NULL CHECK (source_member_count>0 AND source_member_count<=1000),
  target_member_count_before     INTEGER NOT NULL CHECK (target_member_count_before>=0),
  target_member_count_after      INTEGER NOT NULL CHECK (target_member_count_after>=target_member_count_before),
  moved_count                    INTEGER NOT NULL CHECK (moved_count>=0),
  relinked_count                 INTEGER NOT NULL CHECK (relinked_count>=0),
  deduplicated_count             INTEGER NOT NULL CHECK (deduplicated_count>=0),
  manifest                       JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'),
  manifest_sha256                TEXT NOT NULL CHECK (length(manifest_sha256)=64),
  reason                         TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id                       TEXT NOT NULL CHECK (length(trace_id)>0),
  created_by                     TEXT NOT NULL DEFAULT '',
  created_at                     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key),
  UNIQUE (tenant_id,source_campaign_id),
  CHECK (source_campaign_id<>target_campaign_id),
  CHECK (moved_count+relinked_count+deduplicated_count=source_member_count),
  CHECK (target_member_count_after=target_member_count_before+moved_count+relinked_count)
);
CREATE INDEX IF NOT EXISTS idx_campaign_merge_receipts_target
  ON campaign_merge_receipts(tenant_id,target_campaign_id,created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_merge_items (
  merge_id                    UUID NOT NULL REFERENCES campaign_merge_receipts(merge_id)
                              ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
  tenant_id                   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  source_relation_id          UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  target_relation_id          UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  alert_id                    TEXT NOT NULL,
  outcome                     TEXT NOT NULL CHECK (outcome IN ('moved','relinked','deduplicated')),
  source_relation_revision    BIGINT NOT NULL CHECK (source_relation_revision>0),
  target_relation_revision    BIGINT NOT NULL CHECK (target_relation_revision>0),
  source_event_id             UUID NOT NULL REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  target_event_id             UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (merge_id,alert_id),
  UNIQUE (merge_id,source_relation_id),
  CHECK ((outcome='deduplicated' AND target_event_id IS NULL) OR
         (outcome IN ('moved','relinked') AND target_event_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_campaign_merge_items_tenant_alert
  ON campaign_merge_items(tenant_id,alert_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010900','versioned campaign merge receipts and deterministic member outcomes')
ON CONFLICT (version) DO NOTHING;

COMMIT;
