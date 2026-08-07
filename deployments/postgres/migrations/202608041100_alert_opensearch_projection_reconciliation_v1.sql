-- T-OS-004: durable alert OpenSearch projection debt, watermarks, and bounded reconcile runs.
-- Expand-only. The worker and repair paths remain disabled until an approved canary enables them.
BEGIN;

CREATE TABLE IF NOT EXISTS alert_opensearch_projection_debts (
  tenant_id TEXT NOT NULL CHECK (tenant_id <> ''),
  alert_id TEXT NOT NULL CHECK (alert_id <> ''),
  source_event_id TEXT NOT NULL DEFAULT '',
  source_version BIGINT NOT NULL CHECK (source_version > 0),
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256) = 64),
  target_index_version TEXT NOT NULL CHECK (target_index_version <> ''),
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','resolved','dead')),
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  first_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_failed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  resolved_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_debts_ready
  ON alert_opensearch_projection_debts (available_at,first_failed_at,tenant_id,alert_id)
  WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_debts_reclaim
  ON alert_opensearch_projection_debts (locked_until,tenant_id,alert_id)
  WHERE status='processing';

CREATE TABLE IF NOT EXISTS alert_opensearch_projection_watermarks (
  tenant_id TEXT NOT NULL CHECK (tenant_id <> ''),
  alert_id TEXT NOT NULL CHECK (alert_id <> ''),
  source_event_id TEXT NOT NULL DEFAULT '',
  source_version BIGINT NOT NULL CHECK (source_version > 0),
  source_sha256 TEXT NOT NULL CHECK (length(source_sha256) = 64),
  target_index_version TEXT NOT NULL CHECK (target_index_version <> ''),
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,alert_id,target_index_version)
);
CREATE INDEX IF NOT EXISTS idx_alert_os_projection_watermarks_tenant_version
  ON alert_opensearch_projection_watermarks (tenant_id,target_index_version,applied_at DESC);

CREATE TABLE IF NOT EXISTS alert_opensearch_reconcile_runs (
  run_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL CHECK (tenant_id <> ''),
  requested_by TEXT NOT NULL CHECK (requested_by <> ''),
  trace_id TEXT NOT NULL CHECK (trace_id <> ''),
  mode TEXT NOT NULL CHECK (mode IN ('plan','repair')),
  target_index_version TEXT NOT NULL CHECK (target_index_version <> ''),
  start_time TIMESTAMPTZ,
  end_time TIMESTAMPTZ,
  business_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  max_documents INTEGER NOT NULL CHECK (max_documents BETWEEN 1 AND 100000),
  stop_error_count INTEGER NOT NULL CHECK (stop_error_count BETWEEN 1 AND 10000),
  status TEXT NOT NULL DEFAULT 'running'
    CHECK (status IN ('running','completed','partial','failed','stopped')),
  source_count BIGINT NOT NULL DEFAULT 0 CHECK (source_count >= 0),
  target_count BIGINT NOT NULL DEFAULT 0 CHECK (target_count >= 0),
  missing_count BIGINT NOT NULL DEFAULT 0 CHECK (missing_count >= 0),
  extra_count BIGINT NOT NULL DEFAULT 0 CHECK (extra_count >= 0),
  stale_count BIGINT NOT NULL DEFAULT 0 CHECK (stale_count >= 0),
  repaired_count BIGINT NOT NULL DEFAULT 0 CHECK (repaired_count >= 0),
  error_count BIGINT NOT NULL DEFAULT 0 CHECK (error_count >= 0),
  result_manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
  stop_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alert_os_reconcile_runs_tenant_time
  ON alert_opensearch_reconcile_runs (tenant_id,created_at DESC,run_id);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608041100','durable alert OpenSearch projection debt watermarks and bounded reconcile runs')
ON CONFLICT (version) DO NOTHING;

COMMIT;
