-- M06 asset source-fact ClickHouse projection expansion.
--
-- Existing rows remain explicitly disabled. New canonical consumers choose
-- pending only when the ClickHouse target is configured and ready; rollback
-- stops the worker and preserves inbox, source bytes, and watermarks.

BEGIN;

ALTER TABLE asset_projection_inbox
  ADD COLUMN IF NOT EXISTS kafka_topic TEXT NOT NULL DEFAULT 'asset.events.v2',
  ADD COLUMN IF NOT EXISTS kafka_timestamp_ms BIGINT NOT NULL DEFAULT 1
    CHECK (kafka_timestamp_ms > 0),
  ADD COLUMN IF NOT EXISTS raw_payload BYTEA,
  ADD COLUMN IF NOT EXISTS source_sha256 TEXT NOT NULL DEFAULT repeat('0',64)
    CHECK (length(source_sha256)=64),
  ADD COLUMN IF NOT EXISTS ch_status TEXT NOT NULL DEFAULT 'disabled'
    CHECK (ch_status IN ('disabled','pending','applied','dead'));

ALTER TABLE asset_projection_watermarks
  DROP CONSTRAINT IF EXISTS asset_projection_watermarks_target_check;
ALTER TABLE asset_projection_watermarks
  ADD CONSTRAINT asset_projection_watermarks_target_check
  CHECK (target IN ('opensearch','nebulagraph','clickhouse'));

CREATE INDEX IF NOT EXISTS idx_asset_projection_inbox_ch_ready
  ON asset_projection_inbox(available_at,created_at)
  WHERE ch_status='pending';

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608141610','M06 asset ClickHouse source-fact projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
