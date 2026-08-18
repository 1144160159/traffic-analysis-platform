-- Run-scoped PostgreSQL fixture for T1-M09-N018 Kubernetes acceptance.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE codex_ephemeral_whitelist_governance_sentinel (
  marker TEXT PRIMARY KEY CHECK (marker='ephemeral-only')
);
INSERT INTO codex_ephemeral_whitelist_governance_sentinel(marker) VALUES ('ephemeral-only');

CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id BIGSERIAL PRIMARY KEY,
  event_id TEXT NOT NULL DEFAULT ('audit-' || uuid_generate_v4()::text),
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
CREATE UNIQUE INDEX idx_m09_whitelist_audit_event_id ON audit_logs(event_id);

CREATE TABLE whitelist (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('ip','domain','fingerprint','subnet','asset','account','rule','model')),
  value TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',
  approval_status TEXT NOT NULL DEFAULT 'draft',
  source_alert_id TEXT NOT NULL DEFAULT '',
  feedback_id TEXT NOT NULL DEFAULT '',
  owner_role TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'medium',
  covered_alerts INTEGER NOT NULL DEFAULT 0,
  covered_assets INTEGER NOT NULL DEFAULT 0,
  version INTEGER NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL DEFAULT '',
  approved_by TEXT NOT NULL DEFAULT '',
  approved_at TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,type,value),
  CHECK ((status='draft' AND approval_status='draft') OR
         (status='pending' AND approval_status='pending') OR
         (status='active' AND approval_status='approved') OR
         (status='disabled' AND approval_status IN ('approved','rejected')))
);

CREATE TABLE alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
