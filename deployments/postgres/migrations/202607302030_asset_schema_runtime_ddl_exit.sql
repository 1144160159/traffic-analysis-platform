-- F-ASSET-001 / F-ASSET-002 / WP-06
-- Expand: add the asset cursor, governance, discovery and topology columns,
-- tables and indexes previously created by asset-service during process startup.
-- Backfill: copy the legacy ip value into ip_address, and copy ip_address back
-- into ip only where the tenant/IP identity is unambiguous.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202607302030';
--   SELECT count(*) FROM assets WHERE ip_address IS NULL AND ip IS NOT NULL;
--   SELECT count(*) FROM asset_discovery_runs;
--   SELECT count(*) FROM asset_topology_links;
-- Cutover: deploy an asset-service build with no runtime DDL path only after
-- this migration is recorded.
-- Rollback: roll the service image back; retain expanded columns, tables and
-- indexes because they are backward compatible and may contain audit evidence.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE assets ADD COLUMN IF NOT EXISTS ip TEXT;
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

UPDATE assets
SET ip_address=ip
WHERE (ip_address IS NULL OR ip_address='') AND ip IS NOT NULL;

UPDATE assets AS candidate
SET ip=candidate.ip_address
WHERE candidate.ip IS NULL
  AND candidate.ip_address IS NOT NULL
  AND (
    SELECT count(*) FROM assets AS peer
    WHERE peer.tenant_id=candidate.tenant_id
      AND peer.ip_address=candidate.ip_address
  )=1
  AND NOT EXISTS (
    SELECT 1 FROM assets AS peer
    WHERE peer.tenant_id=candidate.tenant_id
      AND peer.ip=candidate.ip_address
  );

DO $asset_events$
DECLARE
  asset_id_type TEXT;
BEGIN
  SELECT udt_name INTO asset_id_type
  FROM information_schema.columns
  WHERE table_schema='public'
    AND table_name='assets'
    AND column_name='asset_id';

  IF asset_id_type='uuid' THEN
    EXECUTE $ddl$
      CREATE TABLE IF NOT EXISTS asset_events (
        event_id   BIGSERIAL PRIMARY KEY,
        asset_id   UUID NOT NULL REFERENCES assets(asset_id) ON DELETE CASCADE,
        tenant_id  TEXT NOT NULL,
        event_type TEXT NOT NULL,
        old_value  JSONB,
        new_value  JSONB,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now()
      )
    $ddl$;
  ELSIF asset_id_type='text' THEN
    EXECUTE $ddl$
      CREATE TABLE IF NOT EXISTS asset_events (
        event_id   BIGSERIAL PRIMARY KEY,
        asset_id   TEXT NOT NULL,
        tenant_id  TEXT NOT NULL,
        event_type TEXT NOT NULL,
        old_value  JSONB,
        new_value  JSONB,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now()
      )
    $ddl$;
  ELSE
    RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type;
  END IF;
END
$asset_events$;

CREATE TABLE IF NOT EXISTS asset_discovery_credentials (
  credential_id TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL,
  name          TEXT NOT NULL,
  protocol      TEXT NOT NULL,
  endpoint      TEXT,
  secret_ref    TEXT NOT NULL,
  created_by    TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,name)
);

CREATE TABLE IF NOT EXISTS asset_discovery_runs (
  run_id            TEXT PRIMARY KEY,
  tenant_id         TEXT NOT NULL,
  mode              TEXT NOT NULL,
  target_cidr       TEXT,
  credential_id     TEXT,
  status            TEXT NOT NULL DEFAULT 'queued',
  requested_by      TEXT,
  discovered_assets INTEGER NOT NULL DEFAULT 0,
  discovered_links  INTEGER NOT NULL DEFAULT 0,
  error_message     TEXT,
  started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS asset_topology_links (
  link_id            TEXT PRIMARY KEY,
  tenant_id          TEXT NOT NULL,
  run_id             TEXT,
  source_asset_id    TEXT,
  source_mac         TEXT,
  source_ip          TEXT,
  source_interface   TEXT NOT NULL DEFAULT '',
  neighbor_asset_id  TEXT,
  neighbor_mac       TEXT NOT NULL DEFAULT '',
  neighbor_ip        TEXT,
  neighbor_interface TEXT NOT NULL DEFAULT '',
  protocol           TEXT NOT NULL,
  confidence         INTEGER NOT NULL DEFAULT 80,
  observed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (
    tenant_id,source_mac,neighbor_mac,protocol,
    source_interface,neighbor_interface
  )
);

CREATE INDEX IF NOT EXISTS idx_assets_tenant ON assets(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_display_code_unique
  ON assets(tenant_id,display_code) WHERE display_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_assets_tenant_type_status
  ON assets(tenant_id,asset_type,status,last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(tenant_id,ip_address);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assets_tenant_mac_unique
  ON assets(tenant_id,mac_address) WHERE mac_address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_asset_events_asset
  ON asset_events(asset_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_runs_tenant
  ON asset_discovery_runs(tenant_id,started_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_tenant
  ON asset_topology_links(tenant_id,observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_asset_topology_links_asset
  ON asset_topology_links(tenant_id,source_asset_id,neighbor_asset_id);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302030','F-ASSET runtime DDL exit')
ON CONFLICT (version) DO NOTHING;

COMMIT;
