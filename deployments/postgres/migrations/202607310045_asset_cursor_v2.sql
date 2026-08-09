-- F-ASSET-001 stable cursor pagination.
-- Expand: add the tenant-scoped stable ordering index online. The signed
-- cursor codec and offset compatibility path are additive application changes.
-- Backfill: none.
-- Shadow: compare cursor and offset row identities against one fixed fixture
-- before moving generated clients to cursor mode.
-- Cutover: canonical clients omit offset and follow meta.pagination.next_cursor.
-- Reconcile:
--   SELECT indexrelid::regclass,indexrelid::regclass::text,indisvalid,indisready
--   FROM pg_index WHERE indexrelid='idx_assets_cursor_v2'::regclass;
-- Rollback: return clients to explicit offset and disable the cursor rollout
-- flag. Preserve the index until the observation window closes; dropping it is
-- an independently approved online operation.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_assets_cursor_v2
  ON assets(tenant_id,last_seen DESC,asset_id DESC)
  INCLUDE(updated_at);

BEGIN;

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607310045','F-ASSET-001 stable tenant cursor index')
ON CONFLICT (version) DO NOTHING;

COMMIT;
