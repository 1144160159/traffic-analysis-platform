-- F-ASSET-002 projection inbox and per-target watermarks.
-- Expand: Kafka ingestion is durable before offset commit; OpenSearch and
-- NebulaGraph targets advance independently and are retryable after partial
-- success.
-- Backfill: none. Historical authoritative assets are replayed through the
-- asset_event_outbox backfill/runbook instead of synthesizing completion rows.
-- Cutover: enable asset.events.v2 consumer only after both target adapters pass
-- readiness and this migration is present.
-- Reconcile:
--   SELECT status,count(*) FROM asset_projection_inbox GROUP BY status;
--   SELECT target,count(*),max(aggregate_version) FROM asset_projection_watermarks GROUP BY target;
-- Rollback: stop the consumer and projector; preserve inbox and watermarks.

BEGIN;

DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format($ddl$CREATE TABLE IF NOT EXISTS asset_projection_inbox (
  event_id           UUID PRIMARY KEY,
  tenant_id          TEXT NOT NULL,
  asset_id           %s NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  aggregate_version  BIGINT NOT NULL CHECK (aggregate_version > 0),
  schema_version     INTEGER NOT NULL CHECK (schema_version = 2),
  partition_key      TEXT NOT NULL CHECK (partition_key <> ''),
  trace_id           TEXT NOT NULL DEFAULT '',
  payload            JSONB NOT NULL,
  payload_sha256     TEXT NOT NULL CHECK (length(payload_sha256)=64),
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  os_status          TEXT NOT NULL DEFAULT 'pending'
                     CHECK (os_status IN ('pending','applied','dead')),
  nebula_status      TEXT NOT NULL DEFAULT 'pending'
                     CHECK (nebula_status IN ('pending','applied','dead')),
  status             TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','processing','applied','dead')),
  attempt_count      INTEGER NOT NULL DEFAULT 0,
  available_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by          TEXT NOT NULL DEFAULT '',
  locked_until       TIMESTAMPTZ,
  last_error         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_at         TIMESTAMPTZ,
  UNIQUE (tenant_id,asset_id,aggregate_version)
)$ddl$,asset_id_type);
END
$asset_id_compat$;
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_ready
  ON asset_projection_inbox(available_at,created_at)
  WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_reclaim
  ON asset_projection_inbox(locked_until,created_at)
  WHERE status='processing';
CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_dead
  ON asset_projection_inbox(updated_at)
  WHERE status='dead';

DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format($ddl$CREATE TABLE IF NOT EXISTS asset_projection_watermarks (
  tenant_id          TEXT NOT NULL,
  asset_id           %s NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
  target             TEXT NOT NULL CHECK (target IN ('opensearch','nebulagraph')),
  aggregate_version  BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_id           UUID NOT NULL REFERENCES asset_projection_inbox(event_id) ON DELETE RESTRICT,
  projection_sha256  TEXT NOT NULL CHECK (length(projection_sha256)=64),
  applied_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,asset_id,target)
)$ddl$,asset_id_type);
END
$asset_id_compat$;
CREATE INDEX IF NOT EXISTS idx_asset_projection_watermarks_target_version
  ON asset_projection_watermarks(target,aggregate_version);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607310030','F-ASSET-002 durable OS and Nebula projection inbox')
ON CONFLICT (version) DO NOTHING;

COMMIT;
