-- =============================================================================
-- 默认数据 — RBAC 角色权限 + 默认配置
-- =============================================================================
BEGIN;

-- 默认角色
INSERT INTO roles (role_id, tenant_id, name, permissions) VALUES
    (uuid_generate_v4(), 'default', 'admin', '{"*":"*"}'::jsonb),
    (uuid_generate_v4(), 'default', 'viewer', '{"alert":"read","flow":"read","dashboard":"read"}'::jsonb),
    (uuid_generate_v4(), 'default', 'operator', '{"alert":"*","flow":"read","pcap":"read"}'::jsonb),
    (uuid_generate_v4(), 'default', 'probe', '{"probe":"ingest","probe":"metrics"}'::jsonb)
ON CONFLICT (tenant_id, name) DO NOTHING;

-- 默认 feature_set (L1 全量统计)
INSERT INTO feature_sets (feature_set_id, name, params, schema_version)
VALUES ('v1-l1-default', 'L1 Flow Statistics',
    '{"window":"flow","features":["packets","bytes","duration","pps","bps","pktlen_stats","iat_stats","tcp_flags","tos"]}'::jsonb,
    'v1')
ON CONFLICT (feature_set_id) DO NOTHING;

-- 默认模型 (XGBoost 行为检测)
INSERT INTO models (model_id, tenant_id, name, model_type, description) VALUES
    (uuid_generate_v4(), 'default', 'behavior-xgboost-v1', 'gbdt', 'XGBoost behavioral detection model'),
    (uuid_generate_v4(), 'default', 'business-rule-v1', 'rules', 'Business rule detection engine'),
    (uuid_generate_v4(), 'default', 'vpn-detector-v1', 'onnx', 'VPN traffic classifier')
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
