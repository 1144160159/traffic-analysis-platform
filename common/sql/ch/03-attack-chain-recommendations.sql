-- Attack-chain response recommendations used by the four recommendation tabs.
-- Safe to run repeatedly on an existing ClickHouse cluster.

CREATE TABLE IF NOT EXISTS traffic.attack_chain_recommendations_local ON CLUSTER traffic_cluster (
  tenant_id          String,
  campaign_id        String,
  recommendation_id String,
  category           LowCardinality(String),
  phase              String,
  priority           LowCardinality(String),
  target             String,
  action             String,
  impact             LowCardinality(String),
  sort_order         UInt16,
  created_at         Int64
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/attack_chain_recommendations', '{replica}')
PARTITION BY toYYYYMMDD(toDateTime(created_at / 1000))
ORDER BY (tenant_id, campaign_id, category, sort_order, recommendation_id)
TTL toDateTime(created_at / 1000) + toIntervalDay(30)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS traffic.attack_chain_recommendations ON CLUSTER traffic_cluster
AS traffic.attack_chain_recommendations_local
ENGINE = Distributed(traffic_cluster, traffic, attack_chain_recommendations_local, rand());
