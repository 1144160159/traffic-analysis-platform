-- F-ASSET-004 / WP-06-ASSET
-- Expand: add tenant-scoped asynchronous asset exports, object manifests,
-- transactional lifecycle events and revisioned user column preferences.
-- Backfill: none; browser-generated CSV files are not authoritative artifacts.
-- Verify:
--   SELECT status,count(*) FROM asset_export_jobs GROUP BY status;
--   SELECT count(*) FROM asset_export_outbox WHERE status='pending';
-- Cutover: enable API and worker only after the object bucket and credentials
-- are ready for the approved internal tenant.
-- Rollback: disable both flags; preserve jobs, manifests, audit and objects.

BEGIN;

CREATE TABLE IF NOT EXISTS asset_export_jobs (
  job_id             UUID PRIMARY KEY,
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  action_id          TEXT NOT NULL CHECK (action_id='asset-inventory-export'),
  format             TEXT NOT NULL CHECK (format IN ('csv','jsonl')),
  status             TEXT NOT NULL DEFAULT 'accepted'
                     CHECK (status IN ('accepted','running','completed','failed','cancelled')),
  revision           BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  columns             JSONB NOT NULL,
  query               JSONB NOT NULL,
  query_sha256        TEXT NOT NULL CHECK (length(query_sha256)=64),
  idempotency_key     TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  reason              TEXT NOT NULL,
  snapshot_id         TEXT NOT NULL DEFAULT '',
  as_of               TIMESTAMPTZ,
  source_watermarks   JSONB NOT NULL DEFAULT '{}'::jsonb,
  row_count           INTEGER NOT NULL DEFAULT 0 CHECK (row_count >= 0),
  object_bucket       TEXT NOT NULL DEFAULT '',
  object_key          TEXT NOT NULL DEFAULT '',
  mime_type           TEXT NOT NULL DEFAULT '',
  artifact_sha256     TEXT NOT NULL DEFAULT '',
  size_bytes          BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  retention_until     TIMESTAMPTZ,
  error_message       TEXT NOT NULL DEFAULT '',
  attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until        TIMESTAMPTZ,
  locked_by           TEXT NOT NULL DEFAULT '',
  created_by          TEXT NOT NULL,
  trace_id            TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at        TIMESTAMPTZ,
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_tenant
  ON asset_export_jobs(tenant_id,created_at DESC,job_id);
CREATE INDEX IF NOT EXISTS idx_asset_export_jobs_ready
  ON asset_export_jobs(next_attempt_at,created_at,job_id)
  WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS asset_export_outbox (
  outbox_id          BIGSERIAL PRIMARY KEY,
  event_id           UUID NOT NULL UNIQUE,
  job_id             UUID NOT NULL REFERENCES asset_export_jobs(job_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  event_type         TEXT NOT NULL,
  aggregate_version  BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version     INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key      TEXT NOT NULL CHECK (partition_key <> ''),
  payload            JSONB NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','published','dead','cancelled')),
  attempts           INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by          TEXT NOT NULL DEFAULT '',
  locked_until       TIMESTAMPTZ,
  last_error         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at       TIMESTAMPTZ,
  UNIQUE (job_id,aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_ready
  ON asset_export_outbox(next_attempt_at,outbox_id) WHERE status='pending';

CREATE TABLE IF NOT EXISTS asset_column_preferences (
  tenant_id   TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  user_id     TEXT NOT NULL,
  view_id     TEXT NOT NULL,
  columns     JSONB NOT NULL,
  revision    BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  updated_by  TEXT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,user_id,view_id)
);
CREATE INDEX IF NOT EXISTS idx_asset_column_preferences_updated
  ON asset_column_preferences(tenant_id,updated_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607311300','F-ASSET server export jobs and column preferences')
ON CONFLICT (version) DO NOTHING;

COMMIT;
