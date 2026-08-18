CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
  task_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  task_type TEXT NOT NULL,
  params JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'queued',
  progress INTEGER NOT NULL DEFAULT 0,
  result_file_key TEXT NOT NULL DEFAULT '',
  result_sha256 TEXT NOT NULL DEFAULT '',
  result_packets BIGINT NOT NULL DEFAULT 0,
  result_bytes BIGINT NOT NULL DEFAULT 0,
  files_scanned INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
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
  request_id TEXT,
  trace_id TEXT,
  success BOOLEAN NOT NULL DEFAULT true,
  risk_level TEXT,
  result TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE codex_ephemeral_forensics_task_sentinel (
  marker TEXT PRIMARY KEY CHECK (marker='ephemeral-only')
);
INSERT INTO codex_ephemeral_forensics_task_sentinel(marker) VALUES ('ephemeral-only');
