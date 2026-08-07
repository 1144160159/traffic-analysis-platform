-- F-AUDIT-001 / T-PG-001 / T-SCHEMA-001 / WP-18 / WP-21
-- Expand: make the audit actor identifier a wire-compatible TEXT value so
-- Kafka AuditLog.user_id and service identities are not restricted to UUIDs.
-- Backfill: UUID values are converted losslessly with user_id::TEXT; nulls
-- remain null. Existing event, tenant and object identifiers are unchanged.
-- Verify:
--   SELECT data_type,is_nullable FROM information_schema.columns
--    WHERE table_schema='public' AND table_name='audit_logs'
--      AND column_name IN ('user_id','object_type','detail');
--   SELECT count(*) FROM audit_logs WHERE object_type IS NULL OR detail IS NULL;
--   SELECT EXISTS (
--     SELECT 1 FROM pg_index i JOIN pg_class t ON t.oid=i.indrelid
--      WHERE t.relname='audit_logs' AND i.indisunique
--        AND pg_get_indexdef(i.indexrelid) ~* '\(event_id\)');
-- Cutover: deploy the audit consumer that verifies this schema and performs no
-- startup DDL only after this migration commits.
-- Rollback: roll the consumer image back while retaining the TEXT column.
-- Converting new non-UUID service identities back to UUID would lose data, so
-- the schema change is intentionally forward-only until the compatibility
-- observation window closes.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';
SELECT pg_advisory_xact_lock(hashtext('alignment:audit-schema:202607302100'));

DO $migration$
DECLARE
  constraint_name TEXT;
  current_type TEXT;
BEGIN
  IF to_regclass('public.audit_logs') IS NULL THEN
    RAISE EXCEPTION 'required table public.audit_logs is missing';
  END IF;

  SELECT data_type
    INTO current_type
    FROM information_schema.columns
   WHERE table_schema='public'
     AND table_name='audit_logs'
     AND column_name='user_id';

  IF current_type IS NULL THEN
    RAISE EXCEPTION 'required column public.audit_logs.user_id is missing';
  END IF;

  IF current_type <> 'text' THEN
    FOR constraint_name IN
      SELECT conname
        FROM pg_constraint
       WHERE conrelid='public.audit_logs'::regclass
         AND pg_get_constraintdef(oid) ~* '\(user_id\)'
    LOOP
      EXECUTE format(
        'ALTER TABLE public.audit_logs DROP CONSTRAINT %I',
        constraint_name
      );
    END LOOP;

    ALTER TABLE public.audit_logs
      ALTER COLUMN user_id TYPE TEXT USING user_id::TEXT;
  END IF;
END
$migration$;

UPDATE public.audit_logs
   SET object_type=''
 WHERE object_type IS NULL;
UPDATE public.audit_logs
   SET detail='{}'::jsonb
 WHERE detail IS NULL;

ALTER TABLE public.audit_logs
  ALTER COLUMN object_type SET NOT NULL,
  ALTER COLUMN detail SET DEFAULT '{}'::jsonb,
  ALTER COLUMN detail SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_event_id
  ON public.audit_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_audit_tenant_time
  ON public.audit_logs(tenant_id,created_at DESC);

CREATE TABLE IF NOT EXISTS public.alignment_schema_migrations (
  version      TEXT PRIMARY KEY,
  description  TEXT NOT NULL,
  applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by   TEXT NOT NULL DEFAULT current_user
);
INSERT INTO public.alignment_schema_migrations(version,description)
VALUES ('202607302100','audit schema authority and startup DDL exit')
ON CONFLICT (version) DO NOTHING;

DO $verify$
DECLARE
  observed_type TEXT;
  observed_nullable TEXT;
BEGIN
  SELECT data_type,is_nullable
    INTO observed_type,observed_nullable
    FROM information_schema.columns
   WHERE table_schema='public'
     AND table_name='audit_logs'
     AND column_name='user_id';
  IF observed_type <> 'text' OR observed_nullable <> 'YES' THEN
    RAISE EXCEPTION
      'audit_logs.user_id verification failed: type=% nullable=%',
      observed_type,observed_nullable;
  END IF;
  IF EXISTS (
    SELECT 1 FROM public.audit_logs
     WHERE object_type IS NULL OR detail IS NULL
  ) THEN
    RAISE EXCEPTION 'audit_logs contains null required values after migration';
  END IF;
END
$verify$;

COMMIT;
