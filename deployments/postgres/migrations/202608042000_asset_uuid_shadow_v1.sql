-- F-ASSET-002 UUID shadow expand for legacy TEXT asset identifiers.
-- Expand: retain the authoritative asset_id column and every existing API while
-- adding a validated UUID shadow to authoritative, audit, outbox and projection facts.
-- Backfill: fail closed if any non-null identifier is not UUID-castable, then
-- populate all shadow columns in the same transaction.
-- Shadow: triggers keep legacy writes and UUID shadows identical. The view
-- asset_uuid_shadow_reconcile_v1 must remain at zero before a later PK cutover.
-- Cutover: intentionally not performed here; it requires an observation window.
-- Rollback: stop UUID-shadow readers and drop the new triggers/columns in a
-- separately approved migration. Existing asset_id keys and compatibility APIs remain unchanged.

BEGIN;

DO $validate_assets$
BEGIN
  IF EXISTS (
    SELECT 1 FROM assets
     WHERE asset_id IS NULL
        OR asset_id::text !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  ) THEN
    RAISE EXCEPTION 'assets.asset_id contains values that cannot enter the UUID shadow';
  END IF;
END
$validate_assets$;

ALTER TABLE assets ADD COLUMN IF NOT EXISTS asset_uuid UUID;
UPDATE assets SET asset_uuid=asset_id::text::uuid
 WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid;
ALTER TABLE assets ALTER COLUMN asset_uuid SET NOT NULL;

DO $constraint$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='assets'::regclass AND conname='uq_assets_asset_uuid') THEN
    ALTER TABLE assets ADD CONSTRAINT uq_assets_asset_uuid UNIQUE(asset_uuid);
  END IF;
END
$constraint$;

CREATE OR REPLACE FUNCTION alignment_sync_asset_uuid_shadow_v1()
RETURNS trigger LANGUAGE plpgsql AS $fn$
DECLARE parsed UUID;
BEGIN
  IF NEW.asset_id IS NULL THEN
    RAISE EXCEPTION '% asset_id cannot be null',TG_TABLE_NAME;
  END IF;
  BEGIN
    parsed := NEW.asset_id::text::uuid;
  EXCEPTION WHEN invalid_text_representation THEN
    RAISE EXCEPTION '% asset_id is not UUID-castable: %',TG_TABLE_NAME,NEW.asset_id;
  END;
  IF NEW.asset_uuid IS NOT NULL AND NEW.asset_uuid<>parsed THEN
    RAISE EXCEPTION '% asset_id/asset_uuid mismatch',TG_TABLE_NAME;
  END IF;
  NEW.asset_uuid := parsed;
  RETURN NEW;
END
$fn$;

DROP TRIGGER IF EXISTS trg_assets_uuid_shadow_v1 ON assets;
CREATE TRIGGER trg_assets_uuid_shadow_v1
BEFORE INSERT OR UPDATE OF asset_id,asset_uuid ON assets
FOR EACH ROW EXECUTE FUNCTION alignment_sync_asset_uuid_shadow_v1();

DO $children$
DECLARE
  table_name TEXT;
  constraint_name TEXT;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'asset_events','asset_event_outbox','asset_upsert_requests',
    'asset_projection_inbox','asset_projection_watermarks','asset_governance_work_orders'
  ] LOOP
    IF to_regclass(table_name) IS NULL THEN CONTINUE; END IF;
    EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS asset_uuid UUID',table_name);
    EXECUTE format('UPDATE %I SET asset_uuid=asset_id::text::uuid WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid',table_name);
    EXECUTE format('ALTER TABLE %I ALTER COLUMN asset_uuid SET NOT NULL',table_name);
    EXECUTE format('DROP TRIGGER IF EXISTS %I ON %I','trg_'||table_name||'_uuid_shadow_v1',table_name);
    EXECUTE format(
      'CREATE TRIGGER %I BEFORE INSERT OR UPDATE OF asset_id,asset_uuid ON %I FOR EACH ROW EXECUTE FUNCTION alignment_sync_asset_uuid_shadow_v1()',
      'trg_'||table_name||'_uuid_shadow_v1',table_name
    );
    constraint_name := 'fk_'||table_name||'_asset_uuid_v1';
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass(table_name) AND conname=constraint_name) THEN
      EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY(asset_uuid) REFERENCES assets(asset_uuid) ON DELETE RESTRICT NOT VALID',table_name,constraint_name);
    END IF;
    EXECUTE format('ALTER TABLE %I VALIDATE CONSTRAINT %I',table_name,constraint_name);
  END LOOP;
