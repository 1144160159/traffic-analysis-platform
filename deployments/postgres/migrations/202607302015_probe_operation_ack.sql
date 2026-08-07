-- F-PROBE-001 / WP-05-PROBE
-- Expand: stable per-probe command revisions, outbox, ordered ACK receipts and state history.
-- Backfill: legacy queued rows receive deterministic per-probe revisions and an expiry
-- derived from their original creation time; already expired commands become expired,
-- while only a still-live command becomes accepted.
-- Verify:
--   SELECT status,count(*) FROM probe_operations GROUP BY status;
--   SELECT probe_id,count(*),max(command_revision) FROM probe_operations GROUP BY probe_id;
--   SELECT count(*) FROM probe_operation_outbox WHERE published=false;
-- Cutover: enable probe_operation_ack_v2 after control-channel producer and agent callback are configured.
-- Rollback: disable the v2 UI/status path; preserve operations, receipts and late ACKs for audit.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE probes ADD COLUMN IF NOT EXISTS hardware_info JSONB;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS software_version TEXT;
ALTER TABLE probes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS command_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS state_revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS desired_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS command_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS reported_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS reported_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS agent_version TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS ack_error TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT (now()+interval '10 minutes');
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS delivered_at TIMESTAMPTZ;
ALTER TABLE probe_operations ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE probe_operations
SET expires_at=created_at+interval '10 minutes'
WHERE command_revision=0;
WITH ranked AS (
  SELECT operation_id,row_number() OVER (
    PARTITION BY tenant_id,probe_id ORDER BY created_at,operation_id
  ) AS revision
  FROM probe_operations WHERE command_revision=0
)
UPDATE probe_operations p SET command_revision=ranked.revision
FROM ranked WHERE p.operation_id=ranked.operation_id;
UPDATE probe_operations
SET status=CASE WHEN expires_at<=now() THEN 'expired' ELSE 'accepted' END
WHERE status='queued';
UPDATE probe_operations
SET desired_version=COALESCE(
  NULLIF(request->>'config_version',''),NULLIF(request->>'target_version',''),
  NULLIF(request->>'desired_state',''),NULLIF(request->>'rotation_window',''),''
) WHERE desired_version='';
UPDATE probe_operations SET command_hash='legacy-unavailable' WHERE command_hash='';

CREATE UNIQUE INDEX IF NOT EXISTS uq_probe_operations_tenant_idempotency
  ON probe_operations (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_probe_operations_command_revision
  ON probe_operations (tenant_id,probe_id,command_revision);
CREATE INDEX IF NOT EXISTS idx_probe_operations_status_expiry
  ON probe_operations (status,expires_at) WHERE status IN ('accepted','delivered');

CREATE TABLE IF NOT EXISTS probe_operation_history (
  history_id     BIGSERIAL PRIMARY KEY,
  operation_id   UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id      TEXT NOT NULL,
  state_revision BIGINT NOT NULL CHECK (state_revision > 0),
  from_status    TEXT NOT NULL,
  to_status      TEXT NOT NULL,
  detail         JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (operation_id,state_revision)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_history_tenant_operation
  ON probe_operation_history (tenant_id,operation_id,state_revision);

CREATE TABLE IF NOT EXISTS probe_operation_ack_receipts (
  ack_id             UUID PRIMARY KEY,
  operation_id       UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  probe_id           TEXT NOT NULL,
  command_revision   BIGINT NOT NULL CHECK (command_revision > 0),
  reported_version   TEXT NOT NULL DEFAULT '',
  reported_hash      TEXT NOT NULL,
  agent_version      TEXT NOT NULL,
  applied            BOOLEAN NOT NULL,
  error              TEXT NOT NULL DEFAULT '',
  acknowledged_at    TIMESTAMPTZ NOT NULL,
  accepted           BOOLEAN NOT NULL,
  rejection_reason   TEXT NOT NULL DEFAULT '',
  payload             JSONB NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (operation_id)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_ack_tenant_probe
  ON probe_operation_ack_receipts (tenant_id,probe_id,command_revision DESC);

CREATE TABLE IF NOT EXISTS probe_operation_outbox (
  event_id          UUID PRIMARY KEY,
  operation_id      UUID NOT NULL REFERENCES probe_operations(operation_id) ON DELETE RESTRICT,
  tenant_id         TEXT NOT NULL,
  event_type        TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version    INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key     TEXT NOT NULL,
  payload           JSONB NOT NULL,
  published         BOOLEAN NOT NULL DEFAULT false,
  attempts          INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error        TEXT NOT NULL DEFAULT '',
  next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until      TIMESTAMPTZ,
  locked_by         TEXT NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at      TIMESTAMPTZ,
  UNIQUE (operation_id,event_type)
);
CREATE INDEX IF NOT EXISTS idx_probe_operation_outbox_pending
  ON probe_operation_outbox (next_attempt_at,created_at) WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations (version,description)
VALUES ('202607302015','F-PROBE-001 stable command and Agent ACK lifecycle')
ON CONFLICT (version) DO NOTHING;

COMMIT;
