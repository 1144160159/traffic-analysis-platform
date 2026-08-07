-- F-ALERT-002 / F-TOPIC-001 / T-PG-001 / T-SCHEMA-001
-- Expand: establish campaign workbench and topic governance tables through the
-- versioned migration authority before removing request-time DDL.
-- Backfill: additive defaults preserve legacy rows; no campaign or topic
-- business state is synthesized.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202607302115';
--   SELECT table_name,column_name FROM information_schema.columns
--     WHERE table_name IN (
--       'campaign_workbench_state','campaign_reports','topic_saved_views',
--       'topic_scope_overrides','topic_subscriptions','topic_exports',
--       'topic_actions'
--     );
-- Cutover: deploy alert-service only after this migration commits and the
-- service schema-capability preflight succeeds.
-- Rollback: roll back the service image; retain the additive tables, columns
-- and migration record so accepted jobs and audit evidence remain available.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS campaign_workbench_state (
  tenant_id    TEXT NOT NULL,
  campaign_id  TEXT NOT NULL,
  assignee     TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','investigating','contained','closed')),
  state_version BIGINT NOT NULL DEFAULT 1,
  updated_by   TEXT NOT NULL DEFAULT '',
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,campaign_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_workbench_state_tenant_status
  ON campaign_workbench_state(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS campaign_reports (
  report_id      TEXT PRIMARY KEY,
  tenant_id      TEXT NOT NULL,
  campaign_id    TEXT NOT NULL,
  format         TEXT NOT NULL DEFAULT 'pdf'
    CHECK (format IN ('pdf','word','json')),
  status         TEXT NOT NULL DEFAULT 'completed'
    CHECK (status IN ('queued','running','completed','failed')),
  sections       JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_count INTEGER NOT NULL DEFAULT 0,
  created_by     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_campaign_reports_campaign_time
  ON campaign_reports(tenant_id,campaign_id,created_at DESC);

CREATE TABLE IF NOT EXISTS topic_saved_views (
  view_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL,
  topic       TEXT NOT NULL,
  name        TEXT NOT NULL,
  filters     JSONB NOT NULL DEFAULT '{}'::jsonb,
  visibility  TEXT NOT NULL DEFAULT 'private',
  favorite    BOOLEAN NOT NULL DEFAULT false,
  shared      BOOLEAN NOT NULL DEFAULT false,
  share_token TEXT,
  created_by  TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_topic_saved_views_tenant_topic
  ON topic_saved_views(tenant_id,topic,updated_at DESC);

CREATE TABLE IF NOT EXISTS topic_scope_overrides (
  tenant_id       TEXT NOT NULL,
  topic           TEXT NOT NULL,
  scope_name      TEXT NOT NULL DEFAULT '',
  included_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
  excluded_assets JSONB NOT NULL DEFAULT '[]'::jsonb,
  risk_levels     JSONB NOT NULL DEFAULT '[]'::jsonb,
  time_window     TEXT NOT NULL DEFAULT '24h',
  detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
  updated_by      TEXT NOT NULL DEFAULT '',
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,topic)
);

CREATE TABLE IF NOT EXISTS topic_subscriptions (
  subscription_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id        TEXT NOT NULL,
  topic            TEXT NOT NULL,
  channel          TEXT NOT NULL,
  threshold        TEXT NOT NULL DEFAULT 'high',
  schedule         TEXT NOT NULL DEFAULT 'realtime',
  recipients       JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_by       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail           JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_topic_subscriptions_tenant_topic
  ON topic_subscriptions(tenant_id,topic,updated_at DESC);

CREATE TABLE IF NOT EXISTS topic_exports (
  export_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL,
  topic        TEXT NOT NULL,
  export_type  TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'completed',
  parameters   JSONB NOT NULL DEFAULT '{}'::jsonb,
  result       JSONB NOT NULL DEFAULT '{}'::jsonb,
  generated_by TEXT NOT NULL DEFAULT '',
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_topic_exports_tenant_time
  ON topic_exports(tenant_id,generated_at DESC);

CREATE TABLE IF NOT EXISTS topic_actions (
  action_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id         TEXT NOT NULL,
  topic             TEXT NOT NULL,
  action            TEXT NOT NULL,
  target            TEXT NOT NULL,
  status            TEXT NOT NULL DEFAULT 'queued',
  detail            JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by      TEXT NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  idempotency_key   TEXT,
  snapshot_id       UUID,
  expected_revision BIGINT NOT NULL DEFAULT 0,
  revision          BIGINT NOT NULL DEFAULT 1,
  reason            TEXT NOT NULL DEFAULT '',
  trace_id          TEXT NOT NULL DEFAULT '',
  executor          TEXT NOT NULL DEFAULT 'legacy',
  receipt           JSONB NOT NULL DEFAULT '{}'::jsonb,
  error             JSONB NOT NULL DEFAULT '{}'::jsonb,
  attempts          INTEGER NOT NULL DEFAULT 0,
  lease_until       TIMESTAMPTZ
);
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS snapshot_id UUID;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS executor TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS receipt JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS error JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_topic_actions_tenant_topic
  ON topic_actions(tenant_id,topic,created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_actions_tenant_idempotency
  ON topic_actions(tenant_id,idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_topic_actions_executor_queue
  ON topic_actions(executor,status,created_at)
  WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302115','campaign and topic schema authority')
ON CONFLICT (version) DO NOTHING;

COMMIT;
