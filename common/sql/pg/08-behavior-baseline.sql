BEGIN;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS behavior_baseline_resets (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id TEXT NOT NULL,
  reset_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  requested_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (tenant_id, baseline_id)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_settings (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id TEXT NOT NULL,
  warning_multiplier DOUBLE PRECISION NOT NULL DEFAULT 2.0 CHECK (warning_multiplier > 0),
  alert_multiplier DOUBLE PRECISION NOT NULL DEFAULT 3.0 CHECK (alert_multiplier > warning_multiplier),
  frozen BOOLEAN NOT NULL DEFAULT false,
  drift_watch BOOLEAN NOT NULL DEFAULT false,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, baseline_id)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_actions (
  action_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id TEXT NOT NULL,
  action_type TEXT NOT NULL CHECK (action_type IN ('create_alert','adjust_threshold','freeze','unfreeze','forensics','feedback_model','cold_start','drift_watch','rebuild','rollback','audit_trace')),
  status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','applied','rejected','failed')),
  reason TEXT NOT NULL DEFAULT '',
  request JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_actions_time ON behavior_baseline_actions (tenant_id, baseline_id, created_at DESC);

CREATE TABLE IF NOT EXISTS behavior_baseline_versions (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version > 0),
  snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_action_id UUID NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE SET NULL,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, baseline_id, version)
);

CREATE TABLE IF NOT EXISTS behavior_baseline_outbox (
  outbox_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  baseline_id TEXT NOT NULL,
  action_id UUID NOT NULL REFERENCES behavior_baseline_actions(action_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  published BOOLEAN NOT NULL DEFAULT false,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_behavior_baseline_outbox_pending ON behavior_baseline_outbox (published, created_at) WHERE published=false;

COMMIT;
