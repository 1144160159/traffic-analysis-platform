-- =============================================================================
-- 资产表 (PostgreSQL) — Asset Service
-- 来源: common/old/postgres_ddl.sql (已合并)
-- =============================================================================
BEGIN;

CREATE TABLE IF NOT EXISTS asset_groups (
  group_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id  TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  name       TEXT NOT NULL,
  selector   JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);

CREATE TABLE IF NOT EXISTS assets (
  asset_id    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  ip          TEXT,
  ip_address  TEXT,
  mac_address TEXT,
  hostname    TEXT,
  vendor      TEXT,
  os_type     TEXT,
  source      TEXT NOT NULL DEFAULT 'manual',
  vlan_id     TEXT,
  switch_port TEXT,
  tags        JSONB NOT NULL DEFAULT '{}'::jsonb,
  criticality INT NOT NULL DEFAULT 0,
  metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS mac_address TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS hostname TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vendor TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS os_type TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS vlan_id TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS switch_port TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS criticality INT NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS first_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS last_seen TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE assets ALTER COLUMN ip DROP NOT NULL;
UPDATE assets SET ip_address = ip WHERE (ip_address IS NULL OR ip_address = '') AND ip IS NOT NULL;
UPDATE assets SET ip = ip_address WHERE ip IS NULL AND ip_address IS NOT NULL;

-- 资产变更事件
CREATE TABLE IF NOT EXISTS asset_events (
  event_id   SERIAL PRIMARY KEY,
  asset_id   UUID NOT NULL REFERENCES assets(asset_id) ON DELETE CASCADE,
  tenant_id  TEXT NOT NULL,
  event_type TEXT NOT NULL,
  old_value  JSONB,
  new_value  JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_asset_events_asset ON asset_events(asset_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(tenant_id, ip_address);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_ip_unique ON assets(tenant_id, ip) WHERE ip IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_mac_unique ON assets(tenant_id, mac_address) WHERE mac_address IS NOT NULL;

COMMIT;
