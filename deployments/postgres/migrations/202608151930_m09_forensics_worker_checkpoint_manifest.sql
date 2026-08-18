-- T1-M09-N010: durable worker lease/checkpoint and final job manifest.
-- Expand only. Rollback disables worker/admission and retains all rows/objects.
BEGIN;

CREATE TABLE IF NOT EXISTS forensics_task_checkpoints (
  tenant_id          TEXT NOT NULL,
  task_id            UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  request_sha256     TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  worker_id          TEXT NOT NULL,
  lease_token        UUID NOT NULL,
  lease_until        TIMESTAMPTZ NOT NULL,
  checkpoint_revision BIGINT NOT NULL DEFAULT 1 CHECK (checkpoint_revision > 0),
  phase              TEXT NOT NULL CHECK (phase IN (
    'leased','reading_source','cutting','restoring_sessions','restoring_files',
    'verifying','publishing','completed','failed','cancelled'
  )),
  checkpoint         JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(checkpoint)='object'),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at       TIMESTAMPTZ,
  PRIMARY KEY (tenant_id,task_id),
  UNIQUE (lease_token),
  CHECK ((phase IN ('completed','failed','cancelled') AND completed_at IS NOT NULL)
      OR (phase NOT IN ('completed','failed','cancelled') AND completed_at IS NULL))
);
CREATE INDEX IF NOT EXISTS idx_forensics_task_checkpoints_reclaim
  ON forensics_task_checkpoints (lease_until,task_id)
  WHERE phase NOT IN ('completed','failed','cancelled');

CREATE TABLE IF NOT EXISTS forensics_job_manifests (
  tenant_id            TEXT NOT NULL,
  task_id              UUID NOT NULL REFERENCES tasks(task_id) ON DELETE RESTRICT,
  manifest_version     INTEGER NOT NULL CHECK (manifest_version=1),
  manifest_sha256      TEXT NOT NULL CHECK (manifest_sha256 ~ '^[0-9a-f]{64}$'),
  manifest             JSONB NOT NULL CHECK (jsonb_typeof(manifest)='object'),
  status               TEXT NOT NULL CHECK (status IN ('completed','partial')),
  result_bucket        TEXT NOT NULL,
  result_object_key    TEXT NOT NULL,
  result_object_version TEXT NOT NULL,
  result_etag          TEXT NOT NULL,
  result_size_bytes    BIGINT NOT NULL CHECK (result_size_bytes > 0),
  result_sha256        TEXT NOT NULL CHECK (result_sha256 ~ '^[0-9a-f]{64}$'),
  retention_until      TIMESTAMPTZ NOT NULL,
  executable           BOOLEAN NOT NULL DEFAULT false CHECK (NOT executable),
  automatic_open       BOOLEAN NOT NULL DEFAULT false CHECK (NOT automatic_open),
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,task_id),
  UNIQUE (tenant_id,manifest_sha256),
  UNIQUE (result_bucket,result_object_key,result_object_version),
  CHECK (manifest->>'tenant_id'=tenant_id AND manifest->>'task_id'=task_id::text),
  CHECK (manifest->>'manifest_version'='1'),
  CHECK (manifest->>'executable'='false' AND manifest->>'automatic_open'='false')
);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608151930','M09 forensics worker lease checkpoint and immutable final manifest')
ON CONFLICT (version) DO NOTHING;

COMMIT;
