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
  event_id     UUID NOT NULL UNIQUE,
  action_id    TEXT NOT NULL UNIQUE,
  revision     BIGINT NOT NULL DEFAULT 1,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  model_id     UUID NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
  version      TEXT NOT NULL DEFAULT '',
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','dispatched','awaiting_executor','completed','partial','failed','cancelled')),
  trace_id     TEXT NOT NULL DEFAULT '',
  result       JSONB NOT NULL DEFAULT '{}'::jsonb,
  error        TEXT NOT NULL DEFAULT '',
  requested_by TEXT NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_model_action_jobs_lookup
  ON model_action_jobs (tenant_id, model_id, created_at DESC);

CREATE TABLE IF NOT EXISTS model_action_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE REFERENCES model_action_jobs(event_id) ON DELETE RESTRICT,
  job_id TEXT NOT NULL UNIQUE REFERENCES model_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  model_id UUID NOT NULL,
  partition_key TEXT NOT NULL,
  event_type TEXT NOT NULL DEFAULT 'model.action.requested.v1',
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version=1),
  aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version=1),
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_model_action_outbox_pending ON model_action_outbox(available_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_model_action_outbox_reclaim ON model_action_outbox(locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS model_action_execution_inbox (
  event_id UUID PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE REFERENCES model_action_jobs(job_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL,
  model_id UUID NOT NULL,
  action_id TEXT NOT NULL,
  action TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'awaiting_executor' CHECK (state IN ('awaiting_executor','processing','completed','partial','failed','cancelled','dead_letter')),
  payload JSONB NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error TEXT NOT NULL DEFAULT '',
  kafka_partition INTEGER NOT NULL,
  kafka_offset BIGINT NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_model_action_execution_ready ON model_action_execution_inbox(available_at,received_at) WHERE state='awaiting_executor';
CREATE INDEX IF NOT EXISTS idx_model_action_execution_tenant_model ON model_action_execution_inbox(tenant_id,model_id,received_at DESC);

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

CREATE TABLE IF NOT EXISTS model_update_consumer_readiness (
  consumer_deployment_id TEXT NOT NULL,
  subtask_index INT NOT NULL CHECK (subtask_index >= 0),
  event_id TEXT NOT NULL UNIQUE,
  consumer_profile_sha256 TEXT NOT NULL CHECK (consumer_profile_sha256 ~ '^[0-9a-f]{64}$'),
  runtime_contract TEXT NOT NULL,
  runtime_version TEXT NOT NULL,
  feature_schema_version INT NOT NULL CHECK (feature_schema_version > 0),
  graph_schema_version INT NOT NULL CHECK (graph_schema_version > 0),
  supported_model_formats TEXT NOT NULL,
  parallelism INT NOT NULL CHECK (parallelism > 0 AND subtask_index < parallelism),
  status TEXT NOT NULL DEFAULT 'ready' CHECK (status='ready'),
  payload JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_deployment_id,subtask_index)
);
CREATE INDEX IF NOT EXISTS idx_model_update_consumer_readiness_profile
  ON model_update_consumer_readiness (consumer_profile_sha256,last_seen_at DESC);

CREATE TABLE IF NOT EXISTS model_update_consumer_ready_receipts (
  consumer_deployment_id TEXT PRIMARY KEY,
  consumer_profile_sha256 TEXT NOT NULL CHECK (consumer_profile_sha256 ~ '^[0-9a-f]{64}$'),
  runtime_contract TEXT NOT NULL,
  runtime_version TEXT NOT NULL,
  feature_schema_version INT NOT NULL CHECK (feature_schema_version > 0),
  graph_schema_version INT NOT NULL CHECK (graph_schema_version > 0),
  supported_model_formats TEXT NOT NULL,
  expected_parallelism INT NOT NULL CHECK (expected_parallelism > 0),
  ready_subtasks INT NOT NULL CHECK (ready_subtasks = expected_parallelism),
  status TEXT NOT NULL DEFAULT 'ready' CHECK (status='ready'),
  ready_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_model_update_consumer_ready_active
  ON model_update_consumer_ready_receipts (expires_at,consumer_profile_sha256) WHERE status='ready';

CREATE TABLE IF NOT EXISTS model_shadow_activation_aggregates (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  model_id UUID NOT NULL REFERENCES models(model_id) ON DELETE CASCADE,
  aggregate_revision BIGINT NOT NULL DEFAULT 0 CHECK (aggregate_revision >= 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,model_id)
);

CREATE TABLE IF NOT EXISTS model_shadow_activation_requests (
  request_id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE REFERENCES model_update_outbox(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  model_id UUID NOT NULL REFERENCES models(model_id) ON DELETE RESTRICT,
  model_version TEXT NOT NULL REFERENCES model_versions(model_version) ON DELETE RESTRICT,
  package_id TEXT NOT NULL,
  package_sha256 TEXT NOT NULL CHECK (package_sha256 ~ '^[0-9a-f]{64}$'),
  idempotency_key TEXT NOT NULL,
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision = expected_revision + 1),
  requested_by TEXT NOT NULL,
  approved_by TEXT NOT NULL,
  approval_reason TEXT NOT NULL CHECK (length(approval_reason) BETWEEN 8 AND 1000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key),
  UNIQUE (tenant_id,model_id,aggregate_revision),
  CHECK (requested_by <> approved_by)
);
CREATE INDEX IF NOT EXISTS idx_model_shadow_activation_model
  ON model_shadow_activation_requests (tenant_id,model_id,aggregate_revision DESC);

CREATE TABLE IF NOT EXISTS model_update_shadow_acks (
  event_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  model_version TEXT NOT NULL,
  package_id TEXT NOT NULL,
  package_sha256 TEXT NOT NULL CHECK (package_sha256 ~ '^[0-9a-f]{64}$'),
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  subtask_index INT NOT NULL CHECK (subtask_index >= 0),
  parallelism INT NOT NULL CHECK (parallelism > 0 AND subtask_index < parallelism),
  status TEXT NOT NULL CHECK (status IN ('shadow_ready','stale','duplicate','failed')),
  error TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (event_id,subtask_index)
);
CREATE INDEX IF NOT EXISTS idx_model_update_shadow_acks_quorum
  ON model_update_shadow_acks (event_id,status,subtask_index);
CREATE INDEX IF NOT EXISTS idx_model_update_shadow_acks_model
  ON model_update_shadow_acks (tenant_id,model_id,aggregate_revision DESC);

CREATE TABLE IF NOT EXISTS model_update_shadow_ready_receipts (
  event_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  model_id TEXT NOT NULL,
  model_version TEXT NOT NULL,
  package_id TEXT NOT NULL,
  package_sha256 TEXT NOT NULL CHECK (package_sha256 ~ '^[0-9a-f]{64}$'),
  aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
  expected_parallelism INT NOT NULL CHECK (expected_parallelism > 0),
  ready_subtasks INT NOT NULL CHECK (ready_subtasks = expected_parallelism),
  status TEXT NOT NULL DEFAULT 'shadow_ready' CHECK (status='shadow_ready'),
  ready_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id,model_id,aggregate_revision)
);
CREATE INDEX IF NOT EXISTS idx_model_update_shadow_ready_active
  ON model_update_shadow_ready_receipts (expires_at,tenant_id,model_id) WHERE status='shadow_ready';

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

CREATE TABLE IF NOT EXISTS deployment_event_projection (
    event_id UUID PRIMARY KEY,
    deployment_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    operator_id TEXT NOT NULL DEFAULT '',
    event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_deployment_event_projection_tenant_deployment ON deployment_event_projection (tenant_id,deployment_id,event_timestamp_ms);

CREATE TABLE IF NOT EXISTS deployment_state_projection (
    tenant_id TEXT NOT NULL,
    deployment_id TEXT NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    operator_id TEXT NOT NULL DEFAULT '',
    event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
    last_event_id UUID NOT NULL,
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id,deployment_id)
);
CREATE INDEX IF NOT EXISTS idx_deployment_state_projection_tenant_status ON deployment_state_projection (tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alert_feedback_event_projection (
    event_id UUID PRIMARY KEY,
    feedback_id UUID NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    alert_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL CHECK (label IN ('TP','FP')),
    reason_code TEXT NOT NULL DEFAULT '',
    event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
    payload JSONB NOT NULL,
    kafka_partition INTEGER NOT NULL,
    kafka_offset BIGINT NOT NULL,
    projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_event_tenant_alert ON alert_feedback_event_projection (tenant_id,alert_id,event_timestamp_ms);

CREATE TABLE IF NOT EXISTS model_feedback_inbox (
    feedback_id UUID PRIMARY KEY,
    event_id UUID NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    alert_id TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    label TEXT NOT NULL CHECK (label IN ('TP','FP')),
    reason_code TEXT NOT NULL DEFAULT '',
    model_version TEXT NOT NULL DEFAULT '',
    rule_version TEXT NOT NULL DEFAULT '',
    event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','applied','failed','dead_letter')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT NOT NULL DEFAULT '',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    applied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_pending ON model_feedback_inbox (next_attempt_at,created_at,feedback_id) WHERE status IN ('pending','failed');
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_tenant_model ON model_feedback_inbox (tenant_id,model_version,event_timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_reclaim ON model_feedback_inbox (locked_until,updated_at,feedback_id) WHERE status='processing';
CREATE INDEX IF NOT EXISTS idx_model_feedback_inbox_dead_letter ON model_feedback_inbox (updated_at,feedback_id) WHERE status='dead_letter';

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
