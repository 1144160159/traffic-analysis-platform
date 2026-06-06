BEGIN;

-- 清理旧数据（可选）
DELETE FROM api_tokens WHERE tenant_id IN ('default', 'tenant01', 'tenant02');
DELETE FROM probes WHERE tenant_id IN ('default', 'tenant01', 'tenant02');
DELETE FROM users WHERE tenant_id IN ('default', 'tenant01', 'tenant02');
DELETE FROM tenants WHERE tenant_id IN ('default', 'tenant01', 'tenant02');

-- 插入测试租户
INSERT INTO tenants (tenant_id, name, status) VALUES
  ('default', 'Default Tenant', 'active'),
  ('tenant01', 'Test Tenant 01', 'active'),
  ('tenant02', 'Test Tenant 02', 'active')
ON CONFLICT (tenant_id) DO NOTHING;

-- 插入测试用户
INSERT INTO users (tenant_id, username, email, status) VALUES
  ('default', 'admin', 'admin@default.local', 'active'),
  ('tenant01', 'admin', 'admin@tenant01.local', 'active'),
  ('tenant02', 'admin', 'admin@tenant02.local', 'active')
ON CONFLICT (tenant_id, username) DO NOTHING;

-- 插入测试探针
INSERT INTO probes (probe_id, tenant_id, name, status) VALUES
  ('probe-default-001', 'default', 'Default Probe 001', 'active'),
  ('probe-tenant01-001', 'tenant01', 'Tenant01 Probe 001', 'active'),
  ('probe-tenant01-002', 'tenant01', 'Tenant01 Probe 002', 'active'),
  ('probe-tenant02-001', 'tenant02', 'Tenant02 Probe 001', 'active')
ON CONFLICT (probe_id) DO NOTHING;

-- 插入测试 API Tokens
-- Token: test-token-001 (SHA256 hash)
INSERT INTO api_tokens (
  tenant_id, 
  name, 
  token_hash, 
  token_prefix, 
  scopes, 
  status, 
  probe_id,
  expires_at
) VALUES (
  'default',
  'Test Token 001',
  encode(sha256('test-token-001'::bytea), 'hex'),
  'tt-001',
  '["ingest:write", "pcap:write"]'::jsonb,
  'active',
  'probe-default-001',
  now() + interval '1 year'
),
(
  'tenant01',
  'Test Token 002',
  encode(sha256('test-token-002'::bytea), 'hex'),
  'tt-002',
  '["ingest:write", "pcap:write"]'::jsonb,
  'active',
  'probe-tenant01-001',
  now() + interval '1 year'
),
(
  'tenant01',
  'Test Token 003 (Read Only)',
  encode(sha256('test-token-003'::bytea), 'hex'),
  'tt-003',
  '["ingest:read"]'::jsonb,
  'active',
  'probe-tenant01-002',
  now() + interval '1 year'
),
(
  'tenant02',
  'Test Token 004',
  encode(sha256('test-token-004'::bytea), 'hex'),
  'tt-004',
  '["ingest:write"]'::jsonb,
  'active',
  'probe-tenant02-001',
  now() + interval '1 year'
)
ON CONFLICT (token_hash) DO NOTHING;

COMMIT;

-- 验证插入结果
\echo '=== Verification ==='
SELECT 'Tenants:' AS item, COUNT(*)::text AS count FROM tenants
UNION ALL
SELECT 'Probes:', COUNT(*)::text FROM probes
UNION ALL
SELECT 'API Tokens:', COUNT(*)::text FROM api_tokens
UNION ALL
SELECT 'Active Tokens:', COUNT(*)::text FROM api_tokens WHERE status = 'active';

\echo ''
\echo '=== Test Tokens ==='
SELECT 
  name,
  tenant_id,
  probe_id,
  scopes,
  'Token: ' || CASE token_hash
    WHEN encode(sha256('test-token-001'::bytea), 'hex') THEN 'test-token-001'
    WHEN encode(sha256('test-token-002'::bytea), 'hex') THEN 'test-token-002'
    WHEN encode(sha256('test-token-003'::bytea), 'hex') THEN 'test-token-003'
    WHEN encode(sha256('test-token-004'::bytea), 'hex') THEN 'test-token-004'
  END AS token_value
FROM api_tokens
WHERE tenant_id IN ('default', 'tenant01', 'tenant02')
ORDER BY tenant_id, name;