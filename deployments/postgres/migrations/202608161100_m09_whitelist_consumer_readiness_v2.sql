-- F-WHITELIST-001 / T1-M09-N018
-- Consumer-first admission receipt for whitelist.events.v2. This migration is
-- expand-only and does not enable the producer, matcher or any network action.

BEGIN;

CREATE TABLE IF NOT EXISTS whitelist_consumer_readiness_receipt (
  consumer_group    TEXT NOT NULL,
  candidate_sha256  CHAR(64) NOT NULL CHECK (candidate_sha256 ~ '^[0-9a-f]{64}$'),
  contract_sha256   CHAR(64) NOT NULL CHECK (contract_sha256 ~ '^[0-9a-f]{64}$'),
  kafka_topic       TEXT NOT NULL CHECK (kafka_topic='whitelist.events.v2'),
  state             TEXT NOT NULL CHECK (state IN ('READY','STOPPED')),
  event_id          UUID,
  kafka_partition   INTEGER CHECK (kafka_partition IS NULL OR kafka_partition >= 0),
  kafka_offset      BIGINT CHECK (kafka_offset IS NULL OR kafka_offset >= 0),
  observed_at       TIMESTAMPTZ NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer_group,candidate_sha256),
  UNIQUE (kafka_topic,consumer_group,candidate_sha256),
  CHECK ((state='READY' AND event_id IS NOT NULL AND kafka_partition IS NOT NULL AND kafka_offset IS NOT NULL)
      OR (state='STOPPED' AND event_id IS NULL AND kafka_partition IS NULL AND kafka_offset IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_whitelist_consumer_readiness_lookup
  ON whitelist_consumer_readiness_receipt
  (kafka_topic,consumer_group,candidate_sha256,contract_sha256,state);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608161100','M09 whitelist consumer broker projection readiness receipt')
ON CONFLICT (version) DO NOTHING;

COMMIT;
