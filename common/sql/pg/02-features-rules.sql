-- =============================================================================
-- 特征集 + 规则版本化 (PostgreSQL)
-- 来源: common/old/postgres_ddl.sql
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS feature_sets (
  feature_set_id TEXT PRIMARY KEY,
  tenant_id      TEXT,
  name           TEXT NOT NULL,
  params         JSONB NOT NULL DEFAULT '{}'::jsonb,
  schema_version TEXT NOT NULL DEFAULT 'v1',
  status         TEXT NOT NULL DEFAULT 'active',
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE feature_sets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS tenant_config (
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  config_key     TEXT NOT NULL,
  config_value   JSONB NOT NULL DEFAULT '{}'::jsonb,
  description    TEXT,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, config_key)
);

CREATE TABLE IF NOT EXISTS rules (
  rule_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  rule_type   TEXT NOT NULL DEFAULT 'custom',
  engine      TEXT NOT NULL DEFAULT 'internal',
  description TEXT NOT NULL DEFAULT '',
  conditions  JSONB NOT NULL DEFAULT '{}'::jsonb,
  labels      TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  severity    TEXT NOT NULL DEFAULT 'medium',
  enabled     BOOLEAN NOT NULL DEFAULT false,
  priority    INT NOT NULL DEFAULT 50,
  version     BIGINT NOT NULL DEFAULT 1,
  status      TEXT NOT NULL DEFAULT 'draft',
  created_by  TEXT NOT NULL DEFAULT '',
  updated_by  TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

ALTER TABLE rules ADD COLUMN IF NOT EXISTS rule_type TEXT NOT NULL DEFAULT 'custom';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS conditions JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS labels TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];
ALTER TABLE rules ADD COLUMN IF NOT EXISTS severity TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 50;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE rules ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT '';
ALTER TABLE rules ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE TABLE IF NOT EXISTS rule_versions (
  rule_version TEXT PRIMARY KEY,
  rule_id      UUID NOT NULL REFERENCES rules(rule_id) ON DELETE CASCADE,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  version      BIGINT NOT NULL DEFAULT 1,
  content_uri  TEXT NOT NULL,
  checksum     TEXT,
  status       TEXT NOT NULL DEFAULT 'registered',
  change_log   TEXT NOT NULL DEFAULT '',
  created_by   TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE rule_versions ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE rule_versions ADD COLUMN IF NOT EXISTS change_log TEXT NOT NULL DEFAULT '';

DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT conname
    FROM pg_constraint
    WHERE conrelid = 'rule_versions'::regclass
      AND contype = 'f'
      AND pg_get_constraintdef(oid) LIKE '%created_by%'
  LOOP
    EXECUTE format('ALTER TABLE rule_versions DROP CONSTRAINT %I', constraint_name);
  END LOOP;
END $$;

ALTER TABLE rule_versions ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE rule_versions ALTER COLUMN created_by TYPE TEXT USING created_by::TEXT;
ALTER TABLE rule_versions ALTER COLUMN created_by SET DEFAULT '';
ALTER TABLE rule_versions ALTER COLUMN created_by SET NOT NULL;

CREATE TABLE IF NOT EXISTS rule_outbox (
  id           BIGSERIAL PRIMARY KEY,
  rule_id      UUID NOT NULL REFERENCES rules(rule_id) ON DELETE CASCADE,
  event_type   TEXT NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published    BOOLEAN NOT NULL DEFAULT false,
  published_at TIMESTAMPTZ,
  retry_count  INT NOT NULL DEFAULT 0,
  last_error   TEXT NOT NULL DEFAULT '',
  next_retry   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rule_outbox_pending ON rule_outbox (published, next_retry, created_at) WHERE published = false;
CREATE INDEX IF NOT EXISTS idx_rule_outbox_rule_id ON rule_outbox (rule_id);

CREATE TABLE IF NOT EXISTS rule_workbench_items (
  item_id      TEXT PRIMARY KEY,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  rule_id      TEXT NOT NULL,
  category     TEXT NOT NULL,
  ordinal      INT NOT NULL DEFAULT 0,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  scenario_id  TEXT NOT NULL DEFAULT 'live',
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, rule_id, category, ordinal, scenario_id)
);

CREATE INDEX IF NOT EXISTS idx_rule_workbench_lookup
  ON rule_workbench_items (tenant_id, rule_id, category, ordinal);

CREATE TABLE IF NOT EXISTS rule_action_jobs (
  job_id       TEXT PRIMARY KEY,
  action_id    TEXT NOT NULL UNIQUE,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  rule_id      TEXT NOT NULL,
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status       TEXT NOT NULL DEFAULT 'queued',
  requested_by TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rule_action_jobs_lookup
  ON rule_action_jobs (tenant_id, rule_id, created_at DESC);

COMMIT;
