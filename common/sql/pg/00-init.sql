-- =============================================================================
-- 初始化: 扩展 + 租户 + 用户 + RBAC + API Tokens
-- 来源: common/old/postgres_ddl.sql (已合并)
-- =============================================================================
BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

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
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id, category)
);

ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_user_settings_user ON user_settings (tenant_id, user_id);

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
  hardware_info    JSONB,
  software_version TEXT,
  last_heartbeat   TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE probes ADD COLUMN IF NOT EXISTS tenant_id TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE probes ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS last_heartbeat TIMESTAMPTZ;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- 探针运维操作流水
CREATE TABLE IF NOT EXISTS probe_operations (
  operation_id  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id     TEXT NOT NULL,
  probe_id      TEXT NOT NULL,
  operation_type TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'completed',
  requested_by  TEXT NOT NULL DEFAULT '',
  request       JSONB NOT NULL DEFAULT '{}'::jsonb,
  result        JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_probe_time ON probe_operations (tenant_id, probe_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_probe_operations_tenant_type_time ON probe_operations (tenant_id, operation_type, created_at DESC);

-- 页面业务状态: 行为基线重置点
CREATE TABLE IF NOT EXISTS behavior_baseline_resets (
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id  TEXT NOT NULL,
  reset_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  requested_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (tenant_id, baseline_id)
);

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
  status               TEXT NOT NULL DEFAULT 'draft',
  strategy             TEXT NOT NULL DEFAULT 'manual-review',
  confidence_threshold DOUBLE PRECISION NOT NULL DEFAULT 0.85,
  note                 TEXT NOT NULL DEFAULT '',
  updated_by           TEXT NOT NULL DEFAULT '',
  updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  detail               JSONB NOT NULL DEFAULT '{}'::jsonb,
  PRIMARY KEY (tenant_id, rule_id)
);
CREATE INDEX IF NOT EXISTS idx_fusion_rule_overrides_time ON fusion_rule_overrides (tenant_id, updated_at DESC);

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

-- 默认数据
INSERT INTO tenants (tenant_id, tenant_name, name)
VALUES ('default', '默认租户', '默认租户')
ON CONFLICT (tenant_id) DO UPDATE
SET
  tenant_name = COALESCE(NULLIF(tenants.tenant_name, ''), EXCLUDED.tenant_name),
  name = COALESCE(NULLIF(tenants.name, ''), EXCLUDED.name);

COMMIT;
