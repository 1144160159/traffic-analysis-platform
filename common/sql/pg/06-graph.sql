-- =============================================================================
-- 图服务配置、缓存预热与查询历史 (PostgreSQL)
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS graph_cache_config (
  config_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  neighbor_ttl_sec INT NOT NULL DEFAULT 300,
  entity_ttl_sec INT NOT NULL DEFAULT 300,
  graph_ttl_sec INT NOT NULL DEFAULT 120,
  max_nodes_per_cache INT NOT NULL DEFAULT 500,
  max_edges_per_cache INT NOT NULL DEFAULT 1000,
  time_granularity_sec INT NOT NULL DEFAULT 300,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id)
);

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

CREATE TABLE IF NOT EXISTS graph_hot_ips (
  hot_ip_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  ip TEXT NOT NULL,
  query_count BIGINT NOT NULL DEFAULT 0,
  last_query_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  priority INT NOT NULL DEFAULT 0,
  warmed_up BOOLEAN NOT NULL DEFAULT FALSE,
  last_warmup_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, ip)
);

CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_priority ON graph_hot_ips (tenant_id, priority DESC, last_query_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_hot_ips_tenant_ip ON graph_hot_ips (tenant_id, ip);

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
  cache_hit BOOLEAN NOT NULL DEFAULT FALSE,
  status TEXT NOT NULL,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_graph_query_history_tenant_time ON graph_query_history (tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_graph_query_history_type_status ON graph_query_history (query_type, status, created_at DESC);

COMMIT;
