-- =============================================================================
-- PostgreSQL Initialization Script for Feature Job
-- =============================================================================

-- 创建数据库（如果不存在）
-- CREATE DATABASE traffic; -- 通过环境变量 POSTGRES_DB 创建

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ==================== 租户表 ====================
CREATE TABLE IF NOT EXISTS tenants (
    tenant_id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==================== 租户配置表 ====================
CREATE TABLE IF NOT EXISTS tenant_config (
    id SERIAL PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    config_key VARCHAR(128) NOT NULL,
    config_value JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(tenant_id, config_key)
);

-- ==================== Feature Set 表 ====================
CREATE TABLE IF NOT EXISTS feature_sets (
    feature_set_id VARCHAR(64) PRIMARY KEY,
    schema_version VARCHAR(32) NOT NULL DEFAULT 'v2.0',
    name VARCHAR(255) NOT NULL,
    description TEXT,
    params JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(32) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==================== 探针注册表 ====================
CREATE TABLE IF NOT EXISTS probes (
    probe_id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL REFERENCES tenants(tenant_id),
    name VARCHAR(255),
    hardware_info JSONB,
    software_version VARCHAR(64),
    status VARCHAR(32) DEFAULT 'active',
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==================== 探针配置表 ====================
CREATE TABLE IF NOT EXISTS probe_config (
    id SERIAL PRIMARY KEY,
    probe_id VARCHAR(64) NOT NULL REFERENCES probes(probe_id),
    config_version VARCHAR(32) NOT NULL,
    config_data JSONB NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==================== 初始化默认数据 ====================

-- 默认租户
INSERT INTO tenants (tenant_id, name, status) VALUES 
    ('default', 'Default Tenant', 'active'),
    ('tenant-001', 'Campus Network', 'active')
ON CONFLICT (tenant_id) DO NOTHING;

-- 默认租户配置
INSERT INTO tenant_config (tenant_id, config_key, config_value) VALUES 
    ('default', 'feature_job', '{
        "priority": 5,
        "enable_l2": true,
        "sampling_rate": 1.0,
        "max_events_per_second": -1
    }'::jsonb),
    ('tenant-001', 'feature_job', '{
        "priority": 8,
        "enable_l2": true,
        "sampling_rate": 1.0,
        "max_events_per_second": 100000
    }'::jsonb)
ON CONFLICT (tenant_id, config_key) DO UPDATE SET
    config_value = EXCLUDED.config_value,
    updated_at = NOW();

-- 默认 Feature Set
INSERT INTO feature_sets (feature_set_id, schema_version, name, description, params) VALUES 
    ('default', 'v2.0', 'Default Feature Set', 'Standard feature extraction configuration', '{
        "iat_threshold_ms": 1000.0,
        "enable_l2_trigger": true,
        "l2_thresholds": {
            "high_pps_threshold": 10000.0,
            "high_bps_threshold": 1000000000.0,
            "encrypted_std_payload_threshold": 100.0,
            "tls_port": 443,
            "http_port": 80
        }
    }'::jsonb),
    ('high-sensitivity', 'v2.0', 'High Sensitivity', 'Lower thresholds for sensitive detection', '{
        "iat_threshold_ms": 500.0,
        "enable_l2_trigger": true,
        "l2_thresholds": {
            "high_pps_threshold": 5000.0,
            "high_bps_threshold": 500000000.0,
            "encrypted_std_payload_threshold": 50.0,
            "tls_port": 443,
            "http_port": 80
        }
    }'::jsonb)
ON CONFLICT (feature_set_id) DO UPDATE SET
    params = EXCLUDED.params,
    updated_at = NOW();

-- ==================== 索引 ====================
CREATE INDEX IF NOT EXISTS idx_tenant_config_tenant_id ON tenant_config(tenant_id);
CREATE INDEX IF NOT EXISTS idx_feature_sets_status ON feature_sets(status);
CREATE INDEX IF NOT EXISTS idx_probes_tenant_id ON probes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_probes_status ON probes(status);
CREATE INDEX IF NOT EXISTS idx_probe_config_probe_id ON probe_config(probe_id);

-- ==================== 更新触发器 ====================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_tenant_config_updated_at BEFORE UPDATE ON tenant_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_feature_sets_updated_at BEFORE UPDATE ON feature_sets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_probes_updated_at BEFORE UPDATE ON probes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ==================== 完成 ====================
SELECT 'PostgreSQL initialization completed' AS status;
