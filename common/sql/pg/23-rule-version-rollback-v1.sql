-- M04 rule version rollback integrity contract.
-- Expand: additive columns/index/NOT VALID checksum constraint only.
-- Backfill: checksum historical inline payloads with the legacy MD5 format;
--           non-inline or empty payloads remain intentionally ineligible.
-- Verify: every new snapshot has a tagged SHA-256 checksum and (rule_id,version)
--         is unique.
-- Cutover: enable the rollback API only after this migration is recorded.
-- Rollback: disable the API; retain snapshots, index and constraint.

BEGIN;

ALTER TABLE rule_versions
    ADD COLUMN IF NOT EXISTS checksum TEXT,
    ADD COLUMN IF NOT EXISTS change_log TEXT NOT NULL DEFAULT '';

UPDATE rule_versions
SET checksum = md5(substr(content_uri, length('inline:') + 1))
WHERE checksum IS NULL
  AND content_uri LIKE 'inline:%'
  AND length(content_uri) > length('inline:');

CREATE UNIQUE INDEX IF NOT EXISTS idx_rule_versions_rule_id_version
    ON rule_versions (rule_id, version);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'rule_versions'::regclass
          AND conname = 'rule_versions_checksum_format_check'
    ) THEN
        ALTER TABLE rule_versions
            ADD CONSTRAINT rule_versions_checksum_format_check
            CHECK (
                checksum IS NULL OR
                checksum ~ '^[0-9a-f]{32}$' OR
                checksum ~ '^sha256:[0-9a-f]{64}$'
            ) NOT VALID;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
    version TEXT PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version, description)
VALUES ('202608141530', 'M04 checksum-verified monotonic rule version rollback')
ON CONFLICT (version) DO NOTHING;

COMMIT;
