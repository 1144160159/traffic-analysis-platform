-- Ingest Gateway 测试数据初始化脚本
-- 用途：创建测试租户、探针与 API Token

BEGIN;

-- 1. 租户
INSERT INTO tenants (tenant_id, name, status, created_at, updated_at)
VALUES ('tenant01', 'Tenant 01', 'active', now(), now())
ON CONFLICT (tenant_id) DO NOTHING;

-- 2. 探针
INSERT INTO probes (probe_id, tenant_id, name, status, created_at, updated_at)
VALUES ('probe-tenant01-001', 'tenant01', 'Probe 001', 'active', now(), now())
ON CONFLICT (probe_id) DO NOTHING;

-- 3. 用户（可选，用于 token 归属）
INSERT INTO users (user_id, tenant_id, username, email, status, created_at, updated_at)
VALUES (
  '11111111-1111-1111-1111-111111111111'::uuid,
  'tenant01',
  'ingest-tester',
  'ingest-tester@localhost',
  'active',
  now(),
  now()
)
ON CONFLICT (user_id) DO NOTHING;

-- 4. API Token（Token: test-token-001）
INSERT INTO api_tokens (
  tenant_id,
  user_id,
  name,
  description,
  token_type,
  token_hash,
  token_prefix,
  scopes,
  status,
  expires_at,
  probe_id,
  created_at,
  updated_at
)
VALUES (
  'tenant01',
  '11111111-1111-1111-1111-111111111111'::uuid,
  'ingest-test-token',
  'Test token for ingest gateway',
  'probe',
  encode(sha256('test-token-001'::bytea), 'hex'),
  'test',
  '{"ingest:write": true, "pcap:write": true, "ingest:read": true}'::jsonb,
  'active',
  NOW() + INTERVAL '1 year',
  'probe-tenant01-001',
  now(),
  now()
)
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;

-- 使用方法：
-- psql -U traffic -d traffic_control -f init_test_data.sql