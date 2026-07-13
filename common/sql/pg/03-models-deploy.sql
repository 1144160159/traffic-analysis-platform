-- =============================================================================
-- 模型版本化 + 部署/灰度发布 (PostgreSQL)
-- 来源: common/old/postgres_ddl.sql
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS models (
  model_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  model_type   TEXT NOT NULL,
  description  TEXT,
  metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

ALTER TABLE models ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE models ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE models SET metadata = '{}'::jsonb WHERE metadata IS NULL;
UPDATE models SET updated_at = created_at WHERE updated_at IS NULL;

CREATE TABLE IF NOT EXISTS model_versions (
  model_version  TEXT PRIMARY KEY,
  model_id       UUID NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  feature_set_id TEXT NOT NULL REFERENCES feature_sets(feature_set_id),
  artifact_uri   TEXT NOT NULL,
  metrics        JSONB NOT NULL DEFAULT '{}'::jsonb,
  status         TEXT NOT NULL DEFAULT 'registered',
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE model_versions ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(user_id);
ALTER TABLE model_versions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE model_versions SET updated_at = created_at WHERE updated_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_models_tenant ON models(tenant_id);
CREATE INDEX IF NOT EXISTS idx_model_versions_model ON model_versions(model_id);
CREATE INDEX IF NOT EXISTS idx_model_versions_status ON model_versions(status);
CREATE INDEX IF NOT EXISTS idx_model_versions_feature_set ON model_versions(feature_set_id);

CREATE TABLE IF NOT EXISTS deployments (
  deployment_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL DEFAULT '',
  description    TEXT NOT NULL DEFAULT '',
  model_version  TEXT REFERENCES model_versions(model_version),
  rule_version   TEXT REFERENCES rule_versions(rule_version),
  feature_set_id TEXT REFERENCES feature_sets(feature_set_id),
  scope          JSONB NOT NULL DEFAULT '{}'::jsonb,
  status         TEXT NOT NULL DEFAULT 'planned',
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  gray_started_at TIMESTAMPTZ,
  gray_expired_at TIMESTAMPTZ,
  activated_at   TIMESTAMPTZ,
  rolled_back_at TIMESTAMPTZ,
  rollback_from  TEXT,
  rollback_reason TEXT NOT NULL DEFAULT '',
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message  TEXT NOT NULL DEFAULT ''
);

ALTER TABLE deployments ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS gray_started_at TIMESTAMPTZ;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS gray_expired_at TIMESTAMPTZ;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS activated_at TIMESTAMPTZ;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rolled_back_at TIMESTAMPTZ;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollback_from TEXT;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollback_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(user_id);

CREATE INDEX IF NOT EXISTS idx_deploy_tenant_time ON deployments (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS deployment_history (
  id            BIGSERIAL PRIMARY KEY,
  deployment_id UUID NOT NULL REFERENCES deployments(deployment_id) ON DELETE CASCADE,
  action        TEXT NOT NULL,
  operator_id   TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail        JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_deployment_history_deployment ON deployment_history (deployment_id, created_at DESC);

COMMIT;
