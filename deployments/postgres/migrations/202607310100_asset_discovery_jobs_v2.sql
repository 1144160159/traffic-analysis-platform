-- F-ASSET-003 durable active-discovery jobs and isolated candidates.
-- Expand: retain /discovery/runs and existing rows while adding versioned,
-- idempotent job state, lease recovery, transition history, an outbox and
-- candidate observations that are not authoritative assets.
-- Backfill: legacy queued/completed/failed rows receive revision 1 and a
-- compatible terminal state. No legacy asset is demoted or deleted.
-- Cutover: enable ASSET_DISCOVERY_JOBS_V2_ENABLED only after this migration
-- and the independent discovery worker are deployed.
-- Verify:
--   SELECT status,count(*) FROM asset_discovery_runs GROUP BY status;
--   SELECT count(*) FROM asset_discovery_candidates c
--    LEFT JOIN asset_discovery_runs r ON r.run_id=c.run_id
--    WHERE r.run_id IS NULL OR r.tenant_id<>c.tenant_id;
--   SELECT count(*) FROM asset_discovery_outbox
--    WHERE status IN ('pending','processing','dead');
-- Rollback: disable the feature flag and worker. Preserve job, candidate,
-- history and outbox rows for audit and replay.

BEGIN;

ALTER TABLE asset_discovery_runs
  ADD COLUMN IF NOT EXISTS action_id TEXT NOT NULL DEFAULT 'asset-active-discovery-run',
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS target_network CIDR,
  ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT 'legacy-compatible',
  ADD COLUMN IF NOT EXISTS rate_limit_per_second INTEGER NOT NULL DEFAULT 10,
  ADD COLUMN IF NOT EXISTS security_window_start TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS security_window_end TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS approved_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
  ADD COLUMN IF NOT EXISTS request_hash TEXT,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS cancel_requested BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS discovered_candidates INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS rejected_records INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS result_watermark TEXT NOT NULL DEFAULT '';

UPDATE asset_discovery_runs
   SET status=CASE status WHEN 'completed' THEN 'succeeded' ELSE status END,
       queued_at=COALESCE(queued_at,started_at),
       updated_at=COALESCE(completed_at,started_at,now())
 WHERE status='completed' OR queued_at IS NULL OR updated_at IS NULL;

ALTER TABLE asset_discovery_runs
  DROP CONSTRAINT IF EXISTS asset_discovery_runs_status_check,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_run_status,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_revision,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_rate,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_window,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_idempotency;
ALTER TABLE asset_discovery_runs
  ADD CONSTRAINT chk_asset_discovery_run_status
    CHECK (status IN (
      'queued','running','cancel_requested','cancelled',
      'succeeded','completed','partial','failed','blocked'
    )),
  ADD CONSTRAINT chk_asset_discovery_revision CHECK (revision > 0),
  ADD CONSTRAINT chk_asset_discovery_rate
    CHECK (rate_limit_per_second BETWEEN 1 AND 10000),
  ADD CONSTRAINT chk_asset_discovery_window CHECK (
    security_window_start IS NULL
    OR security_window_end IS NULL
    OR security_window_end > security_window_start
  ),
  ADD CONSTRAINT chk_asset_discovery_idempotency CHECK (
    idempotency_key IS NULL
    OR length(idempotency_key) BETWEEN 16 AND 200
  );

CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_discovery_run_idempotency
  ON asset_discovery_runs(tenant_id,idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_ready
  ON asset_discovery_runs(queued_at,run_id)
  WHERE status='queued';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_reclaim
  ON asset_discovery_runs(locked_until,run_id)
  WHERE status='running';
CREATE INDEX IF NOT EXISTS idx_asset_discovery_run_overlap
  ON asset_discovery_runs USING gist(target_network inet_ops)
  WHERE status IN ('queued','running','cancel_requested');

CREATE TABLE IF NOT EXISTS asset_discovery_run_history (
  transition_id BIGSERIAL PRIMARY KEY,
  run_id        TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  tenant_id     TEXT NOT NULL,
  from_status   TEXT NOT NULL,
  to_status     TEXT NOT NULL,
  revision      BIGINT NOT NULL CHECK (revision > 0),
  actor         TEXT NOT NULL,
  reason        TEXT NOT NULL,
  trace_id      TEXT NOT NULL,
  detail        JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (run_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_history_tenant
  ON asset_discovery_run_history(tenant_id,run_id,revision);

CREATE TABLE IF NOT EXISTS asset_discovery_control_requests (
  request_id          UUID PRIMARY KEY,
  tenant_id           TEXT NOT NULL,
  run_id              TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  operation           TEXT NOT NULL CHECK (operation IN ('cancel','merge_candidate')),
  candidate_id        UUID,
  idempotency_key     TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
  request_hash        TEXT NOT NULL CHECK (length(request_hash)=64),
  expected_revision   BIGINT NOT NULL CHECK (expected_revision > 0),
  resulting_revision  BIGINT NOT NULL CHECK (resulting_revision > 0),
  result_payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  actor               TEXT NOT NULL,
  reason              TEXT NOT NULL,
  trace_id            TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_control_run
  ON asset_discovery_control_requests(tenant_id,run_id,created_at);

ALTER TABLE asset_discovery_control_requests
  ADD COLUMN IF NOT EXISTS candidate_id UUID,
  ADD COLUMN IF NOT EXISTS result_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  DROP CONSTRAINT IF EXISTS asset_discovery_control_requests_operation_check,
  DROP CONSTRAINT IF EXISTS chk_asset_discovery_control_operation;
ALTER TABLE asset_discovery_control_requests
  ADD CONSTRAINT chk_asset_discovery_control_operation
  CHECK (operation IN ('cancel','merge_candidate'));

DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format($ddl$CREATE TABLE IF NOT EXISTS asset_discovery_candidates (
  candidate_id       UUID PRIMARY KEY,
  run_id             TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  fingerprint        TEXT NOT NULL CHECK (length(fingerprint)=64),
  observation        JSONB NOT NULL,
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','merged','rejected')),
  revision           BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  source_asset_id    %s REFERENCES assets(asset_id) ON DELETE RESTRICT,
  decision_reason    TEXT NOT NULL DEFAULT '',
  decided_by         TEXT NOT NULL DEFAULT '',
  discovered_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_at         TIMESTAMPTZ,
  UNIQUE (tenant_id,run_id,fingerprint)
)$ddl$,asset_id_type);
END
$asset_id_compat$;
CREATE INDEX IF NOT EXISTS idx_asset_discovery_candidates_run
  ON asset_discovery_candidates(tenant_id,run_id,status,discovered_at,candidate_id);

CREATE TABLE IF NOT EXISTS asset_discovery_outbox (
  outbox_id       BIGSERIAL PRIMARY KEY,
  event_id        UUID NOT NULL UNIQUE,
  run_id          TEXT NOT NULL REFERENCES asset_discovery_runs(run_id) ON DELETE RESTRICT,
  tenant_id       TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version  INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key   TEXT NOT NULL CHECK (partition_key <> ''),
  event_type      TEXT NOT NULL,
  payload         JSONB NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending','processing','published','dead','cancelled')),
  attempt_count   INTEGER NOT NULL DEFAULT 0,
  available_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by       TEXT NOT NULL DEFAULT '',
  locked_until    TIMESTAMPTZ,
  last_error      TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at    TIMESTAMPTZ,
  UNIQUE (run_id,aggregate_version,event_type)
);
CREATE INDEX IF NOT EXISTS idx_asset_discovery_outbox_ready
  ON asset_discovery_outbox(available_at,outbox_id)
  WHERE status='pending';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607310100','F-ASSET-003 durable discovery jobs and candidates')
ON CONFLICT (version) DO NOTHING;

COMMIT;
