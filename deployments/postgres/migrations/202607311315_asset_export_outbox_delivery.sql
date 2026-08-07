-- F-ASSET-004 / WP-06-ASSET
-- Expand asset export outbox rows into a lease/retry state machine. This is
-- additive and safe when the initial export migration already ran.

BEGIN;

ALTER TABLE asset_export_outbox
  ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ;

ALTER TABLE asset_export_outbox
  DROP CONSTRAINT IF EXISTS asset_export_outbox_status_check;
ALTER TABLE asset_export_outbox
  ADD CONSTRAINT asset_export_outbox_status_check
  CHECK (status IN ('pending','processing','published','dead','cancelled'));

CREATE INDEX IF NOT EXISTS idx_asset_export_outbox_reclaim
  ON asset_export_outbox(locked_until,outbox_id) WHERE status='processing';

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version      TEXT PRIMARY KEY,
  description  TEXT NOT NULL,
  applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by   TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607311315','F-ASSET export outbox lease and reliable delivery')
ON CONFLICT (version) DO NOTHING;

COMMIT;
