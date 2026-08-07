-- =========================================================================================
-- MERGED PostgreSQL DDL for Traffic Analysis Platform
-- 合并来源：
-- - postgres_ddl.sql (基础表)
-- - alert_postgres.sql (告警服务控制面)
-- - auth_postgres_ddl.sql (认证服务 API Token 管理)
-- - forensics_postgres.sql (取证服务控制面)
-- - graph_postgres.sql (图服务配置)
-- - rules_postgres.sql (规则服务扩展)
--
-- 合并策略：
-- 1. 基础表保留原有定义
-- 2. 扩展表独立创建
-- 3. 字段扩展通过新建表实现（避免 ALTER）
-- 4. 触发器和索引统一管理
-- =========================================================================================

BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

-- =========================================================================================
-- 租户与用户管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- tenants: 租户表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenants (
  tenant_id      TEXT PRIMARY KEY,
  name           TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'active',
  quota_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);

-- -----------------------------------------------------------------------------------------
-- users: 用户表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
  user_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  username       TEXT NOT NULL,
  email          TEXT,
  password_hash  TEXT,
  status         TEXT NOT NULL DEFAULT 'active',
  last_login_at  TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, username)
);

CREATE INDEX IF NOT EXISTS idx_users_tenant ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
ALTER TABLE users ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0);

