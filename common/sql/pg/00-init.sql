-- =============================================================================
-- 初始化: 扩展 + 租户 + 用户 + RBAC + API Tokens
-- 来源: common/old/postgres_ddl.sql (已合并)
-- =============================================================================
BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Migration fragments loaded by other entrypoints may register versions before
-- the historical definition near the end of this file is reached.
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

-- 租户
CREATE TABLE IF NOT EXISTS tenants (
  tenant_id      TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'active',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tenants ADD COLUMN IF NOT EXISTS tenant_name TEXT;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS name TEXT;
UPDATE tenants
SET name = COALESCE(NULLIF(name, ''), NULLIF(tenant_name, ''), tenant_id)
WHERE name IS NULL OR name = '';
ALTER TABLE tenants ALTER COLUMN name SET NOT NULL;
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 用户
CREATE TABLE IF NOT EXISTS users (
  user_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  username       TEXT NOT NULL,
  email          TEXT,
  status         TEXT NOT NULL DEFAULT 'active',
  password_hash  TEXT,
  external_id    TEXT,
  last_login_at  TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, username)
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE users ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_username ON users (tenant_id, username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_external_id ON users (external_id) WHERE external_id IS NOT NULL;

-- 角色
CREATE TABLE IF NOT EXISTS roles (
  role_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  permissions JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_system   BOOLEAN NOT NULL DEFAULT false,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

ALTER TABLE roles ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE roles ADD COLUMN IF NOT EXISTS permissions JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE roles ADD COLUMN IF NOT EXISTS is_system BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE roles ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE roles ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE UNIQUE INDEX IF NOT EXISTS idx_roles_tenant_name ON roles (tenant_id, name);

-- 用户-角色关联
CREATE TABLE IF NOT EXISTS user_roles (
  user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 用户偏好设置
CREATE TABLE IF NOT EXISTS user_settings (
  tenant_id  TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  user_id    UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  category   TEXT NOT NULL,
  settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
  revision   BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id, category)
);

ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_user_settings_user ON user_settings (tenant_id, user_id);

CREATE TABLE IF NOT EXISTS user_settings_history (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  revision BIGINT NOT NULL,
  action_id TEXT NOT NULL,
  reason TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, user_id, category, revision)
);

CREATE TABLE IF NOT EXISTS user_settings_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1,
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_ready ON user_settings_outbox(next_retry_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_reclaim ON user_settings_outbox(locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS user_settings_requests (
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  category TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL,
  action_id TEXT NOT NULL,
  resulting_revision BIGINT NOT NULL,
  event_id UUID NOT NULL REFERENCES user_settings_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,user_id,category,idempotency_key)
);

CREATE TABLE IF NOT EXISTS user_command_history (
  history_id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, user_id UUID NOT NULL,
  revision BIGINT NOT NULL CHECK(revision>0), action_id TEXT NOT NULL, actor_id UUID,
  reason TEXT NOT NULL, trace_id TEXT NOT NULL, old_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  new_value JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,user_id,revision,action_id)
);
CREATE INDEX IF NOT EXISTS idx_user_command_history_lookup ON user_command_history(tenant_id,user_id,revision DESC);
CREATE TABLE IF NOT EXISTS user_command_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL, aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0),
  event_type TEXT NOT NULL, schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version=1),
  partition_key TEXT NOT NULL, payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK(publish_attempts>=0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  UNIQUE(tenant_id,user_id,aggregate_version,event_type)
);
CREATE INDEX IF NOT EXISTS idx_user_command_outbox_ready ON user_command_outbox(next_retry_at,occurred_at,outbox_id) WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS user_command_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, user_id UUID NOT NULL,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK(length(request_hash)=64), action_id TEXT NOT NULL,
  expected_revision BIGINT NOT NULL CHECK(expected_revision>=0), resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0),
  response_payload JSONB NOT NULL, event_id UUID NOT NULL REFERENCES user_command_outbox(event_id) ON DELETE RESTRICT,
  actor_id UUID, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_user_command_requests_user ON user_command_requests(tenant_id,user_id,created_at DESC);

