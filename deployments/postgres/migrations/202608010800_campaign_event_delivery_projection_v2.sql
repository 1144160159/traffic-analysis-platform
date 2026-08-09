-- F-CAMPAIGN-001 / F-ALERT-002 / T-PG-001 / T-KAFKA-001
-- Expand: add recoverable delivery state to both campaign outboxes and a
-- durable PostgreSQL inbox boundary for the aggregate and membership streams.
-- The two streams may share an event_id for one membership transaction, so
-- consumer identity is the composite (stream,event_id), never event_id alone.
-- Backfill: published booleans are preserved as status='published'; legacy
-- membership payloads receive only deterministic envelope fields already held
-- in authoritative outbox columns. No business state or success is synthesized.
-- Verify:
--   SELECT version FROM alignment_schema_migrations WHERE version='202608010800';
--   SELECT status,count(*) FROM campaign_aggregate_outbox GROUP BY status;
--   SELECT status,count(*) FROM campaign_alert_link_outbox GROUP BY status;
--   SELECT stream,count(*) FROM campaign_event_projection_inbox GROUP BY stream;
-- Rollback: set CAMPAIGN_EVENT_PIPELINE_V2_ENABLED=false and roll back the
-- service image. Retain delivery state, inbox, delivery positions and watermarks.

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';

ALTER TABLE campaign_aggregate_outbox
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE campaign_aggregate_outbox
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE campaign_aggregate_outbox
  ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
UPDATE campaign_aggregate_outbox
SET status='published'
WHERE published=true AND status='pending';
ALTER TABLE campaign_aggregate_outbox
  DROP CONSTRAINT IF EXISTS campaign_aggregate_outbox_status_check;
ALTER TABLE campaign_aggregate_outbox
  ADD CONSTRAINT campaign_aggregate_outbox_status_check
  CHECK (status IN ('pending','processing','published','dead'));
CREATE INDEX IF NOT EXISTS idx_campaign_aggregate_outbox_delivery
  ON campaign_aggregate_outbox(next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','processing');

ALTER TABLE campaign_alert_link_outbox
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE campaign_alert_link_outbox
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;
ALTER TABLE campaign_alert_link_outbox
  ADD COLUMN IF NOT EXISTS dead_at TIMESTAMPTZ;
UPDATE campaign_alert_link_outbox
SET status='published'
WHERE published=true AND status='pending';
ALTER TABLE campaign_alert_link_outbox
  DROP CONSTRAINT IF EXISTS campaign_alert_link_outbox_status_check;
ALTER TABLE campaign_alert_link_outbox
  ADD CONSTRAINT campaign_alert_link_outbox_status_check
  CHECK (status IN ('pending','processing','published','dead'));

-- Old V1 writers did not place the event type and full stable identity in the
-- JSON body. These values already exist in immutable outbox columns and the
-- relation row, so this is a deterministic envelope repair rather than a
-- synthetic business event.
UPDATE campaign_alert_link_outbox outbox
SET payload = outbox.payload || jsonb_build_object(
  'event_id',outbox.event_id::text,
  'event_type',outbox.event_type,
  'schema_version',outbox.schema_version,
  'aggregate_type','campaign_alert_link',
  'aggregate_id',outbox.aggregate_id::text,
  'aggregate_version',outbox.aggregate_version,
  'partition_key',outbox.partition_key,
  'relation_id',outbox.aggregate_id::text,
  'relation_revision',outbox.aggregate_version,
  'campaign_revision',CASE
    WHEN COALESCE(outbox.payload->>'campaign_revision','') ~ '^[0-9]+$'
      THEN (outbox.payload->>'campaign_revision')::bigint
    ELSE 0
  END,
  'trace_id',COALESCE(outbox.payload->>'trace_id','legacy-backfill')
)
WHERE NOT (outbox.payload ? 'event_type')
   OR NOT (outbox.payload ? 'relation_id')
   OR NOT (outbox.payload ? 'trace_id');

CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_delivery
  ON campaign_alert_link_outbox(next_attempt_at,created_at,event_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS campaign_event_projection_inbox (
  stream               TEXT NOT NULL CHECK (stream IN ('aggregate','membership')),
  event_id              UUID NOT NULL,
  tenant_id             TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  aggregate_id          TEXT NOT NULL,
  campaign_id           TEXT NOT NULL,
  relation_id           UUID,
  alert_id              TEXT NOT NULL DEFAULT '',
  event_type            TEXT NOT NULL,
  schema_version        INTEGER NOT NULL CHECK (schema_version=2),
  aggregate_revision    BIGINT NOT NULL CHECK (aggregate_revision>=0),
  relation_revision     BIGINT NOT NULL DEFAULT 0 CHECK (relation_revision>=0),
  partition_key         TEXT NOT NULL,
  trace_id              TEXT NOT NULL,
  payload               JSONB NOT NULL,
  projection_status     TEXT NOT NULL DEFAULT 'pending'
                        CHECK (projection_status IN ('pending','processing','partial','applied','dead')),
  target_status         JSONB NOT NULL DEFAULT
                        '{"clickhouse":"pending","opensearch":"pending","nebulagraph":"pending"}'::jsonb,
  first_kafka_topic     TEXT NOT NULL,
  first_kafka_partition INTEGER NOT NULL CHECK (first_kafka_partition>=0),
  first_kafka_offset    BIGINT NOT NULL CHECK (first_kafka_offset>=0),
  received_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (stream,event_id),
  CHECK (
    (stream='aggregate' AND aggregate_revision>0 AND relation_revision=0 AND relation_id IS NULL)
    OR
    (stream='membership' AND relation_revision>0 AND relation_id IS NOT NULL)
  )
);
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_pending
  ON campaign_event_projection_inbox(projection_status,received_at,stream,event_id)
  WHERE projection_status IN ('pending','processing','partial');
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_campaign
  ON campaign_event_projection_inbox(tenant_id,campaign_id,aggregate_revision DESC,relation_revision DESC);

CREATE TABLE IF NOT EXISTS campaign_event_projection_deliveries (
  kafka_topic     TEXT NOT NULL,
  kafka_partition INTEGER NOT NULL CHECK (kafka_partition>=0),
  kafka_offset    BIGINT NOT NULL CHECK (kafka_offset>=0),
  stream          TEXT NOT NULL,
  event_id        UUID NOT NULL,
  received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kafka_topic,kafka_partition,kafka_offset),
  FOREIGN KEY (stream,event_id)
    REFERENCES campaign_event_projection_inbox(stream,event_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_campaign_event_projection_delivery_event
  ON campaign_event_projection_deliveries(stream,event_id,received_at);

CREATE TABLE IF NOT EXISTS campaign_event_projection_watermarks (
  kafka_topic       TEXT NOT NULL,
  kafka_partition   INTEGER NOT NULL CHECK (kafka_partition>=0),
  last_offset       BIGINT NOT NULL CHECK (last_offset>=0),
  last_event_id     UUID NOT NULL,
  last_stream       TEXT NOT NULL CHECK (last_stream IN ('aggregate','membership')),
  last_received_at  TIMESTAMPTZ NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (kafka_topic,kafka_partition)
);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608010800','campaign event outbox delivery and projection inbox v2')
ON CONFLICT (version) DO NOTHING;

COMMIT;
