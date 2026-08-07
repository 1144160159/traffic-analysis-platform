-- F-ASSET-002 atomic authoritative upsert.
-- Expand: revisioned asset identity, immutable history metadata, transactional
-- projection outbox and persistent request idempotency.
-- Backfill: existing assets start at revision 1; existing history receives the
-- current asset revision and deterministic UUIDv5 identity.
-- Cutover: deploy the POST /v1/assets command only after this migration.
-- Reconcile:
--   SELECT count(*) FROM assets WHERE revision < 1;
--   SELECT count(*) FROM asset_event_outbox WHERE status IN ('pending','processing','dead');
-- Rollback: disable the additive POST command and stop its outbox worker.
-- Preserve asset revision, history, request and outbox rows for replay.

BEGIN;

ALTER TABLE assets
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1;
ALTER TABLE assets
  DROP CONSTRAINT IF EXISTS chk_assets_revision;
ALTER TABLE assets
  ADD CONSTRAINT chk_assets_revision CHECK (revision > 0);

ALTER TABLE asset_events
  ADD COLUMN IF NOT EXISTS event_uuid UUID,
  ADD COLUMN IF NOT EXISTS revision BIGINT,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '';

UPDATE asset_events event
SET event_uuid=COALESCE(
      event.event_uuid,
      uuid_generate_v5(
        uuid_ns_url(),
        'traffic.asset.history:' || event.tenant_id || ':' || event.asset_id::text || ':' || event.event_id::text
      )
    ),
    revision=COALESCE(event.revision,asset.revision)
FROM assets asset
WHERE asset.asset_id=event.asset_id
  AND (event.event_uuid IS NULL OR event.revision IS NULL);

ALTER TABLE asset_events
  ALTER COLUMN event_uuid SET NOT NULL,
  ALTER COLUMN event_uuid SET DEFAULT uuid_generate_v4(),
  ALTER COLUMN revision SET NOT NULL,
  ALTER COLUMN revision SET DEFAULT 1;
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_events_event_uuid ON asset_events(event_uuid);
CREATE INDEX IF NOT EXISTS idx_asset_events_tenant_revision
  ON asset_events(tenant_id,asset_id,revision DESC);

DO $asset_id_compat$
DECLARE
  asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod)
    INTO asset_id_type
    FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN
    RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type;
  END IF;
  EXECUTE format($ddl$
    CREATE TABLE IF NOT EXISTS asset_event_outbox (
      outbox_id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL UNIQUE,
      tenant_id TEXT NOT NULL, asset_id %s NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT,
      aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
      schema_version INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
      partition_key TEXT NOT NULL CHECK (partition_key <> ''), event_type TEXT NOT NULL,
      payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','processing','published','dead','cancelled')),
      attempt_count INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      locked_by TEXT NOT NULL DEFAULT '', locked_until TIMESTAMPTZ,
      last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      published_at TIMESTAMPTZ
    )$ddl$,asset_id_type);
END
$asset_id_compat$;
CREATE UNIQUE INDEX IF NOT EXISTS uq_asset_event_outbox_aggregate
  ON asset_event_outbox(tenant_id,asset_id,aggregate_version);
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_ready
  ON asset_event_outbox(available_at,outbox_id)
  WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_asset_event_outbox_reclaim
  ON asset_event_outbox(locked_until,outbox_id)
  WHERE status='processing';

DO $asset_id_compat$
DECLARE asset_id_type TEXT;
BEGIN
  SELECT format_type(a.atttypid,a.atttypmod) INTO asset_id_type FROM pg_attribute a
   WHERE a.attrelid='assets'::regclass AND a.attname='asset_id' AND NOT a.attisdropped;
  IF asset_id_type NOT IN ('uuid','text') THEN RAISE EXCEPTION 'unsupported assets.asset_id type: %',asset_id_type; END IF;
  EXECUTE format($ddl$
    CREATE TABLE IF NOT EXISTS asset_upsert_requests (
      request_id UUID PRIMARY KEY, tenant_id TEXT NOT NULL,
      idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 16 AND 200),
      request_hash TEXT NOT NULL CHECK (length(request_hash)=64), actor TEXT NOT NULL CHECK (actor <> ''),
      expected_revision BIGINT NOT NULL CHECK (expected_revision >= 0),
      asset_id %s NOT NULL REFERENCES assets(asset_id) ON DELETE RESTRICT, created BOOLEAN NOT NULL,
      resulting_revision BIGINT NOT NULL CHECK (resulting_revision > 0),
      event_id UUID NOT NULL REFERENCES asset_event_outbox(event_id) ON DELETE RESTRICT,
      outbox_id BIGINT NOT NULL REFERENCES asset_event_outbox(outbox_id) ON DELETE RESTRICT,
      trace_id TEXT NOT NULL CHECK (trace_id <> ''), created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      UNIQUE (tenant_id,idempotency_key)
    )$ddl$,asset_id_type);
END
$asset_id_compat$;
CREATE INDEX IF NOT EXISTS idx_asset_upsert_requests_asset
  ON asset_upsert_requests(tenant_id,asset_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607310015','F-ASSET-002 atomic revisioned asset upsert')
ON CONFLICT (version) DO NOTHING;

COMMIT;