END
$children$;

CREATE OR REPLACE FUNCTION alignment_sync_nullable_asset_uuid_shadow_v1()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  IF NEW.source_asset_id IS NULL THEN
    NEW.source_asset_uuid := NULL;
  ELSE
    IF NEW.source_asset_uuid IS NOT NULL AND NEW.source_asset_uuid<>NEW.source_asset_id::text::uuid THEN
      RAISE EXCEPTION '% source asset shadow mismatch',TG_TABLE_NAME;
    END IF;
    NEW.source_asset_uuid := NEW.source_asset_id::text::uuid;
  END IF;
  RETURN NEW;
END
$fn$;

DO $candidate$
BEGIN
  IF to_regclass('asset_discovery_candidates') IS NULL THEN RETURN; END IF;
  ALTER TABLE asset_discovery_candidates ADD COLUMN IF NOT EXISTS source_asset_uuid UUID;
  UPDATE asset_discovery_candidates SET source_asset_uuid=source_asset_id::text::uuid
   WHERE source_asset_id IS NOT NULL AND source_asset_uuid IS DISTINCT FROM source_asset_id::text::uuid;
  DROP TRIGGER IF EXISTS trg_asset_discovery_candidates_uuid_shadow_v1 ON asset_discovery_candidates;
  CREATE TRIGGER trg_asset_discovery_candidates_uuid_shadow_v1
    BEFORE INSERT OR UPDATE OF source_asset_id,source_asset_uuid ON asset_discovery_candidates
    FOR EACH ROW EXECUTE FUNCTION alignment_sync_nullable_asset_uuid_shadow_v1();
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='asset_discovery_candidates'::regclass AND conname='fk_asset_discovery_candidates_uuid_v1') THEN
    ALTER TABLE asset_discovery_candidates ADD CONSTRAINT fk_asset_discovery_candidates_uuid_v1
      FOREIGN KEY(source_asset_uuid) REFERENCES assets(asset_uuid) ON DELETE RESTRICT NOT VALID;
  END IF;
  ALTER TABLE asset_discovery_candidates VALIDATE CONSTRAINT fk_asset_discovery_candidates_uuid_v1;
END
$candidate$;

CREATE OR REPLACE FUNCTION alignment_sync_topology_uuid_shadow_v1()
RETURNS trigger LANGUAGE plpgsql AS $fn$
BEGIN
  IF NEW.source_asset_id IS NULL THEN NEW.source_asset_uuid := NULL;
  ELSE
    IF NEW.source_asset_uuid IS NOT NULL AND NEW.source_asset_uuid<>NEW.source_asset_id::text::uuid THEN
      RAISE EXCEPTION 'asset_topology_links source shadow mismatch';
    END IF;
    NEW.source_asset_uuid := NEW.source_asset_id::text::uuid;
  END IF;
  IF NEW.neighbor_asset_id IS NULL THEN NEW.neighbor_asset_uuid := NULL;
  ELSE
    IF NEW.neighbor_asset_uuid IS NOT NULL AND NEW.neighbor_asset_uuid<>NEW.neighbor_asset_id::text::uuid THEN
      RAISE EXCEPTION 'asset_topology_links neighbor shadow mismatch';
    END IF;
    NEW.neighbor_asset_uuid := NEW.neighbor_asset_id::text::uuid;
  END IF;
  RETURN NEW;
END
$fn$;

