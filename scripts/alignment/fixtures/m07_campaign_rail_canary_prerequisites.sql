-- Isolated prerequisites for the M07 campaign-rail K8s canary only.
-- The canary runner creates a fresh PostgreSQL data directory, applies this
-- minimal pre-M07 authority surface, and then applies the real versioned M07
-- migrations. This file must never be applied to a shared database.

BEGIN;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);

CREATE TABLE IF NOT EXISTS tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS campaign_action_jobs (
  job_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  action_id TEXT NOT NULL,
  target TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  simulation BOOLEAN NOT NULL DEFAULT true,
  dry_run BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed')),
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS campaign_workbench_state (
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  assignee TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active','investigating','contained','closed')),
  state_version BIGINT NOT NULL DEFAULT 1,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,campaign_id)
);

CREATE TABLE IF NOT EXISTS campaign_reports (
  report_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  campaign_id TEXT NOT NULL,
  format TEXT NOT NULL DEFAULT 'pdf' CHECK (format IN ('pdf','word','json')),
  status TEXT NOT NULL DEFAULT 'completed'
    CHECK (status IN ('queued','running','completed','failed')),
  sections JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

COMMIT;
