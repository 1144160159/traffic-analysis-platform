-- T1-M09-N012 / P026-P028
-- Additive alert/evidence relationship authority.  Evidence object identity is
-- copied from the immutable manifest and cannot be rewritten by a later link.
-- Rollback is flag-only: stop intake/dispatch and retain relation, command,
-- history, audit and outbox facts.  No evidence object is deleted here.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

CREATE TABLE IF NOT EXISTS alert_evidence_links (
  relation_id       UUID PRIMARY KEY,
  tenant_id         TEXT NOT NULL CHECK (tenant_id<>''),
  alert_id          TEXT NOT NULL CHECK (alert_id<>''),
  evidence_id       TEXT NOT NULL CHECK (evidence_id<>''),
  evidence_type     TEXT NOT NULL CHECK (evidence_type<>''),
  source_store      TEXT NOT NULL CHECK (source_store IN ('postgresql','clickhouse','opensearch','minio','arkime')),
  object_bucket     TEXT NOT NULL DEFAULT '',
  object_key        TEXT NOT NULL DEFAULT '',
  object_version    TEXT NOT NULL DEFAULT '',
  object_sha256     TEXT NOT NULL DEFAULT '',
  size_bytes        BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes>=0),
  content_type      TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL CHECK (status IN ('linked','unlinked')),
  revision          BIGINT NOT NULL CHECK (revision>0),
  last_event_id     UUID NOT NULL,
  reason            TEXT NOT NULL CHECK (char_length(reason) BETWEEN 4 AND 1000),
  created_by        TEXT NOT NULL DEFAULT '',
  updated_by        TEXT NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,alert_id,evidence_id),
  FOREIGN KEY (tenant_id,alert_id,evidence_id)
    REFERENCES alert_evidence_manifests(tenant_id,alert_id,evidence_id) ON DELETE RESTRICT,
  CHECK (source_store<>'minio' OR (
    object_bucket<>'' AND object_key LIKE ('tenants/'||tenant_id||'/%') AND
    object_key NOT LIKE '%..%' AND object_version<>'' AND object_sha256 ~ '^[0-9a-f]{64}$'
  ))
);

CREATE INDEX IF NOT EXISTS idx_alert_evidence_links_active
  ON alert_evidence_links(tenant_id,alert_id,updated_at DESC,evidence_id)
  WHERE status='linked';

CREATE TABLE IF NOT EXISTS alert_evidence_link_history (
  event_id           UUID PRIMARY KEY,
  relation_id        UUID NOT NULL REFERENCES alert_evidence_links(relation_id) ON DELETE RESTRICT,
  tenant_id          TEXT NOT NULL,
  alert_id           TEXT NOT NULL,
  evidence_id        TEXT NOT NULL,
  event_type         TEXT NOT NULL CHECK (event_type IN ('linked','unlinked')),
  relation_revision  BIGINT NOT NULL CHECK (relation_revision>0),
  source_store       TEXT NOT NULL,
  object_bucket      TEXT NOT NULL DEFAULT '',
  object_key         TEXT NOT NULL DEFAULT '',
  object_version     TEXT NOT NULL DEFAULT '',
  object_sha256      TEXT NOT NULL DEFAULT '',
  request_sha256     TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  payload            JSONB NOT NULL CHECK (jsonb_typeof(payload)='object'),
  reason             TEXT NOT NULL,
  created_by         TEXT NOT NULL DEFAULT '',
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (relation_id,relation_revision)
);

CREATE INDEX IF NOT EXISTS idx_alert_evidence_link_history_lookup
  ON alert_evidence_link_history(tenant_id,alert_id,evidence_id,relation_revision DESC);

