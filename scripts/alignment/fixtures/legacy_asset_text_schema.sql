-- Owned ephemeral fixture for the pre-remediation asset schema.
-- Deliberately uses TEXT for the authoritative asset primary key.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE tenants (tenant_id TEXT PRIMARY KEY,name TEXT NOT NULL);
CREATE TABLE assets (
  asset_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id),
  display_code TEXT, asset_type TEXT NOT NULL DEFAULT 'unknown', status TEXT NOT NULL DEFAULT 'active',
  ip TEXT, ip_address TEXT, mac_address TEXT, hostname TEXT, vendor TEXT, os_type TEXT,
  source TEXT NOT NULL DEFAULT 'manual', vlan_id TEXT, switch_port TEXT, department TEXT,
  campus TEXT, owner TEXT, criticality INTEGER NOT NULL DEFAULT 0,
  tags JSONB NOT NULL DEFAULT '{}'::jsonb, metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  first_seen TIMESTAMPTZ NOT NULL DEFAULT now(), last_seen TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE asset_events (
  event_id BIGSERIAL PRIMARY KEY, asset_id TEXT NOT NULL REFERENCES assets(asset_id) ON DELETE CASCADE,
  tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, old_value JSONB, new_value JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE audit_logs (
  event_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, user_id TEXT, action TEXT NOT NULL,
  object_type TEXT NOT NULL, object_id TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_addr TEXT, user_agent TEXT, request_id TEXT, trace_id TEXT,
  success BOOLEAN NOT NULL DEFAULT true, risk_level TEXT, result TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE asset_discovery_runs (
  run_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, mode TEXT NOT NULL DEFAULT 'legacy',
  target_cidr TEXT, credential_id TEXT, status TEXT NOT NULL DEFAULT 'queued', requested_by TEXT,
  discovered_assets INTEGER NOT NULL DEFAULT 0, discovered_links INTEGER NOT NULL DEFAULT 0,
  error_message TEXT, started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ
);
CREATE TABLE asset_topology_links (
  link_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, run_id TEXT,
  source_asset_id TEXT REFERENCES assets(asset_id), neighbor_asset_id TEXT REFERENCES assets(asset_id),
  source_mac TEXT, source_ip TEXT, source_interface TEXT,
  neighbor_mac TEXT, neighbor_ip TEXT, neighbor_interface TEXT,
  protocol TEXT NOT NULL DEFAULT 'legacy', confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO tenants(tenant_id,name) VALUES ('legacy-fixture','Legacy Fixture');
INSERT INTO assets(asset_id,tenant_id,mac_address,ip_address,hostname)
VALUES ('11111111-1111-4111-8111-111111111111','legacy-fixture','02:00:00:00:00:01','10.0.0.1','legacy-one');
INSERT INTO asset_events(asset_id,tenant_id,event_type,new_value)
VALUES ('11111111-1111-4111-8111-111111111111','legacy-fixture','legacy-import','{}'::jsonb);
INSERT INTO asset_topology_links(link_id,tenant_id,source_asset_id)
VALUES ('legacy-link-1','legacy-fixture','11111111-1111-4111-8111-111111111111');
