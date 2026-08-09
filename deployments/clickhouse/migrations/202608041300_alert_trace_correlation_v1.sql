-- T-OBS-001 / WP-03
-- Expand-only alert trace correlation. The authoritative migration runner
-- applies this file on every ClickHouse node without ON CLUSTER.
--
-- Add the shadow/latest columns before the source columns so the existing
-- SELECT * materialized view can continue writing while the migration expands.
-- Existing rows receive an empty trace_id and remain query-compatible. New
-- writers must supply the W3C trace ID after every node has completed expand.
-- Rollback is application-only during the compatibility window; do not drop
-- acknowledged trace data.

ALTER TABLE traffic.alerts_latest_local
  ADD COLUMN IF NOT EXISTS trace_id String AFTER event_id;

ALTER TABLE traffic.alerts_latest
  ADD COLUMN IF NOT EXISTS trace_id String AFTER event_id;

ALTER TABLE traffic.alerts_local
  ADD COLUMN IF NOT EXISTS trace_id String AFTER event_id;

ALTER TABLE traffic.alerts
  ADD COLUMN IF NOT EXISTS trace_id String AFTER event_id;
