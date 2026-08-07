-- Expand-only authority for authenticated probe registration commands.
-- Heartbeats remain bounded liveness projections and intentionally do not
-- create one audit/outbox record per 30-second observation.

ALTER TABLE probes
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS location TEXT,
  ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_probes_tenant ON probes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_probes_status ON probes(status);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid='probes'::regclass AND conname='probes_tenant_id_name_key'
  ) THEN
    ALTER TABLE probes ADD CONSTRAINT probes_tenant_id_name_key UNIQUE (tenant_id,name);
  END IF;
END $$;

ALTER TABLE probes DROP CONSTRAINT IF EXISTS probes_revision_nonnegative;
ALTER TABLE probes
  ADD CONSTRAINT probes_revision_nonnegative CHECK (revision >= 0);

CREATE TABLE IF NOT EXISTS probe_registry_history (
  history_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  revision BIGINT NOT NULL CHECK (revision > 0),
  event_type TEXT NOT NULL,
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, probe_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_probe_registry_history_tenant_probe
  ON probe_registry_history (tenant_id, probe_id, revision);

CREATE TABLE IF NOT EXISTS probe_registry_requests (
  tenant_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  request_sha256 TEXT NOT NULL CHECK (length(request_sha256) = 64),
  probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  event_id UUID NOT NULL UNIQUE,
  resource_revision BIGINT NOT NULL CHECK (resource_revision > 0),
  result JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS probe_registry_outbox (
  event_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  probe_id TEXT NOT NULL REFERENCES probes(probe_id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version > 0),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_probe_registry_outbox_ready
  ON probe_registry_outbox (next_attempt_at, created_at)
  WHERE status = 'pending';

COMMENT ON TABLE probe_registry_history IS
  'Immutable authenticated probe registration revisions; heartbeat liveness is not duplicated here.';
COMMENT ON TABLE probe_registry_requests IS
  'Durable replay registry for deterministic agent registration commands.';
COMMENT ON TABLE probe_registry_outbox IS
  'At-least-once probe registration events; consumers deduplicate by event_id and aggregate_version.';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031540','authenticated probe registration revision history audit outbox and idempotency')
ON CONFLICT (version) DO NOTHING;