DO $topology$
BEGIN
  IF to_regclass('asset_topology_links') IS NULL THEN RETURN; END IF;
  ALTER TABLE asset_topology_links
    ADD COLUMN IF NOT EXISTS source_asset_uuid UUID,
    ADD COLUMN IF NOT EXISTS neighbor_asset_uuid UUID;
  UPDATE asset_topology_links
     SET source_asset_uuid=source_asset_id::text::uuid,
         neighbor_asset_uuid=neighbor_asset_id::text::uuid
   WHERE source_asset_uuid IS DISTINCT FROM source_asset_id::text::uuid
      OR neighbor_asset_uuid IS DISTINCT FROM neighbor_asset_id::text::uuid;
  DROP TRIGGER IF EXISTS trg_asset_topology_uuid_shadow_v1 ON asset_topology_links;
  CREATE TRIGGER trg_asset_topology_uuid_shadow_v1
    BEFORE INSERT OR UPDATE OF source_asset_id,source_asset_uuid,neighbor_asset_id,neighbor_asset_uuid
    ON asset_topology_links FOR EACH ROW EXECUTE FUNCTION alignment_sync_topology_uuid_shadow_v1();
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='asset_topology_links'::regclass AND conname='fk_asset_topology_source_uuid_v1') THEN
    ALTER TABLE asset_topology_links ADD CONSTRAINT fk_asset_topology_source_uuid_v1
      FOREIGN KEY(source_asset_uuid) REFERENCES assets(asset_uuid) ON DELETE RESTRICT NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='asset_topology_links'::regclass AND conname='fk_asset_topology_neighbor_uuid_v1') THEN
    ALTER TABLE asset_topology_links ADD CONSTRAINT fk_asset_topology_neighbor_uuid_v1
      FOREIGN KEY(neighbor_asset_uuid) REFERENCES assets(asset_uuid) ON DELETE RESTRICT NOT VALID;
  END IF;
  ALTER TABLE asset_topology_links VALIDATE CONSTRAINT fk_asset_topology_source_uuid_v1;
  ALTER TABLE asset_topology_links VALIDATE CONSTRAINT fk_asset_topology_neighbor_uuid_v1;
END
$topology$;

CREATE INDEX IF NOT EXISTS idx_assets_cursor_text_compat
  ON assets(last_seen DESC,(asset_id::text) DESC);

CREATE OR REPLACE VIEW asset_uuid_shadow_reconcile_v1 AS
SELECT 'assets'::TEXT AS domain,count(*)::BIGINT AS row_count,
       count(*) FILTER (WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid)::BIGINT AS mismatch_count
  FROM assets
UNION ALL SELECT 'asset_events',count(*)::BIGINT,
       count(*) FILTER (WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid)::BIGINT FROM asset_events
UNION ALL SELECT 'asset_event_outbox',count(*)::BIGINT,
       count(*) FILTER (WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid)::BIGINT FROM asset_event_outbox
UNION ALL SELECT 'asset_projection_inbox',count(*)::BIGINT,
       count(*) FILTER (WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid)::BIGINT FROM asset_projection_inbox
UNION ALL SELECT 'asset_projection_watermarks',count(*)::BIGINT,
       count(*) FILTER (WHERE asset_uuid IS DISTINCT FROM asset_id::text::uuid)::BIGINT FROM asset_projection_watermarks
UNION ALL SELECT 'asset_topology_links.source',count(*) FILTER (WHERE source_asset_id IS NOT NULL)::BIGINT,
       count(*) FILTER (WHERE source_asset_uuid IS DISTINCT FROM source_asset_id::text::uuid)::BIGINT FROM asset_topology_links
UNION ALL SELECT 'asset_topology_links.neighbor',count(*) FILTER (WHERE neighbor_asset_id IS NOT NULL)::BIGINT,
       count(*) FILTER (WHERE neighbor_asset_uuid IS DISTINCT FROM neighbor_asset_id::text::uuid)::BIGINT FROM asset_topology_links;

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608042000','F-ASSET-002 legacy TEXT to UUID shadow expand and reconcile')
ON CONFLICT (version) DO NOTHING;

COMMIT;