-- =========================================================================================
-- RBAC（基于角色的访问控制）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- roles: 角色表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS roles (
  role_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  description    TEXT,
  permissions    JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_system      BOOLEAN NOT NULL DEFAULT false,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_roles_tenant ON roles(tenant_id);

-- -----------------------------------------------------------------------------------------
-- user_roles: 用户-角色关联表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_roles (
  user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  role_id UUID NOT NULL REFERENCES roles(role_id) ON DELETE CASCADE,
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role ON user_roles(role_id);

-- -----------------------------------------------------------------------------------------
-- user_settings / tenant_system_settings: 个人偏好与租户级系统设置分离
-- -----------------------------------------------------------------------------------------
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

ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_user_settings_user ON user_settings (tenant_id, user_id);

CREATE TABLE IF NOT EXISTS user_settings_history (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, user_id UUID NOT NULL, category TEXT NOT NULL,
  revision BIGINT NOT NULL, action_id TEXT NOT NULL, reason TEXT NOT NULL, snapshot JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,user_id,category,revision)
);
CREATE TABLE IF NOT EXISTS user_settings_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL, user_id UUID NOT NULL,
  category TEXT NOT NULL, aggregate_version BIGINT NOT NULL, event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1, partition_key TEXT NOT NULL, payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0, next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_ready ON user_settings_outbox(next_retry_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_user_settings_outbox_reclaim ON user_settings_outbox(locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS user_settings_requests (
  tenant_id TEXT NOT NULL, user_id UUID NOT NULL, category TEXT NOT NULL, idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL, action_id TEXT NOT NULL, resulting_revision BIGINT NOT NULL,
  event_id UUID NOT NULL REFERENCES user_settings_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
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
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK(publish_attempts>=0), next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  UNIQUE(tenant_id,user_id,aggregate_version,event_type)
);
CREATE INDEX IF NOT EXISTS idx_user_command_outbox_ready ON user_command_outbox(next_retry_at,occurred_at,outbox_id) WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS user_command_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, user_id UUID NOT NULL,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
  action_id TEXT NOT NULL, expected_revision BIGINT NOT NULL CHECK(expected_revision>=0),
  resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), response_payload JSONB NOT NULL,
  event_id UUID NOT NULL REFERENCES user_command_outbox(event_id) ON DELETE RESTRICT,
  actor_id UUID, reason TEXT NOT NULL, trace_id TEXT NOT NULL, compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_user_command_requests_user ON user_command_requests(tenant_id,user_id,created_at DESC);

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

-- =========================================================================================
-- 认证服务：API Token 管理（来源：auth_postgres_ddl.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- api_tokens: API 令牌表（主表）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS api_tokens (
  token_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
  
  name TEXT NOT NULL,
  description TEXT,
  token_type TEXT NOT NULL DEFAULT 'api',
  
  token_hash TEXT NOT NULL UNIQUE,
  token_prefix TEXT NOT NULL,
  
  scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
  
  status TEXT NOT NULL DEFAULT 'active',
  
  expires_at TIMESTAMPTZ,
  
  last_used_at TIMESTAMPTZ,
  usage_count BIGINT NOT NULL DEFAULT 0,
  
  created_by UUID REFERENCES users(user_id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  revoked_at TIMESTAMPTZ,
  
  rotation_enabled BOOLEAN NOT NULL DEFAULT false,
  rotation_interval INT,
  last_rotated_at TIMESTAMPTZ,
  previous_token_id UUID REFERENCES api_tokens(token_id) ON DELETE SET NULL,
  
  ip_whitelist JSONB DEFAULT '[]'::jsonb,
  
  metadata JSONB DEFAULT '{}'::jsonb,
  
  probe_id TEXT,
  
  CONSTRAINT api_tokens_tenant_name_unique UNIQUE (tenant_id, name),
  CONSTRAINT api_tokens_status_check CHECK (status IN ('active', 'revoked', 'expired')),
  CONSTRAINT api_tokens_type_check CHECK (token_type IN ('user', 'api', 'probe', 'service'))
);

CREATE INDEX IF NOT EXISTS idx_api_tokens_tenant_status 
  ON api_tokens(tenant_id, status) WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_api_tokens_tenant_type 
  ON api_tokens(tenant_id, token_type);

CREATE INDEX IF NOT EXISTS idx_api_tokens_probe 
  ON api_tokens(probe_id) WHERE probe_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_api_tokens_expires 
  ON api_tokens(expires_at) WHERE expires_at IS NOT NULL AND status = 'active';

CREATE INDEX IF NOT EXISTS idx_api_tokens_created_by 
  ON api_tokens(created_by) WHERE created_by IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_api_tokens_rotation 
  ON api_tokens(rotation_enabled, last_rotated_at) 
  WHERE rotation_enabled = TRUE AND status = 'active';

-- -----------------------------------------------------------------------------------------
-- replay_tasks: 回放任务
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS replay_tasks (
  task_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,      -- MinIO 中的数据集路径
  run_id TEXT NOT NULL UNIQUE,
  
  speed_mode TEXT NOT NULL,      -- original, multiplier, fixed_pps, fixed_mbps, top_speed
  speed_value FLOAT,
  
  status TEXT NOT NULL DEFAULT 'pending',
  
  packets_total BIGINT,
  packets_sent BIGINT DEFAULT 0,
  bytes_sent BIGINT DEFAULT 0,
  
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  error_message TEXT,
  
  created_by UUID REFERENCES users(user_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_replay_tasks_tenant_status ON replay_tasks(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_replay_tasks_run_id ON replay_tasks(run_id);


-- -----------------------------------------------------------------------------------------
-- datasets: 数据集版本库表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS datasets (
  dataset_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  
  file_path TEXT NOT NULL,       -- MinIO 路径
  file_size BIGINT NOT NULL,
  checksum_sha256 TEXT NOT NULL,
  
  packet_count BIGINT,
  tags JSONB DEFAULT '[]'::jsonb,
  
  uploaded_by UUID REFERENCES users(user_id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_datasets_tenant ON datasets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_datasets_tags ON datasets USING GIN(tags);

-- -----------------------------------------------------------------------------------------
-- token_rotation_history: Token 轮转历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS token_rotation_history (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  token_id UUID NOT NULL REFERENCES api_tokens(token_id) ON DELETE CASCADE,
  old_token_hash TEXT NOT NULL,
  new_token_hash TEXT NOT NULL,
  rotated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  rotated_by TEXT NOT NULL,
  reason TEXT NOT NULL,
  grace_period_ends TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_token_rotation_token_id 
  ON token_rotation_history(token_id, rotated_at DESC);

CREATE INDEX IF NOT EXISTS idx_token_rotation_grace_period 
  ON token_rotation_history(grace_period_ends);

-- -----------------------------------------------------------------------------------------
-- token_usage_logs: Token 使用日志表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS token_usage_logs (
  id BIGSERIAL PRIMARY KEY,
  token_id UUID NOT NULL REFERENCES api_tokens(token_id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ip_addr TEXT NOT NULL,
  user_agent TEXT,
  endpoint TEXT NOT NULL,
  method TEXT NOT NULL,
  status_code INT NOT NULL,
  response_time_ms INT
);

CREATE INDEX IF NOT EXISTS idx_token_usage_logs_token_id 
  ON token_usage_logs(token_id, used_at DESC);

CREATE INDEX IF NOT EXISTS idx_token_usage_logs_tenant_time 
  ON token_usage_logs(tenant_id, used_at DESC);

-- -----------------------------------------------------------------------------------------
-- revoked_sessions: Session 撤销表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS revoked_sessions (
  session_id TEXT PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL,
  revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_revoked_sessions_expires 
  ON revoked_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_revoked_sessions_user 
  ON revoked_sessions(user_id, revoked_at DESC);

-- -----------------------------------------------------------------------------------------
-- refresh_tokens: 刷新令牌表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS refresh_tokens (
  token_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  user_id        UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  token_hash     TEXT NOT NULL,
  expires_at     TIMESTAMPTZ NOT NULL,
  revoked_at     TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (token_hash)
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires ON refresh_tokens(expires_at);

-- -----------------------------------------------------------------------------------------
-- token_blacklist: 令牌黑名单
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS token_blacklist (
  jti            TEXT PRIMARY KEY,
  user_id        UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  expires_at     TIMESTAMPTZ NOT NULL,
  revoked_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires ON token_blacklist(expires_at);

-- =========================================================================================
-- 资产管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- asset_groups: 资产组
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS asset_groups (
  group_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  description    TEXT,
  selector       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_asset_groups_tenant ON asset_groups(tenant_id);

-- -----------------------------------------------------------------------------------------
-- assets: 资产表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS assets (
  asset_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  revision       BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  display_code   TEXT,
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  asset_type     TEXT NOT NULL DEFAULT 'unknown',
  status         TEXT NOT NULL DEFAULT 'active',
  ip             TEXT,
  ip_address     TEXT,
  mac_address    TEXT,
  hostname       TEXT,
  vendor         TEXT,
  os_type        TEXT,
  source         TEXT NOT NULL DEFAULT 'manual',
  vlan_id        TEXT,
  switch_port    TEXT,
  department     TEXT,
  campus         TEXT,
  owner          TEXT,
  tags           JSONB NOT NULL DEFAULT '{}'::jsonb,
  criticality    INT NOT NULL DEFAULT 0,
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  first_seen     TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen      TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS display_code TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS asset_type TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS mac_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS hostname TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vendor TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS os_type TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vlan_id TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS switch_port TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS department TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS campus TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS owner TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS criticality INT NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ALTER COLUMN ip DROP NOT NULL;
UPDATE assets SET ip_address = ip WHERE (ip_address IS NULL OR ip_address = '') AND ip IS NOT NULL;
UPDATE assets AS candidate
SET ip = candidate.ip_address
WHERE candidate.ip IS NULL
  AND candidate.ip_address IS NOT NULL
  AND (SELECT COUNT(*) FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip_address = candidate.ip_address) = 1
  AND NOT EXISTS (SELECT 1 FROM assets AS peer WHERE peer.tenant_id = candidate.tenant_id AND peer.ip = candidate.ip_address);
CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_display_code_unique ON assets(tenant_id, display_code) WHERE display_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assets_tenant_type_status ON assets(tenant_id, asset_type, status, last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_assets_cursor_v2 ON assets(tenant_id, last_seen DESC, asset_id DESC) INCLUDE(updated_at);
CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(tenant_id, ip_address);
CREATE INDEX IF NOT EXISTS idx_assets_tags ON assets USING GIN(tags);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_ip_unique ON assets(tenant_id, ip) WHERE ip IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_mac_unique ON assets(tenant_id, mac_address) WHERE mac_address IS NOT NULL;
CREATE TABLE IF NOT EXISTS asset_events (
  event_id SERIAL PRIMARY KEY, event_uuid UUID DEFAULT uuid_generate_v4(),
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, revision BIGINT DEFAULT 1,
  trace_id TEXT NOT NULL DEFAULT '', old_value JSONB, new_value JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS event_uuid UUID;
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS revision BIGINT;
ALTER TABLE asset_events ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
UPDATE asset_events event
SET event_uuid=COALESCE(event.event_uuid,uuid_generate_v5(uuid_ns_url(),'traffic.asset.history:'||event.tenant_id||':'||event.asset_id::text||':'||event.event_id::text)),
    revision=COALESCE(event.revision,asset.revision)
FROM assets asset
WHERE asset.asset_id=event.asset_id AND (event.event_uuid IS NULL OR event.revision IS NULL);
ALTER TABLE asset_events ALTER COLUMN event_uuid SET NOT NULL;
ALTER TABLE asset_events ALTER COLUMN event_uuid SET DEFAULT uuid_generate_v4();
ALTER TABLE asset_events ALTER COLUMN revision SET NOT NULL;
ALTER TABLE asset_events ALTER COLUMN revision SET DEFAULT 1;
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_events_event_uuid ON asset_events(event_uuid);
CREATE INDEX IF NOT EXISTS idx_asset_events_asset ON asset_events(asset_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_events_tenant_revision ON asset_events(tenant_id,asset_id,revision DESC);
CREATE TABLE IF NOT EXISTS asset_event_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version>0),
  partition_key TEXT NOT NULL CHECK (partition_key<>''), event_type TEXT NOT NULL, payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead','cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_event_outbox_aggregate ON asset_event_outbox(tenant_id,asset_id,aggregate_version);
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_ready ON asset_event_outbox(available_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_reclaim ON asset_event_outbox(locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS asset_upsert_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK (length(request_hash)=64), actor TEXT NOT NULL CHECK (actor<>''),
  expected_revision BIGINT NOT NULL CHECK (expected_revision>=0),
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT, created BOOLEAN NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0),
  event_id UUID NOT NULL REFERENCES asset_event_outbox(event_id) ON DELETE RESTRICT,
  outbox_id BIGINT NOT NULL REFERENCES asset_event_outbox(outbox_id) ON DELETE RESTRICT,
  trace_id TEXT NOT NULL CHECK (trace_id<>''), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_upsert_requests_asset ON asset_upsert_requests(tenant_id,asset_id,created_at DESC);
CREATE TABLE IF NOT EXISTS asset_inactive_requests (
  request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK (length(request_hash)=64), actor TEXT NOT NULL CHECK (actor<>''),
  action_id TEXT NOT NULL CHECK (action_id='asset-inactive-sweep'), reason TEXT NOT NULL CHECK (reason<>''),
  cutoff TIMESTAMPTZ NOT NULL, affected_count INTEGER NOT NULL CHECK (affected_count>=0),
  result_payload JSONB NOT NULL, trace_id TEXT NOT NULL CHECK (trace_id<>''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_inactive_requests_cutoff ON asset_inactive_requests(tenant_id,cutoff DESC);
CREATE TABLE IF NOT EXISTS asset_projection_inbox (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  schema_version INTEGER NOT NULL CHECK (schema_version=2),
  partition_key TEXT NOT NULL CHECK (partition_key<>''),
  trace_id TEXT NOT NULL DEFAULT '', payload JSONB NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  os_status TEXT NOT NULL DEFAULT 'pending' CHECK (os_status IN ('pending','applied','dead')),
  nebula_status TEXT NOT NULL DEFAULT 'pending' CHECK (nebula_status IN ('pending','applied','dead')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','applied','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at TIMESTAMPTZ, UNIQUE (tenant_id,asset_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_ready ON asset_projection_inbox(available_at,created_at) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_reclaim ON asset_projection_inbox(locked_until,created_at) WHERE status='processing';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_dead ON asset_projection_inbox(updated_at) WHERE status='dead';
CREATE TABLE IF NOT EXISTS asset_projection_watermarks (
  tenant_id TEXT NOT NULL,
  asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  target TEXT NOT NULL CHECK (target IN ('opensearch','nebulagraph')),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0),
  event_id UUID NOT NULL REFERENCES asset_projection_inbox(event_id) ON DELETE RESTRICT,
  projection_sha256 TEXT NOT NULL CHECK (length(projection_sha256)=64),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,asset_id,target)
);
CREATE INDEX IF NOT EXISTS idx_asset_projection_watermarks_target_version ON asset_projection_watermarks(target,aggregate_version);
CREATE TABLE IF NOT EXISTS asset_discovery_credentials (credential_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL, protocol TEXT NOT NULL, endpoint TEXT, secret_ref TEXT NOT NULL, created_by TEXT, revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name));
CREATE TABLE IF NOT EXISTS asset_discovery_runs (run_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, mode TEXT NOT NULL, target_cidr TEXT, target_network CIDR, credential_id TEXT, action_id TEXT NOT NULL DEFAULT 'asset-active-discovery-run', status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','cancel_requested','cancelled','succeeded','completed','partial','failed','blocked')), revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), requested_by TEXT, reason TEXT NOT NULL DEFAULT 'legacy-compatible', rate_limit_per_second INT NOT NULL DEFAULT 10 CHECK(rate_limit_per_second BETWEEN 1 AND 10000), security_window_start TIMESTAMPTZ, security_window_end TIMESTAMPTZ, approved_by TEXT NOT NULL DEFAULT '', idempotency_key TEXT, request_hash TEXT, trace_id TEXT NOT NULL DEFAULT '', cancel_requested BOOLEAN NOT NULL DEFAULT false, discovered_assets INT NOT NULL DEFAULT 0, discovered_links INT NOT NULL DEFAULT 0, discovered_candidates INT NOT NULL DEFAULT 0, rejected_records INT NOT NULL DEFAULT 0, result_watermark TEXT NOT NULL DEFAULT '', error_message TEXT, queued_at TIMESTAMPTZ NOT NULL DEFAULT now(), started_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ);
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_discovery_run_idempotency ON asset_discovery_runs(tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_ready ON asset_discovery_runs(queued_at,run_id) WHERE status='queued';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_reclaim ON asset_discovery_runs(locked_until,run_id) WHERE status='running';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_overlap ON asset_discovery_runs USING gist(target_network inet_ops) WHERE status IN ('queued','running','cancel_requested');
CREATE TABLE IF NOT EXISTS asset_discovery_run_history (transition_id BIGSERIAL PRIMARY KEY, run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, from_status TEXT NOT NULL, to_status TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(run_id,revision));
CREATE TABLE IF NOT EXISTS asset_discovery_control_requests (request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT, operation TEXT NOT NULL CHECK(operation IN ('cancel','merge_candidate')), candidate_id UUID, idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64), expected_revision BIGINT NOT NULL CHECK(expected_revision>0), resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), result_payload JSONB NOT NULL DEFAULT '{}'::jsonb, actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_asset_discovery_control_run ON asset_discovery_control_requests(tenant_id,run_id,created_at);
CREATE TABLE IF NOT EXISTS asset_discovery_candidates (candidate_id UUID PRIMARY KEY, run_id TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, fingerprint TEXT NOT NULL CHECK(length(fingerprint)=64), observation JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','merged','rejected')), revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), source_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT, decision_reason TEXT NOT NULL DEFAULT '', decided_by TEXT NOT NULL DEFAULT '', discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(), decided_at TIMESTAMPTZ, UNIQUE(tenant_id,run_id,fingerprint));
CREATE INDEX IF NOT EXISTS idx_asset_discovery_candidates_run ON asset_discovery_candidates(tenant_id,run_id,status,discovered_at,candidate_id);
CREATE TABLE IF NOT EXISTS asset_discovery_outbox (outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, run_id TEXT REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT, resource_type TEXT NOT NULL DEFAULT 'run' CHECK(resource_type IN ('run','credential','topology_link')), resource_id TEXT NOT NULL, action_id TEXT NOT NULL, tenant_id TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), schema_version INT NOT NULL DEFAULT 1 CHECK(schema_version>0), partition_key TEXT NOT NULL CHECK(partition_key<>''), event_type TEXT NOT NULL, payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead','cancelled')), attempt_count INT NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE(resource_type,resource_id,aggregate_version,event_type), CHECK((resource_type='run' AND run_id=resource_id) OR (resource_type<>'run' AND run_id IS NULL)));
CREATE INDEX IF NOT EXISTS idx_asset_discovery_outbox_ready ON asset_discovery_outbox(available_at,outbox_id) WHERE status='pending';
CREATE TABLE IF NOT EXISTS asset_discovery_resource_requests (request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, resource_type TEXT NOT NULL CHECK(resource_type IN ('credential','topology_link')), resource_id TEXT NOT NULL, action_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64), expected_revision BIGINT NOT NULL CHECK(expected_revision>=0), resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), result_payload JSONB NOT NULL, event_id UUID NOT NULL, outbox_id BIGINT NOT NULL REFERENCES asset_discovery_outbox(outbox_id) ON DELETE RESTRICT, actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key));
CREATE TABLE IF NOT EXISTS asset_discovery_resource_history (history_id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, resource_type TEXT NOT NULL CHECK(resource_type IN ('credential','topology_link')), resource_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), action_id TEXT NOT NULL, actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL, old_value JSONB NOT NULL DEFAULT '{}'::jsonb, new_value JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(resource_type,resource_id,revision));
CREATE TABLE IF NOT EXISTS asset_topology_links (link_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, run_id TEXT, source_asset_id TEXT, source_mac TEXT, source_ip TEXT, source_interface TEXT NOT NULL DEFAULT '', neighbor_asset_id TEXT, neighbor_mac TEXT NOT NULL DEFAULT '', neighbor_ip TEXT, neighbor_interface TEXT NOT NULL DEFAULT '', protocol TEXT NOT NULL, confidence INT NOT NULL DEFAULT 80, revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), observed_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,source_mac,neighbor_mac,protocol,source_interface,neighbor_interface));
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_tenant ON asset_topology_links(tenant_id,observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_asset ON asset_topology_links(tenant_id,source_asset_id,neighbor_asset_id);
CREATE TABLE IF NOT EXISTS asset_export_jobs (job_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT, action_id TEXT NOT NULL CHECK(action_id='asset-inventory-export'), format TEXT NOT NULL CHECK(format IN ('csv','jsonl')), status TEXT NOT NULL DEFAULT 'accepted' CHECK(status IN ('accepted','running','completed','failed','cancelled')), revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), columns JSONB NOT NULL, query JSONB NOT NULL, query_sha256 TEXT NOT NULL CHECK(length(query_sha256)=64), idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL, snapshot_id TEXT NOT NULL DEFAULT '', as_of TIMESTAMPTZ, source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb, row_count INTEGER NOT NULL DEFAULT 0 CHECK(row_count>=0), object_bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '', mime_type TEXT NOT NULL DEFAULT '', artifact_sha256 TEXT NOT NULL DEFAULT '', size_bytes BIGINT NOT NULL DEFAULT 0 CHECK(size_bytes>=0), retention_until TIMESTAMPTZ, error_message TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL, trace_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, UNIQUE(tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_tenant ON asset_export_jobs(tenant_id,created_at DESC,job_id);
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_ready ON asset_export_jobs(next_attempt_at,created_at,job_id) WHERE status IN ('accepted','running');
CREATE TABLE IF NOT EXISTS asset_export_outbox (outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, job_id UUID NOT NULL REFERENCES asset_export_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT, event_type TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version>0), partition_key TEXT NOT NULL CHECK(partition_key<>''), payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead','cancelled')), attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts>=0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE(job_id,aggregate_version));
CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_ready ON asset_export_outbox(next_attempt_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_reclaim ON asset_export_outbox(locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS asset_column_preferences (tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, user_id TEXT NOT NULL, view_id TEXT NOT NULL, columns JSONB NOT NULL, revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), updated_by TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,user_id,view_id));
CREATE INDEX IF NOT EXISTS idx_asset_column_preferences_updated ON asset_column_preferences(tenant_id,updated_at DESC);
ALTER TABLE assets ADD COLUMN IF NOT EXISTS lifecycle_state TEXT NOT NULL DEFAULT 'managed';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS merged_into_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_lifecycle_state;
ALTER TABLE assets ADD CONSTRAINT chk_assets_lifecycle_state CHECK(lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged'));
ALTER TABLE assets DROP CONSTRAINT IF EXISTS chk_assets_merge_target;
ALTER TABLE assets ADD CONSTRAINT chk_assets_merge_target CHECK((lifecycle_state='merged' AND merged_into_asset_id IS NOT NULL AND merged_into_asset_id<>asset_id) OR (lifecycle_state<>'merged' AND merged_into_asset_id IS NULL));
CREATE TABLE IF NOT EXISTS asset_governance_work_orders (work_order_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, asset_id UUID NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT, action_id TEXT NOT NULL CHECK(action_id='asset-governance-work-order-create'), source_lifecycle_state TEXT NOT NULL, target_lifecycle_state TEXT NOT NULL CHECK(target_lifecycle_state IN ('candidate','confirmed','managed','isolated','retired','merged')), target_asset_id UUID REFERENCES assets(asset_id) ON DELETE RESTRICT, status TEXT NOT NULL DEFAULT 'pending_approval' CHECK(status IN ('pending_approval','approved','rejected','executing','completed','failed','cancelled','compensated')), revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), expected_asset_revision BIGINT NOT NULL CHECK(expected_asset_revision>0), resulting_asset_revision BIGINT, owner TEXT NOT NULL CHECK(owner<>''), requested_by TEXT NOT NULL CHECK(requested_by<>''), approved_by TEXT NOT NULL DEFAULT '', due_at TIMESTAMPTZ NOT NULL, evidence_required BOOLEAN NOT NULL DEFAULT true, evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(evidence_refs)='array'), reason TEXT NOT NULL CHECK(length(reason) BETWEEN 8 AND 2000), external_system TEXT NOT NULL DEFAULT 'internal', external_ticket_id TEXT NOT NULL DEFAULT '', external_status TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64), trace_id TEXT NOT NULL CHECK(trace_id<>''), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, UNIQUE(tenant_id,idempotency_key), CHECK((target_lifecycle_state='merged' AND target_asset_id IS NOT NULL AND target_asset_id<>asset_id) OR (target_lifecycle_state<>'merged' AND target_asset_id IS NULL)));
CREATE INDEX IF NOT EXISTS idx_asset_governance_orders_asset ON asset_governance_work_orders(tenant_id,asset_id,created_at DESC);
DROP INDEX IF EXISTS uq_asset_governance_active_target;
CREATE UNIQUE INDEX uq_asset_governance_active_target ON asset_governance_work_orders(tenant_id,asset_id) WHERE status IN ('pending_approval','approved','executing');
CREATE TABLE IF NOT EXISTS asset_governance_work_order_history (history_id BIGSERIAL PRIMARY KEY, work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0), action_id TEXT NOT NULL, from_status TEXT NOT NULL, to_status TEXT NOT NULL, from_lifecycle_state TEXT NOT NULL, to_lifecycle_state TEXT NOT NULL, actor TEXT NOT NULL CHECK(actor<>''), reason TEXT NOT NULL, evidence_refs JSONB NOT NULL DEFAULT '[]'::jsonb, trace_id TEXT NOT NULL CHECK(trace_id<>''), detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(work_order_id,revision));
CREATE TABLE IF NOT EXISTS asset_governance_control_requests (request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT, idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64), action_id TEXT NOT NULL, actor TEXT NOT NULL CHECK(actor<>''), expected_revision BIGINT NOT NULL CHECK(expected_revision>0), resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), trace_id TEXT NOT NULL CHECK(trace_id<>''), result JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key));
CREATE TABLE IF NOT EXISTS asset_governance_outbox (outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL, work_order_id UUID NOT NULL REFERENCES asset_governance_work_orders(work_order_id) ON DELETE RESTRICT, aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version>0), partition_key TEXT NOT NULL CHECK(partition_key<>''), event_type TEXT NOT NULL, delivery_target TEXT NOT NULL DEFAULT 'internal' CHECK(delivery_target IN ('internal','external')), payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','delivered','dead','cancelled')), attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), delivered_at TIMESTAMPTZ, UNIQUE(work_order_id,aggregate_version));
CREATE INDEX IF NOT EXISTS idx_asset_governance_outbox_ready ON asset_governance_outbox(available_at,outbox_id) WHERE status='pending';

-- =========================================================================================
-- 特征工程注册表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- feature_sets: 特征集定义
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS feature_sets (
  feature_set_id TEXT PRIMARY KEY,
  tenant_id      TEXT REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  description    TEXT,
  params         JSONB NOT NULL DEFAULT '{}'::jsonb,
  schema_version TEXT NOT NULL DEFAULT 'v1',
  status         TEXT NOT NULL DEFAULT 'active',
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feature_sets_tenant ON feature_sets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_feature_sets_status ON feature_sets(status);

-- =========================================================================================
-- 规则与版本管理（来源：rules_postgres.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- rules: 规则定义（扩展版）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rules (
  rule_id      TEXT PRIMARY KEY,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  rule_type    TEXT NOT NULL DEFAULT 'threshold',
  engine       TEXT NOT NULL DEFAULT 'internal',
  description  TEXT,
  conditions   JSONB NOT NULL DEFAULT '{}'::jsonb,
  labels       TEXT[] NOT NULL DEFAULT '{}',
  severity     TEXT NOT NULL DEFAULT 'medium',
  enabled      BOOLEAN NOT NULL DEFAULT false,
  priority     INT NOT NULL DEFAULT 50,
  version      BIGINT NOT NULL DEFAULT 1,
  status       TEXT NOT NULL DEFAULT 'draft',
  created_by   TEXT NOT NULL DEFAULT 'system',
  updated_by   TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT rules_tenant_name_unique UNIQUE (tenant_id, name),
  CONSTRAINT rules_severity_check CHECK (severity IN ('low', 'medium', 'high', 'critical')),
  CONSTRAINT rules_priority_check CHECK (priority >= 0 AND priority <= 100),
  CONSTRAINT rules_status_check CHECK (status IN ('draft', 'active', 'disabled', 'archived', 'deleted', 'pending_sync'))
);

CREATE INDEX IF NOT EXISTS idx_rules_tenant_id ON rules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled) WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_rules_status ON rules(status) WHERE status != 'deleted';
CREATE INDEX IF NOT EXISTS idx_rules_type ON rules(rule_type);
CREATE INDEX IF NOT EXISTS idx_rules_labels ON rules USING GIN(labels);
CREATE INDEX IF NOT EXISTS idx_rules_version ON rules(version);
CREATE INDEX IF NOT EXISTS idx_rules_pending_sync ON rules(status) WHERE status = 'pending_sync';

-- -----------------------------------------------------------------------------------------
-- rule_versions: 规则版本
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rule_versions (
  rule_version TEXT PRIMARY KEY,
  rule_id       TEXT NOT NULL,
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  version       BIGINT NOT NULL,
  content_uri   TEXT NOT NULL,
  checksum      TEXT,
  status        TEXT NOT NULL DEFAULT 'active',
  change_log    TEXT,
  metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by    TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rule_versions_rule_id ON rule_versions(rule_id);
CREATE INDEX IF NOT EXISTS idx_rule_versions_version ON rule_versions(version DESC);
CREATE INDEX IF NOT EXISTS idx_rule_versions_status ON rule_versions(status);

-- -----------------------------------------------------------------------------------------
-- rule_outbox: Outbox 模式表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rule_outbox (
  id           BIGSERIAL PRIMARY KEY,
  rule_id      TEXT NOT NULL,
  event_type   TEXT NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  published    BOOLEAN NOT NULL DEFAULT false,
  published_at TIMESTAMPTZ,
  retry_count  INT NOT NULL DEFAULT 0,
  last_error   TEXT,
  next_retry   TIMESTAMPTZ,
  CONSTRAINT rule_outbox_event_type_check CHECK (event_type IN ('create', 'update', 'delete', 'enable', 'disable', 'sync'))
);

CREATE INDEX IF NOT EXISTS idx_rule_outbox_published 
  ON rule_outbox(published, next_retry) WHERE published = false;

CREATE INDEX IF NOT EXISTS idx_rule_outbox_rule_id ON rule_outbox(rule_id);
CREATE INDEX IF NOT EXISTS idx_rule_outbox_created_at ON rule_outbox(created_at DESC);

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
CREATE INDEX IF NOT EXISTS idx_rule_workbench_lookup ON rule_workbench_items (tenant_id, rule_id, category, ordinal);

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
CREATE INDEX IF NOT EXISTS idx_rule_action_jobs_lookup ON rule_action_jobs (tenant_id, rule_id, created_at DESC);

-- =========================================================================================
-- 模型与版本管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- models: 模型定义
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS models (
  model_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  model_type     TEXT NOT NULL,
  description    TEXT,
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_models_tenant ON models(tenant_id);

-- -----------------------------------------------------------------------------------------
-- model_versions: 模型版本
-- -----------------------------------------------------------------------------------------
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

-- =========================================================================================
-- 部署管理（来源：rules_postgres.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- deployments: 部署配置
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS deployments (
  deployment_id    TEXT PRIMARY KEY,
  tenant_id        TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name             TEXT,
  description      TEXT,
  model_version    TEXT REFERENCES model_versions(model_version),
  rule_version     TEXT REFERENCES rule_versions(rule_version),
  feature_set_id   TEXT REFERENCES feature_sets(feature_set_id),
  scope            JSONB NOT NULL DEFAULT '{}'::jsonb,
  status           TEXT NOT NULL DEFAULT 'planned',
  -- 回滚信息
  rollback_from UUID,
  rollback_reason TEXT,
  created_by       TEXT NOT NULL DEFAULT 'system',
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  gray_started_at  TIMESTAMPTZ,
  gray_expired_at  TIMESTAMPTZ,
  activated_at     TIMESTAMPTZ,
  rolled_back_at   TIMESTAMPTZ,
  -- 元数据
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  CONSTRAINT deployments_status_check CHECK (status IN ('planned', 'gray', 'active', 'paused', 'rolled_back', 'failed', 'cancelled', 'superseded'))
);

CREATE INDEX IF NOT EXISTS idx_deployments_tenant ON deployments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_deployments_tenant_id ON deployments(tenant_id);


CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_model_version ON deployments(model_version);
CREATE INDEX IF NOT EXISTS idx_deployments_rule_version ON deployments(rule_version);
CREATE INDEX IF NOT EXISTS idx_deployments_active ON deployments(status) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_deploy_tenant_time ON deployments (tenant_id, created_at DESC);

-- -----------------------------------------------------------------------------------------
-- deployment_history: 部署历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS deployment_history (
  id            BIGSERIAL PRIMARY KEY,
  deployment_id TEXT NOT NULL,
  action        TEXT NOT NULL,
  operator_id   TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail        JSONB
);

CREATE INDEX IF NOT EXISTS idx_deployment_history_deployment_id 
  ON deployment_history(deployment_id);

CREATE INDEX IF NOT EXISTS idx_deployment_history_created_at 
  ON deployment_history(created_at DESC);

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
  event_id UUID PRIMARY KEY, deployment_id TEXT NOT NULL, tenant_id TEXT NOT NULL,
  action TEXT NOT NULL, status TEXT NOT NULL, operator_id TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0), payload JSONB NOT NULL,
  kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_deployment_event_projection_tenant_deployment ON deployment_event_projection (tenant_id,deployment_id,event_timestamp_ms);
CREATE TABLE IF NOT EXISTS deployment_state_projection (
  tenant_id TEXT NOT NULL, deployment_id TEXT NOT NULL, action TEXT NOT NULL,
  status TEXT NOT NULL, operator_id TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0),
  last_event_id UUID NOT NULL, payload JSONB NOT NULL, kafka_partition INTEGER NOT NULL,
  kafka_offset BIGINT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,deployment_id)
);
CREATE INDEX IF NOT EXISTS idx_deployment_state_projection_tenant_status ON deployment_state_projection (tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alert_feedback_event_projection (
  event_id UUID PRIMARY KEY, feedback_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, user_id TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL CHECK (label IN ('TP','FP')), reason_code TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0), payload JSONB NOT NULL,
  kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_event_tenant_alert ON alert_feedback_event_projection (tenant_id,alert_id,event_timestamp_ms);
CREATE TABLE IF NOT EXISTS model_feedback_inbox (
  feedback_id UUID PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, user_id TEXT NOT NULL DEFAULT '',
  label TEXT NOT NULL CHECK (label IN ('TP','FP')), reason_code TEXT NOT NULL DEFAULT '',
  model_version TEXT NOT NULL DEFAULT '', rule_version TEXT NOT NULL DEFAULT '',
  event_timestamp_ms BIGINT NOT NULL CHECK (event_timestamp_ms > 0), payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','applied','failed','dead_letter')),
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), last_error TEXT NOT NULL DEFAULT '',
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', applied_at TIMESTAMPTZ,
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

-- =========================================================================================
-- 任务管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- tasks: 任务表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tasks (
  task_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL DEFAULT '',
  task_type      TEXT NOT NULL,
  params         JSONB NOT NULL DEFAULT '{}'::jsonb,
  status         TEXT NOT NULL DEFAULT 'queued',
  progress       INT NOT NULL DEFAULT 0,
  result_file_key TEXT NOT NULL DEFAULT '',
  result_sha256   TEXT NOT NULL DEFAULT '',
  result_packets  BIGINT NOT NULL DEFAULT 0,
  result_bytes    BIGINT NOT NULL DEFAULT 0,
  files_scanned   INT NOT NULL DEFAULT 0,
  error_message   TEXT NOT NULL DEFAULT '',
  run_id         TEXT NOT NULL DEFAULT '',
  created_by     TEXT NOT NULL DEFAULT '',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at     TIMESTAMPTZ,
  completed_at   TIMESTAMPTZ
);

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_file_key TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_packets BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS files_scanned INT NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS error_message TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS run_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
DO $$
DECLARE constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT conname FROM pg_constraint
    WHERE conrelid='tasks'::regclass AND contype='f'
      AND pg_get_constraintdef(oid) LIKE '%created_by%'
  LOOP
    EXECUTE format('ALTER TABLE tasks DROP CONSTRAINT %I', constraint_name);
  END LOOP;
END $$;
ALTER TABLE tasks ALTER COLUMN result_file_key SET DEFAULT '';
UPDATE tasks SET result_file_key='' WHERE result_file_key IS NULL;
ALTER TABLE tasks ALTER COLUMN result_file_key SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN result_packets SET DEFAULT 0;
UPDATE tasks SET result_packets=0 WHERE result_packets IS NULL;
ALTER TABLE tasks ALTER COLUMN result_packets SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN result_bytes SET DEFAULT 0;
UPDATE tasks SET result_bytes=0 WHERE result_bytes IS NULL;
ALTER TABLE tasks ALTER COLUMN result_bytes SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN files_scanned SET DEFAULT 0;
UPDATE tasks SET files_scanned=0 WHERE files_scanned IS NULL;
ALTER TABLE tasks ALTER COLUMN files_scanned SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN error_message SET DEFAULT '';
UPDATE tasks SET error_message='' WHERE error_message IS NULL;
ALTER TABLE tasks ALTER COLUMN error_message SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET DEFAULT '';
UPDATE tasks SET run_id='' WHERE run_id IS NULL;
ALTER TABLE tasks ALTER COLUMN run_id SET NOT NULL;
ALTER TABLE tasks ALTER COLUMN created_by TYPE TEXT USING created_by::text;
ALTER TABLE tasks ALTER COLUMN created_by SET DEFAULT '';
UPDATE tasks SET created_by='' WHERE created_by IS NULL;
ALTER TABLE tasks ALTER COLUMN created_by SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_tenant_time ON tasks(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status) WHERE status IN ('queued', 'processing');
CREATE INDEX IF NOT EXISTS idx_tasks_type ON tasks(task_type);
CREATE INDEX IF NOT EXISTS idx_tasks_run_id ON tasks(run_id) WHERE run_id IS NOT NULL;

-- =========================================================================================
-- 探针管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- probes: 探针注册表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS probes (
  probe_id       TEXT PRIMARY KEY,
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name           TEXT NOT NULL,
  status         TEXT NOT NULL DEFAULT 'active',
  location       TEXT,
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  hardware_info  JSONB,
  software_version TEXT,
  last_heartbeat TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_probes_tenant ON probes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_probes_status ON probes(status);

ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;

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
  tenant_id TEXT NOT NULL, state_revision BIGINT NOT NULL CHECK (state_revision > 0), from_status TEXT NOT NULL,
  to_status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (operation_id,state_revision)
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
  UNIQUE (operation_id,event_type)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_outbox_pending ON probe_operation_outbox (next_attempt_at,created_at) WHERE published=false;

-- =========================================================================================
-- 审计日志（合并所有字段）
-- =========================================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT NOT NULL DEFAULT ('audit-' || uuid_generate_v4()::text),
    tenant_id TEXT NOT NULL,
    user_id TEXT,
    
    -- 操作信息
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT,
    
    -- 详细信息
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    
    -- 请求上下文
    ip_addr TEXT,
    user_agent TEXT,
    request_id TEXT,
    
    -- 结果
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS event_id TEXT;
UPDATE audit_logs SET event_id = 'audit-' || id::TEXT WHERE event_id IS NULL OR event_id = '';
ALTER TABLE audit_logs ALTER COLUMN event_id SET DEFAULT ('audit-' || uuid_generate_v4()::text);
ALTER TABLE audit_logs ALTER COLUMN event_id SET NOT NULL;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS request_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS trace_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS success BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS error_message TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS risk_level TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS result TEXT;
UPDATE audit_logs SET request_id=detail->>'request_id' WHERE COALESCE(request_id,'')='' AND detail ? 'request_id';
UPDATE audit_logs SET trace_id=detail->>'trace_id' WHERE COALESCE(trace_id,'')='' AND detail ? 'trace_id';
UPDATE audit_logs SET result=COALESCE(NULLIF(detail->>'result',''), CASE WHEN success THEN 'success' ELSE 'failure' END) WHERE COALESCE(result,'')='';
UPDATE audit_logs SET risk_level=COALESCE(NULLIF(detail->>'risk',''),NULLIF(detail->>'risk_level',''),'low') WHERE COALESCE(risk_level,'')='';
UPDATE audit_logs SET success=false WHERE lower(result) IN ('failure','failed','error','denied');
CREATE INDEX IF NOT EXISTS idx_audit_tenant_request ON audit_logs (tenant_id, request_id) WHERE request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_tenant_trace ON audit_logs (tenant_id, trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_tenant_result_risk_time ON audit_logs (tenant_id, result, risk_level, created_at DESC);
CREATE TABLE IF NOT EXISTS audit_saved_queries (saved_query_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id, name));
CREATE INDEX IF NOT EXISTS idx_audit_saved_queries_tenant_time ON audit_saved_queries (tenant_id, created_at DESC);
CREATE TABLE IF NOT EXISTS audit_exports (export_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, format TEXT NOT NULL CHECK (format IN ('pdf','csv','json')), filters JSONB NOT NULL DEFAULT '{}'::jsonb, row_count INTEGER NOT NULL DEFAULT 0, total_matching INTEGER NOT NULL DEFAULT 0, truncated BOOLEAN NOT NULL DEFAULT false, mask_sensitive BOOLEAN NOT NULL DEFAULT true, filename TEXT NOT NULL, mime_type TEXT NOT NULL, sha256 TEXT NOT NULL, size_bytes BIGINT NOT NULL, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS total_matching INTEGER NOT NULL DEFAULT 0;
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS truncated BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE audit_exports ADD COLUMN IF NOT EXISTS mask_sensitive BOOLEAN NOT NULL DEFAULT true;
CREATE INDEX IF NOT EXISTS idx_audit_exports_tenant_time ON audit_exports (tenant_id, created_at DESC);
CREATE TABLE IF NOT EXISTS audit_reviews (review_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('pending','approved','rejected','escalated')), comment TEXT NOT NULL DEFAULT '', risk_level TEXT NOT NULL CHECK (risk_level IN ('low','medium','high','critical')), reviewed_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS idx_audit_reviews_tenant_log_time ON audit_reviews (tenant_id, audit_log_id, created_at DESC);
CREATE TABLE IF NOT EXISTS audit_integrity_checks (check_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, time_start TIMESTAMPTZ NOT NULL, time_end TIMESTAMPTZ NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, row_count BIGINT NOT NULL, root_sha256 TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('passed','failed','baseline_created','no_records')), matched_count BIGINT NOT NULL DEFAULT 0, baselined_count BIGINT NOT NULL DEFAULT 0, mismatched_count BIGINT NOT NULL DEFAULT 0, added_count BIGINT NOT NULL DEFAULT 0, missing_count BIGINT NOT NULL DEFAULT 0, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS matched_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS baselined_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS mismatched_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS filters JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS added_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks ADD COLUMN IF NOT EXISTS missing_count BIGINT NOT NULL DEFAULT 0;
ALTER TABLE audit_integrity_checks DROP CONSTRAINT IF EXISTS audit_integrity_checks_status_check;
ALTER TABLE audit_integrity_checks ADD CONSTRAINT audit_integrity_checks_status_check CHECK (status IN ('passed','failed','baseline_created','no_records'));
CREATE INDEX IF NOT EXISTS idx_audit_integrity_checks_tenant_time ON audit_integrity_checks (tenant_id, created_at DESC);
CREATE TABLE IF NOT EXISTS audit_log_integrity_baselines (tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL, root_sha256 TEXT NOT NULL, established_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_checked_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, audit_log_id));
CREATE INDEX IF NOT EXISTS idx_audit_log_integrity_baselines_checked ON audit_log_integrity_baselines (tenant_id, last_checked_at DESC);
CREATE TABLE IF NOT EXISTS audit_integrity_manifest_entries (check_id UUID NOT NULL REFERENCES audit_integrity_checks(check_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, audit_log_id TEXT NOT NULL, root_sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (check_id, audit_log_id));
CREATE INDEX IF NOT EXISTS idx_audit_integrity_manifest_tenant_log ON audit_integrity_manifest_entries (tenant_id, audit_log_id);
CREATE TABLE IF NOT EXISTS alert_playbook_executions (
    execution_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL, alert_id TEXT NOT NULL,
    success_actions INTEGER NOT NULL DEFAULT 0, failed_actions INTEGER NOT NULL DEFAULT 0, duration_ms BIGINT NOT NULL DEFAULT 0,
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb, result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    mode TEXT NOT NULL DEFAULT 'legacy', status TEXT NOT NULL DEFAULT 'succeeded', rollback_of TEXT,
    effect_payload JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', rolled_back_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS mode TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'succeeded';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rollback_of TEXT;
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS effect_payload JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS requested_by TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_playbook_executions ADD COLUMN IF NOT EXISTS rolled_back_at TIMESTAMPTZ;
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_mode_check CHECK (mode IN ('legacy', 'drill')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_status_check CHECK (status IN ('succeeded', 'failed', 'rolled_back', 'rollback_recorded')); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_id ON alert_playbook_executions (tenant_id, execution_id);
DO $do$ BEGIN ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_rollback_fk FOREIGN KEY (tenant_id, rollback_of) REFERENCES alert_playbook_executions (tenant_id, execution_id); EXCEPTION WHEN duplicate_object THEN NULL; END $do$;
CREATE INDEX IF NOT EXISTS idx_alert_playbook_executions_tenant_created ON alert_playbook_executions (tenant_id, created_at DESC);

-- F-PLAYBOOK-001 additive live execution workflow; legacy/drill rows remain valid.
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_mode_check;
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_status_check;
ALTER TABLE alert_playbook_executions
  ADD COLUMN IF NOT EXISTS playbook_version INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS workflow_revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required',
  ADD COLUMN IF NOT EXISTS executor_status TEXT NOT NULL DEFAULT 'simulated',
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS request_sha256 TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS execution_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_mode_check CHECK (mode IN ('legacy','drill','live'));
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_status_check CHECK (status IN ('succeeded','rolled_back','rollback_recorded','pending_approval','approved_awaiting_executor','running','completed','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed'));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_workflow_revision_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_workflow_revision_check CHECK (workflow_revision>0);
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_approval_status_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_approval_status_check CHECK (approval_status IN ('not_required','pending','approved','rejected','cancelled'));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_executor_status_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_executor_status_check CHECK (executor_status IN ('simulated','not_dispatched','not_configured','queued','running','succeeded','partial','failed','cancelled','compensating','compensated','compensation_failed'));
ALTER TABLE alert_playbook_executions DROP CONSTRAINT IF EXISTS alert_playbook_execution_attempts_check;
ALTER TABLE alert_playbook_executions ADD CONSTRAINT alert_playbook_execution_attempts_check CHECK (attempts>=0);
CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_playbook_execution_idempotency ON alert_playbook_executions(tenant_id,idempotency_key) WHERE idempotency_key<>'';
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_dispatch ON alert_playbook_executions(next_attempt_at,created_at,execution_id) WHERE mode='live' AND status IN ('approved_awaiting_executor','compensation_queued');
CREATE TABLE IF NOT EXISTS alert_playbook_execution_approvals (
  approval_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approve','reject')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  decided_by TEXT NOT NULL CHECK (decided_by<>''), resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_approvals_job ON alert_playbook_execution_approvals(tenant_id,execution_id,created_at DESC);
CREATE TABLE IF NOT EXISTS alert_playbook_execution_controls (
  request_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  requested_by TEXT NOT NULL CHECK (requested_by<>''), resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_controls_job ON alert_playbook_execution_controls(tenant_id,execution_id,created_at DESC);
CREATE TABLE IF NOT EXISTS alert_playbook_step_receipts (
  receipt_id UUID PRIMARY KEY, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  phase TEXT NOT NULL CHECK (phase IN ('execute','compensate')), attempt INTEGER NOT NULL CHECK (attempt>0), step_index INTEGER NOT NULL CHECK (step_index>=0),
  action_type TEXT NOT NULL, provider TEXT NOT NULL, provider_receipt_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')), external_effect BOOLEAN NOT NULL DEFAULT false,
  payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (execution_id,phase,attempt,step_index), UNIQUE (tenant_id,provider,provider_receipt_id,phase),
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_step_receipts_job ON alert_playbook_step_receipts(tenant_id,execution_id,created_at DESC);
CREATE TABLE IF NOT EXISTS alert_playbook_execution_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, execution_id TEXT NOT NULL, tenant_id TEXT NOT NULL,
  playbook_name TEXT NOT NULL, event_type TEXT NOT NULL, schema_version INTEGER NOT NULL DEFAULT 2,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version>0), partition_key TEXT NOT NULL, payload JSONB NOT NULL,
  published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,execution_id) REFERENCES alert_playbook_executions(tenant_id,execution_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_dispatch ON alert_playbook_execution_outbox(next_attempt_at,outbox_id) WHERE published=false;
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE alert_playbook_execution_outbox ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
UPDATE alert_playbook_execution_outbox SET status=CASE WHEN published THEN 'published' ELSE 'pending' END WHERE status NOT IN ('pending','processing','published','dead') OR (published AND status<>'published');
ALTER TABLE alert_playbook_execution_outbox DROP CONSTRAINT IF EXISTS alert_playbook_execution_outbox_status_check;
ALTER TABLE alert_playbook_execution_outbox ADD CONSTRAINT alert_playbook_execution_outbox_status_check CHECK(status IN ('pending','processing','published','dead'));
CREATE INDEX IF NOT EXISTS idx_alert_playbook_execution_outbox_ready ON alert_playbook_execution_outbox(next_attempt_at,created_at,outbox_id) WHERE published=false AND status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS alert_playbook_execution_event_projection (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, execution_id TEXT NOT NULL, playbook_name TEXT NOT NULL,
  event_type TEXT NOT NULL, schema_version INTEGER NOT NULL CHECK(schema_version=2), aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0),
  partition_key TEXT NOT NULL CHECK(partition_key<>''), trace_id TEXT NOT NULL CHECK(trace_id<>''), payload JSONB NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK(length(payload_sha256)=64), kafka_topic TEXT NOT NULL CHECK(kafka_topic<>''),
  kafka_partition INTEGER NOT NULL CHECK(kafka_partition>=0), kafka_offset BIGINT NOT NULL CHECK(kafka_offset>=0),
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(kafka_topic,kafka_partition,kafka_offset), UNIQUE(tenant_id,execution_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_playbook_event_projection_execution ON alert_playbook_execution_event_projection(tenant_id,execution_id,aggregate_version);
CREATE TABLE IF NOT EXISTS alert_playbook_execution_state_projection (
  tenant_id TEXT NOT NULL, execution_id TEXT NOT NULL, playbook_name TEXT NOT NULL, playbook_version INTEGER NOT NULL DEFAULT 0 CHECK(playbook_version>=0),
  alert_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL, approval_status TEXT NOT NULL DEFAULT '', executor_status TEXT NOT NULL DEFAULT '',
  aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0), event_type TEXT NOT NULL, trace_id TEXT NOT NULL CHECK(trace_id<>''),
  last_event_id UUID NOT NULL, payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL CHECK(length(payload_sha256)=64),
  kafka_topic TEXT NOT NULL CHECK(kafka_topic<>''), kafka_partition INTEGER NOT NULL CHECK(kafka_partition>=0),
  kafka_offset BIGINT NOT NULL CHECK(kafka_offset>=0), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(tenant_id,execution_id)
);
CREATE INDEX IF NOT EXISTS idx_playbook_state_projection_status ON alert_playbook_execution_state_projection(tenant_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS alert_saved_views (view_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL, name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name));
CREATE TABLE IF NOT EXISTS alert_response_actions (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL, dry_run BOOLEAN NOT NULL DEFAULT true, status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS alert_response_outbox (outbox_id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE CASCADE, tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ);
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_retry ON alert_response_outbox (next_attempt_at, outbox_id) WHERE published=false;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS result JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS error TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS schema_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS partition_key TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS alert_response_execution_receipts (event_id UUID PRIMARY KEY, job_id TEXT NOT NULL UNIQUE REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action_id TEXT NOT NULL, state TEXT NOT NULL CHECK (state IN ('simulated_completed','blocked_external_executor','failed')), simulated BOOLEAN NOT NULL, external_effect BOOLEAN NOT NULL DEFAULT false, result JSONB NOT NULL DEFAULT '{}'::jsonb, error TEXT NOT NULL DEFAULT '', kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (kafka_partition,kafka_offset));
ALTER TABLE alert_response_actions ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE alert_response_execution_receipts ADD COLUMN IF NOT EXISTS aggregate_version BIGINT NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_delivery ON alert_response_outbox(next_attempt_at,outbox_id) WHERE published=false AND cancelled_at IS NULL;
CREATE TABLE IF NOT EXISTS alert_response_approvals (approval_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('approve','reject')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), decided_by TEXT NOT NULL CHECK (decided_by<>''), resulting_revision BIGINT NOT NULL, resulting_status TEXT NOT NULL, approval_status TEXT NOT NULL CHECK (approval_status IN ('approved','rejected')), created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_alert_response_approvals_job ON alert_response_approvals(tenant_id,job_id,created_at);
CREATE TABLE IF NOT EXISTS alert_response_control_requests (request_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), requested_by TEXT NOT NULL CHECK (requested_by<>''), state TEXT NOT NULL CHECK (state IN ('cancelled','blocked_external_executor')), resulting_revision BIGINT NOT NULL, resulting_status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_alert_response_control_job ON alert_response_control_requests(tenant_id,job_id,created_at);
CREATE INDEX IF NOT EXISTS idx_alert_response_receipts_tenant_job ON alert_response_execution_receipts(tenant_id,job_id,created_at);
CREATE TABLE IF NOT EXISTS campaign_action_jobs (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, action_id TEXT NOT NULL, target TEXT NOT NULL, metadata JSONB NOT NULL DEFAULT '{}'::jsonb, simulation BOOLEAN NOT NULL DEFAULT true, dry_run BOOLEAN NOT NULL DEFAULT true, status TEXT NOT NULL, result JSONB NOT NULL DEFAULT '{}'::jsonb, error_message TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, idempotency_key TEXT NOT NULL DEFAULT '', request_sha256 TEXT NOT NULL DEFAULT '', expected_revision BIGINT NOT NULL DEFAULT 0, resource_revision BIGINT NOT NULL DEFAULT 0, reason TEXT NOT NULL DEFAULT '', CONSTRAINT campaign_action_jobs_status_check CHECK (status IN ('queued','accepted','pending_approval','approved_awaiting_executor','running','completed','succeeded','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed')));
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_tenant_time ON campaign_action_jobs(tenant_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_action_jobs_campaign_time ON campaign_action_jobs(tenant_id,campaign_id,created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_idempotency ON campaign_action_jobs(tenant_id,idempotency_key) WHERE idempotency_key<>'';
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_action_jobs_tenant_job ON campaign_action_jobs(tenant_id,job_id);
CREATE TABLE IF NOT EXISTS campaign_soar_jobs (job_id TEXT PRIMARY KEY REFERENCES campaign_action_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, playbook_id TEXT NOT NULL, target TEXT NOT NULL, source_snapshot_id TEXT NOT NULL, campaign_revision BIGINT NOT NULL CHECK (campaign_revision>0), status TEXT NOT NULL CHECK (status IN ('pending_approval','approved_awaiting_executor','running','completed','partial','failed','cancelled','compensation_queued','compensating','compensated','compensation_failed')), approval_status TEXT NOT NULL CHECK (approval_status IN ('pending','approved','rejected','cancelled')), executor_status TEXT NOT NULL CHECK (executor_status IN ('not_dispatched','not_configured','queued','running','succeeded','partial','failed','cancelled','compensating','compensated','compensation_failed')), revision BIGINT NOT NULL DEFAULT 1 CHECK (revision>0), request JSONB NOT NULL DEFAULT '{}'::jsonb, execution_receipt JSONB NOT NULL DEFAULT '{}'::jsonb, compensation_receipt JSONB NOT NULL DEFAULT '{}'::jsonb, error_message TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', requested_by TEXT NOT NULL, approved_by TEXT NOT NULL DEFAULT '', approved_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, UNIQUE (tenant_id,job_id), FOREIGN KEY (tenant_id,job_id) REFERENCES campaign_action_jobs(tenant_id,job_id) ON DELETE RESTRICT);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_campaign ON campaign_soar_jobs(tenant_id,campaign_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_soar_jobs_dispatch ON campaign_soar_jobs(next_attempt_at,created_at) WHERE status IN ('approved_awaiting_executor','compensation_queued');
CREATE TABLE IF NOT EXISTS campaign_soar_approvals (approval_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, decision TEXT NOT NULL CHECK (decision IN ('approve','reject')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), decided_by TEXT NOT NULL CHECK (decided_by<>''), resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL, approval_status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_campaign_soar_approvals_job ON campaign_soar_approvals(tenant_id,job_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_soar_execution_receipts (receipt_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, phase TEXT NOT NULL CHECK (phase IN ('execute','compensate')), attempt INTEGER NOT NULL CHECK (attempt>0), provider TEXT NOT NULL, provider_receipt_id TEXT NOT NULL, status TEXT NOT NULL CHECK (status IN ('succeeded','partial','failed')), external_effect BOOLEAN NOT NULL DEFAULT false, payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (job_id,phase,attempt), UNIQUE (tenant_id,provider,provider_receipt_id,phase));
CREATE INDEX IF NOT EXISTS idx_campaign_soar_receipts_job ON campaign_soar_execution_receipts(tenant_id,job_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_soar_control_requests (request_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES campaign_soar_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('cancel','compensate')), expected_revision BIGINT NOT NULL CHECK (expected_revision>0), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), requested_by TEXT NOT NULL CHECK (requested_by<>''), resulting_revision BIGINT NOT NULL CHECK (resulting_revision>0), resulting_status TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_campaign_soar_control_job ON campaign_soar_control_requests(tenant_id,job_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_workbench_state (tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, assignee TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','investigating','contained','closed')), state_version BIGINT NOT NULL DEFAULT 1, updated_by TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), member_count INTEGER NOT NULL DEFAULT 0, last_event_id UUID, PRIMARY KEY (tenant_id,campaign_id));
CREATE INDEX IF NOT EXISTS idx_campaign_workbench_state_tenant_status ON campaign_workbench_state(tenant_id,status,updated_at DESC);
CREATE TABLE IF NOT EXISTS campaign_aggregate_history (event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision>0), event_type TEXT NOT NULL, status TEXT NOT NULL, assignee TEXT NOT NULL DEFAULT '', member_count INTEGER NOT NULL DEFAULT 0 CHECK (member_count>=0), payload JSONB NOT NULL, reason TEXT NOT NULL, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,campaign_id,aggregate_revision));
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_history_revision ON campaign_aggregate_history(tenant_id,campaign_id,aggregate_revision DESC);
CREATE TABLE IF NOT EXISTS campaign_aggregate_outbox (event_id UUID PRIMARY KEY REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL, aggregate_id TEXT NOT NULL, aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision>0), event_type TEXT NOT NULL, schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version>0), partition_key TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0), last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE (tenant_id,aggregate_id,aggregate_revision));
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_pending ON campaign_aggregate_outbox(next_attempt_at,created_at) WHERE published=false;
ALTER TABLE campaign_aggregate_outbox ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE campaign_aggregate_outbox ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE campaign_aggregate_outbox ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
ALTER TABLE campaign_aggregate_outbox DROP CONSTRAINT IF EXISTS campaign_aggregate_outbox_status_check;
ALTER TABLE campaign_aggregate_outbox ADD CONSTRAINT campaign_aggregate_outbox_status_check CHECK (status IN ('pending','processing','published','dead'));
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_delivery ON campaign_aggregate_outbox(next_attempt_at,created_at,event_id) WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS campaign_reports (report_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, campaign_id TEXT NOT NULL, format TEXT NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf','word','json')), status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('queued','accepted','running','completed','succeeded','partial','failed','cancelled')), sections JSONB NOT NULL DEFAULT '[]'::jsonb, evidence_count INTEGER NOT NULL DEFAULT 0, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, campaign_revision BIGINT NOT NULL DEFAULT 0, snapshot_id UUID, snapshot JSONB NOT NULL DEFAULT '{}'::jsonb, snapshot_sha256 TEXT NOT NULL DEFAULT '', object_manifest JSONB NOT NULL DEFAULT '{}'::jsonb, error_message TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_campaign_reports_campaign_time ON campaign_reports(tenant_id,campaign_id,created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_tenant_idempotency ON campaign_reports(tenant_id,idempotency_key) WHERE idempotency_key<>'';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS job_id TEXT;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_bucket TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS object_key TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS artifact_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes>=0);
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0);
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '';
ALTER TABLE campaign_reports ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE UNIQUE INDEX IF NOT EXISTS uq_campaign_reports_job_id ON campaign_reports(job_id) WHERE job_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_campaign_reports_executor_pending ON campaign_reports(next_attempt_at,created_at,report_id) WHERE status IN ('accepted','running');
CREATE TABLE IF NOT EXISTS campaign_alert_links (relation_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL, alert_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'linked' CHECK (status IN ('linked','unlinked')), revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0), campaign_revision BIGINT NOT NULL DEFAULT 0 CHECK (campaign_revision >= 0), reason TEXT NOT NULL, idempotency_key TEXT NOT NULL, created_by TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,campaign_id,alert_id), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_alert ON campaign_alert_links (tenant_id,alert_id,updated_at DESC) WHERE status='linked';
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_campaign ON campaign_alert_links (tenant_id,campaign_id,updated_at DESC) WHERE status='linked';
CREATE TABLE IF NOT EXISTS campaign_alert_link_history (event_id UUID PRIMARY KEY, relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL, alert_id TEXT NOT NULL, event_type TEXT NOT NULL CHECK (event_type IN ('linked','unlinked')), revision BIGINT NOT NULL CHECK (revision > 0), campaign_revision BIGINT NOT NULL DEFAULT 0 CHECK (campaign_revision >= 0), payload JSONB NOT NULL, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (relation_id,revision));
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_history_relation ON campaign_alert_link_history (tenant_id,relation_id,revision);
CREATE TABLE IF NOT EXISTS campaign_alert_link_outbox (event_id UUID PRIMARY KEY REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, aggregate_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), event_type TEXT NOT NULL, schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0), partition_key TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE (aggregate_id,aggregate_version));
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_pending ON campaign_alert_link_outbox (next_attempt_at,created_at) WHERE published=false;
ALTER TABLE campaign_alert_link_outbox ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE campaign_alert_link_outbox ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE campaign_alert_link_outbox ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
ALTER TABLE campaign_alert_link_outbox DROP CONSTRAINT IF EXISTS campaign_alert_link_outbox_status_check;
ALTER TABLE campaign_alert_link_outbox ADD CONSTRAINT campaign_alert_link_outbox_status_check CHECK (status IN ('pending','processing','published','dead'));
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_delivery ON campaign_alert_link_outbox(next_attempt_at,created_at,event_id) WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS campaign_membership_commands (command_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, campaign_id TEXT NOT NULL, alert_id TEXT NOT NULL, operation TEXT NOT NULL CHECK (operation IN ('link','unlink')), idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), request_sha256 TEXT NOT NULL CHECK (length(request_sha256)=64), expected_relation_revision BIGINT NOT NULL CHECK (expected_relation_revision>=0), expected_campaign_revision BIGINT CHECK (expected_campaign_revision>=0), relation_revision BIGINT NOT NULL CHECK (relation_revision>0), campaign_revision BIGINT NOT NULL CHECK (campaign_revision>=0), result JSONB NOT NULL, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_relation ON campaign_membership_commands(tenant_id,relation_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_campaign_membership_commands_campaign ON campaign_membership_commands(tenant_id,campaign_id,campaign_revision DESC);
CREATE TABLE IF NOT EXISTS campaign_merge_receipts (merge_id UUID PRIMARY KEY, job_id TEXT NOT NULL UNIQUE, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, source_campaign_id TEXT NOT NULL, target_campaign_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200), request_sha256 TEXT NOT NULL CHECK (length(request_sha256)=64), source_expected_revision BIGINT NOT NULL CHECK (source_expected_revision>=0), target_expected_revision BIGINT NOT NULL CHECK (target_expected_revision>=0), source_revision BIGINT NOT NULL CHECK (source_revision>source_expected_revision), target_revision BIGINT NOT NULL CHECK (target_revision>target_expected_revision), source_member_count INTEGER NOT NULL CHECK (source_member_count>0 AND source_member_count<=1000), target_member_count_before INTEGER NOT NULL CHECK (target_member_count_before>=0), target_member_count_after INTEGER NOT NULL CHECK (target_member_count_after>=target_member_count_before), moved_count INTEGER NOT NULL CHECK (moved_count>=0), relinked_count INTEGER NOT NULL CHECK (relinked_count>=0), deduplicated_count INTEGER NOT NULL CHECK (deduplicated_count>=0), manifest JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'), manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), trace_id TEXT NOT NULL CHECK (length(trace_id)>0), created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key), UNIQUE (tenant_id,source_campaign_id), CHECK (source_campaign_id<>target_campaign_id), CHECK (moved_count+relinked_count+deduplicated_count=source_member_count), CHECK (target_member_count_after=target_member_count_before+moved_count+relinked_count));
CREATE INDEX IF NOT EXISTS idx_campaign_merge_receipts_target ON campaign_merge_receipts(tenant_id,target_campaign_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_merge_items (merge_id UUID NOT NULL REFERENCES campaign_merge_receipts(merge_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, source_relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, target_relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, alert_id TEXT NOT NULL, outcome TEXT NOT NULL CHECK (outcome IN ('moved','relinked','deduplicated')), source_relation_revision BIGINT NOT NULL CHECK (source_relation_revision>0), target_relation_revision BIGINT NOT NULL CHECK (target_relation_revision>0), source_event_id UUID NOT NULL REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT, target_event_id UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (merge_id,alert_id), UNIQUE (merge_id,source_relation_id), CHECK ((outcome='deduplicated' AND target_event_id IS NULL) OR (outcome IN ('moved','relinked') AND target_event_id IS NOT NULL)));
CREATE INDEX IF NOT EXISTS idx_campaign_merge_items_tenant_alert ON campaign_merge_items(tenant_id,alert_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_membership_backfill_runs (run_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, source_kind TEXT NOT NULL CHECK (source_kind='clickhouse_export'), source_uri TEXT NOT NULL CHECK (length(source_uri)>0), source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64), source_snapshot_id TEXT NOT NULL CHECK (length(source_snapshot_id)>0), source_as_of TIMESTAMPTZ NOT NULL, manifest JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'), manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64), reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000), status TEXT NOT NULL CHECK (status IN ('running','completed','partial','failed')), campaign_count INTEGER NOT NULL CHECK (campaign_count>0 AND campaign_count<=100), source_member_count INTEGER NOT NULL CHECK (source_member_count>=0), completed_campaign_count INTEGER NOT NULL DEFAULT 0 CHECK (completed_campaign_count>=0), failed_campaign_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_campaign_count>=0), inserted_count INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0), bound_count INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0), unchanged_count INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0), skipped_unlinked_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0), created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, UNIQUE (tenant_id,manifest_sha256), CHECK (completed_campaign_count+failed_campaign_count<=campaign_count));
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_runs_tenant ON campaign_membership_backfill_runs(tenant_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_membership_backfill_campaigns (run_id UUID NOT NULL REFERENCES campaign_membership_backfill_runs(run_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL, manifest_sha256 TEXT NOT NULL CHECK (length(manifest_sha256)=64), expected_campaign_revision BIGINT NOT NULL CHECK (expected_campaign_revision>=0), starting_campaign_revision BIGINT CHECK (starting_campaign_revision>=0), resulting_campaign_revision BIGINT CHECK (resulting_campaign_revision>=0), source_member_count INTEGER NOT NULL CHECK (source_member_count>=0 AND source_member_count<=1000), resulting_member_count INTEGER CHECK (resulting_member_count>=0), inserted_count INTEGER NOT NULL DEFAULT 0 CHECK (inserted_count>=0), bound_count INTEGER NOT NULL DEFAULT 0 CHECK (bound_count>=0), unchanged_count INTEGER NOT NULL DEFAULT 0 CHECK (unchanged_count>=0), skipped_unlinked_count INTEGER NOT NULL DEFAULT 0 CHECK (skipped_unlinked_count>=0), status TEXT NOT NULL CHECK (status IN ('running','completed','failed')), error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '', aggregate_event_id UUID REFERENCES campaign_aggregate_history(event_id) ON DELETE RESTRICT, started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, PRIMARY KEY (run_id,campaign_id), CHECK (inserted_count+bound_count+unchanged_count+skipped_unlinked_count<=source_member_count));
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_campaigns_tenant ON campaign_membership_backfill_campaigns(tenant_id,campaign_id,started_at DESC);
CREATE TABLE IF NOT EXISTS campaign_membership_backfill_items (run_id UUID NOT NULL, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, campaign_id TEXT NOT NULL, alert_id TEXT NOT NULL, source_ordinal INTEGER NOT NULL CHECK (source_ordinal>=0), outcome TEXT NOT NULL CHECK (outcome IN ('inserted','bound','unchanged','skipped_explicit_unlink')), relation_id UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT, relation_revision BIGINT NOT NULL CHECK (relation_revision>0), campaign_revision BIGINT NOT NULL CHECK (campaign_revision>=0), event_id UUID REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (run_id,campaign_id,alert_id), UNIQUE (run_id,campaign_id,source_ordinal), FOREIGN KEY (run_id,campaign_id) REFERENCES campaign_membership_backfill_campaigns(run_id,campaign_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED, CHECK ((outcome IN ('inserted','bound') AND event_id IS NOT NULL) OR (outcome IN ('unchanged','skipped_explicit_unlink') AND event_id IS NULL)));
CREATE INDEX IF NOT EXISTS idx_campaign_membership_backfill_items_tenant_alert ON campaign_membership_backfill_items(tenant_id,alert_id,created_at DESC);
CREATE TABLE IF NOT EXISTS campaign_event_projection_inbox (stream TEXT NOT NULL CHECK (stream IN ('aggregate','membership')), event_id UUID NOT NULL, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, aggregate_id TEXT NOT NULL, campaign_id TEXT NOT NULL, relation_id UUID, alert_id TEXT NOT NULL DEFAULT '', event_type TEXT NOT NULL, schema_version INTEGER NOT NULL CHECK (schema_version=2), aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision>=0), relation_revision BIGINT NOT NULL DEFAULT 0 CHECK (relation_revision>=0), partition_key TEXT NOT NULL, trace_id TEXT NOT NULL, payload JSONB NOT NULL, projection_status TEXT NOT NULL DEFAULT 'pending' CHECK (projection_status IN ('pending','processing','partial','applied','dead')), target_status JSONB NOT NULL DEFAULT '{"clickhouse":"pending","opensearch":"pending","nebulagraph":"pending"}'::jsonb CHECK (jsonb_typeof(target_status)='object' AND target_status->>'clickhouse' IN ('pending','applied','dead') AND target_status->>'opensearch' IN ('pending','applied','dead') AND target_status->>'nebulagraph' IN ('pending','applied','dead') AND target_status-'clickhouse'-'opensearch'-'nebulagraph'='{}'::jsonb), first_kafka_topic TEXT NOT NULL, first_kafka_partition INTEGER NOT NULL CHECK (first_kafka_partition>=0), first_kafka_offset BIGINT NOT NULL CHECK (first_kafka_offset>=0), attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0), available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ, last_error TEXT NOT NULL DEFAULT '', applied_at TIMESTAMPTZ, received_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (stream,event_id), CHECK ((stream='aggregate' AND aggregate_revision>0 AND relation_revision=0 AND relation_id IS NULL) OR (stream='membership' AND relation_revision>0 AND relation_id IS NOT NULL)));
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_pending ON campaign_event_projection_inbox(available_at,received_at,stream,event_id) WHERE projection_status IN ('pending','processing','partial');
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_campaign ON campaign_event_projection_inbox(tenant_id,campaign_id,aggregate_revision DESC,relation_revision DESC);
CREATE TABLE IF NOT EXISTS campaign_event_projection_deliveries (kafka_topic TEXT NOT NULL, kafka_partition INTEGER NOT NULL CHECK (kafka_partition>=0), kafka_offset BIGINT NOT NULL CHECK (kafka_offset>=0), stream TEXT NOT NULL, event_id UUID NOT NULL, received_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (kafka_topic,kafka_partition,kafka_offset), FOREIGN KEY (stream,event_id) REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT);
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_delivery_event ON campaign_event_projection_deliveries(stream,event_id,received_at);
CREATE TABLE IF NOT EXISTS campaign_event_projection_watermarks (kafka_topic TEXT NOT NULL, kafka_partition INTEGER NOT NULL CHECK (kafka_partition>=0), last_offset BIGINT NOT NULL CHECK (last_offset>=0), last_event_id UUID NOT NULL, last_stream TEXT NOT NULL CHECK (last_stream IN ('aggregate','membership')), last_received_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (kafka_topic,kafka_partition));
CREATE TABLE IF NOT EXISTS campaign_target_projection_watermarks (tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, projection_key TEXT NOT NULL, target TEXT NOT NULL CHECK (target IN ('clickhouse','opensearch','nebulagraph')), stream TEXT NOT NULL CHECK (stream IN ('aggregate','membership')), projection_version BIGINT NOT NULL CHECK (projection_version>0), event_id UUID NOT NULL, projection_sha256 TEXT NOT NULL CHECK (length(projection_sha256)=64), applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,projection_key,target), FOREIGN KEY (stream,event_id) REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_event ON campaign_target_projection_watermarks(stream,event_id,target);
CREATE INDEX IF NOT EXISTS idx_campaign_target_projection_version ON campaign_target_projection_watermarks(tenant_id,target,projection_version DESC);
CREATE TABLE IF NOT EXISTS alert_report_jobs (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, alert_id TEXT NOT NULL, format TEXT NOT NULL CHECK (format IN ('json','pdf','docx')), status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')), idempotency_key TEXT NOT NULL, requested_snapshot_id TEXT NOT NULL, snapshot JSONB NOT NULL, snapshot_sha256 TEXT NOT NULL, missing_sections JSONB NOT NULL DEFAULT '[]'::jsonb, source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb, object_bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '', mime_type TEXT NOT NULL DEFAULT '', artifact_sha256 TEXT NOT NULL DEFAULT '', size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0), error_message TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ, UNIQUE (tenant_id,idempotency_key));
ALTER TABLE alert_report_jobs ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1, ADD COLUMN IF NOT EXISTS cancellation_reason TEXT NOT NULL DEFAULT '', ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ, ADD COLUMN IF NOT EXISTS cancelled_at TIMESTAMPTZ;
ALTER TABLE alert_report_jobs DROP CONSTRAINT IF EXISTS alert_report_jobs_status_check, DROP CONSTRAINT IF EXISTS chk_alert_report_status, DROP CONSTRAINT IF EXISTS chk_alert_report_revision;
ALTER TABLE alert_report_jobs ADD CONSTRAINT chk_alert_report_status CHECK (status IN ('accepted','running','cancel_requested','completed','partial','failed','cancelled','compensating','compensated','compensation_failed')), ADD CONSTRAINT chk_alert_report_revision CHECK (revision>0);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_alert_time ON alert_report_jobs (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_queue ON alert_report_jobs (status,next_attempt_at,created_at) WHERE status IN ('accepted','running');
CREATE INDEX IF NOT EXISTS idx_alert_report_jobs_cancel_cleanup ON alert_report_jobs(next_attempt_at,created_at) WHERE status='cancel_requested';
CREATE TABLE IF NOT EXISTS alert_report_outbox (event_id UUID PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, event_type TEXT NOT NULL, aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0), partition_key TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ, UNIQUE (job_id,aggregate_version));
CREATE INDEX IF NOT EXISTS idx_alert_report_outbox_pending ON alert_report_outbox (next_attempt_at,created_at) WHERE published=false;
CREATE TABLE IF NOT EXISTS alert_report_job_history (transition_id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, from_status TEXT NOT NULL, to_status TEXT NOT NULL, revision BIGINT NOT NULL CHECK (revision>0), actor TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(job_id,revision));
CREATE INDEX IF NOT EXISTS idx_alert_report_history_tenant_job ON alert_report_job_history(tenant_id,job_id,revision);
INSERT INTO alert_report_job_history(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail) SELECT job_id,tenant_id,'',status,revision,COALESCE(NULLIF(created_by,''),'migration'),'legacy alert report lifecycle backfill','',jsonb_build_object('backfilled',true) FROM alert_report_jobs ON CONFLICT (job_id,revision) DO NOTHING;
CREATE TABLE IF NOT EXISTS alert_report_control_requests (request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, job_id TEXT NOT NULL REFERENCES alert_report_jobs(job_id) ON DELETE RESTRICT, operation TEXT NOT NULL CHECK(operation IN ('cancel','compensate')), idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200), request_hash TEXT NOT NULL CHECK(length(request_hash)=64), expected_revision BIGINT NOT NULL CHECK(expected_revision>0), resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0), result_payload JSONB NOT NULL DEFAULT '{}'::jsonb, actor TEXT NOT NULL, reason TEXT NOT NULL CHECK(length(reason) BETWEEN 8 AND 1000), trace_id TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,idempotency_key));
CREATE INDEX IF NOT EXISTS idx_alert_report_control_job ON alert_report_control_requests(tenant_id,job_id,created_at);
INSERT INTO alignment_schema_migrations(version,description) VALUES ('202608041900','F-ALERT revisioned cooperative report cancellation') ON CONFLICT (version) DO NOTHING;
CREATE TABLE IF NOT EXISTS alert_playbook_overrides (tenant_id TEXT NOT NULL, name TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true, max_runs INTEGER NOT NULL DEFAULT 0, cooldown_seconds BIGINT NOT NULL DEFAULT 0, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, name));
CREATE TABLE IF NOT EXISTS alert_playbook_definitions (
    tenant_id TEXT NOT NULL, name TEXT NOT NULL, display_name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1, stage TEXT NOT NULL DEFAULT 'draft', enabled BOOLEAN NOT NULL DEFAULT false,
    risk_level TEXT NOT NULL DEFAULT 'medium', definition_payload JSONB NOT NULL, created_by TEXT NOT NULL DEFAULT '',
    submitted_by TEXT NOT NULL DEFAULT '', approved_by TEXT NOT NULL DEFAULT '', rejection_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, name),
    CONSTRAINT alert_playbook_definition_stage_check CHECK (stage IN ('draft', 'approval_pending', 'approved', 'rejected')),
    CONSTRAINT alert_playbook_definition_risk_check CHECK (risk_level IN ('low', 'medium', 'high', 'critical'))
);
CREATE INDEX IF NOT EXISTS idx_alert_playbook_definitions_tenant_stage ON alert_playbook_definitions (tenant_id, stage, updated_at DESC);
CREATE TABLE IF NOT EXISTS data_quality_actions (
    action_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL,
    view_name TEXT NOT NULL,
    action_name TEXT NOT NULL,
    target TEXT NOT NULL,
    dry_run BOOLEAN NOT NULL DEFAULT TRUE,
    status TEXT NOT NULL DEFAULT 'dry_run',
    requested_by TEXT NOT NULL DEFAULT '',
    request_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_data_quality_actions_tenant_created ON data_quality_actions (tenant_id, created_at DESC);
CREATE TABLE IF NOT EXISTS data_quality_ui_fixtures (
    tenant_id TEXT PRIMARY KEY,
    fixture_version TEXT NOT NULL,
    payload JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_data_quality_ui_fixtures_active
    ON data_quality_ui_fixtures (tenant_id, active);
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_event_id ON audit_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_object ON audit_logs(object_type, object_id) WHERE object_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS encrypted_traffic_ui_fixtures (
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL CHECK (endpoint IN ('stats','sessions','ja3','tunnels','exfiltration','evidence')),
    fixture_version TEXT NOT NULL,
    payload JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_encrypted_traffic_ui_fixtures_active
    ON encrypted_traffic_ui_fixtures(tenant_id, active, endpoint);

CREATE TABLE IF NOT EXISTS forensics_ui_fixtures (
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL CHECK (endpoint IN ('jobs','stats')),
    fixture_version TEXT NOT NULL,
    payload JSONB NOT NULL,
    active BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, endpoint)
);
CREATE INDEX IF NOT EXISTS idx_forensics_ui_fixtures_active
    ON forensics_ui_fixtures(tenant_id, active, endpoint);

CREATE TABLE IF NOT EXISTS fusion_conflict_resolutions (
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    conflict_id TEXT NOT NULL,
    object_id TEXT NOT NULL DEFAULT '',
    object_type TEXT NOT NULL DEFAULT 'entity',
    field_name TEXT NOT NULL,
    selected_source TEXT NOT NULL,
    selected_value TEXT NOT NULL,
    strategy TEXT NOT NULL DEFAULT 'manual',
    note TEXT NOT NULL DEFAULT '',
    rule_id TEXT NOT NULL DEFAULT '',
    state_version BIGINT NOT NULL DEFAULT 1,
    resolved_by TEXT NOT NULL DEFAULT '',
    resolved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (tenant_id, conflict_id)
);
CREATE INDEX IF NOT EXISTS idx_fusion_conflict_resolutions_time ON fusion_conflict_resolutions(tenant_id, resolved_at DESC);

CREATE TABLE IF NOT EXISTS fusion_rule_overrides (
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL,
    rule_name TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'draft' CONSTRAINT fusion_rule_overrides_status_check CHECK (status IN ('active','draft','disabled')),
    strategy TEXT NOT NULL DEFAULT 'manual-review' CONSTRAINT fusion_rule_overrides_strategy_check CHECK (strategy IN ('authoritative-source','weighted-confidence','latest-observation','manual-review')),
    confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.85 CONSTRAINT fusion_rule_overrides_threshold_check CHECK (confidence_threshold BETWEEN 0 AND 1),
    note TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
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
CREATE INDEX IF NOT EXISTS idx_fusion_rule_overrides_time ON fusion_rule_overrides(tenant_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS fusion_conflicts (
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    conflict_id TEXT NOT NULL,
    object_id TEXT NOT NULL,
    object_type TEXT NOT NULL DEFAULT 'entity',
    field_name TEXT NOT NULL,
    source_values JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_count INTEGER NOT NULL DEFAULT 0,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    severity TEXT NOT NULL DEFAULT 'medium',
    status TEXT NOT NULL DEFAULT 'pending',
    rule_id TEXT NOT NULL DEFAULT '',
    state_version BIGINT NOT NULL DEFAULT 1,
    origin TEXT NOT NULL DEFAULT 'runtime',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, conflict_id)
);
ALTER TABLE fusion_conflicts ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'runtime';
ALTER TABLE fusion_conflicts ADD COLUMN IF NOT EXISTS detail JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS idx_fusion_conflicts_queue ON fusion_conflicts(tenant_id, status, detected_at DESC);

CREATE TABLE IF NOT EXISTS fusion_repair_tasks (
    task_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    conflict_id TEXT NOT NULL,
    object_id TEXT NOT NULL DEFAULT '',
    object_type TEXT NOT NULL DEFAULT 'entity',
    field_name TEXT NOT NULL,
    rule_id TEXT NOT NULL DEFAULT '',
    selected_source TEXT NOT NULL,
    selected_value TEXT NOT NULL,
    state_version BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','in_progress','completed','failed','cancelled')),
    requested_by TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, conflict_id, state_version),
    FOREIGN KEY (tenant_id, conflict_id) REFERENCES fusion_conflicts(tenant_id, conflict_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_fusion_repair_tasks_queue ON fusion_repair_tasks(tenant_id, status, created_at DESC);

-- =========================================================================================
-- Graph Service 配置表（来自 graph_postgres.sql）
-- =========================================================================================

-- graph_cache_config: 缓存配置
CREATE TABLE IF NOT EXISTS graph_cache_config (
    config_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    
    neighbor_ttl_sec INT NOT NULL DEFAULT 300,
    entity_ttl_sec INT NOT NULL DEFAULT 300,
    graph_ttl_sec INT NOT NULL DEFAULT 120,
    
    max_nodes_per_cache INT NOT NULL DEFAULT 500,
    max_edges_per_cache INT NOT NULL DEFAULT 1000,
    
    time_granularity_sec INT NOT NULL DEFAULT 300,
    
    enabled BOOLEAN NOT NULL DEFAULT true,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE (tenant_id)
);

-- graph_query_config: 查询配置
CREATE TABLE IF NOT EXISTS graph_query_config (
    config_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    
    max_depth INT NOT NULL DEFAULT 5,
    default_depth INT NOT NULL DEFAULT 2,
    
    max_nodes INT NOT NULL DEFAULT 500,
    max_neighbors_per_hop INT NOT NULL DEFAULT 50,
    
    default_time_range_hours INT NOT NULL DEFAULT 24,
    
    max_batch_explore_ips INT NOT NULL DEFAULT 10,
    
    max_path_search_hops INT NOT NULL DEFAULT 10,
    
    alert_batch_size INT NOT NULL DEFAULT 100,
    
    query_timeout_sec INT NOT NULL DEFAULT 30,
    
    max_concurrent_queries INT NOT NULL DEFAULT 10,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE (tenant_id)
);

-- graph_hot_ips: 热点 IP 缓存预热
CREATE TABLE IF NOT EXISTS graph_hot_ips (
    hot_ip_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    ip TEXT NOT NULL,
    
    query_count BIGINT NOT NULL DEFAULT 0,
    last_query_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    priority INT NOT NULL DEFAULT 0,
    
    warmed_up BOOLEAN NOT NULL DEFAULT false,
    last_warmup_at TIMESTAMPTZ,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    UNIQUE (tenant_id, ip)
);

CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_priority ON graph_hot_ips(tenant_id, priority DESC, last_query_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_ip ON graph_hot_ips(tenant_id, ip);

-- graph_query_history: 查询历史
CREATE TABLE IF NOT EXISTS graph_query_history (
    query_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(user_id),
    
    query_type TEXT NOT NULL,
    center_ip TEXT,
    center_ips TEXT[],
    depth INT,
    run_id TEXT,
    
    start_time TIMESTAMPTZ,
    end_time TIMESTAMPTZ,
    
    node_count INT,
    edge_count INT,
    path_count INT,
    
    duration_ms BIGINT NOT NULL,
    cache_hit BOOLEAN NOT NULL DEFAULT false,
    
    status TEXT NOT NULL,
    error_message TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_graph_query_history_tenant_time ON graph_query_history(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_query_history_type_status ON graph_query_history(query_type, status, created_at DESC);

-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- alert_feedback: 告警反馈表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_feedback (
  feedback_id    UUID PRIMARY KEY,
  event_id       UUID NOT NULL UNIQUE,
  tenant_id      TEXT NOT NULL,
  alert_id       TEXT NOT NULL,
  user_id        UUID REFERENCES users(user_id),
  label          TEXT NOT NULL CHECK (label IN ('TP','FP')),
  reason_code    TEXT,
  comment        TEXT,
  add_to_whitelist BOOLEAN NOT NULL DEFAULT false,
  alert_type     TEXT NOT NULL DEFAULT '',
  severity       TEXT NOT NULL DEFAULT '',
  model_version  TEXT NOT NULL DEFAULT '',
  rule_version   TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL DEFAULT '',
  trace_id       TEXT NOT NULL DEFAULT '',
  payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
  status         TEXT NOT NULL DEFAULT 'accepted',
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_feedback_tenant_idempotency ON alert_feedback (tenant_id,idempotency_key) WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_alert ON alert_feedback (tenant_id,alert_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_time ON alert_feedback (tenant_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_label ON alert_feedback (tenant_id, label, created_at DESC);
CREATE TABLE IF NOT EXISTS alert_feedback_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  feedback_id UUID NOT NULL REFERENCES alert_feedback(feedback_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, partition_key TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version = 1),
  payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_feedback_outbox_pending ON alert_feedback_outbox (next_attempt_at,outbox_id) WHERE published=false;

-- -----------------------------------------------------------------------------------------
-- whitelist: 白名单治理表（Web UI /api/v1/whitelist）
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS whitelist (
  id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id       TEXT NOT NULL,
  type            TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
  value           TEXT NOT NULL,
  reason          TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'draft',
  approval_status TEXT NOT NULL DEFAULT 'draft',
  source_alert_id TEXT NOT NULL DEFAULT '',
  feedback_id     TEXT NOT NULL DEFAULT '',
  owner_role      TEXT NOT NULL DEFAULT '',
  scope           TEXT NOT NULL DEFAULT '',
  risk_level      TEXT NOT NULL DEFAULT 'medium',
  covered_alerts  INTEGER NOT NULL DEFAULT 0,
  covered_assets  INTEGER NOT NULL DEFAULT 0,
  version         INTEGER NOT NULL DEFAULT 1,
  created_by      TEXT NOT NULL DEFAULT '',
  approved_by     TEXT NOT NULL DEFAULT '',
  approved_at     TIMESTAMPTZ,
  disabled_at     TIMESTAMPTZ,
  expires_at      TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, type, value)
);

ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'draft';
ALTER TABLE whitelist ALTER COLUMN status SET DEFAULT 'draft';
ALTER TABLE whitelist ALTER COLUMN approval_status SET DEFAULT 'draft';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS source_alert_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS feedback_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS owner_role TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_alerts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS covered_assets INTEGER NOT NULL DEFAULT 0;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_type_check;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_type_check CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model'));
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_governance_state_check;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_governance_state_check CHECK (
  (status='draft' AND approval_status='draft') OR
  (status='pending' AND approval_status='pending') OR
  (status='active' AND approval_status='approved') OR
  (status='disabled' AND approval_status IN ('approved','rejected'))
);
CREATE INDEX IF NOT EXISTS idx_whitelist_tenant ON whitelist (tenant_id);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_tenant_status ON whitelist (tenant_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_entries_approval ON whitelist (tenant_id, approval_status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_whitelist_source_alert ON whitelist (tenant_id, source_alert_id) WHERE source_alert_id <> '';
CREATE INDEX IF NOT EXISTS idx_whitelist_expires ON whitelist (expires_at) WHERE expires_at IS NOT NULL;

-- -----------------------------------------------------------------------------------------
-- whitelist_rules: 白名单规则表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS whitelist_rules (
  rule_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  rule_type     TEXT NOT NULL,
  pattern       JSONB NOT NULL,
  reason_code   TEXT,
  comment       TEXT,
  status        TEXT NOT NULL DEFAULT 'active',
  created_by    UUID REFERENCES users(user_id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_whitelist_tenant_status ON whitelist_rules (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_whitelist_type ON whitelist_rules (tenant_id, rule_type, status);

-- -----------------------------------------------------------------------------------------
-- alert_state_history: 告警状态历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_state_history (
  history_id    BIGSERIAL PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  alert_id      TEXT NOT NULL,
  old_status    TEXT,
  new_status    TEXT NOT NULL,
  old_assignee  TEXT,
  new_assignee  TEXT,
  changed_by    UUID REFERENCES users(user_id),
  change_reason TEXT,
  version       BIGINT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_history_tenant_alert 
  ON alert_state_history (tenant_id, alert_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_alert_history_time 
  ON alert_state_history (tenant_id, created_at DESC);

-- -----------------------------------------------------------------------------------------
-- evidence_metadata: 证据元数据表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS evidence_metadata (
  evidence_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  alert_id      TEXT NOT NULL,
  evidence_type TEXT NOT NULL,
  storage_ref   TEXT,
  confidence    FLOAT NOT NULL DEFAULT 0.0,
  status        TEXT NOT NULL DEFAULT 'available',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_evidence_tenant_alert ON evidence_metadata (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_evidence_type ON evidence_metadata (tenant_id, evidence_type, created_at DESC);

-- -----------------------------------------------------------------------------------------
-- campaign_metadata: 战役元数据表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS campaign_metadata (
  campaign_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  name          TEXT NOT NULL,
  description   TEXT,
  severity      TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'active',
  alert_count   INT NOT NULL DEFAULT 0,
  entity_count  INT NOT NULL DEFAULT 0,
  assigned_to   UUID REFERENCES users(user_id),
  started_at    TIMESTAMPTZ NOT NULL,
  ended_at      TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_campaign_tenant_status 
  ON campaign_metadata (tenant_id, status, started_at DESC);

CREATE INDEX IF NOT EXISTS idx_campaign_severity 
  ON campaign_metadata (tenant_id, severity, started_at DESC);

-- -----------------------------------------------------------------------------------------
-- alert_tags: 告警标签表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_tags (
  tag_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL,
  alert_id    TEXT NOT NULL,
  tag_key     TEXT NOT NULL,
  tag_value   TEXT NOT NULL,
  created_by  UUID REFERENCES users(user_id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_tags_tenant_alert ON alert_tags (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_alert_tags_key_value ON alert_tags (tenant_id, tag_key, tag_value);

-- -----------------------------------------------------------------------------------------
-- dedup_fingerprints: 去重指纹表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS dedup_fingerprints (
  fingerprint   TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  alert_id      TEXT NOT NULL,
  alert_type    TEXT NOT NULL,
  first_seen    TIMESTAMPTZ NOT NULL,
  last_seen     TIMESTAMPTZ NOT NULL,
  occurrence_count BIGINT NOT NULL DEFAULT 1,
  last_updated  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dedup_tenant_type ON dedup_fingerprints (tenant_id, alert_type, last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_dedup_last_seen ON dedup_fingerprints (tenant_id, last_seen DESC);

-- -----------------------------------------------------------------------------------------
-- storage_health_history: 存储健康历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS storage_health_history (
  id            BIGSERIAL PRIMARY KEY,
  storage_type  TEXT NOT NULL,
  status        TEXT NOT NULL,
  error_message TEXT,
  check_time    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_storage_health_type_time 
  ON storage_health_history (storage_type, check_time DESC);

-- -----------------------------------------------------------------------------------------
-- export_jobs: 导出任务表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS export_jobs (
  job_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  user_id       UUID REFERENCES users(user_id),
  export_type   TEXT NOT NULL,
  query_params  JSONB NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',
  file_path     TEXT,
  record_count  INT,
  error_message TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at    TIMESTAMPTZ,
  completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_export_tenant_user ON export_jobs (tenant_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_export_status ON export_jobs (status, created_at DESC);

-- -----------------------------------------------------------------------------------------
-- alert_metrics_hourly: 告警小时指标表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_metrics_hourly (
  metric_id     BIGSERIAL PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  hour          TIMESTAMPTZ NOT NULL,
  severity      TEXT NOT NULL,
  alert_type    TEXT NOT NULL,
  status        TEXT NOT NULL,
  count         BIGINT NOT NULL DEFAULT 0,
  avg_score     FLOAT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_metrics_tenant_hour ON alert_metrics_hourly (tenant_id, hour DESC);
CREATE INDEX IF NOT EXISTS idx_alert_metrics_severity ON alert_metrics_hourly (tenant_id, severity, hour DESC);

-- -----------------------------------------------------------------------------------------
-- alert_metrics_daily: 告警日指标表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alert_metrics_daily (
  metric_id     BIGSERIAL PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  day           DATE NOT NULL,
  severity      TEXT NOT NULL,
  alert_type    TEXT NOT NULL,
  status        TEXT NOT NULL,
  count         BIGINT NOT NULL DEFAULT 0,
  avg_score     FLOAT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_metrics_daily_tenant_day ON alert_metrics_daily (tenant_id, day DESC);

-- -----------------------------------------------------------------------------------------
-- model_training_jobs: 模型训练任务表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS model_training_jobs (
  job_id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL,
  model_id       UUID REFERENCES models(model_id),
  training_type  TEXT NOT NULL,
  data_source    JSONB NOT NULL,
  hyperparams    JSONB,
  status         TEXT NOT NULL DEFAULT 'pending',
  metrics        JSONB,
  artifact_uri   TEXT,
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at     TIMESTAMPTZ,
  completed_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_training_tenant_model ON model_training_jobs (tenant_id, model_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_training_status ON model_training_jobs (status, created_at DESC);

-- -----------------------------------------------------------------------------------------
-- arkime_session_links: Arkime 会话链接缓存表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS arkime_session_links (
  link_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  alert_id      TEXT NOT NULL,
  community_id  TEXT NOT NULL,
  session_link  TEXT NOT NULL,
  pcap_link     TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_arkime_tenant_alert ON arkime_session_links (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_arkime_community ON arkime_session_links (tenant_id, community_id);

-- -----------------------------------------------------------------------------------------
-- notification_rules: 通知规则配置表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_rules (
  rule_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  name          TEXT NOT NULL,
  conditions    JSONB NOT NULL DEFAULT '{}'::jsonb,
  channels      JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled       BOOLEAN NOT NULL DEFAULT true,
  created_by    UUID,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_notification_rules_tenant_enabled
  ON notification_rules (tenant_id, enabled, updated_at DESC);

-- -----------------------------------------------------------------------------------------
-- notification_history: 通知历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_history (
  notification_id BIGSERIAL PRIMARY KEY,
  tenant_id       TEXT NOT NULL,
  rule_id         UUID REFERENCES notification_rules(rule_id),
  alert_id        TEXT NOT NULL,
  target_name     TEXT NOT NULL DEFAULT '',
  channel         TEXT NOT NULL,
  alert_type      TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL,
  error_message   TEXT,
  retry_count     INTEGER NOT NULL DEFAULT 0,
  trace_id        TEXT NOT NULL DEFAULT '',
  sent_at         TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS target_name TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS alert_type TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE notification_history ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_notification_tenant_alert ON notification_history (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_notification_status ON notification_history (tenant_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS alert_notification_settings (
  tenant_id  TEXT PRIMARY KEY,
  settings   JSONB NOT NULL DEFAULT '{}'::jsonb,
  revision   BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notification_escalation_policies (
  policy_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL,
  name       TEXT NOT NULL,
  stages     JSONB NOT NULL DEFAULT '[]'::jsonb,
  enabled    BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_notification_escalation_tenant_enabled ON notification_escalation_policies (tenant_id, enabled, updated_at DESC);

CREATE TABLE IF NOT EXISTS notification_escalation_jobs (
  job_id BIGSERIAL PRIMARY KEY, tenant_id TEXT NOT NULL, alert_key TEXT NOT NULL,
  alert_id TEXT NOT NULL DEFAULT '', rule_id UUID NOT NULL REFERENCES notification_rules(rule_id) ON DELETE CASCADE,
  stage_index INTEGER NOT NULL, policy_id UUID, policy_updated_at TIMESTAMPTZ,
  stage_after_minutes DOUBLE PRECISION, stage_fingerprint TEXT NOT NULL DEFAULT '',
  target_role TEXT NOT NULL, channel TEXT NOT NULL,
  due_at TIMESTAMPTZ NOT NULL, alert_payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT, locked_at TIMESTAMPTZ,
  lock_token TEXT NOT NULL DEFAULT '', trace_id TEXT NOT NULL DEFAULT '',
  completed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, alert_key, rule_id, stage_index, channel)
);
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS lock_token TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_id UUID;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS policy_updated_at TIMESTAMPTZ;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_after_minutes DOUBLE PRECISION;
ALTER TABLE notification_escalation_jobs ADD COLUMN IF NOT EXISTS stage_fingerprint TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_notification_escalation_jobs_due ON notification_escalation_jobs (status, due_at, job_id);

CREATE TABLE IF NOT EXISTS notification_templates (
  template_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL,
  template_type TEXT NOT NULL,
  name TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1,
  subject TEXT NOT NULL DEFAULT '',
  body TEXT NOT NULL DEFAULT '',
  variable_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
  validation_status TEXT NOT NULL DEFAULT 'passed',
  enabled BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_notification_templates_tenant_enabled ON notification_templates (tenant_id, enabled, updated_at DESC);

-- -----------------------------------------------------------------------------------------
-- notification_silence_rules: 通知静默窗口
-- -----------------------------------------------------------------------------------------
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
  revision         BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_time
  ON notification_silence_rules (tenant_id, starts_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_silence_tenant_enabled
  ON notification_silence_rules (tenant_id, enabled, starts_at DESC);


CREATE OR REPLACE FUNCTION notification_governance_atomic_audit()
RETURNS TRIGGER AS $$
DECLARE
  row_data JSONB;
  tenant_value TEXT;
  object_value TEXT;
  action_prefix TEXT;
BEGIN
  row_data := CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
  tenant_value := COALESCE(row_data->>'tenant_id', 'default');
  object_value := CASE
    WHEN TG_TABLE_NAME = 'notification_escalation_jobs' THEN COALESCE(row_data->>'job_id', tenant_value)
    ELSE COALESCE(row_data->>'rule_id', row_data->>'template_id', row_data->>'policy_id', row_data->>'notification_id', tenant_value)
  END;
  action_prefix := CASE TG_TABLE_NAME
    WHEN 'alert_notification_settings' THEN 'NOTIFICATION_SETTINGS'
    WHEN 'notification_rules' THEN 'NOTIFICATION_RULE'
    WHEN 'notification_templates' THEN 'NOTIFICATION_TEMPLATE'
    WHEN 'notification_escalation_policies' THEN 'NOTIFICATION_ESCALATION'
    WHEN 'notification_escalation_jobs' THEN 'NOTIFICATION_ESCALATION_JOB'
    WHEN 'notification_silence_rules' THEN 'NOTIFICATION_SILENCE_RULE'
    WHEN 'notification_history' THEN 'NOTIFICATION_DELIVERY'
    ELSE 'NOTIFICATION_GOVERNANCE'
  END;
  INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail)
  VALUES (
    'audit-' || uuid_generate_v4()::TEXT,
    tenant_value,
    NULL,
    action_prefix || '_DB_' || TG_OP,
    TG_TABLE_NAME,
    object_value,
    jsonb_build_object('atomic', true, 'operation', TG_OP)
  );
  RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
  table_name TEXT;
  trigger_name TEXT;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'alert_notification_settings',
    'notification_rules',
    'notification_templates',
    'notification_escalation_policies',
    'notification_escalation_jobs',
    'notification_silence_rules',
    'notification_history'
  ]
  LOOP
    trigger_name := 'trg_' || table_name || '_atomic_audit';
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I', trigger_name, table_name);
    EXECUTE format(
      'CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION notification_governance_atomic_audit()',
      trigger_name,
      table_name
    );
  END LOOP;
END $$;


-- -----------------------------------------------------------------------------------------
-- topic governance: 专题视图、范围、订阅和导出
-- -----------------------------------------------------------------------------------------
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

CREATE TABLE IF NOT EXISTS topic_actions (
  action_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL,
  topic        TEXT NOT NULL,
  action       TEXT NOT NULL,
  target       TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'queued',
  detail       JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_topic_actions_tenant_topic
  ON topic_actions (tenant_id, topic, created_at DESC);

CREATE TABLE IF NOT EXISTS topic_snapshot_manifests (
  snapshot_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  topic TEXT NOT NULL CHECK (topic IN ('tunnel','exfil','apt')),
  resource_revision BIGINT NOT NULL CHECK (resource_revision > 0),
  as_of TIMESTAMPTZ NOT NULL, range_start BIGINT NOT NULL, range_end BIGINT NOT NULL,
  payload JSONB NOT NULL, payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  partial BOOLEAN NOT NULL DEFAULT false, missing_sections TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb, trace_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_tenant_topic ON topic_snapshot_manifests (tenant_id,topic,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_trace ON topic_snapshot_manifests (tenant_id,trace_id) WHERE trace_id<>'';
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
CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_actions_tenant_idempotency ON topic_actions (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_topic_actions_executor_queue ON topic_actions (executor,status,created_at) WHERE status IN ('accepted','running');
CREATE TABLE IF NOT EXISTS topic_action_history (
  history_id BIGSERIAL PRIMARY KEY, job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK (revision > 0), from_status TEXT NOT NULL,
  to_status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (job_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_history_tenant_job ON topic_action_history (tenant_id,job_id,revision);
CREATE TABLE IF NOT EXISTS topic_action_receipts (
  receipt_id UUID PRIMARY KEY, job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, executor TEXT NOT NULL, effect_type TEXT NOT NULL, effect_ref TEXT NOT NULL,
  receipt JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (job_id,effect_type,effect_ref)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_receipts_tenant_job ON topic_action_receipts (tenant_id,job_id);
CREATE TABLE IF NOT EXISTS topic_action_outbox (
  event_id UUID PRIMARY KEY, job_id UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0), partition_key TEXT NOT NULL,
  payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ,
  UNIQUE (job_id,event_type)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_outbox_pending ON topic_action_outbox (next_attempt_at,created_at) WHERE published=false;
CREATE TABLE IF NOT EXISTS topic_action_event_projection (
  event_id UUID PRIMARY KEY, job_id UUID NOT NULL, tenant_id TEXT NOT NULL, topic TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type IN ('traffic.topic.v2.ActionRequested','traffic.topic.v2.ActionResult')),
  revision BIGINT NOT NULL CHECK (revision > 0), action_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '', trace_id TEXT NOT NULL, payload JSONB NOT NULL,
  kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  projected_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_event_projection_tenant_job ON topic_action_event_projection (tenant_id,job_id,revision);
CREATE TABLE IF NOT EXISTS topic_action_job_projection (
  tenant_id TEXT NOT NULL, job_id UUID NOT NULL, topic TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0), event_type TEXT NOT NULL,
  action_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', trace_id TEXT NOT NULL,
  last_event_id UUID NOT NULL, payload JSONB NOT NULL, kafka_partition INTEGER NOT NULL,
  kafka_offset BIGINT NOT NULL, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,job_id)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_job_projection_tenant_topic ON topic_action_job_projection (tenant_id,topic,updated_at DESC);
CREATE TABLE IF NOT EXISTS probe_operation_event_projection (
  event_id UUID PRIMARY KEY, operation_id UUID NOT NULL, tenant_id TEXT NOT NULL,
  probe_id TEXT NOT NULL,
  event_type TEXT NOT NULL CHECK (event_type = 'traffic.probe.v2.OperationAcknowledged'),
  revision BIGINT NOT NULL CHECK (revision > 0), status TEXT NOT NULL,
  trace_id TEXT NOT NULL, payload JSONB NOT NULL, kafka_partition INTEGER NOT NULL,
  kafka_offset BIGINT NOT NULL, projected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_event_projection_tenant_operation ON probe_operation_event_projection (tenant_id,operation_id,revision);
CREATE TABLE IF NOT EXISTS probe_operation_state_projection (
  tenant_id TEXT NOT NULL, operation_id UUID NOT NULL, probe_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0), event_type TEXT NOT NULL,
  status TEXT NOT NULL, trace_id TEXT NOT NULL, last_event_id UUID NOT NULL,
  payload JSONB NOT NULL, kafka_partition INTEGER NOT NULL, kafka_offset BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,operation_id)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_state_projection_tenant_probe ON probe_operation_state_projection (tenant_id,probe_id,updated_at DESC);

-- =========================================================================================
-- Graph Service 表（来源：graph_postgres.sql）
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- graph_cache_config: 图缓存配置表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS graph_cache_config (
  config_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  
  neighbor_ttl_sec  INT NOT NULL DEFAULT 300,
  entity_ttl_sec    INT NOT NULL DEFAULT 300,
  graph_ttl_sec     INT NOT NULL DEFAULT 120,
  
  max_nodes_per_cache INT NOT NULL DEFAULT 500,
  max_edges_per_cache INT NOT NULL DEFAULT 1000,
  
  time_granularity_sec INT NOT NULL DEFAULT 300,
  
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  
  UNIQUE (tenant_id)
);

-- -----------------------------------------------------------------------------------------
-- graph_query_config: 图查询配置表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS graph_query_config (
  config_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  
  max_depth              INT NOT NULL DEFAULT 5,
  default_depth          INT NOT NULL DEFAULT 2,
  
  max_nodes              INT NOT NULL DEFAULT 500,
  max_neighbors_per_hop  INT NOT NULL DEFAULT 50,
  
  default_time_range_hours INT NOT NULL DEFAULT 24,
  
  max_batch_explore_ips INT NOT NULL DEFAULT 10,
  
  max_path_search_hops  INT NOT NULL DEFAULT 10,
  
  alert_batch_size      INT NOT NULL DEFAULT 100,
  
  query_timeout_sec     INT NOT NULL DEFAULT 30,
  
  max_concurrent_queries INT NOT NULL DEFAULT 10,
  
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  
  UNIQUE (tenant_id)
);

-- -----------------------------------------------------------------------------------------
-- graph_hot_ips: 热点 IP 缓存预热表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS graph_hot_ips (
  hot_ip_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  ip           TEXT NOT NULL,
  
  query_count       BIGINT NOT NULL DEFAULT 0,
  last_query_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  
  priority          INT NOT NULL DEFAULT 0,
  
  warmed_up         BOOLEAN NOT NULL DEFAULT FALSE,
  last_warmup_at    TIMESTAMPTZ,
  
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  
  UNIQUE (tenant_id, ip)
);

CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_priority 
  ON graph_hot_ips (tenant_id, priority DESC, last_query_at DESC);

CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_ip 
  ON graph_hot_ips (tenant_id, ip);

-- -----------------------------------------------------------------------------------------
-- graph_query_history: 图查询历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS graph_query_history (
  query_id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id         TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  user_id           UUID REFERENCES users(user_id),
  
  query_type        TEXT NOT NULL,
  center_ip         TEXT,
  center_ips        TEXT[],
  depth             INT,
  run_id            TEXT,
  
  start_time        TIMESTAMPTZ,
  end_time          TIMESTAMPTZ,
  
  node_count        INT,
  edge_count        INT,
  path_count        INT,
  
  duration_ms       BIGINT NOT NULL,
  cache_hit         BOOLEAN NOT NULL DEFAULT FALSE,
  
  status            TEXT NOT NULL,
  error_message     TEXT,
  
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_graph_query_history_tenant_time 
  ON graph_query_history (tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_graph_query_history_type_status 
  ON graph_query_history (query_type, status, created_at DESC);

-- =========================================================================================
-- 系统配置表
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- system_config: 系统配置表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS system_config (
  config_key     TEXT PRIMARY KEY,
  config_value   JSONB NOT NULL,
  description    TEXT,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------------------
-- tenant_config: 租户配置表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tenant_config (
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  config_key     TEXT NOT NULL,
  config_value   JSONB NOT NULL,
  description    TEXT,
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, config_key)
);

-- =========================================================================================
-- 触发器：自动更新 updated_at
-- =========================================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 为所有包含 updated_at 的表创建触发器
DO $$
DECLARE
  t TEXT;
BEGIN
	FOR t IN
	  SELECT columns.table_name
	  FROM information_schema.columns AS columns
	  JOIN information_schema.tables AS tables
	    ON tables.table_schema = columns.table_schema
	   AND tables.table_name = columns.table_name
	  WHERE columns.table_schema = 'public'
	    AND columns.column_name = 'updated_at'
	    AND columns.table_name NOT LIKE '%_old'
	    AND tables.table_type = 'BASE TABLE'
  LOOP
    EXECUTE format('
      DROP TRIGGER IF EXISTS trigger_update_%I_updated_at ON %I;
      CREATE TRIGGER trigger_update_%I_updated_at
      BEFORE UPDATE ON %I
      FOR EACH ROW
      EXECUTE FUNCTION update_updated_at_column();
    ', t, t, t, t);
  END LOOP;
END;
$$ LANGUAGE plpgsql;

-- =========================================================================================
-- 认证服务专用触发器
-- =========================================================================================

CREATE OR REPLACE FUNCTION mark_expired_tokens()
RETURNS TRIGGER AS $$
BEGIN
  IF NEW.expires_at IS NOT NULL AND NEW.expires_at <= NOW() THEN
    NEW.status = 'expired';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS api_tokens_check_expiry ON api_tokens;
CREATE TRIGGER api_tokens_check_expiry
  BEFORE INSERT OR UPDATE ON api_tokens
  FOR EACH ROW
  EXECUTE FUNCTION mark_expired_tokens();

-- =========================================================================================
-- Graph Service 触发器
-- =========================================================================================

CREATE OR REPLACE FUNCTION update_graph_config_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_graph_cache_config_updated_at ON graph_cache_config;
CREATE TRIGGER trigger_graph_cache_config_updated_at
  BEFORE UPDATE ON graph_cache_config
  FOR EACH ROW
  EXECUTE FUNCTION update_graph_config_updated_at();

DROP TRIGGER IF EXISTS trigger_graph_query_config_updated_at ON graph_query_config;
CREATE TRIGGER trigger_graph_query_config_updated_at
  BEFORE UPDATE ON graph_query_config
  FOR EACH ROW
  EXECUTE FUNCTION update_graph_config_updated_at();

-- =========================================================================================
-- 初始化数据
-- =========================================================================================

-- 插入系统租户
INSERT INTO tenants (tenant_id, name, status, created_at, updated_at)
VALUES ('system', 'System Tenant', 'active', now(), now())
ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO tenants (tenant_id, name, status, created_at, updated_at)
VALUES ('default', 'Default Tenant', 'active', now(), now())
ON CONFLICT (tenant_id) DO NOTHING;

-- 插入系统用户
INSERT INTO users (user_id, tenant_id, username, email, status, created_at, updated_at)
VALUES (
  '00000000-0000-0000-0000-000000000001'::uuid,
  'system',
  'system',
  'system@localhost',
  'active',
  now(),
  now()
)
ON CONFLICT (user_id) DO NOTHING;

-- 插入系统角色
INSERT INTO roles (tenant_id, name, description, permissions, is_system)
VALUES
  ('default', 'admin', 'Administrator with full access', '{"*": true}'::jsonb, true),
  ('default', 'analyst', 'Security analyst with read/write alerts', '{"alerts:read": true, "alerts:write": true, "pcap:read": true}'::jsonb, true),
  ('default', 'viewer', 'Read-only access', '{"alerts:read": true, "dashboards:read": true}'::jsonb, true)
ON CONFLICT (tenant_id, name) DO NOTHING;

-- 资产台账验收数据：五类资产各 10 条，使用稳定主键，可重复执行。
-- 默认不写入任何验收资产；仅在显式设置
--   SET traffic.enable_asset_acceptance_fixture = 'on';
-- 后执行，避免生产初始化污染 default 租户或重写验收资产历史。
DO $asset_inventory_acceptance_fixture$
BEGIN
IF COALESCE(current_setting('traffic.enable_asset_acceptance_fixture', true), 'off') NOT IN ('on', 'true', '1') THEN
  RAISE NOTICE 'asset inventory acceptance fixture is disabled';
  RETURN;
END IF;

WITH asset_types(asset_type, prefix, type_ord, department, campus, owner, os_type) AS (
  VALUES
    ('endpoint', 'END', 1, '教务处', '主园区', '终端运维组', 'Windows 11'),
    ('server', 'SRV', 2, '计算中心', '实验楼', '平台运维组', 'Ubuntu 22.04'),
    ('network-device', 'NET', 3, '网络中心', '主园区', '网络运维组', 'Network OS'),
    ('business-system', 'BIZ', 4, '信息化办公室', '数据中心', '应用保障组', 'Application'),
    ('unknown', 'UNK', 5, '', '待确认区', '', 'Unknown')
),
fixture AS (
  SELECT
    (
      substr(md5(asset_type || '-' || n::text), 1, 8) || '-' ||
      substr(md5(asset_type || '-' || n::text), 9, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 13, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 17, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 21, 12)
    )::uuid AS asset_id,
    prefix || '-' || lpad(n::text, 4, '0') AS display_code,
    asset_type,
    CASE WHEN n = 10 THEN 'inactive' WHEN asset_type = 'unknown' THEN 'unknown' ELSE 'active' END AS status,
    format('10.12.%s.%s', type_ord, n + 10) AS ip_address,
    format('02:00:00:00:%s:%s', lpad(type_ord::text, 2, '0'), lpad(n::text, 2, '0')) AS mac_address,
    upper(replace(asset_type, '-', '_')) || '-' || lpad(n::text, 2, '0') AS hostname,
    CASE WHEN asset_type = 'network-device' THEN 'Huawei' WHEN asset_type = 'endpoint' THEN 'Lenovo' ELSE 'OpenStack' END AS vendor,
    os_type,
    department,
    campus,
    owner,
    CASE WHEN n IN (3, 7) THEN 90 WHEN n IN (2, 6) THEN 60 ELSE 30 END AS criticality,
    n
  FROM asset_types CROSS JOIN generate_series(1, 10) AS n
)
INSERT INTO assets (
  asset_id, display_code, tenant_id, asset_type, status, ip_address, mac_address,
  hostname, vendor, os_type, source, department, campus, owner, criticality,
  tags, metadata, first_seen, last_seen
)
SELECT
  asset_id, display_code, 'default', asset_type, status, ip_address, mac_address,
  hostname, vendor, os_type, 'acceptance-fixture', NULLIF(department, ''), campus,
  NULLIF(owner, ''), criticality,
  jsonb_build_object('fixture', 'asset-inventory-v3', 'environment', 'acceptance'),
  jsonb_build_object('seed_index', n, 'data_contract', 'canonical-asset-v1'),
  now() - (n + 30) * interval '1 day',
  now() - n * interval '17 minutes'
FROM fixture
ON CONFLICT (asset_id) DO UPDATE SET
  display_code = EXCLUDED.display_code,
  asset_type = EXCLUDED.asset_type,
  status = EXCLUDED.status,
  ip_address = EXCLUDED.ip_address,
  mac_address = EXCLUDED.mac_address,
  hostname = EXCLUDED.hostname,
  vendor = EXCLUDED.vendor,
  os_type = EXCLUDED.os_type,
  source = EXCLUDED.source,
  department = EXCLUDED.department,
  campus = EXCLUDED.campus,
  owner = EXCLUDED.owner,
  criticality = EXCLUDED.criticality,
  tags = EXCLUDED.tags,
  metadata = EXCLUDED.metadata,
  last_seen = EXCLUDED.last_seen;

-- 服务器详情验收数据：作为资产记录的持久化观测上下文，由 /details 子资源读取。
UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract', 'canonical-asset-detail-v1',
  'network_interfaces', jsonb_build_array(
    jsonb_build_object('name','eth0','adapter','VirtIO Network','ip_address',ip_address,'mac_address',mac_address,'vlan_id','120','mirror_mode','no','status','up','speed','10G','duplex','full','ingress_bytes',13743895347,'egress_bytes',6871947673,'packet_loss_pct',0.02,'error_count',12,'probe_id','probe-12'),
    jsonb_build_object('name','eth1','adapter','VirtIO Network','ip_address','','mac_address','','vlan_id','121','mirror_mode','no','status','up','speed','10G','duplex','full','ingress_bytes',4402341478,'egress_bytes',2362232012,'packet_loss_pct',0.01,'error_count',6,'probe_id','probe-12'),
    jsonb_build_object('name','bond0','adapter','Bond active-backup','ip_address',ip_address,'mac_address',mac_address,'vlan_id','200','mirror_mode','both','status','monitor','speed','20G','duplex','full','ingress_bytes',19542101196,'egress_bytes',10307921510,'packet_loss_pct',0.03,'error_count',21,'probe_id','probe-12'),
    jsonb_build_object('name','eth2','adapter','Mellanox CX5','ip_address','','mac_address','','vlan_id','300','mirror_mode','ingress','status','monitor','speed','40G','duplex','full','ingress_bytes',3006477107,'egress_bytes',0,'packet_loss_pct',0.00,'error_count',0,'probe_id','probe-14'),
    jsonb_build_object('name','eth3','adapter','Intel I350','ip_address','','mac_address','','vlan_id','30','mirror_mode','no','status','down','speed','1G','duplex','full','ingress_bytes',0,'egress_bytes',0,'packet_loss_pct',100.0,'error_count',68,'probe_id','probe-12'),
    jsonb_build_object('name','mgmt0','adapter','Intel I210','ip_address',ip_address,'mac_address',mac_address,'vlan_id','10','mirror_mode','no','status','up','speed','1G','duplex','full','ingress_bytes',335544320,'egress_bytes',188743680,'packet_loss_pct',0.01,'error_count',2,'probe_id','probe-12')
  ),
  'open_services', jsonb_build_array(
    jsonb_build_object('port',22,'protocol','TCP','service','SSH','version','OpenSSH 9.6p1','exposure_scope','内网+外网','access_source_count',18,'risk_level','高危','alert_count',5),
    jsonb_build_object('port',80,'protocol','TCP','service','HTTP','version','Nginx 1.20.1','exposure_scope','外网','access_source_count',9,'risk_level','中危','alert_count',3),
    jsonb_build_object('port',443,'protocol','TCP','service','HTTPS','version','Nginx 1.20.1','exposure_scope','外网','access_source_count',23,'risk_level','中危','alert_count',6),
    jsonb_build_object('port',3306,'protocol','TCP','service','MySQL','version','8.0.32','exposure_scope','内网','access_source_count',6,'risk_level','高危','alert_count',4),
    jsonb_build_object('port',6379,'protocol','TCP','service','Redis','version','6.2.6','exposure_scope','内网','access_source_count',5,'risk_level','高危','alert_count',2),
    jsonb_build_object('port',9200,'protocol','TCP','service','OpenSearch','version','2.11.0','exposure_scope','内网+外网','access_source_count',7,'risk_level','中危','alert_count',3),
    jsonb_build_object('port',9092,'protocol','TCP','service','Kafka','version','3.4.0','exposure_scope','内网','access_source_count',4,'risk_level','中危','alert_count',1)
  ),
  'ownership', jsonb_build_object(
    'campus',campus,'department',department,'owner',owner,
    'responsibilities',jsonb_build_array(
      jsonb_build_object('role','资产管理员','owner',owner,'status','已确认'),
      jsonb_build_object('role','安全复核','owner','sec_manager','status','已确认'),
      jsonb_build_object('role','业务确认','owner','应用保障组','status','已确认'),
      jsonb_build_object('role','取证审批','owner','合规团队','status','待审批')
    ),
    'business_systems',jsonb_build_array(
      jsonb_build_object('name','教学管理系统','role','核心/承载系统','owner','应用保障组','status','已确认'),
      jsonb_build_object('name','统一身份认证','role','认证依赖','owner','平台运维组','status','已确认'),
      jsonb_build_object('name','文件存储系统','role','数据存储依赖','owner','存储运维组','status','已确认')
    ),
    'asset_groups',jsonb_build_array(
      jsonb_build_object('name','计算服务器组','role','核心','owner',owner,'status','已确认'),
      jsonb_build_object('name','数据库服务器组','role','重要','owner',owner,'status','已确认')
    ),
    'data_domains',jsonb_build_array(
      jsonb_build_object('name','教学业务域','role','重要数据','owner',owner,'status','已确认'),
      jsonb_build_object('name','运维日志域','role','运维数据','owner',owner,'status','已确认'),
      jsonb_build_object('name','用户行为域','role','行为数据','owner',owner,'status','待确认')
    ),
    'pending_fields',jsonb_build_array('上级业务系统','数据资产标签')
  )
)
WHERE tenant_id = 'default'
  AND tags->>'fixture' = 'asset-inventory-v3'
  AND asset_type = 'server';

-- 分类 Tab 的持久化业务上下文：列表下方的画像、拓扑、依赖与研判模块均读取这些字段，
-- 禁止前端用视觉常量替代生产 API 数据。
UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'risk_score', CASE WHEN criticality>=80 THEN 86 WHEN criticality>=50 THEN 64 ELSE 28 END,
  'traffic_profile', jsonb_build_array(34,46,29,51,39,58,47,64,38,53,44,61,48,70,56,63),
  'traffic_outbound', jsonb_build_array(22,31,25,36,28,41,33,49,27,38,35,46,32,51,39,45),
  'traffic_east_west', jsonb_build_array(16,24,19,31,22,35,28,42,21,33,26,38,25,44,31,37),
  'traffic_time_labels', jsonb_build_array('03:00','04:30','06:00','07:30','09:00','10:30','12:00','13:30','15:00','16:30','18:00','19:30','21:00','22:30','00:00','01:30'),
  'protocol_total_throughput', '68.4 Gbps',
  'protocols', jsonb_build_array(
    jsonb_build_object('name','TCP','percent',52.6), jsonb_build_object('name','HTTP/HTTPS','percent',21.8),
    jsonb_build_object('name','SMB','percent',8.7), jsonb_build_object('name','DNS','percent',7.1),
    jsonb_build_object('name','MySQL','percent',4.0), jsonb_build_object('name','SSH','percent',2.7),
    jsonb_build_object('name','其他','percent',3.1)
  ),
  'top_peers', jsonb_build_array(
    jsonb_build_object('name','10.12.2.11','type','服务器','share',18.8),
    jsonb_build_object('name','10.12.3.11','type','网络设备','share',12.4),
    jsonb_build_object('name','10.12.4.11','type','业务系统','share',9.6),
    jsonb_build_object('name','10.12.5.11','type','数据库','share',6.3),
    jsonb_build_object('name','8.8.8.8','type','DNS','share',4.2)
  ),
  'periodic_activity', jsonb_build_array(
    22,31,18,36,28,42,34,49,27,38,35,46,29,41,
    18,26,16,32,25,38,31,45,24,35,32,43,27,39,
    20,29,17,34,27,40,33,47,26,37,34,44,28,40,
    21,30,18,35,28,41,35,48,27,39,36,45,29,42,
    19,28,16,33,26,39,32,46,25,36,33,42,27,38,
    15,22,14,27,21,32,26,38,20,30,28,36,23,34,
    14,21,13,26,20,31,25,37,19,29,27,35,22,33
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','暴露端口','value',12,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高风险','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','弱口令','value',2,'max',8,'color','#ffb020'),
    jsonb_build_object('label','异常外联','value',3,'max',20,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',23,'session',186,'dns',342,'tls',97,'alerts',9,'config',3)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='endpoint';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'risk_score', CASE WHEN criticality>=80 THEN 88 WHEN criticality>=50 THEN 67 ELSE 31 END,
  'topology_nodes', jsonb_build_array('教学管理系统','API 服务','MySQL 数据库','Redis 缓存','Kafka 消息','probe-12','取证中心'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','teaching-system','label','教学管理系统','kind','business-system','status','healthy'),
      jsonb_build_object('id','api-service','label','API 服务','kind','service','status','healthy'),
      jsonb_build_object('id','mysql-db','label','MySQL 数据库','kind','database','status','warning'),
      jsonb_build_object('id','redis-cache','label','Redis 缓存','kind','cache','status','healthy'),
      jsonb_build_object('id','kafka-bus','label','Kafka 消息','kind','message-bus','status','healthy'),
      jsonb_build_object('id','probe-12','label','probe-12','kind','probe','status','observed'),
      jsonb_build_object('id','forensics-center','label','取证中心','kind','security','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','srv-hosts-business','source','self','target','teaching-system','relationship','hosts','direction','directed','health','healthy'),
      jsonb_build_object('id','srv-serves-api','source','self','target','api-service','relationship','serves','direction','directed','health','healthy'),
      jsonb_build_object('id','srv-reads-mysql','source','self','target','mysql-db','relationship','reads_writes','direction','bidirectional','health','warning'),
      jsonb_build_object('id','srv-uses-redis','source','self','target','redis-cache','relationship','caches','direction','bidirectional','health','healthy'),
      jsonb_build_object('id','srv-publishes-kafka','source','self','target','kafka-bus','relationship','publishes','direction','directed','health','healthy'),
      jsonb_build_object('id','probe-observes-srv','source','probe-12','target','self','relationship','observes','direction','directed','health','healthy'),
      jsonb_build_object('id','forensics-collects-srv','source','self','target','forensics-center','relationship','evidence_to','direction','directed','health','healthy')
    )
  ),
  'probe_state', jsonb_build_array(
    jsonb_build_object('label',os_type,'value','在线','status','健康'),
    jsonb_build_object('label','probe-12','value','已接入','status','在线'),
    jsonb_build_object('label','接口丢包','value','0.03%','status','健康'),
    jsonb_build_object('label','服务告警','value','9','status','告警')
  ),
  'os_distribution', jsonb_build_array(
    jsonb_build_object('name','Linux','count',68,'color','#1688ff'),
    jsonb_build_object('name','Windows Server','count',18,'color','#39c978'),
    jsonb_build_object('name','Unix','count',8,'color','#ffb020'),
    jsonb_build_object('name','其他','count',6,'color','#7a8ff5')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','暴露端口','value',7,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高危服务','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','弱口令','value',1,'max',8,'color','#ffb020'),
    jsonb_build_object('label','关联告警','value',9,'max',30,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',42,'session',186,'dns',128,'tls',53,'alerts',9,'config',4)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='server';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-network-device-v1',
  'risk_score', CASE WHEN criticality>=80 THEN 84 WHEN criticality>=50 THEN 61 ELSE 26 END,
  'network_interfaces', (
    SELECT jsonb_agg(jsonb_build_object(
      'name', 'GE0/' || port_no,
      'status', CASE WHEN port_no = 3 THEN 'error' WHEN port_no = 48 THEN 'down' ELSE 'up' END,
      'speed', '10G',
      'mirror_mode', CASE WHEN port_no IN (1,2) THEN 'source' WHEN port_no = 4 THEN 'rspan' WHEN port_no IN (47,48) THEN 'remote' ELSE 'no' END,
      'probe_id', CASE WHEN port_no IN (1,4) THEN 'probe-12' WHEN port_no = 2 THEN 'probe-13' WHEN port_no IN (47,48) THEN 'probe-09' ELSE '' END
    ) ORDER BY port_no) FROM generate_series(1,48) AS ports(port_no)
  ),
  'topology_nodes', jsonb_build_array('边界路由器','汇聚交换机','接入交换机','园区防火墙','probe-12','服务器网段'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','edge-router','label','边界路由器','kind','router','status','healthy'),
      jsonb_build_object('id','aggregation-switch','label','汇聚交换机','kind','switch','status','healthy'),
      jsonb_build_object('id','access-switch','label','接入交换机','kind','switch','status','healthy'),
      jsonb_build_object('id','campus-firewall','label','园区防火墙','kind','firewall','status','warning'),
      jsonb_build_object('id','probe-12','label','probe-12','kind','probe','status','observed'),
      jsonb_build_object('id','server-segment','label','服务器网段','kind','subnet','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','router-uplink','source','edge-router','target','self','relationship','uplink','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','aggregation-link','source','aggregation-switch','target','self','relationship','trunk','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','access-link','source','self','target','access-switch','relationship','downlink','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','firewall-link','source','self','target','campus-firewall','relationship','security_path','direction','bidirectional','protocol','LLDP','health','warning'),
      jsonb_build_object('id','probe-mirror','source','self','target','probe-12','relationship','mirror_to','direction','directed','protocol','SPAN','health','healthy'),
      jsonb_build_object('id','server-segment-link','source','self','target','server-segment','relationship','serves_subnet','direction','bidirectional','protocol','VLAN','health','healthy')
    )
  ),
  'mirror_links', jsonb_build_array(
    jsonb_build_object('interface','GE0/1','direction','Out','mode','SPAN Source','target','probe-12','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/2','direction','Out','mode','SPAN Source','target','probe-13','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/4','direction','Out','mode','RSPAN VLAN 300','target','probe-13','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/48','direction','Out','mode','远程镜像','target','FW-01:mirror','bandwidth','10G','status','异常'),
    jsonb_build_object('interface','GE0/47','direction','In','mode','远程镜像','target','probe-09','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/12','direction','Both','mode','ERSPAN','target','probe-08','bandwidth','10G','status','在线')
  ),
  'config_changes', jsonb_build_array(
    jsonb_build_object('time','06-20 02:12','actor','admin','change','接口配置','risk','中'),
    jsonb_build_object('time','06-19 21:35','actor','netops','change','VLAN 变更','risk','中'),
    jsonb_build_object('time','06-19 18:06','actor','admin','change','ACL 规则','risk','高'),
    jsonb_build_object('time','06-18 16:42','actor','netops','change','端口关闭','risk','高'),
    jsonb_build_object('time','06-18 09:18','actor','audit','change','镜像策略','risk','低')
  ),
  'business_impacts', jsonb_build_array(
    jsonb_build_object('name','教学管理系统','links',12,'traffic','32.6%','risk','高'),
    jsonb_build_object('name','科研数据平台','links',9,'traffic','24.1%','risk','高'),
    jsonb_build_object('name','统一认证平台','links',6,'traffic','15.8%','risk','中'),
    jsonb_build_object('name','图书馆门户','links',4,'traffic','9.6%','risk','中'),
    jsonb_build_object('name','财务结算系统','links',3,'traffic','6.1%','risk','低')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','接口总数','value',48,'max',48,'color','#1688ff'),
    jsonb_build_object('label','Up 接口','value',46,'max',48,'color','#39c978'),
    jsonb_build_object('label','Err-Disable','value',1,'max',8,'color','#ffb020'),
    jsonb_build_object('label','Down 接口','value',1,'max',8,'color','#ff4d4f')
  ),
  'evidence', jsonb_build_object('pcap',32,'session',128,'dns',86,'tls',53,'alerts',19,'config',8)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='network-device';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-business-system-v1',
  'risk_score', CASE WHEN criticality>=80 THEN 86 WHEN criticality>=50 THEN 65 ELSE 29 END,
  'business_domain','教学教务','system_level','核心','sla_target','99.5%','sla_current','99.2%',
  'topology_nodes', jsonb_build_array('统一认证平台','数据库集群','科研数据平台','图书馆门户','财务结算系统','消息队列'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','sso-platform','label','统一认证平台','kind','business-system','status','healthy'),
      jsonb_build_object('id','database-cluster','label','数据库集群','kind','database','status','warning'),
      jsonb_build_object('id','research-platform','label','科研数据平台','kind','business-system','status','healthy'),
      jsonb_build_object('id','library-portal','label','图书馆门户','kind','business-system','status','healthy'),
      jsonb_build_object('id','finance-system','label','财务结算系统','kind','business-system','status','healthy'),
      jsonb_build_object('id','message-queue','label','消息队列','kind','message-bus','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','biz-depends-sso','source','self','target','sso-platform','relationship','depends_on','direction','directed','health','healthy'),
      jsonb_build_object('id','biz-reads-db','source','self','target','database-cluster','relationship','reads_writes','direction','bidirectional','health','warning'),
      jsonb_build_object('id','research-calls-biz','source','research-platform','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','library-calls-biz','source','library-portal','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','finance-calls-biz','source','finance-system','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','biz-publishes-queue','source','self','target','message-queue','relationship','publishes','direction','directed','health','healthy')
    )
  ),
  'risk_factors', jsonb_build_array(
    jsonb_build_object('name','漏洞暴露','percent',28), jsonb_build_object('name','异常外联','percent',24),
    jsonb_build_object('name','高危服务','percent',20), jsonb_build_object('name','证据缺口','percent',16)
  ),
  'risk_distribution', jsonb_build_array(
    jsonb_build_object('range','90-100 高风险','count',21,'percent_label','21  14.4%','color','#ff4d4f'),
    jsonb_build_object('range','70-89 较高风险','count',45,'percent_label','45  30.8%','color','#ff8a34'),
    jsonb_build_object('range','40-69 中风险','count',54,'percent_label','54  37.0%','color','#f2c94c'),
    jsonb_build_object('range','20-39 较低风险','count',18,'percent_label','18  12.3%','color','#39c978'),
    jsonb_build_object('range','0-19 低风险','count',8,'percent_label','8  5.5%','color','#1688ff')
  ),
  'key_services', jsonb_build_array(
    jsonb_build_object('name','Web Portal','endpoint','443/TCP','dependency','统一认证平台','risk','高危','health','降级'),
    jsonb_build_object('name','教务 API','endpoint','8080/TCP','dependency','数据库集群','risk','高危','health','健康'),
    jsonb_build_object('name','数据库服务','endpoint','5432/TCP','dependency','数据库集群','risk','高危','health','健康'),
    jsonb_build_object('name','消息队列','endpoint','5672/TCP','dependency','科研数据平台','risk','中危','health','健康'),
    jsonb_build_object('name','缓存服务','endpoint','6379/TCP','dependency','Redis 集群','risk','低危','health','健康')
  ),
  'dependency_health', jsonb_build_array(
    jsonb_build_object('type','服务器','total',86,'abnormal',2,'health','98.0%'),
    jsonb_build_object('type','数据库','total',12,'abnormal',1,'health','95.8%'),
    jsonb_build_object('type','存储设备','total',6,'abnormal',1,'health','97.2%'),
    jsonb_build_object('type','网络设备','total',18,'abnormal',0,'health','99.1%')
  ),
  'responsibility', jsonb_build_array(
    jsonb_build_object('department','教务处','role','主管部门','owner','张老师','sla','99.5%','status','正常'),
    jsonb_build_object('department','信息中心','role','技术支撑','owner','李老师','sla','99.0%','status','正常'),
    jsonb_build_object('department','运维团队','role','运维保障','owner','王老师','sla','99.0%','status','正常'),
    jsonb_build_object('department','安全中心','role','安全治理','owner','赵老师','sla','98.5%','status','正常')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','风险评分','value',86,'max',100,'color','#ff4d4f'),
    jsonb_build_object('label','依赖资产','value',186,'max',220,'color','#1688ff'),
    jsonb_build_object('label','关键服务','value',24,'max',30,'color','#7a8ff5'),
    jsonb_build_object('label','高风险服务','value',12,'max',20,'color','#ff8a34')
  ),
  'evidence', jsonb_build_object('pcap',42,'session',186,'dns',128,'tls',53,'alerts',9,'config',6)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='business-system';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-unknown-asset-v1','risk_score',49,'suspected_type','未知识别','confidence',42,'ticket_status','待确认',
  'discovery_timeline', jsonb_build_array(
    jsonb_build_object('event','流量探针首次发现','time','06-19 21:33','status','已完成'),
    jsonb_build_object('event','ARP 绑定采集','time','06-19 21:36','status','已完成'),
    jsonb_build_object('event','DNS/TLS 指纹聚合','time','06-20 03:41','status','已完成'),
    jsonb_build_object('event','归属候选匹配','time','06-20 03:44','status','待复核')
  ),
  'discovery_activity', jsonb_build_object(
    'labels',jsonb_build_array('00:00','02:00','04:00','06:00','08:00','10:00','12:00','14:00','16:00','18:00','20:00','22:00'),
    'discovered',jsonb_build_array(4,6,5,8,12,18,15,21,16,11,8,4),
    'pending_rate',jsonb_build_array(72,69,66,63,61,58,55,52,49,47,45,42)
  ),
  'device_profile_distribution', jsonb_build_array(
    jsonb_build_object('name','Windows 终端','count',38,'color','#1688ff'),
    jsonb_build_object('name','Linux 主机','count',26,'color','#39c978'),
    jsonb_build_object('name','IoT 设备','count',22,'color','#ffb020'),
    jsonb_build_object('name','网络设备','count',18,'color','#7a8ff5'),
    jsonb_build_object('name','移动终端','count',14,'color','#27b8e6'),
    jsonb_build_object('name','其他','count',10,'color','#8a9aaa')
  ),
  'fingerprint', jsonb_build_object('mac_oui','Intel X710','dhcp_hostname',hostname,'ttl_os','128 / Windows 10/11','open_ports','135, 445, 5985, 3389','ja3','ja3_72d8b9a3c1f2','behavior','频繁访问内网文件共享'),
  'ownership_candidates', jsonb_build_array(
    jsonb_build_object('department','计算中心','owner','张老师','matched','15','confidence','72%'),
    jsonb_build_object('department','信息中心','owner','李老师','matched','12','confidence','65%'),
    jsonb_build_object('department','实验室','owner','王老师','matched','8','confidence','58%'),
    jsonb_build_object('department','图书馆','owner','赵老师','matched','6','confidence','46%'),
    jsonb_build_object('department','后勤中心','owner','陈老师','matched','4','confidence','38%')
  ),
  'exposure', jsonb_build_object('open_ports',7,'high_services',3,'weak_password',1,'related_alerts',5,'risk_score',49),
  'risk_distribution', jsonb_build_array(
    jsonb_build_object('name','高风险','count',22,'color','#ff4d4f'),
    jsonb_build_object('name','中风险','count',47,'color','#ffb020'),
    jsonb_build_object('name','低风险','count',59,'color','#39c978')
  ),
  'ticket_steps', jsonb_build_array('发现 / 已完成','归属确认 / 待处理','风险复核 / 待处理','验证 / 未开始','关闭 / 未开始'),
  'source_distribution', jsonb_build_array(
    jsonb_build_object('name','流量探针','count',62), jsonb_build_object('name','DHCP 日志','count',34),
    jsonb_build_object('name','ARP 扫描','count',18), jsonb_build_object('name','终端扫描','count',9),
    jsonb_build_object('name','其他','count',5)
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','风险评分','value',49,'max',100,'color','#ffb020'),
    jsonb_build_object('label','暴露端口','value',7,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高危服务','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','关联告警','value',5,'max',30,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',68,'session',156,'dns',342,'tls',97,'alerts',19,'config',8)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='unknown';

DELETE FROM asset_events
WHERE asset_id IN (
  SELECT asset_id FROM assets
  WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3'
);

INSERT INTO asset_events (asset_id, tenant_id, event_type, old_value, new_value, created_at)
SELECT asset_id, tenant_id, 'asset.discovered', '{}'::jsonb,
       jsonb_build_object('display_code', display_code, 'asset_type', asset_type),
       first_seen
FROM assets
WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3'
UNION ALL
SELECT asset_id, tenant_id, 'asset.governance.updated',
       jsonb_build_object('owner', NULL),
       jsonb_build_object('owner', owner, 'department', department, 'campus', campus),
       last_seen
FROM assets
WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3';

-- 跨页取证验收必须返回真实且按资产范围过滤的任务。该记录仍受上方
-- traffic.enable_asset_acceptance_fixture 显式开关保护，不进入默认生产初始化。
INSERT INTO tasks (
  task_id, tenant_id, name, task_type, params, status, progress,
  result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
  run_id, created_by, created_at, updated_at, started_at, completed_at
)
SELECT
  md5('asset-inventory-v3-pcap-' || asset_id::text)::uuid,
  tenant_id,
  '资产台账取证验收-' || display_code,
  'pcap_cut',
  jsonb_build_object(
    'asset_id', asset_id::text,
    'display_code', display_code,
    'fixture', 'asset-inventory-v3',
    'start_time', (extract(epoch FROM first_seen) * 1000)::bigint,
    'end_time', (extract(epoch FROM last_seen) * 1000)::bigint
  ),
  'completed', 100,
  'acceptance/assets/' || asset_id::text || '/capture.pcap',
  repeat('a', 64), 128, 65536, 1,
  'asset-inventory-v3', 'asset-inventory-acceptance',
  first_seen, last_seen, first_seen, last_seen
FROM assets
WHERE tenant_id = 'default'
  AND tags->>'fixture' = 'asset-inventory-v3'
  AND display_code = 'SRV-0001'
ON CONFLICT (task_id) DO UPDATE SET
  params = EXCLUDED.params,
  status = EXCLUDED.status,
  progress = EXCLUDED.progress,
  result_file_key = EXCLUDED.result_file_key,
  result_sha256 = EXCLUDED.result_sha256,
  result_packets = EXCLUDED.result_packets,
  result_bytes = EXCLUDED.result_bytes,
  files_scanned = EXCLUDED.files_scanned,
  updated_at = EXCLUDED.updated_at,
  completed_at = EXCLUDED.completed_at;

END;
$asset_inventory_acceptance_fixture$;

-- 初始化 Graph Service 配置
INSERT INTO graph_cache_config (tenant_id, neighbor_ttl_sec, entity_ttl_sec, graph_ttl_sec)
SELECT tenant_id, 300, 300, 120
FROM tenants
ON CONFLICT (tenant_id) DO NOTHING;

INSERT INTO graph_query_config (tenant_id, max_depth, default_depth, max_nodes)
SELECT tenant_id, 5, 2, 500
FROM tenants
ON CONFLICT (tenant_id) DO NOTHING;

-- =========================================================================================
-- 视图：简化常用查询
-- =========================================================================================

-- 行为基线治理状态，与集群初始化脚本保持一致。
CREATE TABLE IF NOT EXISTS behavior_baseline_resets (tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, baseline_id TEXT NOT NULL, reset_at TIMESTAMPTZ NOT NULL DEFAULT now(), requested_by TEXT NOT NULL DEFAULT '', PRIMARY KEY (tenant_id, baseline_id));
CREATE TABLE IF NOT EXISTS behavior_baseline_settings (tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, baseline_id TEXT NOT NULL, warning_multiplier DOUBLE PRECISION NOT NULL DEFAULT 2.0 CHECK (warning_multiplier > 0), alert_multiplier DOUBLE PRECISION NOT NULL DEFAULT 3.0 CHECK (alert_multiplier > warning_multiplier), frozen BOOLEAN NOT NULL DEFAULT false, drift_watch BOOLEAN NOT NULL DEFAULT false, version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0), updated_by TEXT NOT NULL DEFAULT '', updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, baseline_id));
CREATE TABLE IF NOT EXISTS behavior_baseline_actions (action_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, baseline_id TEXT NOT NULL, action_type TEXT NOT NULL CHECK (action_type IN ('create_alert','adjust_threshold','freeze','unfreeze','forensics','feedback_model','cold_start','drift_watch','rebuild','rollback','audit_trace')), status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','applied','rejected','failed')), reason TEXT NOT NULL DEFAULT '', request JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_actions_time ON behavior_baseline_actions (tenant_id, baseline_id, created_at DESC);
CREATE TABLE IF NOT EXISTS behavior_baseline_versions (tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, baseline_id TEXT NOT NULL, version INTEGER NOT NULL CHECK (version > 0), snapshot JSONB NOT NULL DEFAULT '{}'::jsonb, source_action_id UUID NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE SET NULL, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id, baseline_id, version));
CREATE TABLE IF NOT EXISTS behavior_baseline_outbox (outbox_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE, baseline_id TEXT NOT NULL, action_id UUID NOT NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE CASCADE, event_type TEXT NOT NULL, payload JSONB NOT NULL DEFAULT '{}'::jsonb, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ NULL);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_outbox_pending ON behavior_baseline_outbox (published, created_at) WHERE published=false;

-- 用户权限视图
CREATE OR REPLACE VIEW user_permissions AS
SELECT
  u.user_id,
  u.tenant_id,
  u.username,
  r.role_id,
  r.name AS role_name,
  r.permissions
FROM users u
JOIN user_roles ur ON u.user_id = ur.user_id
JOIN roles r ON ur.role_id = r.role_id
WHERE u.status = 'active';

-- 活跃部署视图
CREATE OR REPLACE VIEW active_deployments AS
SELECT
  d.deployment_id,
  d.tenant_id,
  d.model_version,
  d.rule_version,
  d.feature_set_id,
  d.status,
  d.scope,
  d.created_at,
  d.updated_at
FROM deployments d
WHERE d.status IN ('gray', 'active');

-- 待处理任务视图
CREATE OR REPLACE VIEW pending_tasks AS
SELECT
  task_id,
  tenant_id,
  task_type,
  params,
  status,
  created_at
FROM tasks
WHERE status = 'queued'
ORDER BY created_at ASC;

-- Graph Service 统计视图
CREATE OR REPLACE VIEW graph_query_stats AS
SELECT
  tenant_id,
  query_type,
  status,
  COUNT(*) AS query_count,
  AVG(duration_ms) AS avg_duration_ms,
  PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95_duration_ms,
  SUM(CASE WHEN cache_hit THEN 1 ELSE 0 END) AS cache_hits,
  SUM(CASE WHEN cache_hit THEN 1 ELSE 0 END)::FLOAT / COUNT(*)::FLOAT AS cache_hit_rate,
  DATE_TRUNC('hour', created_at) AS hour
FROM graph_query_history
WHERE created_at > NOW() - INTERVAL '24 hours'
GROUP BY tenant_id, query_type, status, DATE_TRUNC('hour', created_at);

-- =========================================================================================
-- 注释
-- =========================================================================================

COMMENT ON TABLE rules IS 'Rule definitions with versioning and RBAC support';
COMMENT ON TABLE rule_outbox IS 'Outbox pattern table for reliable Kafka message publishing';
COMMENT ON TABLE deployment_history IS 'Deployment state change history';
COMMENT ON COLUMN rules.version IS 'Optimistic locking version number';
COMMENT ON COLUMN rules.status IS 'Rule status: draft/active/disabled/archived/deleted/pending_sync';
COMMENT ON COLUMN rules.conditions IS 'Rule condition definitions in JSONB format';
COMMENT ON COLUMN deployments.gray_started_at IS 'Gray deployment start time';
COMMENT ON COLUMN deployments.gray_expired_at IS 'Gray deployment expiration time';
COMMENT ON COLUMN rule_outbox.published IS 'Whether the event has been successfully published to Kafka';
COMMENT ON COLUMN rule_outbox.next_retry IS 'Next retry time for failed publications';
COMMENT ON TABLE revoked_sessions IS 'Session 撤销记录，建议每天清理 expires_at < now() - INTERVAL ''7 days'' 的记录';
COMMENT ON TABLE token_rotation_history IS 'Token 轮转历史，建议保留 90 天，超过后可归档到对象存储';
COMMENT ON TABLE token_usage_logs IS 'Token 使用日志，建议使用分区表按月分区，保留 6 个月';
COMMIT;

-- T-PG-002 notification rule atomic command boundary (202608031300).
BEGIN;
ALTER TABLE notification_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_escalation_policies
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE alert_notification_settings
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE notification_silence_rules
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS notification_governance_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  action TEXT NOT NULL,
  snapshot JSONB NOT NULL,
  changed_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, aggregate_type, aggregate_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_history_aggregate
  ON notification_governance_history (tenant_id,aggregate_type,aggregate_id,revision DESC);
CREATE TABLE IF NOT EXISTS notification_governance_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  last_error TEXT NOT NULL DEFAULT '',
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_notification_governance_outbox_ready
  ON notification_governance_outbox (next_retry_at,occurred_at,outbox_id)
  WHERE status IN ('pending','processing');
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
CREATE TABLE IF NOT EXISTS notification_governance_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256)=64),
  action_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES notification_governance_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031300','T-PG-002 notification rule atomic command history outbox idempotency')
ON CONFLICT (version) DO NOTHING;
COMMIT;


-- T-PG-002 / F-ALERT-006 atomic saved-view transaction schema.
BEGIN;
ALTER TABLE alert_saved_views
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS updated_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS alert_saved_view_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key),
  UNIQUE (event_id)
);
CREATE TABLE IF NOT EXISTS alert_saved_view_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL,
  view_id UUID NOT NULL REFERENCES alert_saved_views(view_id) ON DELETE CASCADE,
  revision BIGINT NOT NULL CHECK (revision > 0),
  name TEXT NOT NULL,
  filters JSONB NOT NULL,
  action TEXT NOT NULL,
  changed_by TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, view_id, revision),
  UNIQUE (event_id)
);
CREATE TABLE IF NOT EXISTS alert_saved_view_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  aggregate_type TEXT NOT NULL DEFAULT 'alert_saved_view',
  aggregate_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  tenant_id TEXT NOT NULL,
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL,
  trace_id TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  locked_by TEXT NOT NULL DEFAULT '',
  locked_until TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_alert_saved_view_outbox_ready
  ON alert_saved_view_outbox (next_retry_at, occurred_at, outbox_id)
  WHERE status IN ('pending','processing');
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
CREATE TABLE IF NOT EXISTS threat_intel (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL DEFAULT 'default',
  type TEXT NOT NULL CHECK (type IN ('ip','domain','hash')), value TEXT NOT NULL,
  reputation TEXT NOT NULL DEFAULT 'unknown', category TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'manual', description TEXT NOT NULL DEFAULT '',
  last_seen TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,type,value)
);
CREATE TABLE IF NOT EXISTS threat_intel_feeds (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(), tenant_id TEXT NOT NULL DEFAULT 'default',
  name TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT true,
  interval_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (interval_seconds >= 1),
  entries JSONB NOT NULL DEFAULT '[]'::jsonb, last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_status TEXT NOT NULL DEFAULT 'never',
  last_error TEXT NOT NULL DEFAULT '', run_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,name)
);
CREATE TABLE IF NOT EXISTS threat_intel_event_outbox (
  event_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, partition_key TEXT NOT NULL,
  payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_value ON threat_intel(type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_tenant_value ON threat_intel(tenant_id,type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_rep ON threat_intel(reputation);
CREATE INDEX IF NOT EXISTS idx_threat_intel_source ON threat_intel(source);
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_due ON threat_intel_feeds(enabled,next_run_at);
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_tenant ON threat_intel_feeds(tenant_id,name);
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_pending
  ON threat_intel_event_outbox(available_at,created_at) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_threat_intel_event_outbox_reclaim
  ON threat_intel_event_outbox(locked_until,created_at) WHERE status='processing';
ALTER TABLE threat_intel ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE threat_intel DROP CONSTRAINT IF EXISTS threat_intel_revision_positive;
ALTER TABLE threat_intel ADD CONSTRAINT threat_intel_revision_positive CHECK (revision > 0);
ALTER TABLE threat_intel_feeds ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE threat_intel_feeds DROP CONSTRAINT IF EXISTS threat_intel_feeds_revision_positive;
ALTER TABLE threat_intel_feeds ADD CONSTRAINT threat_intel_feeds_revision_positive CHECK (revision > 0);
CREATE TABLE IF NOT EXISTS threat_intel_command_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('entry','feed')),
  aggregate_id TEXT NOT NULL, revision BIGINT NOT NULL CHECK (revision > 0),
  action_id TEXT NOT NULL, operation TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, snapshot JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,aggregate_type,aggregate_id,revision),
  UNIQUE (event_id,aggregate_type,aggregate_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_history_lookup
  ON threat_intel_command_history (tenant_id,aggregate_type,aggregate_id,revision DESC);
CREATE TABLE IF NOT EXISTS threat_intel_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL, command_type TEXT NOT NULL,
  expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id TEXT NOT NULL REFERENCES threat_intel_event_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_threat_intel_command_requests_created
  ON threat_intel_command_requests (tenant_id,created_at DESC);
CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031100', 'alert saved view atomic history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031540', 'authenticated probe registration revision history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031550', 'threat intel revision history audit outbox and durable command replay')
ON CONFLICT (version) DO NOTHING;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS last_action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS last_trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_revision_positive;
ALTER TABLE tasks ADD CONSTRAINT tasks_revision_positive CHECK (revision > 0);
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_progress_range;
ALTER TABLE tasks ADD CONSTRAINT tasks_progress_range CHECK (progress BETWEEN 0 AND 100);
CREATE TABLE IF NOT EXISTS forensics_task_history (
  history_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0), action_id TEXT NOT NULL,
  operation TEXT NOT NULL, previous_status TEXT NOT NULL DEFAULT '', resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, snapshot JSONB NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,task_id,revision), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_history_lookup
  ON forensics_task_history (tenant_id,task_id,revision DESC);
CREATE TABLE IF NOT EXISTS forensics_task_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''), payload JSONB NOT NULL,
  trace_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_outbox_ready
  ON forensics_task_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_forensics_task_outbox_reclaim
  ON forensics_task_outbox (locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS forensics_task_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL, operation TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES forensics_task_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_requests_task
  ON forensics_task_requests (tenant_id,task_id,created_at DESC);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608031600', 'forensics task revision history audit outbox idempotency and soft retention')
ON CONFLICT (version) DO NOTHING;
COMMIT;

BEGIN;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS last_action_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist ADD COLUMN IF NOT EXISTS last_trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE whitelist DROP CONSTRAINT IF EXISTS whitelist_version_positive;
ALTER TABLE whitelist ADD CONSTRAINT whitelist_version_positive CHECK (version > 0);
CREATE INDEX IF NOT EXISTS idx_whitelist_visible_updated
  ON whitelist (tenant_id,updated_at DESC,id) WHERE archived_at IS NULL;
CREATE TABLE IF NOT EXISTS whitelist_entry_versions (
  history_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  version BIGINT NOT NULL CHECK (version > 0), action_id TEXT NOT NULL,
  operation TEXT NOT NULL, actor_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL,
  trace_id TEXT NOT NULL, previous_status TEXT NOT NULL DEFAULT '', resulting_status TEXT NOT NULL,
  snapshot JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,entry_id,version)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_entry_versions_lookup
  ON whitelist_entry_versions (tenant_id,entry_id,version DESC);
CREATE TABLE IF NOT EXISTS whitelist_event_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL, entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version = 2),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''), payload JSONB NOT NULL,
  trace_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_whitelist_event_outbox_ready
  ON whitelist_event_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_whitelist_event_outbox_reclaim
  ON whitelist_event_outbox (locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS whitelist_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64), action_id TEXT NOT NULL,
  operation TEXT NOT NULL, entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  expected_version BIGINT NOT NULL CHECK (expected_version >= 0),
  resulting_version BIGINT NOT NULL CHECK (resulting_version > 0),
  event_id UUID NOT NULL REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  reason TEXT NOT NULL, trace_id TEXT NOT NULL, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_command_requests_entry
  ON whitelist_command_requests (tenant_id,entry_id,created_at DESC);
CREATE TABLE IF NOT EXISTS whitelist_rule_effects (
  tenant_id TEXT NOT NULL, entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  entry_version BIGINT NOT NULL CHECK (entry_version > 0),
  event_id UUID NOT NULL REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  desired_state TEXT NOT NULL CHECK (desired_state IN ('effective','revoked')),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','failed')),
  rule_revision TEXT NOT NULL DEFAULT '', ack_event_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '', requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  acknowledged_at TIMESTAMPTZ, PRIMARY KEY (tenant_id,entry_id,entry_version), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_rule_effects_pending
  ON whitelist_rule_effects (requested_at,entry_id) WHERE status='pending';
CREATE TABLE IF NOT EXISTS whitelist_rule_projection (
  tenant_id TEXT NOT NULL,
  entry_id UUID NOT NULL REFERENCES whitelist(id) ON DELETE RESTRICT,
  entry_version BIGINT NOT NULL CHECK (entry_version > 0),
  source_event_id UUID NOT NULL UNIQUE REFERENCES whitelist_event_outbox(event_id) ON DELETE RESTRICT,
  desired_state TEXT NOT NULL CHECK (desired_state IN ('effective','revoked')),
  entry_type TEXT NOT NULL CHECK (entry_type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
  match_value TEXT NOT NULL CHECK (match_value <> ''), scope TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ, rule_revision TEXT NOT NULL CHECK (length(rule_revision) = 64),
  payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,entry_id)
);
CREATE INDEX IF NOT EXISTS idx_whitelist_rule_projection_effective
  ON whitelist_rule_projection (tenant_id,entry_type,match_value)
  WHERE desired_state='effective';
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031610','whitelist immutable versions audit outbox idempotency rule ACK and soft archive')
ON CONFLICT (version) DO NOTHING;
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608071930','whitelist rule-manager projection and ingestion enforcement')
ON CONFLICT (version) DO NOTHING;
COMMIT;

BEGIN;
CREATE TABLE IF NOT EXISTS dashboard_tasks (
  task_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  action_id TEXT NOT NULL, task_type TEXT NOT NULL, target TEXT NOT NULL,
  priority TEXT NOT NULL CHECK (priority IN ('low','medium','high','critical')),
  status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted','running','completed','partial','failed','cancelled')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0), snapshot_id TEXT NOT NULL,
  reason TEXT NOT NULL, requested_by TEXT NOT NULL, trace_id TEXT NOT NULL,
  input JSONB NOT NULL DEFAULT '{}'::jsonb, result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, cancelled_at TIMESTAMPTZ,
  UNIQUE (tenant_id,task_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_tasks_tenant_status_time
  ON dashboard_tasks (tenant_id,status,updated_at DESC,task_id);
CREATE TABLE IF NOT EXISTS dashboard_task_history (
  history_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0), action_id TEXT NOT NULL,
  previous_status TEXT NOT NULL DEFAULT '', resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL, reason TEXT NOT NULL, trace_id TEXT NOT NULL,
  snapshot JSONB NOT NULL, occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,task_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_history_lookup
  ON dashboard_task_history (tenant_id,task_id,revision DESC);
CREATE TABLE IF NOT EXISTS dashboard_task_outbox (
  outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0), event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''), payload JSONB NOT NULL,
  trace_id TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_outbox_ready
  ON dashboard_task_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_outbox_reclaim
  ON dashboard_task_outbox (locked_until,outbox_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS dashboard_task_requests (
  tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64), action_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  trace_id TEXT NOT NULL, response_payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY (tenant_id,idempotency_key), UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_requests_task
  ON dashboard_task_requests (tenant_id,task_id,created_at DESC);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031620','dashboard durable task history audit outbox and idempotent command receipts')
ON CONFLICT (version) DO NOTHING;
COMMIT;

-- T-OS-004 entrypoint mirror; canonical DDL is deployments/postgres/migrations/202608041100_*.sql.
BEGIN;
CREATE TABLE IF NOT EXISTS alert_opensearch_projection_debts (
  tenant_id TEXT NOT NULL CHECK (tenant_id<>''), alert_id TEXT NOT NULL CHECK (alert_id<>''),
  source_event_id TEXT NOT NULL DEFAULT '', source_version BIGINT NOT NULL CHECK (source_version>0),
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64), target_index_version TEXT NOT NULL CHECK (target_index_version<>''),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','resolved','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count>=0), available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_debts_ready ON alert_opensearch_projection_debts(available_at,first_failed_at,tenant_id,alert_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_debts_reclaim ON alert_opensearch_projection_debts(locked_until,tenant_id,alert_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS alert_opensearch_projection_watermarks (
  tenant_id TEXT NOT NULL CHECK (tenant_id<>''), alert_id TEXT NOT NULL CHECK (alert_id<>''), source_event_id TEXT NOT NULL DEFAULT '',
  source_version BIGINT NOT NULL CHECK (source_version>0), source_sha256 TEXT NOT NULL CHECK (length(source_sha256)=64),
  target_index_version TEXT NOT NULL CHECK (target_index_version<>''), applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE TABLE IF NOT EXISTS alert_opensearch_reconcile_runs (
  run_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL CHECK (tenant_id<>''), requested_by TEXT NOT NULL CHECK (requested_by<>''),
  trace_id TEXT NOT NULL CHECK (trace_id<>''), mode TEXT NOT NULL CHECK (mode IN ('plan','repair')),
  target_index_version TEXT NOT NULL CHECK (target_index_version<>''), start_time TIMESTAMPTZ, end_time TIMESTAMPTZ,
  business_ids JSONB NOT NULL DEFAULT '[]'::jsonb, max_documents INTEGER NOT NULL CHECK (max_documents BETWEEN 1 AND 100000),
  stop_error_count INTEGER NOT NULL CHECK (stop_error_count BETWEEN 1 AND 10000), status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','completed','partial','failed','stopped')),
  source_count BIGINT NOT NULL DEFAULT 0, target_count BIGINT NOT NULL DEFAULT 0, missing_count BIGINT NOT NULL DEFAULT 0,
  extra_count BIGINT NOT NULL DEFAULT 0, stale_count BIGINT NOT NULL DEFAULT 0, repaired_count BIGINT NOT NULL DEFAULT 0,
  error_count BIGINT NOT NULL DEFAULT 0, result_manifest JSONB NOT NULL DEFAULT '{}'::jsonb, stop_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
INSERT INTO alignment_schema_migrations(version,description) VALUES ('202608041100','durable alert OpenSearch projection debt watermarks and bounded reconcile runs') ON CONFLICT (version) DO NOTHING;
COMMIT;

-- BEGIN GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE
-- Source: deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql
-- Do not edit this block directly; run scripts/alignment/sync_data_quality_postgres_entrypoints.py --write.
-- T-DQ-001: persistent, versioned data-quality control plane.
--
-- Expand: add dataset, rule, baseline, watermark, event, repair and outbox tables.
-- Backfill: register datasets explicitly per tenant; do not infer green status from
--           an absent rule, baseline or watermark.
-- Verify: compare rule/baseline versions, real hand-off watermarks and outbox/audit
--         rows before enabling any release-blocking policy.
-- Cutover: first shadow evaluation, then approved per-rule activation.
-- Rollback: disable evaluators and retain all immutable versions and evidence;
--           no destructive table or column rollback is permitted.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_datasets (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  dataset_id TEXT NOT NULL CHECK (dataset_id <> ''),
  display_name TEXT NOT NULL,
  owner TEXT NOT NULL CHECK (owner <> ''),
  schema_version BIGINT NOT NULL CHECK (schema_version > 0),
  signal_contract_version TEXT NOT NULL CHECK (signal_contract_version <> ''),
  business_keys JSONB NOT NULL CHECK (jsonb_typeof(business_keys) = 'array'),
  allowed_lateness_seconds BIGINT NOT NULL CHECK (allowed_lateness_seconds >= 0),
  retention_seconds BIGINT NOT NULL CHECK (retention_seconds > 0),
  upstreams JSONB NOT NULL CHECK (jsonb_typeof(upstreams) = 'array'),
  downstreams JSONB NOT NULL CHECK (jsonb_typeof(downstreams) = 'array'),
  slo_target NUMERIC(8,7) NOT NULL CHECK (slo_target > 0 AND slo_target <= 1),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','paused','retired')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,dataset_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_datasets_owner_status
  ON data_quality_datasets (tenant_id,owner,status,dataset_id);

CREATE TABLE IF NOT EXISTS data_quality_rules (
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_key TEXT NOT NULL CHECK (rule_key <> ''),
  dataset_id TEXT NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  dimension TEXT NOT NULL CHECK (dimension IN (
    'completeness','uniqueness','timeliness','validity','referential_integrity',
    'ordering','duplicate','lateness','tenant_ownership','object_availability'
  )),
  field_path TEXT NOT NULL DEFAULT '',
  predicate JSONB NOT NULL CHECK (jsonb_typeof(predicate) = 'object'),
  threshold JSONB NOT NULL CHECK (jsonb_typeof(threshold) = 'object'),
  window_seconds BIGINT NOT NULL CHECK (window_seconds > 0),
  sampling JSONB NOT NULL CHECK (jsonb_typeof(sampling) = 'object'),
  severity TEXT NOT NULL CHECK (severity IN ('info','warning','high','critical')),
  owner TEXT NOT NULL CHECK (owner <> ''),
  exemption_policy JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(exemption_policy) = 'object'),
  repair_action TEXT NOT NULL DEFAULT '',
  gate_policy TEXT NOT NULL DEFAULT 'observe'
    CHECK (gate_policy IN ('observe','degrade','quarantine','release_block')),
  status TEXT NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft','shadow','approval_pending','active','superseded','rejected','retired')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by TEXT NOT NULL,
  approved_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,rule_id,rule_version),
  UNIQUE (tenant_id,rule_key,rule_version),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_quality_rules_one_active
  ON data_quality_rules (tenant_id,rule_id) WHERE status='active';
CREATE INDEX IF NOT EXISTS idx_data_quality_rules_dataset_status
  ON data_quality_rules (tenant_id,dataset_id,status,updated_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_baselines (
  baseline_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  baseline_version BIGINT NOT NULL CHECK (baseline_version > 0),
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('shadow','active','superseded','rejected')),
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  sample_count BIGINT NOT NULL CHECK (sample_count >= 0),
  metrics JSONB NOT NULL CHECK (jsonb_typeof(metrics) = 'object'),
  schema_columns JSONB NOT NULL CHECK (jsonb_typeof(schema_columns) = 'array'),
  schema_sha256 TEXT NOT NULL CHECK (length(schema_sha256) = 64),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  created_by TEXT NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,dataset_id,baseline_version),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_data_quality_baselines_one_active
  ON data_quality_baselines (tenant_id,dataset_id) WHERE status='active';

CREATE TABLE IF NOT EXISTS data_quality_watermarks (
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  source_kind TEXT NOT NULL CHECK (source_kind IN (
    'kafka_offset','flink_watermark','sink_commit','business_version','object_manifest'
  )),
  source_id TEXT NOT NULL CHECK (source_id <> ''),
  partition_id TEXT NOT NULL DEFAULT '',
  measurement_status TEXT NOT NULL CHECK (measurement_status IN (
    'measured','unknown','not_applicable','error'
  )),
  watermark_value TEXT,
  observed_at TIMESTAMPTZ,
  collected_at TIMESTAMPTZ NOT NULL,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  measurement_error TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (measurement_status = 'measured' AND watermark_value IS NOT NULL AND watermark_value <> '' AND observed_at IS NOT NULL AND measurement_error = '') OR
    (measurement_status = 'error' AND watermark_value IS NULL AND observed_at IS NULL AND measurement_error <> '') OR
    (measurement_status IN ('unknown','not_applicable') AND watermark_value IS NULL AND observed_at IS NULL AND measurement_error = '')
  ),
  PRIMARY KEY (tenant_id,dataset_id,source_kind,source_id,partition_id),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_watermarks_freshness
  ON data_quality_watermarks (tenant_id,dataset_id,collected_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_events (
  quality_event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  status TEXT NOT NULL DEFAULT 'detected' CHECK (status IN (
    'detected','triaged','repair_planned','dry_run_passed','approved',
    'replaying','reconciled','closed','failed'
  )),
  severity TEXT NOT NULL CHECK (severity IN ('info','warning','high','critical')),
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  affected_count BIGINT NOT NULL DEFAULT 0 CHECK (affected_count >= 0),
  measured_value JSONB NOT NULL CHECK (jsonb_typeof(measured_value) = 'object'),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  sample_manifest JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(sample_manifest) = 'object'),
  owner TEXT NOT NULL CHECK (owner <> ''),
  root_cause TEXT NOT NULL DEFAULT '',
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  detected_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  closed_at TIMESTAMPTZ,
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_events_work_queue
  ON data_quality_events (tenant_id,status,severity,detected_at DESC);

CREATE TABLE IF NOT EXISTS data_quality_repairs (
  repair_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  quality_event_id UUID NOT NULL REFERENCES data_quality_events(quality_event_id) ON DELETE RESTRICT,
  operation_id TEXT NOT NULL CHECK (operation_id <> ''),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  status TEXT NOT NULL DEFAULT 'planned' CHECK (status IN (
    'planned','dry_run_running','dry_run_passed','approval_pending','approved',
    'executing','executed','partial','failed','reconciled','cancelled','compensating','compensated'
  )),
  input_scope JSONB NOT NULL CHECK (jsonb_typeof(input_scope) = 'object'),
  resource_budget JSONB NOT NULL CHECK (jsonb_typeof(resource_budget) = 'object'),
  repair_summary JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(repair_summary) = 'object'),
  reconcile_summary JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(reconcile_summary) = 'object'),
  requested_by TEXT NOT NULL,
  approved_by TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key),
  CHECK (approved_by = '' OR approved_by <> requested_by)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repairs_status
  ON data_quality_repairs (tenant_id,status,updated_at,repair_id);

CREATE TABLE IF NOT EXISTS data_quality_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('dataset','rule','baseline','quality_event','repair')),
  aggregate_id TEXT NOT NULL CHECK (aggregate_id <> ''),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type TEXT NOT NULL CHECK (event_type <> ''),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
  partition_key TEXT NOT NULL CHECK (partition_key <> ''),
  payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_data_quality_outbox_ready
  ON data_quality_outbox (available_at,occurred_at,outbox_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_data_quality_outbox_reclaim
  ON data_quality_outbox (locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041400','persistent versioned data quality control plane and real handoff watermarks')
ON CONFLICT (version) DO NOTHING;

COMMIT;

-- T-DQ-001: idempotent dataset/rule governance and immutable command history.
--
-- Expand only. Runtime code must not create these objects. Rollback disables
-- the governance routes and retains history, outbox and command receipts.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_dataset_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  dataset_id TEXT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN ('created','updated','retired')),
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,dataset_id,revision),
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_dataset_history_lookup
  ON data_quality_dataset_history (tenant_id,dataset_id,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_rule_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN (
    'created','shadow_started','approval_submitted','approved','rejected','retired'
  )),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,rule_id,rule_version,revision),
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_rule_history_lookup
  ON data_quality_rule_history (tenant_id,rule_id,rule_version DESC,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_command_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL CHECK (action_id <> ''),
  operation TEXT NOT NULL CHECK (operation <> ''),
  aggregate_type TEXT NOT NULL CHECK (aggregate_type IN ('dataset','rule')),
  aggregate_id TEXT NOT NULL CHECK (aggregate_id <> ''),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES data_quality_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_command_requests_aggregate
  ON data_quality_command_requests (tenant_id,aggregate_type,aggregate_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041500','idempotent data quality dataset and rule governance history')
ON CONFLICT (version) DO NOTHING;

COMMIT;

-- T-DQ-001: durable bounded rule evaluations and detected quality events.
-- Expand only. The evaluator remains default-off until this migration and the
-- approved active-rule catalog are verified in a candidate environment.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_rule_evaluations (
  evaluation_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  rule_id UUID NOT NULL,
  rule_version BIGINT NOT NULL CHECK (rule_version > 0),
  dataset_id TEXT NOT NULL,
  window_start TIMESTAMPTZ NOT NULL,
  window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
  status TEXT NOT NULL CHECK (status IN ('pass','fail','unknown')),
  total_count BIGINT NOT NULL CHECK (total_count >= 0),
  passed_count BIGINT NOT NULL CHECK (passed_count >= 0 AND passed_count <= total_count),
  affected_count BIGINT NOT NULL CHECK (affected_count = total_count - passed_count),
  measured_value JSONB NOT NULL CHECK (jsonb_typeof(measured_value) = 'object'),
  source_watermarks JSONB NOT NULL CHECK (jsonb_typeof(source_watermarks) = 'object'),
  quality_event_id UUID,
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,rule_id,rule_version,window_start,window_end),
  FOREIGN KEY (tenant_id,rule_id,rule_version)
    REFERENCES data_quality_rules(tenant_id,rule_id,rule_version) ON DELETE RESTRICT,
  FOREIGN KEY (tenant_id,dataset_id)
    REFERENCES data_quality_datasets(tenant_id,dataset_id) ON DELETE RESTRICT,
  FOREIGN KEY (quality_event_id)
    REFERENCES data_quality_events(quality_event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_rule_evaluations_window
  ON data_quality_rule_evaluations (tenant_id,dataset_id,window_end DESC,evaluation_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041600','durable bounded data quality rule evaluations and event detection')
ON CONFLICT (version) DO NOTHING;

COMMIT;

-- T-DQ-001: immutable repair lifecycle commands and bounded replay evidence.
-- Expand only. Replay execution remains default-off and no runtime DDL is used.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_repair_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0),
  operation TEXT NOT NULL CHECK (operation IN (
    'planned','dry_run_completed','approval_submitted','approved','rejected',
    'execution_started','execution_completed','execution_partial','execution_failed','reconciled','cancelled'
  )),
  previous_status TEXT NOT NULL,
  resulting_status TEXT NOT NULL,
  actor_id TEXT NOT NULL CHECK (actor_id <> ''),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 1000),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,repair_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_history_lookup
  ON data_quality_repair_history (tenant_id,repair_id,revision DESC);

CREATE TABLE IF NOT EXISTS data_quality_repair_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  action_id TEXT NOT NULL CHECK (action_id <> ''),
  operation TEXT NOT NULL CHECK (operation <> ''),
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
  event_id UUID NOT NULL REFERENCES data_quality_outbox(event_id) ON DELETE RESTRICT,
  response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,idempotency_key),
  UNIQUE (event_id)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_repair_requests_lookup
  ON data_quality_repair_requests (tenant_id,repair_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041700','immutable data quality repair dry-run approval replay and reconcile lifecycle')
ON CONFLICT (version) DO NOTHING;

COMMIT;

-- T-DQ-001: authoritative bounded replay projection and commit receipts.
-- Expand only. The projection consumer and executor remain default-off.
BEGIN;

CREATE TABLE IF NOT EXISTS data_quality_flow_replay_projection (
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL REFERENCES data_quality_repairs(repair_id) ON DELETE RESTRICT,
  event_id TEXT NOT NULL CHECK (event_id <> ''),
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  source_event_sha256 TEXT NOT NULL CHECK (length(source_event_sha256) = 64),
  flow_payload BYTEA NOT NULL CHECK (octet_length(flow_payload) > 0),
  source_event_ts BIGINT NOT NULL,
  source_ingest_ts BIGINT NOT NULL,
  projection_version TEXT NOT NULL DEFAULT 'flow-replay-pg-v1'
    CHECK (projection_version = 'flow-replay-pg-v1'),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  committed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,repair_id,event_id),
  UNIQUE (tenant_id,repair_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_data_quality_flow_replay_projection_repair
  ON data_quality_flow_replay_projection (tenant_id,repair_id,committed_at,event_id);

CREATE TABLE IF NOT EXISTS data_quality_replay_projection_receipts (
  tenant_id TEXT NOT NULL,
  repair_id UUID NOT NULL,
  event_id TEXT NOT NULL,
  projection_id TEXT NOT NULL CHECK (projection_id = 'flow-replay-pg-v1'),
  target_store TEXT NOT NULL CHECK (target_store = 'postgresql'),
  target_object_id TEXT NOT NULL CHECK (target_object_id <> ''),
  target_version TEXT NOT NULL CHECK (target_version = 'flow-replay-pg-v1'),
  source_event_sha256 TEXT NOT NULL CHECK (length(source_event_sha256) = 64),
  target_payload_sha256 TEXT NOT NULL CHECK (length(target_payload_sha256) = 64),
  kafka_topic TEXT NOT NULL CHECK (kafka_topic = 'flow.projection-replay.v1'),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  committed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,repair_id,event_id,projection_id),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset),
  FOREIGN KEY (tenant_id,repair_id,event_id)
    REFERENCES data_quality_flow_replay_projection(tenant_id,repair_id,event_id)
    ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_data_quality_replay_projection_receipts_repair
  ON data_quality_replay_projection_receipts (tenant_id,repair_id,committed_at,event_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041800','authoritative bounded flow replay projection and commit receipts')
ON CONFLICT (version) DO NOTHING;

COMMIT;
-- END GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE
-- BEGIN GENERATED F-DASHBOARD-002 EXECUTION PIPELINE
BEGIN;

CREATE TABLE IF NOT EXISTS dashboard_task_execution_attempts (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  task_id UUID NOT NULL UNIQUE REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_attempts_ready
  ON dashboard_task_execution_attempts (available_at,created_at,request_event_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_attempts_reclaim
  ON dashboard_task_execution_attempts (locked_until,request_event_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS dashboard_task_execution_receipts (
  task_id UUID PRIMARY KEY REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, request_event_id UUID NOT NULL UNIQUE,
  provider TEXT NOT NULL CHECK (provider <> ''),
  provider_receipt_id TEXT NOT NULL CHECK (provider_receipt_id <> ''),
  status TEXT NOT NULL CHECK (status IN ('completed','partial','failed')),
  effect_state TEXT NOT NULL CHECK (effect_state IN ('confirmed','none','unknown')),
  effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(effect_ids) = 'array'),
  result JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, executed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,provider,provider_receipt_id),
  CHECK (status <> 'completed' OR effect_state = 'confirmed'),
  CHECK (status <> 'completed' OR jsonb_array_length(effect_ids) > 0)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_execution_receipts_tenant_time
  ON dashboard_task_execution_receipts (tenant_id,executed_at DESC,task_id);

CREATE TABLE IF NOT EXISTS dashboard_task_event_inbox (
  event_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
  task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL CHECK (event_type IN (
    'traffic.dashboard.v1.TaskRequested','traffic.dashboard.v1.TaskResult'
  )),
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  payload_sha256 TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, kafka_topic TEXT NOT NULL CHECK (kafka_topic <> ''),
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition >= 0),
  kafka_offset BIGINT NOT NULL CHECK (kafka_offset >= 0),
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_topic,kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_event_inbox_task
  ON dashboard_task_event_inbox (tenant_id,task_id,aggregate_version,event_id);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041930','dashboard task Kafka execution inbox provider receipts and terminal result outbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
-- END GENERATED F-DASHBOARD-002 EXECUTION PIPELINE
-- BEGIN GENERATED F-DASHBOARD-002 COMPENSATION
BEGIN;
ALTER TABLE dashboard_tasks DROP CONSTRAINT IF EXISTS dashboard_tasks_status_check;
ALTER TABLE dashboard_tasks ADD CONSTRAINT dashboard_tasks_status_check CHECK (status IN (
  'accepted','running','completed','partial','failed','cancelled',
  'compensating','compensated','compensation_partial','compensation_failed'
));
ALTER TABLE dashboard_task_event_inbox DROP CONSTRAINT IF EXISTS dashboard_task_event_inbox_event_type_check;
ALTER TABLE dashboard_task_event_inbox ADD CONSTRAINT dashboard_task_event_inbox_event_type_check CHECK (event_type IN (
  'traffic.dashboard.v1.TaskRequested','traffic.dashboard.v1.TaskResult',
  'traffic.dashboard.v1.TaskCompensationRequested','traffic.dashboard.v1.TaskCompensationResult'
));
CREATE TABLE IF NOT EXISTS dashboard_task_compensation_requests (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_outbox(event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256 TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  expected_revision BIGINT NOT NULL CHECK (expected_revision > 0),
  resulting_revision BIGINT NOT NULL CHECK (resulting_revision > expected_revision),
  action_id TEXT NOT NULL CHECK (action_id = 'dashboard-task-compensate'),
  reason TEXT NOT NULL CHECK (length(reason) BETWEEN 8 AND 2000), requested_by TEXT NOT NULL,
  trace_id TEXT NOT NULL, response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE (tenant_id,idempotency_key), UNIQUE (tenant_id,task_id)
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_requests_task
  ON dashboard_task_compensation_requests (tenant_id,task_id,created_at DESC);
CREATE TABLE IF NOT EXISTS dashboard_task_compensation_attempts (
  request_event_id UUID PRIMARY KEY REFERENCES dashboard_task_compensation_requests(request_event_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, task_id UUID NOT NULL UNIQUE REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 300),
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '', last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_attempts_ready
  ON dashboard_task_compensation_attempts (available_at,created_at,request_event_id) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_attempts_reclaim
  ON dashboard_task_compensation_attempts (locked_until,request_event_id) WHERE status='processing';
CREATE TABLE IF NOT EXISTS dashboard_task_compensation_receipts (
  task_id UUID PRIMARY KEY REFERENCES dashboard_tasks(task_id) ON DELETE RESTRICT,
  tenant_id TEXT NOT NULL, request_event_id UUID NOT NULL UNIQUE,
  provider TEXT NOT NULL CHECK (provider <> ''), provider_receipt_id TEXT NOT NULL CHECK (provider_receipt_id <> ''),
  status TEXT NOT NULL CHECK (status IN ('compensated','compensation_partial','compensation_failed')),
  effect_state TEXT NOT NULL CHECK (effect_state IN ('confirmed','none','unknown')),
  compensated_effect_ids JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(compensated_effect_ids) = 'array'),
  result JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(result) = 'object'),
  error_code TEXT NOT NULL DEFAULT '', error_message TEXT NOT NULL DEFAULT '',
  receipt_sha256 TEXT NOT NULL CHECK (receipt_sha256 ~ '^[0-9a-f]{64}$'),
  trace_id TEXT NOT NULL, compensated_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,provider,provider_receipt_id),
  CHECK (status <> 'compensated' OR effect_state = 'confirmed'),
  CHECK (status <> 'compensated' OR jsonb_array_length(compensated_effect_ids) > 0),
  CHECK (status <> 'compensation_failed' OR effect_state <> 'unknown')
);
CREATE INDEX IF NOT EXISTS idx_dashboard_task_compensation_receipts_tenant_time
  ON dashboard_task_compensation_receipts (tenant_id,compensated_at DESC,task_id);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608082100','dashboard task provider-confirmed compensation lifecycle')
ON CONFLICT (version) DO NOTHING;
COMMIT;
-- END GENERATED F-DASHBOARD-002 COMPENSATION
