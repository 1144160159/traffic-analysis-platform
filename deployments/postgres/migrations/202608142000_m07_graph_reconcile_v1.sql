-- M07 closed-window graph reconcile manifests and bounded repair authorization.
-- The PostgreSQL authority and NebulaGraph target are compared; target-only
-- extras are evidence, never implicit delete instructions.
BEGIN;

CREATE TABLE IF NOT EXISTS graph_projection_reconcile_runs_v1 (
  run_id                    UUID PRIMARY KEY,
  tenant_id                 TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  window_from               TIMESTAMPTZ NOT NULL,
  window_through            TIMESTAMPTZ NOT NULL,
  max_facts                 INTEGER NOT NULL CHECK (max_facts BETWEEN 1 AND 100000),
  max_duration_ms           BIGINT NOT NULL CHECK (max_duration_ms BETWEEN 1000 AND 300000),
  state                     TEXT NOT NULL CHECK (state IN ('compared','repairing','exact','extra_preserved','not_converged')),
  requested_by              TEXT,
  approved_by               TEXT,
  approved_at               TIMESTAMPTZ,
  repair_max_items          INTEGER CHECK (repair_max_items BETWEEN 1 AND 100000),
  before_authority_sha256   TEXT NOT NULL CHECK (before_authority_sha256 ~ '^[0-9a-f]{64}$'),
  before_target_sha256      TEXT NOT NULL CHECK (before_target_sha256 ~ '^[0-9a-f]{64}$'),
  before_manifest_sha256    TEXT NOT NULL CHECK (before_manifest_sha256 ~ '^[0-9a-f]{64}$'),
  after_authority_sha256    TEXT CHECK (after_authority_sha256 ~ '^[0-9a-f]{64}$'),
  after_target_sha256       TEXT CHECK (after_target_sha256 ~ '^[0-9a-f]{64}$'),
  after_manifest_sha256     TEXT CHECK (after_manifest_sha256 ~ '^[0-9a-f]{64}$'),
  before_missing_count      INTEGER NOT NULL CHECK (before_missing_count>=0),
  before_stale_count        INTEGER NOT NULL CHECK (before_stale_count>=0),
  before_extra_count        INTEGER NOT NULL CHECK (before_extra_count>=0),
  after_missing_count       INTEGER CHECK (after_missing_count>=0),
  after_stale_count         INTEGER CHECK (after_stale_count>=0),
  after_extra_count         INTEGER CHECK (after_extra_count>=0),
  before_profile_json       JSONB NOT NULL,
  after_profile_json        JSONB,
  started_at                TIMESTAMPTZ NOT NULL,
  completed_at              TIMESTAMPTZ,
  created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (window_from<window_through),
  CHECK ((requested_by IS NULL AND approved_by IS NULL AND approved_at IS NULL AND repair_max_items IS NULL)
      OR (requested_by IS NOT NULL AND approved_by IS NOT NULL AND requested_by<>approved_by
          AND approved_at IS NOT NULL AND repair_max_items IS NOT NULL)),
  CHECK ((state IN ('exact','extra_preserved','not_converged'))=(completed_at IS NOT NULL)),
  CHECK ((after_manifest_sha256 IS NULL AND after_authority_sha256 IS NULL AND after_target_sha256 IS NULL
          AND after_missing_count IS NULL AND after_stale_count IS NULL AND after_extra_count IS NULL
          AND after_profile_json IS NULL)
      OR (after_manifest_sha256 IS NOT NULL AND after_authority_sha256 IS NOT NULL AND after_target_sha256 IS NOT NULL
          AND after_missing_count IS NOT NULL AND after_stale_count IS NOT NULL AND after_extra_count IS NOT NULL
          AND after_profile_json IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_graph_projection_reconcile_runs_v1_tenant_window
  ON graph_projection_reconcile_runs_v1(tenant_id,window_through DESC,created_at DESC);

CREATE TABLE IF NOT EXISTS graph_projection_reconcile_items_v1 (
  run_id                    UUID NOT NULL REFERENCES graph_projection_reconcile_runs_v1(run_id) ON DELETE RESTRICT,
  phase                     TEXT NOT NULL CHECK (phase IN ('before','after')),
  difference_class          TEXT NOT NULL CHECK (difference_class IN ('missing','stale','extra')),
  projection_kind          TEXT NOT NULL CHECK (projection_kind IN ('entity','relation')),
  projection_id            TEXT NOT NULL,
  authority_version        BIGINT CHECK (authority_version>0),
  authority_sha256         TEXT CHECK (authority_sha256 ~ '^[0-9a-f]{64}$'),
  authority_revoked        BOOLEAN,
  target_version           BIGINT CHECK (target_version>0),
  target_sha256            TEXT CHECK (target_sha256 ~ '^[0-9a-f]{64}$'),
  target_revoked           BOOLEAN,
  repair_eligible          BOOLEAN NOT NULL,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id,phase,projection_kind,projection_id),
  CHECK ((difference_class='missing' AND authority_version IS NOT NULL AND target_version IS NULL AND repair_eligible)
      OR (difference_class='stale' AND authority_version IS NOT NULL AND target_version IS NOT NULL AND repair_eligible)
      OR (difference_class='extra' AND authority_version IS NULL AND target_version IS NOT NULL AND NOT repair_eligible))
);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608142000','M07 graph closed-window reconcile manifests')
ON CONFLICT(version) DO NOTHING;

COMMIT;
