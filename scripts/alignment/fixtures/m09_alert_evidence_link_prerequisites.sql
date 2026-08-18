-- Run-scoped N012 canary prerequisites. Never apply to a shared database.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id TEXT,
  action TEXT NOT NULL,
  object_type TEXT NOT NULL,
  object_id TEXT,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  ip_addr TEXT,
  user_agent TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE codex_ephemeral_alert_evidence_link_sentinel (
  marker TEXT PRIMARY KEY CHECK (marker='ephemeral-only')
);
INSERT INTO codex_ephemeral_alert_evidence_link_sentinel(marker) VALUES ('ephemeral-only');