CREATE TABLE IF NOT EXISTS alert_evidence_link_commands (
  command_id          UUID PRIMARY KEY,
  tenant_id           TEXT NOT NULL,
  relation_id         UUID NOT NULL REFERENCES alert_evidence_links(relation_id) ON DELETE RESTRICT,
  alert_id            TEXT NOT NULL,
  evidence_id         TEXT NOT NULL,
  operation           TEXT NOT NULL CHECK (operation IN ('link','unlink')),
  idempotency_key     TEXT NOT NULL CHECK (char_length(idempotency_key) BETWEEN 16 AND 200),
  request_sha256      TEXT NOT NULL CHECK (request_sha256 ~ '^[0-9a-f]{64}$'),
  expected_revision   BIGINT NOT NULL CHECK (expected_revision>=0),
  relation_revision   BIGINT NOT NULL CHECK (relation_revision>0),
  result              JSONB NOT NULL CHECK (jsonb_typeof(result)='object'),
  created_by          TEXT NOT NULL DEFAULT '',
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id,idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_alert_evidence_link_commands_relation
  ON alert_evidence_link_commands(tenant_id,relation_id,created_at DESC);

CREATE TABLE IF NOT EXISTS alert_evidence_link_outbox (
  event_id             UUID PRIMARY KEY REFERENCES alert_evidence_link_history(event_id) ON DELETE RESTRICT,
  tenant_id            TEXT NOT NULL,
  aggregate_id         UUID NOT NULL REFERENCES alert_evidence_links(relation_id) ON DELETE RESTRICT,
  aggregate_version    BIGINT NOT NULL CHECK (aggregate_version>0),
  event_type           TEXT NOT NULL CHECK (event_type IN ('traffic.alert-evidence.v1.Linked','traffic.alert-evidence.v1.Unlinked')),
  schema_version       INTEGER NOT NULL DEFAULT 1 CHECK (schema_version=1),
  partition_key        TEXT NOT NULL CHECK (partition_key<>''),
  payload              JSONB NOT NULL CHECK (jsonb_typeof(payload)='object'),
  status               TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','published','dead')),
  attempts             INTEGER NOT NULL DEFAULT 0 CHECK (attempts>=0),
  next_attempt_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_attempt_at      TIMESTAMPTZ,
  locked_until         TIMESTAMPTZ,
  locked_by            TEXT NOT NULL DEFAULT '',
  last_error           TEXT NOT NULL DEFAULT '',
  broker_partition     INTEGER,
  broker_offset        BIGINT,
  broker_acknowledged_at TIMESTAMPTZ,
  created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at         TIMESTAMPTZ,
  dead_at              TIMESTAMPTZ,
  UNIQUE (aggregate_id,aggregate_version),
  CHECK ((status='published' AND published_at IS NOT NULL AND broker_partition IS NOT NULL AND broker_offset IS NOT NULL AND broker_acknowledged_at IS NOT NULL)
      OR status<>'published')
);

CREATE INDEX IF NOT EXISTS idx_alert_evidence_link_outbox_delivery
  ON alert_evidence_link_outbox(next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS alert_evidence_link_projection_inbox (
  event_id              UUID PRIMARY KEY,
  tenant_id             TEXT NOT NULL,
  aggregate_id          UUID NOT NULL,
  aggregate_version     BIGINT NOT NULL CHECK (aggregate_version>0),
  event_type            TEXT NOT NULL,
  partition_key         TEXT NOT NULL,
  payload_sha256        TEXT NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  payload               JSONB NOT NULL CHECK (jsonb_typeof(payload)='object'),
  first_kafka_topic     TEXT NOT NULL,
  first_kafka_partition INTEGER NOT NULL,
  first_kafka_offset    BIGINT NOT NULL,
  projection_status     TEXT NOT NULL DEFAULT 'pending' CHECK (projection_status IN ('pending','projected')),
  projection_attempts   INTEGER NOT NULL DEFAULT 0 CHECK (projection_attempts>=0),
  last_error            TEXT NOT NULL DEFAULT '',
  received_at           TIMESTAMPTZ NOT NULL,
  projected_at          TIMESTAMPTZ,
  UNIQUE (tenant_id,aggregate_id,aggregate_version)
);

CREATE TABLE IF NOT EXISTS alert_evidence_link_projection_deliveries (
  kafka_topic      TEXT NOT NULL,
  kafka_partition  INTEGER NOT NULL,
  kafka_offset     BIGINT NOT NULL,
  event_id         UUID NOT NULL REFERENCES alert_evidence_link_projection_inbox(event_id) ON DELETE RESTRICT,
  received_at      TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (kafka_topic,kafka_partition,kafka_offset)
);

CREATE TABLE IF NOT EXISTS alert_evidence_link_projection_watermarks (
  kafka_topic      TEXT NOT NULL,
  kafka_partition  INTEGER NOT NULL,
  last_offset      BIGINT NOT NULL,
  last_event_id    UUID NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kafka_topic,kafka_partition)
);

CREATE OR REPLACE FUNCTION enforce_alert_evidence_link_identity()
RETURNS trigger LANGUAGE plpgsql AS $body$
BEGIN
  IF TG_OP='UPDATE' THEN
    IF NEW.revision<=OLD.revision THEN
      RAISE EXCEPTION 'alert evidence link revision must increase';
    END IF;
    IF (NEW.relation_id,NEW.tenant_id,NEW.alert_id,NEW.evidence_id,NEW.evidence_type,
        NEW.source_store,NEW.object_bucket,NEW.object_key,NEW.object_version,
        NEW.object_sha256,NEW.size_bytes,NEW.content_type,NEW.created_by,NEW.created_at)
       IS DISTINCT FROM
       (OLD.relation_id,OLD.tenant_id,OLD.alert_id,OLD.evidence_id,OLD.evidence_type,
        OLD.source_store,OLD.object_bucket,OLD.object_key,OLD.object_version,
        OLD.object_sha256,OLD.size_bytes,OLD.content_type,OLD.created_by,OLD.created_at) THEN
      RAISE EXCEPTION 'immutable alert evidence link identity or object reference changed';
    END IF;
    NEW.updated_at=now();
  END IF;
  RETURN NEW;
END
$body$;

DROP TRIGGER IF EXISTS trg_alert_evidence_link_identity ON alert_evidence_links;
CREATE TRIGGER trg_alert_evidence_link_identity
BEFORE UPDATE ON alert_evidence_links
FOR EACH ROW EXECUTE FUNCTION enforce_alert_evidence_link_identity();

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608160030','M09 alert evidence link command history outbox v1')
ON CONFLICT (version) DO NOTHING;

COMMIT;
