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
CREATE TABLE replay_tasks (
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

CREATE INDEX idx_replay_tasks_tenant_status ON replay_tasks(tenant_id, status);
CREATE INDEX idx_replay_tasks_run_id ON replay_tasks(run_id);


-- -----------------------------------------------------------------------------------------
-- datasets: 数据集版本库表
-- -----------------------------------------------------------------------------------------
CREATE TABLE datasets (
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

CREATE INDEX idx_datasets_tenant ON datasets(tenant_id);
CREATE INDEX idx_datasets_tags ON datasets USING GIN(tags);

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
  ON token_rotation_history(grace_period_ends) 
  WHERE grace_period_ends > now();

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
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  ip             TEXT NOT NULL,
  hostname       TEXT,
  tags           JSONB NOT NULL DEFAULT '{}'::jsonb,
  criticality    INT NOT NULL DEFAULT 0,
  metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, ip)
);

CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(ip);
CREATE INDEX IF NOT EXISTS idx_assets_tags ON assets USING GIN(tags);

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

-- =========================================================================================
-- 任务管理
-- =========================================================================================

-- -----------------------------------------------------------------------------------------
-- tasks: 任务表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS tasks (
  task_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  task_type      TEXT NOT NULL,
  params         JSONB NOT NULL DEFAULT '{}'::jsonb,
  status         TEXT NOT NULL DEFAULT 'queued',
  progress       INT NOT NULL DEFAULT 0,
  result_file_key TEXT,
  result_packets  BIGINT DEFAULT 0,
  result_bytes    BIGINT DEFAULT 0,
  files_scanned   INT DEFAULT 0,
  error_message   TEXT,
  run_id         TEXT,
  created_by     UUID REFERENCES users(user_id),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at   TIMESTAMPTZ
);

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
  last_heartbeat TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_probes_tenant ON probes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_probes_status ON probes(status);

-- =========================================================================================
-- 审计日志（合并所有字段）
-- =========================================================================================

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
    
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

CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_object ON audit_logs(object_type, object_id) WHERE object_id IS NOT NULL;

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
  feedback_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id      TEXT NOT NULL,
  alert_id       TEXT NOT NULL,
  user_id        UUID REFERENCES users(user_id),
  label          TEXT NOT NULL,
  reason_code    TEXT,
  comment        TEXT,
  add_to_whitelist BOOLEAN NOT NULL DEFAULT false,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feedback_tenant_alert ON alert_feedback (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_feedback_tenant_time ON alert_feedback (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_feedback_label ON alert_feedback (tenant_id, label, created_at DESC);

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
  conditions    JSONB NOT NULL,
  channels      JSONB NOT NULL,
  enabled       BOOLEAN NOT NULL DEFAULT true,
  created_by    UUID REFERENCES users(user_id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_tenant_enabled ON notification_rules (tenant_id, enabled);

-- -----------------------------------------------------------------------------------------
-- notification_history: 通知历史表
-- -----------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notification_history (
  notification_id BIGSERIAL PRIMARY KEY,
  tenant_id       TEXT NOT NULL,
  rule_id         UUID REFERENCES notification_rules(rule_id),
  alert_id        TEXT NOT NULL,
  channel         TEXT NOT NULL,
  status          TEXT NOT NULL,
  error_message   TEXT,
  sent_at         TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_tenant_alert ON notification_history (tenant_id, alert_id);
CREATE INDEX IF NOT EXISTS idx_notification_status ON notification_history (tenant_id, status, created_at DESC);

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
    SELECT table_name
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND column_name = 'updated_at'
      AND table_name NOT LIKE '%_old'
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

