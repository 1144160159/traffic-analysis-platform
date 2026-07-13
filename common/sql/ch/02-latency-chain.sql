-- Latency-chain compatibility columns for GATE-P0-05.
-- Safe to run repeatedly on an existing ClickHouse cluster.

ALTER TABLE traffic.flows_raw_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER ingest_ts;
ALTER TABLE traffic.flows_raw_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;
ALTER TABLE traffic.flows_raw ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER ingest_ts;
ALTER TABLE traffic.flows_raw ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;

ALTER TABLE traffic.sessions_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER ingest_ts;
ALTER TABLE traffic.sessions_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;
ALTER TABLE traffic.sessions ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER ingest_ts;
ALTER TABLE traffic.sessions ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;

ALTER TABLE traffic.alerts_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER event_id;
ALTER TABLE traffic.alerts_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;
ALTER TABLE traffic.alerts ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS kafka_ts Int64 AFTER event_id;
ALTER TABLE traffic.alerts ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS flink_out_ts Int64 AFTER kafka_ts;
