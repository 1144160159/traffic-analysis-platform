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

CREATE TABLE IF NOT EXISTS model_action_jobs (
  job_id       TEXT PRIMARY KEY,
  action_id    TEXT NOT NULL UNIQUE,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  model_id     UUID NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
  version      TEXT NOT NULL DEFAULT '',
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  requested_by TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_model_action_jobs_lookup
  ON model_action_jobs (tenant_id, model_id, created_at DESC);

-- Transactional model registry outbox. The state change, applied audit and
-- event are committed together; deterministic event_id makes broker retries
-- safe for idempotent consumers. action_job_id links rollback publication to
-- the durable job whose terminal state is acknowledged in the same DB tx.
CREATE TABLE IF NOT EXISTS model_update_outbox (
  id             BIGSERIAL PRIMARY KEY,
  event_id       TEXT NOT NULL UNIQUE,
  tenant_id      TEXT NOT NULL,
  model_id       TEXT NOT NULL,
  model_version  TEXT NOT NULL,
  action         TEXT NOT NULL,
  partition_key  TEXT NOT NULL,
  payload        JSONB NOT NULL,
  action_job_id  TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'published', 'dead')),
  attempt_count  INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at      TIMESTAMPTZ,
  locked_by      TEXT,
  published_at   TIMESTAMPTZ,
  last_error     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_model_update_outbox_ready ON model_update_outbox (available_at, id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_model_update_outbox_aggregate ON model_update_outbox (model_id, id);
CREATE INDEX IF NOT EXISTS idx_model_update_outbox_job ON model_update_outbox (action_job_id) WHERE action_job_id <> '';
CREATE INDEX IF NOT EXISTS idx_model_update_outbox_lease ON model_update_outbox (locked_at) WHERE status = 'processing';

CREATE TABLE IF NOT EXISTS model_update_applied_acks (
    event_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    model_id TEXT NOT NULL,
    model_version TEXT NOT NULL,
    subtask_index INT NOT NULL CHECK (subtask_index >= 0),
    parallelism INT NOT NULL CHECK (parallelism > 0 AND subtask_index < parallelism),
    status TEXT NOT NULL CHECK (status IN ('applied', 'failed')),
    artifact_uri TEXT NOT NULL,
    artifact_sha256 TEXT NOT NULL DEFAULT '',
    warmup_score DOUBLE PRECISION,
    error TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, subtask_index)
);
CREATE INDEX IF NOT EXISTS idx_model_update_applied_acks_status
    ON model_update_applied_acks (event_id, status, subtask_index);
CREATE INDEX IF NOT EXISTS idx_model_update_applied_acks_model
    ON model_update_applied_acks (tenant_id, model_id, applied_at DESC);

CREATE TABLE IF NOT EXISTS model_workbench_items (
  item_id      TEXT PRIMARY KEY,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  model_id     UUID NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
  category     TEXT NOT NULL,
  ordinal      INT NOT NULL DEFAULT 0,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  scenario_id  TEXT NOT NULL DEFAULT 'live',
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, model_id, category, ordinal, scenario_id)
);

CREATE INDEX IF NOT EXISTS idx_model_workbench_lookup
  ON model_workbench_items (tenant_id, model_id, category, ordinal);

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

-- Durable, multi-replica-safe deployment event outbox. Aggregate identifiers
-- intentionally use TEXT and no FK so an event survives aggregate deletion and
-- remains compatible with both UUID and legacy TEXT deployment schemas.
CREATE TABLE IF NOT EXISTS deployment_outbox (
  id             BIGSERIAL PRIMARY KEY,
  event_id       TEXT NOT NULL UNIQUE,
  deployment_id  TEXT NOT NULL,
  tenant_id      TEXT NOT NULL,
  event_type     TEXT NOT NULL,
  schema_version INT NOT NULL DEFAULT 1,
  topic          TEXT NOT NULL DEFAULT 'deployment.events.v1',
  partition_key  TEXT NOT NULL,
  payload        JSONB NOT NULL,
  occurred_at    TIMESTAMPTZ NOT NULL,
  status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'published', 'dead')),
  attempt_count  INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_at      TIMESTAMPTZ,
  locked_by      TEXT,
  published_at   TIMESTAMPTZ,
  last_error     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_deployment_outbox_ready ON deployment_outbox (available_at, created_at, id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_deployment_outbox_aggregate ON deployment_outbox (deployment_id, id);
CREATE INDEX IF NOT EXISTS idx_deployment_outbox_lease ON deployment_outbox (locked_at) WHERE status = 'processing';
CREATE INDEX IF NOT EXISTS idx_deployment_outbox_published ON deployment_outbox (published_at) WHERE status = 'published';

CREATE TABLE IF NOT EXISTS deployment_workbench_items (
  item_id       TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  deployment_id TEXT NOT NULL,
  category      TEXT NOT NULL,
  ordinal       INT NOT NULL DEFAULT 0,
  payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
  scenario_id   TEXT NOT NULL DEFAULT 'live',
  occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, deployment_id, category, ordinal, scenario_id)
);

CREATE INDEX IF NOT EXISTS idx_deployment_workbench_lookup
  ON deployment_workbench_items (tenant_id, deployment_id, category, ordinal);

COMMIT;
