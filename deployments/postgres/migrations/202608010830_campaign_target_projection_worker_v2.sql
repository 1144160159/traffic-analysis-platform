-- F-CAMPAIGN-001 / WP-08 and T-PG-001 / WP-21
-- Expand-only worker state for independently acknowledged ClickHouse,
-- OpenSearch and NebulaGraph campaign projections.
--
-- Backfill: existing inbox rows retain the three pending target states and
-- become immediately eligible. No target is marked applied by this migration.
-- Cutover: enable CAMPAIGN_TARGET_PROJECTION_V2_ENABLED only after all three
-- target schemas, aliases and credentials pass readiness checks.
-- Rollback: disable the worker. Keep inbox attempts and target watermarks for
-- replay/reconciliation; do not drop or rewrite acknowledged target state.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_event_projection_inbox
  ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0
    CHECK (attempt_count>=0),
  ADD COLUMN IF NOT EXISTS available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ;

ALTER TABLE campaign_event_projection_inbox
  DROP CONSTRAINT IF EXISTS campaign_event_projection_target_status_check;
ALTER TABLE campaign_event_projection_inbox
  ADD CONSTRAINT campaign_event_projection_target_status_check CHECK (
    jsonb_typeof(target_status)='object'
    AND target_status->>'clickhouse' IN ('pending','applied','dead')
    AND target_status->>'opensearch' IN ('pending','applied','dead')
    AND target_status->>'nebulagraph' IN ('pending','applied','dead')
    AND target_status-'clickhouse'-'opensearch'-'nebulagraph'='{}'::jsonb
  );

DROP INDEX IF EXISTS idx_campaign_event_projection_pending;
CREATE INDEX idx_campaign_event_projection_pending
  ON campaign_event_projection_inbox(available_at,received_at,stream,event_id)
  WHERE projection_status IN ('pending','processing','partial');

CREATE TABLE IF NOT EXISTS campaign_target_projection_watermarks (
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  projection_key     TEXT NOT NULL,
  target             TEXT NOT NULL
                     CHECK (target IN ('clickhouse','opensearch','nebulagraph')),
  stream             TEXT NOT NULL CHECK (stream IN ('aggregate','membership')),
  projection_version BIGINT NOT NULL CHECK (projection_version>0),
  event_id           UUID NOT NULL,
  projection_sha256  TEXT NOT NULL CHECK (length(projection_sha256)=64),
  applied_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,projection_key,target),
  FOREIGN KEY (stream,event_id)
    REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_event
  ON campaign_target_projection_watermarks(stream,event_id,target);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_version
  ON campaign_target_projection_watermarks(tenant_id,target,projection_version DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010830','campaign three-target projection worker v2')
ON CONFLICT (version) DO NOTHING;

COMMIT;
