-- Run-scoped N013/M07 attack-chain snapshot prerequisites.
-- This file must never be applied to a shared database.
CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

CREATE TABLE codex_ephemeral_attack_chain_snapshot_sentinel (
  marker TEXT PRIMARY KEY CHECK (marker='ephemeral-only')
);

INSERT INTO tenants(tenant_id,name) VALUES ('tenant-a','M09 N013 canary');
INSERT INTO codex_ephemeral_attack_chain_snapshot_sentinel(marker) VALUES ('ephemeral-only');
