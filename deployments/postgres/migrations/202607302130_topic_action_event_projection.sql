-- F-TOPIC-002 / T-KAFKA-001 / WP-09
-- Expand: add an idempotent read projection for traffic.topic.action.v2.
-- Verify:
--   SELECT event_type,count(*) FROM topic_action_event_projection GROUP BY event_type;
--   SELECT tenant_id,status,count(*) FROM topic_action_job_projection GROUP BY tenant_id,status;
--   SELECT p.job_id,p.revision,a.revision
--     FROM topic_action_job_projection p
--     JOIN topic_actions a ON a.action_id=p.job_id
--    WHERE p.revision<>a.revision;
-- Cutover: enable the consumer only after this migration is present.
-- Rollback: stop the consumer; projection tables are disposable and can be
-- rebuilt from traffic.topic.action.v2 without changing topic_actions.

BEGIN;

CREATE TABLE IF NOT EXISTS topic_action_event_projection (
  event_id          UUID PRIMARY KEY,
  job_id            UUID NOT NULL,
  tenant_id         TEXT NOT NULL,
  topic             TEXT NOT NULL,
  event_type        TEXT NOT NULL CHECK (
    event_type IN ('traffic.topic.v2.ActionRequested','traffic.topic.v2.ActionResult')
  ),
  revision          BIGINT NOT NULL CHECK (revision > 0),
  action_id         TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL DEFAULT '',
  trace_id           TEXT NOT NULL,
  payload            JSONB NOT NULL,
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  projected_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (kafka_partition,kafka_offset)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_event_projection_tenant_job
  ON topic_action_event_projection (tenant_id,job_id,revision);

CREATE TABLE IF NOT EXISTS topic_action_job_projection (
  tenant_id         TEXT NOT NULL,
  job_id            UUID NOT NULL,
  topic             TEXT NOT NULL,
  revision          BIGINT NOT NULL CHECK (revision > 0),
  event_type        TEXT NOT NULL,
  action_id         TEXT NOT NULL DEFAULT '',
  status            TEXT NOT NULL DEFAULT '',
  trace_id           TEXT NOT NULL,
  last_event_id      UUID NOT NULL,
  payload            JSONB NOT NULL,
  kafka_partition    INTEGER NOT NULL,
  kafka_offset       BIGINT NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,job_id)
);
CREATE INDEX IF NOT EXISTS idx_topic_action_job_projection_tenant_topic
  ON topic_action_job_projection (tenant_id,topic,updated_at DESC);

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607302130','traffic.topic.action.v2 idempotent event projection')
ON CONFLICT (version) DO NOTHING;

COMMIT;
