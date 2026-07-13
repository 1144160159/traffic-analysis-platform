-- =============================================================================
-- 任务管理 + 审计日志 (PostgreSQL)
-- 来源: common/old/postgres_ddl.sql
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS tasks (
  task_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name            TEXT NOT NULL DEFAULT '',
  task_type       TEXT NOT NULL,
  params          JSONB NOT NULL DEFAULT '{}'::jsonb,
  status          TEXT NOT NULL DEFAULT 'queued',
  progress        INT NOT NULL DEFAULT 0,
  result_file_key TEXT NOT NULL DEFAULT '',
  result_sha256   TEXT NOT NULL DEFAULT '',
  result_packets  BIGINT NOT NULL DEFAULT 0,
  result_bytes    BIGINT NOT NULL DEFAULT 0,
  files_scanned   INT NOT NULL DEFAULT 0,
  error_message   TEXT NOT NULL DEFAULT '',
  run_id          TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at      TIMESTAMPTZ,
  completed_at    TIMESTAMPTZ
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_file_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_packets BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS files_scanned INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'tasks'::regclass
      AND contype = 'f'
      AND pg_get_constraintdef(oid) LIKE '%created_by%'
  LOOP
    EXECUTE format('ALTER TABLE tasks DROP CONSTRAINT %I', constraint_name);
  END LOOP;
END $$;

ALTER TABLE tasks ALTER COLUMN created_by TYPE TEXT USING created_by::TEXT;
ALTER TABLE tasks ALTER COLUMN created_by SET DEFAULT '';
ALTER TABLE tasks ALTER COLUMN created_by SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET DEFAULT '';
UPDATE tasks SET run_id = '' WHERE run_id IS NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS audit_logs (
  id          BIGSERIAL PRIMARY KEY,
  event_id    TEXT NOT NULL DEFAULT ('audit-' || uuid_generate_v4()::text),
  tenant_id   TEXT NOT NULL,
  user_id     UUID,
  action      TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id   TEXT,
  detail      JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_addr     TEXT,
  user_agent  TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS event_id TEXT;
UPDATE audit_logs SET event_id = 'audit-' || id::TEXT WHERE event_id IS NULL OR event_id = '';
ALTER TABLE audit_logs ALTER COLUMN event_id SET DEFAULT ('audit-' || uuid_generate_v4()::text);
ALTER TABLE audit_logs ALTER COLUMN event_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_event_id ON audit_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant_time ON tasks (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS campaign_action_jobs (
  job_id        TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  campaign_id   TEXT NOT NULL,
  action_id     TEXT NOT NULL,
  target        TEXT NOT NULL,
  metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
  simulation    BOOLEAN NOT NULL DEFAULT true,
  dry_run       BOOLEAN NOT NULL DEFAULT true,
  status        TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  result        JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  created_by    TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_tenant_time
  ON campaign_action_jobs (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_campaign_time
  ON campaign_action_jobs (tenant_id, campaign_id, created_at DESC);

CREATE TABLE IF NOT EXISTS whitelist (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL,
  type            TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet')),
  value           TEXT NOT NULL,
  reason          TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'active',
  approval_status TEXT NOT NULL DEFAULT 'approved',
  source_alert_id TEXT NOT NULL DEFAULT '',
  feedback_id     TEXT NOT NULL DEFAULT '',
  owner_role      TEXT NOT NULL DEFAULT '',
  created_by      TEXT NOT NULL DEFAULT '',
  approved_by     TEXT NOT NULL DEFAULT '',
  approved_at     TIMESTAMPTZ,
  disabled_at     TIMESTAMPTZ,
  expires_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, type, value)
);

ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'approved';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS source_alert_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS feedback_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS owner_role TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist (tenant_id);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_tenant_status ON whitelist (tenant_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_approval ON whitelist (tenant_id, approval_status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_source_alert ON whitelist (tenant_id, source_alert_id) WHERE source_alert_id <> '';
CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist (expires_at) WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS notification_silence_rules (
  rule_id          TEXT PRIMARY KEY,
  tenant_id        TEXT NOT NULL,
  name             TEXT NOT NULL,
  scope            TEXT NOT NULL DEFAULT '',
  starts_at        TIMESTAMPTZ NOT NULL,
  ends_at          TIMESTAMPTZ NOT NULL,
  affected_targets JSONB NOT NULL DEFAULT '[]'::jsonb,
  policy           TEXT NOT NULL DEFAULT 'all',
  reason           TEXT NOT NULL DEFAULT '',
  enabled          BOOLEAN NOT NULL DEFAULT true,
  created_by       TEXT NOT NULL DEFAULT '',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_time
  ON notification_silence_rules (tenant_id, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_enabled
  ON notification_silence_rules (tenant_id, enabled, starts_at DESC);

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
  ON topic_saved_views (tenant_id, topic, updated_at DESC);

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
  PRIMARY KEY (tenant_id, topic)
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
  ON topic_subscriptions (tenant_id, topic, updated_at DESC);

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
  ON topic_exports (tenant_id, generated_at DESC);

COMMIT;
