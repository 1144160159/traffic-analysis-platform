-- T-CH-001 / WP-22
-- Bootstrap the immutable ClickHouse migration ledger. Migrations are applied
-- directly to every ClickHouse node by scripts/clickhouse/run-migrations.sh;
-- therefore this file intentionally does not use ON CLUSTER.
--
-- The runner records the migration filename checksum only after this DDL has
-- succeeded. A checksum mismatch for an already applied version is fatal.

CREATE DATABASE IF NOT EXISTS traffic;

CREATE TABLE IF NOT EXISTS traffic.alignment_schema_migrations_local (
  version       String,
  checksum      FixedString(64),
  description   String,
  applied_at    DateTime64(3, 'UTC') DEFAULT now64(3),
  applied_by    String DEFAULT 'clickhouse-migration-runner'
)
ENGINE = ReplicatedReplacingMergeTree(
  '/clickhouse/tables/{shard}/alignment_schema_migrations_local',
  '{replica}',
  applied_at
)
ORDER BY version
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.alignment_schema_migrations
AS traffic.alignment_schema_migrations_local
ENGINE = Distributed(
  traffic_cluster,
  traffic,
  alignment_schema_migrations_local,
  cityHash64(version)
);
