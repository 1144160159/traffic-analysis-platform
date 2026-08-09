-- F-ALERT-001 / F-ALERT-002 / F-WHITELIST-001 / WP-07
-- Expand: version the threat-intelligence schema that was previously created
-- by both alert-service and threat-intel-service during process startup.
-- Adopt: verify the whitelist, playbook, notification and data-quality tables
-- already supplied by the PostgreSQL base schema before disabling their
-- startup InitSchema calls.
-- Backfill: tenant_id and updated_at defaults are applied to legacy threat
-- intelligence rows by the ALTER statements below.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202607302045';
--   SELECT count(*) FROM threat_intel WHERE tenant_id IS NULL;
--   SELECT count(*) FROM threat_intel_feeds WHERE tenant_id IS NULL;
-- Cutover: deploy alert-service and threat-intel-service builds without
-- startup DDL only after this migration commits.
-- Rollback: roll service images back; retain the additive schema and migration
-- record because existing rows and audit evidence must not be discarded.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS threat_intel (
  id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id   TEXT NOT NULL DEFAULT 'default',
  type        TEXT NOT NULL CHECK (type IN ('ip','domain','hash')),
  value       TEXT NOT NULL,
  reputation  TEXT NOT NULL DEFAULT 'unknown',
  category    TEXT NOT NULL DEFAULT '',
  source      TEXT NOT NULL DEFAULT 'manual',
  description TEXT NOT NULL DEFAULT '',
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,type,value)
);
ALTER TABLE threat_intel
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE threat_intel
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE threat_intel
  DROP CONSTRAINT IF EXISTS threat_intel_type_value_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_threat_intel_tenant_type_value
  ON threat_intel(tenant_id,type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_value
  ON threat_intel(type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_tenant_value
  ON threat_intel(tenant_id,type,value);
CREATE INDEX IF NOT EXISTS idx_threat_intel_rep
  ON threat_intel(reputation);
CREATE INDEX IF NOT EXISTS idx_threat_intel_source
  ON threat_intel(source);

CREATE TABLE IF NOT EXISTS threat_intel_feeds (
  id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id        TEXT NOT NULL DEFAULT 'default',
  name             TEXT NOT NULL,
  enabled          BOOLEAN NOT NULL DEFAULT true,
  interval_seconds INTEGER NOT NULL DEFAULT 3600 CHECK (interval_seconds >= 1),
  entries          JSONB NOT NULL DEFAULT '[]'::jsonb,
  last_run_at      TIMESTAMPTZ,
  next_run_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_status      TEXT NOT NULL DEFAULT 'never',
  last_error       TEXT NOT NULL DEFAULT '',
  run_count        INTEGER NOT NULL DEFAULT 0,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,name)
);
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS interval_seconds INTEGER NOT NULL DEFAULT 3600;
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS entries JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS last_run_at TIMESTAMPTZ;
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS last_status TEXT NOT NULL DEFAULT 'never';
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS run_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE threat_intel_feeds
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_due
  ON threat_intel_feeds(enabled,next_run_at);
CREATE INDEX IF NOT EXISTS idx_threat_intel_feeds_tenant
  ON threat_intel_feeds(tenant_id,name);

DO $adopt$
DECLARE
  required_table TEXT;
BEGIN
  FOREACH required_table IN ARRAY ARRAY[
    'whitelist',
    'alert_playbook_executions',
    'alert_playbook_overrides',
    'alert_playbook_definitions',
    'alert_notification_settings',
    'notification_silence_rules',
    'notification_rules',
    'notification_history',
    'notification_escalation_policies',
    'notification_escalation_jobs',
    'notification_templates',
    'data_quality_actions',
    'data_quality_ui_fixtures'
  ]
  LOOP
    IF to_regclass('public.'||required_table) IS NULL THEN
      RAISE EXCEPTION
        'required base-schema table public.% is missing',required_table;
    END IF;
  END LOOP;
END
$adopt$;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302045','F-ALERT startup DDL exit and schema adoption')
ON CONFLICT (version) DO NOTHING;

COMMIT;
