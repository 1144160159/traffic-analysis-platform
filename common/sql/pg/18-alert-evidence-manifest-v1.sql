-- F-ALERT-005 alert evidence manifest authority. Keep byte-for-byte semantics
-- aligned with deployments/postgres/migrations/202608091700_alert_evidence_manifest_v1.sql.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS alert_evidence_manifests (
  tenant_id TEXT NOT NULL CHECK(tenant_id<>''), alert_id TEXT NOT NULL CHECK(alert_id<>''), evidence_id TEXT NOT NULL CHECK(evidence_id<>''),
  event_id TEXT NOT NULL DEFAULT '', evidence_type TEXT NOT NULL CHECK(evidence_type<>''),
  source_store TEXT NOT NULL CHECK(source_store IN ('postgresql','clickhouse','opensearch','minio','arkime')),
  object_bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '', object_version TEXT NOT NULL DEFAULT '', object_sha256 TEXT NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL DEFAULT 0 CHECK(size_bytes>=0), content_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'available' CHECK(state IN ('available','missing','expired','unavailable','integrity_failed')),
  revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0), source_watermarks JSONB NOT NULL DEFAULT '{}'::jsonb CHECK(jsonb_typeof(source_watermarks)='object'),
  observed_at TIMESTAMPTZ NOT NULL DEFAULT now(), expires_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(tenant_id,alert_id,evidence_id),
  CHECK(source_store<>'minio' OR (object_bucket<>'' AND object_key LIKE ('tenants/'||tenant_id||'/%') AND object_key NOT LIKE '%..%' AND object_sha256 ~ '^[0-9a-f]{64}$'))
);
CREATE INDEX IF NOT EXISTS idx_alert_evidence_manifest_event ON alert_evidence_manifests(tenant_id,event_id) WHERE event_id<>'';
CREATE INDEX IF NOT EXISTS idx_alert_evidence_manifest_state ON alert_evidence_manifests(tenant_id,state,observed_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_alert_evidence_manifest_object_version ON alert_evidence_manifests(tenant_id,object_bucket,object_key,object_version) WHERE source_store='minio';

CREATE TABLE IF NOT EXISTS alert_evidence_manifest_history (
  tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, evidence_id TEXT NOT NULL, event_id TEXT NOT NULL DEFAULT '', evidence_type TEXT NOT NULL,
  source_store TEXT NOT NULL, object_bucket TEXT NOT NULL DEFAULT '', object_key TEXT NOT NULL DEFAULT '', object_version TEXT NOT NULL DEFAULT '', object_sha256 TEXT NOT NULL DEFAULT '',
  size_bytes BIGINT NOT NULL, content_type TEXT NOT NULL DEFAULT '', state TEXT NOT NULL, revision BIGINT NOT NULL CHECK(revision>0),
  source_watermarks JSONB NOT NULL, observed_at TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ, recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY(tenant_id,alert_id,evidence_id,revision)
);

CREATE OR REPLACE FUNCTION enforce_alert_evidence_manifest_revision() RETURNS trigger LANGUAGE plpgsql AS $body$
BEGIN
  IF TG_OP='UPDATE' THEN
    IF NEW.revision<=OLD.revision THEN RAISE EXCEPTION 'alert evidence manifest revision must increase'; END IF;
    IF (NEW.tenant_id,NEW.alert_id,NEW.evidence_id,NEW.source_store,NEW.object_bucket,NEW.object_key,NEW.object_version,NEW.object_sha256,NEW.size_bytes)
       IS DISTINCT FROM (OLD.tenant_id,OLD.alert_id,OLD.evidence_id,OLD.source_store,OLD.object_bucket,OLD.object_key,OLD.object_version,OLD.object_sha256,OLD.size_bytes)
    THEN RAISE EXCEPTION 'immutable alert evidence identity or object reference changed'; END IF;
    NEW.updated_at=now();
  END IF;
  RETURN NEW;
END $body$;
DROP TRIGGER IF EXISTS trg_alert_evidence_manifest_revision ON alert_evidence_manifests;
CREATE TRIGGER trg_alert_evidence_manifest_revision BEFORE UPDATE ON alert_evidence_manifests FOR EACH ROW EXECUTE FUNCTION enforce_alert_evidence_manifest_revision();

CREATE OR REPLACE FUNCTION record_alert_evidence_manifest_history() RETURNS trigger LANGUAGE plpgsql AS $body$
BEGIN
  INSERT INTO alert_evidence_manifest_history(tenant_id,alert_id,evidence_id,event_id,evidence_type,source_store,object_bucket,object_key,object_version,object_sha256,size_bytes,content_type,state,revision,source_watermarks,observed_at,expires_at)
  VALUES(NEW.tenant_id,NEW.alert_id,NEW.evidence_id,NEW.event_id,NEW.evidence_type,NEW.source_store,NEW.object_bucket,NEW.object_key,NEW.object_version,NEW.object_sha256,NEW.size_bytes,NEW.content_type,NEW.state,NEW.revision,NEW.source_watermarks,NEW.observed_at,NEW.expires_at)
  ON CONFLICT DO NOTHING;
  RETURN NEW;
END $body$;
DROP TRIGGER IF EXISTS trg_alert_evidence_manifest_history ON alert_evidence_manifests;
CREATE TRIGGER trg_alert_evidence_manifest_history AFTER INSERT OR UPDATE ON alert_evidence_manifests FOR EACH ROW EXECUTE FUNCTION record_alert_evidence_manifest_history();

CREATE TABLE IF NOT EXISTS alignment_schema_migrations(version TEXT PRIMARY KEY,description TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),applied_by TEXT NOT NULL DEFAULT current_user);
INSERT INTO alignment_schema_migrations(version,description) VALUES('202608091700','tenant-bound immutable alert evidence manifest v1') ON CONFLICT(version) DO NOTHING;
COMMIT;
