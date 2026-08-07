-- F-TOPIC-001 / F-TOPIC-002 / WP-09
-- Expand: immutable topic snapshots plus durable, idempotent action jobs.
-- Backfill: legacy topic_actions remain queryable with executor='legacy'.
-- Verify:
--   SELECT topic,partial,count(*) FROM topic_snapshot_manifests GROUP BY topic,partial;
--   SELECT status,executor,count(*) FROM topic_actions GROUP BY status,executor;
--   SELECT count(*) FROM topic_action_outbox WHERE published=false;
-- Cutover: enable topic_snapshot_v1 and topic_executor_v2 after application rollout.
-- Rollback: disable both flags; preserve manifests, histories and receipts for audit.

BEGIN;

CREATE TABLE IF NOT EXISTS topic_snapshot_manifests (
  snapshot_id       UUID PRIMARY KEY,
  tenant_id         TEXT NOT NULL,
  topic              TEXT NOT NULL CHECK (topic IN ('tunnel','exfil','apt')),
  resource_revision BIGINT NOT NULL CHECK (resource_revision > 0),
  as_of              TIMESTAMPTZ NOT NULL,
  range_start        BIGINT NOT NULL,
  range_end          BIGINT NOT NULL,
  payload            JSONB NOT NULL,
  payload_sha256     TEXT NOT NULL CHECK (length(payload_sha256)=64),
  partial            BOOLEAN NOT NULL DEFAULT false,
  missing_sections   TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
  source_watermarks  JSONB NOT NULL DEFAULT '{}'::jsonb,
  trace_id           TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_tenant_topic
  ON topic_snapshot_manifests (tenant_id,topic,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_topic_snapshot_manifest_trace
  ON topic_snapshot_manifests (tenant_id,trace_id) WHERE trace_id<>'';

ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS snapshot_id UUID;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS expected_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS executor TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS receipt JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS error JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE topic_actions ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_actions_tenant_idempotency
  ON topic_actions (tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_topic_actions_executor_queue
  ON topic_actions (executor,status,created_at)
  WHERE status IN ('accepted','running');

CREATE TABLE IF NOT EXISTS topic_action_history (
  history_id   BIGSERIAL PRIMARY KEY,
  job_id       UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id    TEXT NOT NULL,
  revision     BIGINT NOT NULL CHECK (revision > 0),
  from_status  TEXT NOT NULL,
  to_status    TEXT NOT NULL,
  detail       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,revision)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_history_tenant_job
  ON topic_action_history (tenant_id,job_id,revision);

CREATE TABLE IF NOT EXISTS topic_action_receipts (
  receipt_id   UUID PRIMARY KEY,
  job_id       UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id    TEXT NOT NULL,
  executor     TEXT NOT NULL,
  effect_type  TEXT NOT NULL,
  effect_ref   TEXT NOT NULL,
  receipt      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (job_id,effect_type,effect_ref)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_receipts_tenant_job
  ON topic_action_receipts (tenant_id,job_id);

CREATE TABLE IF NOT EXISTS topic_action_outbox (
  event_id          UUID PRIMARY KEY,
  job_id            UUID NOT NULL REFERENCES topic_actions(action_id) ON DELETE RESTRICT,
  tenant_id         TEXT NOT NULL,
  event_type        TEXT NOT NULL,
  aggregate_version BIGINT NOT NULL DEFAULT 1 CHECK (aggregate_version > 0),
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
  UNIQUE (job_id,event_type)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_outbox_pending
  ON topic_action_outbox (next_attempt_at,created_at) WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302000','F-TOPIC snapshot and durable executor')
ON CONFLICT (version) DO NOTHING;

COMMIT;
