-- F-ALERT-002 / WP-07
-- Expand: add the authoritative relation, append-only history and transactional outbox.
-- Backfill: no synthetic links are created; existing ClickHouse campaign arrays remain read-only evidence.
-- Verify:
--   SELECT count(*) FROM campaign_alert_links WHERE status='linked';
--   SELECT count(*) FROM campaign_alert_link_history;
--   SELECT count(*) FROM campaign_alert_link_outbox WHERE published=false;
-- Cutover: enable campaign_alert_links_v1 only after the three tables and indexes exist.
-- Rollback: disable the feature flag and keep all rows; do not drop relation history or outbox evidence.

BEGIN;

CREATE TABLE IF NOT EXISTS campaign_alert_links (
  relation_id     UUID PRIMARY KEY,
  tenant_id       TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id     TEXT NOT NULL,
  alert_id        TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'linked'
                  CHECK (status IN ('linked','unlinked')),
  revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  reason          TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  created_by      TEXT NOT NULL DEFAULT '',
  updated_by      TEXT NOT NULL DEFAULT '',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, campaign_id, alert_id),
  UNIQUE (tenant_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_alert
  ON campaign_alert_links (tenant_id, alert_id, updated_at DESC)
  WHERE status='linked';
CREATE INDEX IF NOT EXISTS idx_campaign_alert_links_campaign
  ON campaign_alert_links (tenant_id, campaign_id, updated_at DESC)
  WHERE status='linked';

CREATE TABLE IF NOT EXISTS campaign_alert_link_history (
  event_id     UUID PRIMARY KEY,
  relation_id  UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  tenant_id    TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  campaign_id  TEXT NOT NULL,
  alert_id     TEXT NOT NULL,
  event_type   TEXT NOT NULL CHECK (event_type IN ('linked','unlinked')),
  revision     BIGINT NOT NULL CHECK (revision > 0),
  payload      JSONB NOT NULL,
  created_by   TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (relation_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_history_relation
  ON campaign_alert_link_history (tenant_id, relation_id, revision);

CREATE TABLE IF NOT EXISTS campaign_alert_link_outbox (
  event_id          UUID PRIMARY KEY REFERENCES campaign_alert_link_history(event_id) ON DELETE RESTRICT,
  tenant_id         TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  aggregate_id      UUID NOT NULL REFERENCES campaign_alert_links(relation_id) ON DELETE RESTRICT,
  aggregate_version BIGINT NOT NULL CHECK (aggregate_version > 0),
  event_type        TEXT NOT NULL,
  schema_version    INTEGER NOT NULL DEFAULT 2 CHECK (schema_version > 0),
  partition_key     TEXT NOT NULL,
  payload           JSONB NOT NULL,
  published         BOOLEAN NOT NULL DEFAULT false,
  attempts          INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error        TEXT NOT NULL DEFAULT '',
  next_attempt_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until      TIMESTAMPTZ,
  locked_by         TEXT NOT NULL DEFAULT '',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at      TIMESTAMPTZ,
  UNIQUE (aggregate_id, aggregate_version)
);
CREATE INDEX IF NOT EXISTS idx_campaign_alert_link_outbox_pending
  ON campaign_alert_link_outbox (next_attempt_at, created_at)
  WHERE published=false;

CREATE TABLE IF NOT EXISTS alignment_schema_migrations (
  version     TEXT PRIMARY KEY,
  description TEXT NOT NULL,
  applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  applied_by  TEXT NOT NULL DEFAULT current_user
);
INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202607301930','F-ALERT authoritative campaign relations')
ON CONFLICT (version) DO NOTHING;

COMMIT;
