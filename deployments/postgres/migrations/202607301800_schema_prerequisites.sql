-- T-SCHEMA-001 / T-PG-001 / WP-21
-- Bootstrap prerequisites used by every later versioned migration.
-- This file sorts before the first remediation migration so a clean database
-- and an upgraded database follow the same deterministic chain.

BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607301800','versioned schema prerequisites')
ON CONFLICT (version) DO NOTHING;

COMMIT;