-- 租户级系统设置。与 user_settings 的个人偏好分离，revision 用于防止并发覆盖。
CREATE TABLE IF NOT EXISTS tenant_system_settings (
  tenant_id   TEXT PRIMARY KEY REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  settings    JSONB NOT NULL DEFAULT '{}'::jsonb,
  revision    BIGINT NOT NULL DEFAULT 1,
  updated_by  UUID,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tenant_system_settings ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE tenant_system_settings ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE tenant_system_settings ADD COLUMN IF NOT EXISTS updated_by UUID;
ALTER TABLE tenant_system_settings ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE tenant_system_settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_tenant_system_settings_updated ON tenant_system_settings (updated_at DESC);

-- API Tokens
CREATE TABLE IF NOT EXISTS api_tokens (
  token_id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id         TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  user_id           UUID REFERENCES users(user_id) ON DELETE SET NULL,
  name              TEXT NOT NULL,
  description       TEXT,
  token_type        TEXT NOT NULL DEFAULT 'api',
  token_hash        TEXT NOT NULL,
  token_prefix      TEXT,
  scopes            JSONB NOT NULL DEFAULT '[]'::jsonb,
  status            TEXT NOT NULL DEFAULT 'active',
  expires_at        TIMESTAMPTZ,
  last_used_at      TIMESTAMPTZ,
  usage_count       BIGINT NOT NULL DEFAULT 0,
  created_by        UUID REFERENCES users(user_id),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at        TIMESTAMPTZ,
  rotation_enabled  BOOLEAN NOT NULL DEFAULT false,
  rotation_interval INT,
  last_rotated_at   TIMESTAMPTZ,
  previous_token_id UUID,
  ip_whitelist      JSONB NOT NULL DEFAULT '[]'::jsonb,
  metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
  probe_id          TEXT
);

ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(user_id) ON DELETE SET NULL;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS token_type TEXT NOT NULL DEFAULT 'api';
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS token_prefix TEXT;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS usage_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(user_id);
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS rotation_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS rotation_interval INT;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS last_rotated_at TIMESTAMPTZ;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS previous_token_id UUID;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS ip_whitelist JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS probe_id TEXT;
ALTER TABLE api_tokens ALTER COLUMN scopes TYPE JSONB USING COALESCE(to_jsonb(scopes), '[]'::jsonb);
ALTER TABLE api_tokens ALTER COLUMN scopes SET DEFAULT '[]'::jsonb;
ALTER TABLE api_tokens ALTER COLUMN scopes SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens (token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_tenant_status ON api_tokens (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_api_tokens_probe ON api_tokens (probe_id);

-- 会话撤销表
CREATE TABLE IF NOT EXISTS revoked_sessions (
  session_id TEXT PRIMARY KEY,
  user_id    UUID,
  tenant_id  TEXT NOT NULL DEFAULT '',
  revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  reason     TEXT NOT NULL DEFAULT ''
);

ALTER TABLE revoked_sessions ADD COLUMN IF NOT EXISTS user_id UUID;
ALTER TABLE revoked_sessions ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';
ALTER TABLE revoked_sessions ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE revoked_sessions ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE revoked_sessions ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_revoked_sessions_expires ON revoked_sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_revoked_sessions_tenant ON revoked_sessions (tenant_id, revoked_at DESC);

-- 探针注册
CREATE TABLE IF NOT EXISTS probes (
  probe_id         TEXT PRIMARY KEY,
  tenant_id        TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name             TEXT NOT NULL,
  status           TEXT NOT NULL DEFAULT 'active',
  location         TEXT,
  metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
  hardware_info    JSONB,
  software_version TEXT,
  last_heartbeat   TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

ALTER TABLE probes ADD COLUMN IF NOT EXISTS tenant_id TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE probes ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE probes ADD COLUMN IF NOT EXISTS location TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS last_heartbeat TIMESTAMPTZ;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_probes_tenant ON probes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_probes_status ON probes(status);
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='probes'::regclass AND conname='probes_tenant_id_name_key'
  ) THEN
    ALTER TABLE probes ADD CONSTRAINT probes_tenant_id_name_key UNIQUE (tenant_id,name);
  END IF;
END $$;

-- 探针运维操作流水
CREATE TABLE IF NOT EXISTS probe_operations (
  operation_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  probe_id      TEXT NOT NULL,
  operation_type TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'queued',
  requested_by  TEXT NOT NULL DEFAULT '',
  request       JSONB NOT NULL DEFAULT '{}'::jsonb,
  result        JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_probe_time ON probe_operations (tenant_id, probe_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_type_time ON probe_operations (tenant_id, operation_type, created_at DESC);

ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS command_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS state_revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS desired_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS command_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS reported_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS reported_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS agent_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS ack_error TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT (now()+interval '10 minutes');
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE probe_operations SET expires_at=created_at+interval '10 minutes' WHERE command_revision=0;
WITH ranked AS (
  SELECT operation_id,row_number() OVER (
    PARTITION BY tenant_id,probe_id ORDER BY created_at,operation_id
  ) AS revision
  FROM probe_operations WHERE command_revision=0
)
UPDATE probe_operations p SET command_revision=ranked.revision
FROM ranked WHERE p.operation_id=ranked.operation_id;
UPDATE probe_operations
SET status=CASE WHEN expires_at<=now() THEN 'expired' ELSE 'accepted' END
WHERE status='queued';
UPDATE probe_operations
SET desired_version=COALESCE(
  NULLIF(request->>'config_version',''),NULLIF(request->>'target_version',''),
  NULLIF(request->>'desired_state',''),NULLIF(request->>'rotation_window',''),''
) WHERE desired_version='';
UPDATE probe_operations SET command_hash='legacy-unavailable' WHERE command_hash='';
CREATE UNIQUE INDEX IF NOT EXISTS uq_probe_operations_tenant_idempotency ON probe_operations (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_probe_operations_command_revision ON probe_operations (tenant_id,probe_id,command_revision);
CREATE INDEX IF NOT EXISTS idx_probe_operations_status_expiry ON probe_operations (status,expires_at) WHERE status IN ('accepted','delivered');
CREATE TABLE IF NOT EXISTS probe_operation_history (
  history_id BIGSERIAL PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, state_revision BIGINT NOT NULL CHECK (state_revision > 0),
  from_status TEXT NOT NULL, to_status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (operation_id,state_revision)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_history_tenant_operation ON probe_operation_history (tenant_id,operation_id,state_revision);
CREATE TABLE IF NOT EXISTS probe_operation_ack_receipts (
  ack_id UUID PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, probe_id TEXT NOT NULL, command_revision BIGINT NOT NULL CHECK (command_revision > 0),
  reported_version TEXT NOT NULL DEFAULT '', reported_hash TEXT NOT NULL, agent_version TEXT NOT NULL,
  applied BOOLEAN NOT NULL, error TEXT NOT NULL DEFAULT '', acknowledged_at TIMESTAMPTZ NOT NULL,
  accepted BOOLEAN NOT NULL, rejection_reason TEXT NOT NULL DEFAULT '', payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (operation_id)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_ack_tenant_probe ON probe_operation_ack_receipts (tenant_id,probe_id,command_revision DESC);
CREATE TABLE IF NOT EXISTS probe_operation_outbox (
  event_id UUID PRIMARY KEY, operation_id UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0), partition_key TEXT NOT NULL,
  payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
	publish_state TEXT NOT NULL DEFAULT 'PENDING',
	broker_topic TEXT NOT NULL DEFAULT '', broker_partition INTEGER, broker_offset BIGINT,
	publish_attempt UUID, acked_at TIMESTAMPTZ,
  UNIQUE (operation_id,event_type)
);
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS publish_state TEXT NOT NULL DEFAULT 'PENDING';
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS broker_topic TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS broker_partition INTEGER;
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS broker_offset BIGINT;
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS publish_attempt UUID;
ALTER TABLE probe_operation_outbox ADD COLUMN IF NOT EXISTS acked_at TIMESTAMPTZ;
UPDATE probe_operation_outbox SET publish_state=CASE WHEN published THEN 'KAFKA_ACKED' ELSE 'PENDING' END
WHERE publish_state NOT IN ('OUTCOME_UNKNOWN','KAFKA_ACKED') OR published;
ALTER TABLE probe_operation_outbox DROP CONSTRAINT IF EXISTS probe_operation_outbox_publish_state_check;
ALTER TABLE probe_operation_outbox ADD CONSTRAINT probe_operation_outbox_publish_state_check
CHECK (publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')) NOT VALID;
ALTER TABLE probe_operation_outbox VALIDATE CONSTRAINT probe_operation_outbox_publish_state_check;
ALTER TABLE probe_operation_outbox DROP CONSTRAINT IF EXISTS probe_operation_outbox_publish_compatibility_check;
ALTER TABLE probe_operation_outbox ADD CONSTRAINT probe_operation_outbox_publish_compatibility_check
CHECK ((published AND publish_state='KAFKA_ACKED') OR
       (NOT published AND publish_state IN ('PENDING','OUTCOME_UNKNOWN'))) NOT VALID;
ALTER TABLE probe_operation_outbox VALIDATE CONSTRAINT probe_operation_outbox_publish_compatibility_check;
DROP INDEX IF EXISTS idx_probe_operation_outbox_pending;
CREATE INDEX idx_probe_operation_outbox_pending ON probe_operation_outbox (next_attempt_at,created_at)
WHERE publish_state IN ('PENDING','OUTCOME_UNKNOWN');
CREATE TABLE IF NOT EXISTS probe_pipeline_readiness_epochs (
  pipeline_id TEXT NOT NULL,
  consumer_role TEXT NOT NULL CHECK (consumer_role IN ('COMMAND_DELIVERY','ACK_AUTHORITY','LIFECYCLE_PROJECTION')),
  consumer_group TEXT NOT NULL, owner_id TEXT NOT NULL, owner_epoch BIGINT NOT NULL CHECK (owner_epoch > 0),
  ready BOOLEAN NOT NULL, observed_at TIMESTAMPTZ NOT NULL, lease_expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (pipeline_id,consumer_role),
  CHECK ((ready AND lease_expires_at IS NOT NULL AND revoked_at IS NULL) OR
         (NOT ready AND revoked_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_probe_pipeline_readiness_live
ON probe_pipeline_readiness_epochs (pipeline_id,lease_expires_at) WHERE ready;
CREATE TABLE IF NOT EXISTS kafka_dlq_acknowledgement_receipts (
  consumer_group TEXT NOT NULL, source_topic TEXT NOT NULL,
  source_partition INTEGER NOT NULL CHECK (source_partition >= 0),
  source_offset BIGINT NOT NULL CHECK (source_offset >= 0),
  source_key_sha256 TEXT NOT NULL CHECK (source_key_sha256 ~ '^[0-9a-f]{64}$'),
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  headers_sha256 TEXT NOT NULL CHECK (headers_sha256 ~ '^[0-9a-f]{64}$'),
  error_sha256 TEXT NOT NULL CHECK (error_sha256 ~ '^[0-9a-f]{64}$'),
  acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_group,source_topic,source_partition,source_offset)
);

-- 页面业务状态: 行为基线重置点
CREATE TABLE IF NOT EXISTS behavior_baseline_resets (
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id  TEXT NOT NULL,
  reset_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  requested_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (tenant_id, baseline_id)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_settings (
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id        TEXT NOT NULL,
  warning_multiplier DOUBLE PRECISION NOT NULL DEFAULT 2.0 CHECK (warning_multiplier > 0),
  alert_multiplier   DOUBLE PRECISION NOT NULL DEFAULT 3.0 CHECK (alert_multiplier > warning_multiplier),
  frozen             BOOLEAN NOT NULL DEFAULT false,
  drift_watch        BOOLEAN NOT NULL DEFAULT false,
  version            INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  updated_by         TEXT NOT NULL DEFAULT '',
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, baseline_id)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_actions (
  action_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id    TEXT NOT NULL,
  action_type    TEXT NOT NULL CHECK (action_type IN ('create_alert','adjust_threshold','freeze','unfreeze','forensics','feedback_model','cold_start','drift_watch','rebuild','rollback','audit_trace')),
  status         TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','applied','rejected','failed')),
  reason         TEXT NOT NULL DEFAULT '',
  request        JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by   TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_actions_time ON behavior_baseline_actions (tenant_id, baseline_id, created_at DESC);

CREATE TABLE IF NOT EXISTS behavior_baseline_versions (
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id        TEXT NOT NULL,
  version            INTEGER NOT NULL CHECK (version > 0),
  snapshot           JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_action_id   UUID NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE SET NULL,
  created_by         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, baseline_id, version)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_outbox (
  outbox_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id    TEXT NOT NULL,
  action_id      UUID NOT NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE CASCADE,
  event_type     TEXT NOT NULL,
  payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
  published      BOOLEAN NOT NULL DEFAULT false,
  attempts       INTEGER NOT NULL DEFAULT 0,
  last_error     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at   TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_outbox_pending ON behavior_baseline_outbox (published, created_at) WHERE published=false;

-- 页面业务状态: Fusion 冲突处理与规则编辑回写
CREATE TABLE IF NOT EXISTS fusion_conflict_resolutions (
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  conflict_id     TEXT NOT NULL,
  object_id       TEXT NOT NULL DEFAULT '',
  object_type     TEXT NOT NULL DEFAULT 'entity',
  field_name      TEXT NOT NULL,
  selected_source TEXT NOT NULL,
  selected_value  TEXT NOT NULL,
  strategy        TEXT NOT NULL DEFAULT 'manual',
  note            TEXT NOT NULL DEFAULT '',
  rule_id         TEXT NOT NULL DEFAULT '',
  state_version   BIGINT NOT NULL DEFAULT 1,
  resolved_by     TEXT NOT NULL DEFAULT '',
  resolved_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (tenant_id, conflict_id)
);
CREATE INDEX IF NOT EXISTS idx_fusion_conflict_resolutions_time ON fusion_conflict_resolutions (tenant_id, resolved_at DESC);

CREATE TABLE IF NOT EXISTS fusion_rule_overrides (
  tenant_id            TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  rule_id              TEXT NOT NULL,
  rule_name            TEXT NOT NULL DEFAULT '',
  version              BIGINT NOT NULL DEFAULT 1,
  status               TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('active','draft','disabled')),
  strategy             TEXT NOT NULL DEFAULT 'manual-review' CHECK (strategy IN ('authoritative-source','weighted-confidence','latest-observation','manual-review')),
  confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.85 CHECK (confidence_threshold BETWEEN 0 AND 1),
  note                 TEXT NOT NULL DEFAULT '',
  updated_by           TEXT NOT NULL DEFAULT '',
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail               JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (tenant_id, rule_id)
);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fusion_rule_overrides_status_check') THEN
    ALTER TABLE fusion_rule_overrides ADD CONSTRAINT fusion_rule_overrides_status_check CHECK (status IN ('active','draft','disabled'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fusion_rule_overrides_strategy_check') THEN
    ALTER TABLE fusion_rule_overrides ADD CONSTRAINT fusion_rule_overrides_strategy_check CHECK (strategy IN ('authoritative-source','weighted-confidence','latest-observation','manual-review'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fusion_rule_overrides_threshold_check') THEN
    ALTER TABLE fusion_rule_overrides ADD CONSTRAINT fusion_rule_overrides_threshold_check CHECK (confidence_threshold BETWEEN 0 AND 1);
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_fusion_rule_overrides_time ON fusion_rule_overrides (tenant_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS fusion_conflicts (
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  conflict_id   TEXT NOT NULL,
  object_id     TEXT NOT NULL,
  object_type   TEXT NOT NULL DEFAULT 'entity',
  field_name    TEXT NOT NULL,
  source_values JSONB NOT NULL DEFAULT '[]'::jsonb,
  source_count  INTEGER NOT NULL DEFAULT 0,
  confidence    DOUBLE PRECISION NOT NULL DEFAULT 0,
  severity      TEXT NOT NULL DEFAULT 'medium',
  status        TEXT NOT NULL DEFAULT 'pending',
  rule_id       TEXT NOT NULL DEFAULT '',
  state_version BIGINT NOT NULL DEFAULT 1,
  origin        TEXT NOT NULL DEFAULT 'runtime',
  detail        JSONB NOT NULL DEFAULT '{}'::jsonb,
  detected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, conflict_id)
);
ALTER TABLE fusion_conflicts ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'runtime';
ALTER TABLE fusion_conflicts ADD COLUMN IF NOT EXISTS detail JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_fusion_conflicts_queue ON fusion_conflicts (tenant_id, status, detected_at DESC);

CREATE TABLE IF NOT EXISTS fusion_repair_tasks (
  task_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  conflict_id     TEXT NOT NULL,
  object_id       TEXT NOT NULL DEFAULT '',
  object_type     TEXT NOT NULL DEFAULT 'entity',
  field_name      TEXT NOT NULL,
  rule_id         TEXT NOT NULL DEFAULT '',
  selected_source TEXT NOT NULL,
  selected_value  TEXT NOT NULL,
  state_version   BIGINT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','in_progress','completed','failed','cancelled')),
  requested_by    TEXT NOT NULL DEFAULT '',
  note            TEXT NOT NULL DEFAULT '',
  detail          JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, conflict_id, state_version),
  FOREIGN KEY (tenant_id, conflict_id) REFERENCES fusion_conflicts(tenant_id, conflict_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fusion_repair_tasks_queue ON fusion_repair_tasks (tenant_id, status, created_at DESC);

-- 页面业务状态: 合规报告生成结果
CREATE TABLE IF NOT EXISTS compliance_reports (
  report_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  report_type  TEXT NOT NULL,
  time_start   BIGINT NOT NULL,
  time_end     BIGINT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'completed',
  summary      JSONB NOT NULL DEFAULT '{}'::jsonb,
  sections     JSONB NOT NULL DEFAULT '[]'::jsonb,
  generated_by TEXT NOT NULL DEFAULT '',
  generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_compliance_reports_tenant_time ON compliance_reports (tenant_id, generated_at DESC);

-- 旧版本曾将零样本报告错误标记为 completed/pass；迁移时吊销，禁止继续导出或固化。
UPDATE compliance_reports
SET status = 'invalidated'
WHERE status = 'completed'
  AND COALESCE((summary->>'total_alerts')::bigint, 0) = 0
  AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements(sections) AS section
    WHERE COALESCE(section->>'status', '') <> 'pass'
  );

UPDATE compliance_reports
SET status = 'insufficient_evidence'
WHERE status = 'completed'
  AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(sections) AS section
    WHERE COALESCE(section->>'status', '') IN ('insufficient_evidence', 'blocked')
  );

UPDATE compliance_reports
SET status = 'non_compliant'
WHERE status = 'completed'
  AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(sections) AS section
    WHERE COALESCE(section->>'status', '') IN ('fail', 'warning', 'warn')
  );

CREATE TABLE IF NOT EXISTS compliance_remediation_tasks (
  task_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  report_id    UUID NOT NULL REFERENCES compliance_reports(report_id) ON DELETE CASCADE,
  section_name TEXT NOT NULL,
  title        TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'open',
  created_by   TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, report_id, section_name)
);
CREATE INDEX IF NOT EXISTS idx_compliance_remediation_tenant_time ON compliance_remediation_tasks (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS compliance_finalizations (
  finalization_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  report_id       UUID NOT NULL REFERENCES compliance_reports(report_id) ON DELETE RESTRICT,
  report_sha256   TEXT NOT NULL,
  snapshot        JSONB NOT NULL,
  finalized_by    TEXT NOT NULL DEFAULT '',
  finalized_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, report_id)
);
CREATE INDEX IF NOT EXISTS idx_compliance_finalizations_tenant_time ON compliance_finalizations (tenant_id, finalized_at DESC);

CREATE OR REPLACE FUNCTION prevent_compliance_finalization_mutation()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'compliance finalizations are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS compliance_finalizations_immutable ON compliance_finalizations;
CREATE TRIGGER compliance_finalizations_immutable
BEFORE UPDATE OR DELETE ON compliance_finalizations
FOR EACH ROW EXECUTE FUNCTION prevent_compliance_finalization_mutation();

-- Authenticated probe registration commands are revisioned and replayable.
-- Heartbeat liveness remains a bounded projection and is not audited per tick.
ALTER TABLE probes ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE probes DROP CONSTRAINT IF EXISTS probes_revision_nonnegative;
ALTER TABLE probes ADD CONSTRAINT probes_revision_nonnegative CHECK (revision >= 0);
CREATE TABLE IF NOT EXISTS probe_registry_history (
  history_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0), event_type TEXT NOT NULL,
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,probe_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_probe_registry_history_tenant_probe
  ON probe_registry_history (tenant_id,probe_id,revision);
CREATE TABLE IF NOT EXISTS probe_registry_requests (
  tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL,
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  event_id UUID NOT NULL UNIQUE, resource_revision BIGINT NOT NULL CHECK (resource_revision > 0),
  result JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key)
);
CREATE TABLE IF NOT EXISTS probe_registry_outbox (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0), partition_key TEXT NOT NULL,
  payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_probe_registry_outbox_ready
  ON probe_registry_outbox (next_attempt_at,created_at) WHERE status='pending';
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031540','authenticated probe registration revision history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;

-- 默认数据
INSERT INTO tenants (tenant_id, tenant_name, name)
VALUES ('default', '默认租户', '默认租户')
ON CONFLICT (tenant_id) DO UPDATE
SET
  tenant_name = COALESCE(NULLIF(tenants.tenant_name, ''), EXCLUDED.tenant_name),
  name = COALESCE(NULLIF(tenants.name, ''), EXCLUDED.name);

COMMIT;

-- T-OS-004 is an additive migration owned by deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql.
-- Runtime startup never creates these tables; this bootstrap mirror is for clean-room environments only.
BEGIN;
CREATE TABLE IF NOT EXISTS alert_opensearch_projection_debts (
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, source_event_id TEXT NOT NULL DEFAULT '',
  source_version BIGINT NOT NULL CHECK (source_version>0), source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64),
  target_index_version TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','resolved','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '', first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(), resolved_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_debts_ready ON alert_opensearch_projection_debts(available_at,first_failed_at,tenant_id,alert_id) WHERE status='pending';
CREATE TABLE IF NOT EXISTS alert_opensearch_projection_watermarks (
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, source_event_id TEXT NOT NULL DEFAULT '', source_version BIGINT NOT NULL,
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64), target_index_version TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE TABLE IF NOT EXISTS alert_opensearch_reconcile_runs (
  run_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, requested_by TEXT NOT NULL, trace_id TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('plan','repair')), target_index_version TEXT NOT NULL, start_time TIMESTAMPTZ, end_time TIMESTAMPTZ,
  business_ids JSONB NOT NULL DEFAULT '[]'::jsonb, max_documents INTEGER NOT NULL, stop_error_count INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'running', source_count BIGINT NOT NULL DEFAULT 0, target_count BIGINT NOT NULL DEFAULT 0,
  missing_count BIGINT NOT NULL DEFAULT 0, extra_count BIGINT NOT NULL DEFAULT 0, stale_count BIGINT NOT NULL DEFAULT 0,
  repaired_count BIGINT NOT NULL DEFAULT 0, error_count BIGINT NOT NULL DEFAULT 0, result_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
  stop_reason TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
INSERT INTO alignment_schema_migrations(version,description) VALUES ('202608041100','durable alert OpenSearch projection debt watermarks and bounded reconcile runs') ON CONFLICT (version) DO NOTHING;
COMMIT;
