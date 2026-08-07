BEGIN;

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0);

CREATE TABLE IF NOT EXISTS user_command_history (
  history_id BIGSERIAL PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  revision BIGINT NOT NULL CHECK(revision>0),
  action_id TEXT NOT NULL,
  actor_id UUID,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  old_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  new_value JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,user_id,revision,action_id)
);
CREATE INDEX IF NOT EXISTS idx_user_command_history_lookup
  ON user_command_history(tenant_id,user_id,revision DESC);

CREATE TABLE IF NOT EXISTS user_command_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL CHECK(aggregate_version>0),
  event_type TEXT NOT NULL,
  schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version=1),
  partition_key TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','processing','published','dead')),
  publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK(publish_attempts>=0),
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until TIMESTAMPTZ,
  locked_by TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE(tenant_id,user_id,aggregate_version,event_type)
);
CREATE INDEX IF NOT EXISTS idx_user_command_outbox_ready
  ON user_command_outbox(next_retry_at,occurred_at,outbox_id)
  WHERE status IN ('pending','processing');

CREATE TABLE IF NOT EXISTS user_command_requests (
  request_id UUID PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  user_id UUID NOT NULL,
  idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 16 AND 200),
  request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
  action_id TEXT NOT NULL,
  expected_revision BIGINT NOT NULL CHECK(expected_revision>=0),
  resulting_revision BIGINT NOT NULL CHECK(resulting_revision>0),
  response_payload JSONB NOT NULL,
  event_id UUID NOT NULL REFERENCES user_command_outbox(event_id) ON DELETE RESTRICT,
  actor_id UUID,
  reason TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  compatibility_mode BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(tenant_id,idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_user_command_requests_user
  ON user_command_requests(tenant_id,user_id,created_at DESC);

INSERT INTO alignment_schema_migrations(version,description)
VALUES ('202608031530','T-PG-002 make authenticated user mutations atomic')
ON CONFLICT (version) DO NOTHING;

COMMIT;
